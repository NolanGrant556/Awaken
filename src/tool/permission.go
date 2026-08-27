package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Decide returns the zero-approval decision derived from the provider
// contract. Ordinary tools default to allow; deny/unsupported is reserved for
// an explicit hard boundary such as an unrecoverable delete. Authentication
// and runtime readiness remain structured non-generic states.
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

	if def.Annotations.RequiresAuth {
		return NeedsAuthPermission("tool requires authentication")
	}
	if def.Policy == PolicyAlwaysDeny {
		return DenyPermission("tool definition explicitly disables execution")
	}
	if def.Policy == PolicyAlwaysAllow {
		return AllowPermission()
	}

	if definitionDeclaresDelete(def) && !HasCapability(def.Capabilities, CapabilityFileDelete) {
		return UnsupportedPermission("unsupported_recoverable_delete: provider does not declare a recoverable file-delete contract")
	}
	return AllowPermission()
}

func definitionDeclaresDelete(def ToolDefinition) bool {
	for _, effect := range def.SideEffects {
		if strings.EqualFold(strings.TrimSpace(effect.Kind), SideEffectDelete) {
			return true
		}
	}
	return false
}

// DecideWithServerCheck is like Decide but accepts a server state callback.
// When the decision is "needs_auth" and the server is
// actually connected, the decision is overridden to allow.
func DecideWithServerCheck(ctx context.Context, def ToolDefinition, serverConnected func(serverName string) bool) PermissionDecision {
	decision := DecideWithOverride(ctx, def, nil)
	if decision.Behavior == PermissionNeedsAuth {
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
	case PolicyAlwaysDeny:
		return DenyPermission("user policy denies execution")
	default:
		return DenyPermission("unimplemented policy: " + string(policy))
	}
}

type PermissionBehavior string

const (
	PermissionAllow        PermissionBehavior = "allow"
	PermissionDeny         PermissionBehavior = "deny"
	PermissionNeedsAuth    PermissionBehavior = "needs_auth"
	PermissionNeedsRuntime PermissionBehavior = "needs_runtime"
	PermissionUnsupported  PermissionBehavior = "unsupported"
)

type PermissionDecision struct {
	Behavior  PermissionBehavior
	Reason    string
	ErrorCode string
}

type PermissionContext struct {
	ScopeID string
	// ProjectID and SessionWorkspace are Host-owned identity facts used by
	// project-scoped configuration tools. They never grant path access.
	ProjectID        string
	SessionWorkspace bool
	WorkspaceRoot    string
	ReadableRoots    []string
	WritableRoots    []string
	OutputRoots      []string
	AttachedFiles    []ScopedFile
	MediaArtifacts   []ScopedMediaArtifact
	ProtectedRoots   []string
	// ProtectedRootExceptions are narrow carve-outs inside an otherwise
	// protected subtree. They exist because Awaken keeps managed project
	// working directories under its own business-data root, which is protected
	// as a whole. Only the listed subtrees are released, and the rules that
	// apply inside any working directory (.git, .env, credential files) still
	// hold there, so the default stays fail-closed.
	ProtectedRootExceptions []string
	TemporaryWorkRoot       string
}

type permissionContextKey struct{}

type PermissionCheckingTool interface {
	CheckPermission(ctx context.Context, input string, permissionContext PermissionContext) PermissionDecision
}

func AllowPermission() PermissionDecision {
	return PermissionDecision{Behavior: PermissionAllow}
}

func DenyPermission(reason string) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionDeny,
		Reason:   reason,
	}
}

func NeedsAuthPermission(reason string) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionNeedsAuth,
		Reason:   reason,
	}
}

func NeedsRuntimePermission(reason string) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionNeedsRuntime,
		Reason:   reason,
	}
}

func UnsupportedPermission(reason string) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionUnsupported,
		Reason:   reason,
	}
}

func UnsupportedPermissionWithCode(reason, code string) PermissionDecision {
	return PermissionDecision{
		Behavior:  PermissionUnsupported,
		Reason:    reason,
		ErrorCode: strings.TrimSpace(code),
	}
}

func NewPermissionContext(workspaceRoot string, writableRoots []string) PermissionContext {
	return PermissionContext{
		WorkspaceRoot: workspaceRoot,
		WritableRoots: writableRoots,
	}.Normalize()
}

