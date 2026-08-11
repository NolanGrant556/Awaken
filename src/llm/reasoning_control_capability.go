package llm

import (
	"net/url"
	"strings"
)

type ReasoningControlWireStrategy string

const (
	ReasoningControlThinkingType   ReasoningControlWireStrategy = "thinking_type"
	ReasoningControlEnableThinking ReasoningControlWireStrategy = "enable_thinking"
)

// ReasoningControlCapability is deliberately independent from prompt-cache
// support. Providers document cache affinity and reasoning switches as
// separate contracts, and an endpoint may safely support one without the
// other. Private reasoning fields are therefore admitted only after an exact
// provider, official Base URL, protocol, and model-contract match.
type ReasoningControlCapability struct {
	ID             string
	ProviderID     string
	BaseURL        string
	ProtocolFamily Protocol
	Models         []string
	WireStrategy   ReasoningControlWireStrategy
}

var reasoningControlCapabilities = []ReasoningControlCapability{
	{
		ID: "deepseek_thinking_type_v1", ProviderID: "deepseek", BaseURL: "https://api.deepseek.com",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"deepseek-v4-flash", "deepseek-v4-pro"},
		WireStrategy: ReasoningControlThinkingType,
	},
	{
		ID: "kimi_cn_thinking_type_v1", ProviderID: "kimi-cn", BaseURL: "https://api.moonshot.cn/v1",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"kimi-k2.5", "kimi-k2.6"},
		WireStrategy: ReasoningControlThinkingType,
	},
	{
		ID: "qwen_api_enable_thinking_v1", ProviderID: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"qwen3.7-max", "qwen3.8-max"},
		WireStrategy: ReasoningControlEnableThinking,
	},
	{
		ID: "qwen_token_plan_enable_thinking_v1", ProviderID: "qwen-token-plan", BaseURL: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"qwen3.8-max-preview"},
		WireStrategy: ReasoningControlEnableThinking,
	},
}

func ResolveReasoningControlCapability(cfg Config, req Request) (ReasoningControlCapability, bool) {
	providerID := strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	baseURL := normalizeReasoningControlBaseURL(cfg.BaseURL)
	protocol := resolveProtocol(cfg)
	model := strings.ToLower(strings.TrimSpace(resolveModel(cfg, req)))
	for _, capability := range reasoningControlCapabilities {
		if capability.ProviderID != providerID ||
			normalizeReasoningControlBaseURL(capability.BaseURL) != baseURL ||
			capability.ProtocolFamily != protocol ||
			!reasoningControlModelMatches(capability.Models, model) {
			continue
		}
		return capability, true
	}
	return ReasoningControlCapability{}, false
}

func reasoningControlModelMatches(models []string, model string) bool {
	for _, candidate := range models {
		if strings.EqualFold(strings.TrimSpace(candidate), model) {
			return true
		}
	}
	return false
}

func normalizeReasoningControlBaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "/" {
		parsed.Path = ""
	}
	return parsed.String()
}
