package mcpclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"awaken/src/filestore"

	"golang.org/x/oauth2"
)

func TestCredentialIdentitySeparatesScopeAndEndpoint(t *testing.T) {
	userConfig := McpServerConfig{HTTP: &HTTPTransportConfig{URL: "HTTPS://Example.COM/mcp#fragment"}}
	projectConfig := McpServerConfig{HTTP: &HTTPTransportConfig{URL: "https://example.com/mcp"}}
	otherEndpoint := McpServerConfig{HTTP: &HTTPTransportConfig{URL: "https://example.com/other"}}

	userIdentity := CredentialIdentity(ScopeUser, "shared", userConfig)
	if userIdentity != CredentialIdentity(ScopeUser, "shared", projectConfig) {
		t.Fatal("canonical endpoint variants should share one credential identity")
	}
	if userIdentity == CredentialIdentity(ScopeProject, "shared", projectConfig) {
		t.Fatal("project and user configs must not share credentials")
	}
	if userIdentity == CredentialIdentity(ScopeUser, "shared", otherEndpoint) {
		t.Fatal("different endpoints must not share credentials")
	}
}

func TestCredentialIdentityChangesWithOAuthRegistrationFingerprint(t *testing.T) {
	base := McpServerConfig{HTTP: &HTTPTransportConfig{URL: "https://mcp.example/mcp"}, OAuthResource: "https://mcp.example/mcp", OAuthClientID: "client-a", OAuthScopes: []string{"read"}}
	changedClient := base
	changedClient.OAuthClientID = "client-b"
	changedScopes := base
	changedScopes.OAuthScopes = []string{"read", "write"}
	if CredentialIdentity(ScopePlugin, "server", base) == CredentialIdentity(ScopePlugin, "server", changedClient) {
		t.Fatal("OAuth client change reused credential identity")
	}
	if CredentialIdentity(ScopePlugin, "server", base) == CredentialIdentity(ScopePlugin, "server", changedScopes) {
		t.Fatal("OAuth scope change reused credential identity")
	}
}

func TestPersistentOAuthHandlerClearsMismatchedEndpointCredential(t *testing.T) {
	store := NewInMemoryCredentialStore()
	identity := CredentialIdentity(ScopeUser, "example", McpServerConfig{HTTP: &HTTPTransportConfig{URL: "https://new.example/mcp"}})
	if err := store.SaveTokenSource(identity, StoredTokenSource{
		ServerURL: "https://old.example/mcp",
		Token:     &oauth2.Token{AccessToken: "must-not-be-reused"},
	}); err != nil {
		t.Fatalf("SaveTokenSource: %v", err)
	}
	handler := newPersistentOAuthHandler(identity, AuthFlowConfig{ServerURL: "https://new.example/mcp"}, store)
	tokenSource, err := handler.TokenSource(context.Background())
	if err != nil {
		t.Fatalf("TokenSource: %v", err)
	}
	if tokenSource != nil {
		t.Fatal("mismatched endpoint credential must not produce a token source")
	}
	if exists, err := store.HasTokenSource(identity); err != nil || exists {
		t.Fatalf("mismatched endpoint credential was not cleared: exists=%v err=%v", exists, err)
	}
}

func TestUserConfigPathUsesAwakenHome(t *testing.T) {
	awakenHome, _, _ := setUserConfigTestEnv(t)

	want := filepath.Join(awakenHome, "config", "mcp-servers.json")
	if got := UserConfigPath(); got != want {
		t.Fatalf("UserConfigPath() = %q; want %q", got, want)
	}
}

func TestUserConfigIgnoresElectronUserDataWithoutAwakenHome(t *testing.T) {
	fallbackTemp := t.TempDir()
	userData := t.TempDir()
	t.Setenv("TMPDIR", fallbackTemp)
	t.Setenv("AWAKEN_HOME", "")
	t.Setenv("AWAKEN_USER_DATA_DIR", userData)

	legacyPath := filepath.Join(userData, "awaken-home", "config", "mcp-servers.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("create legacy userData config directory: %v", err)
	}
	legacy := []byte("{\"servers\":{\"must-not-load\":{\"stdio\":{\"command\":\"legacy\"}}}}\n")
	if err := os.WriteFile(legacyPath, legacy, 0o600); err != nil {
		t.Fatalf("write legacy userData config: %v", err)
	}

	configPath := UserConfigPath()
	rel, err := filepath.Rel(userData, configPath)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		t.Fatalf("UserConfigPath() = %q; must not fall back beneath Electron userData %q", configPath, userData)
	}
	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig returned error: %v", err)
	}
	if len(loaded.Servers) != 0 {
		t.Fatalf("LoadUserConfig read Electron userData fallback: %+v", loaded.Servers)
	}
	gotLegacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy userData config after load: %v", err)
	}
	if string(gotLegacy) != string(legacy) {
		t.Fatalf("legacy userData config was modified: got %q, want %q", gotLegacy, legacy)
	}
}

