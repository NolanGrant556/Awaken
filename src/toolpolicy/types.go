package toolpolicy

import "strings"

type ToolSource string

const (
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceMCP     ToolSource = "mcp"
	ToolSourcePlugin  ToolSource = "plugin"
	ToolSourceRuntime ToolSource = "runtime"
)

type ToolCapability string

const (
	ToolCapabilityRead    ToolCapability = "read"
	ToolCapabilityWrite   ToolCapability = "write"
	ToolCapabilityShell   ToolCapability = "shell"
	ToolCapabilityNetwork ToolCapability = "network"
	ToolCapabilityPlan    ToolCapability = "plan"
)

type ToolRef struct {
	Name         string
	Source       ToolSource
	Capabilities []ToolCapability
	MCPServer    string
	PluginID     string
	Runtime      string
	Description  string
}

func (t ToolRef) CanonicalName() string {
	name := strings.TrimSpace(t.Name)
	switch t.Source {
	case ToolSourceMCP:
		server := strings.TrimSpace(t.MCPServer)
		if server != "" {
			return "mcp:" + server + ":" + name
		}
	case ToolSourcePlugin:
		pluginID := strings.TrimSpace(t.PluginID)
		if pluginID != "" {
			return "plugin:" + pluginID + ":" + name
		}
	case ToolSourceRuntime:
		runtime := strings.TrimSpace(t.Runtime)
		if runtime != "" {
			return "runtime:" + runtime + ":" + name
		}
	}
	source := strings.TrimSpace(string(t.Source))
	if source == "" {
		return name
	}
	return source + ":" + name
}

func (t ToolRef) HasCapability(capability ToolCapability) bool {
	for _, candidate := range t.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

type AgentToolPolicy struct {
	// AllowedTools is a glob allowlist. nil means all available tools; an empty
	// non-nil slice means no tools.
	AllowedTools []string

	// DisallowedTools is always applied after AllowedTools and wins conflicts.
	DisallowedTools []string

	// RequiredMCPServers names MCP servers the agent expects to be available.
	RequiredMCPServers []string

	PermissionMode PermissionMode
}

type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeConservative      PermissionMode = "conservative"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
)

type MCPServerStatus string

const (
	MCPServerStatusAvailable   MCPServerStatus = "available"
	MCPServerStatusUnavailable MCPServerStatus = "unavailable"
	MCPServerStatusDisabled    MCPServerStatus = "disabled"
)

type MCPServerState struct {
	Name   string
	Status MCPServerStatus
	Reason string
}

func (s MCPServerState) IsAvailable() bool {
	return s.Status == "" || s.Status == MCPServerStatusAvailable
}

type ResolutionRequest struct {
	AvailableTools   []ToolRef
	Policy           AgentToolPolicy
	MCPServers       []MCPServerState
	WorkspaceTrusted bool
}

type ResolutionWarningCode string

const (
	ResolutionWarningMCPUnavailable ResolutionWarningCode = "mcp_unavailable"
)

type ResolutionWarning struct {
	Code      ResolutionWarningCode
	Message   string
	MCPServer string
	Tool      string
	Pattern   string
}

type ExclusionReason string

const (
	ExclusionNotAllowlisted     ExclusionReason = "not_allowlisted"
	ExclusionDisallowed         ExclusionReason = "disallowed"
	ExclusionMCPUnavailable     ExclusionReason = "mcp_unavailable"
	ExclusionPermissionMode     ExclusionReason = "permission_mode"
	ExclusionUntrustedWorkspace ExclusionReason = "untrusted_workspace"
)

type ResolutionExclusion struct {
	Tool    ToolRef
	Reason  ExclusionReason
	Pattern string
	Message string
}

type ResolvedToolSet struct {
	Tools      []ToolRef
	Warnings   []ResolutionWarning
	Exclusions []ResolutionExclusion
}
