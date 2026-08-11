package llm

// openAIChatStreamingUsageCapability records official OpenAI-compatible Chat
// endpoints that accept stream_options.include_usage independently of any
// prompt-cache request contract.
type openAIChatStreamingUsageCapability struct {
	ProviderID string
	BaseURL    string
}

var openAIChatStreamingUsageCapabilities = []openAIChatStreamingUsageCapability{
	{
		ProviderID: "doubao",
		BaseURL:    "https://ark.cn-beijing.volces.com/api/v3",
	},
}

func supportsOpenAIChatStreamingUsage(cfg Config) bool {
	if resolveProtocol(cfg) != ProtocolOpenAIChat {
		return false
	}
	if capability, ok := ResolvePromptCacheCapability(cfg); ok && capability.StreamingUsage {
		return true
	}
	providerID := normalizeProviderID(cfg.ProviderID)
	baseURL := normalizeCapabilityBaseURL(cfg.BaseURL)
	for _, capability := range openAIChatStreamingUsageCapabilities {
		if capability.ProviderID == providerID &&
			normalizeCapabilityBaseURL(capability.BaseURL) == baseURL {
			return true
		}
	}
	return false
}