func NewTaskScopePermissionContext(
	scopeID string,
	workspaceRoot string,
	readableRoots []string,
	writableRoots []string,
	outputRoots []string,
	attachedFiles []ScopedFile,
	protectedRoots []string,
	temporaryWorkRoot string,
) PermissionContext {
	return PermissionContext{
		ScopeID:           scopeID,
		WorkspaceRoot:     workspaceRoot,
		ReadableRoots:     readableRoots,
		WritableRoots:     writableRoots,
		OutputRoots:       outputRoots,
		AttachedFiles:     attachedFiles,
		ProtectedRoots:    protectedRoots,
		TemporaryWorkRoot: temporaryWorkRoot,
	}.Normalize()
}

func (c PermissionContext) Normalize() PermissionContext {
	root, ok := absoluteCleanRoot(c.WorkspaceRoot)
	if !ok {
		// Unreachable from a run: the runtime resolves every scope to a real
		// directory, giving a project with no folder bound its own session
		// workspace. This last resort only keeps a directly constructed context
		// usable, and it is why relative paths must never be the way a caller
		// reaches outside the project.
		root, _ = absoluteCleanRoot(".")
	}

	readableRoots := make([]string, 0, len(c.ReadableRoots)+1)
	for _, candidate := range c.ReadableRoots {
		if normalized, ok := AbsoluteCleanPath(candidate, root); ok {
			readableRoots = append(readableRoots, normalized)
		}
	}
	if len(readableRoots) == 0 && root != "" {
		readableRoots = append(readableRoots, root)
	}

	writableRoots := make([]string, 0, len(c.WritableRoots)+1)
	for _, candidate := range c.WritableRoots {
		if normalized, ok := AbsoluteCleanPath(candidate, root); ok {
			writableRoots = append(writableRoots, normalized)
		}
	}
	if len(writableRoots) == 0 && root != "" {
		writableRoots = append(writableRoots, root)
	}

	outputRoots := make([]string, 0, len(c.OutputRoots)+1)
	for _, candidate := range c.OutputRoots {
		if normalized, ok := AbsoluteCleanPath(candidate, root); ok {
			outputRoots = append(outputRoots, normalized)
		}
	}
	if len(outputRoots) == 0 && root != "" {
		outputRoots = append(outputRoots, root)
	}

	attachedFiles := make([]ScopedFile, 0, len(c.AttachedFiles))
	for _, file := range c.AttachedFiles {
		if normalized, ok := AbsoluteCleanPath(file.Path, root); ok {
			attachedFiles = append(attachedFiles, ScopedFile{
				Path:        normalized,
				DisplayName: strings.TrimSpace(file.DisplayName),
			})
		}
	}

	mediaArtifacts := make([]ScopedMediaArtifact, 0, len(c.MediaArtifacts))
	seenMediaArtifacts := make(map[string]struct{}, len(c.MediaArtifacts))
	for _, artifact := range c.MediaArtifacts {
		id := strings.TrimSpace(artifact.ID)
		kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
		path, ok := AbsoluteCleanPath(artifact.Path, root)
		if id == "" || !ok || (kind != "image" && kind != "video") {
			continue
		}
		if _, duplicate := seenMediaArtifacts[id]; duplicate {
			continue
		}
		seenMediaArtifacts[id] = struct{}{}
		artifact.ID = id
		artifact.RunID = strings.TrimSpace(artifact.RunID)
		artifact.GroupID = strings.TrimSpace(artifact.GroupID)
		artifact.Kind = kind
		artifact.Path = path
		artifact.Status = strings.ToLower(strings.TrimSpace(artifact.Status))
		mediaArtifacts = append(mediaArtifacts, artifact)
	}

	protectedRoots := make([]string, 0, len(c.ProtectedRoots))
	for _, candidate := range c.ProtectedRoots {
		if normalized, ok := AbsoluteCleanPath(candidate, root); ok {
			protectedRoots = append(protectedRoots, normalized)
		}
	}

	protectedRootExceptions := make([]string, 0, len(c.ProtectedRootExceptions))
	for _, candidate := range c.ProtectedRootExceptions {
		if normalized, ok := AbsoluteCleanPath(candidate, root); ok {
			protectedRootExceptions = append(protectedRootExceptions, normalized)
		}
	}

	temporaryWorkRoot := strings.TrimSpace(c.TemporaryWorkRoot)
	if normalized, ok := AbsoluteCleanPath(temporaryWorkRoot, root); ok {
		temporaryWorkRoot = normalized
	} else {
		temporaryWorkRoot = filepath.Join(os.TempDir(), "awaken-task-scope")
	}

	return PermissionContext{
		ScopeID:                 strings.TrimSpace(c.ScopeID),
		ProjectID:               strings.TrimSpace(c.ProjectID),
		SessionWorkspace:        c.SessionWorkspace,
		WorkspaceRoot:           root,
		ReadableRoots:           dedupeStrings(readableRoots),
		WritableRoots:           dedupeStrings(writableRoots),
		OutputRoots:             dedupeStrings(outputRoots),
		AttachedFiles:           dedupeScopedFiles(attachedFiles),
		MediaArtifacts:          mediaArtifacts,
		ProtectedRoots:          dedupeStrings(protectedRoots),
		ProtectedRootExceptions: dedupeStrings(protectedRootExceptions),
		TemporaryWorkRoot:       temporaryWorkRoot,
	}
}

