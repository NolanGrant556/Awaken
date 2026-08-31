package dialect

import (
	"fmt"
	"path/filepath"
	"sort"

	"awaken/src/plugins/ir"
)

type cursorAdapter struct{}

func (cursorAdapter) Dialect() ir.Dialect { return ir.DialectCursor }

var cursorHookEvents = map[string]string{
	"sessionStart": "RunStart", "sessionEnd": "RunEnd", "beforeSubmitPrompt": "UserPromptSubmit",
	"preToolUse": "PreToolUse", "postToolUse": "PostToolUse", "postToolUseFailure": "PostToolUseFailure",
	"preCompact": "PreCompact", "subagentStart": "SubagentStart", "subagentStop": "SubagentStop", "stop": "BeforeFinish",
}

func (cursorAdapter) Parse(root string, candidate ir.ManifestCandidate, options ParseOptions) (ir.NormalizedPlugin, error) {
	manifestPath := candidate.ManifestPath
	if manifestPath == "" {
		manifestPath = ".cursor-plugin/plugin.json"
	}
	raw, object, err := readManifest(root, manifestPath)
	if err != nil {
		return ir.NormalizedPlugin{}, err
	}
	identity := identityFromManifest(object)
	if identity.Name == "" {
		return ir.NormalizedPlugin{}, fmt.Errorf("Cursor plugin manifest requires name")
	}
	plugin := ir.NormalizedPlugin{
		Identity:    identity,
		Layout:      ir.PackageLayout{Dialect: ir.DialectCursor, Surface: "cursor_native", ManifestPath: manifestPath},
		RawManifest: raw,
	}

	// Cursor explicit fields replace folder discovery for that component.
	skillPaths, hasSkills, pathErr := cursorPaths(object, "skills")
	if pathErr != nil {
		plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(ir.KindSkill, manifestPath, pathErr, ir.DialectCursor))
	} else if !hasSkills && existingPath(root, "skills") {
		skillPaths = []string{"skills"}
	} else if !hasSkills && existingPath(root, "SKILL.md") {
		skillPaths = []string{""}
	}
	items, diags := parseSkillComponents(root, skillPaths, ir.DialectCursor, manifestPath, ir.MappingAdapted)
	plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)

	for _, definition := range []struct {
		Field       string
		DefaultPath string
		Kind        ir.ComponentKind
		Extensions  map[string]bool
	}{
		{Field: "rules", DefaultPath: "rules", Kind: ir.KindRule, Extensions: map[string]bool{".md": true, ".mdc": true, ".markdown": true}},
		{Field: "agents", DefaultPath: "agents", Kind: ir.KindAgent, Extensions: map[string]bool{".md": true, ".mdc": true, ".markdown": true}},
		{Field: "commands", DefaultPath: "commands", Kind: ir.KindCommand, Extensions: map[string]bool{".md": true, ".mdc": true, ".markdown": true, ".txt": true}},
	} {
		paths, declared, err := cursorPaths(object, definition.Field)
		if err != nil {
			plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(definition.Kind, manifestPath, err, ir.DialectCursor))
			continue
		}
		if !declared && existingPath(root, definition.DefaultPath) {
			paths = []string{definition.DefaultPath}
		}
		mapping := ir.MappingRecognizedUnsupported
		if definition.Kind == ir.KindRule || definition.Kind == ir.KindCommand || definition.Kind == ir.KindAgent {
			mapping = ir.MappingAdapted
		}
		items, diags = parseMarkdownComponents(root, paths, definition.Extensions, definition.Kind, ir.DialectCursor, manifestPath, mapping, identity.Name)
		if definition.Kind == ir.KindAgent {
			items = finalizeAgentMappings(items, ir.DialectCursor)
		}
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}

	if hookValue, declared := object["hooks"]; declared {
		paths := jsonPaths(hookValue)
		if len(paths) == 0 {
			plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(ir.KindHook, manifestPath, fmt.Errorf("inline Cursor hook config is recognized but not yet parsed"), ir.DialectCursor))
		}
		for _, path := range paths {
			clean, err := declaredRelative(path)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, dialectDiagnostic(ir.KindHook, path, err, ir.DialectCursor))
				continue
			}
			items, diags = parseHookFileComponents(root, clean, ir.DialectCursor, manifestPath, ir.MappingAdapted, cursorHookEvents)
			items = finalizeHookMappings(items, ir.DialectCursor)
			plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
		}
	} else if existingPath(root, "hooks/hooks.json") {
		items, diags = parseHookFileComponents(root, "hooks/hooks.json", ir.DialectCursor, manifestPath, ir.MappingAdapted, cursorHookEvents)
		items = finalizeHookMappings(items, ir.DialectCursor)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}

	if mcpValue, declared := object["mcpServers"]; declared {
		items, diags = parseCursorMCPSpec(root, mcpValue, manifestPath)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	} else if existingPath(root, "mcp.json") {
		items, diags = parseMCPFileComponents(root, "mcp.json", ir.DialectCursor, manifestPath, ir.MappingAdapted)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}
	if variables, ok := object["variables"].(map[string]any); ok {
		plugin.Components = append(plugin.Components, parseCursorVariables(variables, manifestPath)...)
	} else if _, present := object["variables"]; present {
		plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "variable_schema_invalid", Message: "Cursor variables must be a JSON Schema object", Severity: "error", Phase: "parse", ComponentKind: ir.KindVariable, SourcePath: manifestPath, SourceDialect: ir.DialectCursor})
	}
	if err := appendSetupDeclarations(root, object, manifestPath, ir.DialectCursor, &plugin); err != nil {
		return ir.NormalizedPlugin{}, err
	}
	_ = options
	return plugin, nil
}

