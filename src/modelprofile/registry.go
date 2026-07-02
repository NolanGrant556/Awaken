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
	CodingPreset      bool                       `json:"coding_preset"`
	EndpointModel     bool                       `json:"endpoint_model"`
	AllowCustomModel  bool                       `json:"allow_custom_model"`
	ModelCapabilities map[string]ModelCapability `json:"model_capabilities,omitempty"`
}

type ModelCapability struct {
	ContextWindow      int     `json:"context_window,omitempty"`
	MaxOutputTokens    int     `json:"max_output_tokens,omitempty"`
	DefaultMaxTokens   int     `json:"default_max_tokens,omitempty"`
	DefaultTemperature float64 `json:"default_temperature,omitempty"`
}

func BuiltinProviders() []Provider {
	return []Provider{
		{
			ID:           "deepseek",
			DisplayName:  "DeepSeek",
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
			ModelCapabilities: map[string]ModelCapability{
				"deepseek-v4-pro": {
					ContextWindow:    1000000,
					MaxOutputTokens:  384000,
					DefaultMaxTokens: 8192,
				},
				"deepseek-v4-flash": {
					ContextWindow:    1000000,
					MaxOutputTokens:  384000,
					DefaultMaxTokens: 8192,
				},
			},
		},
		{
			ID:           "kimi-cn",
			DisplayName:  "Kimi CN",
			BaseURL:      "https://api.moonshot.cn/v1",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "MOONSHOT_API_KEY",
			DefaultModel: "kimi-k2.6",
			Models: []string{
				"kimi-k2.6",
				"kimi-k2.7-code",
				"kimi-k2.7-code-highspeed",
				"kimi-k2.5",
				"moonshot-v1-128k",
				"moonshot-v1-32k",
				"moonshot-v1-8k",
			},
			DocsURL:    "https://platform.kimi.com/docs",
			APIDocsURL: "https://platform.kimi.com/console/api-keys",
			ModelCapabilities: map[string]ModelCapability{
				"kimi-k2.7-code": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
				},
				"kimi-k2.7-code-highspeed": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
				},
				"kimi-k2.6": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
				},
				"kimi-k2.5": {
					ContextWindow:    256 * 1024,
					MaxOutputTokens:  256 * 1024,
					DefaultMaxTokens: 32 * 1024,
				},
				"moonshot-v1-128k": {
					ContextWindow:    128 * 1024,
					MaxOutputTokens:  128 * 1024,
					DefaultMaxTokens: 8192,
				},
				"moonshot-v1-32k": {
					ContextWindow:    32 * 1024,
					MaxOutputTokens:  32 * 1024,
					DefaultMaxTokens: 8192,
				},
				"moonshot-v1-8k": {
					ContextWindow:    8 * 1024,
					MaxOutputTokens:  8 * 1024,
					DefaultMaxTokens: 4096,
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
			DefaultModel: "kimi-for-coding",
			Models: []string{
				"kimi-for-coding",
			},
			DocsURL:      "https://www.kimi.com/code/docs/en/third-party-tools/other-coding-agents.html",
			APIDocsURL:   "https://www.kimi.com/code/console",
			CodingPreset: true,
			ModelCapabilities: map[string]ModelCapability{
				"kimi-for-coding": {
					ContextWindow:      256 * 1024,
					MaxOutputTokens:    32 * 1024,
					DefaultMaxTokens:   32 * 1024,
					DefaultTemperature: 1,
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
			AllowCustomModel: true,
			ModelCapabilities: map[string]ModelCapability{
				"ark-code-latest": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"doubao-seed-code": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"doubao-seed-2.0-code": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"doubao-seed-2.0-pro": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"doubao-seed-2.0-lite": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"minimax-m2.5": {
					ContextWindow:    200000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"minimax-m2.7": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"minimax-m3": {
					ContextWindow:    256000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"glm-4.7": {
					ContextWindow:    200000,
					MaxOutputTokens:  128000,
					DefaultMaxTokens: 128000,
				},
				"glm-5.2": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"glm-latest": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"deepseek-v3.2": {
					ContextWindow:    128000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"deepseek-v4-flash": {
					ContextWindow:    128000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"deepseek-v4-pro": {
					ContextWindow:    128000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"kimi-k2.5": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"kimi-k2.6": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
				"kimi-k2.7-code": {
					ContextWindow:    256000,
					MaxOutputTokens:  32000,
					DefaultMaxTokens: 32000,
				},
			},
		},
	}
}

func ProviderByID(id string) (Provider, bool) {
	for _, provider := range BuiltinProviders() {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}
