package mcpclient

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"awaken/src/desktopdata"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"

	_ "modernc.org/sqlite"
)

const credentialKeyringService = "awaken-mcp"

type secretBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

type keyringSecretBackend struct{}

func (keyringSecretBackend) Get(service, user string) (string, error) {
	value, err := keyring.Get(service, user)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrCredentialNotFound
		}
		return "", err
	}
	return value, nil
}

func (keyringSecretBackend) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

func (keyringSecretBackend) Delete(service, user string) error {
	if err := keyring.Delete(service, user); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

type PersistentCredentialStore struct {
	paths   desktopdata.Paths
	secrets secretBackend

	mu sync.Mutex
	db *sql.DB
}

func NewPersistentCredentialStore() *PersistentCredentialStore {
	return &PersistentCredentialStore{
		paths:   desktopdata.Detect(),
		secrets: keyringSecretBackend{},
	}
}

func (s *PersistentCredentialStore) LoadTokenSource(serverName string) (StoredTokenSource, error) {
	meta, err := s.metadata(serverName)
	if err != nil {
		return StoredTokenSource{}, err
	}
	payload, err := s.secrets.Get(credentialKeyringService, meta.CredentialRef)
	if err != nil {
		return StoredTokenSource{}, err
	}
	var stored StoredTokenSource
	if err := json.Unmarshal([]byte(payload), &stored); err != nil {
		return StoredTokenSource{}, fmt.Errorf("decode stored credential for %q: %w", serverName, err)
	}
	stored.CredentialRef = meta.CredentialRef
	stored.ServerURL = firstNonEmpty(stored.ServerURL, meta.ServerURL)
	stored.AuthURL = firstNonEmpty(stored.AuthURL, meta.AuthURL)
	stored.TokenURL = firstNonEmpty(stored.TokenURL, meta.TokenURL)
	stored.RedirectURL = firstNonEmpty(stored.RedirectURL, meta.RedirectURL)
	stored.ClientID = firstNonEmpty(stored.ClientID, meta.ClientID)
	if len(stored.Scopes) == 0 {
		stored.Scopes = append([]string(nil), meta.Scopes...)
	}
	stored.NeedsReauth = meta.State == "needs_reauth"
	return stored, nil
}

func (s *PersistentCredentialStore) SaveTokenSource(serverName string, source StoredTokenSource) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	if strings.TrimSpace(source.CredentialRef) == "" {
		source.CredentialRef = uuid.NewString()
	}
	payload, err := json.Marshal(source)
	if err != nil {
		return fmt.Errorf("encode credential for %q: %w", serverName, err)
	}
	if err := s.secrets.Set(credentialKeyringService, source.CredentialRef, string(payload)); err != nil {
		return fmt.Errorf("persist credential secret for %q: %w", serverName, err)
	}
	scopeJSON, err := json.Marshal(source.Scopes)
	if err != nil {
		return fmt.Errorf("encode scopes for %q: %w", serverName, err)
	}
	expiresAt := ""
	if source.Token != nil && !source.Token.Expiry.IsZero() {
		expiresAt = source.Token.Expiry.UTC().Format(time.RFC3339Nano)
	}
	state := "authorized"
	if source.NeedsReauth {
		state = "needs_reauth"
	}
	_, err = db.Exec(`
INSERT INTO connector_accounts (
	server_name, credential_ref, state, server_url, auth_url, token_url,
	redirect_url, client_id, scopes_json, updated_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(server_name) DO UPDATE SET
	credential_ref = excluded.credential_ref,
	state = excluded.state,
	server_url = excluded.server_url,
	auth_url = excluded.auth_url,
	token_url = excluded.token_url,
	redirect_url = excluded.redirect_url,
	client_id = excluded.client_id,
	scopes_json = excluded.scopes_json,
	updated_at = excluded.updated_at,
	expires_at = excluded.expires_at
`,
		serverName,
		source.CredentialRef,
		state,
		source.ServerURL,
		source.AuthURL,
		source.TokenURL,
		source.RedirectURL,
		source.ClientID,
		string(scopeJSON),
		time.Now().UTC().Format(time.RFC3339Nano),
		expiresAt,
	)
	if err != nil {
		return fmt.Errorf("upsert credential metadata for %q: %w", serverName, err)
	}
	return nil
}

