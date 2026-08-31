package dialect

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/plugins/ir"
)

type claudeCodeAdapter struct{}

func (claudeCodeAdapter) Dialect() ir.Dialect { return ir.DialectClaudeCode }

var claudeHookEvents = map[string]string{
	"SessionStart": "RunStart", "SessionEnd": "RunEnd", "UserPromptSubmit": "UserPromptSubmit",
	"PreToolUse": "PreToolUse", "PostToolUse": "PostToolUse", "PostToolUseFailure": "PostToolUseFailure",
	"PreCompact": "PreCompact", "PostCompact": "PostCompact", "SubagentStart": "SubagentStart",
	"SubagentStop": "SubagentStop", "Stop": "BeforeFinish",
}

func (claudeCodeAdapter) Parse(root string, candidate ir.ManifestCandidate, options ParseOptions) (ir.NormalizedPlugin, error) {
	manifestPath := candidate.ManifestPath
	var raw []byte
	object := map[string]any{}
	var err error
	if !candidate.Manifestless {
		if manifestPath == "" {
			manifestPath = ".claude-plugin/plugin.json"
		}
		raw, object, err = readManifest(root, manifestPath)
		if err != nil {
			return ir.NormalizedPlugin{}, err
		}
	}
	identity := identityFromManifest(object)
	if identity.Name == "" {
		identity.Name = firstNonEmpty(candidate.IdentityHint, filepath.Base(root))
		identity.DisplayName = identity.Name
	}
	plugin := ir.NormalizedPlugin{
		Identity:    identity,
		Layout:      ir.PackageLayout{Dialect: ir.DialectClaudeCode, Surface: "claude_code_native", ManifestPath: manifestPath, Manifestless: candidate.Manifestless},
		RawManifest: raw,
	}

	// Claude always scans default skills/ and appends explicitly declared paths.
	var skillPaths []string
	if existingPath(root, "skills") {
		skillPaths = append(skillPaths, "skills")
	}
	declaredSkillPaths, hasDeclaredSkills, pathErr := pathsField(object, "skills")
	if pathErr != nil {
		plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(ir.KindSkill, manifestPath, pathErr, ir.DialectClaudeCode))
	} else {
		for _, path := range declaredSkillPaths {
			clean, err := claudeDeclaredPath(path)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(ir.KindSkill, path, err, ir.DialectClaudeCode))
				continue
			}
			skillPaths = append(skillPaths, clean)
		}
	}
	if len(skillPaths) == 0 && !hasDeclaredSkills && existingPath(root, "SKILL.md") {
		skillPaths = []string{""}
	}
	items, diags := parseSkillComponents(root, skillPaths, ir.DialectClaudeCode, manifestPath, ir.MappingAdapted)
	plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)

	// commands and agents use explicit manifest paths instead of defaults.
	plugin.Components, plugin.Diagnostics = appendMarkdownType(plugin.Components, plugin.Diagnostics, root, object, "commands", "commands", map[string]bool{".md": true}, ir.KindCommand, ir.DialectClaudeCode, manifestPath, identity.Name)
	beforeAgents := len(plugin.Components)
	plugin.Components, plugin.Diagnostics = appendMarkdownType(plugin.Components, plugin.Diagnostics, root, object, "agents", "agents", map[string]bool{".md": true}, ir.KindAgent, ir.DialectClaudeCode, manifestPath, identity.Name)
	copy(plugin.Components[beforeAgents:], finalizeAgentMappings(plugin.Components[beforeAgents:], ir.DialectClaudeCode))

	if existingPath(root, "hooks/hooks.json") {
		items, diags = parseHookFileComponents(root, "hooks/hooks.json", ir.DialectClaudeCode, manifestPath, ir.MappingAdapted, claudeHookEvents)
		items = finalizeHookMappings(items, ir.DialectClaudeCode)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}
	if hookValue, ok := object["hooks"]; ok {
		for _, path := range jsonPaths(hookValue) {
			clean, err := claudeDeclaredPath(path)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(ir.KindHook, path, err, ir.DialectClaudeCode))
				continue
			}
			if clean == "hooks/hooks.json" {
				continue
			}
			items, diags = parseHookFileComponents(root, clean, ir.DialectClaudeCode, manifestPath, ir.MappingAdapted, claudeHookEvents)
			items = finalizeHookMappings(items, ir.DialectClaudeCode)
			plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
		}
	}

	if existingPath(root, ".mcp.json") {
		items, diags = parseMCPFileComponents(root, ".mcp.json", ir.DialectClaudeCode, manifestPath, ir.MappingAdapted)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}
	if mcpValue, ok := object["mcpServers"]; ok {
		mcpItems, mcpDiags := parseClaudeMCPSpec(root, mcpValue, manifestPath)
		plugin.Components = mergeMCPComponents(plugin.Components, mcpItems)
		plugin.Diagnostics = append(plugin.Diagnostics, mcpDiags...)
	}

	if userConfig, ok := object["userConfig"].(map[string]any); ok {
		plugin.Components = append(plugin.Components, parseClaudeVariables(userConfig, manifestPath)...)
	}
	if existingPath(root, "rules") {
		ignored, ignoredDiagnostics := ignoredFiles(root, "rules", map[string]bool{".md": true, ".mdc": true, ".markdown": true}, ir.KindRule, ir.DialectClaudeCode, manifestPath, "Claude Code plugin loader does not define Rules as a plugin component")
		plugin.Components = append(plugin.Components, ignored...)
		plugin.Diagnostics = append(plugin.Diagnostics, ignoredDiagnostics...)
	}
	for _, unsupported := range []struct {
		field string
		kind  ir.ComponentKind
	}{
		{"lspServers", ir.KindUnknown}, {"outputStyles", ir.KindUnknown}, {"channels", ir.KindUnknown},
	} {
		if value, ok := object[unsupported.field]; ok {
			rawValue, _ := json.Marshal(value)
			component := ir.Component{
				LogicalID: ir.LogicalID(unsupported.kind, manifestPath, unsupported.field), Kind: unsupported.kind,
				DeclaredName: unsupported.field, DerivedName: unsupported.field, DisplayName: unsupported.field,
				SourceDialect: ir.DialectClaudeCode, SourceManifestPath: manifestPath, SourcePath: manifestPath,
				SourceDigest: digestBytes(rawValue), Mapping: ir.MappingRecognizedUnsupported, Validation: "valid",
				CanonicalProvenance: string(ir.DialectClaudeCode),
			}
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("component_out_of_scope", "component is outside the seven-type compatibility scope", "info", component))
			plugin.Components = append(plugin.Components, component)
		}
	}
	if err := appendSetupDeclarations(root, object, manifestPath, ir.DialectClaudeCode, &plugin); err != nil {
		return ir.NormalizedPlugin{}, err
	}
	_ = options
	return plugin, nil
}

