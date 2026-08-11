package llm

import (
	"encoding/json"
	"testing"
)

func TestReasoningEffortProviderMapping(t *testing.T) {
	tests := []struct {
		name, provider, model, capabilityModel, selectorKind, effort, want string
	}{
		{"kimi low", "kimi-cn", "kimi-k3", "", "", "low", "low"},
		{"kimi max", "kimi-cn", "kimi-k3", "", "", "max", "max"},
		{"kimi coding high", "kimi-coding-plan", "k3-256k", "", "", "high", "high"},
		{"glm high", "glm", "glm-5.2", "", "", "high", "high"},
		{"glm coding max", "glm-coding-plan", "glm-5.2", "", "", "max", "max"},
		{"google low", "google", "gemini-3.6-flash", "", "", "low", "low"},
		{"google high", "google", "gemini-3.5-flash-lite", "", "", "high", "high"},
		{"google unsupported max hidden", "google", "gemini-3.6-flash", "", "", "max", ""},
		{"deepseek max", "deepseek", "deepseek-v4-pro", "", "", "max", "max"},
		{"qwen high", "qwen-token-plan", "qwen3.8-max-preview", "", "", "high", "medium"},
		{"qwen max", "qwen-token-plan", "qwen3.8-max-preview", "", "", "max", "xhigh"},
		{"qwen api low", "qwen", "qwen3.8-max", "", "", "low", "low"},
		{"qwen api high", "qwen", "qwen3.8-max", "", "", "high", "medium"},
		{"qwen api max", "qwen", "qwen3.8-max", "", "", "max", "xhigh"},
		{"doubao high exact family", "doubao", "ep-test", "doubao-seed-2.1-pro", "endpoint", "high", "medium"},
		{"doubao max exact family", "doubao", "ep-test", "doubao-seed-2.1-pro", "endpoint", "max", "high"},
		{"doubao direct exact model", "doubao", "doubao-seed-2-1-pro-260628", "", "model", "high", "medium"},
		{"doubao opaque endpoint hidden", "doubao", "ep-test", "", "endpoint", "max", ""},
		{"qwen 3.7 api hidden", "qwen", "qwen3.7-max", "", "", "max", ""},
		{"minimax hidden", "minimax", "MiniMax-M3", "", "", "max", ""},
		{"agent exact qwen max", "volcengine-agent-plan", "qwen3.8-max-preview", "", "", "max", "xhigh"},
		{"agent exact glm high", "volcengine-agent-plan", "glm-5.2", "", "", "high", "high"},
		{"agent routed model hidden", "volcengine-agent-plan", "ark-code-latest", "", "", "max", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := PrepareRequest(Config{
				APIKey: "test", BaseURL: "https://example.invalid/v1", Protocol: ProtocolOpenAIChat,
				ProviderID: tt.provider, Model: tt.model, CapabilityModelID: tt.capabilityModel,
				ModelSelectorKind: tt.selectorKind,
			}, Request{Purpose: RequestPurposeAgentTurn, ReasoningPolicy: tt.effort}, false)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatal(err)
			}
			got, _ := body["reasoning_effort"].(string)
			if got != tt.want {
				t.Fatalf("reasoning_effort = %q, want %q; body=%s", got, tt.want, prepared.Body)
			}
		})
	}
}

func TestQwen38AuxiliaryRequestDoesNotDisableRequiredReasoning(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Protocol: ProtocolOpenAIChat, ProviderID: "qwen", Model: "qwen3.8-max",
	}, Request{Purpose: RequestPurposeThreadTitle, DisableReasoning: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["enable_thinking"]; exists {
		t.Fatalf("reasoning-only Qwen auxiliary request tried to disable thinking: %s", prepared.Body)
	}
}

func TestAuxiliaryRequestDoesNotInheritReasoningEffort(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat,
		ProviderID: "kimi-cn", Model: "kimi-k3", ReasoningEffort: "max",
	}, Request{Purpose: RequestPurposeThreadTitle}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["reasoning_effort"]; exists {
		t.Fatalf("auxiliary request inherited reasoning_effort: %s", prepared.Body)
	}
}

func TestMiniMaxM3UsesOfficialChatCompletionV2Endpoint(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://api.minimaxi.com/v1", Protocol: ProtocolMiniMaxChat,
		ProviderID: "minimax", Model: "MiniMax-M3",
	}, Request{Purpose: RequestPurposeAgentTurn}, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Protocol != ProtocolMiniMaxChat {
		t.Fatalf("protocol = %q", prepared.Protocol)
	}
	if prepared.Endpoint != "https://api.minimaxi.com/v1/text/chatcompletion_v2" {
		t.Fatalf("endpoint = %q", prepared.Endpoint)
	}
}