func TestUserConfigSaveLoadRoundTripAtAwakenHome(t *testing.T) {
	awakenHome, _, _ := setUserConfigTestEnv(t)
	cfg := ConfigFile{Servers: map[string]McpServerConfig{
		"demo": {
			DisplayName: "Demo server",
			Stdio: &StdioTransportConfig{
				Command: "demo-command",
				Args:    []string{"--stdio"},
			},
		},
	}}

	if err := SaveUserConfig(cfg); err != nil {
		t.Fatalf("SaveUserConfig returned error: %v", err)
	}

	configPath := filepath.Join(awakenHome, "config", "mcp-servers.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat saved user config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("saved user config mode = %v; want 0600", info.Mode().Perm())
	}

	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig returned error: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("LoadUserConfig() = %+v; want %+v", loaded, cfg)
	}
}

func TestSaveUserConfigAtomicallyTightensExistingPermissions(t *testing.T) {
	awakenHome, _, _ := setUserConfigTestEnv(t)
	configPath := filepath.Join(awakenHome, "config", "mcp-servers.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o777); err != nil {
		t.Fatalf("create permissive config directory: %v", err)
	}
	if err := os.Chmod(filepath.Dir(configPath), 0o777); err != nil {
		t.Fatalf("chmod permissive config directory: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{\"servers\":{}}\n"), 0o666); err != nil {
		t.Fatalf("seed permissive config: %v", err)
	}
	if err := os.Chmod(configPath, 0o666); err != nil {
		t.Fatalf("chmod permissive config: %v", err)
	}

	cfg := ConfigFile{Servers: map[string]McpServerConfig{
		"atomic": {Stdio: &StdioTransportConfig{Command: "atomic-server"}},
	}}
	if err := SaveUserConfig(cfg); err != nil {
		t.Fatalf("SaveUserConfig returned error: %v", err)
	}

	fileInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat saved user config: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("saved user config mode = %v; want 0600", fileInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(configPath))
	if err != nil {
		t.Fatalf("stat user config directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("user config directory mode = %v; want 0700", dirInfo.Mode().Perm())
	}
	loaded, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig returned error: %v", err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("LoadUserConfig() = %+v; want %+v", loaded, cfg)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(configPath), ".mcp-servers.json.tmp-*")); err != nil {
		t.Fatalf("glob temp files: %v", err)
	} else if len(matches) != 0 {
		t.Fatalf("atomic save left temp files behind: %v", matches)
	}
}

func TestLoadUserConfigIgnoresLegacyLocations(t *testing.T) {
	awakenHome, fakeHome, userData := setUserConfigTestEnv(t)
	legacyConfigs := map[string][]byte{
		filepath.Join(fakeHome, ".awaken", "mcp.json"):        []byte("{\"servers\":{\"home-legacy\":{\"stdio\":{\"command\":\"home-legacy\"}}}}\n"),
		filepath.Join(userData, "config", "mcp-servers.json"): []byte("{\"servers\":{\"user-data-legacy\":{\"stdio\":{\"command\":\"user-data-legacy\"}}}}\n"),
	}
	for path, contents := range legacyConfigs {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create legacy config directory: %v", err)
		}
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatalf("write legacy config %s: %v", path, err)
		}
	}

	cfg, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig returned error: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("LoadUserConfig read a legacy config: %+v", cfg.Servers)
	}

	canonicalPath := filepath.Join(awakenHome, "config", "mcp-servers.json")
	if _, err := os.Stat(canonicalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy config was migrated to %s: %v", canonicalPath, err)
	}
	for path, want := range legacyConfigs {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read legacy config %s after LoadUserConfig: %v", path, err)
		}
		if string(got) != string(want) {
			t.Fatalf("legacy config %s was modified: got %q, want %q", path, got, want)
		}
	}
}

func setUserConfigTestEnv(t *testing.T) (awakenHome, fakeHome, userData string) {
	t.Helper()
	fakeHome = t.TempDir()
	awakenHome = filepath.Join(fakeHome, ".awaken")
	userData = t.TempDir()
	t.Setenv("AWAKEN_HOME", awakenHome)
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv("AWAKEN_USER_DATA_DIR", userData)
	return awakenHome, fakeHome, userData
}

