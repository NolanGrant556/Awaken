package connectors

import (
	"context"

	"awaken/src/mcpclient"
)

// ConnectorState represents the lifecycle state of a connector.
type ConnectorState string

const (
	StateDisconnected ConnectorState = "disconnected"
	StatePending      ConnectorState = "pending"
	StateConnected    ConnectorState = "connected"
	StateNeedsAuth    ConnectorState = "needs-auth"
	StateFailed       ConnectorState = "failed"
)

// AuthType enumerates the authentication methods a connector supports.
type AuthType string

const (
	AuthOAuth2 AuthType = "oauth2"
	AuthAPIKey AuthType = "api_key"
	AuthNone   AuthType = "none"
)

// TrustLevel indicates the trust classification of a connector source.
type TrustLevel string

const (
	TrustBuiltin    TrustLevel = "builtin"
	TrustTeam       TrustLevel = "team"
	TrustUserLocal  TrustLevel = "user_local"
	TrustUserManual TrustLevel = "user_manual"
)

// Category groups connectors for UI organization.
type Category string

const (
	CategoryDocs  Category = "docs"
	CategoryChat  Category = "chat"
	CategoryCode  Category = "code"
	CategoryData  Category = "data"
	CategoryOther Category = "other"
)

// SourceOrigin tags where a connector definition came from.
type SourceOrigin string

const (
	SourceBuiltin SourceOrigin = "builtin"
	SourceLocal   SourceOrigin = "local"
	SourceTeam    SourceOrigin = "team"
)

// McpConfigFactory is a function that produces one or more MCP server configs
// from a connector definition and optional OAuth scopes. The returned map keys
// are server keys (e.g. "notion", "notion_search") and the values are
// ScopedMcpServerConfig entries to inject into the MCP manager.
type McpConfigFactory func(def ConnectorDefinition, scopes []string) map[string]mcpclient.ScopedMcpServerConfig

// ConnectorDefinition is the static metadata for a single connector.
type ConnectorDefinition struct {
	// ID is the unique identifier (e.g. "notion", "slack").
	ID string `json:"id"`

	// DisplayName is the human-readable label shown in the UI.
	DisplayName string `json:"displayName"`

	// Description is a short textual description of what this connector provides.
	Description string `json:"description,omitempty"`

	// Category groups connectors for UI browsing.
	Category Category `json:"category"`

	// Provider is the service provider name (e.g. "Notion", "Slack").
	Provider string `json:"provider"`

	// AuthType describes how authentication is performed.
	AuthType AuthType `json:"authType"`

	// RequiredScopes lists the OAuth scopes needed for this connector to function.
	RequiredScopes []string `json:"requiredScopes,omitempty"`

	// TrustLevel indicates the trust classification of the source.
	TrustLevel TrustLevel `json:"trustLevel"`

	// SourceOrigin records which source provided this definition.
	SourceOrigin SourceOrigin `json:"sourceOrigin"`

	// McpConfigFactory is the function that generates MCP server configs from
	// this connector definition. Only set for builtin connectors; local and team
	// connectors carry their config inline in MCPRawConfig.
	McpConfigFactory McpConfigFactory `json:"-"`

	// MCPRawConfig carries an inline MCP server config for local connectors.
	// When non-nil, this config is used directly instead of the factory.
	MCPRawConfig *mcpclient.McpServerConfig `json:"mcpRawConfig,omitempty"`
}

// ConnectorStatus is the runtime-derived state of a connector, combining the
// connector's own authorization state with its MCP servers' connection states.
type ConnectorStatus struct {
	// ID matches the connector definition's ID.
	ID string `json:"id"`

	// DisplayName matches the connector definition's DisplayName.
	DisplayName string `json:"displayName"`

	// Category matches the connector definition's Category.
	Category Category `json:"category"`

	// Provider matches the connector definition's Provider.
	Provider string `json:"provider,omitempty"`

	// State is the derived connector state (disconnected/connected/needs-auth/failed).
	State ConnectorState `json:"state"`

	// SourceOrigin records which source provided this connector.
	SourceOrigin SourceOrigin `json:"sourceOrigin"`

	// AuthType describes how authentication is performed.
	AuthType AuthType `json:"authType"`

	// LastError contains the most recent error message, if any.
	LastError string `json:"lastError,omitempty"`

	// ServerNames lists the MCP server names injected by this connector.
	ServerNames []string `json:"serverNames,omitempty"`

	// ShadowedBy indicates that another connector with higher priority covers
	// the same server. Empty string means this connector is active.
	ShadowedBy string `json:"shadowedBy,omitempty"`
}

// ConnectorSource is the interface all connector sources must implement.
type ConnectorSource interface {
	// List returns all connector definitions available from this source.
	List(ctx context.Context) ([]ConnectorDefinition, error)

	// Origin returns the source origin identifier for this source.
	Origin() SourceOrigin

	// Priority returns the merge priority for this source (lower = higher priority).
	Priority() int
}

// Logger is the minimal logging surface the connectors package consumes.
type Logger interface {
	Debugf(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// noopLogger discards all output.
type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}