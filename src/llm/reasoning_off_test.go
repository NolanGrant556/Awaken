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
			name: "doubao responses compact falls back to lowest effort",
			cfg: Config{
				ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
				Protocol: ProtocolOpenAIResponses, Model: "doubao-seed-2-1-pro-260628",
			},
			req:  Request{Purpose: RequestPurposeCompactSummary, DisableReasoning: true},
			want: ReasoningOffLowestEffort,
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
