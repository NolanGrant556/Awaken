package connectors

import (
	"context"
	"runtime"
	"strings"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

// npxCommand returns the OS-appropriate command and arguments for running an
// npx package. On Windows, Go cannot reliably execute .cmd scripts as a long-
// lived stdio server directly, so we run them through cmd /c. On other
// platforms npx is invoked directly.
func npxCommand(args ...string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", append([]string{"/c", "npx"}, args...)
	}
	return "npx", args
}

// builtinSource implements ConnectorSource for first-party connectors.
type builtinSource struct {
	defs []ConnectorDefinition
}

// BuiltinOptions contains public, release-owned connector configuration.
// OAuth client IDs are intentionally supplied by the host build/environment;
// they are not plugin-authored and are not secrets.
type BuiltinOptions struct {
	MicrosoftClientID string
}

// NewBuiltinSource returns a ConnectorSource populated with all first-party
// connector definitions. Add new builtins here.
func NewBuiltinSource(options ...BuiltinOptions) ConnectorSource {
	return newBuiltinSource(options...)
}

// newBuiltinSource returns a ConnectorSource populated with all first-party
// connector definitions. Add new builtins here.

func newBuiltinSource(options ...BuiltinOptions) *builtinSource {
	var configured BuiltinOptions
	if len(options) > 0 {
		configured = options[0]
	}
	return &builtinSource{
		defs: []ConnectorDefinition{
			puppeteerConnector(),
			chromeDevToolsConnector(),
			playwrightConnector(),
			githubConnector(),
			dingTalkConnector(),
			larkConnector(),
			context7Connector(),
			outlookConnector(configured),
		},
	}
}

// List implements ConnectorSource.
func (s *builtinSource) List(_ context.Context) ([]ConnectorDefinition, error) {
	return s.defs, nil
}

// Origin implements ConnectorSource.
func (s *builtinSource) Origin() SourceOrigin { return SourceBuiltin }

// Priority implements ConnectorSource. Builtin has the highest priority (0).
func (s *builtinSource) Priority() int { return 0 }

// puppeteerConnector returns the Puppeteer MCP connector definition.
func puppeteerConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:               "puppeteer",
		DisplayName:      "Puppeteer",
		Description:      "Control a headless browser via Puppeteer to navigate pages, take screenshots, and run automations.",
		Category:         CategoryCode,
		Provider:         "Puppeteer",
		ProviderID:       tool.ProviderID("connector:puppeteer"),
		Capabilities:     []tool.Capability{tool.CapabilityBrowserInteractV2},
		Dependencies:     []tool.DependencyRequirement{npxDependency("@modelcontextprotocol/server-puppeteer")},
		AuthType:         AuthNone,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: puppeteerConfigFactory,
	}
}

// chromeDevToolsConnector returns the Chrome DevTools MCP connector definition.
func chromeDevToolsConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:               "chrome-devtools-mcp",
		DisplayName:      "Chrome DevTools MCP",
		Description:      "Inspect and debug web pages using the Chrome DevTools Protocol MCP server.",
		Category:         CategoryCode,
		Provider:         "Chrome DevTools MCP",
		ProviderID:       tool.ProviderID("connector:chrome-devtools-mcp"),
		Capabilities:     []tool.Capability{tool.CapabilityBrowserInteractV2},
		Dependencies:     []tool.DependencyRequirement{npxDependency("chrome-devtools-mcp")},
		AuthType:         AuthNone,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: chromeDevToolsConfigFactory,
	}
}

// playwrightConnector returns the Playwright MCP connector definition.
func playwrightConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:               "playwright",
		DisplayName:      "Playwright",
		Description:      "Automate browsers with Playwright for testing, scraping, and page interactions.",
		Category:         CategoryCode,
		Provider:         "Playwright",
		ProviderID:       tool.ProviderID("connector:playwright"),
		Capabilities:     []tool.Capability{tool.CapabilityBrowserInteractV2},
		Dependencies:     []tool.DependencyRequirement{npxDependency("@executeautomation/playwright-mcp-server")},
		AuthType:         AuthNone,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: playwrightConfigFactory,
	}
}