func TestPersistentCredentialStoreRoundTripsTokenSource(t *testing.T) {
	root := t.TempDir()
	metadataPath := filepath.Join(root, "auth", "connector-accounts.json")
	secrets := &memorySecretBackend{values: map[string]string{}}
	store := &PersistentCredentialStore{
		metadataPath: metadataPath,
		secrets:      secrets,
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
	rawMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read connector metadata: %v", err)
	}
	for _, secret := range []string{"access-token", "refresh-token", "client-secret"} {
		if strings.Contains(string(rawMetadata), secret) {
			t.Fatalf("connector metadata leaked secret %q: %s", secret, rawMetadata)
		}
	}
	if !strings.Contains(string(rawMetadata), `"credentialRef"`) ||
		!strings.Contains(string(rawMetadata), `"serverUrl": "https://example.com/mcp"`) {
		t.Fatalf("connector metadata missing non-secret recovery facts: %s", rawMetadata)
	}
	info, err := os.Stat(metadataPath)
	if err != nil {
		t.Fatalf("stat connector metadata: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("connector metadata mode = %v; want 0600", info.Mode().Perm())
	}

	restarted := &PersistentCredentialStore{
		metadataPath: metadataPath,
		secrets:      secrets,
	}
	loaded, err := restarted.LoadTokenSource("example")
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

	if err := restarted.ClearTokenSource("example"); err != nil {
		t.Fatalf("ClearTokenSource returned error: %v", err)
	}
	if ok, err := restarted.HasTokenSource("example"); err != nil || ok {
		t.Fatalf("HasTokenSource after clear = %v, %v; want false, nil", ok, err)
	}
}

func TestPersistentCredentialStoreShardsLargeKeychainPayloadAndRollsBack(t *testing.T) {
	root := t.TempDir()
	metadataPath := filepath.Join(root, "auth", "connector-accounts.json")
	secrets := &limitedSecretBackend{
		memorySecretBackend: memorySecretBackend{values: map[string]string{}},
		maxBytes:            2200,
	}
	store := &PersistentCredentialStore{metadataPath: metadataPath, secrets: secrets}
	first := StoredTokenSource{Token: &oauth2.Token{
		AccessToken:  strings.Repeat("enterprise-access-token-", 220),
		RefreshToken: strings.Repeat("enterprise-refresh-token-", 180),
		TokenType:    "Bearer",
	}}
	if err := store.SaveTokenSource("enterprise-outlook", first); err != nil {
		t.Fatalf("save large enterprise credential: %v", err)
	}
	loaded, err := store.LoadTokenSource("enterprise-outlook")
	if err != nil {
		t.Fatalf("load sharded enterprise credential: %v", err)
	}
	if loaded.Token == nil || loaded.Token.AccessToken != first.Token.AccessToken || loaded.Token.RefreshToken != first.Token.RefreshToken {
		t.Fatal("sharded enterprise credential did not round-trip")
	}
	manifestKey := secrets.key(credentialKeyringService, loaded.CredentialRef)
	if !strings.HasPrefix(secrets.values[manifestKey], credentialSecretManifestPrefix) {
		t.Fatalf("large credential root is not a chunk manifest: %q", secrets.values[manifestKey])
	}
	if len(secrets.values) < 3 {
		t.Fatalf("large credential was not split across Keychain entries: %d", len(secrets.values))
	}
	for key, value := range secrets.values {
		if len(value) > secrets.maxBytes {
			t.Fatalf("Keychain entry %q exceeds backend limit: %d", key, len(value))
		}
	}

	store.writeMetadataForTest = func(credentialMetadataFile) error {
		return errors.New("injected metadata write failure")
	}
	replacement := StoredTokenSource{Token: &oauth2.Token{
		AccessToken:  strings.Repeat("replacement-access-token-", 240),
		RefreshToken: strings.Repeat("replacement-refresh-token-", 190),
	}}
	if err := store.SaveTokenSource("enterprise-outlook", replacement); err == nil {
		t.Fatal("expected metadata failure for replacement credential")
	}
	store.writeMetadataForTest = nil
	rolledBack, err := store.LoadTokenSource("enterprise-outlook")
	if err != nil {
		t.Fatalf("load rolled-back enterprise credential: %v", err)
	}
	if rolledBack.Token == nil || rolledBack.Token.AccessToken != first.Token.AccessToken || rolledBack.Token.RefreshToken != first.Token.RefreshToken {
		t.Fatal("metadata failure did not restore the previous sharded credential")
	}

	if err := store.ClearTokenSource("enterprise-outlook"); err != nil {
		t.Fatalf("clear sharded enterprise credential: %v", err)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("sharded credential left Keychain entries behind: %d", len(secrets.values))
	}
}

func TestPersistentCredentialStoreSerializesMetadataAcrossInstances(t *testing.T) {
	root := t.TempDir()
	metadataPath := filepath.Join(root, "auth", "connector-accounts.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		t.Fatal(err)
	}
	held, err := filestore.AcquireLock(metadataPath+".lock", "test-holder", time.Minute)
	if err != nil {
		t.Fatalf("acquire credential metadata lock: %v", err)
	}
	secrets := &memorySecretBackend{values: map[string]string{}}
	store := &PersistentCredentialStore{metadataPath: metadataPath, secrets: secrets}
	done := make(chan error, 1)
	go func() {
		done <- store.SaveTokenSource("blocked", StoredTokenSource{
			Token: &oauth2.Token{AccessToken: "serialized-token"},
		})
	}()

	select {
	case err := <-done:
		_ = held.Release()
		t.Fatalf("SaveTokenSource bypassed the cross-process metadata lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := held.Release(); err != nil {
		t.Fatalf("release credential metadata lock: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SaveTokenSource after lock release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SaveTokenSource did not resume after metadata lock release")
	}
	if ok, err := store.HasTokenSource("blocked"); err != nil || !ok {
		t.Fatalf("HasTokenSource after serialized save = %v, %v; want true, nil", ok, err)
	}
}

func TestPersistentCredentialStoreNeverOverwritesCorruptMetadataAsEmpty(t *testing.T) {
	root := t.TempDir()
	metadataPath := filepath.Join(root, "auth", "connector-accounts.json")
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte(`{"version":1,"accounts":`)
	if err := os.WriteFile(metadataPath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	secrets := &memorySecretBackend{values: map[string]string{}}
	store := &PersistentCredentialStore{metadataPath: metadataPath, secrets: secrets}

	err := store.SaveTokenSource("example", StoredTokenSource{
		Token: &oauth2.Token{AccessToken: "must-not-be-written"},
	})
	if err == nil || !strings.Contains(err.Error(), "corrupt JSON") {
		t.Fatalf("SaveTokenSource corrupt metadata error = %v", err)
	}
	got, readErr := os.ReadFile(metadataPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt metadata was overwritten: %q", got)
	}
	if len(secrets.values) != 0 {
		t.Fatalf("secret backend was mutated before metadata validation: %+v", secrets.values)
	}
}

func TestPersistentCredentialStoreRollsBackSecretWhenMetadataWriteFails(t *testing.T) {
	root := t.TempDir()
	metadataPath := filepath.Join(root, "auth", "connector-accounts.json")
	secrets := &memorySecretBackend{values: map[string]string{}}
	store := &PersistentCredentialStore{metadataPath: metadataPath, secrets: secrets}
	first := StoredTokenSource{Token: &oauth2.Token{AccessToken: "first-token"}}
	if err := store.SaveTokenSource("example", first); err != nil {
		t.Fatal(err)
	}
	beforeMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadTokenSource("example")
	if err != nil {
		t.Fatal(err)
	}
	ref := loaded.CredentialRef

	store.writeMetadataForTest = func(credentialMetadataFile) error {
		return errors.New("injected metadata write failure")
	}
	if err := store.SaveTokenSource("example", StoredTokenSource{
		Token: &oauth2.Token{AccessToken: "replacement-token"},
	}); err == nil {
		t.Fatal("expected metadata write failure")
	}
	storedPayload := secrets.values[secrets.key(credentialKeyringService, ref)]
	if !strings.Contains(storedPayload, "first-token") || strings.Contains(storedPayload, "replacement-token") {
		t.Fatalf("previous secret was not restored: %s", storedPayload)
	}
	afterMetadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterMetadata) != string(beforeMetadata) {
		t.Fatal("metadata changed despite failed atomic write")
	}

	if err := store.ClearTokenSource("example"); err == nil {
		t.Fatal("expected metadata delete failure")
	}
	if _, ok := secrets.values[secrets.key(credentialKeyringService, ref)]; !ok {
		t.Fatal("secret was not restored after metadata delete failure")
	}
}

func TestPersistentCredentialStoreRemovesNewSecretWhenMetadataWriteFails(t *testing.T) {
	secrets := &memorySecretBackend{values: map[string]string{}}
	store := &PersistentCredentialStore{
		metadataPath: filepath.Join(t.TempDir(), "auth", "connector-accounts.json"),
		secrets:      secrets,
		writeMetadataForTest: func(credentialMetadataFile) error {
			return errors.New("injected metadata write failure")
		},
	}
	if err := store.SaveTokenSource("example", StoredTokenSource{
		Token: &oauth2.Token{AccessToken: "orphan-token"},
	}); err == nil {
		t.Fatal("expected metadata write failure")
	}
	if len(secrets.values) != 0 {
		t.Fatalf("new unreferenced secret was not removed: %+v", secrets.values)
	}
}

type memorySecretBackend struct {
	values map[string]string
}

type limitedSecretBackend struct {
	memorySecretBackend
	maxBytes int
}

func (m *limitedSecretBackend) Set(service, user, secret string) error {
	if len(secret) > m.maxBytes {
		return errors.New("secret backend payload too large")
	}
	return m.memorySecretBackend.Set(service, user, secret)
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
