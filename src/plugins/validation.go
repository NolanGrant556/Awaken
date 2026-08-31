package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"awaken/src/plugins/dialect"
	"awaken/src/plugins/ir"
	"awaken/src/skills"
)

// Validate materializes a source in an isolated staging directory and applies
// the same package checks used by installation and runtime loading without
// changing plugins.json, the installed package tree, or the live runtimes.
func (s *Store) Validate(ctx context.Context, source Source) (ValidationResult, error) {
	if s == nil {
		return ValidationResult{}, ErrHomeRequired
	}
	if err := s.Available(); err != nil {
		return ValidationResult{}, err
	}
	if err := validateSource(source); err != nil {
		return ValidationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ValidationResult{}, err
	}

	staging, err := os.MkdirTemp(s.stagingRoot, "validate-*")
	if err != nil {
		return ValidationResult{}, fmt.Errorf("create validation staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := materializeSource(source, staging); err != nil {
		return ValidationResult{}, err
	}
	resolved, err := resolvePackage(staging, ResolveOptions{
		DialectHint: source.DialectHint, SelectedDialect: source.SelectedDialect,
		AllowManifestless: source.DialectHint == ir.DialectClaudeCode || source.SelectedDialect == ir.DialectClaudeCode,
	})
	if err != nil {
		return ValidationResult{}, err
	}
	if resolved.RequiresSelection {
		return ValidationResult{
			Valid: false, Source: source, Candidates: resolved.Candidates, RequiresSelection: true,
			Diagnostics: []Diagnostic{errorDiag("manifest_selection_required", "multiple valid plugin manifests were found; select a dialect")},
		}, nil
	}
	if resolved.Candidate.Manifestless && source.Kind == SourceLocalPath {
		if info, statErr := os.Stat(source.Path); statErr == nil && info.IsDir() {
			resolved.Candidate.IdentityHint = filepath.Base(filepath.Clean(source.Path))
		}
	}

	validationData, err := os.MkdirTemp(s.stagingRoot, "validate-data-*")
	if err != nil {
		return ValidationResult{}, fmt.Errorf("create validation plugin data directory: %w", err)
	}
	defer os.RemoveAll(validationData)
	return s.validateResolvedPackage(resolved, validationData, Record{Enabled: true, Source: source})
}

// validatePackageTree applies the parser and component failure-isolation
// contract to an already-materialized package tree. Callers are responsible
// for ensuring the tree is a stable snapshot and the data directory is
// controlled by the appropriate lifecycle.
func (s *Store) validatePackageTree(pluginRoot, dataDir string, rec Record) (ValidationResult, error) {
	resolved, err := resolvePackage(pluginRoot, ResolveOptions{
		DialectHint: rec.Source.DialectHint, SelectedDialect: rec.Source.SelectedDialect,
		AllowManifestless: rec.Source.DialectHint == ir.DialectClaudeCode || rec.Source.SelectedDialect == ir.DialectClaudeCode,
	})
	if err != nil {
		return ValidationResult{}, err
	}
	if resolved.RequiresSelection {
		return ValidationResult{Valid: false, Source: rec.Source, Candidates: resolved.Candidates, RequiresSelection: true}, nil
	}
	return s.validateResolvedPackage(resolved, dataDir, rec)
}

func (s *Store) validateResolvedPackage(resolved ResolvedPackage, dataDir string, rec Record) (ValidationResult, error) {
	digest, err := digestTree(resolved.Root)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("compute plugin digest: %w", err)
	}
	normalized, err := dialect.NewDefaultRegistry().Parse(resolved.Root, resolved.Candidate, dialect.ParseOptions{PluginDigest: digest})
	if err != nil {
		return ValidationResult{}, err
	}
	normalized.Layout.Secondary = make([]string, 0, len(resolved.Secondary))
	for _, secondary := range resolved.Secondary {
		normalized.Layout.Secondary = append(normalized.Layout.Secondary, secondary.ManifestPath)
	}
	parsedManifest := Manifest{
		Name: normalized.Identity.Name, Version: normalized.Identity.Version, Description: normalized.Identity.Description,
		Homepage: normalized.Identity.Homepage, Repository: normalized.Identity.Repository, License: normalized.Identity.License,
		Keywords: append([]string(nil), normalized.Identity.Keywords...),
	}
	if normalized.Identity.Author != nil {
		parsedManifest.Author = &Author{Name: normalized.Identity.Author.Name, Email: normalized.Identity.Author.Email, URL: normalized.Identity.Author.URL}
	}
	if normalized.Layout.Dialect == ir.DialectAgentPlugins {
		parsedManifest.Schema = SchemaPluginV1
	}

	diagnostics := append([]Diagnostic(nil), normalized.Diagnostics...)
	diagnosticKeys := make(map[string]bool, len(diagnostics))
	for _, diagnostic := range diagnostics {
		diagnosticKeys[diagnostic.Code+"\x00"+diagnostic.ComponentID+"\x00"+diagnostic.SourcePath+"\x00"+diagnostic.Message] = true
	}
	for _, component := range normalized.Components {
		for _, diagnostic := range component.Diagnostics {
			if diagnostic.ComponentID == "" {
				diagnostic.ComponentID = component.LogicalID
			}
			if diagnostic.ComponentKind == "" {
				diagnostic.ComponentKind = component.Kind
			}
			key := diagnostic.Code + "\x00" + diagnostic.ComponentID + "\x00" + diagnostic.SourcePath + "\x00" + diagnostic.Message
			if diagnosticKeys[key] {
				continue
			}
			diagnosticKeys[key] = true
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	loaded := &InstalledPlugin{}
	if normalized.Layout.Dialect == ir.DialectAgentPlugins {
		loaded = LoadPackage(resolved.Root, dataDir, rec)
		diagnostics = append(diagnostics, loaded.Diagnostics...)
	}
	skillCount := 0
	if normalized.Layout.Dialect == ir.DialectAgentPlugins && loaded.SkillRoot != "" {
		defs := skills.LoadDir(loaded.SkillRoot, skills.SourcePlugin)
		keys := make([]string, 0, len(defs))
		for key := range defs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			def := defs[key]
			if def == nil {
				continue
			}
			switch def.State {
			case skills.StateLoaded:
				skillCount++
			case skills.StateInvalid:
				message := def.UnavailableReason
				if message == "" {
					message = "invalid Skill package"
				}
				// Agent Plugins isolates one invalid Skill from its siblings and
				// independent MCP components. Report the skipped component without
				// turning it into a package-fatal installation error.
				diagnostics = append(diagnostics, warning("plugin_skill_invalid", def.Name+": "+message))
			case skills.StateShadowed:
				diagnostics = append(diagnostics, warning("plugin_skill_shadowed", def.Name+" is shadowed inside the plugin package"))
			}
		}
	}
	serverCount := 0
	if normalized.Layout.Dialect == ir.DialectAgentPlugins {
		for _, server := range loaded.MCPServers {
			if server.Invalid == nil {
				serverCount++
			}
		}
	} else {
		for _, component := range normalized.Components {
			if component.Validation == "invalid" || component.Mapping == ir.MappingIgnoredByDialect {
				continue
			}
			switch component.Kind {
			case ir.KindSkill:
				skillCount++
			case ir.KindMCPServer:
				serverCount++
			}
		}
	}
	if skillCount == 0 && serverCount == 0 {
		diagnostics = append(diagnostics, warning("plugin_no_components", "plugin contains no loadable Skills or MCP servers"))
	}
	valid := true
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" && diagnostic.ComponentID == "" && validationDiagnosticBlocksInstall(diagnostic.Code) {
			valid = false
			break
		}
	}
	totals, requiredVariables, intents := summarizeComponents(normalized.Components)
	return ValidationResult{
		Valid: valid, Source: rec.Source, Manifest: parsedManifest,
		Name: normalized.Identity.Name, Version: normalized.Identity.Version, Description: normalized.Identity.Description,
		Digest: digest, TargetRoot: filepath.Join(s.packagesRoot, normalized.Identity.Name),
		SkillCount: skillCount, ServerCount: serverCount, Dialect: normalized.Layout.Dialect,
		ManifestPath: normalized.Layout.ManifestPath, Components: normalized.Components, Setup: normalized.Setup, ComponentTotals: totals,
		Candidates: resolved.Candidates, SelectedDialect: normalized.Layout.Dialect,
		RequiredVariables: requiredVariables, ProcessIntents: intents, Diagnostics: diagnostics, Normalized: normalized,
	}, nil
}