// githubConnector returns the GitHub MCP connector definition.
func githubConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:               "github",
		DisplayName:      "GitHub",
		Description:      "Interact with GitHub repositories, issues, pull requests, and more. Set GITHUB_PERSONAL_ACCESS_TOKEN to authenticate.",
		Category:         CategoryCode,
		Provider:         "GitHub",
		ProviderID:       tool.ProviderID("connector:github"),
		Dependencies:     []tool.DependencyRequirement{npxDependency("@modelcontextprotocol/server-github")},
		AuthType:         AuthAPIKey,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: githubConfigFactory,
	}
}

func dingTalkConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:           "dingtalk-workspace",
		Kind:         KindCLI,
		DisplayName:  "钉钉",
		Description:  "连接钉钉，管理日历、联系人、群聊、任务和审批。",
		Category:     CategoryChat,
		Provider:     "DingTalk",
		ProviderID:   tool.ProviderID("connector:dingtalk-workspace"),
		Capabilities: []tool.Capability{tool.CapabilityMessageSend},
		Dependencies: []tool.DependencyRequirement{{
			Kind: tool.DependencyOSExecutable, Name: "dws", ProbeID: "executable.dws", Source: "connector",
		}},
		AuthType:     AuthOAuth2,
		TrustLevel:   TrustBuiltin,
		SourceOrigin: SourceBuiltin,
	}
}

func larkConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:           "lark-cli",
		Kind:         KindCLI,
		DisplayName:  "飞书",
		Description:  "连接飞书，通过官方 CLI 授权调用日历、云文档与消息能力。",
		Category:     CategoryChat,
		Provider:     "Lark",
		ProviderID:   tool.ProviderID("connector:lark-cli"),
		Capabilities: []tool.Capability{tool.CapabilityMessageSend},
		Dependencies: []tool.DependencyRequirement{{
			Kind: tool.DependencyOSExecutable, Name: "lark-cli", ProbeID: "executable.lark-cli", Source: "connector",
		}},
		AuthType:     AuthOAuth2,
		TrustLevel:   TrustBuiltin,
		SourceOrigin: SourceBuiltin,
	}
}

// context7Connector returns the Upstash Context7 MCP connector definition.
func context7Connector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:               "context7",
		DisplayName:      "context7",
		Description:      "Use Upstash Context7 to retrieve contextual documentation and code references.",
		Category:         CategoryData,
		Provider:         "context7",
		ProviderID:       tool.ProviderID("connector:context7"),
		Capabilities:     []tool.Capability{tool.CapabilityWebSearch},
		Dependencies:     []tool.DependencyRequirement{npxDependency("@upstash/context7-mcp")},
		AuthType:         AuthNone,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: context7ConfigFactory,
	}
}

func outlookConnector(options BuiltinOptions) ConnectorDefinition {
	const graphResource = "https://graph.microsoft.com/"
	scopes := []string{
		"openid", "profile", "offline_access",
		"Mail.ReadWrite", "Mail.Send", "Mail.ReadWrite.Shared", "Mail.Send.Shared", "MailboxSettings.ReadWrite",
	}
	return ConnectorDefinition{
		ID:           "awaken.microsoft.outlook",
		Kind:         KindMCP,
		DisplayName:  "Outlook",
		Description:  "连接 Microsoft Outlook，管理个人与共享邮箱、草稿、附件、文件夹、分类、回复、转发、退订和定时发送。",
		Category:     CategoryChat,
		Provider:     "Microsoft",
		ProviderID:   tool.ProviderID("connector:awaken.microsoft.outlook"),
		Capabilities: []tool.Capability{tool.CapabilityMessageSend},
		Dependencies: []tool.DependencyRequirement{{
			Kind: tool.DependencyNetworkService, Name: "graph.microsoft.com", ProbeID: "network.microsoft-graph", Source: "connector",
		}},
		AuthType:       AuthOAuth2,
		RequiredScopes: append([]string(nil), scopes...),
		ServerNames:    []string{"awaken-outlook"},
		OAuth: &OAuthConnectorSpec{
			AuthorizationEndpoint: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenEndpoint:         "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			ClientID:              strings.TrimSpace(options.MicrosoftClientID),
			Scopes:                append([]string(nil), scopes...),
			Resource:              graphResource,
			AuthorizationParams:   map[string]string{"prompt": "select_account"},
		},
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		CatalogPlacement: CatalogPluginOnly,
		McpConfigFactory: outlookConfigFactory,
	}
}

