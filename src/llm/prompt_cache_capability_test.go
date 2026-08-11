package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestResolvePromptCacheCapabilityRequiresExactProviderEndpointAndProtocol(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "deepseek official", cfg: Config{ProviderID: "DeepSeek", BaseURL: "https://api.deepseek.com/", Protocol: ProtocolOpenAIChat}, want: "deepseek_openai_chat_v1"},
		{name: "kimi cn official", cfg: Config{ProviderID: "kimi-cn", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat}, want: "kimi_cn_openai_chat_v1"},
		{name: "kimi coding official", cfg: Config{ProviderID: "kimi-coding-plan", BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat}, want: "kimi_coding_plan_openai_chat_v1"},
		{name: "volc official", cfg: Config{ProviderID: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", Protocol: ProtocolOpenAIChat}, want: "volcengine_coding_plan_openai_chat_v1"},
		{name: "custom endpoint denied", cfg: Config{ProviderID: "kimi-cn", BaseURL: "https://proxy.example/v1", Protocol: ProtocolOpenAIChat}},
		{name: "custom endpoint admitted by profile contract", cfg: Config{ProviderID: "custom", BaseURL: "https://proxy.example/v1", Protocol: ProtocolOpenAIResponses, SupportsPromptCache: true}, want: "profile_capability_openai_responses"},
		{name: "provider mismatch denied", cfg: Config{ProviderID: "deepseek", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat}},
		{name: "protocol mismatch denied", cfg: Config{ProviderID: "deepseek", BaseURL: "https://api.deepseek.com", Protocol: ProtocolAnthropicMessages}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability, ok := ResolvePromptCacheCapability(tt.cfg)
			if tt.want == "" {
				if ok {
					t.Fatalf("unexpected capability: %+v", capability)
				}
				return
			}
			if !ok || capability.ID != tt.want {
				t.Fatalf("want %q, got ok=%v capability=%+v", tt.want, ok, capability)
			}
		})
	}
}

func TestPromptCacheAffinityKeyIsStableAndPurposeBound(t *testing.T) {
	cfg := Config{
		ProviderID: "kimi-coding-plan", ProfileID: "profile-1", CredentialRevision: 2,
		BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-for-coding",
	}
	capability, ok := ResolvePromptCacheCapability(cfg)
	if !ok {
		t.Fatal("expected Kimi Coding Plan capability")
	}
	baseRequest := Request{AffinityScopeID: "thread-1", Purpose: RequestPurposeAgentTurn, ReasoningPolicy: "provider_default"}
	first := promptCacheAffinityKey(cfg, baseRequest, capability)
	second := promptCacheAffinityKey(cfg, baseRequest, capability)
	if first != second || !strings.HasPrefix(first, "aw1_") {
		t.Fatalf("expected stable aw1_ key, first=%q second=%q", first, second)
	}

	mutations := []struct {
		name string
		cfg  Config
		req  Request
	}{
		{name: "thread", cfg: cfg, req: Request{AffinityScopeID: "thread-2", Purpose: RequestPurposeAgentTurn, ReasoningPolicy: "provider_default"}},
		{name: "revision", cfg: func() Config { next := cfg; next.CredentialRevision = 3; return next }(), req: baseRequest},
		{name: "model", cfg: func() Config { next := cfg; next.Model = "k3"; return next }(), req: baseRequest},
		{name: "purpose", cfg: cfg, req: Request{AffinityScopeID: "thread-1", Purpose: RequestPurposeCompactSummary, ReasoningPolicy: "provider_default"}},
		{name: "subagent purpose", cfg: cfg, req: Request{AffinityScopeID: "thread-1", Purpose: RequestPurposeSubagent, ReasoningPolicy: "provider_default"}},
		{name: "scheduled purpose", cfg: cfg, req: Request{AffinityScopeID: "thread-1", Purpose: RequestPurposeScheduledTask, ReasoningPolicy: "provider_default"}},
		{name: "reasoning", cfg: cfg, req: Request{AffinityScopeID: "thread-1", Purpose: RequestPurposeAgentTurn, ReasoningPolicy: "max"}},
	}
	for _, mutation := range mutations {
		if got := promptCacheAffinityKey(mutation.cfg, mutation.req, capability); got == first {
			t.Fatalf("%s mutation reused affinity key", mutation.name)
		}
	}
}