func (s *PersistentCredentialStore) ClearTokenSource(serverName string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	meta, err := s.metadata(serverName)
	if err != nil && !errors.Is(err, ErrCredentialNotFound) {
		return err
	}
	if err == nil && strings.TrimSpace(meta.CredentialRef) != "" {
		if err := s.secrets.Delete(credentialKeyringService, meta.CredentialRef); err != nil {
			return fmt.Errorf("delete credential secret for %q: %w", serverName, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM connector_accounts WHERE server_name = ?`, serverName); err != nil {
		return fmt.Errorf("delete credential metadata for %q: %w", serverName, err)
	}
	return nil
}

func (s *PersistentCredentialStore) HasTokenSource(serverName string) (bool, error) {
	db, err := s.open()
	if err != nil {
		return false, err
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM connector_accounts WHERE server_name = ?`, serverName).Scan(&count); err != nil {
		return false, fmt.Errorf("count credential metadata for %q: %w", serverName, err)
	}
	return count > 0, nil
}

type credentialMetadata struct {
	CredentialRef string
	State         string
	ServerURL     string
	AuthURL       string
	TokenURL      string
	RedirectURL   string
	ClientID      string
	Scopes        []string
}

func (s *PersistentCredentialStore) metadata(serverName string) (credentialMetadata, error) {
	db, err := s.open()
	if err != nil {
		return credentialMetadata{}, err
	}
	var (
		meta      credentialMetadata
		scopesRaw string
	)
	err = db.QueryRow(`
SELECT credential_ref, state, server_url, auth_url, token_url, redirect_url, client_id, scopes_json
FROM connector_accounts
WHERE server_name = ?
`, serverName).Scan(
		&meta.CredentialRef,
		&meta.State,
		&meta.ServerURL,
		&meta.AuthURL,
		&meta.TokenURL,
		&meta.RedirectURL,
		&meta.ClientID,
		&scopesRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentialMetadata{}, ErrCredentialNotFound
		}
		return credentialMetadata{}, fmt.Errorf("query credential metadata for %q: %w", serverName, err)
	}
	if strings.TrimSpace(scopesRaw) != "" {
		if err := json.Unmarshal([]byte(scopesRaw), &meta.Scopes); err != nil {
			return credentialMetadata{}, fmt.Errorf("decode scopes for %q: %w", serverName, err)
		}
	}
	return meta, nil
}

func (s *PersistentCredentialStore) open() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	if err := s.paths.Ensure(); err != nil {
		return nil, fmt.Errorf("ensure desktop data dirs: %w", err)
	}
	db, err := sql.Open("sqlite", s.paths.ConnectorAccountsDBPath())
	if err != nil {
		return nil, fmt.Errorf("open credential db: %w", err)
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS connector_accounts (
	server_name TEXT PRIMARY KEY,
	credential_ref TEXT NOT NULL,
	state TEXT NOT NULL DEFAULT 'authorized',
	server_url TEXT NOT NULL DEFAULT '',
	auth_url TEXT NOT NULL DEFAULT '',
	token_url TEXT NOT NULL DEFAULT '',
	redirect_url TEXT NOT NULL DEFAULT '',
	client_id TEXT NOT NULL DEFAULT '',
	scopes_json TEXT NOT NULL DEFAULT '[]',
	updated_at TEXT NOT NULL,
	expires_at TEXT NOT NULL DEFAULT ''
);
`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init credential db: %w", err)
	}
	s.db = db
	return s.db, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
