package runtimecatalog

import (
	"fmt"
	"strings"
	"time"
)

const RedactedValue = "[redacted]"

type RuntimeKind string

const (
	RuntimeKindAgent        RuntimeKind = "agent"
	RuntimeKindLocalCLI     RuntimeKind = "local_cli"
	RuntimeKindManagedAgent RuntimeKind = "managed_agent"
	RuntimeKindACP          RuntimeKind = "acp"
	RuntimeKindRemote       RuntimeKind = "remote"
	RuntimeKindMCPServer    RuntimeKind = "mcp_server"
	RuntimeKindLLMProvider  RuntimeKind = "llm_provider"
)

type InstallStatus string

const (
	InstallStatusUnknown      InstallStatus = "unknown"
	InstallStatusInstalled    InstallStatus = "installed"
	InstallStatusMissing      InstallStatus = "missing"
	InstallStatusNeedsInstall InstallStatus = "needs_install"
	InstallStatusInstalling   InstallStatus = "installing"
	InstallStatusUpgradable   InstallStatus = "upgradable"
	InstallStatusError        InstallStatus = "error"
)

type HealthStatus string

const (
	HealthStatusUnknown     HealthStatus = "unknown"
	HealthStatusHealthy     HealthStatus = "healthy"
	HealthStatusDegraded    HealthStatus = "degraded"
	HealthStatusUnreachable HealthStatus = "unreachable"
	HealthStatusDisabled    HealthStatus = "disabled"
)

type MCPTransport string

const (
	MCPTransportStdio MCPTransport = "stdio"
	MCPTransportHTTP  MCPTransport = "http"
	MCPTransportSSE   MCPTransport = "sse"
)

type IssueSeverity string

const (
	IssueSeverityInfo     IssueSeverity = "info"
	IssueSeverityWarning  IssueSeverity = "warning"
	IssueSeverityError    IssueSeverity = "error"
	IssueSeverityCritical IssueSeverity = "critical"
)

type RuntimeEntry struct {
	ID            string         `json:"id"`
	Kind          RuntimeKind    `json:"kind"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Enabled       bool           `json:"enabled"`
	Command       string         `json:"command,omitempty"`
	Args          []string       `json:"args,omitempty"`
	Version       string         `json:"version,omitempty"`
	InstallStatus InstallStatus  `json:"install_status"`
	Health        HealthSnapshot `json:"health"`
	Env           []EnvVar       `json:"env,omitempty"`
	Capabilities  []Capability   `json:"capabilities,omitempty"`
	ConfigOptions []ConfigOption `json:"config_options,omitempty"`
	Models        []ModelOption  `json:"models,omitempty"`
	Modes         []ModeOption   `json:"modes,omitempty"`
	MCPTransports []MCPTransport `json:"mcp_transports,omitempty"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
	Redacted    bool   `json:"redacted,omitempty"`
}

type HealthSnapshot struct {
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Code      string       `json:"code,omitempty"`
	Guidance  string       `json:"guidance,omitempty"`
	CheckedAt time.Time    `json:"checked_at,omitempty"`
	LatencyMS int          `json:"latency_ms,omitempty"`
}

type Capability struct {
	ID            string         `json:"id"`
	Name          string         `json:"name,omitempty"`
	Type          string         `json:"type,omitempty"`
	Description   string         `json:"description,omitempty"`
	Enabled       bool           `json:"enabled,omitempty"`
	MCPTransports []MCPTransport `json:"mcp_transports,omitempty"`
}

type ConfigOption struct {
	ID           string               `json:"id"`
	Label        string               `json:"label,omitempty"`
	Category     string               `json:"category,omitempty"`
	Type         string               `json:"type,omitempty"`
	Required     bool                 `json:"required,omitempty"`
	CurrentValue string               `json:"current_value,omitempty"`
	Sensitive    bool                 `json:"sensitive,omitempty"`
	Options      []ConfigOptionChoice `json:"options,omitempty"`
}

