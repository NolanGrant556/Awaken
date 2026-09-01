package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	// EnvBuiltinPluginDir is set by Desktop to the product-owned bundled
	// plugin directory. Login-shell configuration is not allowed to replace it.
	EnvBuiltinPluginDir = "AWAKEN_BUNDLED_PLUGIN_DIR"
	builtinMetadataPath = ".awaken/builtin.json"
)

var builtinBrandIconPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

type builtinMetadataFile struct {
	Version     int    `json:"version"`
	DisplayName string `json:"displayName"`
	BrandIcon   string `json:"brandIcon"`
	Order       int    `json:"order"`
}

// SyncBuiltinPlugins validates and materializes each release-owned plugin
// package into the normal Plugin Store. First installation is disabled; later
// releases preserve the user's enabled state, setup values, and component
// overrides while replacing only the immutable package tree.
func (s *Store) SyncBuiltinPlugins(ctx context.Context, root string) ([]*InstalledPlugin, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, fmt.Errorf("built-in plugin root is required")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect built-in plugin root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("built-in plugin root must be a real directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read built-in plugin root: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	plugins := make([]*InstalledPlugin, 0, len(entries))
	var syncErrors []error
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		packageRoot := filepath.Join(root, entry.Name())
		presentation, metadataErr := readBuiltinPresentation(packageRoot)
		if metadataErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("%s: %w", entry.Name(), metadataErr))
			continue
		}
		result, installErr := s.Install(ctx, InstallRequest{
			Source:  Source{Kind: SourceLocalPath, Path: packageRoot},
			Replace: true,
			Builtin: &presentation,
		})
		if installErr != nil {
			syncErrors = append(syncErrors, fmt.Errorf("%s: %w", entry.Name(), installErr))
			continue
		}
		plugins = append(plugins, result.Plugin)
	}
	return plugins, errors.Join(syncErrors...)
}

func readBuiltinPresentation(packageRoot string) (BuiltinPresentation, error) {
	metadataFile := filepath.Join(packageRoot, filepath.FromSlash(builtinMetadataPath))
	if err := rejectSymlink(metadataFile); err != nil {
		return BuiltinPresentation{}, err
	}
	if !isRegularFile(metadataFile) {
		return BuiltinPresentation{}, fmt.Errorf("%s is missing or not a regular file", builtinMetadataPath)
	}
	if err := resolvedStayWithin(packageRoot, metadataFile); err != nil {
		return BuiltinPresentation{}, fmt.Errorf("%s escapes the package root", builtinMetadataPath)
	}
	raw, err := os.ReadFile(metadataFile)
	if err != nil {
		return BuiltinPresentation{}, fmt.Errorf("read %s: %w", builtinMetadataPath, err)
	}
	var metadata builtinMetadataFile
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return BuiltinPresentation{}, fmt.Errorf("parse %s: %w", builtinMetadataPath, err)
	}
	metadata.DisplayName = strings.TrimSpace(metadata.DisplayName)
	metadata.BrandIcon = strings.TrimSpace(metadata.BrandIcon)
	if metadata.Version != 1 {
		return BuiltinPresentation{}, fmt.Errorf("unsupported built-in metadata version %d", metadata.Version)
	}
	if metadata.DisplayName == "" || len([]rune(metadata.DisplayName)) > 80 {
		return BuiltinPresentation{}, fmt.Errorf("built-in displayName is required and must not exceed 80 characters")
	}
	if metadata.BrandIcon != "" && !builtinBrandIconPattern.MatchString(metadata.BrandIcon) {
		return BuiltinPresentation{}, fmt.Errorf("invalid built-in brandIcon %q", metadata.BrandIcon)
	}
	if metadata.Order < 0 || metadata.Order > 10_000 {
		return BuiltinPresentation{}, fmt.Errorf("built-in order must be between 0 and 10000")
	}
	return BuiltinPresentation{
		DisplayName: metadata.DisplayName,
		BrandIcon:   metadata.BrandIcon,
		Order:       metadata.Order,
	}, nil
}
