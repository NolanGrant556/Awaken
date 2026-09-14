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
		{"glm 5.3 low", "glm", "glm-5.3", "", "", "low", "low"},
		{"glm coding max", "glm-coding-plan", "glm-5.2", "", "", "max", "max"},
		{"glm coding 5.3 max", "glm-coding-plan", "glm-5.3", "", "", "max", "max"},
		{"deepseek flash max", "deepseek", "deepseek-flash", "", "", "max", "max"},
		{"qwen token max official", "qwen-token-plan", "qwen3.8-max", "", "", "max", "xhigh"},
		{"qwen flash high", "qwen", "qwen3.8-flash", "", "", "high", "medium"},
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
		{"doubao evolving max", "doubao", "doubao-seed-evolving", "", "model", "max", "high"},
		{"doubao turbo high", "doubao", "doubao-seed-2-1-turbo-260628", "", "model", "high", "medium"},
		{"doubao 2.0 lite low", "doubao", "doubao-seed-2-0-lite-260428", "", "model", "low", "low"},
		{"doubao hosted deepseek ga max", "doubao", "deepseek-v4-pro-ga-260813", "", "model", "max", "high"},
		{"doubao hosted deepseek ga low", "doubao", "deepseek-v4-flash-ga-260731", "", "model", "low", "low"},
		{"doubao hosted glm 5.2 max", "doubao", "glm-5-2-260617", "", "model", "max", "max"},
		{"doubao hosted glm flash hidden", "doubao", "glm-5-3-flash-260828", "", "model", "max", ""},
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

func TestKimiK3AuxiliaryRequestUsesLowEffortInsteadOfInheritingThreadEffort(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat,
		ProviderID: "kimi-cn", Model: "kimi-k3", ReasoningEffort: "max",
	}, Request{Purpose: RequestPurposeThreadTitle, ReasoningPolicy: "max"}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if got, _ := body["reasoning_effort"].(string); got != "low" {
		t.Fatalf("auxiliary request reasoning_effort = %q, want low: %s", got, prepared.Body)
	}
}

func TestKimiK3CompactSummaryUsesLowEffortWhenDisableReasoningContractUnavailable(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat,
		ProviderID: "kimi-cn", Model: "kimi-k3", ReasoningEffort: "max",
	}, Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	var effort string
	if err := json.Unmarshal(body["reasoning_effort"], &effort); err != nil {
		t.Fatalf("missing reasoning_effort for Kimi K3 compact summary: %s", prepared.Body)
	}
	if effort != "low" {
		t.Fatalf("compact_summary reasoning_effort = %q, want low: %s", effort, prepared.Body)
	}
	if raw, exists := body["thinking"]; exists {
		t.Fatalf("Kimi K3 compact summary must not emit unsupported thinking disable contract: %s", raw)
	}
	if raw, exists := body["enable_thinking"]; exists {
		t.Fatalf("Kimi K3 compact summary must not emit unsupported enable_thinking contract: %s", raw)
	}
}

func TestDoubaoSeedResponsesAuxiliaryRequestDisablesThinking(t *testing.T) {
	for _, model := range []string{"doubao-seed-evolving", "doubao-seed-2-1-pro-260628"} {
		prepared, err := PrepareRequest(Config{
			APIKey: "test", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
			Protocol: ProtocolOpenAIResponses, ProviderID: "doubao",
			Model: model, ReasoningEffort: "max",
		}, Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true, ReasoningPolicy: "max"}, false)
		if err != nil {
			t.Fatal(err)
		}
		if prepared.ReasoningPolicy != "disabled" {
			t.Fatalf("%s compact summary policy = %q, want disabled", model, prepared.ReasoningPolicy)
		}
		var body struct {
			Thinking  *openaiThinkingConfig     `json:"thinking"`
			Reasoning *openAIResponsesReasoning `json:"reasoning"`
		}
		if err := json.Unmarshal(prepared.Body, &body); err != nil {
			t.Fatal(err)
		}
		if body.Thinking == nil || body.Thinking.Type != "disabled" {
			t.Fatalf("%s compact summary must send thinking.type disabled: %s", model, prepared.Body)
		}
		if body.Reasoning != nil {
			t.Fatalf("%s compact summary must not also send reasoning effort: %s", model, prepared.Body)
		}
	}
}

func TestDoubaoSeedChatTitleAndRecallDisableThinking(t *testing.T) {
	for _, purpose := range []RequestPurpose{RequestPurposeThreadTitle, RequestPurposeMemoryRecall} {
		prepared, err := PrepareRequest(Config{
			APIKey: "test", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
			Protocol: ProtocolOpenAIChat, ProviderID: "doubao",
			Model: "doubao-seed-2-1-pro-260628",
		}, Request{Purpose: purpose, DisableReasoning: true}, false)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(prepared.Body, &body); err != nil {
			t.Fatal(err)
		}
		var thinking openaiThinkingConfig
		if err := json.Unmarshal(body["thinking"], &thinking); err != nil || thinking.Type != "disabled" {
			t.Fatalf("%s must send thinking.type disabled: %s", purpose, prepared.Body)
		}
		if raw, exists := body["reasoning_effort"]; exists {
			t.Fatalf("%s must not send reasoning_effort with thinking disabled: %s", purpose, raw)
		}
	}
}

