package connectors

import (
	"context"
	"time"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

// ConnectorState represents the lifecycle state of a connector.
type ConnectorState string

const (
	StateDisconnected     ConnectorState = "disconnected"
	StatePending          ConnectorState = "pending"
	StateConnected        ConnectorState = "connected"
	StateNeedsAuth        ConnectorState = "needs-auth"
	StateApprovalRequired ConnectorState = "approval-required"
	StateFailed           ConnectorState = "failed"
)

// ConnectorKind separates ordinary MCP-backed connectors from Desktop-owned
// CLI lifecycle providers. CLI connectors never create MCP server configs.
type ConnectorKind string

const (
	KindMCP ConnectorKind = "mcp"
	KindCLI ConnectorKind = "cli"
)

// AuthType enumerates the authentication methods a connector supports.
type AuthType string

const (
	AuthOAuth2 AuthType = "oauth2"
	AuthAPIKey AuthType = "api_key"
	AuthNone   AuthType = "none"
)

// OAuthConnectorSpec is the host-owned authorization contract for a built-in
// connector. Public client identifiers are configuration, not secrets; access
// and refresh tokens never live in the definition or cross the Core boundary.
type OAuthConnectorSpec struct {
	AuthorizationEndpoint string            `json:"authorizationEndpoint"`
	TokenEndpoint         string            `json:"tokenEndpoint"`
	ClientID              string            `json:"-"`
	Scopes                []string          `json:"scopes,omitempty"`
	Resource              string            `json:"resource,omitempty"`
	AuthorizationParams   map[string]string `json:"-"`
}

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

// CatalogPlacement declares where a connector is presented to users. The
// connector runtime always remains registered; plugin-only host connectors are
// operated through the referencing plugin rather than duplicated in the
// top-level Connector catalog.
type CatalogPlacement string

const (
	CatalogConnectors CatalogPlacement = "connectors"
	CatalogPluginOnly CatalogPlacement = "plugin_only"
)

// McpConfigFactory is a function that produces one or more MCP server configs
// from a connector definition and optional OAuth scopes. The returned map keys
// are server keys (e.g. "notion", "notion_search") and the values are
// ScopedMcpServerConfig entries to inject into the MCP manager. Factories may
// prepare connector-owned runtime assets, so preparation failures are returned
// instead of being deferred to an opaque child-process startup error.
type McpConfigFactory func(def ConnectorDefinition, scopes []string) (map[string]mcpclient.ScopedMcpServerConfig, error)

// ConnectorDefinition is the static metadata for a single connector.
type ConnectorDefinition struct {
	// ID is the unique identifier (e.g. "notion", "slack").
	ID string `json:"id"`

	// Kind determines which lifecycle owner handles the connector.
	Kind ConnectorKind `json:"kind"`

	// DisplayName is the human-readable label shown in the UI.
	DisplayName string `json:"displayName"`

	// Description is a short textual description of what this connector provides.
	Description string `json:"description,omitempty"`

	// Category groups connectors for UI browsing.
	Category Category `json:"category"`

	// Provider is the service provider name (e.g. "Notion", "Slack").
	Provider string `json:"provider"`

	// ProviderID is the stable implementation identity propagated into every
	// MCP tool registered from this connector. Capabilities are connector-level
	// discovery facts; per-tool message/delete semantics must still be declared
	// by MCP tool metadata.
	ProviderID   tool.ProviderID              `json:"providerId"`
	Capabilities []tool.Capability            `json:"capabilities,omitempty"`
	Dependencies []tool.DependencyRequirement `json:"dependencies,omitempty"`

	// AuthType describes how authentication is performed.
	AuthType AuthType `json:"authType"`

	// RequiredScopes lists the OAuth scopes needed for this connector to function.
	RequiredScopes []string `json:"requiredScopes,omitempty"`

	// ServerNames declares stable MCP runtime identities for host-provided
	// servers. It lets plugin inventory resolve a host_connector reference even
	// before the account is connected; live readiness still comes from Manager.
	ServerNames []string `json:"serverNames,omitempty"`

	// OAuth is present only for host-owned OAuth connectors. It is consumed by
	// Core's connector account service and is never returned to Renderer.
	OAuth *OAuthConnectorSpec `json:"-"`

	// TrustLevel indicates the trust classification of the source.
	TrustLevel TrustLevel `json:"trustLevel"`

	// SourceOrigin records which source provided this definition.
	SourceOrigin SourceOrigin `json:"sourceOrigin"`

	// CatalogPlacement controls the user-facing catalog that owns this
	// connector's setup entry. Empty values normalize to CatalogConnectors.
	CatalogPlacement CatalogPlacement `json:"catalogPlacement,omitempty"`

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

	Kind ConnectorKind `json:"kind"`

	// DisplayName matches the connector definition's DisplayName.
	DisplayName string `json:"displayName"`

	// Description matches the connector definition's Description.
	Description string `json:"description,omitempty"`

	// Category matches the connector definition's Category.
	Category Category `json:"category"`

	// Provider matches the connector definition's Provider.
	Provider string `json:"provider,omitempty"`

	ProviderID        tool.ProviderID              `json:"providerId"`
	Capabilities      []tool.Capability            `json:"capabilities,omitempty"`
	Dependencies      []tool.DependencyRequirement `json:"dependencies,omitempty"`
	ProviderReadiness tool.ProviderReadiness       `json:"providerReadiness"`

	// State is the derived connector state (disconnected/connected/needs-auth/failed).
	State ConnectorState `json:"state"`

	// SourceOrigin records which source provided this connector.
	SourceOrigin SourceOrigin `json:"sourceOrigin"`

	// CatalogPlacement is copied from the definition so API consumers can
	// enforce the same no-duplicate presentation boundary.
	CatalogPlacement CatalogPlacement `json:"catalogPlacement"`

	// AuthType describes how authentication is performed.
	AuthType AuthType `json:"authType"`

	// LastError contains the most recent error message, if any.
	LastError string `json:"lastError,omitempty"`

	// ServerNames lists the MCP server names injected by this connector.
	ServerNames []string `json:"serverNames,omitempty"`

	// ShadowedBy indicates that another connector with higher priority covers
	// the same server. Empty string means this connector is active.
	ShadowedBy string `json:"shadowedBy,omitempty"`

	// CLIReadiness is present only for Desktop-owned CLI connectors.
	CLIReadiness *CLIConnectorReadiness `json:"cliReadiness,omitempty"`

	// LegacyMCPReviewRequired is true when a same-name user MCP record did not
	// exactly match the retired built-in factory and was therefore preserved.
	LegacyMCPReviewRequired bool `json:"legacyMcpReviewRequired,omitempty"`

	// Account is a deliberately small, non-sensitive connected-account view.
	Account *ConnectorAccountProfile `json:"account,omitempty"`
}

type ConnectorAccountProfile struct {
	DisplayName string `json:"displayName,omitempty"`
	TenantName  string `json:"tenantName,omitempty"`
}

// AccountReadiness is supplied by the Core-owned connector account service.
// Registry uses it to keep OAuth readiness separate from MCP transport state.
type AccountReadiness struct {
	State     ConnectorState
	LastError string
	Account   *ConnectorAccountProfile
}

type AccountReadinessProvider interface {
	AccountReadiness(definition ConnectorDefinition) AccountReadiness
}

type ConnectorAuthStartResult struct {
	SessionID        string     `json:"sessionId,omitempty"`
	ConnectorID      string     `json:"connectorId"`
	Status           string     `json:"status"`
	AuthorizationURL string     `json:"authorizationUrl,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}

type ConnectorAuthSession struct {
	SessionID    string     `json:"sessionId"`
	ConnectorID  string     `json:"connectorId"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
}

type CLIActivation string
type CLIInstallState string
type CLIExecutableState string
type CLIConfigurationState string
type CLIAuthenticationState string
type CLIOverallState string

const (
	CLIActivationEnabled  CLIActivation = "enabled"
	CLIActivationDisabled CLIActivation = "disabled"

	CLIInstallMissing     CLIInstallState = "missing"
	CLIInstallInstalling  CLIInstallState = "installing"
	CLIInstallInstalled   CLIInstallState = "installed"
	CLIInstallUnsupported CLIInstallState = "unsupported"
	CLIInstallFailed      CLIInstallState = "failed"

	CLIExecutableUnknown   CLIExecutableState = "unknown"
	CLIExecutableMissing   CLIExecutableState = "missing"
	CLIExecutableAvailable CLIExecutableState = "available"
	CLIExecutableInvalid   CLIExecutableState = "invalid"

	CLIConfigurationUnknown  CLIConfigurationState = "unknown"
	CLIConfigurationRequired CLIConfigurationState = "required"
	CLIConfigurationReady    CLIConfigurationState = "ready"
	CLIConfigurationFailed   CLIConfigurationState = "failed"

	CLIAuthenticationUnknown    CLIAuthenticationState = "unknown"
	CLIAuthenticationRequired   CLIAuthenticationState = "required"
	CLIAuthenticationInProgress CLIAuthenticationState = "in_progress"
	CLIAuthenticationAuthorized CLIAuthenticationState = "authorized"
	CLIAuthenticationFailed     CLIAuthenticationState = "failed"

	CLIOverallDisabled       CLIOverallState = "disabled"
	CLIOverallNotReady       CLIOverallState = "not_ready"
	CLIOverallActionRequired CLIOverallState = "action_required"
	CLIOverallWorking        CLIOverallState = "working"
	CLIOverallReady          CLIOverallState = "ready"
	CLIOverallFailed         CLIOverallState = "failed"
)

type CLISafeError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Recovery string `json:"recovery,omitempty"`
}