// ResolveReadyMediaArtifact binds a model-provided artifact ID back to the
// exact host-verified ready media path in this run's task scope.
func (c PermissionContext) ResolveReadyMediaArtifact(id, expectedKind string) (ScopedMediaArtifact, string, error) {
	c = c.Normalize()
	id = strings.TrimSpace(id)
	expectedKind = strings.ToLower(strings.TrimSpace(expectedKind))
	if id == "" {
		return ScopedMediaArtifact{}, "", fmt.Errorf("artifact id is required")
	}
	for _, artifact := range c.MediaArtifacts {
		if artifact.ID != id {
			continue
		}
		if artifact.Role != ArtifactRoleDeliverable || artifact.Status != "ready" {
			return ScopedMediaArtifact{}, "", fmt.Errorf("artifact is not a ready deliverable")
		}
		if expectedKind != "" && artifact.Kind != expectedKind {
			return ScopedMediaArtifact{}, "", fmt.Errorf("artifact kind must be %s", expectedKind)
		}
		resolved, err := ResolvePathWithExistingAncestor(artifact.Path)
		if err != nil {
			return ScopedMediaArtifact{}, "", err
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return ScopedMediaArtifact{}, "", fmt.Errorf("artifact path is not a readable regular file")
		}
		return artifact, resolved, nil
	}
	return ScopedMediaArtifact{}, "", fmt.Errorf("artifact id is not available in this thread")
}

func (c PermissionContext) IsWithinWorkspace(path string) bool {
	normalized := c.Normalize()
	return PathWithinRoot(normalized.resolvePath(path), normalized.WorkspaceRoot)
}

