// Package searchroots resolves which directories a read-only search tool should
// cover. A project can own several folders, so a search that does not name an
// explicit path has to look at every readable root instead of only the thread
// cwd. Tools keep their single-root output shape whenever exactly one directory
// is resolved, and switch to absolute paths once results from more than one root
// are merged, because relative paths from different roots collide.
package searchroots

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/tool"
)

const (
	ScopeModeProject      = "project"
	ScopeModeExplicitPath = "explicit_path"

	ErrorCodeProjectScopePathMismatch = "PROJECT_SCOPE_PATH_MISMATCH"
)

type Resolution struct {
	Roots         []string
	ScopeMode     string
	ScopeRelation string
	ScopeSource   string
}

type ScopeError struct {
	Code    string
	Message string
}

func (e *ScopeError) Error() string { return e.Message }

// Resolve returns the ordered directories to search. When explicitPath is set
// the caller asked for one specific place and the result is exactly that path.
// Otherwise the thread cwd comes first, followed by the remaining readable
// roots in scope order. Nested and duplicate roots are dropped so a file is
// never reported twice.
func Resolve(permissionContext tool.PermissionContext, cwd, explicitPath, scopeMode string) (Resolution, error) {
	if scopeMode == "" {
		scopeMode = ScopeModeProject
	}
	if scopeMode != ScopeModeProject && scopeMode != ScopeModeExplicitPath {
		return Resolution{}, fmt.Errorf("scope_mode must be project or explicit_path")
	}
	if scopeMode == ScopeModeExplicitPath && strings.TrimSpace(explicitPath) == "" {
		return Resolution{}, fmt.Errorf("path is required when scope_mode is explicit_path")
	}

	projectRoots, err := defaultRoots(permissionContext, cwd)
	if err != nil {
		return Resolution{}, err
	}
	if strings.TrimSpace(explicitPath) == "" {
		return Resolution{
			Roots:         projectRoots,
			ScopeMode:     ScopeModeProject,
			ScopeRelation: "inside_project",
			ScopeSource:   "project_default",
		}, nil
	}

	resolved, err := permissionContext.ResolveReadablePathChecked(explicitPath)
	if err != nil {
		return Resolution{}, err
	}
	relation := scopeRelation(resolved, projectRoots)
	if scopeMode == ScopeModeProject && relation != "inside_project" {
		return Resolution{}, &ScopeError{
			Code:    ErrorCodeProjectScopePathMismatch,
			Message: fmt.Sprintf("“本项目”只包含 Host 列出的精确 workspace_roots；%s 不属于这些 roots。请省略 path 重新调用，或仅在用户明确点名项目外位置时使用 scope_mode=explicit_path。", resolved),
		}
	}
	source := "model_explicit"
	if scopeMode == ScopeModeExplicitPath {
		source = "user_explicit"
	}
	return Resolution{
		Roots:         []string{resolved},
		ScopeMode:     scopeMode,
		ScopeRelation: relation,
		ScopeSource:   source,
	}, nil
}

func defaultRoots(permissionContext tool.PermissionContext, cwd string) ([]string, error) {
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

func scopeRelation(path string, projectRoots []string) string {
	for _, root := range projectRoots {
		if isSameOrBelow(path, root) {
			return "inside_project"
		}
	}
	for _, root := range projectRoots {
		if isSameOrBelow(root, path) {
			return "common_ancestor"
		}
	}
	return "outside_project"
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
