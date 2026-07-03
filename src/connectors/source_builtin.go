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
			notionConnector(),
			slackConnector(),
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

// notionConnector returns the first-party Notion connector definition.
func notionConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:          "notion",
		DisplayName: "Notion",
		Description: "Connect your Notion workspace to search and edit pages, databases, and blocks.",
		Category:    CategoryDocs,
		Provider:    "Notion",
		AuthType:    AuthOAuth2,
		RequiredScopes: []string{
			"read:content",
			"write:content",
			"read:user",
		},
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: notionConfigFactory,
	}
}

// slackConnector returns the first-party Slack connector definition.
func slackConnector() ConnectorDefinition {
	return ConnectorDefinition{
		ID:          "slack",
		DisplayName: "Slack",
		Description: "Connect your Slack workspace to read and send messages, search channels, and manage conversations.",
		Category:    CategoryChat,
		Provider:    "Slack",
		AuthType:    AuthOAuth2,
		RequiredScopes: []string{
			"channels:read",
			"channels:history",
			"chat:write",
			"users:read",
		},
		TrustLevel:       TrustBuiltin,
		SourceOrigin:     SourceBuiltin,
		McpConfigFactory: slackConfigFactory,
	}
}

// notionConfigFactory generates MCP server configs for the Notion connector.
func notionConfigFactory(def ConnectorDefinition, scopes []string) map[string]mcpclient.ScopedMcpServerConfig {
	mergedScopes := mergeScopes(def.RequiredScopes, scopes)
	_ = mergedScopes // scopes are passed through the OAuth flow; config factory does not embed them.

	return map[string]mcpclient.ScopedMcpServerConfig{
		"notion": {
			Name:  "notion",
			Scope: mcpclient.ScopeDynamic,
			Config: mcpclient.McpServerConfig{
				HTTP: &mcpclient.HTTPTransportConfig{
					URL: "https://api.notion.com/v1/mcp",
				},
				DisplayName: def.DisplayName,
			},
		},
	}
}

// slackConfigFactory generates MCP server configs for the Slack connector.
func slackConfigFactory(def ConnectorDefinition, scopes []string) map[string]mcpclient.ScopedMcpServerConfig {
	mergedScopes := mergeScopes(def.RequiredScopes, scopes)
	_ = mergedScopes

	return map[string]mcpclient.ScopedMcpServerConfig{
		"slack": {
			Name:  "slack",
			Scope: mcpclient.ScopeDynamic,
			Config: mcpclient.McpServerConfig{
				HTTP: &mcpclient.HTTPTransportConfig{
					URL: "https://slack.com/api/mcp",
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