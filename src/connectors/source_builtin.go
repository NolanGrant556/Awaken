package connectors

import (
	"context"
	"runtime"

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

// NewBuiltinSource returns a ConnectorSource populated with all first-party
// connector definitions. Add new builtins here.
func NewBuiltinSource() ConnectorSource {
	return newBuiltinSource()
}

// newBuiltinSource returns a ConnectorSource populated with all first-party
// connector definitions. Add new builtins here.
func newBuiltinSource() *builtinSource {
	return &builtinSource{
		defs: []ConnectorDefinition{
			puppeteerConnector(),
			chromeDevToolsConnector(),
			playwrightConnector(),
			githubConnector(),
			dingTalkConnector(),
			larkConnector(),
			context7Connector(),
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
		DisplayName:  "钉钉CLI",
		Description:  "通过 DingTalk-Real-AI/dingtalk-workspace-cli 操作钉钉消息、日程、通讯录等官方连接器能力。",
		Category:     CategoryChat,
		Provider:     "DingTalk",
		ProviderID:   tool.ProviderID("connector:dingtalk-workspace"),
		Capabilities: []tool.Capability{tool.CapabilityMessageSend},
		Dependencies: []tool.DependencyRequirement{{
			Kind: tool.DependencyOSExecutable, Name: "dingtalk-workspace-cli", ProbeID: "executable.dingtalk-workspace-cli", Source: "connector",
		}},
		AuthType:         AuthOAuth2,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: dingTalkConfigFactory,
	}
}

func larkConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:           "lark-cli",
		DisplayName:  "飞书CLI",
		Description:  "通过 larksuite/cli 操作飞书消息、组织与协作能力。",
		Category:     CategoryChat,
		Provider:     "Lark",
		ProviderID:   tool.ProviderID("connector:lark-cli"),
		Capabilities: []tool.Capability{tool.CapabilityMessageSend},
		Dependencies: []tool.DependencyRequirement{{
			Kind: tool.DependencyOSExecutable, Name: "lark-cli", ProbeID: "executable.lark-cli", Source: "connector",
		}},
		AuthType:         AuthOAuth2,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: larkConfigFactory,
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

func npxDependency(source string) tool.DependencyRequirement {
	return tool.DependencyRequirement{
		Kind:    tool.DependencyPackageManager,
		Name:    "npx",
		ProbeID: "executable.npx",
		Source:  source,
	}
}

// puppeteerConfigFactory generates MCP server configs for the Puppeteer connector.
func puppeteerConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
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
	}
}

// chromeDevToolsConfigFactory generates MCP server configs for the Chrome DevTools MCP connector.
func chromeDevToolsConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
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
	}
}

// playwrightConfigFactory generates MCP server configs for the Playwright connector.
func playwrightConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	cmd, args := npxCommand("-y", "@executeautomation/playwright-mcp-server")
	return map[string]mcpclient.ScopedMcpServerConfig{
		"playwright": {
			Name:  "playwright",
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
	}
}

// githubConfigFactory generates MCP server configs for the GitHub connector.
func githubConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
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
	}
}

func dingTalkConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"dingtalk-workspace": {
			Name:  "dingtalk-workspace",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "dingtalk-workspace-cli",
					Args:    []string{"mcp"},
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

func larkConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"lark": {
			Name:  "lark",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "lark-cli",
					Args:    []string{"mcp"},
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

// context7ConfigFactory generates MCP server configs for the Context7 connector.
func context7ConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
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
	}
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