func cursorPaths(object map[string]any, field string) ([]string, bool, error) {
	paths, declared, err := pathsField(object, field)
	if err != nil || !declared {
		return paths, declared, err
	}
	for index, path := range paths {
		clean, cleanErr := declaredRelative(path)
		if cleanErr != nil {
			return nil, true, cleanErr
		}
		paths[index] = clean
	}
	return paths, true, nil
}

func parseCursorMCPSpec(root string, value any, manifestPath string) ([]ir.Component, []ir.Diagnostic) {
	var components []ir.Component
	var diagnostics []ir.Diagnostic
	parseOne := func(spec any, index int) {
		switch typed := spec.(type) {
		case string:
			clean, err := declaredRelative(typed)
			if err != nil {
				diagnostics = append(diagnostics, dialectDiagnostic(ir.KindMCPServer, typed, err, ir.DialectCursor))
				return
			}
			items, diags := parseMCPFileComponents(root, clean, ir.DialectCursor, manifestPath, ir.MappingAdapted)
			components, diagnostics = append(components, items...), append(diagnostics, diags...)
		case map[string]any:
			items, diags := parseMCPInlineComponents(typed, fmt.Sprintf("%s#mcpServers/%d", manifestPath, index), ir.DialectCursor, manifestPath, ir.MappingAdapted)
			components, diagnostics = append(components, items...), append(diagnostics, diags...)
		default:
			diagnostics = append(diagnostics, dialectDiagnostic(ir.KindMCPServer, manifestPath, fmt.Errorf("unsupported Cursor mcpServers entry"), ir.DialectCursor))
		}
	}
	if list, ok := value.([]any); ok {
		for index, item := range list {
			parseOne(item, index)
		}
	} else {
		parseOne(value, 0)
	}
	return components, diagnostics
}

func parseCursorVariables(schema map[string]any, manifestPath string) []ir.Component {
	properties, ok := schema["properties"].(map[string]any)
	requiredList := stringList(schema["required"])
	required := make(map[string]bool, len(requiredList))
	for _, key := range requiredList {
		required[key] = true
	}
	if !ok || stringField(schema, "type") != "object" {
		component := ir.Component{
			LogicalID: ir.LogicalID(ir.KindVariable, "", "schema"), Kind: ir.KindVariable,
			DeclaredName: "variables", DerivedName: "variables", DisplayName: "Variables",
			SourceDialect: ir.DialectCursor, SourceManifestPath: manifestPath, SourcePath: manifestPath,
			SourceDigest: canonicalDigest(schema), Mapping: ir.MappingInvalid, Validation: "invalid",
			CanonicalProvenance: string(ir.DialectCursor),
		}
		component.Diagnostics = append(component.Diagnostics, componentDiagnostic("variable_schema_invalid", "Cursor variables require type=object and properties", "error", component))
		return []ir.Component{component}
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	components := make([]ir.Component, 0, len(keys))
	for _, key := range keys {
		definition, ok := properties[key].(map[string]any)
		component := ir.Component{
			LogicalID: ir.LogicalID(ir.KindVariable, "", key), Kind: ir.KindVariable,
			DeclaredName: key, DerivedName: key, DisplayName: firstNonEmpty(stringField(definition, "title"), key), Description: stringField(definition, "description"),
			SourceDialect: ir.DialectCursor, SourceManifestPath: manifestPath, SourcePath: manifestPath,
			SourceDigest: canonicalDigest(properties[key]), Mapping: ir.MappingAdapted, Validation: "valid",
			CanonicalProvenance: string(ir.DialectCursor),
		}
		if !ok {
			component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("variable_schema_invalid", "variable property must be an object", "error", component))
		} else {
			component.Variable = &ir.VariableSpec{
				Key: key, Type: stringField(definition, "type"), Title: stringField(definition, "title"), Description: stringField(definition, "description"),
				Required: required[key], Default: definition["default"], Enum: anyList(definition["enum"]), Const: definition["const"],
				Secret: cursorLooksSecret(key, definition), Minimum: floatPointer(firstValue(definition, "minimum", "min")), Maximum: floatPointer(firstValue(definition, "maximum", "max")), Raw: mustJSON(definition),
			}
			if component.Variable.Type == "" {
				component.Mapping, component.Validation = ir.MappingInvalid, "invalid"
				component.Diagnostics = append(component.Diagnostics, componentDiagnostic("variable_schema_invalid", "variable property requires type", "error", component))
			}
		}
		components = append(components, component)
	}
	return components
}

func anyList(value any) []any {
	list, _ := value.([]any)
	return append([]any(nil), list...)
}

func cursorLooksSecret(key string, definition map[string]any) bool {
	if secret, ok := definition["secret"].(bool); ok {
		return secret
	}
	format, _ := definition["format"].(string)
	if format == "password" {
		return true
	}
	upper := filepath.Base(key)
	for _, marker := range []string{"TOKEN", "SECRET", "PASSWORD", "API_KEY"} {
		if containsFold(upper, marker) {
			return true
		}
	}
	return false
}

func containsFold(value, part string) bool {
	if len(part) > len(value) {
		return false
	}
	for i := 0; i+len(part) <= len(value); i++ {
		if equalFoldASCII(value[i:i+len(part)], part) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'a' && ca <= 'z' {
			ca -= 'a' - 'A'
		}
		if cb >= 'a' && cb <= 'z' {
			cb -= 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
