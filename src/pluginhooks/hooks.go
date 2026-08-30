// Package pluginhooks compiles normalized vendor Hook components into the
// existing hookruntime. It owns vendor command/protocol adaptation but never
// executes handlers or builds a second dispatcher.
package pluginhooks

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"awaken/src/hookruntime"
	"awaken/src/plugins/ir"
	pluginvariables "awaken/src/plugins/variables"
)

const (
	DefaultPluginHookTimeoutMS       = 10 * 60 * 1000
	DefaultPluginSessionEndTimeoutMS = 1_500
	MaxPluginSessionEndTimeoutMS     = 3_000
)

func EffectiveTimeoutMS(spec *ir.HookSpec) int {
	if spec == nil {
		return DefaultPluginHookTimeoutMS
	}
	if strings.EqualFold(strings.TrimSpace(spec.SourceEvent), "SessionEnd") {
		timeout := DefaultPluginSessionEndTimeoutMS
		if spec.TimeoutSeconds > 0 {
			timeout = spec.TimeoutSeconds * 1000
		}
		if timeout > MaxPluginSessionEndTimeoutMS {
			return MaxPluginSessionEndTimeoutMS
		}
		return timeout
	}
	if spec.TimeoutSeconds > 0 {
		return spec.TimeoutSeconds * 1000
	}
	return DefaultPluginHookTimeoutMS
}

func EffectiveAsync(spec *ir.HookSpec) (async bool, asyncRewake bool) {
	if spec == nil || strings.EqualFold(strings.TrimSpace(spec.SourceEvent), "SessionEnd") {
		return false, false
	}
	handlerType := strings.ToLower(strings.TrimSpace(spec.HandlerType))
	if handlerType != "" && handlerType != "command" {
		return false, false
	}
	return spec.Async, spec.Async && spec.AsyncRewake
}

type Entry struct {
	PluginName   string
	PluginDigest string
	PluginRoot   string
	PluginData   string
	ProjectRoot  string
	Dialect      ir.Dialect
	Values       map[string]string
	Component    ir.Component
}

func Compile(entries []Entry) ([]hookruntime.CompiledHook, []hookruntime.Diagnostic) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PluginName != entries[j].PluginName {
			return entries[i].PluginName < entries[j].PluginName
		}
		return entries[i].Component.LogicalID < entries[j].Component.LogicalID
	})
	var hooks []hookruntime.CompiledHook
	var diagnostics []hookruntime.Diagnostic
	for _, entry := range entries {
		compiled, err := compileOne(entry)
		if err != nil {
			diagnostics = append(diagnostics, hookruntime.Diagnostic{Code: "plugin_hook_compile_failed", Message: err.Error(), SourcePath: entry.Component.SourcePath})
			continue
		}
		hooks = append(hooks, compiled)
	}
	return hooks, diagnostics
}

func compileOne(entry Entry) (hookruntime.CompiledHook, error) {
	component := entry.Component
	if component.Kind != ir.KindHook || component.Hook == nil || component.Mapping != ir.MappingAdapted && component.Mapping != ir.MappingNative || component.Validation == "invalid" {
		return hookruntime.CompiledHook{}, fmt.Errorf("Plugin Hook %s is not runnable", component.LogicalID)
	}
	event := hookruntime.Event(strings.TrimSpace(component.Hook.MappedEvent))
	if !supportedEvent(event) {
		return hookruntime.CompiledHook{}, fmt.Errorf("Plugin Hook %s has no supported event mapping", component.LogicalID)
	}
	command, err := compileHandler(entry)
	if err != nil {
		return hookruntime.CompiledHook{}, err
	}
	matcher, err := matcherFor(event, component.Hook.Matcher, component.Hook.Condition)
	if err != nil {
		return hookruntime.CompiledHook{}, fmt.Errorf("Plugin Hook %s matcher: %w", component.LogicalID, err)
	}
	protocol := string(entry.Dialect)
	if entry.Dialect == ir.DialectClaudeCode {
		protocol = "claude_code"
	}
	environment := map[string]string{
		"PLUGIN_ROOT": entry.PluginRoot, "PLUGIN_DATA": entry.PluginData,
		"CLAUDE_PLUGIN_ROOT": entry.PluginRoot, "CLAUDE_PLUGIN_DATA": entry.PluginData,
		"CURSOR_PLUGIN_ROOT": entry.PluginRoot, "CURSOR_PLUGIN_DATA": entry.PluginData,
		"CLAUDE_PROJECT_DIR": entry.ProjectRoot, "CURSOR_PROJECT_DIR": entry.ProjectRoot,
	}
	if entry.Dialect == ir.DialectClaudeCode {
		for key, value := range entry.Values {
			if safeEnvironmentKey(key) {
				environment["CLAUDE_PLUGIN_OPTION_"+key] = value
			}
		}
	}
	return hookruntime.CompiledHook{
		StableID: "plugin:" + entry.PluginName + ":" + component.LogicalID,
		Event:    event, Matcher: matcher, Command: command, SourceKind: hookruntime.SourcePlugin,
		SourcePath: component.SourcePath, ProjectRoot: entry.ProjectRoot,
		PluginName: entry.PluginName, PluginDigest: entry.PluginDigest,
		LogicalID: component.LogicalID, RevisionID: component.RevisionID,
		SourceEvent: component.Hook.SourceEvent, Protocol: protocol, Environment: environment,
	}, nil
}

