package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PermissionBehavior string

const (
	PermissionAllow PermissionBehavior = "allow"
	PermissionAsk   PermissionBehavior = "ask"
	PermissionDeny  PermissionBehavior = "deny"
)

type PermissionDecision struct {
	Behavior        PermissionBehavior
	Reason          string
	ApprovalRequest ApprovalRequest
}

type PermissionContext struct {
	WorkspaceRoot string
	WritableRoots []string
}

type permissionContextKey struct{}

type PermissionCheckingTool interface {
	CheckPermission(ctx context.Context, input string, permissionContext PermissionContext) PermissionDecision
}

func AllowPermission() PermissionDecision {
	return PermissionDecision{Behavior: PermissionAllow}
}

func AskPermission(reason string, request ApprovalRequest) PermissionDecision {
	return PermissionDecision{
		Behavior:        PermissionAsk,
		Reason:          reason,
		ApprovalRequest: request,
	}
}

func DenyPermission(reason string) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionDeny,
		Reason:   reason,
	}
}

func NewPermissionContext(workspaceRoot string, writableRoots []string) PermissionContext {
	return PermissionContext{
		WorkspaceRoot: workspaceRoot,
		WritableRoots: writableRoots,
	}.Normalize()
}

func (c PermissionContext) Normalize() PermissionContext {
	root, ok := absoluteCleanRoot(c.WorkspaceRoot)
	if !ok {
		root, _ = absoluteCleanRoot(".")
	}

	writableRoots := make([]string, 0, len(c.WritableRoots)+1)
	for _, candidate := range c.WritableRoots {
		if root, ok := AbsoluteCleanPath(candidate, root); ok {
			writableRoots = append(writableRoots, root)
		}
	}
	if len(writableRoots) == 0 && root != "" {
		writableRoots = append(writableRoots, root)
	}

	return PermissionContext{
		WorkspaceRoot: root,
		WritableRoots: dedupeStrings(writableRoots),
	}
}

func (c PermissionContext) IsWithinWorkspace(path string) bool {
	normalized := c.Normalize()
	return PathWithinRoot(normalized.resolvePath(path), normalized.WorkspaceRoot)
}

func (c PermissionContext) IsWithinWritableRoots(path string) bool {
	normalized := c.Normalize()
	for _, root := range normalized.WritableRoots {
		if PathWithinRoot(normalized.resolvePath(path), root) {
			return true
		}
	}
	return false
}

// IsWithinWritableRootsResolved resolves symlinks on both the path and the
// writable roots before checking boundary. It returns false if the path cannot
// be resolved, treating unresolvable paths as unsafe.
func (c PermissionContext) IsWithinWritableRootsResolved(path string) bool {
	normalized := c.Normalize()
	resolved := normalized.resolvePath(path)
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return false
	}
	for _, root := range normalized.WritableRoots {
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if PathWithinRoot(realPath, realRoot) {
			return true
		}
	}
	return false
}

func (c PermissionContext) WorkspaceRootOrCWD() string {
	normalized := c.Normalize()
	return normalized.WorkspaceRoot
}

func (c PermissionContext) ResolvePath(path string) string {
	return c.Normalize().resolvePath(path)
}

// IsWithinWorkspaceResolved resolves symlinks before checking workspace boundary.
// It returns false if the path cannot be resolved, treating unresolvable paths as unsafe.
func (c PermissionContext) IsWithinWorkspaceResolved(path string) bool {
	normalized := c.Normalize()
	resolved := normalized.resolvePath(path)
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(normalized.WorkspaceRoot)
	if err != nil {
		return false
	}
	return PathWithinRoot(realPath, realRoot)
}

// ResolvePathChecked returns the cleaned path if its symlink-resolved target is within the workspace,
// otherwise returns an error describing the boundary violation (including symlink escapes).
func (c PermissionContext) ResolvePathChecked(path string) (string, error) {
	normalized := c.Normalize()
	resolved := normalized.resolvePath(path)
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
	}
	realRoot, err := filepath.EvalSymlinks(normalized.WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("cannot resolve workspace root %q: %w", normalized.WorkspaceRoot, err)
	}
	if !PathWithinRoot(realPath, realRoot) {
		return "", fmt.Errorf("path %q resolves to %q which is outside workspace", path, realPath)
	}
	return resolved, nil
}

func ContextWithPermissionContext(ctx context.Context, permissionContext PermissionContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, permissionContextKey{}, permissionContext.Normalize())
}

func PermissionContextFromContext(ctx context.Context) PermissionContext {
	if ctx != nil {
		if permissionContext, ok := ctx.Value(permissionContextKey{}).(PermissionContext); ok {
			return permissionContext.Normalize()
		}
	}
	return NewPermissionContext("", nil)
}

// HasPermissionContext reports whether a PermissionContext was explicitly
// attached to the context. When false, callers may choose to skip workspace
// boundary enforcement for backward-compatible direct tool execution.
func HasPermissionContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(permissionContextKey{}).(PermissionContext)
	return ok
}

func (c PermissionContext) resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return c.WorkspaceRoot
	}
	path = expandTilde(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.WorkspaceRoot, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func expandTilde(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func absoluteCleanRoot(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	return AbsoluteCleanPath(path, "")
}

func AbsoluteCleanPath(path string, base string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	if !filepath.IsAbs(path) && strings.TrimSpace(base) != "" {
		path = filepath.Join(base, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(abs), true
}

func PathWithinRoot(path string, root string) bool {
	absPath, ok := AbsoluteCleanPath(path, "")
	if !ok {
		return false
	}
	absRoot, ok := AbsoluteCleanPath(root, "")
	if !ok {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		deduped = append(deduped, value)
	}
	return deduped
}
