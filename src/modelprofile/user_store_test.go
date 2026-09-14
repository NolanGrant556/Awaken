package modelprofile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestKimiProtocolProfileResolution(t *testing.T) {
	for _, providerID := range []string{"kimi-cn", "kimi-coding-plan"} {
		provider, ok := ProviderByID(providerID)
		if !ok {
			t.Fatal(providerID)
		}
		model := "kimi-k3"
		if providerID == "kimi-coding-plan" {
			model = "kimi-for-coding"
		}
		for _, protocol := range allLLMProtocols() {
			stored := storedProfile{ID: "kimi-test", ProviderID: providerID, ModelID: model, Protocol: protocol, Enabled: true}
			profile := normalizeStoredProfile(stored, provider, nil)
			if profile.ModelID != model || profile.Protocol != protocol || profile.ReasoningEffort != "max" || len(profile.ReasoningEffortLevels) != 3 {
				t.Fatalf("%s/%s did not retain native effort", providerID, protocol)
			}
			if protocol != "openai_chat" && profile.SupportsVideo {
				t.Fatal("video must not leak from Chat capability")
			}
			if providerID == "kimi-cn" && protocol == "anthropic_messages" && profile.BaseURL != "https://api.moonshot.cn/anthropic" {
				t.Fatal("wrong API Messages base")
			}
			lowered := 256 * 1024
			stored.Advanced.ContextWindow = &lowered
			if normalizeStoredProfile(stored, provider, nil).ContextWindow != lowered {
				t.Fatal("must preserve user's smaller context")
			}
		}
	}
	provider, _ := ProviderByID("kimi-cn")
	profile := normalizeStoredProfile(storedProfile{ID: "old-model", ModelID: "kimi-k2.6", Protocol: "openai_responses", Enabled: true}, provider, nil)
	if profile.ID != "" {
		t.Fatal("K2.x must not silently gain Responses support")
	}
}

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

func TestUserStoreKeepsSameModelAcrossProtocolsAndRoutesPurposes(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "purposeRoutes": {"compact_summary":"openai-chat"},
  "profiles": [
    {"id":"openai-responses","providerId":"openai","modelId":"gpt-5.6-sol","protocol":"openai_responses","enabled":true,"isDefault":true},
    {"id":"openai-chat","providerId":"openai","modelId":"gpt-5.6-sol","protocol":"openai_chat","enabled":true,"isDefault":false}
  ]
}`, "AWAKEN_API_KEY_openai-responses=responses-key\nAWAKEN_API_KEY_openai-chat=chat-key\n")

	store := NewUserStore(root)
	catalog, err := store.ListReady()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Profiles) != 2 {
		t.Fatalf("same model profiles with different protocols were collapsed: %+v", catalog.Profiles)
	}
	compact, err := store.ResolveForPurpose("compact_summary", "")
	if err != nil {
		t.Fatal(err)
	}
	if compact.ID != "openai-chat" || compact.Protocol != "openai_chat" {
		t.Fatalf("unexpected compact route: %+v", compact)
	}
	if _, err := store.ResolveForPurpose("scheduled_task", "missing-pin"); !errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("explicit scheduled pin must fail closed, got %v", err)
	}
}

func TestUserStoreKeepsNativePDFScopedToProtocolContract(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"openai-responses","providerId":"openai","modelId":"gpt-5.6-sol","protocol":"openai_responses","enabled":true,"isDefault":true},
    {"id":"google-chat","providerId":"google","modelId":"gemini-3.6-flash","protocol":"openai_chat","enabled":true,"isDefault":false}
  ]
}`, "AWAKEN_API_KEY_openai-responses=responses-key\nAWAKEN_API_KEY_google-chat=google-key\n")

	store := NewUserStore(root)
	responses, err := store.Resolve("openai-responses")
	if err != nil {
		t.Fatal(err)
	}
	if !responses.SupportsNativePDF || !responses.SupportsPromptCache {
		t.Fatalf("OpenAI Responses protocol capability missing: %+v", responses)
	}
	google, err := store.Resolve("google-chat")
	if err != nil {
		t.Fatal(err)
	}
	if !google.SupportsVision || google.SupportsNativePDF || google.SupportsVideo || google.SupportsPromptCache {
		t.Fatalf("Google Chat compatibility inherited unsupported native features: %+v", google)
	}
}