type ConfigOptionChoice struct {
	Value       string `json:"value"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type ModelOption struct {
	ID                  string `json:"id"`
	Label               string `json:"label,omitempty"`
	Description         string `json:"description,omitempty"`
	ContextWindowTokens int    `json:"context_window_tokens,omitempty"`
	MaxOutputTokens     int    `json:"max_output_tokens,omitempty"`
	Default             bool   `json:"default,omitempty"`
}

type ModeOption struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

type DiagnosticSummary struct {
	Total        int               `json:"total"`
	Available    int               `json:"available"`
	NeedsInstall int               `json:"needs_install"`
	Unreachable  int               `json:"unreachable"`
	Disabled     int               `json:"disabled"`
	Issues       []DiagnosticIssue `json:"issues,omitempty"`
}

type DiagnosticIssue struct {
	RuntimeID   string        `json:"runtime_id"`
	RuntimeName string        `json:"runtime_name"`
	Severity    IssueSeverity `json:"severity"`
	Code        string        `json:"code"`
	Message     string        `json:"message"`
	Guidance    string        `json:"guidance,omitempty"`
}

func Normalize(entry RuntimeEntry) RuntimeEntry {
	entry.ID = normalizeRuntimeID(entry.ID)
	entry.Kind = normalizeRuntimeKind(entry.Kind)
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Description = strings.TrimSpace(entry.Description)
	entry.Command = strings.TrimSpace(entry.Command)
	entry.Version = strings.TrimSpace(entry.Version)
	entry.InstallStatus = normalizeInstallStatus(entry.InstallStatus)
	entry.Health = normalizeHealthSnapshot(entry.Health)
	entry.Args = normalizeStringSlice(entry.Args)
	entry.Env = normalizeEnv(entry.Env)
	entry.Capabilities = dedupeCapabilities(entry.Capabilities)
	entry.ConfigOptions = normalizeConfigOptions(entry.ConfigOptions)
	entry.Models = dedupeModels(entry.Models)
	entry.Modes = dedupeModes(entry.Modes)
	entry.MCPTransports = dedupeMCPTransports(entry.MCPTransports)
	return entry
}

func normalizeRuntimeID(id string) string {
	return strings.TrimSpace(id)
}

func normalizeRuntimeKind(kind RuntimeKind) RuntimeKind {
	return RuntimeKind(strings.ToLower(strings.TrimSpace(string(kind))))
}

func Validate(entry RuntimeEntry) error {
	entry = Normalize(entry)
	if entry.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidRuntime)
	}
	if entry.Kind == "" {
		return fmt.Errorf("%w: kind is required", ErrInvalidRuntime)
	}
	if !isValidRuntimeKind(entry.Kind) {
		return fmt.Errorf("%w: invalid kind %q", ErrInvalidRuntime, entry.Kind)
	}
	if entry.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidRuntime)
	}
	if !isValidInstallStatus(entry.InstallStatus) {
		return fmt.Errorf("%w: invalid install status %q", ErrInvalidRuntime, entry.InstallStatus)
	}
	if !isValidHealthStatus(entry.Health.Status) {
		return fmt.Errorf("%w: invalid health status %q", ErrInvalidRuntime, entry.Health.Status)
	}
	if err := validateMCPTransports(entry.MCPTransports); err != nil {
		return err
	}
	for _, capability := range entry.Capabilities {
		if capability.ID == "" {
			return fmt.Errorf("%w: capability id is required", ErrInvalidRuntime)
		}
		if err := validateMCPTransports(capability.MCPTransports); err != nil {
			return err
		}
	}
	for _, model := range entry.Models {
		if model.ID == "" {
			return fmt.Errorf("%w: model id is required", ErrInvalidRuntime)
		}
	}
	for _, mode := range entry.Modes {
		if mode.ID == "" {
			return fmt.Errorf("%w: mode id is required", ErrInvalidRuntime)
		}
	}
	return nil
}

func normalizeInstallStatus(status InstallStatus) InstallStatus {
	status = InstallStatus(strings.ToLower(strings.TrimSpace(string(status))))
	if status == "" {
		return InstallStatusUnknown
	}
	return status
}

func normalizeHealthSnapshot(health HealthSnapshot) HealthSnapshot {
	health.Status = HealthStatus(strings.ToLower(strings.TrimSpace(string(health.Status))))
	if health.Status == "" {
		health.Status = HealthStatusUnknown
	}
	health.Message = strings.TrimSpace(health.Message)
	health.Code = strings.TrimSpace(health.Code)
	health.Guidance = strings.TrimSpace(health.Guidance)
	return health
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func normalizeEnv(vars []EnvVar) []EnvVar {
	if len(vars) == 0 {
		return nil
	}
	normalized := make([]EnvVar, 0, len(vars))
	for _, env := range vars {
		env.Name = strings.TrimSpace(env.Name)
		env.Description = strings.TrimSpace(env.Description)
		if env.Name == "" {
			continue
		}
		if env.Redacted || isSensitiveEnvName(env.Name) || looksSensitiveEnvValue(env.Value) {
			env.Value = RedactedValue
			env.Redacted = true
		}
		normalized = append(normalized, env)
	}
	return normalized
}

func dedupeCapabilities(capabilities []Capability) []Capability {
	if len(capabilities) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(capabilities))
	normalized := make([]Capability, 0, len(capabilities))
	for _, capability := range capabilities {
		capability.ID = strings.TrimSpace(capability.ID)
		capability.Name = strings.TrimSpace(capability.Name)
		capability.Type = strings.TrimSpace(capability.Type)
		capability.Description = strings.TrimSpace(capability.Description)
		capability.MCPTransports = dedupeMCPTransports(capability.MCPTransports)
		if capability.ID == "" {
			normalized = append(normalized, capability)
			continue
		}
		if _, ok := seen[capability.ID]; ok {
			continue
		}
		seen[capability.ID] = struct{}{}
		normalized = append(normalized, capability)
	}
	return normalized
}

func normalizeConfigOptions(options []ConfigOption) []ConfigOption {
	if len(options) == 0 {
		return nil
	}
	normalized := make([]ConfigOption, 0, len(options))
	for _, option := range options {
		option.ID = strings.TrimSpace(option.ID)
		option.Label = strings.TrimSpace(option.Label)
		option.Category = strings.TrimSpace(option.Category)
		option.Type = strings.TrimSpace(option.Type)
		option.CurrentValue = strings.TrimSpace(option.CurrentValue)
		option.Options = normalizeConfigOptionChoices(option.Options)
		normalized = append(normalized, option)
	}
	return normalized
}

func normalizeConfigOptionChoices(choices []ConfigOptionChoice) []ConfigOptionChoice {
	if len(choices) == 0 {
		return nil
	}
	normalized := make([]ConfigOptionChoice, 0, len(choices))
	for _, choice := range choices {
		choice.Value = strings.TrimSpace(choice.Value)
		choice.Label = strings.TrimSpace(choice.Label)
		choice.Description = strings.TrimSpace(choice.Description)
		if choice.Value != "" {
			normalized = append(normalized, choice)
		}
	}
	return normalized
}

func dedupeModels(models []ModelOption) []ModelOption {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	normalized := make([]ModelOption, 0, len(models))
	for _, model := range models {
		model.ID = strings.TrimSpace(model.ID)
		model.Label = strings.TrimSpace(model.Label)
		model.Description = strings.TrimSpace(model.Description)
		if model.ID == "" {
			normalized = append(normalized, model)
			continue
		}
		if _, ok := seen[model.ID]; ok {
			continue
		}
		seen[model.ID] = struct{}{}
		normalized = append(normalized, model)
	}
	return normalized
}

func dedupeModes(modes []ModeOption) []ModeOption {
	if len(modes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(modes))
	normalized := make([]ModeOption, 0, len(modes))
	for _, mode := range modes {
		mode.ID = strings.TrimSpace(mode.ID)
		mode.Label = strings.TrimSpace(mode.Label)
		mode.Description = strings.TrimSpace(mode.Description)
		if mode.ID == "" {
			normalized = append(normalized, mode)
			continue
		}
		if _, ok := seen[mode.ID]; ok {
			continue
		}
		seen[mode.ID] = struct{}{}
		normalized = append(normalized, mode)
	}
	return normalized
}

func dedupeMCPTransports(transports []MCPTransport) []MCPTransport {
	if len(transports) == 0 {
		return nil
	}
	seen := make(map[MCPTransport]struct{}, len(transports))
	normalized := make([]MCPTransport, 0, len(transports))
	for _, transport := range transports {
		transport = MCPTransport(strings.ToLower(strings.TrimSpace(string(transport))))
		if transport == "" {
			continue
		}
		if _, ok := seen[transport]; ok {
			continue
		}
		seen[transport] = struct{}{}
		normalized = append(normalized, transport)
	}
	return normalized
}

func isSensitiveEnvName(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if upper == "" {
		return false
	}
	if upper == "AUTHORIZATION" || upper == "BEARER" {
		return true
	}
	for _, part := range strings.FieldsFunc(upper, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	}) {
		switch part {
		case "KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "AUTH", "CREDENTIAL", "CREDENTIALS":
			return true
		}
	}
	return false
}

func looksSensitiveEnvValue(value string) bool {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "bearer ")
}

func isValidRuntimeKind(kind RuntimeKind) bool {
	switch kind {
	case RuntimeKindAgent, RuntimeKindLocalCLI, RuntimeKindManagedAgent, RuntimeKindACP, RuntimeKindRemote, RuntimeKindMCPServer, RuntimeKindLLMProvider:
		return true
	default:
		return false
	}
}

func isValidInstallStatus(status InstallStatus) bool {
	switch status {
	case InstallStatusUnknown, InstallStatusInstalled, InstallStatusMissing, InstallStatusNeedsInstall, InstallStatusInstalling, InstallStatusUpgradable, InstallStatusError:
		return true
	default:
		return false
	}
}

func isValidHealthStatus(status HealthStatus) bool {
	switch status {
	case HealthStatusUnknown, HealthStatusHealthy, HealthStatusDegraded, HealthStatusUnreachable, HealthStatusDisabled:
		return true
	default:
		return false
	}
}

func isValidMCPTransport(transport MCPTransport) bool {
	switch transport {
	case MCPTransportStdio, MCPTransportHTTP, MCPTransportSSE:
		return true
	default:
		return false
	}
}

func validateMCPTransports(transports []MCPTransport) error {
	for _, transport := range transports {
		if !isValidMCPTransport(transport) {
			return fmt.Errorf("%w: invalid MCP transport %q", ErrInvalidRuntime, transport)
		}
	}
	return nil
}