// CLIAccountProfile is a non-sensitive connected-account projection for UI.
// It must never include phone numbers, emails, tokens, or raw CLI output.
type CLIAccountProfile struct {
	DisplayName  string `json:"displayName,omitempty"`
	Organization string `json:"organization,omitempty"`
}

// CLIConnectorReadiness is a non-sensitive Desktop-to-Core snapshot. It must
// never contain tokens, device codes, authorization URLs, or raw CLI output.
type CLIConnectorReadiness struct {
	ProviderID     string                 `json:"providerId"`
	Activation     CLIActivation          `json:"activation"`
	Install        CLIInstallState        `json:"install"`
	Executable     CLIExecutableState     `json:"executable"`
	Configuration  CLIConfigurationState  `json:"configuration"`
	Authentication CLIAuthenticationState `json:"authentication"`
	Overall        CLIOverallState        `json:"overall"`
	Version        string                 `json:"version,omitempty"`
	ExecutablePath string                 `json:"executablePath,omitempty"`
	Ownership      string                 `json:"ownership,omitempty"`
	InstallMethod  string                 `json:"installMethod,omitempty"`
	Account        *CLIAccountProfile     `json:"account,omitempty"`
	SafeError      *CLISafeError          `json:"safeError,omitempty"`
	UpdatedAt      string                 `json:"updatedAt"`
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
