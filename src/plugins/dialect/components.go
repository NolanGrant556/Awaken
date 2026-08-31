package dialect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/plugins/ir"
)

func parseSkillComponents(root string, paths []string, dialect ir.Dialect, manifestPath string, mapping ir.MappingState) ([]ir.Component, []ir.Diagnostic) {
	var components []ir.Component
	var diagnostics []ir.Diagnostic
	seen := map[string]bool{}
	for _, declared := range paths {
		clean, err := safeRelative(declared)
		if err != nil {
			diagnostics = append(diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindSkill, SourcePath: declared, SourceDialect: dialect})
			continue
		}
		files, err := directSkillFiles(root, clean)
		if err != nil {
			diagnostics = append(diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindSkill, SourcePath: clean, SourceDialect: dialect})
			continue
		}
		for _, file := range files {
			if seen[file] {
				continue
			}
			seen[file] = true
			frontmatter, _, raw, parseErr := parseMarkdown(root, file)
			derived := sourceName(file)
			packagePath := filepath.ToSlash(filepath.Dir(file))
			identityPath := packagePath
			if identityPath == "." {
				identityPath = filepath.ToSlash(file)
			}
			declaredName, _ := frontmatter["name"].(string)
			description, _ := frontmatter["description"].(string)
			component := ir.Component{
				LogicalID: ir.LogicalID(ir.KindSkill, identityPath, ""),
				Kind:      ir.KindSkill, DeclaredName: declaredName, DerivedName: derived,
				DisplayName: firstNonEmpty(declaredName, derived), Description: description,
				SourceDialect: dialect, SourceManifestPath: manifestPath, SourcePath: file,
				SourceDigest: digestBytes(raw), Mapping: mapping, Validation: "valid",
				CanonicalProvenance: string(dialect), Skill: &ir.SkillSpec{Path: packagePath},
			}
			if parseErr != nil || declaredName == "" || description == "" {
				component.Mapping = ir.MappingInvalid
				component.Validation = "invalid"
				message := "Skill requires name and description frontmatter"
				if parseErr != nil {
					message = parseErr.Error()
				}
				component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_skill_invalid", message, "error", component))
			}
			components = append(components, component)
		}
	}
	return components, diagnostics
}

