package modelprofile

// Provider describes a built-in model provider that can be configured from the
// desktop model settings UI. It intentionally contains no secrets.
type Provider struct {
	ID                string                     `json:"id"`
	DisplayName       string                     `json:"display_name"`
	BaseURL           string                     `json:"base_url"`
	Protocol          string                     `json:"protocol"`
	AuthScheme        string                     `json:"auth_scheme"`
	KeyEnvName        string                     `json:"key_env_name"`
	DefaultModel      string                     `json:"default_model"`
	Models            []string                   `json:"models"`
	DocsURL           string                     `json:"docs_url"`
	APIDocsURL        string                     `json:"api_docs_url"`
	AccessType        string                     `json:"access_type"`
	CodingPreset      bool                       `json:"coding_preset"`
	EndpointModel     bool                       `json:"endpoint_model"`
	AllowCustomModel  bool                       `json:"allow_custom_model"`
	ModelCapabilities map[string]ModelCapability `json:"model_capabilities,omitempty"`
}

type ModelCapability struct {
	ContextWindow      int                        `json:"context_window,omitempty"`
	MaxOutputTokens    int                        `json:"max_output_tokens,omitempty"`
	DefaultMaxTokens   int                        `json:"default_max_tokens,omitempty"`
	DefaultTemperature float64                    `json:"default_temperature,omitempty"`
	SupportsVision     bool                       `json:"supports_vision,omitempty"`
	SupportsNativePDF  bool                       `json:"supports_native_pdf,omitempty"`
	SupportsVideo      bool                       `json:"supports_video,omitempty"`
	SupportsTools      string                     `json:"supports_tools,omitempty"`
	ReasoningEffort    *ReasoningEffortCapability `json:"reasoning_effort,omitempty"`
}

type ReasoningEffortCapability struct {
	Levels       []string `json:"levels"`
	DefaultLevel string   `json:"default_level"`
}

func effort(levels []string, defaultLevel string) *ReasoningEffortCapability {
	return &ReasoningEffortCapability{Levels: levels, DefaultLevel: defaultLevel}
}