func claudeDeclaredPath(path string) (string, error) {
	if path == "." {
		return "", nil
	}
	return declaredRelative(path)
}

func appendMarkdownType(components []ir.Component, diagnostics []ir.Diagnostic, root string, object map[string]any, field, defaultPath string, extensions map[string]bool, kind ir.ComponentKind, dialect ir.Dialect, manifestPath, pluginName string) ([]ir.Component, []ir.Diagnostic) {
	value, declared := object[field]
	var paths []string
	if declared {
		paths = jsonPaths(value)
		if len(paths) == 0 {
			if mapping, ok := value.(map[string]any); ok && kind == ir.KindCommand {
				inline, diags := parseClaudeCommandMapping(root, mapping, manifestPath, pluginName)
				return append(components, inline...), append(diagnostics, diags...)
			}
			diagnostics = append(diagnostics, dialectDiagnostic(kind, manifestPath, fmt.Errorf("%s must be a path, path array, or supported mapping", field), dialect))
			return components, diagnostics
		}
		for i, path := range paths {
			clean, err := claudeDeclaredPath(path)
			if err != nil {
				diagnostics = append(diagnostics, dialectDiagnostic(kind, path, err, dialect))
				paths[i] = "__invalid__"
				continue
			}
			paths[i] = clean
		}
	} else if existingPath(root, defaultPath) {
		paths = []string{defaultPath}
	}
	mappingState := ir.MappingRecognizedUnsupported
	if kind == ir.KindCommand || kind == ir.KindAgent {
		mappingState = ir.MappingAdapted
	}
	items, diags := parseMarkdownComponents(root, paths, extensions, kind, dialect, manifestPath, mappingState, pluginName)
	return append(components, items...), append(diagnostics, diags...)
}

func parseClaudeCommandMapping(root string, mapping map[string]any, manifestPath, pluginName string) ([]ir.Component, []ir.Diagnostic) {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var components []ir.Component
	var diagnostics []ir.Diagnostic
	for _, key := range keys {
		meta, ok := mapping[key].(map[string]any)
		if !ok {
			continue
		}
		path := stringField(meta, "source")
		content := stringField(meta, "content")
		var raw []byte
		var body string
		var sourcePath string
		if path != "" {
			clean, err := claudeDeclaredPath(path)
			if err != nil {
				diagnostics = append(diagnostics, dialectDiagnostic(ir.KindCommand, path, err, ir.DialectClaudeCode))
				continue
			}
			sourcePath = clean
			_, body, raw, err = parseMarkdown(root, clean)
			if err != nil {
				diagnostics = append(diagnostics, dialectDiagnostic(ir.KindCommand, clean, err, ir.DialectClaudeCode))
				continue
			}
		} else {
			sourcePath = manifestPath + "#commands/" + key
			body, raw = content, []byte(content)
		}
		component := ir.Component{
			LogicalID: ir.LogicalID(ir.KindCommand, sourcePath, key), Kind: ir.KindCommand,
			DeclaredName: key, DerivedName: key, DisplayName: key, Description: stringField(meta, "description"),
			SourceDialect: ir.DialectClaudeCode, SourceManifestPath: manifestPath, SourcePath: sourcePath,
			SourceDigest: digestBytes(raw), Mapping: ir.MappingAdapted, Validation: "valid",
			CanonicalProvenance: string(ir.DialectClaudeCode), Command: &ir.CommandSpec{
				InvocationName: "/" + pluginName + ":" + key, ArgumentHint: stringField(meta, "argumentHint"), PromptTemplate: body,
				Model: stringField(meta, "model"), AllowedTools: stringList(meta["allowedTools"]), ManualOnly: true, RawFrontmatter: meta,
			},
		}
		if strings.TrimSpace(component.Description) == "" {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_command_invalid", "Command requires description", "error", component))
		}
		components = append(components, component)
	}
	return components, diagnostics
}

