package connectors

import (
	"context"

	"awaken/src/mcpclient"
)

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
		AuthType:         AuthAPIKey,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: githubConfigFactory,
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
		AuthType:         AuthNone,
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: context7ConfigFactory,
	}
}

// puppeteerConfigFactory generates MCP server configs for the Puppeteer connector.
func puppeteerConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"puppeteer": {
			Name:  "puppeteer",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-puppeteer"},
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

// chromeDevToolsConfigFactory generates MCP server configs for the Chrome DevTools MCP connector.
func chromeDevToolsConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"chrome-devtools-mcp": {
			Name:  "chrome-devtools-mcp",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "npx",
					Args:    []string{"-y", "chrome-devtools-mcp@latest"},
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

// playwrightConfigFactory generates MCP server configs for the Playwright connector.
func playwrightConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"playwright": {
			Name:  "playwright",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "npx",
					Args:    []string{"-y", "@executeautomation/playwright-mcp-server"},
					Env:     map[string]string{},
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

// githubConfigFactory generates MCP server configs for the GitHub connector.
func githubConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"github": {
			Name:  "github",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-github"},
					Env: map[string]string{
						"GITHUB_PERSONAL_ACCESS_TOKEN": "",
					},
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

// context7ConfigFactory generates MCP server configs for the Context7 connector.
func context7ConfigFactory(def ConnectorDefinition, _ []string) map[string]mcpclient.ScopedMcpServerConfig {
	return map[string]mcpclient.ScopedMcpServerConfig{
		"context7": {
			Name:  "context7",
			Scope: mcpclient.ScopeUser,
			Config: mcpclient.McpServerConfig{
				Stdio: &mcpclient.StdioTransportConfig{
					Command: "npx",
					Args:    []string{"-y", "@upstash/context7-mcp@latest"},
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