func compileHandler(entry Entry) (hookruntime.CommandHook, error) {
	spec := entry.Component.Hook
	timeout := EffectiveTimeoutMS(spec)
	async, asyncRewake := EffectiveAsync(spec)
	command := hookruntime.CommandHook{
		ID: entry.Component.LogicalIDEncoded, Type: spec.HandlerType, TimeoutMS: timeout,
		MaxOutputBytes: 64 * 1024, OnFailure: "continue", CWD: "project_root",
		Async: async, AsyncRewake: asyncRewake, RewakeMessage: spec.RewakeMessage,
	}
	context := pluginvariables.InterpolationContext{PluginRoot: entry.PluginRoot, PluginData: entry.PluginData, ProjectDir: entry.ProjectRoot, Values: entry.Values}
	switch spec.HandlerType {
	case "command", "":
		expanded, err := pluginvariables.Expand(spec.Command, entry.Dialect, "hook.command", context)
		if err != nil {
			return command, err
		}
		argv, err := splitCommand(expanded)
		if err != nil {
			return command, fmt.Errorf("Plugin Hook %s command: %w", entry.Component.LogicalID, err)
		}
		command.Type, command.Command = "command", argv
	case "http":
		command.URL = spec.URL
	case "prompt", "agent":
		expanded, err := pluginvariables.Expand(spec.Prompt, entry.Dialect, "hook.prompt", context)
		if err != nil {
			return command, err
		}
		command.Prompt = expanded
		if spec.HandlerType == "agent" {
			command.MaxTurns = 8
		}
	case "mcp_tool":
		command.MCPServer, command.MCPTool = spec.MCPServer, spec.MCPTool
		var raw map[string]any
		if len(spec.Raw) > 0 && json.Unmarshal(spec.Raw, &raw) == nil {
			if arguments, ok := raw["arguments"]; ok {
				command.Arguments, _ = json.Marshal(arguments)
			} else if arguments, ok := raw["input"]; ok {
				command.Arguments, _ = json.Marshal(arguments)
			}
		}
	default:
		return command, fmt.Errorf("Plugin Hook %s handler %q is unsupported", entry.Component.LogicalID, spec.HandlerType)
	}
	return command, nil
}

func matcherFor(event hookruntime.Event, value, condition string) (hookruntime.Matcher, error) {
	if event != hookruntime.EventPreToolUse && event != hookruntime.EventPostToolUse && event != hookruntime.EventPostToolUseFailure {
		if strings.TrimSpace(condition) != "" {
			return hookruntime.Matcher{}, fmt.Errorf("handler condition is only supported for tool events")
		}
		return hookruntime.Matcher{}, nil
	}
	value = strings.TrimSpace(value)
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '|' || r == ',' })
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	if len(parts) == 0 {
		parts = []string{"*"}
	}
	matcher := hookruntime.Matcher{ToolNames: parts}
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return matcher, nil
	}
	open := strings.IndexByte(condition, '(')
	if open <= 0 || !strings.HasSuffix(condition, ")") {
		return hookruntime.Matcher{}, fmt.Errorf("unsupported condition %q", condition)
	}
	toolName := strings.TrimSpace(condition[:open])
	inputPattern := strings.TrimSpace(condition[open+1 : len(condition)-1])
	if toolName == "" || inputPattern == "" {
		return hookruntime.Matcher{}, fmt.Errorf("unsupported condition %q", condition)
	}
	compatible := false
	for _, candidate := range parts {
		if candidate == "*" || strings.EqualFold(candidate, toolName) {
			compatible = true
			break
		}
	}
	if !compatible {
		return hookruntime.Matcher{}, fmt.Errorf("condition tool %q conflicts with matcher %q", toolName, value)
	}
	// Claude's command matcher writes Bash(git commit:*): the colon separates
	// the literal prefix from its wildcard and is not part of the shell input.
	inputPattern = strings.ReplaceAll(inputPattern, ":*", "*")
	if _, err := path.Match(inputPattern, ""); err != nil {
		return hookruntime.Matcher{}, fmt.Errorf("invalid input pattern %q", inputPattern)
	}
	matcher.ToolNames = []string{toolName}
	matcher.InputPattern = inputPattern
	return matcher, nil
}

func supportedEvent(event hookruntime.Event) bool {
	switch event {
	case hookruntime.EventRunStart, hookruntime.EventUserPromptSubmit, hookruntime.EventPreToolUse,
		hookruntime.EventPostToolUse, hookruntime.EventPostToolUseFailure, hookruntime.EventPreCompact,
		hookruntime.EventPostCompact, hookruntime.EventSubagentStart, hookruntime.EventSubagentStop,
		hookruntime.EventBeforeFinish, hookruntime.EventRunEnd:
		return true
	}
	return false
}

func splitCommand(value string) ([]string, error) {
	var argv []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			argv = append(argv, current.String())
			current.Reset()
		}
	}
	for _, char := range strings.TrimSpace(value) {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if unicode.IsSpace(char) {
			flush()
			continue
		}
		if strings.ContainsRune("|;&><`", char) {
			return nil, fmt.Errorf("shell operator %q is not allowed", char)
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated escape or quote")
	}
	flush()
	if len(argv) == 0 {
		return nil, fmt.Errorf("command is empty")
	}
	if filepath.IsAbs(argv[0]) {
		argv[0] = filepath.Clean(argv[0])
	}
	return argv, nil
}

func safeEnvironmentKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !(char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