func (c PermissionContext) IsWithinReadableRoots(path string) bool {
	normalized := c.Normalize()
	resolved := normalized.resolvePath(path)
	for _, root := range normalized.ReadableRoots {
		if PathWithinRoot(resolved, root) {
			return true
		}
	}
	for _, attached := range normalized.AttachedFiles {
		if samePermissionPath(resolved, attached.Path) {
			return true
		}
	}
	return false
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

func (c PermissionContext) IsWithinOutputRoots(path string) bool {
	normalized := c.Normalize()
	for _, root := range normalized.OutputRoots {
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

func (c PermissionContext) IsWithinReadableRootsResolved(path string) bool {
	_, err := c.ResolveReadablePathChecked(path)
	return err == nil
}

func (c PermissionContext) WorkspaceRootOrCWD() string {
	normalized := c.Normalize()
	return normalized.WorkspaceRoot
}

func (c PermissionContext) ResolvePath(path string) string {
	return c.Normalize().resolvePath(path)
}

// ResolveACPCwd validates a working directory for an external ACP runner.
// Empty or "." cwd resolves to the workspace root. Relative paths are joined
// to the workspace root; absolute paths are checked as-is. The candidate must
// be contained within the workspace root after Clean/Abs/EvalSymlinks, must
// not fall inside a protected task-scope root, and must not resolve to a
// sensitive path. The returned path is the cleaned, lexical workspace path.
func (c PermissionContext) ResolveACPCwd(cwd string) (string, error) {
	normalized := c.Normalize()
	if normalized.WorkspaceRoot == "" {
		return "", fmt.Errorf("workspace root not available")
	}

	cwd = strings.TrimSpace(cwd)
	if cwd == "" || cwd == "." {
		return normalized.WorkspaceRoot, nil
	}

	candidate := normalized.resolvePath(cwd)
	rootReal, err := ResolvePathWithExistingAncestor(normalized.WorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("workspace root cannot be resolved: %w", err)
	}

	if !PathWithinRoot(candidate, normalized.WorkspaceRoot) {
		return "", fmt.Errorf("cwd %q is outside workspace root", cwd)
	}

	realPath, err := ResolvePathWithExistingAncestor(candidate)
	if err != nil {
		return "", fmt.Errorf("cwd %q cannot be resolved: %w", cwd, err)
	}
	if !PathWithinRoot(realPath, rootReal) {
		return "", fmt.Errorf("cwd %q resolves outside workspace root", cwd)
	}

	exception := narrowestContainingRoot(realPath, normalized.resolvedProtectedRootExceptions())
	for _, protectedRoot := range normalized.ProtectedRoots {
		realProtected, err := ResolvePathWithExistingAncestor(protectedRoot)
		if err != nil {
			continue
		}
		if !PathWithinRoot(realPath, realProtected) {
			continue
		}
		if exception != "" && PathWithinRoot(exception, realProtected) {
			continue
		}
		return "", fmt.Errorf("cwd %q is inside protected task-scope root %q", cwd, protectedRoot)
	}

	if err := normalized.checkResolvedSensitiveAbsolutePath(realPath); err != nil {
		return "", fmt.Errorf("cwd %q is a sensitive path: %w", cwd, err)
	}

	return candidate, nil
}

// IsWithinWorkspaceResolved resolves symlinks before checking workspace
// boundary. It returns false if the path cannot be resolved, treating
// unresolvable paths as unsafe.
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

// ResolveReadablePathChecked turns a tool-provided path into an absolute path.
//
// Roots describe where work happens by default, not what the agent is allowed
// to touch: a relative path resolves against the primary folder, while an
// absolute path is honoured wherever it points. Confining reads to the roots
// would only have constrained the structured tools, because Bash runs arbitrary
// commands with no path check at all, so the fence pushed out-of-scope work
// into shell invocations instead of preventing it.
func (c PermissionContext) ResolveReadablePathChecked(path string) (string, error) {
	normalized := c.Normalize()
	if attachedPath, ok, err := normalized.resolveAttachedFileReference(path); ok || err != nil {
		return attachedPath, err
	}
	return normalized.resolvePath(path), nil
}

// ResolvePathChecked preserves the historical API name for read-tool callers.
func (c PermissionContext) ResolvePathChecked(path string) (string, error) {
	return c.ResolveReadablePathChecked(path)
}

// ResolveWritablePathChecked resolves a path a caller intends to modify. It
// carries the mutation policy rather than a bare resolution, so a caller that
// reaches for the "writable" name cannot accidentally skip the sensitive-path
// checks now that writable roots no longer form a boundary.
func (c PermissionContext) ResolveWritablePathChecked(path string) (string, error) {
	return c.ResolveMutationPathChecked(path)
}

// ResolveOutputPathChecked resolves a path that is read or inspected as a
// produced artifact. Output roots are where such files are expected to live,
// not where they are required to live, so this is a plain resolution.
func (c PermissionContext) ResolveOutputPathChecked(path string) (string, error) {
	return c.ResolveReadablePathChecked(path)
}

// ResolveMutationPathChecked resolves a mutation target and applies the
// sensitive-path policy to both the lexical path and its symlink destination.
// Use it for any file-chain mutation (write/edit/delete).
//
// Like reads, mutations are not fenced to the roots. What stays denied is a
// fixed set of shapes that a mutation is never meant to hit -- .git internals,
// .env, key material, shell profiles and Awaken's own state directory -- so an
// out-of-scope write succeeds while a write that would corrupt a repository or
// the agent's own store still fails.
func (c PermissionContext) ResolveMutationPathChecked(path string) (string, error) {
	return c.Normalize().resolveMutationPath(path)
}

// ResolveOutputMutationPathChecked resolves a publish/replace target. Output
// roots are the default destination for produced files; the path itself is
// subject to the same sensitive-path policy as any other mutation.
func (c PermissionContext) ResolveOutputMutationPathChecked(path string) (string, error) {
	return c.Normalize().resolveMutationPath(path)
}

// RecoverableDeletePathError reports a path that cannot safely enter the
// recoverable-delete pipeline. The adapter must never treat this as permission
// to fall back to permanent deletion.
type RecoverableDeletePathError struct {
	Path   string
	Reason string
}

func (e *RecoverableDeletePathError) Error() string {
	return fmt.Sprintf("recoverable delete path %q denied: %s", e.Path, e.Reason)
}

// ResolveRecoverableDeletePathChecked validates a path for trash_file without
// imposing workspace writable-root scope: user-requested project-external
// files remain supported. Sensitive/protected paths, symlinks, filesystem
// roots, missing paths, and special files fail closed before the Trash adapter.
func (c PermissionContext) ResolveRecoverableDeletePathChecked(path string) (string, error) {
	normalized := c.Normalize()
	resolved := normalized.resolvePath(path)
	if err := normalized.checkSensitiveAbsolutePath(resolved); err != nil {
		return "", err
	}
	if isFilesystemRoot(resolved) {
		return "", &RecoverableDeletePathError{Path: resolved, Reason: "filesystem root cannot be moved to Trash"}
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", &RecoverableDeletePathError{Path: resolved, Reason: err.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", &RecoverableDeletePathError{Path: resolved, Reason: "symbolic links require an explicit non-link target"}
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", &RecoverableDeletePathError{Path: resolved, Reason: "special filesystem objects are not supported"}
	}
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", &RecoverableDeletePathError{Path: resolved, Reason: "real path cannot be resolved: " + err.Error()}
	}
	if err := normalized.checkResolvedSensitiveAbsolutePath(realPath); err != nil {
		return "", err
	}
	for _, protectedRoot := range normalized.ProtectedRoots {
		if PathWithinRoot(protectedRoot, resolved) {
			return "", &RecoverableDeletePathError{Path: resolved, Reason: "path contains a protected task-scope root"}
		}
		if realProtectedRoot, resolveErr := ResolvePathWithExistingAncestor(protectedRoot); resolveErr == nil && PathWithinRoot(realProtectedRoot, realPath) {
			return "", &RecoverableDeletePathError{Path: resolved, Reason: "real path contains a protected task-scope root"}
		}
	}
	matcher := NewSensitivePathMatcher()
	for _, root := range []string{matcher.HomeDir, matcher.ConfigDir} {
		if root == "" {
			continue
		}
		realRoot, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr == nil && samePermissionPath(realPath, realRoot) {
			return "", &RecoverableDeletePathError{Path: resolved, Reason: "user home or configuration root cannot be moved to Trash"}
		}
	}
	return resolved, nil
}

func isFilesystemRoot(path string) bool {
	cleaned := filepath.Clean(path)
	parent := filepath.Dir(cleaned)
	return cleaned == parent
}

func samePermissionPath(left, right string) bool {
	return PathWithinRoot(left, right) && PathWithinRoot(right, left)
}

func (c PermissionContext) resolveMutationPath(path string) (string, error) {
	resolved := c.resolvePath(path)
	if err := c.checkSensitiveAbsolutePath(resolved); err != nil {
		return "", err
	}
	lexicalException := narrowestContainingRoot(resolved, c.ProtectedRootExceptions)
	realPath, err := ResolvePathWithExistingAncestor(resolved)
	if err != nil {
		return "", fmt.Errorf("path %q cannot be resolved safely for mutation: %w", path, err)
	}
	if lexicalException == "" {
		for _, exception := range c.ProtectedRootExceptions {
			realException, resolveErr := ResolvePathWithExistingAncestor(exception)
			if resolveErr == nil && PathWithinRoot(realPath, realException) {
				return "", &SensitivePathError{Path: resolved, Reason: "protected-root exception must be addressed through its lexical root"}
			}
		}
	}
	if lexicalException != "" {
		realException, resolveErr := ResolvePathWithExistingAncestor(lexicalException)
		if resolveErr != nil {
			return "", fmt.Errorf("protected-root exception %q cannot be resolved safely: %w", lexicalException, resolveErr)
		}
		if !PathWithinRoot(realPath, realException) {
			return "", &SensitivePathError{Path: resolved, Reason: "protected-root exception symlink escape"}
		}
		// An exception nested in a protected root must resolve inside that same
		// protected root. Otherwise replacing the exception directory itself
		// with a symlink would turn the carve-out into an arbitrary write grant.
		for _, protectedRoot := range c.ProtectedRoots {
			if !PathWithinRoot(lexicalException, protectedRoot) {
				continue
			}
			realProtectedRoot, protectedErr := ResolvePathWithExistingAncestor(protectedRoot)
			if protectedErr != nil || !PathWithinRoot(realException, realProtectedRoot) {
				return "", &SensitivePathError{Path: resolved, Reason: "protected-root exception resolves outside its protected root"}
			}
		}
	}
	if err := c.checkResolvedSensitiveAbsolutePath(realPath); err != nil {
		return "", err
	}
	return resolved, nil
}

// ResolvePathWithExistingAncestor resolves every symlink in an existing path.
// For a path whose leaf does not exist yet, it resolves the nearest existing
// ancestor and appends the missing suffix. Broken symlinks fail closed.
func ResolvePathWithExistingAncestor(path string) (string, error) {
	cursor := filepath.Clean(path)
	missing := make([]string, 0, 2)
	for {
		if _, err := os.Lstat(cursor); err == nil {
			realAncestor, evalErr := filepath.EvalSymlinks(cursor)
			if evalErr != nil {
				return "", evalErr
			}
			for i := len(missing) - 1; i >= 0; i-- {
				realAncestor = filepath.Join(realAncestor, missing[i])
			}
			return filepath.Clean(realAncestor), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = parent
	}
}

// SensitivePathError describes a mutation attempt against a path that the
// runtime treats as sensitive and therefore denies by default, regardless of
// whether the path is inside the workspace or writable roots. Callers can
// use errors.As to detect it and produce a structured tool_result rejection.
type SensitivePathError struct {
	Path   string
	Reason string
}

func (e *SensitivePathError) Error() string {
	return fmt.Sprintf("sensitive path %q denied: %s", e.Path, e.Reason)
}

func (e *SensitivePathError) Is(target error) bool {
	other, ok := target.(*SensitivePathError)
	if !ok {
		return false
	}
	return e.Path == other.Path && e.Reason == other.Reason
}

// SensitivePathMatcher decides whether a given absolute path is considered
// sensitive for mutation purposes (write/delete). Reads are NOT affected by
// this matcher; only file-chain mutations are denied.
//
// Categories denied by default:
//   - .git/ directories (any path component equal to ".git")
//   - .env files (basename ".env" or starting with ".env.", including
//     ".env.example" — denying is safer than allowing templates to leak)
//   - Key/certificate/credential files: extensions .pem, .key, .crt, .p12,
//     .pfx, .keystore, .jks, and basenames containing id_rsa, id_ed25519,
//     id_ecdsa, credentials, secrets
//   - Agent metadata directory: ~/.awaken and ~/.trae-cn
//   - Desktop/runtime local config: os.UserConfigDir()/awaken and /Awaken
type SensitivePathMatcher struct {
	HomeDir   string
	ConfigDir string
}

// NewSensitivePathMatcher builds a matcher using the current process home and
// config directories. If either cannot be resolved, the corresponding checks
// are skipped rather than failing.
func NewSensitivePathMatcher() SensitivePathMatcher {
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	return SensitivePathMatcher{HomeDir: home, ConfigDir: configDir}
}

// sensitiveExtensions maps a lowercased file extension to a human-readable
// reason. Both the bare extension (e.g. "server.pem") and compound names
// (e.g. "server.pem.bak") are matched via filepath.Ext.
var sensitiveExtensions = map[string]string{
	".pem":      "PEM key/certificate file",
	".key":      "private key file",
	".crt":      "certificate file",
	".p12":      "PKCS#12 archive",
	".pfx":      "PFX archive",
	".keystore": "Java keystore",
	".jks":      "Java keystore",
}

// sensitiveTokens are substrings that, when present in a basename, indicate
// key/credential material.
var sensitiveTokens = []string{
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
	"credentials",
	"secrets",
}

var sensitivePathComponents = map[string]string{
	".git":    ".git directory is read-only",
	".awaken": "Awaken project metadata directory",
	".vscode": "IDE config directory",
	".idea":   "IDE config directory",
}

var sensitiveHomeBasenames = map[string]string{
	".zshrc":                           "shell profile",
	".zprofile":                        "shell profile",
	".zlogin":                          "shell profile",
	".zlogout":                         "shell profile",
	".bashrc":                          "shell profile",
	".bash_profile":                    "shell profile",
	".bash_logout":                     "shell profile",
	".profile":                         "shell profile",
	"config.fish":                      "shell profile",
	"microsoft.powershell_profile.ps1": "shell profile",
	"profile.ps1":                      "shell profile",
}

// IsSensitive reports whether path should be denied for mutation, and returns
// a human-readable reason. The path should be absolute; if it is relative,
// only the lexical checks (.git component, .env basename, extensions, tokens)
// are applied.
func (m SensitivePathMatcher) IsSensitive(path string) (bool, string) {
	cleaned := filepath.Clean(path)
	base := filepath.Base(cleaned)
	lowerBase := strings.ToLower(base)

	if sensitive, reason := sensitivePathShape(cleaned); sensitive {
		return true, reason
	}

	// Agent metadata directory: ~/.awaken and ~/.trae-cn.
	if m.HomeDir != "" {
		if reason, ok := sensitiveHomeBasenames[lowerBase]; ok && PathWithinRoot(cleaned, m.HomeDir) {
			return true, reason
		}
		for _, name := range []string{".awaken", ".trae-cn"} {
			metaRoot := filepath.Join(m.HomeDir, name)
			if PathWithinRoot(cleaned, metaRoot) {
				return true, "agent metadata directory (~/" + name + ")"
			}
		}
	}

	// Desktop/runtime local config directory: UserConfigDir/awaken or /Awaken.
	if m.ConfigDir != "" {
		for _, name := range []string{"awaken", "Awaken"} {
			configRoot := filepath.Join(m.ConfigDir, name)
			if PathWithinRoot(cleaned, configRoot) {
				return true, "runtime local config directory"
			}
		}
	}

	return false, ""
}

// IsSensitiveBelow applies only the location-independent rules to the part of
// path that lives below root. It exists for roots that are an explicit carve-out
// from an otherwise protected subtree: the carve-out must stop the enclosing
// location rule from matching, without also unlocking `.git`, `.env` or
// credential files inside the carve-out itself.
func (m SensitivePathMatcher) IsSensitiveBelow(root, path string) (bool, string) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == "." {
		return false, ""
	}
	return sensitivePathShape(relative)
}

// sensitivePathShape holds the rules that depend only on the shape of a path:
// protected directory components, .env files, key/certificate extensions and
// credential-looking basenames.
func sensitivePathShape(path string) (bool, string) {
	base := filepath.Base(path)
	lowerBase := strings.ToLower(base)

	// Protected directories anywhere in the path hierarchy.
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if reason, ok := sensitivePathComponents[component]; ok {
			return true, reason
		}
	}

	// .env files: basename ".env" or starting with ".env.". This
	// intentionally covers ".env.example" as well — denying is safer than
	// allowing, because templates are easy to mistake for real secret files.
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true, ".env file may contain secrets"
	}

	// Key/certificate extensions. Check all dot-separated extension
	// components so that backups like "server.pem.bak" are still flagged.
	parts := strings.Split(lowerBase, ".")
	for _, part := range parts[1:] {
		if reason, ok := sensitiveExtensions["."+part]; ok {
			return true, reason
		}
	}

	// Basenames containing sensitive tokens (id_rsa, credentials, etc.).
	for _, token := range sensitiveTokens {
		if strings.Contains(lowerBase, token) {
			return true, "key/credential file"
		}
	}

	return false, ""
}