func TestDoubaoPaygHostedModelsDisableThinkingOnAuxiliaryRequests(t *testing.T) {
	models := []string{
		"deepseek-v4-pro-ga-260813",
		"deepseek-v4-flash-ga-260731",
		"deepseek-v4-pro-260425",
		"glm-5-2-260617",
	}
	purposes := []RequestPurpose{
		RequestPurposeThreadTitle,
		RequestPurposeCompactSummary,
		RequestPurposeMemoryRecall,
	}
	for _, model := range models {
		for _, purpose := range purposes {
			prepared, err := PrepareRequest(Config{
				APIKey: "test", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, ProviderID: "doubao",
				Model: model, ReasoningEffort: "max",
			}, Request{Purpose: purpose, DisableReasoning: true, ReasoningPolicy: "max"}, false)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Thinking  *openaiThinkingConfig     `json:"thinking"`
				Reasoning *openAIResponsesReasoning `json:"reasoning"`
			}
			if err := json.Unmarshal(prepared.Body, &body); err != nil {
				t.Fatal(err)
			}
			if body.Thinking == nil || body.Thinking.Type != "disabled" {
				t.Fatalf("%s %s must send thinking.type disabled: %s", model, purpose, prepared.Body)
			}
			if body.Reasoning != nil {
				t.Fatalf("%s %s must not also send reasoning effort: %s", model, purpose, prepared.Body)
			}
		}
	}
}

func TestDoubaoPaygThinkingTypeModelsStayAlignedWithOfficialEffortTable(t *testing.T) {
	for _, model := range doubaoPaygThinkingTypeModels {
		if !supportsDoubaoPaygReasoningEffort(model) {
			t.Fatalf("%s is in the thinking.type list but not the official 深度思考 effort table", model)
		}
	}
	for _, model := range []string{
		"doubao-seed-evolving", "doubao-seed-2.1-pro", "doubao-seed-2-1-pro-260628",
		"doubao-seed-2-1-turbo-260628", "doubao-seed-2-0-code-preview-260215",
		"doubao-seed-2-0-pro-260215", "doubao-seed-2-0-lite-260428", "doubao-seed-2-0-mini-260428",
		"deepseek-v4-pro-ga-260813", "deepseek-v4-flash-ga-260731",
		"deepseek-v4-pro-260425", "deepseek-v4-flash-260425",
		"deepseek-v4-pro", "deepseek-v4-flash",
		"glm-5-2-260617", "glm-5.2",
	} {
		if !reasoningControlModelMatches(doubaoPaygThinkingTypeModels, model) {
			t.Fatalf("%s is on the official 深度思考 effort table but missing from thinking.type", model)
		}
	}
}

func TestDoubaoResponsesOpaqueEndpointDoesNotInventDisableOrEffort(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Protocol: ProtocolOpenAIResponses, ProviderID: "doubao",
		Model: "ep-test", ModelSelectorKind: "endpoint",
	}, Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if raw, exists := body["reasoning"]; exists {
		t.Fatalf("opaque Doubao endpoint must not invent reasoning control: %s", raw)
	}
}

func TestKimiK3MemoryAuxiliaryRequestUsesLowEffort(t *testing.T) {
	prepared, err := PrepareRequest(Config{
		APIKey: "test", BaseURL: "https://api.moonshot.cn/v1", Protocol: ProtocolOpenAIChat,
		ProviderID: "kimi-cn", Model: "kimi-k3", ReasoningEffort: "max",
	}, Request{Purpose: RequestPurposeMemoryRecall, ReasoningPolicy: "max"}, false)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if string(body["reasoning_effort"]) != `"low"` {
		t.Fatalf("memory auxiliary request must use low effort: %s", body["reasoning_effort"])
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
			cfg:             Config{ProviderID: "kimi-coding-plan", BaseURL: "https://api.kimi.com/coding/v1", Protocol: ProtocolOpenAIChat, Model: "kimi-for-coding-highspeed"},
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
			name:             "doubao seed uses published thinking.type disabled",
			cfg:              Config{ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Protocol: ProtocolOpenAIChat, Model: "doubao-seed-2-1-pro-260628"},
			wantCapability:   true,
			wantThinkingType: true,
		},
		{
			name:             "doubao hosted deepseek ga uses published thinking.type disabled",
			cfg:              Config{ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Protocol: ProtocolOpenAIChat, Model: "deepseek-v4-pro-ga-260813"},
			wantCapability:   true,
			wantThinkingType: true,
		},
		{
			name:             "doubao hosted glm 5.2 uses published thinking.type disabled",
			cfg:              Config{ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Protocol: ProtocolOpenAIChat, Model: "glm-5-2-260617"},
			wantCapability:   true,
			wantThinkingType: true,
		},
		{
			name: "doubao endpoint model remains fail closed without confirmed disable contract",
			cfg:  Config{ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Protocol: ProtocolOpenAIChat, Model: "ep-test"},
		},
		{
			name:             "doubao endpoint bound to seed uses published thinking.type disabled",
			cfg:              Config{ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", Protocol: ProtocolOpenAIChat, Model: "ep-test", ModelSelectorKind: "endpoint", CapabilityModelID: "doubao-seed-2-1-pro-260628"},
			wantCapability:   true,
			wantThinkingType: true,
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