func TestUserStoreRejectsUnknownProtocol(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"unsafe","providerId":"custom","modelId":"model","baseURL":"https://gateway.test/v1","protocol":"guessed_protocol","enabled":true,"isDefault":true}
  ]
}`, "AWAKEN_API_KEY_unsafe=key\n")

	if _, err := NewUserStore(root).Resolve(""); !errors.Is(err, ErrNoUsableProfile) {
		t.Fatalf("unknown protocol must fail closed, got %v", err)
	}
}

func TestUserStoreMigratesRetiredBuiltInProtocolToProviderContract(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"anthropic-legacy-chat","providerId":"anthropic","modelId":"claude-sonnet-5","baseURL":"https://api.anthropic.com/v1","protocol":"openai_chat","authScheme":"bearer","enabled":true,"isDefault":true}
  ]
}`, "AWAKEN_API_KEY_anthropic-legacy-chat=key\n")

	profile, err := NewUserStore(root).Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Protocol != "anthropic_messages" || profile.BaseURL != "https://api.anthropic.com" || profile.AuthScheme != "x-api-key" {
		t.Fatalf("retired protocol was not migrated to the provider contract: %+v", profile)
	}
}

func TestUserStoreIgnoresLegacyCapabilitySwitchesButKeepsTokenLimits(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"google","providerId":"google","modelId":"gemini-3.6-flash","protocol":"openai_chat","enabled":true,"isDefault":true,"advanced":{"contextWindow":64000,"maxTokens":12000,"supportsVision":false,"supportsNativePDF":true,"supportsVideo":true,"supportsTools":"unsupported","supportsPromptCache":true}}
  ]
}`, "AWAKEN_API_KEY_google=key\n")

	profile, err := NewUserStore(root).Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if profile.ContextWindow != 64000 || profile.MaxTokens != 12000 {
		t.Fatalf("user token limits were not preserved: %+v", profile)
	}
	if !profile.SupportsVision || profile.SupportsNativePDF || profile.SupportsVideo || profile.SupportsTools != "ready" || profile.SupportsPromptCache {
		t.Fatalf("legacy capability switches overrode provider facts: %+v", profile)
	}
}

func TestUserStoreAcceptsOnlyExplicitCustomVisionCapability(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		root := t.TempDir()
		writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"custom-vision","providerId":"custom","modelId":"custom-model","baseURL":"https://gateway.example/v1","protocol":"openai_chat","authScheme":"bearer","enabled":true,"isDefault":true,"advanced":{"supportsVision":true,"supportsNativePDF":true}}
  ]
}`, "AWAKEN_API_KEY_custom-vision=key\n")

		profile, err := NewUserStore(root).Resolve("")
		if err != nil {
			t.Fatal(err)
		}
		if !profile.SupportsVision || profile.SupportsNativePDF {
			t.Fatalf("custom vision contract was not preserved safely: %+v", profile)
		}
	})

	t.Run("omitted defaults off", func(t *testing.T) {
		root := t.TempDir()
		writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"custom-default","providerId":"custom","modelId":"custom-model","baseURL":"https://gateway.example/v1","protocol":"openai_chat","authScheme":"bearer","enabled":true,"isDefault":true}
  ]
}`, "AWAKEN_API_KEY_custom-default=key\n")

		profile, err := NewUserStore(root).Resolve("")
		if err != nil {
			t.Fatal(err)
		}
		if profile.SupportsVision || profile.SupportsNativePDF {
			t.Fatalf("custom media capabilities did not fail closed: %+v", profile)
		}
	})
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

func TestMergeCachedCapabilityPreservesProviderNativeEffortLabels(t *testing.T) {
	base := ModelCapability{
		ReasoningEffort: effortWithLabels(
			[]string{"low", "high", "max"},
			"max",
			map[string]string{"low": "low", "high": "medium", "max": "xhigh"},
		),
	}
	cached := &cachedModelCapability{
		ReasoningEffort: effort([]string{"low", "high", "max"}, "max"),
	}

	merged := mergeCachedCapability(base, cached)
	if merged.ReasoningEffort == nil || merged.ReasoningEffort.DisplayLabels["high"] != "medium" || merged.ReasoningEffort.DisplayLabels["max"] != "xhigh" {
		t.Fatalf("provider-native labels were dropped by an older cache: %+v", merged.ReasoningEffort)
	}
}

func TestUserStoreResolvesDoubaoModelAndEndpointSelectorsSeparately(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"doubao-model","providerId":"doubao","modelId":"doubao-seed-2-1-pro-260628","protocol":"openai_chat","enabled":true,"isDefault":true},
    {"id":"doubao-endpoint","providerId":"doubao","modelId":"ep-custom-endpoint","protocol":"openai_responses","enabled":true,"isDefault":false,"advanced":{"endpointId":"ep-custom-endpoint"}}
  ]
}`, "AWAKEN_API_KEY_doubao-model=model-key\nAWAKEN_API_KEY_doubao-endpoint=endpoint-key\n")

	store := NewUserStore(root)
	direct, err := store.Resolve("doubao-model")
	if err != nil {
		t.Fatal(err)
	}
	if direct.ModelID != "doubao-seed-2-1-pro-260628" || direct.CapabilityModelID != direct.ModelID || direct.ModelSelectorKind != "model" || direct.EndpointSelector != "" {
		t.Fatalf("unexpected direct Doubao profile: %+v", direct)
	}

	endpoint, err := store.Resolve("doubao-endpoint")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.ModelID != "ep-custom-endpoint" || endpoint.CapabilityModelID != "ep-custom-endpoint" || endpoint.ModelSelectorKind != "endpoint" || endpoint.EndpointSelector != "ep-custom-endpoint" {
		t.Fatalf("unexpected endpoint Doubao profile: %+v", endpoint)
	}
}