// IsSensitivePath reports whether path is considered sensitive for mutation
// purposes. The path is resolved against the workspace root before checking.
// Sensitive paths are denied for writes/deletes regardless of whether they
// are inside the workspace or writable roots.
func (c PermissionContext) IsSensitivePath(path string) bool {
	return c.CheckSensitivePath(path) != nil
}

// CheckSensitivePath returns a *SensitivePathError if the path is sensitive,
// otherwise nil. The path is resolved against the workspace root first.
// Callers handling mutation paths should invoke this before any filesystem
// write/delete; read paths must NOT be gated by this check.
func (c PermissionContext) CheckSensitivePath(path string) error {
	normalized := c.Normalize()
	resolved := normalized.resolvePath(path)
	return normalized.checkSensitiveAbsolutePath(resolved)
}

func (c PermissionContext) checkSensitiveAbsolutePath(path string) error {
	return checkSensitiveAbsolutePathAgainst(path, c.ProtectedRoots, c.ProtectedRootExceptions, NewSensitivePathMatcher())
}

// resolvedProtectedRootExceptions resolves carve-out roots through symlinks so
// they can be compared against already-resolved real paths.
func (c PermissionContext) resolvedProtectedRootExceptions() []string {
	exceptions := make([]string, 0, len(c.ProtectedRootExceptions))
	for _, exception := range c.ProtectedRootExceptions {
		if realException, err := ResolvePathWithExistingAncestor(exception); err == nil {
			exceptions = append(exceptions, realException)
		}
	}
	return exceptions
}

