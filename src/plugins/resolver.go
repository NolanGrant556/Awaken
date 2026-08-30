package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/plugins/dialect"
	"awaken/src/plugins/ir"
)

type ResolveOptions struct {
	DialectHint       ir.Dialect
	SelectedDialect   ir.Dialect
	AllowManifestless bool
}

type ResolvedPackage struct {
	Root              string                 `json:"-"`
	Candidate         ir.ManifestCandidate   `json:"candidate"`
	Candidates        []ir.ManifestCandidate `json:"candidates"`
	RequiresSelection bool                   `json:"requiresSelection"`
	Secondary         []ir.ManifestCandidate `json:"secondaryReferences,omitempty"`
}

var manifestLocations = []struct {
	Dialect ir.Dialect
	Path    string
}{
	{Dialect: ir.DialectAgentPlugins, Path: "plugin.json"},
	{Dialect: ir.DialectCodex, Path: ".codex-plugin/plugin.json"},
	{Dialect: ir.DialectClaudeCode, Path: ".claude-plugin/plugin.json"},
	{Dialect: ir.DialectCursor, Path: ".cursor-plugin/plugin.json"},
}

func resolvePackage(staging string, options ResolveOptions) (ResolvedPackage, error) {
	root, candidates, err := enumeratePackageRoot(staging)
	if err != nil {
		return ResolvedPackage{}, err
	}
	if len(candidates) == 0 {
		if claudeManifestlessShape(root) {
			candidates = append(candidates, ir.ManifestCandidate{
				Dialect: ir.DialectClaudeCode, Manifestless: true, IdentityHint: filepath.Base(root), Valid: true,
			})
		} else if isRegularFile(filepath.Join(root, "SKILL.md")) {
			return ResolvedPackage{}, fmt.Errorf("%w: use the Skill installer for this package", ErrSkillPackageSelected)
		}
	}
	if len(candidates) == 0 {
		return ResolvedPackage{}, fmt.Errorf("%w: no supported plugin manifest was found at the package root", ErrManifestInvalid)
	}

	registry := dialect.NewDefaultRegistry()
	for index := range candidates {
		plugin, parseErr := registry.Parse(root, candidates[index], dialect.ParseOptions{})
		if parseErr != nil {
			candidates[index].Valid = false
			candidates[index].Reason = parseErr.Error()
			continue
		}
		candidates[index].Valid = true
		candidates[index].IdentityHint = plugin.Identity.Name
		candidates[index].VersionHint = plugin.Identity.Version
	}
	valid := make([]ir.ManifestCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Valid {
			valid = append(valid, candidate)
		}
	}
	if len(valid) == 0 {
		return ResolvedPackage{Root: root, Candidates: candidates}, fmt.Errorf("%w: supported manifests were found but none parsed successfully", ErrManifestInvalid)
	}

	selected, selectedOK := selectCandidate(valid, options.SelectedDialect, options.DialectHint)
	if !selectedOK {
		return ResolvedPackage{Root: root, Candidates: candidates, RequiresSelection: true}, nil
	}
	resolved := ResolvedPackage{Root: root, Candidate: selected, Candidates: candidates}
	for _, candidate := range valid {
		if candidate.Dialect == selected.Dialect && candidate.ManifestPath == selected.ManifestPath {
			continue
		}
		if candidate.IdentityHint != "" && candidate.IdentityHint == selected.IdentityHint {
			resolved.Secondary = append(resolved.Secondary, candidate)
		}
	}
	return resolved, nil
}

func enumeratePackageRoot(staging string) (string, []ir.ManifestCandidate, error) {
	if candidates := manifestsAt(staging); len(candidates) > 0 {
		return staging, candidates, nil
	}
	// Keep a standalone root Skill at the archive root so resolvePackage can
	// return the typed cross-installer result instead of treating one of its
	// resource directories as an archive wrapper.
	if isRegularFile(filepath.Join(staging, "SKILL.md")) {
		return staging, nil, nil
	}
	// A manifestless Claude package commonly consists of one top-level
	// component directory (for example commands/). Treat that directory as
	// package content instead of mistaking it for an archive wrapper.
	if claudeManifestlessShape(staging) {
		return staging, nil, nil
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return "", nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.Name() == "__MACOSX" || strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
			continue
		}
		dirs = append(dirs, filepath.Join(staging, entry.Name()))
	}
	if len(dirs) == 1 {
		return dirs[0], manifestsAt(dirs[0]), nil
	}
	return staging, nil, nil
}

func manifestsAt(root string) []ir.ManifestCandidate {
	var candidates []ir.ManifestCandidate
	for _, location := range manifestLocations {
		if isRegularFile(filepath.Join(root, filepath.FromSlash(location.Path))) {
			candidates = append(candidates, ir.ManifestCandidate{Dialect: location.Dialect, ManifestPath: location.Path})
		}
	}
	return candidates
}

func selectCandidate(candidates []ir.ManifestCandidate, selected, hint ir.Dialect) (ir.ManifestCandidate, bool) {
	for _, requested := range []ir.Dialect{selected, hint} {
		if requested == "" {
			continue
		}
		for _, candidate := range candidates {
			if candidate.Dialect == requested {
				return candidate, true
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	identities := map[string]bool{}
	for _, candidate := range candidates {
		identities[candidate.IdentityHint] = true
	}
	if len(identities) == 1 {
		versions := map[string]bool{}
		for _, candidate := range candidates {
			if version := strings.TrimSpace(candidate.VersionHint); version != "" {
				versions[version] = true
			}
		}
		if len(versions) > 1 {
			return ir.ManifestCandidate{}, false
		}
		// The portable root is canonical when the user/source did not prefer a
		// vendor overlay with the same identity.
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].Dialect == ir.DialectAgentPlugins
		})
		return candidates[0], true
	}
	return ir.ManifestCandidate{}, false
}

func claudeManifestlessShape(root string) bool {
	for _, relative := range []string{"skills", "commands", "agents", "hooks/hooks.json", ".mcp.json"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			return true
		}
	}
	return false
}
