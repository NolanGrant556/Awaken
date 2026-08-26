package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const MaxSkillResourceInventoryEntries = 512

// SkillResourceEntry is bounded metadata for one file-system entry inside a
// local Skill package. It never contains file contents or an absolute path.
type SkillResourceEntry struct {
	Path      string
	Kind      string
	SizeBytes int64
}

type SkillResourceInventory struct {
	Entries   []SkillResourceEntry
	Truncated bool
}

// ListSkillPackageResources revalidates the discovery anchors, then lists a
// bounded, deterministic package-relative tree without reading file contents.
func ListSkillPackageResources(def *SkillDefinition) (SkillResourceInventory, error) {
	return ListSkillPackageResourcesAt(def, "")
}

// ListSkillPackageResourcesAt lists one package-relative subtree. Filtering is
// applied before the entry budget so a caller can inspect a later subtree even
// when the full package inventory exceeds the per-call limit.
func ListSkillPackageResourcesAt(def *SkillDefinition, rawPrefix string) (SkillResourceInventory, error) {
	if def == nil || def.Source == SourceMcp {
		return SkillResourceInventory{}, fmt.Errorf("%w: no local Skill package", ErrSkillPreviewUnavailable)
	}
	prefix, err := normalizeSkillResourcePrefix(rawPrefix)
	if err != nil {
		return SkillResourceInventory{}, err
	}
	anchor := strings.TrimSpace(def.SourceRoot)
	rootDir := strings.TrimSpace(def.RootDir)
	skillPath := strings.TrimSpace(def.SkillFilePath)
	trustedRoot := strings.TrimSpace(def.ResolvedRootDir)
	if anchor == "" || rootDir == "" || skillPath == "" || trustedRoot == "" {
		return SkillResourceInventory{}, fmt.Errorf("%w: incomplete discovery identity", ErrSkillPreviewUnavailable)
	}

	resolvedAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return SkillResourceInventory{}, skillPreviewResolveError("source root", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return SkillResourceInventory{}, skillPreviewResolveError("package root", err)
	}
	if filepath.Clean(resolvedRoot) != filepath.Clean(trustedRoot) {
		return SkillResourceInventory{}, fmt.Errorf("%w: package root changed after discovery", ErrSkillPreviewPathDenied)
	}
	if !pathWithinRoot(resolvedRoot, resolvedAnchor) {
		return SkillResourceInventory{}, fmt.Errorf("%w: package root escapes source root", ErrSkillPreviewPathDenied)
	}

	resolvedSkillPath, err := filepath.EvalSymlinks(skillPath)
	if err != nil {
		return SkillResourceInventory{}, skillPreviewResolveError("SKILL.md", err)
	}
	if !pathWithinRoot(resolvedSkillPath, resolvedAnchor) {
		return SkillResourceInventory{}, fmt.Errorf("%w: SKILL.md escapes source root", ErrSkillPreviewPathDenied)
	}
	flatLegacy := def.Source == SourceLegacy && filepath.Clean(resolvedRoot) == filepath.Clean(resolvedAnchor)
	if flatLegacy {
		info, err := os.Stat(resolvedSkillPath)
		if err != nil || !info.Mode().IsRegular() {
			return SkillResourceInventory{}, fmt.Errorf("%w: legacy entrypoint is unavailable", ErrSkillPreviewUnavailable)
		}
		entry := SkillResourceEntry{
			Path: filepath.Base(resolvedSkillPath), Kind: "file", SizeBytes: info.Size(),
		}
		if prefix != "" && prefix != entry.Path {
			return SkillResourceInventory{Entries: []SkillResourceEntry{}}, nil
		}
		return SkillResourceInventory{Entries: []SkillResourceEntry{entry}}, nil
	}

	packageRel, err := filepath.Rel(resolvedAnchor, resolvedRoot)
	if err != nil || filepath.IsAbs(packageRel) || packageRel == "." || strings.Contains(filepath.ToSlash(packageRel), "/") {
		return SkillResourceInventory{}, fmt.Errorf("%w: package is not one directory below its source root", ErrSkillPreviewPathDenied)
	}
	if !pathWithinRoot(resolvedSkillPath, resolvedRoot) {
		return SkillResourceInventory{}, fmt.Errorf("%w: SKILL.md escapes package root", ErrSkillPreviewPathDenied)
	}

	root, err := os.OpenRoot(resolvedRoot)
	if err != nil {
		return SkillResourceInventory{}, fmt.Errorf("%w: open package root: %v", ErrSkillPreviewPathDenied, err)
	}
	defer root.Close()

	inventory := SkillResourceInventory{Entries: make([]SkillResourceEntry, 0, 32)}
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		if prefix != "" && path != prefix && !strings.HasPrefix(path, prefix+"/") {
			return nil
		}
		if len(inventory.Entries) >= MaxSkillResourceInventoryEntries {
			inventory.Truncated = true
			return fs.SkipAll
		}
		kind := "other"
		size := int64(0)
		switch {
		case entry.IsDir():
			kind = "directory"
		case entry.Type()&os.ModeSymlink != 0:
			kind = "symlink"
		default:
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			if info.Mode().IsRegular() {
				kind = "file"
				size = info.Size()
			}
		}
		inventory.Entries = append(inventory.Entries, SkillResourceEntry{
			Path: filepath.ToSlash(path), Kind: kind, SizeBytes: size,
		})
		return nil
	})
	if err != nil {
		return SkillResourceInventory{}, fmt.Errorf("%w: list package resources: %v", ErrSkillPreviewUnavailable, err)
	}
	return inventory, nil
}

func normalizeSkillResourcePrefix(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || raw == "." {
		return "", nil
	}
	clean := path.Clean(raw)
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: prefix must stay inside the Skill package", ErrSkillPreviewPathDenied)
	}
	return clean, nil
}
