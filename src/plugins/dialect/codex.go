package dialect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/plugins/ir"
)

type codexAdapter struct{}

func (codexAdapter) Dialect() ir.Dialect { return ir.DialectCodex }

var codexHookEvents = map[string]string{
	"SessionStart": "RunStart", "SessionEnd": "RunEnd", "UserPromptSubmit": "UserPromptSubmit",
	"PreToolUse": "PreToolUse", "PostToolUse": "PostToolUse", "SubagentStart": "SubagentStart",
	"SubagentStop": "SubagentStop", "Stop": "BeforeFinish", "PreCompact": "PreCompact", "PostCompact": "PostCompact",
}

func (codexAdapter) Parse(root string, candidate ir.ManifestCandidate, options ParseOptions) (ir.NormalizedPlugin, error) {
	manifestPath := candidate.ManifestPath
	if manifestPath == "" {
		manifestPath = ".codex-plugin/plugin.json"
	}
	raw, object, err := readManifest(root, manifestPath)
	if err != nil {
		return ir.NormalizedPlugin{}, err
	}
	identity := identityFromManifest(object)
	if identity.Name == "" {
		identity.Name = filepath.Base(root)
	}
	plugin := ir.NormalizedPlugin{
		Identity:    identity,
		Layout:      ir.PackageLayout{Dialect: ir.DialectCodex, Surface: "codex_native", ManifestPath: manifestPath},
		RawManifest: raw,
	}

	skillPaths, declaredSkills, pathErr := pathsField(object, "skills")
	if pathErr != nil {
		plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: pathErr.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindSkill, SourcePath: manifestPath, SourceDialect: ir.DialectCodex})
	} else if !declaredSkills && existingPath(root, "skills") {
		skillPaths = []string{"skills"}
	}
	for i, path := range skillPaths {
		if declaredSkills {
			clean, err := declaredRelative(path)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindSkill, SourcePath: path, SourceDialect: ir.DialectCodex})
				continue
			}
			skillPaths[i] = clean
		}
	}
	components, diagnostics := parseSkillComponents(root, skillPaths, ir.DialectCodex, manifestPath, ir.MappingAdapted)
	plugin.Components = append(plugin.Components, components...)
	plugin.Diagnostics = append(plugin.Diagnostics, diagnostics...)

	if mcpValue, ok := object["mcpServers"]; ok {
		switch typed := mcpValue.(type) {
		case string:
			clean, err := declaredRelative(typed)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindMCPServer, SourcePath: typed, SourceDialect: ir.DialectCodex})
			} else {
				items, diags := parseMCPFileComponents(root, clean, ir.DialectCodex, manifestPath, ir.MappingAdapted)
				plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
			}
		case map[string]any:
			items, diags := parseMCPInlineComponents(typed, manifestPath+"#mcpServers", ir.DialectCodex, manifestPath, ir.MappingAdapted)
			plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
		default:
			plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "plugin_mcp_invalid", Message: "mcpServers must be a path or object", Severity: "error", Phase: "parse", ComponentKind: ir.KindMCPServer, SourcePath: manifestPath, SourceDialect: ir.DialectCodex})
		}
	} else if existingPath(root, ".mcp.json") {
		items, diags := parseMCPFileComponents(root, ".mcp.json", ir.DialectCodex, manifestPath, ir.MappingAdapted)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}

	if hookValue, ok := object["hooks"]; ok {
		for _, path := range jsonPaths(hookValue) {
			clean, err := declaredRelative(path)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindHook, SourcePath: path, SourceDialect: ir.DialectCodex})
				continue
			}
			items, diags := parseHookFileComponents(root, clean, ir.DialectCodex, manifestPath, ir.MappingAdapted, codexHookEvents)
			items = finalizeHookMappings(items, ir.DialectCodex)
			plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
		}
	} else if existingPath(root, "hooks/hooks.json") {
		items, diags := parseHookFileComponents(root, "hooks/hooks.json", ir.DialectCodex, manifestPath, ir.MappingAdapted, codexHookEvents)
		items = finalizeHookMappings(items, ir.DialectCodex)
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}
	if existingPath(root, "hooks.json") {
		ignored, ignoredDiagnostics := ignoredFiles(root, "hooks.json", map[string]bool{".json": true}, ir.KindHook, ir.DialectCodex, manifestPath, "Codex only defaults hooks/hooks.json; root hooks.json is not loaded")
		plugin.Components = append(plugin.Components, ignored...)
		plugin.Diagnostics = append(plugin.Diagnostics, ignoredDiagnostics...)
	}

	if appValue, ok := object["apps"]; ok {
		if path, ok := appValue.(string); ok {
			clean, err := declaredRelative(path)
			if err != nil {
				plugin.Diagnostics = append(plugin.Diagnostics, ir.Diagnostic{Code: "component_path_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindApp, SourcePath: path, SourceDialect: ir.DialectCodex})
			} else {
				plugin.Components = append(plugin.Components, parseAppComponents(root, clean, manifestPath)...)
			}
		}
	} else if existingPath(root, ".app.json") {
		plugin.Components = append(plugin.Components, parseAppComponents(root, ".app.json", manifestPath)...)
	}

	if existingPath(root, "commands") {
		items, diags := parseMarkdownComponents(root, []string{"commands"}, map[string]bool{".md": true}, ir.KindCommand, ir.DialectCodex, manifestPath, ir.MappingAdapted, identity.Name)
		for i := range items {
			info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(items[i].SourcePath)))
			if strings.TrimSpace(items[i].Description) == "" || statErr != nil || info.Size() > 4000 {
				items[i].Mapping, items[i].Validation = ir.MappingInvalid, "invalid"
				items[i].Diagnostics = append(items[i].Diagnostics, componentDiagnostic("codex_command_not_migratable", "Codex legacy Command migration requires description frontmatter and a file no larger than 4000 bytes", "error", items[i]))
			}
		}
		plugin.Components, plugin.Diagnostics = append(plugin.Components, items...), append(plugin.Diagnostics, diags...)
	}
	if existingPath(root, "agents") {
		ignored, ignoredDiagnostics := ignoredFiles(root, "agents", map[string]bool{".md": true}, ir.KindAgent, ir.DialectCodex, manifestPath, "Codex plugin loader does not discover agents/")
		plugin.Components = append(plugin.Components, ignored...)
		plugin.Diagnostics = append(plugin.Diagnostics, ignoredDiagnostics...)
	}
	if err := appendSetupDeclarations(root, object, manifestPath, ir.DialectCodex, &plugin); err != nil {
		return ir.NormalizedPlugin{}, err
	}
	_ = options
	return plugin, nil
}

