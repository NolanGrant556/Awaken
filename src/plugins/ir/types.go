// Package ir defines the vendor-neutral Plugin intermediate representation.
// It deliberately has no dependency on the Plugin Store, server, or runtimes.
package ir

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type Dialect string

const (
	DialectAgentPlugins Dialect = "agent_plugins"
	DialectCodex        Dialect = "codex"
	DialectClaudeCode   Dialect = "claude_code"
	DialectCursor       Dialect = "cursor"
)

func (d Dialect) Valid() bool {
	switch d {
	case DialectAgentPlugins, DialectCodex, DialectClaudeCode, DialectCursor:
		return true
	default:
		return false
	}
}

type ComponentKind string

const (
	KindSkill     ComponentKind = "skill"
	KindMCPServer ComponentKind = "mcp_server"
	KindRule      ComponentKind = "rule"
	KindAgent     ComponentKind = "agent"
	KindCommand   ComponentKind = "command"
	KindHook      ComponentKind = "hook"
	KindVariable  ComponentKind = "variable"
	KindApp       ComponentKind = "app"
	KindUnknown   ComponentKind = "unknown"
)

type MappingState string

const (
	MappingNative                MappingState = "native"
	MappingAdapted               MappingState = "adapted"
	MappingRecognizedUnsupported MappingState = "recognized_unsupported"
	MappingIgnoredByDialect      MappingState = "ignored_by_dialect"
	MappingInvalid               MappingState = "invalid"
)

type Diagnostic struct {
	Code          string        `json:"code"`
	Message       string        `json:"message"`
	Severity      string        `json:"severity"`
	Phase         string        `json:"phase,omitempty"`
	ComponentKind ComponentKind `json:"componentKind,omitempty"`
	ComponentID   string        `json:"componentId,omitempty"`
	SourcePath    string        `json:"sourcePath,omitempty"`
	SourceDialect Dialect       `json:"sourceDialect,omitempty"`
	Remediation   string        `json:"remediation,omitempty"`
	RawField      string        `json:"rawField,omitempty"`
}

type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

type Identity struct {
	Name         string          `json:"name"`
	DisplayName  string          `json:"displayName,omitempty"`
	Version      string          `json:"version,omitempty"`
	Description  string          `json:"description,omitempty"`
	Author       *Author         `json:"author,omitempty"`
	Homepage     string          `json:"homepage,omitempty"`
	Repository   string          `json:"repository,omitempty"`
	License      string          `json:"license,omitempty"`
	Keywords     []string        `json:"keywords,omitempty"`
	Category     string          `json:"category,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Logo         string          `json:"logo,omitempty"`
	InterfaceRaw json.RawMessage `json:"interfaceRaw,omitempty"`
}

type PackageLayout struct {
	Dialect      Dialect  `json:"dialect"`
	Surface      string   `json:"surface"`
	ManifestPath string   `json:"manifestPath"`
	Manifestless bool     `json:"manifestless,omitempty"`
	Secondary    []string `json:"secondaryReferences,omitempty"`
}

type SkillSpec struct {
	Path string `json:"path"`
}

type MCPSpec struct {
	ShortName              string            `json:"shortName"`
	SourceShape            string            `json:"sourceShape,omitempty"`
	SourceTransport        string            `json:"sourceTransport,omitempty"`
	Transport              string            `json:"transport,omitempty"`
	Command                string            `json:"command,omitempty"`
	Args                   []string          `json:"args,omitempty"`
	Env                    map[string]string `json:"env,omitempty"`
	Cwd                    string            `json:"cwd,omitempty"`
	URL                    string            `json:"url,omitempty"`
	Headers                map[string]string `json:"headers,omitempty"`
	OAuthResource          string            `json:"oauthResource,omitempty"`
	OAuthClientID          string            `json:"oauthClientId,omitempty"`
	OAuthClientMetadataURL string            `json:"oauthClientMetadataUrl,omitempty"`
	OAuthScopes            []string          `json:"oauthScopes,omitempty"`
	Raw                    json.RawMessage   `json:"raw,omitempty"`
}

type RuleSpec struct {
	Description string         `json:"description,omitempty"`
	AlwaysApply *bool          `json:"alwaysApply,omitempty"`
	Globs       []string       `json:"globs,omitempty"`
	Activation  string         `json:"activation,omitempty"`
	Scope       string         `json:"scope,omitempty"`
	Body        string         `json:"-"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
}

type AgentSpec struct {
	Name            string         `json:"name,omitempty"`
	Description     string         `json:"description,omitempty"`
	Prompt          string         `json:"-"`
	Model           string         `json:"model,omitempty"`
	Effort          string         `json:"effort,omitempty"`
	Tools           []string       `json:"tools,omitempty"`
	DisallowedTools []string       `json:"disallowedTools,omitempty"`
	Skills          []string       `json:"skills,omitempty"`
	MCPServers      []string       `json:"mcpServers,omitempty"`
	MaxTurns        int            `json:"maxTurns,omitempty"`
	TimeoutSeconds  int            `json:"timeoutSeconds,omitempty"`
	Background      bool           `json:"background,omitempty"`
	IsolationHint   string         `json:"isolationHint,omitempty"`
	ReadonlyHint    bool           `json:"readonlyHint,omitempty"`
	RawFrontmatter  map[string]any `json:"rawFrontmatter,omitempty"`
}