func (c PermissionContext) checkResolvedSensitiveAbsolutePath(realPath string) error {
	protectedRoots := make([]string, 0, len(c.ProtectedRoots))
	for _, protectedRoot := range c.ProtectedRoots {
		if realProtectedRoot, err := ResolvePathWithExistingAncestor(protectedRoot); err == nil {
			protectedRoots = append(protectedRoots, realProtectedRoot)
		}
	}
	exceptions := c.resolvedProtectedRootExceptions()

	matcher := NewSensitivePathMatcher()
	if matcher.HomeDir != "" {
		if realHome, err := ResolvePathWithExistingAncestor(matcher.HomeDir); err == nil {
			matcher.HomeDir = realHome
		}
	}
	if matcher.ConfigDir != "" {
		if realConfigDir, err := ResolvePathWithExistingAncestor(matcher.ConfigDir); err == nil {
			matcher.ConfigDir = realConfigDir
		}
	}
	return checkSensitiveAbsolutePathAgainst(realPath, protectedRoots, exceptions, matcher)
}

func checkSensitiveAbsolutePathAgainst(path string, protectedRoots, exceptions []string, matcher SensitivePathMatcher) error {
	exception := narrowestContainingRoot(path, exceptions)
	for _, protectedRoot := range protectedRoots {
		if !PathWithinRoot(path, protectedRoot) {
			continue
		}
		// A protected root that encloses the carve-out is what the carve-out
		// exists to release. One that sits inside the carve-out still applies,
		// so per-folder protections such as `<root>/.git` keep working.
		if exception != "" && PathWithinRoot(exception, protectedRoot) {
			continue
		}
		return &SensitivePathError{Path: path, Reason: "protected task-scope root"}
	}
	if exception != "" {
		if sensitive, reason := matcher.IsSensitiveBelow(exception, path); sensitive {
			return &SensitivePathError{Path: path, Reason: reason}
		}
		return nil
	}
	sensitive, reason := matcher.IsSensitive(path)
	if !sensitive {
		return nil
	}
	return &SensitivePathError{Path: path, Reason: reason}
}

