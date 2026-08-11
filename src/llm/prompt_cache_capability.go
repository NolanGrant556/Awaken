package llm

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
)

type PromptCacheMode string

const (
	PromptCacheAutomaticPrefix PromptCacheMode = "automatic_prefix"
	PromptCacheAffinity        PromptCacheMode = "affinity"
	PromptCacheExplicit        PromptCacheMode = "explicit_breakpoint"
	PromptCacheUndocumented    PromptCacheMode = "undocumented"
)

type AffinityKeyContract string

const (
	AffinityKeyUnsupported AffinityKeyContract = "unsupported"
	AffinityKeyRecommended AffinityKeyContract = "recommended"
	AffinityKeyRequired    AffinityKeyContract = "required"
)

const (
	EndpointContractOpenAIChat        = "openai_chat_completions_v1"
	EndpointContractOpenAIResponses   = "openai_responses_v1"
	EndpointContractAnthropicMessages = "anthropic_messages_v1"
	promptSchemaVersion               = "awaken_prompt_v1"
)

type PromptCacheCapability struct {
	ID                 string
	ProviderID         string
	BaseURL            string
	ProtocolFamily     Protocol
	EndpointContractID string
	Mode               PromptCacheMode
	AffinityKey        AffinityKeyContract
	WireStrategy       string
	UsageNormalizerID  string
	TokenIdentity      string
	StreamingUsage     bool
	ReasoningReplay    bool
}

var promptCacheCapabilities = []PromptCacheCapability{
	{
		ID: "openai_chat_v1", ProviderID: "openai",
		BaseURL: "https://api.openai.com/v1", ProtocolFamily: ProtocolOpenAIChat,
		EndpointContractID: EndpointContractOpenAIChat, Mode: PromptCacheAffinity,
		AffinityKey: AffinityKeyRecommended, WireStrategy: "prompt_cache_key_and_include_usage",
		UsageNormalizerID: "openai_chat_usage_v1", TokenIdentity: "prompt_tokens_with_cached_subset",
		StreamingUsage: true,
	},
	{
		ID: "openai_responses_v1", ProviderID: "openai",
		BaseURL: "https://api.openai.com/v1", ProtocolFamily: ProtocolOpenAIResponses,
		EndpointContractID: EndpointContractOpenAIResponses, Mode: PromptCacheAffinity,
		AffinityKey: AffinityKeyRecommended, WireStrategy: "prompt_cache_key",
		UsageNormalizerID: "openai_responses_usage_v1", TokenIdentity: "input_tokens_with_cached_subset",
		StreamingUsage: true,
	},
	{
		ID: "anthropic_messages_v1", ProviderID: "anthropic",
		BaseURL: "https://api.anthropic.com", ProtocolFamily: ProtocolAnthropicMessages,
		EndpointContractID: EndpointContractAnthropicMessages, Mode: PromptCacheExplicit,
		AffinityKey: AffinityKeyUnsupported, WireStrategy: "system_cache_control_ephemeral",
		UsageNormalizerID: "anthropic_usage_v2", TokenIdentity: "noncached_plus_read_plus_creation",
		StreamingUsage: true,
	},
	{
		ID: "deepseek_openai_chat_v1", ProviderID: "deepseek",
		BaseURL: "https://api.deepseek.com", ProtocolFamily: ProtocolOpenAIChat,
		EndpointContractID: EndpointContractOpenAIChat, Mode: PromptCacheAutomaticPrefix,
		AffinityKey: AffinityKeyUnsupported, WireStrategy: "automatic_prefix_no_request_fields",
		UsageNormalizerID: "deepseek_cache_usage_v1", TokenIdentity: "prompt_hit_plus_miss",
		StreamingUsage: true,
	},
	{
		ID: "kimi_cn_openai_chat_v1", ProviderID: "kimi-cn",
		BaseURL: "https://api.moonshot.cn/v1", ProtocolFamily: ProtocolOpenAIChat,
		EndpointContractID: EndpointContractOpenAIChat, Mode: PromptCacheAffinity,
		AffinityKey: AffinityKeyRecommended, WireStrategy: "prompt_cache_key_and_include_usage",
		UsageNormalizerID: "kimi_cached_tokens_v1", TokenIdentity: "prompt_tokens_with_cached_subset",
		StreamingUsage: true, ReasoningReplay: true,
	},
	{
		ID: "kimi_coding_plan_openai_chat_v1", ProviderID: "kimi-coding-plan",
		BaseURL: "https://api.kimi.com/coding/v1", ProtocolFamily: ProtocolOpenAIChat,
		EndpointContractID: EndpointContractOpenAIChat, Mode: PromptCacheAffinity,
		AffinityKey: AffinityKeyRequired, WireStrategy: "prompt_cache_key_and_include_usage",
		UsageNormalizerID: "kimi_cached_tokens_v1", TokenIdentity: "prompt_tokens_with_cached_subset",
		StreamingUsage: true, ReasoningReplay: true,
	},
	{
		ID: "volcengine_coding_plan_openai_chat_v1", ProviderID: "volcengine",
		BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", ProtocolFamily: ProtocolOpenAIChat,
		EndpointContractID: EndpointContractOpenAIChat, Mode: PromptCacheUndocumented,
		AffinityKey: AffinityKeyUnsupported, WireStrategy: "response_probe_only",
		UsageNormalizerID: "openai_usage_probe_v1", TokenIdentity: "provider_reported_unknown",
		StreamingUsage: false,
	},
}

