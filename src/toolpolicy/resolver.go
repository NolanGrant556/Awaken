package toolpolicy

import (
	"path"
	"sort"
	"strings"
)

type Resolver struct{}

func Resolve(request ResolutionRequest) ResolvedToolSet {
	return Resolver{}.Resolve(request)
}

func (Resolver) Resolve(request ResolutionRequest) ResolvedToolSet {
	mcpServers := indexMCPServers(request.MCPServers)
	unavailableMCPServers := unavailableMCPServerSet(mcpServers)
	result := ResolvedToolSet{}
	warnings := warningSet{}

	for _, server := range request.Policy.RequiredMCPServers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		state, ok := mcpServers[normalizeMCPServer(server)]
		if !ok {
			state = MCPServerState{Name: server, Status: MCPServerStatusUnavailable, Reason: "required MCP server is not configured"}
			unavailableMCPServers[normalizeMCPServer(server)] = state
			result.Warnings = warnings.add(result.Warnings, mcpUnavailableWarning(server, "", mcpUnavailableMessage(state)))
			continue
		}
		if !state.IsAvailable() {
			unavailableMCPServers[normalizeMCPServer(server)] = state
			result.Warnings = warnings.add(result.Warnings, mcpUnavailableWarning(server, "", mcpUnavailableMessage(state)))
		}
	}

	allowAll := allowsAllTools(request.Policy.AllowedTools)
	seen := make(map[string]struct{}, len(request.AvailableTools))

	for _, tool := range request.AvailableTools {
		key := tool.CanonicalName()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if state, ok := unavailableMCPState(tool, unavailableMCPServers); ok {
			result.Warnings = warnings.add(result.Warnings, mcpUnavailableWarning(state.Name, "", mcpUnavailableMessage(state)))
			result.Exclusions = append(result.Exclusions, ResolutionExclusion{
				Tool:    tool,
				Reason:  ExclusionMCPUnavailable,
				Message: mcpUnavailableMessage(state),
			})
			continue
		}

		if !allowAll {
			if matched, _ := matchesAny(tool, request.Policy.AllowedTools); !matched {
				result.Exclusions = append(result.Exclusions, ResolutionExclusion{
					Tool:   tool,
					Reason: ExclusionNotAllowlisted,
				})
				continue
			}
		}

		if matched, pattern := matchesAny(tool, request.Policy.DisallowedTools); matched {
			result.Exclusions = append(result.Exclusions, ResolutionExclusion{
				Tool:    tool,
				Reason:  ExclusionDisallowed,
				Pattern: pattern,
			})
			continue
		}

		if !permissionModeAllows(request.Policy.PermissionMode, tool) {
			result.Exclusions = append(result.Exclusions, ResolutionExclusion{
				Tool:    tool,
				Reason:  ExclusionPermissionMode,
				Message: "tool capability is not allowed by permission mode",
			})
			continue
		}

		if !request.WorkspaceTrusted && !readOnlyOrPlanTool(tool) {
			result.Exclusions = append(result.Exclusions, ResolutionExclusion{
				Tool:    tool,
				Reason:  ExclusionUntrustedWorkspace,
				Message: "workspace is not trusted",
			})
			continue
		}

		result.Tools = append(result.Tools, tool)
	}

	sortToolRefs(result.Tools)
	sortWarnings(result.Warnings)
	sortExclusions(result.Exclusions)
	return result
}

func allowsAllTools(patterns []string) bool {
	if patterns == nil {
		return true
	}
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "*" {
			return true
		}
	}
	return false
}

func matchesAny(tool ToolRef, patterns []string) (bool, string) {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		for _, candidate := range matchCandidates(tool) {
			if globMatch(pattern, candidate) {
				return true, pattern
			}
		}
	}
	return false, ""
}

func globMatch(pattern string, candidate string) bool {
	if pattern == candidate {
		return true
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return false
	}
	matched, err := path.Match(pattern, candidate)
	return err == nil && matched
}

