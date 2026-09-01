package mcpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/desktopdata"
	"awaken/src/filestore"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const (
	credentialKeyringService       = "awaken-mcp"
	credentialMetadataVersion      = 1
	credentialSecretChunkSize      = 2000
	credentialSecretMaxChunks      = 128
	credentialSecretManifestPrefix = "awaken-keychain-chunks-v1:"
)

type credentialSecretManifest struct {
	Version  int    `json:"version"`
	Revision string `json:"revision"`
	Chunks   int    `json:"chunks"`
	SHA256   string `json:"sha256"`
}

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
	metadataPath         string
	secrets              secretBackend
	initErr              error
	writeMetadataForTest func(credentialMetadataFile) error

	mu sync.Mutex
}

type credentialMetadataFile struct {
	Version  int                           `json:"version"`
	Accounts map[string]credentialMetadata `json:"accounts"`
}

type credentialMetadata struct {
	CredentialRef string   `json:"credentialRef"`
	State         string   `json:"state"`
	ServerURL     string   `json:"serverUrl,omitempty"`
	AuthURL       string   `json:"authUrl,omitempty"`
	TokenURL      string   `json:"tokenUrl,omitempty"`
	RedirectURL   string   `json:"redirectUrl,omitempty"`
	ClientID      string   `json:"clientId,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	UpdatedAt     string   `json:"updatedAt"`
	ExpiresAt     string   `json:"expiresAt,omitempty"`
}

func NewPersistentCredentialStore() *PersistentCredentialStore {
	root := desktopdata.AwakenHomeRoot()
	var initErr error
	if root == "" {
		root, initErr = os.MkdirTemp("", "awaken-mcp-credentials-")
	}
	return &PersistentCredentialStore{
		metadataPath: filepath.Join(root, "auth", "connector-accounts.json"),
		secrets:      keyringSecretBackend{},
		initErr:      initErr,
	}
}

func (s *PersistentCredentialStore) LoadTokenSource(serverName string) (StoredTokenSource, error) {
	s.mu.Lock()
	meta, err := s.metadataLocked(serverName)
	s.mu.Unlock()
	if err != nil {
		return StoredTokenSource{}, err
	}
	payload, err := s.loadCredentialSecret(meta.CredentialRef)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withMetadataMutationLock(func() error {
		return s.saveTokenSourceLocked(serverName, source)
	})
}

func (s *PersistentCredentialStore) saveTokenSourceLocked(serverName string, source StoredTokenSource) error {
	file, err := s.readMetadataLocked()
	if err != nil {
		return err
	}
	if strings.TrimSpace(source.CredentialRef) == "" {
		if previous, ok := file.Accounts[serverName]; ok && strings.TrimSpace(previous.CredentialRef) != "" {
			source.CredentialRef = previous.CredentialRef
		} else {
			source.CredentialRef = uuid.NewString()
		}
	}
	payload, err := json.Marshal(source)
	if err != nil {
		return fmt.Errorf("encode credential for %q: %w", serverName, err)
	}
	previousPayload, previousErr := s.loadCredentialSecret(source.CredentialRef)
	hadPreviousSecret := previousErr == nil
	if previousErr != nil && !errors.Is(previousErr, ErrCredentialNotFound) {
		return fmt.Errorf("read previous credential secret for %q: %w", serverName, previousErr)
	}
	if err := s.saveCredentialSecret(source.CredentialRef, string(payload)); err != nil {
		return fmt.Errorf("persist credential secret for %q: %w", serverName, err)
	}

	expiresAt := ""
	if source.Token != nil && !source.Token.Expiry.IsZero() {
		expiresAt = source.Token.Expiry.UTC().Format(time.RFC3339Nano)
	}
	state := "authorized"
	if source.NeedsReauth {
		state = "needs_reauth"
	}
	file.Accounts[serverName] = credentialMetadata{
		CredentialRef: source.CredentialRef,
		State:         state,
		ServerURL:     source.ServerURL,
		AuthURL:       source.AuthURL,
		TokenURL:      source.TokenURL,
		RedirectURL:   source.RedirectURL,
		ClientID:      source.ClientID,
		Scopes:        append([]string(nil), source.Scopes...),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		ExpiresAt:     expiresAt,
	}
	if err := s.writeMetadataLocked(file); err != nil {
		rollbackErr := s.restoreSecret(source.CredentialRef, previousPayload, hadPreviousSecret)
		return errors.Join(
			fmt.Errorf("upsert credential metadata for %q: %w", serverName, err),
			rollbackErr,
		)
	}
	return nil
}

func (s *PersistentCredentialStore) ClearTokenSource(serverName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withMetadataMutationLock(func() error {
		return s.clearTokenSourceLocked(serverName)
	})
}

func (s *PersistentCredentialStore) clearTokenSourceLocked(serverName string) error {
	file, err := s.readMetadataLocked()
	if err != nil {
		return err
	}
	meta, ok := file.Accounts[serverName]
	if !ok {
		return nil
	}
	previousPayload := ""
	hadPreviousSecret := false
	if strings.TrimSpace(meta.CredentialRef) != "" {
		var getErr error
		previousPayload, getErr = s.loadCredentialSecret(meta.CredentialRef)
		hadPreviousSecret = getErr == nil
		if getErr != nil && !errors.Is(getErr, ErrCredentialNotFound) {
			return fmt.Errorf("read credential before delete for %q: %w", serverName, getErr)
		}
		if err := s.deleteCredentialSecret(meta.CredentialRef); err != nil {
			rollbackErr := s.restoreSecret(meta.CredentialRef, previousPayload, hadPreviousSecret)
			return errors.Join(fmt.Errorf("delete credential secret for %q: %w", serverName, err), rollbackErr)
		}
	}
	delete(file.Accounts, serverName)
	if err := s.writeMetadataLocked(file); err != nil {
		rollbackErr := s.restoreSecret(meta.CredentialRef, previousPayload, hadPreviousSecret)
		return errors.Join(
			fmt.Errorf("delete credential metadata for %q: %w", serverName, err),
			rollbackErr,
		)
	}
	return nil
}

func (s *PersistentCredentialStore) restoreSecret(ref, previousPayload string, hadPrevious bool) error {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	if hadPrevious {
		if err := s.saveCredentialSecret(ref, previousPayload); err != nil {
			return fmt.Errorf("restore previous credential secret: %w", err)
		}
		return nil
	}
	if err := s.deleteCredentialSecret(ref); err != nil {
		return fmt.Errorf("remove unreferenced credential secret: %w", err)
	}
	return nil
}

func (s *PersistentCredentialStore) loadCredentialSecret(ref string) (string, error) {
	root, err := s.secrets.Get(credentialKeyringService, ref)
	if err != nil {
		return "", err
	}
	manifest, chunked, err := parseCredentialSecretManifest(root)
	if err != nil {
		return "", err
	}
	if !chunked {
		return root, nil
	}
	var payload strings.Builder
	payload.Grow(manifest.Chunks * credentialSecretChunkSize)
	for index := 0; index < manifest.Chunks; index++ {
		part, getErr := s.secrets.Get(credentialKeyringService, credentialSecretChunkRef(ref, manifest.Revision, index))
		if getErr != nil {
			return "", fmt.Errorf("read credential secret chunk %d of %d: %w", index+1, manifest.Chunks, getErr)
		}
		payload.WriteString(part)
	}
	joined := payload.String()
	digest := sha256.Sum256([]byte(joined))
	if hex.EncodeToString(digest[:]) != manifest.SHA256 {
		return "", errors.New("credential secret chunks failed integrity verification")
	}
	return joined, nil
}

func (s *PersistentCredentialStore) saveCredentialSecret(ref, payload string) error {
	previousRoot, previousErr := s.secrets.Get(credentialKeyringService, ref)
	hadPreviousRoot := previousErr == nil
	if previousErr != nil && !errors.Is(previousErr, ErrCredentialNotFound) {
		return previousErr
	}
	previousManifest, previousChunked, manifestErr := parseCredentialSecretManifest(previousRoot)
	if manifestErr != nil {
		return manifestErr
	}

	if len(payload) <= credentialSecretChunkSize {
		if err := s.secrets.Set(credentialKeyringService, ref, payload); err != nil {
			return err
		}
		if previousChunked {
			if err := s.deleteCredentialSecretChunks(ref, previousManifest); err != nil {
				return errors.Join(err, s.restoreCredentialSecretRoot(ref, previousRoot, hadPreviousRoot))
			}
		}
		return nil
	}

	chunkCount := (len(payload) + credentialSecretChunkSize - 1) / credentialSecretChunkSize
	if chunkCount > credentialSecretMaxChunks {
		return fmt.Errorf("credential secret exceeds the supported Keychain payload size")
	}
	manifest := credentialSecretManifest{Version: 1, Revision: uuid.NewString(), Chunks: chunkCount}
	digest := sha256.Sum256([]byte(payload))
	manifest.SHA256 = hex.EncodeToString(digest[:])
	written := 0
	for index := 0; index < chunkCount; index++ {
		start := index * credentialSecretChunkSize
		end := min(start+credentialSecretChunkSize, len(payload))
		if err := s.secrets.Set(credentialKeyringService, credentialSecretChunkRef(ref, manifest.Revision, index), payload[start:end]); err != nil {
			return errors.Join(err, s.deleteCredentialSecretChunks(ref, credentialSecretManifest{Revision: manifest.Revision, Chunks: written}))
		}
		written++
	}
	encodedManifest, err := json.Marshal(manifest)
	if err != nil {
		return errors.Join(err, s.deleteCredentialSecretChunks(ref, manifest))
	}
	if err := s.secrets.Set(credentialKeyringService, ref, credentialSecretManifestPrefix+string(encodedManifest)); err != nil {
		return errors.Join(err, s.deleteCredentialSecretChunks(ref, manifest))
	}
	if previousChunked {
		if err := s.deleteCredentialSecretChunks(ref, previousManifest); err != nil {
			return errors.Join(err, s.restoreCredentialSecretRoot(ref, previousRoot, hadPreviousRoot), s.deleteCredentialSecretChunks(ref, manifest))
		}
	}
	return nil
}

func (s *PersistentCredentialStore) deleteCredentialSecret(ref string) error {
	root, err := s.secrets.Get(credentialKeyringService, ref)
	if err != nil {
		if errors.Is(err, ErrCredentialNotFound) {
			return nil
		}
		return err
	}
	manifest, chunked, err := parseCredentialSecretManifest(root)
	if err != nil {
		return err
	}
	if err := s.secrets.Delete(credentialKeyringService, ref); err != nil {
		return err
	}
	if chunked {
		return s.deleteCredentialSecretChunks(ref, manifest)
	}
	return nil
}

func (s *PersistentCredentialStore) restoreCredentialSecretRoot(ref, root string, hadRoot bool) error {
	if hadRoot {
		return s.secrets.Set(credentialKeyringService, ref, root)
	}
	return s.secrets.Delete(credentialKeyringService, ref)
}

func (s *PersistentCredentialStore) deleteCredentialSecretChunks(ref string, manifest credentialSecretManifest) error {
	var cleanupErr error
	for index := 0; index < manifest.Chunks; index++ {
		if err := s.secrets.Delete(credentialKeyringService, credentialSecretChunkRef(ref, manifest.Revision, index)); err != nil && !errors.Is(err, ErrCredentialNotFound) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete credential secret chunk %d of %d: %w", index+1, manifest.Chunks, err))
		}
	}
	return cleanupErr
}

func credentialSecretChunkRef(ref, revision string, index int) string {
	return fmt.Sprintf("%s:chunk:%s:%03d", ref, revision, index)
}

func parseCredentialSecretManifest(value string) (credentialSecretManifest, bool, error) {
	if !strings.HasPrefix(value, credentialSecretManifestPrefix) {
		return credentialSecretManifest{}, false, nil
	}
	var manifest credentialSecretManifest
	if err := json.Unmarshal([]byte(strings.TrimPrefix(value, credentialSecretManifestPrefix)), &manifest); err != nil {
		return credentialSecretManifest{}, false, fmt.Errorf("decode credential secret manifest: %w", err)
	}
	if manifest.Version != 1 || manifest.Chunks < 1 || manifest.Chunks > credentialSecretMaxChunks || strings.TrimSpace(manifest.SHA256) == "" {
		return credentialSecretManifest{}, false, errors.New("invalid credential secret manifest")
	}
	if _, err := uuid.Parse(manifest.Revision); err != nil {
		return credentialSecretManifest{}, false, errors.New("invalid credential secret manifest revision")
	}
	return manifest, true, nil
}

func (s *PersistentCredentialStore) HasTokenSource(serverName string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.readMetadataLocked()
	if err != nil {
		return false, err
	}
	_, ok := file.Accounts[serverName]
	return ok, nil
}

func (s *PersistentCredentialStore) metadataLocked(serverName string) (credentialMetadata, error) {
	file, err := s.readMetadataLocked()
	if err != nil {
		return credentialMetadata{}, err
	}
	meta, ok := file.Accounts[serverName]
	if !ok {
		return credentialMetadata{}, ErrCredentialNotFound
	}
	return meta, nil
}

func (s *PersistentCredentialStore) readMetadataLocked() (credentialMetadataFile, error) {
	if s.initErr != nil {
		return credentialMetadataFile{}, fmt.Errorf("initialize credential metadata root: %w", s.initErr)
	}
	file := credentialMetadataFile{
		Version:  credentialMetadataVersion,
		Accounts: map[string]credentialMetadata{},
	}
	err := filestore.ReadJSON(s.metadataPath, &file)
	if os.IsNotExist(err) {
		return file, nil
	}
	if err != nil {
		return credentialMetadataFile{}, err
	}
	if file.Version != credentialMetadataVersion {
		return credentialMetadataFile{}, fmt.Errorf("unsupported connector account metadata version %d", file.Version)
	}
	if file.Accounts == nil {
		file.Accounts = map[string]credentialMetadata{}
	}
	return file, nil
}

func (s *PersistentCredentialStore) writeMetadataLocked(file credentialMetadataFile) error {
	if s.writeMetadataForTest != nil {
		return s.writeMetadataForTest(file)
	}
	file.Version = credentialMetadataVersion
	if file.Accounts == nil {
		file.Accounts = map[string]credentialMetadata{}
	}
	if err := ensurePrivateConfigDir(filepath.Dir(s.metadataPath)); err != nil {
		return fmt.Errorf("create credential metadata directory: %w", err)
	}
	encoded, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credential metadata: %w", err)
	}
	return filestore.WriteFileAtomic(s.metadataPath, append(encoded, '\n'))
}

func (s *PersistentCredentialStore) withMetadataMutationLock(action func() error) error {
	if s.initErr != nil {
		return fmt.Errorf("initialize credential metadata root: %w", s.initErr)
	}
	if err := ensurePrivateConfigDir(filepath.Dir(s.metadataPath)); err != nil {
		return fmt.Errorf("create credential metadata directory: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	lockPath := s.metadataPath + ".lock"
	owner := fmt.Sprintf("awaken-mcp-credentials-%d", os.Getpid())
	for {
		lock, err := filestore.AcquireLock(lockPath, owner, 10*time.Second)
		if err == nil {
			return errors.Join(action(), lock.Release())
		}
		if !errors.Is(err, filestore.ErrLockHeld) || time.Now().After(deadline) {
			return fmt.Errorf("lock credential metadata: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