func normalizeProviderID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeCapabilityBaseURL(value string) string {
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

func NormalizeBaseURLForContract(value string) string {
	return normalizeCapabilityBaseURL(value)
}

func EndpointContractIDForConfig(cfg Config) string {
	if capability, ok := ResolvePromptCacheCapability(cfg); ok {
		return capability.EndpointContractID
	}
	switch resolveProtocol(cfg) {
	case ProtocolAnthropicMessages:
		return EndpointContractAnthropicMessages
	case ProtocolOpenAIResponses:
		return EndpointContractOpenAIResponses
	default:
		return EndpointContractOpenAIChat
	}
}

func ResolvePromptCacheCapability(cfg Config) (PromptCacheCapability, bool) {
	providerID := normalizeProviderID(cfg.ProviderID)
	baseURL := normalizeCapabilityBaseURL(cfg.BaseURL)
	protocol := resolveProtocol(cfg)
	endpointContract := endpointContractForProtocol(protocol)
	for _, capability := range promptCacheCapabilities {
		if capability.ProviderID == providerID &&
			normalizeCapabilityBaseURL(capability.BaseURL) == baseURL &&
			capability.ProtocolFamily == protocol &&
			capability.EndpointContractID == endpointContract {
			return capability, true
		}
	}
	if cfg.SupportsPromptCache {
		capability := PromptCacheCapability{
			ID:                 "profile_capability_" + string(protocol),
			ProviderID:         providerID,
			BaseURL:            baseURL,
			ProtocolFamily:     protocol,
			EndpointContractID: endpointContract,
			StreamingUsage:     true,
		}
		switch protocol {
		case ProtocolAnthropicMessages:
			capability.Mode = PromptCacheExplicit
			capability.AffinityKey = AffinityKeyUnsupported
			capability.WireStrategy = "system_cache_control_ephemeral"
			capability.UsageNormalizerID = "anthropic_usage_v2"
			capability.TokenIdentity = "noncached_plus_read_plus_creation"
		case ProtocolOpenAIResponses:
			capability.Mode = PromptCacheAffinity
			capability.AffinityKey = AffinityKeyRecommended
			capability.WireStrategy = "prompt_cache_key"
			capability.UsageNormalizerID = "openai_responses_usage_v1"
			capability.TokenIdentity = "input_tokens_with_cached_subset"
		case ProtocolOpenAIChat:
			capability.Mode = PromptCacheAffinity
			capability.AffinityKey = AffinityKeyRecommended
			capability.WireStrategy = "prompt_cache_key_and_include_usage"
			capability.UsageNormalizerID = "openai_chat_usage_v1"
			capability.TokenIdentity = "prompt_tokens_with_cached_subset"
		default:
			return PromptCacheCapability{}, false
		}
		return capability, true
	}
	return PromptCacheCapability{}, false
}

func endpointContractForProtocol(protocol Protocol) string {
	switch protocol {
	case ProtocolOpenAIResponses:
		return EndpointContractOpenAIResponses
	case ProtocolAnthropicMessages:
		return EndpointContractAnthropicMessages
	default:
		return EndpointContractOpenAIChat
	}
}

func promptCacheAffinityKey(cfg Config, req Request, capability PromptCacheCapability) string {
	revision := cfg.CredentialRevision
	if revision == 0 {
		revision = 1
	}
	scope := strings.TrimSpace(req.AffinityScopeID)
	if scope == "" {
		scope = "profile_fallback"
	}
	reasoningPolicy := strings.TrimSpace(req.ReasoningPolicy)
	if reasoningPolicy == "" {
		if req.DisableReasoning {
			reasoningPolicy = "disabled"
		} else {
			reasoningPolicy = "provider_default"
		}
	}
	parts := []string{
		normalizeProviderID(cfg.ProviderID), strings.TrimSpace(cfg.ProfileID),
		strconv.FormatUint(revision, 10), capability.EndpointContractID, scope,
		resolveModel(cfg, req), reasoningPolicy, promptSchemaVersion, string(req.Purpose),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "aw1_" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func promptCacheMetadata(cfg Config, req Request) (PreparedPromptCache, string) {
	capability, ok := ResolvePromptCacheCapability(cfg)
	if !ok {
		return PreparedPromptCache{}, ""
	}
	metadata := PreparedPromptCache{
		CapabilityID: capability.ID, EndpointContract: capability.EndpointContractID,
		Mode: capability.Mode, ReasoningReplay: capability.ReasoningReplay,
	}
	if capability.AffinityKey == AffinityKeyRecommended || capability.AffinityKey == AffinityKeyRequired {
		metadata.KeyApplied = true
		return metadata, promptCacheAffinityKey(cfg, req, capability)
	}
	return metadata, ""
}

func usageNormalizerID(cfg Config) string {
	if capability, ok := ResolvePromptCacheCapability(cfg); ok {
		return capability.UsageNormalizerID
	}
	switch resolveProtocol(cfg) {
	case ProtocolOpenAIResponses:
		return "openai_responses_usage_v1"
	case ProtocolAnthropicMessages:
		return "anthropic_usage_v2"
	default:
		return "openai_usage_probe_v1"
	}
}
