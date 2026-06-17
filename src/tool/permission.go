package tool

import (
	"context"
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
		if root, ok := absoluteCleanPath(candidate, root); ok {
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
	return pathWithinRoot(normalized.resolvePath(path), normalized.WorkspaceRoot)
}

func (c PermissionContext) IsWithinWritableRoots(path string) bool {
	normalized := c.Normalize()
	for _, root := range normalized.WritableRoots {
		if pathWithinRoot(normalized.resolvePath(path), root) {
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

func (c PermissionContext) resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return c.WorkspaceRoot
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.WorkspaceRoot, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func absoluteCleanRoot(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	return absoluteCleanPath(path, "")
}

func absoluteCleanPath(path string, base string) (string, bool) {
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

func pathWithinRoot(path string, root string) bool {
	absPath, ok := absoluteCleanPath(path, "")
	if !ok {
		return false
	}
	absRoot, ok := absoluteCleanPath(root, "")
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