// narrowestContainingRoot returns the deepest root that contains path, or an
// empty string when none does.
func narrowestContainingRoot(path string, roots []string) string {
	best := ""
	for _, root := range roots {
		if root == "" || !PathWithinRoot(path, root) {
			continue
		}
		if len(root) > len(best) {
			best = root
		}
	}
	return best
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

func (c PermissionContext) resolveAttachedFileReference(path string) (string, bool, error) {
	raw := strings.TrimSpace(path)
	if raw == "" || filepath.IsAbs(expandTilde(raw)) {
		return "", false, nil
	}

	matches := make([]ScopedFile, 0, 1)
	seen := make(map[string]struct{})
	for _, attachment := range c.AttachedFiles {
		if !attachedFileReferenceMatches(raw, attachment) {
			continue
		}
		if _, ok := seen[attachment.Path]; ok {
			continue
		}
		seen[attachment.Path] = struct{}{}
		matches = append(matches, attachment)
	}
	switch len(matches) {
	case 0:
		return "", false, nil
	case 1:
		return matches[0].Path, true, nil
	default:
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.Path)
		}
		sort.Strings(paths)
		return "", true, fmt.Errorf("附件名 %q 匹配到多个文件，请使用完整路径: %s", raw, strings.Join(paths, ", "))
	}
}

