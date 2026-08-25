package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
	if err := materializeSource(ctx, source, staging); err != nil {
		return ValidationResult{}, err
	}
	pluginRoot, err := locatePluginRoot(staging)
	if err != nil {
		return ValidationResult{}, err
	}
	parsed, err := loadManifestFile(pluginRoot)
	if err != nil {
		return ValidationResult{}, err
	}
	digest, err := digestTree(pluginRoot)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("compute plugin digest: %w", err)
	}

	validationData, err := os.MkdirTemp(s.stagingRoot, "validate-data-*")
	if err != nil {
		return ValidationResult{}, fmt.Errorf("create validation plugin data directory: %w", err)
	}
	defer os.RemoveAll(validationData)
	loaded := LoadPackage(pluginRoot, validationData, Record{Enabled: true, Source: source})
	diagnostics := append([]Diagnostic(nil), loaded.Diagnostics...)
	skillCount := 0
	if loaded.SkillRoot != "" {
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
				diagnostics = append(diagnostics, errorDiag("plugin_skill_invalid", def.Name+": "+message))
			case skills.StateShadowed:
				diagnostics = append(diagnostics, warning("plugin_skill_shadowed", def.Name+" is shadowed inside the plugin package"))
			}
		}
	}
	serverCount := 0
	for _, server := range loaded.MCPServers {
		if server.Invalid == nil {
			serverCount++
		}
	}
	if skillCount == 0 && serverCount == 0 {
		diagnostics = append(diagnostics, warning("plugin_no_components", "plugin contains no loadable Skills or MCP servers"))
	}
	valid := true
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			valid = false
			break
		}
	}
	return ValidationResult{
		Valid: valid, Source: source, Manifest: parsed.Manifest,
		Name: parsed.Manifest.Name, Version: parsed.Manifest.Version, Description: parsed.Manifest.Description,
		Digest: digest, TargetRoot: filepath.Join(s.packagesRoot, parsed.Manifest.Name),
		SkillCount: skillCount, ServerCount: serverCount, Diagnostics: diagnostics,
	}, nil
}