func summarizeComponents(components []ir.Component) (ComponentTotals, []string, []ProcessIntent) {
	var totals ComponentTotals
	var required []string
	var intents []ProcessIntent
	for _, component := range components {
		totals.Total++
		switch component.Mapping {
		case ir.MappingNative, ir.MappingAdapted:
			totals.Runnable++
		case ir.MappingRecognizedUnsupported:
			totals.Unsupported++
		case ir.MappingIgnoredByDialect:
			totals.Ignored++
		case ir.MappingInvalid:
			totals.Invalid++
		}
		if component.Variable != nil && component.Variable.Required {
			required = append(required, component.Variable.Key)
			totals.NeedsConfig++
		}
		if component.MCP != nil {
			if component.MCP.Transport == "stdio" {
				intents = append(intents, ProcessIntent{ComponentID: component.LogicalID, Kind: "local_process", Summary: component.MCP.Command})
			} else if component.MCP.URL != "" {
				intents = append(intents, ProcessIntent{ComponentID: component.LogicalID, Kind: "network", Summary: component.MCP.URL})
			}
		}
		if component.Hook != nil && component.Hook.Command != "" {
			intents = append(intents, ProcessIntent{ComponentID: component.LogicalID, Kind: "local_process", Summary: component.Hook.Command})
		}
	}
	sort.Strings(required)
	return totals, required, intents
}

func validationDiagnosticBlocksInstall(code string) bool {
	switch code {
	case "plugin_skills_invalid", "plugin_skill_invalid", "plugin_mcp_invalid", "plugin_mcp_server_invalid", "plugin_mcp_server_unsupported":
		return false
	default:
		return true
	}
}
