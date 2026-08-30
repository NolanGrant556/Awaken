package dialect

import (
	"encoding/json"
	"fmt"
	"regexp"

	"awaken/src/plugins/ir"
)

var portableNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,62}[a-z0-9])?$`)

type agentPluginsAdapter struct{}

func (agentPluginsAdapter) Dialect() ir.Dialect { return ir.DialectAgentPlugins }

func (agentPluginsAdapter) Parse(root string, candidate ir.ManifestCandidate, options ParseOptions) (ir.NormalizedPlugin, error) {
	raw, object, err := readManifest(root, "plugin.json")
	if err != nil {
		return ir.NormalizedPlugin{}, err
	}
	if stringField(object, "$schema") != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" {
		return ir.NormalizedPlugin{}, fmt.Errorf("unsupported Agent Plugins schema")
	}
	identity := identityFromManifest(object)
	if !validPortableName(identity.Name) {
		return ir.NormalizedPlugin{}, fmt.Errorf("invalid Agent Plugins name %q", identity.Name)
	}
	plugin := ir.NormalizedPlugin{
		Identity:    identity,
		Layout:      ir.PackageLayout{Dialect: ir.DialectAgentPlugins, Surface: "agent_plugins_v1", ManifestPath: "plugin.json"},
		RawManifest: raw,
	}
	if existingPath(root, "skills") {
		components, diagnostics := parseSkillComponents(root, []string{"skills"}, ir.DialectAgentPlugins, "plugin.json", ir.MappingNative)
		plugin.Components = append(plugin.Components, components...)
		plugin.Diagnostics = append(plugin.Diagnostics, diagnostics...)
	}
	if existingPath(root, "mcp.json") {
		components, diagnostics := parseMCPFileComponents(root, "mcp.json", ir.DialectAgentPlugins, "plugin.json", ir.MappingNative)
		plugin.Components = append(plugin.Components, components...)
		plugin.Diagnostics = append(plugin.Diagnostics, diagnostics...)
	}
	if extensions, ok := object["extensions"].(map[string]any); ok {
		keys := make([]string, 0, len(extensions))
		for key := range extensions {
			keys = append(keys, key)
		}
		for _, key := range keys {
			rawExtension, _ := json.Marshal(extensions[key])
			component := ir.Component{
				LogicalID: ir.LogicalID(ir.KindUnknown, "plugin.json", "extensions/"+key), Kind: ir.KindUnknown,
				DeclaredName: key, DerivedName: key, DisplayName: key,
				SourceDialect: ir.DialectAgentPlugins, SourceManifestPath: "plugin.json", SourcePath: "plugin.json",
				SourceDigest: digestBytes(rawExtension), Mapping: ir.MappingRecognizedUnsupported, Validation: "valid",
				CanonicalProvenance: string(ir.DialectAgentPlugins),
			}
			component.Diagnostics = append(component.Diagnostics, componentDiagnostic("plugin_extension_unsupported", "client extension namespace is preserved but not implemented", "info", component))
			plugin.Components = append(plugin.Components, component)
		}
	}
	_ = candidate
	_ = options
	return plugin, nil
}

func validPortableName(name string) bool {
	return len(name) >= 1 && len(name) <= 64 && portableNamePattern.MatchString(name) &&
		!regexp.MustCompile(`--|\.\.`).MatchString(name)
}
