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
	Metadata  DecisionMetadata
}

type PermissionContext struct {
	ScopeID           string
	WorkspaceRoot     string
	ReadableRoots     []string
	WritableRoots     []string
	OutputRoots       []string
	AttachedFiles     []ScopedFile
	ProtectedRoots    []string
	TemporaryWorkRoot string
}

type permissionContextKey struct{}

type PermissionCheckingTool interface {
	CheckPermission(ctx context.Context, input string, permissionContext PermissionContext) PermissionDecision
}

func AllowPermission() PermissionDecision {
	return PermissionDecision{Behavior: PermissionAllow}
}

func AllowPermissionWithMetadata(metadata DecisionMetadata) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionAllow,
		Metadata: metadata,
	}
}

func DenyPermission(reason string) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionDeny,
		Reason:   reason,
	}
}

func DenyPermissionWithMetadata(reason string, metadata DecisionMetadata) PermissionDecision {
	return PermissionDecision{
		Behavior: PermissionDeny,
		Reason:   reason,
		Metadata: metadata,
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

	protectedRoots := make([]string, 0, len(c.ProtectedRoots))
	for _, candidate := range c.ProtectedRoots {
		if normalized, ok := AbsoluteCleanPath(candidate, root); ok {
			protectedRoots = append(protectedRoots, normalized)
		}
	}

	temporaryWorkRoot := strings.TrimSpace(c.TemporaryWorkRoot)
	if normalized, ok := AbsoluteCleanPath(temporaryWorkRoot, root); ok {
		temporaryWorkRoot = normalized
	} else {
		temporaryWorkRoot = filepath.Join(os.TempDir(), "awaken-task-scope")
	}

	return PermissionContext{
		ScopeID:           strings.TrimSpace(c.ScopeID),
		WorkspaceRoot:     root,
		ReadableRoots:     dedupeStrings(readableRoots),
		WritableRoots:     dedupeStrings(writableRoots),
		OutputRoots:       dedupeStrings(outputRoots),
		AttachedFiles:     dedupeScopedFiles(attachedFiles),
		ProtectedRoots:    dedupeStrings(protectedRoots),
		TemporaryWorkRoot: temporaryWorkRoot,
	}
}

func (c PermissionContext) IsWithinWorkspace(path string) bool {
	normalized := c.Normalize()
	return PathWithinRoot(normalized.resolvePath(path), normalized.WorkspaceRoot)
}

func (c PermissionContext) IsWithinReadableRoots(path string) bool {
	return true
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

// IsWithinReadableRootsResolved is retained for older read-tool call sites.
// Local read tools no longer use readable_roots as a permission boundary; the
// resolver below normalizes local paths and maps attached-file display names.
func (c PermissionContext) IsWithinReadableRootsResolved(path string) bool {
	return true
}

func (c PermissionContext) WorkspaceRootOrCWD() string {
	normalized := c.Normalize()
	return normalized.WorkspaceRoot
}

func (c PermissionContext) ResolvePath(path string) string {
	return c.Normalize().resolvePath(path)
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

// ResolveReadablePathChecked resolves a local read path without enforcing a
// readable_roots boundary. If the caller supplies an attached file's display
// name or basename, it returns the attached file's real local path.
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

func (c PermissionContext) ResolveWritablePathChecked(path string) (string, error) {
	normalized := c.Normalize()
	resolved, _, err := normalized.resolvePathWithinRootsChecked(path, normalized.WritableRoots, "writable")
	return resolved, err
}

func (c PermissionContext) ResolveOutputPathChecked(path string) (string, error) {
	normalized := c.Normalize()
	resolved, _, err := normalized.resolvePathWithinRootsChecked(path, normalized.OutputRoots, "output")
	return resolved, err
}

// ResolveMutationPathChecked is like ResolvePathChecked but additionally
// rejects paths that the runtime considers sensitive (e.g. .git/, .env, key
// files, agent metadata directories). Use this for any file-chain mutation
// (write/delete) path resolution. Reads must continue to use ResolvePathChecked
// so that sensitive paths remain readable.
func (c PermissionContext) ResolveMutationPathChecked(path string) (string, error) {
	normalized := c.Normalize()
	return normalized.resolveMutationPathWithinRootsChecked(path, normalized.WritableRoots, "writable")
}

// ResolveOutputMutationPathChecked enforces the output-root boundary and the
// mutation sensitive/protected-path policy against both the lexical path and
// its symlink-resolved destination. Use this for tools that publish or replace
// files in OutputRoots.
func (c PermissionContext) ResolveOutputMutationPathChecked(path string) (string, error) {
	normalized := c.Normalize()
	return normalized.resolveMutationPathWithinRootsChecked(path, normalized.OutputRoots, "output")
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
		if realProtectedRoot, resolveErr := resolvePathWithExistingAncestor(protectedRoot); resolveErr == nil && PathWithinRoot(realProtectedRoot, realPath) {
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

func (c PermissionContext) resolveMutationPathWithinRootsChecked(path string, roots []string, rootKind string) (string, error) {
	resolved := c.resolvePath(path)
	if err := c.checkSensitiveAbsolutePath(resolved); err != nil {
		return "", err
	}
	realPath, err := resolvePathWithExistingAncestor(resolved)
	if err != nil {
		return "", fmt.Errorf("path %q cannot be resolved safely for mutation: %w", path, err)
	}
	if err := c.checkResolvedSensitiveAbsolutePath(realPath); err != nil {
		return "", err
	}
	if err := checkPathWithinRealRoots(path, resolved, realPath, roots, rootKind); err != nil {
		return "", err
	}
	return resolved, nil
}

func (c PermissionContext) resolvePathWithinRootsChecked(path string, roots []string, rootKind string) (string, string, error) {
	resolved := c.resolvePath(path)
	lexicallyAllowed := false
	for _, root := range roots {
		if PathWithinRoot(resolved, root) {
			lexicallyAllowed = true
			break
		}
	}
	if !lexicallyAllowed {
		return "", "", fmt.Errorf("path %q resolves to %q which is outside %s roots", path, resolved, rootKind)
	}

	realPath, err := resolvePathWithExistingAncestor(resolved)
	if err != nil {
		return "", "", fmt.Errorf("path %q cannot be resolved safely inside %s roots: %w", path, rootKind, err)
	}
	if err := checkPathWithinRealRoots(path, resolved, realPath, roots, rootKind); err != nil {
		return "", "", err
	}
	return resolved, realPath, nil
}

func checkPathWithinRealRoots(path, resolved, realPath string, roots []string, rootKind string) error {
	lexicallyAllowed := false
	for _, root := range roots {
		if PathWithinRoot(resolved, root) {
			lexicallyAllowed = true
			break
		}
	}
	if !lexicallyAllowed {
		return fmt.Errorf("path %q resolves to %q which is outside %s roots", path, resolved, rootKind)
	}

	resolvedRootCount := 0
	for _, root := range roots {
		realRoot, err := resolvePathWithExistingAncestor(root)
		if err != nil {
			continue
		}
		resolvedRootCount++
		if PathWithinRoot(realPath, realRoot) {
			return nil
		}
	}
	if resolvedRootCount == 0 {
		return fmt.Errorf("path %q cannot be checked because no %s root resolves safely", path, rootKind)
	}
	return fmt.Errorf(
		"path %q resolves through symlinks from %q to %q which is outside real %s roots",
		path,
		resolved,
		realPath,
		rootKind,
	)
}

// resolvePathWithExistingAncestor resolves every symlink in an existing path.
// For a path whose leaf does not exist yet, it resolves the nearest existing
// ancestor and appends the missing suffix. Broken symlinks fail closed.
func resolvePathWithExistingAncestor(path string) (string, error) {
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

	// Protected directories anywhere in the path hierarchy.
	for _, component := range strings.Split(filepath.ToSlash(cleaned), "/") {
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
	return checkSensitiveAbsolutePathAgainst(path, c.ProtectedRoots, NewSensitivePathMatcher())
}

func (c PermissionContext) checkResolvedSensitiveAbsolutePath(realPath string) error {
	protectedRoots := make([]string, 0, len(c.ProtectedRoots))
	for _, protectedRoot := range c.ProtectedRoots {
		if realProtectedRoot, err := resolvePathWithExistingAncestor(protectedRoot); err == nil {
			protectedRoots = append(protectedRoots, realProtectedRoot)
		}
	}

	matcher := NewSensitivePathMatcher()
	if matcher.HomeDir != "" {
		if realHome, err := resolvePathWithExistingAncestor(matcher.HomeDir); err == nil {
			matcher.HomeDir = realHome
		}
	}
	if matcher.ConfigDir != "" {
		if realConfigDir, err := resolvePathWithExistingAncestor(matcher.ConfigDir); err == nil {
			matcher.ConfigDir = realConfigDir
		}
	}
	return checkSensitiveAbsolutePathAgainst(realPath, protectedRoots, matcher)
}

func checkSensitiveAbsolutePathAgainst(path string, protectedRoots []string, matcher SensitivePathMatcher) error {
	for _, protectedRoot := range protectedRoots {
		if PathWithinRoot(path, protectedRoot) {
			return &SensitivePathError{Path: path, Reason: "protected task-scope root"}
		}
	}
	sensitive, reason := matcher.IsSensitive(path)
	if !sensitive {
		return nil
	}
	return &SensitivePathError{Path: path, Reason: reason}
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
