// Package plugins owns Agent Plugins 1.0.0 package loading, install
// lifecycle, and the source roots / MCP configs handed to the existing
// Skill and MCP runtimes.
package plugins

import (
	"errors"
	"time"

	"awaken/src/mcpclient"
	"awaken/src/plugins/ir"
)

const (
	SchemaPluginV1 = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	SchemaMCPV1    = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

	SchemaPrefix = "https://agent-plugins.org/schemas/"

	PluginIdentityPrefix = "plugin__"
	StateVersion         = 2

	MaxArchiveBytes         = 100 << 20
	MaxExtractBytes         = 500 << 20
	MaxPackageFiles         = 10000
	MaxComponentSourceBytes = 2 << 20
)

var (
	ErrUnsupportedSchema       = errors.New("plugin_unsupported_schema")
	ErrManifestInvalid         = errors.New("plugin_manifest_invalid")
	ErrMCPInvalid              = errors.New("plugin_mcp_invalid")
	ErrAlreadyExists           = errors.New("plugin already exists")
	ErrReplaceTrashUnavailable = errors.New("plugin replacement Trash unavailable")
	ErrNotFound                = errors.New("plugin not found")
	ErrStateCorrupt            = errors.New("plugins.json is corrupt")
	ErrHomeRequired            = errors.New("AWAKEN_HOME is required")
	ErrPathEscapes             = errors.New("path escapes the plugin root")
	ErrSourceChanged           = errors.New("plugin source changed after validation")
	ErrRecordedDigestChanged   = errors.New("plugin recorded digest changed")
	ErrDisabled                = errors.New("plugin is disabled")
	ErrDigestChanged           = errors.New("plugin digest changed")
	ErrNoUsableComponents      = errors.New("plugin has no usable Skill or MCP component")
	ErrRuntimeLeaseActive      = errors.New("plugin runtime generation is in use")
	ErrSkillPackageSelected    = errors.New("Skill package selected in Plugin installer")
	ErrBuiltinImmutable        = errors.New("built-in plugin is managed by Awaken")
)

type RuntimeLease struct {
	ID         string   `json:"id"`
	Generation string   `json:"generation"`
	Plugins    []string `json:"plugins,omitempty"`
}

// Diagnostic is the additive v2 component finding shared with dialect IR.
// The alias preserves the existing Agent Plugins API while allowing richer
// source and remediation metadata in new validation responses.
type Diagnostic = ir.Diagnostic

// Author is the optional plugin.json author object.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Manifest is the portable Agent Plugins 1.0.0 identity document.
type Manifest struct {
	Schema      string
	Name        string
	Version     string
	Description string
	Author      *Author
	Homepage    string
	Repository  string
	License     string
	Keywords    []string
}

// SourceKind identifies how a plugin was installed.
type SourceKind string

const (
	SourceLocalPath SourceKind = "local_path"
)

// Source is the durable install origin recorded in plugins.json.
type Source struct {
	Kind            SourceKind `json:"kind"`
	Path            string     `json:"path,omitempty"`
	DialectHint     ir.Dialect `json:"dialectHint,omitempty"`
	SelectedDialect ir.Dialect `json:"selectedDialect,omitempty"`
}

// MCPServerToggle is the per-server enable override stored in plugins.json.
type MCPServerToggle struct {
	Enabled bool `json:"enabled"`
}

// Record is one plugins.json entry.
type Record struct {
	Enabled            bool                         `json:"enabled"`
	Version            string                       `json:"version,omitempty"`
	Source             Source                       `json:"source"`
	InstalledAt        time.Time                    `json:"installedAt"`
	Digest             string                       `json:"digest"`
	MCPServers         map[string]MCPServerToggle   `json:"mcpServers,omitempty"`
	Dialect            ir.Dialect                   `json:"dialect"`
	ManifestPath       string                       `json:"manifestPath"`
	InstallationID     string                       `json:"installationId"`
	DisplayName        string                       `json:"displayName,omitempty"`
	Builtin            bool                         `json:"builtin,omitempty"`
	BrandIcon          string                       `json:"brandIcon,omitempty"`
	BuiltinOrder       int                          `json:"builtinOrder,omitempty"`
	Components         map[string]ComponentOverride `json:"components,omitempty"`
	Variables          map[string]VariableValueRef  `json:"variables,omitempty"`
	SecondaryManifests []string                     `json:"secondaryManifests,omitempty"`
	MigrationConflicts []string                     `json:"migrationConflicts,omitempty"`
}