func attachedFileReferenceMatches(query string, attachment ScopedFile) bool {
	queryAlias := cleanAttachedFileAlias(query)
	if queryAlias == "" {
		return false
	}
	queryBase := cleanAttachedFileAlias(filepath.Base(queryAlias))
	queryHasSeparator := strings.ContainsAny(query, `/\`)
	for _, candidate := range []string{attachment.DisplayName, filepath.Base(attachment.Path)} {
		candidateAlias := cleanAttachedFileAlias(candidate)
		if candidateAlias == "" {
			continue
		}
		if queryAlias == candidateAlias {
			return true
		}
		if !queryHasSeparator && queryBase == cleanAttachedFileAlias(filepath.Base(candidateAlias)) {
			return true
		}
	}
	return false
}

func cleanAttachedFileAlias(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, string(filepath.Separator))
	value = filepath.Clean(filepath.FromSlash(value))
	if value == "." {
		return ""
	}
	return value
}

func (c PermissionContext) matchesAttachedFile(path string) bool {
	for _, attachment := range c.AttachedFiles {
		if attachmentPathMatches(path, attachment.Path) {
			return true
		}
	}
	return false
}

func (c PermissionContext) matchesResolvedAttachedFile(realPath string) bool {
	for _, attachment := range c.AttachedFiles {
		target := attachment.Path
		realTarget, err := filepath.EvalSymlinks(target)
		if err != nil {
			realTarget = target
		}
		if attachmentPathMatches(realPath, realTarget) {
			return true
		}
	}
	return false
}

func attachmentPathMatches(candidate string, target string) bool {
	if candidate == target {
		return true
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return false
	}
	return PathWithinRoot(candidate, target)
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

func dedupeScopedFiles(values []ScopedFile) []ScopedFile {
	seen := make(map[string]struct{}, len(values))
	deduped := make([]ScopedFile, 0, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		value.Path = path
		deduped = append(deduped, value)
	}
	return deduped
}
