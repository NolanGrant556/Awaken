// Package pluginagents compiles normalized Plugin Agent components into the
// existing native Subagent role shape.
package pluginagents

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"awaken/src/plugins/ir"
	"awaken/src/subagentruntime"
)

var unsafeTypeCharacter = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type RuntimeBudget struct {
	MaxTurns int
	Timeout  time.Duration
}

func EffectiveRuntimeBudget(spec *ir.AgentSpec) RuntimeBudget {
	if spec == nil {
		return RuntimeBudget{}
	}
	budget := RuntimeBudget{}
	if spec.MaxTurns > 0 {
		budget.MaxTurns = spec.MaxTurns
	}
	if spec.TimeoutSeconds > 0 {
		budget.Timeout = time.Duration(spec.TimeoutSeconds) * time.Second
	}
	return budget
}

type Definition struct {
	Role        subagentruntime.Role
	PluginName  string
	Digest      string
	LogicalID   string
	RevisionID  string
	Diagnostics []ir.Diagnostic
}

func TypeName(pluginName, agentName string) string {
	clean := func(value string) string {
		value = strings.Trim(unsafeTypeCharacter.ReplaceAllString(strings.TrimSpace(value), "_"), "_")
		return strings.ToLower(value)
	}
	return "plugin__" + clean(pluginName) + "__" + clean(agentName)
}

func Compile(pluginName, digest string, component ir.Component) (Definition, error) {
	if component.Kind != ir.KindAgent || component.Agent == nil || component.Validation == "invalid" || component.Mapping != ir.MappingAdapted && component.Mapping != ir.MappingNative {
		return Definition{}, fmt.Errorf("Plugin Agent %s is not runnable", component.LogicalID)
	}
	spec := component.Agent
	budget := EffectiveRuntimeBudget(spec)
	name := firstNonEmpty(spec.Name, component.DisplayName, component.DerivedName)
	readOnly := spec.ReadonlyHint || inferredReadOnly(spec.Tools)
	role := subagentruntime.Role{
		Type: TypeName(pluginName, name), WhenToUse: spec.Description,
		SystemPrompt: strings.TrimSpace(spec.Prompt), ReadOnly: readOnly,
		AllowedTools: append([]string(nil), spec.Tools...), DisallowedTools: append([]string(nil), spec.DisallowedTools...),
		ModelHint: spec.Model, MaxTurns: budget.MaxTurns,
		Timeout:    budget.Timeout,
		PluginName: pluginName, PluginDigest: digest, LogicalID: component.LogicalID, RevisionID: component.RevisionID,
	}
	definition := Definition{Role: role, PluginName: pluginName, Digest: digest, LogicalID: component.LogicalID, RevisionID: component.RevisionID}
	if model := strings.TrimSpace(spec.Model); model != "" && model != "inherit" {
		definition.Diagnostics = append(definition.Diagnostics, diagnostic(component, "plugin_agent_model_fallback", "vendor model hint is preserved for inspection; Awaken uses the host-resolved Subagent profile"))
	}
	if len(spec.Skills) > 0 || len(spec.MCPServers) > 0 {
		definition.Diagnostics = append(definition.Diagnostics, diagnostic(component, "plugin_agent_reference_not_applied", "unresolved Skill or MCP references are not silently granted to the child"))
	}
	if spec.Background || spec.IsolationHint != "" {
		definition.Diagnostics = append(definition.Diagnostics, diagnostic(component, "plugin_agent_execution_hint_adapted", "background/isolation hints are adapted to Awaken native Subagent lifecycle and workspace modes"))
	}
	return definition, nil
}

func inferredReadOnly(tools []string) bool {
	if len(tools) == 0 {
		return false
	}
	readOnly := map[string]bool{"read": true, "grep": true, "glob": true, "webfetch": true, "websearch": true, "ls": true}
	for _, name := range tools {
		if !readOnly[strings.ToLower(strings.TrimSpace(name))] {
			return false
		}
	}
	return true
}

func diagnostic(component ir.Component, code, message string) ir.Diagnostic {
	return ir.Diagnostic{Code: code, Message: message, Severity: "warning", Phase: "compile", ComponentKind: ir.KindAgent, ComponentID: component.LogicalID, SourcePath: component.SourcePath, SourceDialect: component.SourceDialect}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "agent"
}