func outlookConfigFactory(def ConnectorDefinition, _ []string) (map[string]mcpclient.ScopedMcpServerConfig, error) {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"awaken-outlook": {
			Name:  "awaken-outlook",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				InProc:      &mcpclient.InProcTransportConfig{ServerName: "awaken-outlook"},
				DisplayName: def.DisplayName,
			},
		},
	}, nil
}

func npxDependency(source string) tool.DependencyRequirement {
	return tool.DependencyRequirement{
		Kind:    tool.DependencyPackageManager,
		Name:    "npx",
		ProbeID: "executable.npx",
		Source:  source,
	}
}

// puppeteerConfigFactory generates MCP server configs for the Puppeteer connector.
func puppeteerConfigFactory(def ConnectorDefinition, _ []string) (map[string]mcpclient.ScopedMcpServerConfig, error) {
	cmd, args := npxCommand("-y", "@modelcontextprotocol/server-puppeteer")
	return map[string]mcpclient.ScopedMcpServerConfig{
		"puppeteer": {
			Name:  "puppeteer",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: cmd,
					Args:    args,
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}, nil
}

// chromeDevToolsConfigFactory generates MCP server configs for the Chrome DevTools MCP connector.
func chromeDevToolsConfigFactory(def ConnectorDefinition, _ []string) (map[string]mcpclient.ScopedMcpServerConfig, error) {
	cmd, args := npxCommand("-y", "chrome-devtools-mcp@latest")
	return map[string]mcpclient.ScopedMcpServerConfig{
		"chrome-devtools-mcp": {
			Name:  "chrome-devtools-mcp",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: cmd,
					Args:    args,
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}, nil
}

// playwrightConfigFactory generates MCP server configs for the Playwright connector.
func playwrightConfigFactory(def ConnectorDefinition, _ []string) (map[string]mcpclient.ScopedMcpServerConfig, error) {
	env, err := preparePlaywrightLogEnvironment()
	if err != nil {
		return nil, err
	}
	cmd, args := npxCommand("-y", "@executeautomation/playwright-mcp-server")
	return map[string]mcpclient.ScopedMcpServerConfig{
		"playwright": {
			Name:  "playwright",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: cmd,
					Args:    args,
					Env:     env,
				},
				DisplayName: def.DisplayName,
			},
		},
	}, nil
}

// githubConfigFactory generates MCP server configs for the GitHub connector.
func githubConfigFactory(def ConnectorDefinition, _ []string) (map[string]mcpclient.ScopedMcpServerConfig, error) {
	cmd, args := npxCommand("-y", "@modelcontextprotocol/server-github")
	return map[string]mcpclient.ScopedMcpServerConfig{
		"github": {
			Name:  "github",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: cmd,
					Args:    args,
				},
				DisplayName: def.DisplayName,
			},
		},
	}, nil
}

// context7ConfigFactory generates MCP server configs for the Context7 connector.
func context7ConfigFactory(def ConnectorDefinition, _ []string) (map[string]mcpclient.ScopedMcpServerConfig, error) {
	cmd, args := npxCommand("-y", "@upstash/context7-mcp@latest")
	return map[string]mcpclient.ScopedMcpServerConfig{
		"context7": {
			Name:  "context7",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: cmd,
					Args:    args,
					Env: map[string]string{
						"DEFAULT_MINIMUM_TOKENS": "10000",
					},
				},
				DisplayName: def.DisplayName,
			},
		},
	}, nil
}

// mergeScopes merges the connector's required scopes with any user-supplied
// additional scopes, deduplicating.
func mergeScopes(required, additional []string) []string {
	seen := make(map[string]bool, len(required)+len(additional))
	out := make([]string, 0, len(required)+len(additional))
	for _, s := range required {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range additional {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