func jsonPaths(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		var out []string
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func parseAppComponents(root, relative, manifestPath string) []ir.Component {
	raw, err := safeRead(root, relative)
	if err != nil {
		return []ir.Component{{
			LogicalID: ir.LogicalID(ir.KindApp, relative, "invalid"), Kind: ir.KindApp, SourceDialect: ir.DialectCodex,
			SourceManifestPath: manifestPath, SourcePath: relative, Mapping: ir.MappingInvalid, Validation: "invalid",
			Diagnostics: []ir.Diagnostic{{Code: "plugin_app_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindApp, SourcePath: relative, SourceDialect: ir.DialectCodex}},
		}}
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return []ir.Component{{
			LogicalID: ir.LogicalID(ir.KindApp, relative, "invalid"), Kind: ir.KindApp, SourceDialect: ir.DialectCodex,
			SourceManifestPath: manifestPath, SourcePath: relative, Mapping: ir.MappingInvalid, Validation: "invalid",
			Diagnostics: []ir.Diagnostic{{Code: "plugin_app_invalid", Message: err.Error(), Severity: "error", Phase: "parse", ComponentKind: ir.KindApp, SourcePath: relative, SourceDialect: ir.DialectCodex}},
		}}
	}
	entries := map[string]any{"app": value}
	if object, ok := value.(map[string]any); ok {
		entries = object
		// Codex .app.json files use an `apps` envelope whose children are the
		// actual named App declarations. Keep accepting the older direct object
		// shape, but never turn the envelope itself into a synthetic App.
		if appEntries, ok := object["apps"].(map[string]any); ok {
			entries = appEntries
		}
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	components := make([]ir.Component, 0, len(keys))
	for _, key := range keys {
		entryRaw, _ := json.Marshal(entries[key])
		connectorID := ""
		if object, ok := entries[key].(map[string]any); ok {
			connectorID = firstNonEmpty(stringField(object, "connector_id"), stringField(object, "connectorId"), stringField(object, "id"))
		}
		components = append(components, ir.Component{
			LogicalID: ir.LogicalID(ir.KindApp, relative, key), Kind: ir.KindApp,
			DeclaredName: key, DerivedName: key, DisplayName: key,
			SourceDialect: ir.DialectCodex, SourceManifestPath: manifestPath, SourcePath: relative,
			SourceDigest: digestBytes(entryRaw), Mapping: ir.MappingRecognizedUnsupported, Validation: "valid",
			CanonicalProvenance: string(ir.DialectCodex), App: &ir.AppSpec{ConnectorID: connectorID, Raw: entryRaw},
		})
	}
	return components
}

var _ = fmt.Sprintf
