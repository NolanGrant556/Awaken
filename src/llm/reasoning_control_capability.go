package llm

import (
	"net/url"
	"strings"
)

type ReasoningControlWireStrategy string

const (
	ReasoningControlThinkingType        ReasoningControlWireStrategy = "thinking_type"
	ReasoningControlEnableThinking      ReasoningControlWireStrategy = "enable_thinking"
	ReasoningControlResponsesEffortNone ReasoningControlWireStrategy = "responses_effort_none"
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
		ID: "deepseek_responses_effort_none_v1", ProviderID: "deepseek", BaseURL: "https://api.deepseek.com",
		ProtocolFamily: ProtocolOpenAIResponses, Models: []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro"},
		WireStrategy: ReasoningControlResponsesEffortNone,
	},
	{
		ID: "deepseek_thinking_type_v1", ProviderID: "deepseek", BaseURL: "https://api.deepseek.com",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-pro"},
		WireStrategy: ReasoningControlThinkingType,
	},
	{
		ID: "kimi_cn_thinking_type_v1", ProviderID: "kimi-cn", BaseURL: "https://api.moonshot.cn/v1",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"kimi-k2.5", "kimi-k2.6"},
		WireStrategy: ReasoningControlThinkingType,
	},
	{
		ID: "qwen_api_enable_thinking_v1", ProviderID: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"qwen3.7-max", "qwen3.8-max", "qwen3.8-flash"},
		WireStrategy: ReasoningControlEnableThinking,
	},
	{
		ID: "qwen_token_plan_enable_thinking_v1", ProviderID: "qwen-token-plan", BaseURL: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
		ProtocolFamily: ProtocolOpenAIChat, Models: []string{"qwen3.8-max-preview", "qwen3.8-max", "qwen3.8-flash"},
		WireStrategy: ReasoningControlEnableThinking,
	},
	{
		ID: "doubao_chat_thinking_type_v1", ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		ProtocolFamily: ProtocolOpenAIChat, Models: doubaoPaygThinkingTypeModels,
		WireStrategy: ReasoningControlThinkingType,
	},
	{
		ID: "doubao_responses_thinking_type_v1", ProviderID: "doubao", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		ProtocolFamily: ProtocolOpenAIResponses, Models: doubaoPaygThinkingTypeModels,
		WireStrategy: ReasoningControlThinkingType,
	},
}

// Official Ark 深度思考 Chat/Responses switch for pay-as-you-go models listed
// on that page. GLM-5.3-Flash, unbound ep-*, and Coding/Agent Plan stay out.
var doubaoPaygThinkingTypeModels = []string{
	"doubao-seed-evolving",
	"doubao-seed-2.1-pro",
	"doubao-seed-2-1-pro-260628",
	"doubao-seed-2-1-turbo-260628",
	"doubao-seed-2-0-code-preview-260215",
	"doubao-seed-2-0-pro-260215",
	"doubao-seed-2-0-lite-260428",
	"doubao-seed-2-0-mini-260428",
	"deepseek-v4-pro-ga-260813",
	"deepseek-v4-flash-ga-260731",
	"deepseek-v4-pro-260425",
	"deepseek-v4-flash-260425",
	"deepseek-v4-pro",
	"deepseek-v4-flash",
	"glm-5-2-260617",
	"glm-5.2",
}

func ResolveReasoningControlCapability(cfg Config, req Request) (ReasoningControlCapability, bool) {
	providerID := strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	baseURL := normalizeReasoningControlBaseURL(cfg.BaseURL)
	protocol := resolveProtocol(cfg)
	model := strings.ToLower(strings.TrimSpace(resolveModel(cfg, req)))
	capabilityModel := capabilityModelID(cfg, req)
	for _, capability := range reasoningControlCapabilities {
		if capability.ProviderID != providerID ||
			normalizeReasoningControlBaseURL(capability.BaseURL) != baseURL ||
			capability.ProtocolFamily != protocol {
			continue
		}
		if reasoningControlModelMatches(capability.Models, model) ||
			(capabilityModel != "" && reasoningControlModelMatches(capability.Models, capabilityModel)) {
			return capability, true
		}
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