func matchCandidates(tool ToolRef) []string {
	name := strings.TrimSpace(tool.Name)
	candidates := []string{name, tool.CanonicalName()}

	if tool.Source == ToolSourceMCP {
		server := strings.TrimSpace(tool.MCPServer)
		if server != "" {
			candidates = append(candidates,
				"mcp:"+server+":"+name,
				"mcp__"+server+"__"+name,
				server+":"+name,
			)
		}
	}
	if tool.Source == ToolSourcePlugin {
		pluginID := strings.TrimSpace(tool.PluginID)
		if pluginID != "" {
			candidates = append(candidates, pluginID+":"+name)
		}
	}
	if tool.Source == ToolSourceRuntime {
		runtime := strings.TrimSpace(tool.Runtime)
		if runtime != "" {
			candidates = append(candidates, runtime+":"+name)
		}
	}

	return dedupeStrings(candidates)
}

func permissionModeAllows(mode PermissionMode, tool ToolRef) bool {
	switch mode {
	case "", PermissionModeDefault, PermissionModeBypassPermissions:
		return true
	case PermissionModeAcceptEdits:
		return !tool.HasCapability(ToolCapabilityShell)
	case PermissionModeConservative, PermissionModePlan:
		return readOnlyOrPlanTool(tool)
	default:
		return readOnlyOrPlanTool(tool)
	}
}

func readOnlyOrPlanTool(tool ToolRef) bool {
	if tool.HasCapability(ToolCapabilityWrite) || tool.HasCapability(ToolCapabilityShell) {
		return false
	}
	return tool.HasCapability(ToolCapabilityRead) || tool.HasCapability(ToolCapabilityPlan)
}

func indexMCPServers(servers []MCPServerState) map[string]MCPServerState {
	indexed := make(map[string]MCPServerState, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		server.Name = name
		indexed[normalizeMCPServer(name)] = server
	}
	return indexed
}

func unavailableMCPServerSet(servers map[string]MCPServerState) map[string]MCPServerState {
	unavailable := make(map[string]MCPServerState)
	for key, server := range servers {
		if !server.IsAvailable() {
			unavailable[key] = server
		}
	}
	return unavailable
}

func unavailableMCPState(tool ToolRef, servers map[string]MCPServerState) (MCPServerState, bool) {
	if tool.Source != ToolSourceMCP || strings.TrimSpace(tool.MCPServer) == "" {
		return MCPServerState{}, false
	}
	state, ok := servers[normalizeMCPServer(tool.MCPServer)]
	if !ok || state.IsAvailable() {
		return MCPServerState{}, false
	}
	return state, true
}

func normalizeMCPServer(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func mcpUnavailableWarning(server string, tool string, message string) ResolutionWarning {
	return ResolutionWarning{
		Code:      ResolutionWarningMCPUnavailable,
		Message:   message,
		MCPServer: server,
		Tool:      tool,
	}
}

func mcpUnavailableMessage(state MCPServerState) string {
	if strings.TrimSpace(state.Reason) != "" {
		return state.Reason
	}
	switch state.Status {
	case MCPServerStatusDisabled:
		return "MCP server is disabled"
	case MCPServerStatusUnavailable:
		return "MCP server is unavailable"
	default:
		return "MCP server is unavailable"
	}
}

func sortToolRefs(tools []ToolRef) {
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].CanonicalName() < tools[j].CanonicalName()
	})
}

func sortWarnings(warnings []ResolutionWarning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		left := warningSortKey(warnings[i])
		right := warningSortKey(warnings[j])
		return left < right
	})
}

func sortExclusions(exclusions []ResolutionExclusion) {
	sort.SliceStable(exclusions, func(i, j int) bool {
		left := string(exclusions[i].Reason) + ":" + exclusions[i].Tool.CanonicalName()
		right := string(exclusions[j].Reason) + ":" + exclusions[j].Tool.CanonicalName()
		return left < right
	})
}

func warningSortKey(warning ResolutionWarning) string {
	return string(warning.Code) + ":" + warning.MCPServer + ":" + warning.Tool + ":" + warning.Pattern
}

type warningSet map[string]struct{}

func (s warningSet) add(warnings []ResolutionWarning, warning ResolutionWarning) []ResolutionWarning {
	key := warningSortKey(warning)
	if _, ok := s[key]; ok {
		return warnings
	}
	s[key] = struct{}{}
	return append(warnings, warning)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}