type CommandSpec struct {
	InvocationName string         `json:"invocationName,omitempty"`
	ArgumentHint   string         `json:"argumentHint,omitempty"`
	PromptTemplate string         `json:"-"`
	Model          string         `json:"model,omitempty"`
	AllowedTools   []string       `json:"allowedTools,omitempty"`
	ManualOnly     bool           `json:"manualOnly,omitempty"`
	RawFrontmatter map[string]any `json:"rawFrontmatter,omitempty"`
}

type HookSpec struct {
	SourceEvent      string          `json:"sourceEvent"`
	MappedEvent      string          `json:"mappedEvent,omitempty"`
	Matcher          string          `json:"matcher,omitempty"`
	HandlerType      string          `json:"handlerType"`
	Scope            string          `json:"scope,omitempty"`
	OwnerComponentID string          `json:"ownerComponentId,omitempty"`
	Command          string          `json:"command,omitempty"`
	URL              string          `json:"url,omitempty"`
	Prompt           string          `json:"prompt,omitempty"`
	MCPServer        string          `json:"mcpServer,omitempty"`
	MCPTool          string          `json:"mcpTool,omitempty"`
	TimeoutSeconds   int             `json:"timeoutSeconds,omitempty"`
	Condition        string          `json:"condition,omitempty"`
	Async            bool            `json:"async,omitempty"`
	AsyncRewake      bool            `json:"asyncRewake,omitempty"`
	RewakeMessage    string          `json:"rewakeMessage,omitempty"`
	RewakeSummary    string          `json:"rewakeSummary,omitempty"`
	LoopLimit        *int            `json:"loopLimit,omitempty"`
	Raw              json.RawMessage `json:"raw,omitempty"`
}

