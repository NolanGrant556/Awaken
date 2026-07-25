// Package searchroots resolves which directories a read-only search tool should
// cover. A project can own several folders, so a search that does not name an
// explicit path has to look at every readable root instead of only the thread
// cwd. Tools keep their single-root output shape whenever exactly one directory
// is resolved, and switch to absolute paths once results from more than one root
// are merged, because relative paths from different roots collide.
package searchroots

import (
	"os"
	"path/filepath"
	"strings"

	"awaken/src/tool"
)

// Resolve returns the ordered directories to search. When explicitPath is set
// the caller asked for one specific place and the result is exactly that path.
// Otherwise the thread cwd comes first, followed by the remaining readable
// roots in scope order. Nested and duplicate roots are dropped so a file is
// never reported twice.
func Resolve(permissionContext tool.PermissionContext, cwd, explicitPath string) ([]string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		resolved, err := permissionContext.ResolveReadablePathChecked(explicitPath)
		if err != nil {
			return nil, err
		}
		return []string{resolved}, nil
	}

	primary, err := permissionContext.ResolveReadablePathChecked(cwd)
	if err != nil {
		return nil, err
	}

	roots := []string{primary}
	for _, candidate := range permissionContext.ReadableRoots {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		resolved, err := permissionContext.ResolveReadablePathChecked(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
			continue
		}
		if containedInAny(resolved, roots) {
			continue
		}
		roots = dropContainedIn(roots, resolved)
		roots = append(roots, resolved)
	}
	return roots, nil
}

// DisplayPath renders a match for the caller. With a single root the tool keeps
// its established root-relative output; with several roots only absolute paths
// stay unambiguous.
func DisplayPath(root, path string, multiRoot bool) string {
	if multiRoot {
		if filepath.IsAbs(path) {
			return filepath.ToSlash(path)
		}
		return filepath.ToSlash(filepath.Join(root, path))
	}
	if filepath.IsAbs(path) {
		if relative, err := filepath.Rel(root, path); err == nil && !escapesRoot(relative) {
			return filepath.ToSlash(relative)
		}
	}
	return filepath.ToSlash(path)
}

func containedInAny(path string, roots []string) bool {
	for _, root := range roots {
		if isSameOrBelow(path, root) {
			return true
		}
	}
	return false
}

func dropContainedIn(roots []string, parent string) []string {
	kept := roots[:0:len(roots)]
	for _, root := range roots {
		if isSameOrBelow(root, parent) {
			continue
		}
		kept = append(kept, root)
	}
	return kept
}

func isSameOrBelow(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return !escapesRoot(relative)
}

func escapesRoot(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
