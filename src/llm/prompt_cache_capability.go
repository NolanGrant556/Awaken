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
	PromptCacheUndocumented    PromptCacheMode = "undocumented"
)

type AffinityKeyContract string

const (
	AffinityKeyUnsupported AffinityKeyContract = "unsupported"
	AffinityKeyRecommended AffinityKeyContract = "recommended"
	AffinityKeyRequired    AffinityKeyContract = "required"
)

const (
	EndpointContractOpenAIChat = "openai_chat_completions_v1"
	promptSchemaVersion        = "awaken_prompt_v1"
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
	if resolveProtocol(cfg) == ProtocolAnthropicMessages {
		return "anthropic_messages_v1"
	}
	return EndpointContractOpenAIChat
}

func ResolvePromptCacheCapability(cfg Config) (PromptCacheCapability, bool) {
	providerID := normalizeProviderID(cfg.ProviderID)
	baseURL := normalizeCapabilityBaseURL(cfg.BaseURL)
	protocol := resolveProtocol(cfg)
	for _, capability := range promptCacheCapabilities {
		if capability.ProviderID == providerID &&
			normalizeCapabilityBaseURL(capability.BaseURL) == baseURL &&
			capability.ProtocolFamily == protocol &&
			capability.EndpointContractID == EndpointContractOpenAIChat {
			return capability, true
		}
	}
	return PromptCacheCapability{}, false
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
	return "openai_usage_probe_v1"
}

func vendorPrivateFieldsAllowed(cfg Config) bool {
	if strings.TrimSpace(cfg.ProviderID) == "" {
		return true
	}
	_, ok := ResolvePromptCacheCapability(cfg)
	return ok
}
