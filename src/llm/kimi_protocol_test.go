package llm

import (
	"encoding/json"
	"testing"
)

// Verify the bytes sent by both streaming and non-streaming paths, rather
// than only the capability/UI vocabulary. Sources: Kimi API responses/messages
// references and Kimi Code models (reviewed 2026-09-14).
func TestKimiNativeEffortAcrossProtocols(t *testing.T) {
	for _, entry := range []struct{ provider, model string }{
		{"kimi-cn", "kimi-k3"}, {"kimi-coding-plan", "k3"},
		{"kimi-coding-plan", "k3-256k"}, {"kimi-coding-plan", "kimi-for-coding"},
	} {
		for _, protocol := range []Protocol{ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolAnthropicMessages} {
			for _, effort := range []string{"low", "high", "max"} {
				for _, stream := range []bool{false, true} {
					cfg := kimiTestConfig(entry.provider, entry.model, protocol)
					p, err := PrepareRequest(cfg, Request{Purpose: RequestPurposeAgentTurn, UserPrompt: "hello", ReasoningPolicy: effort}, stream)
					if err != nil {
						t.Fatal(err)
					}
					var body map[string]any
					if err := json.Unmarshal(p.Body, &body); err != nil {
						t.Fatal(err)
					}
					var got any
					switch protocol {
					case ProtocolOpenAIChat:
						got = body["reasoning_effort"]
					case ProtocolOpenAIResponses:
						if value, ok := body["reasoning"].(map[string]any); ok {
							got = value["effort"]
						}
						if body["store"] != false {
							t.Fatal("Kimi Responses must retain stateless replay")
						}
					case ProtocolAnthropicMessages:
						if value, ok := body["output_config"].(map[string]any); ok {
							got = value["effort"]
						}
					}
					if got != effort || body["thinking"] != nil || p.ReasoningPolicy != effort {
						t.Fatalf("%s/%s/%s/%s stream=%v: %s", entry.provider, entry.model, protocol, effort, stream, p.Body)
					}
				}
			}
		}
	}
}

func kimiTestConfig(provider, model string, protocol Protocol) Config {
	base := "https://api.kimi.com/coding/v1"
	if provider == "kimi-cn" {
		base = "https://api.moonshot.cn/v1"
		if protocol == ProtocolAnthropicMessages {
			base = "https://api.moonshot.cn/anthropic"
		}
	} else if protocol == ProtocolAnthropicMessages {
		base = "https://api.kimi.com/coding"
	}
	return Config{APIKey: "test", ProviderID: provider, Model: model, BaseURL: base, Protocol: protocol, MaxTokens: 32768}
}

func TestKimiAuxiliaryPolicyPreservesPlanIdentity(t *testing.T) {
	for _, provider := range []string{"kimi-cn", "kimi-coding-plan"} {
		model := "kimi-k3"
		if provider == "kimi-coding-plan" {
			model = "kimi-for-coding"
		}
		for _, protocol := range []Protocol{ProtocolOpenAIChat, ProtocolOpenAIResponses, ProtocolAnthropicMessages} {
			for _, purpose := range []RequestPurpose{RequestPurposeThreadTitle, RequestPurposeCompactSummary, RequestPurposeMemoryRecall, RequestPurposeMemoryMaintenance, RequestPurposeHookPrompt} {
				cfg := kimiTestConfig(provider, model, protocol)
				p, err := PrepareRequest(cfg, Request{Purpose: purpose, DisableReasoning: true, ReasoningPolicy: "max"}, false)
				if err != nil {
					t.Fatal(err)
				}
				want := "low"
				if provider == "kimi-coding-plan" && protocol != ProtocolOpenAIResponses {
					want = "disabled"
				}
				if p.ReasoningPolicy != want {
					t.Fatalf("%s/%s/%s: policy=%s body=%s", provider, protocol, purpose, p.ReasoningPolicy, p.Body)
				}
				var body map[string]any
				json.Unmarshal(p.Body, &body)
				if provider == "kimi-coding-plan" && protocol != ProtocolOpenAIResponses {
					field, key, value := "thinking", "type", "disabled"
					if protocol == ProtocolOpenAIChat && body["temperature"] != 0.6 {
						t.Fatalf("non-thinking K2.8 requires temperature 0.6: %s", p.Body)
					}
					control, ok := body[field].(map[string]any)
					if !ok || control[key] != value {
						t.Fatalf("missing off control: %s", p.Body)
					}
				} else if body["thinking"] != nil {
					t.Fatalf("API K3 must not receive thinking off: %s", p.Body)
				}
			}
		}
	}
}

func TestKimiFixedThinkingModelsDoNotInventEffort(t *testing.T) {
	for _, entry := range []struct{ provider, model string }{
		{"kimi-cn", "kimi-k2.7-code"}, {"kimi-cn", "kimi-k2.7-code-highspeed"},
		{"kimi-cn", "kimi-k2.6"}, {"kimi-coding-plan", "kimi-for-coding-highspeed"},
	} {
		p, err := PrepareRequest(kimiTestConfig(entry.provider, entry.model, ProtocolOpenAIChat), Request{Purpose: RequestPurposeAgentTurn, ReasoningPolicy: "max"}, false)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		json.Unmarshal(p.Body, &body)
		if body["reasoning_effort"] != nil {
			t.Fatalf("unsupported model received effort: %s", p.Body)
		}
	}
}