func parseClaudeMCPSpec(root string, value any, manifestPath string) ([]ir.Component, []ir.Diagnostic) {
	var components []ir.Component
	var diagnostics []ir.Diagnostic
	parseOne := func(spec any, index int) {
		switch typed := spec.(type) {
		case string:
			clean, err := claudeDeclaredPath(typed)
			if err != nil {
				diagnostics = append(diagnostics, dialectDiagnostic(ir.KindMCPServer, typed, err, ir.DialectClaudeCode))
				return
			}
			items, diags := parseMCPFileComponents(root, clean, ir.DialectClaudeCode, manifestPath, ir.MappingAdapted)
			components, diagnostics = append(components, items...), append(diagnostics, diags...)
		case map[string]any:
			items, diags := parseMCPInlineComponents(typed, fmt.Sprintf("%s#mcpServers/%d", manifestPath, index), ir.DialectClaudeCode, manifestPath, ir.MappingAdapted)
			components, diagnostics = append(components, items...), append(diagnostics, diags...)
		default:
			diagnostics = append(diagnostics, dialectDiagnostic(ir.KindMCPServer, manifestPath, fmt.Errorf("unsupported mcpServers entry"), ir.DialectClaudeCode))
		}
	}
	if list, ok := value.([]any); ok {
		for index, spec := range list {
			parseOne(spec, index)
		}
	} else {
		parseOne(value, 0)
	}
	return components, diagnostics
}

func mergeMCPComponents(existing, overrides []ir.Component) []ir.Component {
	if len(overrides) == 0 {
		return existing
	}
	byName := make(map[string]int)
	for index, component := range existing {
		if component.Kind == ir.KindMCPServer {
			byName[component.DerivedName] = index
		}
	}
	for _, component := range overrides {
		if index, ok := byName[component.DerivedName]; ok {
			component.SecondaryProvenance = append(component.SecondaryProvenance, existing[index].SourcePath)
			existing[index] = component
		} else {
			existing = append(existing, component)
		}
	}
	return existing
}

func parseClaudeVariables(userConfig map[string]any, manifestPath string) []ir.Component {
	keys := make([]string, 0, len(userConfig))
	for key := range userConfig {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	components := make([]ir.Component, 0, len(keys))
	for _, key := range keys {
		definition, ok := userConfig[key].(map[string]any)
		component := ir.Component{
			LogicalID: ir.LogicalID(ir.KindVariable, "", key), Kind: ir.KindVariable,
			DeclaredName: key, DerivedName: key, DisplayName: firstNonEmpty(stringField(definition, "title"), key), Description: stringField(definition, "description"),
			SourceDialect: ir.DialectClaudeCode, SourceManifestPath: manifestPath, SourcePath: manifestPath,
			SourceDigest: canonicalDigest(userConfig[key]), Mapping: ir.MappingAdapted, Validation: "valid",
			CanonicalProvenance: string(ir.DialectClaudeCode),
		}
		if !ok {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("variable_schema_invalid", "userConfig entry must be an object", "error", component))
		} else {
			component.Variable = &ir.VariableSpec{
				Key: key, Type: stringField(definition, "type"), Title: stringField(definition, "title"), Description: stringField(definition, "description"),
				Required: valueBool(definition["required"]), Default: definition["default"], Secret: valueBool(definition["sensitive"]),
				Multiple: valueBool(definition["multiple"]), Minimum: floatPointer(definition["min"]), Maximum: floatPointer(definition["max"]), Raw: mustJSON(definition),
			}
			if component.Variable.Type == "" || component.Variable.Title == "" || component.Variable.Description == "" {
				component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
				component.Diagnostics = append(component.Diagnostics, componentDiagnostic("variable_schema_invalid", "userConfig requires type, title, and description", "error", component))
			}
		}
		components = append(components, component)
	}
	return components
}

func floatPointer(value any) *float64 {
	if number, ok := value.(float64); ok {
		return &number
	}
	return nil
}

func dialectDiagnostic(kind ir.ComponentKind, path string, err error, dialect ir.Dialect) ir.Diagnostic {
	return ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: kind, SourcePath: path, SourceDialect: dialect}
}