func TestPrepareOpenAIPromptCacheRequestContracts(t *testing.T) {
	tests := []struct {
		name              string
		cfg               Config
		stream            bool
		disableReasoning  bool
		wantCapability    string
		wantKey           bool
		wantIncludeUsage  bool
		wantThinkingField bool
	}{
		{
			name: "deepseek automatic prefix", stream: true,
			cfg:            Config{APIKey: "test", ProviderID: "deepseek", ProfileID: "p1", BaseURL: "https://api.deepseek.com", Protocol: ProtocolOpenAIChat, Model: "deepseek-v4-pro"},
			wantCapability: "deepseek_openai_chat_v1",
		},
		{
			name: "kimi cn affinity streaming", stream: true,
			cfg:            Config{APIKey: "test", ProviderID: "kimi-cn", ProfileID: "p1", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-k2.6"},
			wantCapability: "kimi_cn_openai_chat_v1", wantKey: true, wantIncludeUsage: true,
		},
		{
			name: "kimi coding affinity nonstream", stream: false,
			cfg:            Config{APIKey: "test", ProviderID: "kimi-coding-plan", ProfileID: "p1", BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-for-coding"},
			wantCapability: "kimi_coding_plan_openai_chat_v1", wantKey: true,
		},
		{
			name: "volc response probe only", stream: true,
			cfg:            Config{APIKey: "test", ProviderID: "volcengine", ProfileID: "p1", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", Protocol: ProtocolOpenAIChat, Model: "ark-code-latest"},
			wantCapability: "volcengine_coding_plan_openai_chat_v1",
		},
		{
			name: "identified custom endpoint gets no private fields", stream: true, disableReasoning: true,
			cfg: Config{APIKey: "test", ProviderID: "kimi-cn", ProfileID: "p1", BaseURL: "https://proxy.example/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-k2.6"},
		},
		{
			name: "unidentified custom endpoint fails closed", stream: true, disableReasoning: true,
			cfg: Config{APIKey: "test", BaseURL: "https://proxy.example/deepseek", Protocol: ProtocolOpenAIChat, Model: "deepseek-chat"},
		},
		{
			name: "custom endpoint uses explicit cache capability", stream: true, disableReasoning: true,
			cfg:            Config{APIKey: "test", ProviderID: "custom", ProfileID: "p-custom", BaseURL: "https://proxy.example/v1", Protocol: ProtocolOpenAIChat, Model: "compatible-model", SupportsPromptCache: true},
			wantCapability: "profile_capability_openai_chat", wantKey: true, wantIncludeUsage: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := PrepareRequest(tt.cfg, Request{
				UserPrompt: "hello", AffinityScopeID: "thread-1", Purpose: RequestPurposeAgentTurn,
				DisableReasoning: tt.disableReasoning,
			}, tt.stream)
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if prepared.Cache.CapabilityID != tt.wantCapability || prepared.Cache.KeyApplied != tt.wantKey {
				t.Fatalf("unexpected cache metadata: %+v", prepared.Cache)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			keyRaw, hasKey := body["prompt_cache_key"]
			if hasKey != tt.wantKey {
				t.Fatalf("prompt_cache_key presence=%v body=%s", hasKey, prepared.Body)
			}
			if hasKey {
				var key string
				if err := json.Unmarshal(keyRaw, &key); err != nil || !strings.HasPrefix(key, "aw1_") {
					t.Fatalf("invalid affinity key %s: %v", keyRaw, err)
				}
			}
			_, hasStreamOptions := body["stream_options"]
			if hasStreamOptions != tt.wantIncludeUsage {
				t.Fatalf("stream_options presence=%v body=%s", hasStreamOptions, prepared.Body)
			}
			_, hasThinking := body["thinking"]
			if hasThinking != tt.wantThinkingField {
				t.Fatalf("thinking presence=%v body=%s", hasThinking, prepared.Body)
			}
		})
	}
}

func TestStreamingPreparedKimiBodyMatchesMockServerExactly(t *testing.T) {
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var err error
		captured, err = io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("x-request-id", "kimi-request-1")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	originalClient := streamHTTPClient
	streamHTTPClient = &http.Client{Transport: rewriteHostTransport{target: serverURL, base: http.DefaultTransport}}
	defer func() { streamHTTPClient = originalClient }()

	cfg := Config{
		APIKey: "test", ProviderID: "kimi-coding-plan", ProfileID: "profile-1", CredentialRevision: 1,
		BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-for-coding",
	}
	prepared, err := PrepareRequest(cfg, Request{
		RequestID: "request-1", UserPrompt: "hello", AffinityScopeID: "thread-1", Purpose: RequestPurposeAgentTurn,
	}, true)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	wantBody := append([]byte(nil), prepared.Body...)
	response, err := StreamPreparedWithObserver(context.Background(), cfg, prepared, StreamObserver{})
	if err != nil {
		t.Fatalf("send prepared: %v", err)
	}
	if !bytes.Equal(captured, wantBody) {
		t.Fatalf("wire body differs from prepared body\nwant: %s\n got: %s", wantBody, captured)
	}
	if response.ProviderRequestID != "kimi-request-1" {
		t.Fatalf("provider request ID not captured: %+v", response)
	}
}

func TestChoiceUsageKeepsNullableTypedProviderFields(t *testing.T) {
	t.Run("deepseek hit and miss", func(t *testing.T) {
		usage := choiceUsage(&openAIUsage{
			PromptCacheHitTokens: testIntPointer(80), PromptCacheMissTokens: testIntPointer(20),
			CompletionTokens: testIntPointer(5), TotalTokens: testIntPointer(105),
		}, "deepseek_cache_usage_v1")
		if usage == nil || tokenValue(usage.InputTokens) != 100 || tokenValue(usage.CachedInputTokens) != 80 || tokenValue(usage.UncachedInputTokens) != 20 {
			t.Fatalf("unexpected DeepSeek usage: %+v", usage)
		}
		if usage.Raw.PromptCacheHitTokens == nil || *usage.Raw.PromptCacheHitTokens != 80 {
			t.Fatalf("raw DeepSeek fields lost: %+v", usage.Raw)
		}
	})

	t.Run("kimi cached subset", func(t *testing.T) {
		usage := choiceUsage(&openAIUsage{PromptTokens: testIntPointer(100), CachedTokens: testIntPointer(64)}, "kimi_cached_tokens_v1")
		if usage == nil || tokenValue(usage.CachedInputTokens) != 64 || tokenValue(usage.UncachedInputTokens) != 36 {
			t.Fatalf("unexpected Kimi usage: %+v", usage)
		}
	})

	t.Run("cache write is excluded from non-cached input", func(t *testing.T) {
		usage := choiceUsage(&openAIUsage{
			PromptTokens: testIntPointer(100), CachedTokens: testIntPointer(60),
			CacheCreationInputTokens: testIntPointer(10), CompletionTokens: testIntPointer(5),
		}, "openai_usage_probe_v1")
		if usage == nil || tokenValue(usage.UncachedInputTokens) != 30 || tokenValue(usage.TotalTokens) != 105 {
			t.Fatalf("cache write breakdown is incorrect: %+v", usage)
		}
	})

	t.Run("missing stays unknown", func(t *testing.T) {
		usage := choiceUsage(&openAIUsage{TotalTokens: testIntPointer(10)}, "openai_usage_probe_v1")
		if usage == nil || usage.InputTokens != nil || usage.CachedInputTokens != nil || usage.UncachedInputTokens != nil || tokenValue(usage.TotalTokens) != 10 {
			t.Fatalf("missing fields collapsed to zero: %+v", usage)
		}
	})

	t.Run("invalid relation does not invent uncached", func(t *testing.T) {
		usage := choiceUsage(&openAIUsage{PromptTokens: testIntPointer(10), CachedTokens: testIntPointer(12)}, "kimi_cached_tokens_v1")
		if usage == nil || usage.UncachedInputTokens != nil {
			t.Fatalf("invalid relation produced uncached tokens: %+v", usage)
		}
	})

	t.Run("openai reasoning remains an output subset", func(t *testing.T) {
		details := &struct {
			ReasoningTokens *int `json:"reasoning_tokens,omitempty"`
		}{ReasoningTokens: testIntPointer(7)}
		usage := choiceUsage(&openAIUsage{
			PromptTokens: testIntPointer(20), CompletionTokens: testIntPointer(10),
			CompletionTokensDetails: details,
		}, "openai_usage_probe_v1")
		if usage == nil || tokenValue(usage.OutputTokens) != 10 || tokenValue(usage.ReasoningTokens) != 7 || tokenValue(usage.Raw.ReasoningTokens) != 7 {
			t.Fatalf("OpenAI reasoning usage was not preserved: %+v", usage)
		}
	})

	t.Run("anthropic derives inclusive input from native breakdown", func(t *testing.T) {
		usage := anthropicResponseUsage(&anthropicUsage{
			InputTokens: testIntPointer(20), OutputTokens: testIntPointer(5),
			CacheReadInputTokens: testIntPointer(80), CacheCreationInputTokens: testIntPointer(10),
		})
		if usage == nil || tokenValue(usage.InputTokens) != 110 || tokenValue(usage.UncachedInputTokens) != 20 ||
			tokenValue(usage.CachedInputTokens) != 80 || tokenValue(usage.CacheWriteTokens) != 10 || tokenValue(usage.TotalTokens) != 115 {
			t.Fatalf("Anthropic inclusive usage is incorrect: %+v", usage)
		}
		if usage.Raw.NormalizerID != "anthropic_usage_v2" {
			t.Fatalf("unexpected Anthropic normalizer: %+v", usage.Raw)
		}
	})

	t.Run("anthropic stream merges final output with initial input", func(t *testing.T) {
		initial := anthropicResponseUsage(&anthropicUsage{
			InputTokens: testIntPointer(20), CacheReadInputTokens: testIntPointer(80),
		})
		final := anthropicResponseUsage(&anthropicUsage{OutputTokens: testIntPointer(5)})
		usage := mergeAnthropicResponseUsage(initial, final)
		if usage == nil || tokenValue(usage.InputTokens) != 100 || tokenValue(usage.OutputTokens) != 5 || tokenValue(usage.TotalTokens) != 105 {
			t.Fatalf("Anthropic stream usage merge is incorrect: %+v", usage)
		}
	})
}

func TestKimiProviderOpaqueReasoningReplaysOnlyForExactContractIdentity(t *testing.T) {
	cfg := Config{
		APIKey: "test", ProviderID: "kimi-coding-plan", ProfileID: "profile-1", CredentialRevision: 2,
		BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-for-coding",
	}
	state := &convcontext.ProviderOpaqueMessageState{
		ProviderID: "kimi-coding-plan", ProfileID: "profile-1", CredentialRevision: 2,
		ModelID: "kimi-for-coding", ProtocolFamily: "openai_chat",
		EndpointContractID: EndpointContractOpenAIChat, Content: "provider-private-reasoning",
	}
	request := Request{
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: "first"},
			{Role: convcontext.RoleAssistant, Content: "answer", ProviderOpaqueState: state},
			{Role: convcontext.RoleUser, Content: "continue"},
		},
		AffinityScopeID: "thread-1", Purpose: RequestPurposeAgentTurn,
	}
	prepared, err := PrepareRequest(cfg, request, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := reasoningContentFromPrepared(t, prepared, 1); got != state.Content {
		t.Fatalf("matching Kimi state was not replayed: %q", got)
	}

	mutations := []struct {
		name string
		cfg  Config
	}{
		{name: "profile", cfg: func() Config { next := cfg; next.ProfileID = "profile-2"; return next }()},
		{name: "credential", cfg: func() Config { next := cfg; next.CredentialRevision = 3; return next }()},
		{name: "model", cfg: func() Config { next := cfg; next.Model = "k3"; return next }()},
		{name: "custom endpoint", cfg: func() Config { next := cfg; next.BaseURL = "https://proxy.example/v1"; return next }()},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			prepared, err := PrepareRequest(mutation.cfg, request, true)
			if err != nil {
				t.Fatal(err)
			}
			if got := reasoningContentFromPrepared(t, prepared, 1); got != "" {
				t.Fatalf("mismatched provider state leaked to request: %q", got)
			}
		})
	}

	encoded, err := json.Marshal(request.Messages[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), state.Content) || strings.Contains(string(encoded), "ProviderOpaque") {
		t.Fatalf("provider opaque state leaked through generic message JSON: %s", encoded)
	}
}

func reasoningContentFromPrepared(t *testing.T, prepared PreparedRequest, messageIndex int) string {
	t.Helper()
	var body struct {
		Messages []struct {
			ReasoningContent string `json:"reasoning_content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if messageIndex < 0 || messageIndex >= len(body.Messages) {
		t.Fatalf("message index %d out of range: %s", messageIndex, prepared.Body)
	}
	return body.Messages[messageIndex].ReasoningContent
}

type rewriteHostTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (transport rewriteHostTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.URL.Scheme = transport.target.Scheme
	cloned.URL.Host = transport.target.Host
	cloned.Host = transport.target.Host
	return transport.base.RoundTrip(cloned)
}

func testIntPointer(value int) *int {
	return &value
}
