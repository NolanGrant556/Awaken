package mcpclient

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"awaken/src/desktopdata"

	"golang.org/x/oauth2"
)

func TestLoadUserConfigMigratesLegacyConfigToDesktopPath(t *testing.T) {
	home := t.TempDir()
	userData := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AWAKEN_USER_DATA_DIR", userData)

	legacyDir := filepath.Join(home, ".awaken")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("MkdirAll legacy dir: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, "mcp.json")
	if err := os.WriteFile(legacyPath, []byte("{\"servers\":{\"demo\":{\"stdio\":{\"command\":\"demo\"}}}}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile legacy config: %v", err)
	}

	cfg, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig returned error: %v", err)
	}
	if _, ok := cfg.Servers["demo"]; !ok {
		t.Fatalf("expected migrated server config, got %+v", cfg.Servers)
	}

	migratedPath := UserConfigPath()
	if _, err := os.Stat(migratedPath); err != nil {
		t.Fatalf("expected migrated config at %s: %v", migratedPath, err)
	}
}

func TestPersistentCredentialStoreRoundTripsTokenSource(t *testing.T) {
	store := &PersistentCredentialStore{
		paths:   desktopdata.FromRoot(t.TempDir()),
		secrets: &memorySecretBackend{values: map[string]string{}},
	}
	source := StoredTokenSource{
		Token: &oauth2.Token{
			AccessToken:  "access-token",
			TokenType:    "Bearer",
			RefreshToken: "refresh-token",
			Expiry:       time.Now().UTC().Add(30 * time.Minute).Round(0),
		},
		ServerURL:    "https://example.com/mcp",
		AuthURL:      "https://auth.example.com/authorize",
		TokenURL:     "https://auth.example.com/token",
		RedirectURL:  "http://127.0.0.1:3000/callback",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Scopes:       []string{"read", "write"},
	}

	if err := store.SaveTokenSource("example", source); err != nil {
		t.Fatalf("SaveTokenSource returned error: %v", err)
	}
	if ok, err := store.HasTokenSource("example"); err != nil || !ok {
		t.Fatalf("HasTokenSource = %v, %v; want true, nil", ok, err)
	}

	loaded, err := store.LoadTokenSource("example")
	if err != nil {
		t.Fatalf("LoadTokenSource returned error: %v", err)
	}
	if loaded.CredentialRef == "" {
		t.Fatalf("expected credential ref to be assigned")
	}
	if loaded.Token == nil || loaded.Token.AccessToken != source.Token.AccessToken || loaded.Token.RefreshToken != source.Token.RefreshToken {
		t.Fatalf("unexpected loaded token: %+v", loaded.Token)
	}
	if loaded.ClientSecret != source.ClientSecret {
		t.Fatalf("expected client secret to round-trip through secure store")
	}
	if !reflect.DeepEqual(loaded.Scopes, source.Scopes) {
		t.Fatalf("expected scopes %v, got %v", source.Scopes, loaded.Scopes)
	}

	if err := store.ClearTokenSource("example"); err != nil {
		t.Fatalf("ClearTokenSource returned error: %v", err)
	}
	if ok, err := store.HasTokenSource("example"); err != nil || ok {
		t.Fatalf("HasTokenSource after clear = %v, %v; want false, nil", ok, err)
	}
}

type memorySecretBackend struct {
	values map[string]string
}

func (m *memorySecretBackend) key(service, user string) string {
	return service + "::" + user
}

func (m *memorySecretBackend) Get(service, user string) (string, error) {
	value, ok := m.values[m.key(service, user)]
	if !ok {
		return "", ErrCredentialNotFound
	}
	return value, nil
}

func (m *memorySecretBackend) Set(service, user, secret string) error {
	m.values[m.key(service, user)] = secret
	return nil
}

func (m *memorySecretBackend) Delete(service, user string) error {
	delete(m.values, m.key(service, user))
	return nil
}