type ComponentOverride struct {
	Desired            string          `json:"desired"`
	ActivationOverride string          `json:"activationOverride,omitempty"`
	ToolOverrides      map[string]bool `json:"toolOverrides,omitempty"`
	BaseRevisionID     string          `json:"baseRevisionId"`
}

type VariableValueRef struct {
	Value     any       `json:"value,omitempty"`
	SecretRef string    `json:"secretRef,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// StateFile is the on-disk plugins.json shape.
type StateFile struct {
	Version int               `json:"version"`
	Plugins map[string]Record `json:"plugins"`
}

// LoadedServer is one translated plugin MCP server.
type LoadedServer struct {
	ShortName string
	Identity  string
	Config    mcpclient.McpServerConfig
	Enabled   bool
	Invalid   *Diagnostic
}

// InstalledPlugin is the runtime view of one installed package.
type InstalledPlugin struct {
	Name                string
	Root                string
	DataDir             string
	Manifest            Manifest
	Enabled             bool
	Source              Source
	Digest              string
	ActualDigest        string
	IntegrityValid      bool
	InstalledAt         time.Time
	SkillRoot           string
	MCPServers          []LoadedServer
	Dialect             ir.Dialect
	ManifestPath        string
	InstallationID      string
	DisplayName         string
	Builtin             bool
	BrandIcon           string
	BuiltinOrder        int
	Components          []ir.Component
	Setup               []ir.SetupRequirement
	ComponentState      []ComponentState
	ConfigurationState  string
	AuthenticationState string
	CompatibilityState  string
	Diagnostics         []Diagnostic
}

// ComposerSelection is the host-resolved identity of an enabled Plugin chosen
// from a Composer. It is intentionally metadata-only: Skills and MCP keep
// their existing runtimes and the Plugin layer only proves which components
// belong to the selected package.
type ComposerSelection struct {
	Name        string
	Version     string
	Description string
	Digest      string
	SkillIDs    []string
	ServerIDs   []string
}

// InstallRequest is the user-facing install input.
type InstallRequest struct {
	Source         Source
	Replace        bool
	ExpectedDigest string
	InstallName    string
	Builtin        *BuiltinPresentation
}

// BuiltinPresentation is trusted release metadata read only from the
// Desktop-bundled plugin catalog. Ordinary plugin packages cannot opt into
// built-in ownership through their portable manifest.
type BuiltinPresentation struct {
	DisplayName string `json:"displayName"`
	BrandIcon   string `json:"brandIcon"`
	Order       int    `json:"order"`
}

type ComponentState struct {
	Component      ir.Component    `json:"component"`
	DesiredState   string          `json:"desiredState"`
	RuleActivation string          `json:"ruleActivation,omitempty"`
	Configuration  string          `json:"configurationState"`
	Authentication string          `json:"authenticationState"`
	Runtime        string          `json:"runtimeState"`
	Integrity      string          `json:"integrityState"`
	Mapping        ir.MappingState `json:"mappingState"`
	Switchable     bool            `json:"switchable"`
}

type SetupStatus string

const (
	SetupReady                SetupStatus = "ready"
	SetupNeedsConfiguration   SetupStatus = "needs_configuration"
	SetupNeedsAuth            SetupStatus = "needs_auth"
	SetupAuthorizing          SetupStatus = "authorizing"
	SetupVerifying            SetupStatus = "verifying"
	SetupPendingAdminApproval SetupStatus = "pending_admin_approval"
	SetupExpired              SetupStatus = "expired"
	SetupFailed               SetupStatus = "failed"
	SetupUnsupported          SetupStatus = "unsupported"
)

type SetupAction string

const (
	SetupActionConfigure         SetupAction = "configure"
	SetupActionAuthorize         SetupAction = "authorize"
	SetupActionCancel            SetupAction = "cancel"
	SetupActionMarkAdminApproval SetupAction = "mark_admin_approval"
	SetupActionRetry             SetupAction = "retry"
	SetupActionProbe             SetupAction = "probe"
	SetupActionDisconnect        SetupAction = "disconnect"
	SetupActionInspectOnly       SetupAction = "inspect_only"
)

type SetupItem struct {
	Requirement ir.SetupRequirement `json:"requirement"`
	Status      SetupStatus         `json:"status"`
	ReasonCode  string              `json:"reasonCode,omitempty"`
	Message     string              `json:"message,omitempty"`
	Actions     []SetupAction       `json:"actions"`
}

type SetupSummary struct {
	Total              int `json:"total"`
	Ready              int `json:"ready"`
	NeedsConfiguration int `json:"needsConfiguration"`
	NeedsAuth          int `json:"needsAuth"`
	NeedsAdminApproval int `json:"needsAdminApproval"`
	InProgress         int `json:"inProgress"`
	Failed             int `json:"failed"`
	Unsupported        int `json:"unsupported"`
}

// SetupReadinessProvider supplies host-owned authentication and external
// dependency readiness without giving the Plugin Store access to credentials.
type SetupReadinessProvider interface {
	SetupStatus(pluginName string, requirement ir.SetupRequirement) (SetupStatus, string)
}

// SetupCredentialReferenceProvider creates opaque Core-only references for
// targeted runtime bindings. Store and compiler layers never resolve the
// referenced credential value.
type SetupCredentialReferenceProvider interface {
	SetupCredentialReference(pluginName string, requirement ir.SetupRequirement, binding ir.SetupBinding) (string, error)
}

type ComponentDetail struct {
	State          ComponentState `json:"state"`
	Preview        string         `json:"preview,omitempty"`
	Source         string         `json:"source,omitempty"`
	SourceEditable bool           `json:"sourceEditable"`
	Properties     map[string]any `json:"properties,omitempty"`
	Resources      []string       `json:"resources,omitempty"`
}

type UpdateComponentSourceResult struct {
	Plugin         *InstalledPlugin
	PreviousDigest string
	Digest         string
	Changed        bool
}

type VariableField struct {
	Key          string               `json:"key"`
	Schema       ir.VariableSpec      `json:"schema"`
	Configured   bool                 `json:"configured"`
	Masked       bool                 `json:"masked"`
	Value        any                  `json:"value,omitempty"`
	Dependencies []VariableDependency `json:"dependencies,omitempty"`
}

type VariableDependency struct {
	LogicalID string           `json:"logicalId"`
	Kind      ir.ComponentKind `json:"kind"`
	Name      string           `json:"name"`
}

// InstallResult reports a successful install or replace.
type InstallResult struct {
	Plugin              *InstalledPlugin
	Replaced            bool
	PreviousTrashedPath string
	Message             string
}

// CommitInstalledChangesResult reports an authorized signing of the current
// installed package tree. Changed is false when the live tree and manifest
// version were already represented by the recorded state.
type CommitInstalledChangesResult struct {
	Plugin         *InstalledPlugin
	PreviousDigest string
	Digest         string
	Changed        bool
	Message        string
}

// ValidationResult is an authoring-time dry-run produced by the same
// materialization, manifest, containment, Skill discovery, MCP parsing, and
// digest code used by installation and runtime loading.
type ValidationResult struct {
	Valid             bool                   `json:"valid"`
	Source            Source                 `json:"source"`
	Manifest          Manifest               `json:"-"`
	Name              string                 `json:"name"`
	Version           string                 `json:"version,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Digest            string                 `json:"digest"`
	TargetRoot        string                 `json:"targetRoot"`
	SkillCount        int                    `json:"skillCount"`
	ServerCount       int                    `json:"serverCount"`
	Dialect           ir.Dialect             `json:"dialect,omitempty"`
	ManifestPath      string                 `json:"manifestPath,omitempty"`
	Components        []ir.Component         `json:"components,omitempty"`
	Setup             []ir.SetupRequirement  `json:"setupItems,omitempty"`
	ComponentTotals   ComponentTotals        `json:"componentTotals"`
	Candidates        []ir.ManifestCandidate `json:"candidates,omitempty"`
	RequiresSelection bool                   `json:"requiresSelection,omitempty"`
	SelectedDialect   ir.Dialect             `json:"selectedDialect,omitempty"`
	RequiredVariables []string               `json:"requiredVariables,omitempty"`
	ProcessIntents    []ProcessIntent        `json:"processIntents,omitempty"`
	Diagnostics       []Diagnostic           `json:"diagnostics"`
	Normalized        ir.NormalizedPlugin    `json:"-"`
}

type ComponentTotals struct {
	Total       int `json:"total"`
	Runnable    int `json:"runnable"`
	NeedsConfig int `json:"needsConfiguration"`
	Invalid     int `json:"invalid"`
	Unsupported int `json:"unsupported"`
	Ignored     int `json:"ignored"`
}

type ProcessIntent struct {
	ComponentID string `json:"componentId"`
	Kind        string `json:"kind"`
	Summary     string `json:"summary"`
}

func warning(code, message string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "warning"}
}

func errorDiag(code, message string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error"}
}
