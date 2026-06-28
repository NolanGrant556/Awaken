package toolpolicy

import (
	"reflect"
	"testing"
)

func TestResolveDenyWinsOverAllowlist(t *testing.T) {
	result := Resolve(ResolutionRequest{
		WorkspaceTrusted: true,
		AvailableTools: []ToolRef{
			builtinTool("read_file", ToolCapabilityRead),
			builtinTool("shell", ToolCapabilityShell),
		},
		Policy: AgentToolPolicy{
			AllowedTools:    []string{"read_file", "shell"},
			DisallowedTools: []string{"shell"},
		},
	})

	assertToolNames(t, result.Tools, []string{"builtin:read_file"})
	assertExcluded(t, result.Exclusions, "builtin:shell", ExclusionDisallowed)
}

func TestResolveAppliesAllowlistGlob(t *testing.T) {
	result := Resolve(ResolutionRequest{
		WorkspaceTrusted: true,
		AvailableTools: []ToolRef{
			builtinTool("shell", ToolCapabilityShell),
			builtinTool("file_write", ToolCapabilityWrite),
			builtinTool("file_read", ToolCapabilityRead),
		},
		Policy: AgentToolPolicy{
			AllowedTools: []string{"builtin:file_*"},
		},
	})

	assertToolNames(t, result.Tools, []string{"builtin:file_read", "builtin:file_write"})
	assertExcluded(t, result.Exclusions, "builtin:shell", ExclusionNotAllowlisted)
}

func TestResolveWarnsAndCutsMissingRequiredMCP(t *testing.T) {
	result := Resolve(ResolutionRequest{
		WorkspaceTrusted: true,
		AvailableTools: []ToolRef{
			mcpTool("github", "create_issue", ToolCapabilityWrite),
			builtinTool("read_file", ToolCapabilityRead),
		},
		Policy: AgentToolPolicy{
			RequiredMCPServers: []string{"github"},
		},
	})

	assertToolNames(t, result.Tools, []string{"builtin:read_file"})
	assertExcluded(t, result.Exclusions, "mcp:github:create_issue", ExclusionMCPUnavailable)

	if len(result.Warnings) != 1 {
		t.Fatalf("expected one warning, got %d: %#v", len(result.Warnings), result.Warnings)
	}
	warning := result.Warnings[0]
	if warning.Code != ResolutionWarningMCPUnavailable || warning.MCPServer != "github" || warning.Message != "required MCP server is not configured" {
		t.Fatalf("unexpected warning: %#v", warning)
	}
}

func TestResolveTrimsByPermissionMode(t *testing.T) {
	result := Resolve(ResolutionRequest{
		WorkspaceTrusted: true,
		AvailableTools: []ToolRef{
			builtinTool("apply_patch", ToolCapabilityWrite),
			builtinTool("exit_plan", ToolCapabilityPlan),
			builtinTool("read_file", ToolCapabilityRead),
			builtinTool("shell", ToolCapabilityShell),
		},
		Policy: AgentToolPolicy{
			PermissionMode: PermissionModePlan,
		},
	})

	assertToolNames(t, result.Tools, []string{"builtin:exit_plan", "builtin:read_file"})
	assertExcluded(t, result.Exclusions, "builtin:apply_patch", ExclusionPermissionMode)
	assertExcluded(t, result.Exclusions, "builtin:shell", ExclusionPermissionMode)
}

func TestResolveTrimsUntrustedWorkspace(t *testing.T) {
	result := Resolve(ResolutionRequest{
		WorkspaceTrusted: false,
		AvailableTools: []ToolRef{
			builtinTool("apply_patch", ToolCapabilityWrite),
			builtinTool("read_file", ToolCapabilityRead),
			builtinTool("shell", ToolCapabilityShell),
		},
		Policy: AgentToolPolicy{
			PermissionMode: PermissionModeDefault,
		},
	})

	assertToolNames(t, result.Tools, []string{"builtin:read_file"})
	assertExcluded(t, result.Exclusions, "builtin:apply_patch", ExclusionUntrustedWorkspace)
	assertExcluded(t, result.Exclusions, "builtin:shell", ExclusionUntrustedWorkspace)
}

func TestResolveSortsToolsStably(t *testing.T) {
	request := ResolutionRequest{
		WorkspaceTrusted: true,
		AvailableTools: []ToolRef{
			mcpTool("github", "search", ToolCapabilityRead),
			builtinTool("zeta", ToolCapabilityRead),
			pluginTool("lint", "format", ToolCapabilityWrite),
			builtinTool("alpha", ToolCapabilityRead),
			runtimeTool("workspace", "status", ToolCapabilityRead),
		},
	}

	first := Resolve(request)
	second := Resolve(request)

	expected := []string{
		"builtin:alpha",
		"builtin:zeta",
		"mcp:github:search",
		"plugin:lint:format",
		"runtime:workspace:status",
	}
	assertToolNames(t, first.Tools, expected)
	assertToolNames(t, second.Tools, expected)
}

func builtinTool(name string, capabilities ...ToolCapability) ToolRef {
	return ToolRef{
		Name:         name,
		Source:       ToolSourceBuiltin,
		Capabilities: capabilities,
	}
}

func mcpTool(server string, name string, capabilities ...ToolCapability) ToolRef {
	return ToolRef{
		Name:         name,
		Source:       ToolSourceMCP,
		MCPServer:    server,
		Capabilities: capabilities,
	}
}

func pluginTool(pluginID string, name string, capabilities ...ToolCapability) ToolRef {
	return ToolRef{
		Name:         name,
		Source:       ToolSourcePlugin,
		PluginID:     pluginID,
		Capabilities: capabilities,
	}
}

func runtimeTool(runtime string, name string, capabilities ...ToolCapability) ToolRef {
	return ToolRef{
		Name:         name,
		Source:       ToolSourceRuntime,
		Runtime:      runtime,
		Capabilities: capabilities,
	}
}

func assertToolNames(t *testing.T, tools []ToolRef, expected []string) {
	t.Helper()
	actual := make([]string, 0, len(tools))
	for _, tool := range tools {
		actual = append(actual, tool.CanonicalName())
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("tool names mismatch\nexpected: %#v\nactual:   %#v", expected, actual)
	}
}

func assertExcluded(t *testing.T, exclusions []ResolutionExclusion, toolName string, reason ExclusionReason) {
	t.Helper()
	for _, exclusion := range exclusions {
		if exclusion.Tool.CanonicalName() == toolName && exclusion.Reason == reason {
			return
		}
	}
	t.Fatalf("expected exclusion %s with reason %s, got %#v", toolName, reason, exclusions)
}