type VariableSpec struct {
	Key         string          `json:"key"`
	Type        string          `json:"type"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Default     any             `json:"default,omitempty"`
	Enum        []any           `json:"enum,omitempty"`
	Const       any             `json:"const,omitempty"`
	Secret      bool            `json:"secret,omitempty"`
	Multiple    bool            `json:"multiple,omitempty"`
	Minimum     *float64        `json:"minimum,omitempty"`
	Maximum     *float64        `json:"maximum,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

type AppSpec struct {
	ConnectorID string          `json:"connectorId,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

type SetupKind string

const (
	SetupConfiguration SetupKind = "configuration"
	SetupSecret        SetupKind = "secret"
	SetupOAuthPKCE     SetupKind = "oauth_pkce"
	SetupOAuthDevice   SetupKind = "oauth_device"
	SetupMCPOAuth      SetupKind = "mcp_oauth"
	SetupCLILogin      SetupKind = "cli_login"
	SetupExternal      SetupKind = "external_setup"
	SetupHostConnector SetupKind = "host_connector"
	SetupAppConnector  SetupKind = "app_connector"
)

func (k SetupKind) Valid() bool {
	switch k {
	case SetupConfiguration, SetupSecret, SetupOAuthPKCE, SetupOAuthDevice, SetupMCPOAuth, SetupCLILogin, SetupExternal, SetupHostConnector, SetupAppConnector:
		return true
	default:
		return false
	}
}

type SetupBindingTarget string

const (
	SetupBindingMCPTransport SetupBindingTarget = "mcp_transport"
	SetupBindingHTTPHeader   SetupBindingTarget = "http_header"
	SetupBindingProcessEnv   SetupBindingTarget = "process_env"
)

type SetupBinding struct {
	ComponentID string             `json:"componentId"`
	Target      SetupBindingTarget `json:"target"`
	Name        string             `json:"name,omitempty"`
	Scheme      string             `json:"scheme,omitempty"`
}

type OAuthSetupSpec struct {
	Issuer                      string   `json:"issuer,omitempty"`
	AuthorizationEndpoint       string   `json:"authorizationEndpoint,omitempty"`
	TokenEndpoint               string   `json:"tokenEndpoint,omitempty"`
	DeviceAuthorizationEndpoint string   `json:"deviceAuthorizationEndpoint,omitempty"`
	ClientID                    string   `json:"clientId,omitempty"`
	ClientMetadataURL           string   `json:"clientMetadataUrl,omitempty"`
	ClientIDVariable            string   `json:"clientIdVariable,omitempty"`
	ClientSecretVariable        string   `json:"clientSecretVariable,omitempty"`
	Scopes                      []string `json:"scopes,omitempty"`
	Resource                    string   `json:"resource,omitempty"`
	Audience                    string   `json:"audience,omitempty"`
}

// SetupRequirement is the source-neutral description of one user action that
// must be completed before one or more plugin components are ready. It carries
// declarations only; credentials and mutable authorization state live in the
// host-owned auth stores.
type SetupRequirement struct {
	ID            string          `json:"id"`
	Fingerprint   string          `json:"fingerprint"`
	Kind          SetupKind       `json:"kind"`
	Title         string          `json:"title,omitempty"`
	Description   string          `json:"description,omitempty"`
	Required      bool            `json:"required,omitempty"`
	ComponentIDs  []string        `json:"componentIds,omitempty"`
	MCPServerName string          `json:"mcpServerName,omitempty"`
	Field         *VariableSpec   `json:"field,omitempty"`
	OAuth         *OAuthSetupSpec `json:"oauth,omitempty"`
	Bindings      []SetupBinding  `json:"bindings,omitempty"`
	ProviderID    string          `json:"providerId,omitempty"`
	ExternalURL   string          `json:"externalUrl,omitempty"`
	ConnectorID   string          `json:"connectorId,omitempty"`
	SourceDialect Dialect         `json:"sourceDialect"`
	SourcePath    string          `json:"sourcePath,omitempty"`
	Raw           json.RawMessage `json:"raw,omitempty"`
}

type Component struct {
	LogicalID           string        `json:"logicalId"`
	LogicalIDEncoded    string        `json:"logicalIdEncoded"`
	RevisionID          string        `json:"revisionId,omitempty"`
	Kind                ComponentKind `json:"kind"`
	DeclaredName        string        `json:"declaredName,omitempty"`
	DerivedName         string        `json:"derivedName,omitempty"`
	DisplayName         string        `json:"displayName,omitempty"`
	Description         string        `json:"description,omitempty"`
	SourceDialect       Dialect       `json:"sourceDialect"`
	SourceManifestPath  string        `json:"sourceManifestPath,omitempty"`
	SourcePath          string        `json:"sourcePath,omitempty"`
	SourceDigest        string        `json:"sourceDigest,omitempty"`
	Mapping             MappingState  `json:"mappingState"`
	Validation          string        `json:"validation"`
	CanonicalProvenance string        `json:"canonicalProvenance,omitempty"`
	SecondaryProvenance []string      `json:"secondaryProvenance,omitempty"`
	Diagnostics         []Diagnostic  `json:"diagnostics,omitempty"`
	Skill               *SkillSpec    `json:"skill,omitempty"`
	MCP                 *MCPSpec      `json:"mcp,omitempty"`
	Rule                *RuleSpec     `json:"rule,omitempty"`
	Agent               *AgentSpec    `json:"agent,omitempty"`
	Command             *CommandSpec  `json:"command,omitempty"`
	Hook                *HookSpec     `json:"hook,omitempty"`
	Variable            *VariableSpec `json:"variable,omitempty"`
	App                 *AppSpec      `json:"app,omitempty"`
}

type NormalizedPlugin struct {
	Identity    Identity           `json:"identity"`
	Layout      PackageLayout      `json:"layout"`
	RawManifest json.RawMessage    `json:"rawManifest,omitempty"`
	Components  []Component        `json:"components"`
	Setup       []SetupRequirement `json:"setup,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}

type ManifestCandidate struct {
	Dialect      Dialect `json:"dialect"`
	ManifestPath string  `json:"manifestPath"`
	Manifestless bool    `json:"manifestless,omitempty"`
	IdentityHint string  `json:"identityHint,omitempty"`
	VersionHint  string  `json:"versionHint,omitempty"`
	Valid        bool    `json:"valid"`
	Reason       string  `json:"reason,omitempty"`
}

func LogicalID(kind ComponentKind, sourcePath, stableKey string) string {
	sourcePath = strings.TrimPrefix(strings.ReplaceAll(sourcePath, "\\", "/"), "./")
	var id string
	if sourcePath == "" {
		id = string(kind) + "/"
	} else {
		id = string(kind) + "/" + sourcePath
	}
	if stableKey != "" {
		id += "#" + stableKey
	}
	return id
}

func EncodeLogicalID(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

func DecodeLogicalID(encoded string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode logical id: %w", err)
	}
	if len(raw) == 0 || !strings.Contains(string(raw), "/") {
		return "", fmt.Errorf("invalid logical id")
	}
	return string(raw), nil
}

func SourceDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func FinalizeComponent(component *Component, pluginDigest string) {
	if component == nil {
		return
	}
	component.LogicalIDEncoded = EncodeLogicalID(component.LogicalID)
	if component.Validation == "" {
		component.Validation = "valid"
	}
	if component.SourceDigest != "" && pluginDigest != "" {
		pluginPart := strings.TrimPrefix(pluginDigest, "sha256-v2:")
		pluginPart = strings.TrimPrefix(pluginPart, "sha256:")
		if len(pluginPart) > 12 {
			pluginPart = pluginPart[:12]
		}
		sourcePart := component.SourceDigest
		if len(sourcePart) > 12 {
			sourcePart = sourcePart[:12]
		}
		component.RevisionID = component.LogicalID + "@" + pluginPart + "+" + sourcePart
	}
}
