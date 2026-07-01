package modelprofile

// Provider describes a built-in model provider that can be configured from the
// desktop model settings UI. It intentionally contains no secrets.
type Provider struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	BaseURL       string   `json:"base_url"`
	Protocol      string   `json:"protocol"`
	AuthScheme    string   `json:"auth_scheme"`
	KeyEnvName    string   `json:"key_env_name"`
	DefaultModel  string   `json:"default_model"`
	Models        []string `json:"models"`
	DocsURL       string   `json:"docs_url"`
	APIDocsURL    string   `json:"api_docs_url"`
	CodingPreset  bool     `json:"coding_preset"`
	EndpointModel bool     `json:"endpoint_model"`
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
		},
		{
			ID:           "kimi-coding-plan",
			DisplayName:  "Kimi Coding Plan",
			BaseURL:      "https://api.moonshot.cn/v1",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "MOONSHOT_API_KEY",
			DefaultModel: "kimi-k2.7-code",
			Models: []string{
				"kimi-k2.7-code",
				"kimi-k2.7-code-highspeed",
				"kimi-k2.6",
			},
			DocsURL:      "https://platform.kimi.com/docs/guide/kimi-k2-7-code-quickstart",
			APIDocsURL:   "https://platform.kimi.com/console/api-keys",
			CodingPreset: true,
		},
		{
			ID:           "volcengine",
			DisplayName:  "火山引擎",
			BaseURL:      "https://ark.cn-beijing.volces.com/api/v3",
			Protocol:     "openai_chat",
			AuthScheme:   "bearer",
			KeyEnvName:   "ARK_API_KEY",
			DefaultModel: "ep-your-ark-endpoint-id",
			Models: []string{
				"ep-your-ark-endpoint-id",
			},
			DocsURL:       "https://www.volcengine.com/docs/82379/1330626",
			APIDocsURL:    "https://www.volcengine.com/docs/82379/1541594",
			EndpointModel: true,
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