func TestUserStoreMigratesLegacyDoubaoDisplayAlias(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"doubao-legacy","providerId":"doubao","modelId":"doubao-seed-2.1-pro","protocol":"openai_chat","enabled":true,"isDefault":true}
  ]
}`, "AWAKEN_API_KEY_doubao-legacy=legacy-key\n")

	legacy, err := NewUserStore(root).Resolve("doubao-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.ModelID != "doubao-seed-2-1-pro-260628" || legacy.CapabilityModelID != legacy.ModelID || legacy.ModelSelectorKind != "model" {
		t.Fatalf("legacy Doubao profile was not normalized: %+v", legacy)
	}
}

func TestUserStoreMigratesArkHostedPricingAlias(t *testing.T) {
	root := t.TempDir()
	writeUserStoreFixture(t, root, `{
  "version": 3,
  "profiles": [
    {"id":"doubao-hosted","providerId":"doubao","modelId":"deepseek-v4-pro","protocol":"openai_chat","enabled":true,"isDefault":true}
  ]
}`, "AWAKEN_API_KEY_doubao-hosted=hosted-key\n")

	hosted, err := NewUserStore(root).Resolve("doubao-hosted")
	if err != nil {
		t.Fatal(err)
	}
	if hosted.ModelID != "deepseek-v4-pro-ga-260813" || hosted.CapabilityModelID != hosted.ModelID {
		t.Fatalf("hosted Doubao alias was not normalized: %+v", hosted)
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