func parseMCPFileComponents(root, relative string, dialect ir.Dialect, manifestPath string, mapping ir.MappingState) ([]ir.Component, []ir.Diagnostic) {
	raw, err := safeRead(root, relative)
	if err != nil {
		return nil, []ir.Diagnostic{{Code: "plugin_mcp_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindMCPServer, SourcePath: relative, SourceDialect: dialect}}
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, []ir.Diagnostic{{Code: "plugin_mcp_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindMCPServer, SourcePath: relative, SourceDialect: dialect}}
	}
	shape := "flat"
	servers := object
	for _, key := range []string{"mcpServers", "mcp_servers"} {
		if nested, ok := object[key].(map[string]any); ok {
			servers = nested
			shape = key
			break
		}
	}
	return parseMCPServerMap(servers, raw, shape, relative, dialect, manifestPath, mapping)
}

func parseMCPInlineComponents(value any, sourcePath string, dialect ir.Dialect, manifestPath string, mapping ir.MappingState) ([]ir.Component, []ir.Diagnostic) {
	servers, ok := value.(map[string]any)
	if !ok {
		return nil, []ir.Diagnostic{{Code: "plugin_mcp_invalid", Message: "inline mcpServers must be an object", Severity: "error", Phase: "parse", ComponentKind: ir.KindMCPServer, SourcePath: sourcePath, SourceDialect: dialect}}
	}
	if nested, ok := servers["mcpServers"].(map[string]any); ok {
		servers = nested
	}
	raw, _ := json.Marshal(servers)
	return parseMCPServerMap(servers, raw, "inline", sourcePath, dialect, manifestPath, mapping)
}

func parseMCPServerMap(servers map[string]any, raw []byte, shape, sourcePath string, dialect ir.Dialect, manifestPath string, mapping ir.MappingState) ([]ir.Component, []ir.Diagnostic) {
	keys := make([]string, 0, len(servers))
	for key := range servers {
		if key == "$schema" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	components := make([]ir.Component, 0, len(keys))
	for _, short := range keys {
		config, ok := servers[short].(map[string]any)
		component := ir.Component{
			LogicalID: ir.LogicalID(ir.KindMCPServer, sourcePath, short), Kind: ir.KindMCPServer,
			DeclaredName: short, DerivedName: short, DisplayName: short,
			SourceDialect: dialect, SourceManifestPath: manifestPath, SourcePath: sourcePath,
			SourceDigest: canonicalDigest(servers[short]), Mapping: mapping, Validation: "valid",
			CanonicalProvenance: string(dialect),
		}
		if !ok {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_mcp_server_invalid", "MCP server config must be an object", "error", component))
			components = append(components, component)
			continue
		}
		serverRaw, _ := json.Marshal(config)
		sourceTransport := stringField(config, "type")
		transport := sourceTransport
		if transport == "" {
			if stringField(config, "command") != "" {
				transport, sourceTransport = "stdio", "inferred-stdio"
			} else if stringField(config, "url") != "" {
				transport, sourceTransport = "streamable-http", "inferred-http"
			}
		}
		if transport == "http" {
			transport = "streamable-http"
		}
		spec := &ir.MCPSpec{
			ShortName: short, SourceShape: shape, SourceTransport: sourceTransport, Transport: transport,
			Command: stringField(config, "command"), Args: stringList(config["args"]), Cwd: stringField(config, "cwd"),
			URL: stringField(config, "url"), OAuthResource: firstNonEmpty(stringField(config, "oauth_resource"), stringField(config, "oauthResource")),
			OAuthClientID:          firstNonEmpty(stringField(config, "oauth_client_id"), stringField(config, "oauthClientId")),
			OAuthClientMetadataURL: firstNonEmpty(stringField(config, "oauth_client_metadata_url"), stringField(config, "oauthClientMetadataUrl")),
			OAuthScopes:            stringList(firstValue(config, "oauth_scopes", "oauthScopes")), Raw: serverRaw,
			Env: stringMap(config["env"]), Headers: stringMap(config["headers"]),
		}
		component.MCP = spec
		if transport != "stdio" && transport != "streamable-http" && transport != "sse" {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_mcp_server_unsupported", "unsupported MCP transport "+transport, "error", component))
		} else if transport == "stdio" && spec.Command == "" {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_mcp_server_invalid", "stdio MCP server requires command", "error", component))
		} else if (transport == "streamable-http" || transport == "sse") && spec.URL == "" {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_mcp_server_invalid", "remote MCP server requires url", "error", component))
		}
		components = append(components, component)
	}
	_ = raw
	return components, nil
}

func parseMarkdownComponents(root string, paths []string, extensions map[string]bool, kind ir.ComponentKind, dialect ir.Dialect, manifestPath string, mapping ir.MappingState, pluginName string) ([]ir.Component, []ir.Diagnostic) {
	var components []ir.Component
	var diagnostics []ir.Diagnostic
	seen := map[string]bool{}
	for _, path := range paths {
		files, err := walkFiles(root, path, extensions)
		if err != nil {
			diagnostics = append(diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: kind, SourcePath: path, SourceDialect: dialect})
			continue
		}
		for _, file := range files {
			if seen[file] {
				continue
			}
			seen[file] = true
			frontmatter, body, raw, parseErr := parseMarkdown(root, file)
			derived := sourceName(file)
			declaredName, _ := frontmatter["name"].(string)
			description, _ := frontmatter["description"].(string)
			component := ir.Component{
				LogicalID: ir.LogicalID(kind, file, ""), Kind: kind,
				DeclaredName: declaredName, DerivedName: derived, DisplayName: firstNonEmpty(declaredName, derived), Description: description,
				SourceDialect: dialect, SourceManifestPath: manifestPath, SourcePath: file, SourceDigest: digestBytes(raw),
				Mapping: mapping, Validation: "valid", CanonicalProvenance: string(dialect),
			}
			if parseErr != nil {
				component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
				component.Diagnostics = append(component.Diagnostics, componentDiagnostic("component_frontmatter_invalid", parseErr.Error(), "error", component))
			}
			switch kind {
			case ir.KindRule:
				globs := stringList(frontmatter["globs"])
				activation := "model_decides"
				always := boolPointer(frontmatter["alwaysApply"])
				if always != nil && *always {
					activation = "always"
				} else if len(globs) > 0 {
					activation = "file_globs"
				}
				component.Rule = &ir.RuleSpec{Description: description, AlwaysApply: always, Globs: globs, Activation: activation, Scope: firstNonEmpty(valueString(frontmatter["scope"]), "user"), Body: body, Frontmatter: frontmatter}
			case ir.KindAgent:
				component.Agent = &ir.AgentSpec{
					Name: firstNonEmpty(declaredName, derived), Description: description, Prompt: body,
					Model: valueString(frontmatter["model"]), Effort: valueString(frontmatter["effort"]),
					Tools: stringList(frontmatter["tools"]), DisallowedTools: stringList(firstValue(frontmatter, "disallowedTools", "disallowed-tools")),
					Skills: stringList(frontmatter["skills"]), MCPServers: stringList(firstValue(frontmatter, "mcpServers", "mcp-servers")),
					MaxTurns: valueInt(firstValue(frontmatter, "maxTurns", "max-turns")), TimeoutSeconds: valueInt(firstValue(frontmatter, "timeoutSeconds", "timeout-seconds", "timeout")),
					Background: valueBool(firstValue(frontmatter, "background", "is_background")), IsolationHint: valueString(frontmatter["isolation"]),
					ReadonlyHint: valueBool(firstValue(frontmatter, "readonly", "read_only", "is_read_only")), RawFrontmatter: frontmatter,
				}
				if strings.TrimSpace(body) == "" {
					component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
					component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_agent_invalid", "Agent requires a prompt body", "error", component))
				}
			case ir.KindCommand:
				invocation := "/" + pluginName + ":" + derived
				component.Command = &ir.CommandSpec{
					InvocationName: invocation, ArgumentHint: valueString(firstValue(frontmatter, "argument-hint", "argumentHint")),
					PromptTemplate: body, Model: valueString(frontmatter["model"]), AllowedTools: stringList(firstValue(frontmatter, "allowed-tools", "allowedTools")),
					ManualOnly: true, RawFrontmatter: frontmatter,
				}
				if strings.TrimSpace(description) == "" {
					component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
					component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_command_invalid", "Command requires description frontmatter", "error", component))
				}
			}
			components = append(components, component)
		}
	}
	return components, diagnostics
}

func finalizeAgentMappings(components []ir.Component, dialect ir.Dialect) []ir.Component {
	for index := range components {
		component := &components[index]
		if component.Kind != ir.KindAgent || component.Agent == nil || component.Validation == "invalid" {
			continue
		}
		if dialect == ir.DialectClaudeCode {
			for _, field := range []string{"hooks", "mcpServers", "mcp-servers", "permissionMode", "permission-mode"} {
				if _, exists := component.Agent.RawFrontmatter[field]; exists {
					component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_agent_field_not_applied", "Claude Code plugin Agent field "+field+" is not applied by the official plugin loader", "warning", *component))
				}
			}
			component.Agent.MCPServers = nil
		}
		component.Mapping = ir.MappingAdapted
	}
	return components
}

func parseHookFileComponents(root, relative string, dialect ir.Dialect, manifestPath string, mapping ir.MappingState, eventMap map[string]string) ([]ir.Component, []ir.Diagnostic) {
	raw, err := safeRead(root, relative)
	if err != nil {
		return nil, []ir.Diagnostic{{Code: "plugin_hook_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindHook, SourcePath: relative, SourceDialect: dialect}}
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, []ir.Diagnostic{{Code: "plugin_hook_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindHook, SourcePath: relative, SourceDialect: dialect}}
	}
	hooks, ok := document["hooks"].(map[string]any)
	if !ok {
		return nil, []ir.Diagnostic{{Code: "plugin_hook_invalid", Message: "hooks config requires hooks object", Severity: "error", Phase: "parse", ComponentKind: ir.KindHook, SourcePath: relative, SourceDialect: dialect}}
	}
	events := hookEventOrder(raw)
	seenEvents := make(map[string]bool, len(events))
	for _, event := range events {
		seenEvents[event] = true
	}
	var remaining []string
	for event := range hooks {
		if !seenEvents[event] {
			remaining = append(remaining, event)
		}
	}
	sort.Strings(remaining)
	events = append(events, remaining...)
	var components []ir.Component
	var diagnostics []ir.Diagnostic
	seenStable := map[string]int{}
	for _, event := range events {
		entries, ok := hooks[event].([]any)
		if !ok {
			diagnostics = append(diagnostics, ir.Diagnostic{Code: "plugin_hook_invalid", Message: event + " hook list must be an array", Severity: "error", Phase: "parse", ComponentKind: ir.KindHook, SourcePath: relative, SourceDialect: dialect})
			continue
		}
		for _, entryValue := range entries {
			entry, ok := entryValue.(map[string]any)
			if !ok {
				continue
			}
			matcher := stringField(entry, "matcher")
			handlers, hasNested := entry["hooks"].([]any)
			if !hasNested {
				handlers = []any{entry}
			}
			for _, handlerValue := range handlers {
				handler, ok := handlerValue.(map[string]any)
				if !ok {
					continue
				}
				handlerType := stringField(handler, "type")
				if handlerType == "" {
					handlerType = "command"
				}
				digest := canonicalDigest(handler)
				stable := event + "/" + digest[:8]
				ordinal := seenStable[stable]
				seenStable[stable]++
				if ordinal > 0 {
					stable += fmt.Sprintf("/%d", ordinal+1)
				}
				mappedEvent := eventMap[event]
				asyncRewake := valueBool(firstValue(handler, "asyncRewake", "async_rewake")) || valueBool(firstValue(entry, "asyncRewake", "async_rewake"))
				component := ir.Component{
					LogicalID: ir.LogicalID(ir.KindHook, relative, stable), Kind: ir.KindHook,
					DeclaredName: event, DerivedName: stable, DisplayName: event,
					SourceDialect: dialect, SourceManifestPath: manifestPath, SourcePath: relative,
					SourceDigest: digest, Mapping: mapping, Validation: "valid", CanonicalProvenance: string(dialect),
					Hook: &ir.HookSpec{
						SourceEvent: event, MappedEvent: mappedEvent, Matcher: matcher, HandlerType: handlerType,
						Scope: firstNonEmpty(stringField(handler, "scope"), stringField(entry, "scope"), "plugin"), OwnerComponentID: firstNonEmpty(stringField(handler, "ownerComponentId"), stringField(entry, "ownerComponentId")),
						Command: stringField(handler, "command"), URL: stringField(handler, "url"), Prompt: stringField(handler, "prompt"),
						MCPServer: stringField(handler, "server"), MCPTool: firstNonEmpty(stringField(handler, "tool"), stringField(handler, "toolName")),
						TimeoutSeconds: valueInt(handler["timeout"]),
						Condition:      firstNonEmpty(stringField(handler, "if"), stringField(entry, "if")),
						Async:          valueBool(handler["async"]) || valueBool(entry["async"]) || asyncRewake,
						AsyncRewake:    asyncRewake,
						RewakeMessage:  firstNonEmpty(stringField(handler, "rewakeMessage"), stringField(entry, "rewakeMessage")),
						RewakeSummary:  firstNonEmpty(stringField(handler, "rewakeSummary"), stringField(entry, "rewakeSummary")),
						LoopLimit:      intPointer(handler["loop_limit"]), Raw: mustJSON(handler),
					},
				}
				if mappedEvent == "" {
					component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_hook_event_unsupported", "hook event is recognized but has no Awaken mapping", "warning", component))
				}
				if ordinal > 0 {
					component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_hook_duplicate", "duplicate hook handler received a disambiguating suffix", "warning", component))
				}
				components = append(components, component)
			}
		}
	}
	return components, diagnostics
}

func hookEventOrder(raw []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil
		}
		key, _ := token.(string)
		if key != "hooks" {
			var ignored any
			if err := decoder.Decode(&ignored); err != nil {
				return nil
			}
			continue
		}
		if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
			return nil
		}
		var events []string
		for decoder.More() {
			eventToken, err := decoder.Token()
			if err != nil {
				return nil
			}
			event, _ := eventToken.(string)
			events = append(events, event)
			var ignored any
			if err := decoder.Decode(&ignored); err != nil {
				return nil
			}
		}
		return events
	}
	return nil
}

func finalizeHookMappings(components []ir.Component, dialect ir.Dialect) []ir.Component {
	for index := range components {
		component := &components[index]
		if component.Hook == nil {
			continue
		}
		supportedHandler := false
		switch dialect {
		case ir.DialectCodex:
			supportedHandler = component.Hook.HandlerType == "command"
		case ir.DialectClaudeCode:
			switch component.Hook.HandlerType {
			case "command", "http", "mcp_tool", "prompt", "agent":
				supportedHandler = true
			}
		case ir.DialectCursor:
			supportedHandler = component.Hook.HandlerType == "command" || component.Hook.HandlerType == "prompt"
		}
		if component.Hook.MappedEvent == "" || !supportedHandler {
			component.Mapping = ir.MappingRecognizedUnsupported
			if !supportedHandler {
				component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_hook_handler_unsupported", "hook handler is recognized but has no Awaken adapter", "warning", *component))
			}
			continue
		}
		if component.Hook.Scope != "" && component.Hook.Scope != "plugin" && component.Hook.Scope != "package" {
			component.Mapping = ir.MappingRecognizedUnsupported
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_hook_scope_deferred", "Agent/Skill-scoped Hook is preserved but only package-level Hook activation is currently available", "warning", *component))
			continue
		}
		if component.Hook.HandlerType == "command" && strings.TrimSpace(component.Hook.Command) == "" ||
			component.Hook.HandlerType == "http" && strings.TrimSpace(component.Hook.URL) == "" ||
			(component.Hook.HandlerType == "prompt" || component.Hook.HandlerType == "agent") && strings.TrimSpace(component.Hook.Prompt) == "" ||
			component.Hook.HandlerType == "mcp_tool" && (strings.TrimSpace(component.Hook.MCPServer) == "" || strings.TrimSpace(component.Hook.MCPTool) == "") {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_hook_handler_invalid", "hook handler is missing required fields", "error", *component))
			continue
		}
		component.Mapping = ir.MappingAdapted
	}
	return components
}

func ignoredFiles(root, relative string, extensions map[string]bool, kind ir.ComponentKind, dialect ir.Dialect, manifestPath, reason string) ([]ir.Component, []ir.Diagnostic) {
	files, err := walkFiles(root, relative, extensions)
	if err != nil {
		return nil, []ir.Diagnostic{{
			Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse",
			ComponentKind: kind, SourcePath: relative, SourceDialect: dialect,
		}}
	}
	components := make([]ir.Component, 0, len(files))
	for _, file := range files {
		raw, _ := safeRead(root, file)
		component := ir.Component{
			LogicalID: ir.LogicalID(kind, file, ""), Kind: kind, DerivedName: sourceName(file), DisplayName: sourceName(file),
			SourceDialect: dialect, SourceManifestPath: manifestPath, SourcePath: file, SourceDigest: digestBytes(raw),
			Mapping: ir.MappingIgnoredByDialect, Validation: "valid", CanonicalProvenance: string(dialect),
		}
		component.Diagnostics = append(component.Diagnostics, componentDiagnostic("component_ignored_by_dialect", reason, "info", component))
		components = append(components, component)
	}
	return components, nil
}

func existingPath(root, relative string) bool {
	clean, err := safeRelative(relative)
	if err != nil {
		return false
	}
	_, err = os.Lstat(filepath.Join(root, filepath.FromSlash(clean)))
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringMap(value any) map[string]string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(object))
	for key, raw := range object {
		if text, ok := raw.(string); ok {
			out[key] = text
		}
	}
	return out
}

func valueString(value any) string {
	text, _ := value.(string)
	return text
}

func valueInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func valueBool(value any) bool {
	boolean, _ := value.(bool)
	return boolean
}

func firstValue(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value
		}
	}
	return nil
}

func intPointer(value any) *int {
	if value == nil {
		return nil
	}
	integer := valueInt(value)
	return &integer
}

func mustJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