func TestDisableReasoningCapabilityIsIndependentFromPromptCache(t *testing.T) {
	tests := []struct {
		name               string
		cfg                Config
		wantCapability     bool
		wantPromptCache    bool
		wantThinkingType   bool
		wantEnableThinking bool
	}{
		{
			name:           "qwen reasoning control without prompt cache",
			cfg:            Config{ProviderID: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Protocol: ProtocolOpenAIChat, Model: "qwen3.7-max"},
			wantCapability: true, wantEnableThinking: true,
		},
		{
			name:           "kimi k2.6 supports both independent contracts",
			cfg:            Config{ProviderID: "kimi-cn", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-k2.6"},
			wantCapability: true, wantPromptCache: true, wantThinkingType: true,
		},
		{
			name:            "kimi k2.7 keeps cache but rejects disable reasoning",
			cfg:             Config{ProviderID: "kimi-coding-plan", BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-for-coding"},
			wantPromptCache: true,
		},
		{
			name:            "kimi k3 uses effort and rejects disable reasoning",
			cfg:             Config{ProviderID: "kimi-cn", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-k3"},
			wantPromptCache: true,
		},
		{
			name: "identified custom endpoint fails closed",
			cfg:  Config{ProviderID: "kimi-cn", BaseURL: "https://proxy.example/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-k2.6"},
		},
		{
			name: "anonymous model-shaped endpoint fails closed",
			cfg:  Config{BaseURL: "https://proxy.example/deepseek", Protocol: ProtocolOpenAIChat, Model: "deepseek-v4-pro"},
		},
		{
			name:            "official endpoint with unknown model fails closed",
			cfg:             Config{ProviderID: "kimi-cn", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat, Model: "custom-model"},
			wantPromptCache: true,
		},
		{
			name: "glm remains fail closed without confirmed disable contract",
			cfg:  Config{ProviderID: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Protocol: ProtocolOpenAIChat, Model: "glm-5.2"},
		},
		{
			name: "doubao endpoint model remains fail closed without confirmed disable contract",
			cfg:  Config{ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Protocol: ProtocolOpenAIChat, Model: "ep-test"},
		},
		{
			name: "agent plan remains fail closed without fixed routed model contract",
			cfg:  Config{ProviderID: "volcengine-agent-plan", BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3", Protocol: ProtocolOpenAIChat, Model: "ark-code-latest"},
		},
		{
			name:            "volcengine floating coding route keeps cache probe but fails reasoning closed",
			cfg:             Config{ProviderID: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", Protocol: ProtocolOpenAIChat, Model: "ark-code-latest"},
			wantPromptCache: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, capabilityOK := ResolveReasoningControlCapability(tt.cfg, Request{})
			if capabilityOK != tt.wantCapability {
				t.Fatalf("reasoning capability = %v, want %v", capabilityOK, tt.wantCapability)
			}
			_, cacheOK := ResolvePromptCacheCapability(tt.cfg)
			if cacheOK != tt.wantPromptCache {
				t.Fatalf("prompt cache capability = %v, want %v", cacheOK, tt.wantPromptCache)
			}
			cfg := tt.cfg
			cfg.APIKey = "test"
			prepared, err := PrepareRequest(cfg, Request{Purpose: RequestPurposeThreadTitle, DisableReasoning: true}, false)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatal(err)
			}
			_, hasThinkingType := body["thinking"]
			if hasThinkingType != tt.wantThinkingType {
				t.Fatalf("thinking field = %v, want %v; body=%s", hasThinkingType, tt.wantThinkingType, prepared.Body)
			}
			rawEnableThinking, hasEnableThinking := body["enable_thinking"]
			if hasEnableThinking != tt.wantEnableThinking {
				t.Fatalf("enable_thinking field = %v, want %v; body=%s", hasEnableThinking, tt.wantEnableThinking, prepared.Body)
			}
			if hasEnableThinking {
				var enabled bool
				if err := json.Unmarshal(rawEnableThinking, &enabled); err != nil || enabled {
					t.Fatalf("enable_thinking must be explicit false: raw=%s err=%v", rawEnableThinking, err)
				}
			}
		})
	}
}
