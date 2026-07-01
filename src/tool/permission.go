package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Decide returns a PermissionDecision based solely on ToolDefinition annotations
// and user-configured overrides — never on hard-coded tool name matches.
//
// Default rules:
//   - readOnly==true → allow (regardless of openWorld)
//   - destructive==true → ask
//   - readOnly==false && destructive==false → ask (conservative)
//   - requiresAuth==true && server/connector not connected → deny with reason "needs_auth"
func Decide(ctx context.Context, def ToolDefinition) PermissionDecision {
	return DecideWithOverride(ctx, def, nil)
}

// DecideWithOverride applies user-configured overrides on top of the default
// rules. Overrides are a map of tool name or server prefix to PermissionPolicy.
// An override key of "*" applies to all tools.
func DecideWithOverride(ctx context.Context, def ToolDefinition, overrides map[string]PermissionPolicy) PermissionDecision {
	if overrides != nil {
		// Check exact name match first
		if policy, ok := overrides[def.Name]; ok {
			return policyToDecision(policy)
		}
		// Check server prefix match (e.g., "mcp__notion__*")
		if def.Annotations.ServerName != "" {
			prefix := "mcp__" + def.Annotations.ServerName + "__*"
			if policy, ok := overrides[prefix]; ok {
				return policyToDecision(policy)
			}
		}
		// Check wildcard
		if policy, ok := overrides["*"]; ok {
			return policyToDecision(policy)
		}
	}

	// Default rules based on annotations
	if def.Annotations.ReadOnly {
			return AllowPermission()
	}

	if def.Annotations.RequiresAuth {
		return DenyPermission("needs_auth")
	}

	if def.Annotations.Destructive {
		return AskPermission("destructive tool requires approval", ApprovalRequest{
			Title:    fmt.Sprintf("Approve %s", def.Name),
			RiskTone: "warning",
		})
	}

	// readOnly==false && destructive==false → conservative: ask
	return AskPermission("tool may have unknown side-effects", ApprovalRequest{
		Title:    fmt.Sprintf("Approve %s", def.Name),
		RiskTone: "warning",
	})
}

// DecideWithServerCheck is like Decide but accepts a server state callback.
// When the decision is "deny" with reason "needs_auth" and the server is
// actually connected, the decision is overridden to allow.
func DecideWithServerCheck(ctx context.Context, def ToolDefinition, serverConnected func(serverName string) bool) PermissionDecision {
	decision := DecideWithOverride(ctx, def, nil)
	if decision.Behavior == PermissionDeny && decision.Reason == "needs_auth" {
		if serverConnected != nil && def.Annotations.ServerName != "" && serverConnected(def.Annotations.ServerName) {
				return AllowPermission()
		}
	}
	return decision
}

func policyToDecision(policy PermissionPolicy) PermissionDecision {
	switch policy {
	case PolicyAlwaysAllow:
			return AllowPermission()
	case PolicyAlwaysAsk:
		return AskPermission("user policy requires approval", ApprovalRequest{
			Title:    "User policy override",
			RiskTone: "warning",
		})
	default:
		return AskPermission("unimplemented policy: "+string(policy), ApprovalRequest{Title: "Policy not yet supported", RiskTone: "warning"})
	}
}

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