func BuiltinProviders() []Provider {
	providers := []Provider{
		{
			ID:           "deepseek",
			DisplayName:  "DeepSeek API",
			BaseURL:      "https://api.deepseek.com",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "DEEPSEEK_API_KEY",
			DefaultModel: "deepseek-v4-pro",
			Models: []string{
				"deepseek-v4-pro",
				"deepseek-v4-flash",
			},
			DocsURL:    "https://api-docs.deepseek.com/",
			APIDocsURL: "https://platform.deepseek.com/api_keys",
			AccessType: "api",
			ModelCapabilities: map[string]ModelCapability{
				"deepseek-v4-pro": {
					ContextWindow:    1000000,
					MaxOutputTokens:  384000,
					DefaultMaxTokens: 8192,
					SupportsVision:   false,
					SupportsVideo:    false,
					SupportsTools:    "ready",
					ReasoningEffort:  effort([]string{"high", "max"}, "high"),
				},
				"deepseek-v4-flash": {
					ContextWindow:    1000000,
					MaxOutputTokens:  384000,
					DefaultMaxTokens: 8192,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
			},
		},
		{
			ID:           "kimi-cn",
			DisplayName:  "Kimi API",
			BaseURL:      "https://api.moonshot.cn/v1",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "MOONSHOT_API_KEY",
			DefaultModel: "kimi-k3",
			Models: []string{
				"kimi-k3",
			},
			DocsURL:    "https://platform.kimi.com/docs",
			APIDocsURL: "https://platform.kimi.com/console/api-keys",
			AccessType: "api",
			ModelCapabilities: map[string]ModelCapability{
				"kimi-k3": {
					ContextWindow: 1024 * 1024, MaxOutputTokens: 256 * 1024, DefaultMaxTokens: 32 * 1024,
					SupportsVision: true, SupportsVideo: true, SupportsTools: "ready",
					ReasoningEffort: effort([]string{"low", "high", "max"}, "max"),
				},
				"kimi-k2.7-code": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"kimi-k2.7-code-highspeed": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"kimi-k2.6": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"kimi-k2.5": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"moonshot-v1-128k": {
					ContextWindow:    128 * 1024,
					MaxOutputTokens:  128 * 1024,
					DefaultMaxTokens: 8192,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"moonshot-v1-32k": {
					ContextWindow:    32 * 1024,
					MaxOutputTokens:  32 * 1024,
					DefaultMaxTokens: 8192,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"moonshot-v1-8k": {
					ContextWindow:    8 * 1024,
					MaxOutputTokens:  8 * 1024,
					DefaultMaxTokens: 4096,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
			},
		},
		{
			ID:           "kimi-coding-plan",
			DisplayName:  "Kimi Coding Plan",
			BaseURL:      "https://api.kimi.com/coding/v1",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "KIMI_CODE_API_KEY",
			DefaultModel: "k3",
			Models: []string{
				"k3",
				"k3-256k",
				"kimi-for-coding",
				"kimi-for-coding-highspeed",
			},
			DocsURL:      "https://www.kimi.com/code/docs/kimi-code/models.html",
			APIDocsURL:   "https://www.kimi.com/code/console",
			CodingPreset: true,
			AccessType:   "coding_plan",
			ModelCapabilities: map[string]ModelCapability{
				"k3": {
					ContextWindow:      1024 * 1024,
					MaxOutputTokens:    32 * 1024,
					DefaultMaxTokens:   32 * 1024,
					DefaultTemperature: 1,
					SupportsVision:     true,
					SupportsVideo:      true,
					SupportsTools:      "ready",
					ReasoningEffort:    effort([]string{"low", "high", "max"}, "high"),
				},
				"k3-256k": {
					ContextWindow: 256 * 1024, MaxOutputTokens: 32 * 1024, DefaultMaxTokens: 32 * 1024,
					DefaultTemperature: 1, SupportsVision: true, SupportsVideo: true, SupportsTools: "ready",
					ReasoningEffort: effort([]string{"low", "high", "max"}, "high"),
				},
				"kimi-for-coding": {
					ContextWindow:      256 * 1024,
					MaxOutputTokens:    32 * 1024,
					DefaultMaxTokens:   32 * 1024,
					DefaultTemperature: 1,
					SupportsVision:     true,
					SupportsVideo:      true,
				},
				"kimi-for-coding-highspeed": {
					ContextWindow:      256 * 1024,
					MaxOutputTokens:    32 * 1024,
					DefaultMaxTokens:   32 * 1024,
					DefaultTemperature: 1,
					SupportsVision:     true,
					SupportsVideo:      true,
				},
			},
		},
		{
			ID:           "volcengine",
			DisplayName:  "火山 Coding Plan",
			BaseURL:      "https://ark.cn-beijing.volces.com/api/coding/v3",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "ARK_API_KEY",
			DefaultModel: "ark-code-latest",
			Models: []string{
				"ark-code-latest",
				"doubao-seed-code",
				"doubao-seed-2.0-code",
				"doubao-seed-2.0-pro",
				"doubao-seed-2.0-lite",
				"minimax-m2.5",
				"minimax-m2.7",
				"minimax-m3",
				"glm-4.7",
				"glm-5.2",
				"glm-latest",
				"deepseek-v3.2",
				"deepseek-v4-flash",
				"deepseek-v4-pro",
				"kimi-k2.5",
				"kimi-k2.6",
				"kimi-k2.7-code",
			},
			DocsURL:          "https://www.volcengine.com/docs/82379/2188958",
			APIDocsURL:       "https://www.volcengine.com/docs/82379/1541594",
			CodingPreset:     true,
			AccessType:       "coding_plan",
			AllowCustomModel: true,
			ModelCapabilities: map[string]ModelCapability{
				"ark-code-latest": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"doubao-seed-code": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"doubao-seed-2.0-code": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"doubao-seed-2.0-pro": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"doubao-seed-2.0-lite": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"minimax-m2.5": {
					ContextWindow:    200000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"minimax-m2.7": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"minimax-m3": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   true,
					SupportsVideo:    false,
				},
				"glm-4.7": {
					ContextWindow:    200000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
				"glm-5.2": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
				"glm-latest": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
				"deepseek-v3.2": {
					ContextWindow:    128000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
				"deepseek-v4-flash": {
					ContextWindow:    128000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
				"deepseek-v4-pro": {
					ContextWindow:    128000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   false,
					SupportsVideo:    false,
				},
				"kimi-k2.5": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"kimi-k2.6": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
				"kimi-k2.7-code": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
					SupportsVision:   true,
					SupportsVideo:    true,
				},
			},
		},
		{
			ID: "qwen-token-plan", DisplayName: "Qwen Token Plan",
			BaseURL:  "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
			Protocol: "openai_chat", AuthScheme: "bearer", KeyEnvName: "DASHSCOPE_TOKEN_PLAN_API_KEY",
			DefaultModel: "qwen3.8-max-preview", Models: []string{"qwen3.8-max-preview"},
			DocsURL: "https://help.aliyun.com/zh/model-studio/token-plan", APIDocsURL: "https://bailian.console.aliyun.com/",
			AccessType: "coding_plan", CodingPreset: true,
			ModelCapabilities: map[string]ModelCapability{
				"qwen3.8-max-preview": {ContextWindow: 1024 * 1024, MaxOutputTokens: 64000, DefaultMaxTokens: 32000, SupportsVision: true, SupportsVideo: true, SupportsTools: "ready", ReasoningEffort: effort([]string{"low", "high", "max"}, "max")},
			},
		},
		{
			ID: "minimax", DisplayName: "MiniMax API", BaseURL: "https://api.minimax.io/v1",
			Protocol: "minimax_chat", AuthScheme: "bearer", KeyEnvName: "MINIMAX_API_KEY",
			DefaultModel: "MiniMax-M3", Models: []string{"MiniMax-M3"},
			DocsURL: "https://www.minimax.io/models/text/m3", APIDocsURL: "https://platform.minimax.io/user-center/basic-information/interface-key", AccessType: "api",
			ModelCapabilities: map[string]ModelCapability{"MiniMax-M3": {ContextWindow: 256000, MaxOutputTokens: 128000, DefaultMaxTokens: 32000, SupportsVision: true, SupportsVideo: true, SupportsTools: "ready"}},
		},
		{
			ID: "glm", DisplayName: "GLM API", BaseURL: "https://open.bigmodel.cn/api/paas/v4",
			Protocol: "openai_chat", AuthScheme: "bearer", KeyEnvName: "ZHIPU_API_KEY",
			DefaultModel: "glm-5.2", Models: []string{"glm-5.2"},
			DocsURL: "https://docs.bigmodel.cn/cn/guide/models/text/glm-5.2", APIDocsURL: "https://open.bigmodel.cn/usercenter/apikeys", AccessType: "api",
			ModelCapabilities: map[string]ModelCapability{"glm-5.2": {ContextWindow: 256000, MaxOutputTokens: 128000, DefaultMaxTokens: 32000, SupportsTools: "ready", ReasoningEffort: effort([]string{"high", "max"}, "high")}},
		},
		{
			ID: "qwen", DisplayName: "Qwen API", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Protocol: "openai_chat", AuthScheme: "bearer", KeyEnvName: "DASHSCOPE_API_KEY",
			DefaultModel: "qwen3.7-max", Models: []string{"qwen3.7-max"}, AllowCustomModel: true,
			DocsURL: "https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions", APIDocsURL: "https://bailian.console.aliyun.com/", AccessType: "api",
			ModelCapabilities: map[string]ModelCapability{"qwen3.7-max": {ContextWindow: 256000, MaxOutputTokens: 64000, DefaultMaxTokens: 32000, SupportsVision: true, SupportsVideo: true, SupportsTools: "ready"}},
		},
		{
			ID: "doubao", DisplayName: "Doubao Ark API", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
			Protocol: "openai_chat", AuthScheme: "bearer", KeyEnvName: "ARK_API_KEY",
			DefaultModel: "doubao-seed-2.1-pro", Models: []string{"doubao-seed-2.1-pro"}, EndpointModel: true,
			DocsURL: "https://www.volcengine.com/product/doubao", APIDocsURL: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey", AccessType: "api",
			ModelCapabilities: map[string]ModelCapability{"doubao-seed-2.1-pro": {ContextWindow: 256000, MaxOutputTokens: 128000, DefaultMaxTokens: 32000, SupportsVision: true, SupportsVideo: true, SupportsTools: "ready", ReasoningEffort: effort([]string{"low", "high", "max"}, "high")}},
		},
		{
			ID: "volcengine-agent-plan", DisplayName: "火山 Agent Plan", BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3",
			Protocol: "openai_chat", AuthScheme: "bearer", KeyEnvName: "AWAKEN_AGENT_PLAN_API_KEY",
			DefaultModel: "ark-code-latest", Models: []string{"ark-code-latest"}, AllowCustomModel: true,
			DocsURL: "https://www.volcengine.com/docs/82379", APIDocsURL: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey", AccessType: "agent_plan",
			ModelCapabilities: map[string]ModelCapability{"ark-code-latest": {ContextWindow: 256000, MaxOutputTokens: 32000, DefaultMaxTokens: 32000, SupportsVision: true, SupportsTools: "ready"}},
		},
	}
	for providerIndex := range providers {
		for modelID, capability := range providers[providerIndex].ModelCapabilities {
			capability.SupportsTools = "ready"
			providers[providerIndex].ModelCapabilities[modelID] = capability
		}
	}
	return providers
}

func ProviderByID(id string) (Provider, bool) {
	for _, provider := range BuiltinProviders() {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}
