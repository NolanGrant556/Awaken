package llm

import "testing"

func TestResolveReasoningOffDecision(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		req  Request
		want ReasoningOffKind
	}{
		{
			name: "deepseek responses compact uses published none wire",
			cfg: Config{
				ProviderID: "deepseek", BaseURL: "https://api.deepseek.com",
				Protocol: ProtocolOpenAIResponses, Model: "deepseek-v4-pro",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "anthropic compact omits thinking block",
			cfg: Config{
				ProviderID: "minimax", BaseURL: "https://api.minimax.io/anthropic",
				Protocol: ProtocolAnthropicMessages, Model: "MiniMax-M3",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffProtocolOmit,
		},
		{
			name: "doubao responses compact uses published thinking.type disabled",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, Model: "doubao-seed-2-1-pro-260628",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "doubao evolving compact uses published thinking.type disabled",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, Model: "doubao-seed-evolving",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "doubao seed endpoint family uses published thinking.type disabled",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIChat, Model: "ep-test", ModelSelectorKind: "endpoint",
				CapabilityModelID: "doubao-seed-2-1-pro-260628",
			},
			req:  Request{Purpose: RequestPurposeMemoryRecall, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "doubao hosted deepseek ga compact uses published thinking.type disabled",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, Model: "deepseek-v4-pro-ga-260813",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "doubao hosted glm 5.2 recall uses published thinking.type disabled",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIChat, Model: "glm-5-2-260617",
			},
			req:  Request{Purpose: RequestPurposeMemoryRecall, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "doubao hosted deepseek preview title uses published thinking.type disabled",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIChat, Model: "deepseek-v4-flash-260425",
			},
			req:  Request{Purpose: RequestPurposeThreadTitle, DisableReasoning: true},
			want: ReasoningOffWire,
		},
		{
			name: "doubao hosted glm flash stays unpublished",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, Model: "glm-5-3-flash-260828",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffUnsupported,
		},
		{
			name: "volcengine coding plan chat stays unpublished",
			cfg: Config{
				ProviderID: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
				Protocol: ProtocolOpenAIChat, Model: "doubao-seed-evolving",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffUnsupported,
		},
		{
			name: "volcengine agent plan chat stays unpublished",
			cfg: Config{
				ProviderID: "volcengine-agent-plan", BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3",
				Protocol: ProtocolOpenAIChat, Model: "ark-code-latest",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffUnsupported,
		},
		{
			name: "doubao opaque endpoint stays unpublished",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, Model: "ep-test", ModelSelectorKind: "endpoint",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffUnsupported,
		},
		{
			name: "unmatched responses omit is not off",
			cfg: Config{
				ProviderID: "openai", BaseURL: "https://api.openai.com/v1",
				Protocol: ProtocolOpenAIResponses, Model: "gpt-test",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveReasoningOffDecision(tt.cfg, tt.req)
			if got.Kind != tt.want {
				t.Fatalf("kind = %q, want %q", got.Kind, tt.want)
			}
		})
	}
}
