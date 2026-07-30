package modelprofile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUserStoreResolveExplicitAndDefaultWithoutExposingKeys(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 1,
  "profiles": [
    {
      "id": "profile-default",
      "providerId": "deepseek",
      "displayName": "Fast",
      "modelId": "deepseek-v4-flash",
      "baseURL": "https://api.deepseek.com/",
      "protocol": "openai_chat",
      "authScheme": "bearer",
      "enabled": true,
      "isDefault": true,
      "credentialRevision": 2,
      "advanced": {"maxTokens": 4096}
    },
    {
      "id": "profile-kimi",
      "providerId": "kimi-cn",
      "displayName": "Kimi",
      "modelId": "kimi-k2.6",
      "enabled": true,
      "isDefault": false
    }
  ]
}`, "AWAKEN_API_KEY_profile-default=\"secret-default\"\nAWAKEN_API_KEY_profile-kimi='secret-kimi'\n")

	store := NewUserStore(root)
	catalog, err := store.ListReady()
	if err != nil {
		t.Fatalf("list ready: %v", err)
	}
	if catalog.DefaultProfileID != "profile-default" || len(catalog.Profiles) != 2 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	defaultProfile, err := store.Resolve("")
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if defaultProfile.ID != "profile-default" || defaultProfile.APIKey != "secret-default" || defaultProfile.MaxTokens != 4096 {
		t.Fatalf("unexpected default profile: %+v", defaultProfile)
	}
	explicit, err := store.Resolve("profile-kimi")
	if err != nil {
		t.Fatalf("resolve explicit: %v", err)
	}
	if explicit.ID != "profile-kimi" || explicit.APIKey != "secret-kimi" {
		t.Fatalf("unexpected explicit profile: %+v", explicit)
	}
}

func TestUserStoreFallsBackToFirstReadyAndRejectsUnavailableSelection(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 1,
  "profiles": [
    {"id":"missing-key","providerId":"deepseek","modelId":"deepseek-v4-pro","enabled":true,"isDefault":true},
    {"id":"ready","providerId":"deepseek","modelId":"deepseek-v4-flash","enabled":true,"isDefault":false},
    {"id":"disabled","providerId":"kimi-cn","modelId":"kimi-k2.6","enabled":false,"isDefault":false}
  ]
}`, "AWAKEN_API_KEY_ready=usable\nAWAKEN_API_KEY_disabled=ignored\n")

	store := NewUserStore(root)
	profile, err := store.Resolve("")
	if err != nil {
		t.Fatalf("resolve fallback: %v", err)
	}
	if profile.ID != "ready" {
		t.Fatalf("expected first ready profile, got %q", profile.ID)
	}
	if _, err := store.Resolve("missing-key"); !errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("expected unavailable profile, got %v", err)
	}
}

func TestUserStoreFailsClosedOnMalformedEnv(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 1,
  "profiles": [
    {"id":"ready","providerId":"deepseek","modelId":"deepseek-v4-flash","enabled":true,"isDefault":true}
  ]
}`, "not-an-assignment\n")
	store := NewUserStore(root)
	if _, err := store.ListReady(); err == nil {
		t.Fatal("expected malformed env to fail closed")
	}
}

func TestUserStoreRejectsAgentPlanMediaModelAsMainProfile(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 2,
  "profiles": [
    {"id":"agent-llm","providerId":"volcengine-agent-plan","modelId":"glm-5.2","enabled":true,"isDefault":true,"credentialReference":{"kind":"provider_account","providerId":"volcengine-agent-plan","accountId":"volcengine-agent-plan-default"}},
    {"id":"agent-image","providerId":"volcengine-agent-plan","modelId":"doubao-seedream-test","enabled":true,"isDefault":false,"credentialReference":{"kind":"provider_account","providerId":"volcengine-agent-plan","accountId":"volcengine-agent-plan-default"}}
  ]
}`, "AWAKEN_AGENT_PLAN_API_KEY_volcengine-agent-plan-default=secret\n")
	account := `{"version":2,"accountId":"volcengine-agent-plan-default","credentialRevision":2,"catalog":[{"id":"glm-5.2","kind":"llm"},{"id":"doubao-seedream-test","kind":"image"}]}`
	if err := os.WriteFile(filepath.Join(root, "config", "agent-plan-account.json"), []byte(account), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewUserStore(root)
	catalog, err := store.ListReady()
	if err != nil {
		t.Fatalf("list ready: %v", err)
	}
	if len(catalog.Profiles) != 1 || catalog.Profiles[0].ID != "agent-llm" {
		t.Fatalf("media profile escaped Agent Plan catalog split: %+v", catalog)
	}
	if _, err := store.Resolve("agent-image"); !errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("expected image model profile to be unavailable, got %v", err)
	}
}

func writeUserStoreFixture(t *testing.T, root, models, env string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "models.json"), []byte(models), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
}
