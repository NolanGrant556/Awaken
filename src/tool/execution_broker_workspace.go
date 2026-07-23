package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/tool/trash"
)

const workspaceExecutionBrokerID = "workspace_staging_trash_commit_v1"

// WorkspaceExecutionBrokerOptions are host-owned dependencies. StagingParent
// and TrashAdapter are injectable so tests never touch the user's real Trash.
type WorkspaceExecutionBrokerOptions struct {
	StagingParent string
	TrashAdapter  trash.TrashAdapter
}

// BrokerWorkspaceScope is the workspace-local projection of a
// PermissionContext. Roots are canonical paths relative to the real workspace
// root. Writable roots resolve symlinks before projection; protected roots
// retain both their lexical location (so a protected symlink itself cannot be
// removed) and any workspace-local resolved target.
type BrokerWorkspaceScope struct {
	WritableRoots  []string
	ProtectedRoots []string
}

// WorkspaceExecutionBroker executes against a private workspace snapshot.
// The Shell executor must apply an OS sandbox whose only writable root is the
// returned SandboxRoot. Complete then commits create/update deltas and moves
// every deleted or replaced preimage to the operating-system Trash first.
//
// This is intentionally conservative: symlink creation/replacement, special
// files, concurrent changes to the real workspace, an unavailable Trash
// adapter, or an unapplied sandbox all fail closed without committing raw
// writes. Deleting an existing symlink is safe because the recovery bundle is
// itself a normal directory moved to Trash.
type WorkspaceExecutionBroker struct {
	stagingParent string
	trashAdapter  trash.TrashAdapter

	mu       sync.Mutex
	sessions map[string]*workspaceExecutionSession
}

type workspaceExecutionSession struct {
	mu               sync.Mutex
	request          ExecutionBrokerRequest
	authorization    BrokerAuthorization
	workspaceRoot    string
	stageRoot        string
	stageWorkspace   string
	writableRoots    []string
	protectedRoots   []string
	originalManifest workspaceManifest
	completed        bool
	receipt          BrokerExecutionReceipt
	completionErr    error
}

type workspaceEntryKind string

const (
	workspaceEntryDirectory workspaceEntryKind = "directory"
	workspaceEntryFile      workspaceEntryKind = "file"
	workspaceEntrySymlink   workspaceEntryKind = "symlink"
)

type workspaceEntry struct {
	Kind             workspaceEntryKind
	Mode             fs.FileMode
	Size             int64
	ModifiedUnixNano int64
	Target           string
}

type workspaceManifest map[string]workspaceEntry

type workspaceCommitPlan struct {
	trashRoots  []string
	directories []string
	files       []string
	changes     map[string]string
}

func NewWorkspaceExecutionBroker() *WorkspaceExecutionBroker {
	return NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{})
}

func NewWorkspaceExecutionBrokerWithOptions(options WorkspaceExecutionBrokerOptions) *WorkspaceExecutionBroker {
	adapter := options.TrashAdapter
	if adapter == nil {
		adapter = trash.DefaultAdapter()
	}
	return &WorkspaceExecutionBroker{
		stagingParent: strings.TrimSpace(options.StagingParent),
		trashAdapter:  adapter,
		sessions:      make(map[string]*workspaceExecutionSession),
	}
}

func (*WorkspaceExecutionBroker) ID() string { return workspaceExecutionBrokerID }

func (*WorkspaceExecutionBroker) EnforcesWorkspaceScope() bool { return true }

// CanonicalBrokerWorkspaceScope projects the established PermissionContext
// semantics into the staging broker. PermissionContext.Normalize supplies the
// historical empty-WritableRoots default (the workspace root). Writable roots
// outside the workspace are intentionally omitted: this broker cannot stage or
// publish them, and WorkspaceScopeDeclared preserves an empty workspace-local
// set without widening it back to the workspace root.
func CanonicalBrokerWorkspaceScope(permissionContext PermissionContext) (BrokerWorkspaceScope, error) {
	normalized := permissionContext.Normalize()
	workspaceRoot, _, err := normalizeWorkspaceExecutionPaths(normalized.WorkspaceRoot, normalized.WorkspaceRoot)
	if err != nil {
		return BrokerWorkspaceScope{}, err
	}

	writableRoots := make([]string, 0, len(normalized.WritableRoots))
	for _, root := range normalized.WritableRoots {
		realRoot, resolveErr := ResolvePathWithExistingAncestor(root)
		if resolveErr != nil {
			return BrokerWorkspaceScope{}, fmt.Errorf("resolve writable root %q: %w", root, resolveErr)
		}
		if !PathWithinRoot(realRoot, workspaceRoot) {
			continue
		}
		relative, relErr := filepath.Rel(workspaceRoot, realRoot)
		if relErr != nil || relativePathEscapes(relative) {
			return BrokerWorkspaceScope{}, fmt.Errorf("project writable root %q into workspace", root)
		}
		writableRoots = append(writableRoots, filepath.Clean(relative))
	}

	protectedRoots := make([]string, 0, len(normalized.ProtectedRoots)*2)
	for _, root := range normalized.ProtectedRoots {
		// Preserve lexical containment against the PermissionContext's lexical
		// workspace root. On macOS, /var and /tmp may resolve to /private/*;
		// comparing the lexical root to the already-real workspace root would
		// otherwise silently discard a valid protected alias.
		if relative, relErr := filepath.Rel(normalized.WorkspaceRoot, root); relErr == nil && !relativePathEscapes(relative) {
			protectedRoots = append(protectedRoots, filepath.Clean(relative))
		}
		if realRoot, resolveErr := ResolvePathWithExistingAncestor(root); resolveErr == nil && PathWithinRoot(realRoot, workspaceRoot) {
			relative, relErr := filepath.Rel(workspaceRoot, realRoot)
			if relErr != nil || relativePathEscapes(relative) {
				return BrokerWorkspaceScope{}, fmt.Errorf("project resolved protected root %q into workspace", root)
			}
			protectedRoots = append(protectedRoots, filepath.Clean(relative))
		}
	}

	protectedRoots = dedupeStrings(protectedRoots)
	sort.Strings(protectedRoots)
	return BrokerWorkspaceScope{
		WritableRoots:  topLevelPaths(dedupeStrings(writableRoots)),
		ProtectedRoots: protectedRoots,
	}, nil
}

func (b *WorkspaceExecutionBroker) Authorize(ctx context.Context, request ExecutionBrokerRequest) (BrokerAuthorization, error) {
	if err := ctx.Err(); err != nil {
		return BrokerAuthorization{}, err
	}
	request = cloneExecutionBrokerRequest(request)
	if request.DependencyInstall != nil && request.Operation != BrokerOperationDependencyInstall {
		return deniedWorkspaceAuthorization(request, "typed dependency plan was attached to a non-dependency operation", "dependency_install_operation_mismatch"), ErrExecutionBrokerDenied
	}
	if request.GitRemote != nil && request.Operation != BrokerOperationGitRemote {
		return deniedWorkspaceAuthorization(request, "typed Git projection was attached to a non-Git operation", "git_remote_operation_mismatch"), ErrExecutionBrokerDenied
	}
	if request.ArtifactPublish != nil && request.Operation != BrokerOperationArtifactPublish {
		return deniedWorkspaceAuthorization(request, "host artifact projection was attached to a non-artifact operation", "artifact_publish_operation_mismatch"), ErrExecutionBrokerDenied
	}
	if request.Operation == BrokerOperationDependencyInstall {
		if err := validateBrokerDependencyInstallRequest(request); err != nil {
			return deniedWorkspaceAuthorization(request, err.Error(), "invalid_typed_dependency_install_request"), ErrExecutionBrokerDenied
		}
	}
	if request.Operation == BrokerOperationGitRemote {
		if err := validateBrokerGitRemoteRequest(request); err != nil {
			return deniedWorkspaceAuthorization(request, err.Error(), "invalid_typed_git_remote_request"), ErrExecutionBrokerDenied
		}
	}
	if request.Operation == BrokerOperationArtifactPublish {
		if err := validateBrokerArtifactPublishRequest(request); err != nil {
			return deniedWorkspaceAuthorization(request, err.Error(), "invalid_host_artifact_publish_request"), ErrExecutionBrokerDenied
		}
	}
	if request.Operation == BrokerOperationDelete {
		return BrokerAuthorization{
			AuthorizationID: brokerAuthorizationIdentity(request),
			Action:          BrokerTransform,
			Reason:          "explicit deletion must use trash_file",
			EnforcementMode: workspaceExecutionBrokerID,
			Transform: &BrokerTransformProposal{
				Kind: BrokerTransformTrashPaths, Paths: normalizeBrokerPaths(request.ReferencedPaths), ToolName: "trash_file",
			},
			IssuedAt: time.Now().UTC(),
		}, ErrExecutionTransformRequired
	}
	if request.Operation == BrokerOperationNetwork {
		return BrokerAuthorization{
			AuthorizationID: brokerAuthorizationIdentity(request),
			Action:          BrokerDeny,
			Reason:          "raw network execution has no recipient-observable egress gateway; use a typed web/MCP/connector provider",
			EnforcementMode: workspaceExecutionBrokerID,
			Evidence:        []string{"network_denied_without_egress_gateway"},
			IssuedAt:        time.Now().UTC(),
		}, ErrExecutionBrokerDenied
	}

	workspaceRoot, workingDirectory, err := normalizeWorkspaceExecutionPaths(request.WorkspaceRoot, request.WorkingDirectory)
	if err != nil {
		return BrokerAuthorization{}, fmt.Errorf("%w: %v", ErrExecutionBrokerDenied, err)
	}
	writableRoots, protectedRoots, err := normalizeExecutionBrokerRequestScope(workspaceRoot, request)
	if err != nil {
		return BrokerAuthorization{}, fmt.Errorf("%w: invalid workspace scope: %v", ErrExecutionBrokerDenied, err)
	}
	stageRoot, err := b.newStageRoot()
	if err != nil {
		return BrokerAuthorization{}, fmt.Errorf("%w: create staging root: %v", ErrExecutionBrokerUnavailable, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stageRoot)
		}
	}()
	if relative, relErr := filepath.Rel(workspaceRoot, stageRoot); relErr == nil && !relativePathEscapes(relative) {
		return BrokerAuthorization{}, fmt.Errorf("%w: staging root must be outside the user workspace", ErrExecutionBrokerDenied)
	}

	stageWorkspace := filepath.Join(stageRoot, "workspace")
	copiedManifest, err := copyWorkspaceSnapshot(ctx, workspaceRoot, stageWorkspace)
	if err != nil {
		return BrokerAuthorization{}, fmt.Errorf("%w: snapshot workspace: %v", ErrExecutionBrokerDenied, err)
	}
	currentManifest, err := scanWorkspaceManifest(ctx, workspaceRoot)
	if err != nil {
		return BrokerAuthorization{}, fmt.Errorf("%w: verify workspace snapshot: %v", ErrExecutionBrokerDenied, err)
	}
	if !workspaceManifestsEqual(copiedManifest, currentManifest) {
		return BrokerAuthorization{}, fmt.Errorf("%w: workspace changed while the staging snapshot was created", ErrExecutionWorkspaceChanged)
	}

	relativeWorkingDirectory, err := filepath.Rel(workspaceRoot, workingDirectory)
	if err != nil || relativePathEscapes(relativeWorkingDirectory) {
		return BrokerAuthorization{}, fmt.Errorf("%w: working directory escapes workspace", ErrExecutionBrokerDenied)
	}
	executionWorkingDirectory := filepath.Join(stageWorkspace, relativeWorkingDirectory)
	if info, statErr := os.Stat(executionWorkingDirectory); statErr != nil || !info.IsDir() {
		return BrokerAuthorization{}, fmt.Errorf("%w: staged working directory is not a directory", ErrExecutionBrokerDenied)
	}

	scratchRoot := filepath.Join(stageRoot, "scratch")
	if err := os.MkdirAll(scratchRoot, 0o700); err != nil {
		return BrokerAuthorization{}, fmt.Errorf("%w: create broker scratch: %v", ErrExecutionBrokerUnavailable, err)
	}
	authorizationID := workspaceAuthorizationIdentity(request, stageRoot)
	reason := "command will execute in a private workspace snapshot and commit through recoverable file changes"
	evidence := []string{"private_workspace_snapshot", "applied_platform_sandbox_required", "real_workspace_not_writable_by_child", "network_denied", "deleted_and_replaced_preimages_to_os_trash", "writable_root_commit_guard", "protected_root_commit_guard", "concurrent_workspace_change_guard"}
	sandboxTier := "workspace_mutation"
	requiresAppliedSandbox := true
	if request.Operation == BrokerOperationDependencyInstall {
		reason = "a host-generated dependency plan will execute as direct argv in private staging; only exit zero may commit"
		evidence = []string{
			"typed_dependency_install_operation", "host_generated_plan_identity_verified", "current_environment_snapshot_bound",
			"direct_argv_no_shell", "package_scripts_disabled_by_plan", "private_workspace_snapshot",
			"applied_platform_sandbox_required", "open_world_network_with_stage_only_writes",
			"nonzero_timeout_cancel_discard", "writable_root_commit_guard", "protected_root_commit_guard",
			"concurrent_workspace_change_guard", "deleted_and_replaced_preimages_to_os_trash",
		}
		sandboxTier = "open_world_system"
	} else if request.Operation == BrokerOperationGitRemote {
		reason = "a pinned builtin Git executable will perform an HTTPS-only remote read in private staging"
		evidence = []string{
			"typed_git_remote_operation", "current_run_environment_snapshot_bound", "host_trusted_git_toolchain",
			"git_executable_fingerprint_bound", "git_https_helper_fingerprint_bound", "task_controlled_path_entries_removed",
			"direct_argv_no_shell", "https_only_no_inline_credentials", "credential_helpers_and_interaction_disabled",
			"hooks_filters_and_submodules_disabled", "private_workspace_snapshot", "applied_platform_sandbox_required",
			"open_world_network_with_stage_only_writes", "nonzero_timeout_cancel_discard",
			"writable_root_commit_guard", "protected_root_commit_guard", "concurrent_workspace_change_guard",
			"deleted_and_replaced_preimages_to_os_trash",
		}
		sandboxTier = "open_world_system"
	} else if request.Operation == BrokerOperationArtifactPublish {
		reason = "host-validated provider bytes will be written to one exact path in a private workspace snapshot and committed recoverably"
		evidence = []string{
			"host_artifact_publish_operation", "provider_path_not_accepted_as_publication", "exact_digest_and_size_bound",
			"single_regular_file_delta", "private_workspace_snapshot", "host_only_stage_write", "no_child_process",
			"writable_root_commit_guard", "protected_root_commit_guard", "concurrent_workspace_change_guard",
			"deleted_and_replaced_preimages_to_os_trash",
		}
		sandboxTier = "host_artifact_publish"
		requiresAppliedSandbox = false
	}
	authorization := BrokerAuthorization{
		AuthorizationID:           authorizationID,
		Action:                    BrokerAllow,
		Reason:                    reason,
		EnforcementMode:           workspaceExecutionBrokerID,
		Evidence:                  evidence,
		RequiresAppliedSandbox:    requiresAppliedSandbox,
		SandboxRoot:               stageRoot,
		SandboxTier:               sandboxTier,
		ExecutionWorkspaceRoot:    stageWorkspace,
		ExecutionWorkingDirectory: executionWorkingDirectory,
		EnvironmentOverrides:      workspaceScratchEnvironment(scratchRoot),
		IssuedAt:                  time.Now().UTC(),
	}

	b.mu.Lock()
	if _, exists := b.sessions[authorizationID]; exists {
		b.mu.Unlock()
		return BrokerAuthorization{}, fmt.Errorf("%w: duplicate authorization identity", ErrExecutionBrokerUnavailable)
	}
	b.sessions[authorizationID] = &workspaceExecutionSession{
		request: request, authorization: cloneBrokerAuthorization(authorization),
		workspaceRoot: workspaceRoot, stageRoot: stageRoot, stageWorkspace: stageWorkspace,
		writableRoots: append([]string(nil), writableRoots...), protectedRoots: append([]string(nil), protectedRoots...),
		originalManifest: currentManifest,
	}
	b.mu.Unlock()
	cleanup = false
	return authorization, nil
}

func (b *WorkspaceExecutionBroker) Complete(ctx context.Context, authorization BrokerAuthorization, result BrokerExecutionResult) (BrokerExecutionReceipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	session := b.sessions[authorization.AuthorizationID]
	b.mu.Unlock()
	if session == nil {
		return BrokerExecutionReceipt{}, fmt.Errorf("%w: unknown authorization", ErrExecutionBrokerDenied)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.completed {
		receipt, completionErr := cloneBrokerReceipt(session.receipt), session.completionErr
		return receipt, completionErr
	}

	receipt, completionErr := b.completeSession(ctx, session, result)
	if !session.completed {
		session.completed = true
		session.receipt = cloneBrokerReceipt(receipt)
		session.completionErr = completionErr
	}
	receipt, completionErr = cloneBrokerReceipt(session.receipt), session.completionErr
	return receipt, completionErr
}

func (b *WorkspaceExecutionBroker) completeSession(ctx context.Context, session *workspaceExecutionSession, result BrokerExecutionResult) (BrokerExecutionReceipt, error) {
	receipt := BrokerExecutionReceipt{
		AuthorizationID: session.authorization.AuthorizationID,
		BrokerID:        b.ID(), EnforcementMode: session.authorization.EnforcementMode,
		Action: session.authorization.Action, Evidence: append([]string(nil), session.authorization.Evidence...),
		WorkspaceRoot: session.workspaceRoot, SideEffectState: result.SideEffectState,
		CompletedAt: time.Now().UTC(),
	}
	if session.authorization.Action != BrokerAllow {
		_ = os.RemoveAll(session.stageRoot)
		return receipt, ErrExecutionBrokerDenied
	}
	if result.DiscardWorkspace {
		_ = os.RemoveAll(session.stageRoot)
		receipt.CommitStatus = "discarded_by_typed_executor"
		receipt.SideEffectState = OutcomeSideEffectNone
		receipt.Evidence = append(receipt.Evidence, "staging_discarded_without_live_publish")
		if reason := sanitizeProbeText(result.DiscardReason, 160); reason != "" {
			receipt.Evidence = append(receipt.Evidence, "discard_reason:"+reason)
		}
		return receipt, nil
	}
	completed := result.TransportStatus == TransportCompleted && result.ProcessStatus == ProcessExited
	if session.request.Operation == BrokerOperationArtifactPublish {
		completed = result.TransportStatus == TransportCompleted && result.ProcessStatus == ProcessNotApplicable
	}
	if !completed {
		_ = os.RemoveAll(session.stageRoot)
		receipt.CommitStatus = "discarded_not_completed"
		return receipt, nil
	}

	sealedRoot := session.stageRoot + ".sealed"
	if err := os.Rename(session.stageRoot, sealedRoot); err != nil {
		_ = os.RemoveAll(session.stageRoot)
		receipt.CommitStatus = "failed_to_seal"
		return receipt, fmt.Errorf("%w: seal staging workspace: %v", ErrExecutionBrokerCommit, err)
	}
	session.stageRoot = sealedRoot
	session.stageWorkspace = filepath.Join(sealedRoot, "workspace")
	defer func() { _ = os.RemoveAll(sealedRoot) }()

	stagedManifest, err := scanWorkspaceManifest(ctx, session.stageWorkspace)
	if err != nil {
		receipt.CommitStatus = "invalid_staged_workspace"
		return receipt, fmt.Errorf("%w: inspect staged workspace: %v", ErrExecutionBrokerCommit, err)
	}
	plan, err := buildWorkspaceCommitPlan(session.originalManifest, stagedManifest)
	if err != nil {
		receipt.CommitStatus = "unsupported_delta"
		return receipt, fmt.Errorf("%w: %v", ErrExecutionBrokerCommit, err)
	}
	if session.request.Operation == BrokerOperationArtifactPublish {
		if err := validateBrokerArtifactPublishDelta(ctx, session, stagedManifest, plan); err != nil {
			receipt.Action = BrokerDeny
			receipt.CommitStatus = "invalid_host_artifact_delta"
			receipt.SideEffectState = OutcomeSideEffectNone
			receipt.Evidence = append(receipt.Evidence, "artifact_delta_not_exact", "live_workspace_unchanged", "trash_not_invoked")
			return receipt, fmt.Errorf("%w: %v", ErrExecutionBrokerDenied, err)
		}
	}
	if violation := validateWorkspaceCommitPlanScope(plan, session.writableRoots, session.protectedRoots); violation != nil {
		receipt.Action = BrokerDeny
		receipt.CommitStatus = violation.commitStatus
		receipt.SideEffectState = OutcomeSideEffectNone
		receipt.Evidence = append(receipt.Evidence, violation.evidence, "live_workspace_unchanged", "trash_not_invoked")
		return receipt, fmt.Errorf("%w: %s", ErrExecutionBrokerDenied, violation.reason)
	}

	currentManifest, err := scanWorkspaceManifest(ctx, session.workspaceRoot)
	if err != nil {
		receipt.CommitStatus = "workspace_recheck_failed"
		return receipt, fmt.Errorf("%w: inspect real workspace: %v", ErrExecutionBrokerCommit, err)
	}
	if !workspaceManifestsEqual(session.originalManifest, currentManifest) {
		receipt.CommitStatus = "concurrent_workspace_change"
		return receipt, fmt.Errorf("%w: refusing to overwrite concurrent user changes", ErrExecutionWorkspaceChanged)
	}
	if len(plan.changes) == 0 {
		receipt.CommitStatus = "no_workspace_changes"
		if session.request.Operation == BrokerOperationNetwork || session.request.Operation == BrokerOperationUnknown {
			receipt.SideEffectState = OutcomeSideEffectUnknown
		} else {
			receipt.SideEffectState = OutcomeSideEffectNone
		}
		return receipt, nil
	}

	changes, err := b.applyWorkspaceCommit(ctx, session, stagedManifest, plan)
	receipt.WorkspaceChanges = changes
	receipt.CompletedAt = time.Now().UTC()
	if err != nil {
		receipt.CommitStatus = "partial"
		receipt.SideEffectState = OutcomeSideEffectPartial
		return receipt, fmt.Errorf("%w: %v", ErrExecutionBrokerCommit, err)
	}
	receipt.CommitStatus = "committed"
	receipt.SideEffectState = OutcomeSideEffectApplied
	return receipt, nil
}

func deniedWorkspaceAuthorization(request ExecutionBrokerRequest, reason string, evidence string) BrokerAuthorization {
	return BrokerAuthorization{
		AuthorizationID: brokerAuthorizationIdentity(request), Action: BrokerDeny,
		Reason: sanitizeProbeText(reason, 512), EnforcementMode: workspaceExecutionBrokerID,
		Evidence: []string{evidence, "transport_not_started"}, IssuedAt: time.Now().UTC(),
	}
}

func validateBrokerDependencyInstallRequest(request ExecutionBrokerRequest) error {
	if request.ProviderID != ProviderID("builtin:dependency_install") || request.Capability != CapabilityDependencyInstall {
		return fmt.Errorf("dependency install operation is reserved for builtin:dependency_install with dependency.install capability")
	}
	if strings.TrimSpace(request.Command) != "" {
		return fmt.Errorf("typed dependency install cannot carry a command string")
	}
	projection := request.DependencyInstall
	if projection == nil {
		return fmt.Errorf("typed dependency install requires a validated plan projection")
	}
	plan := projection.Plan
	if strings.TrimSpace(plan.PlanID) == "" || plan.PlanID != candidateInstallPlanIdentity(plan) {
		return fmt.Errorf("candidate install plan identity is invalid")
	}
	if plan.RequirementFingerprint != dependencyRequirementFingerprint(projection.Requirement) {
		return fmt.Errorf("candidate install plan is not bound to the projected dependency requirement")
	}
	if strings.TrimSpace(plan.ExecutableFingerprint) == "" {
		return fmt.Errorf("candidate install plan lacks executable fingerprint")
	}
	if strings.TrimSpace(request.EnvironmentSnapshotID) == "" || plan.EnvironmentID != request.EnvironmentSnapshotID {
		return fmt.Errorf("candidate install plan is not bound to the requested environment snapshot")
	}
	if plan.InstallScope != InstallWorkspace || plan.RequiresAdmin {
		return fmt.Errorf("only non-admin workspace dependency plans are supported")
	}
	if plan.Provenance != "host_dependency_resolver_v1" {
		return fmt.Errorf("candidate install plan provenance is not trusted")
	}
	if strings.TrimSpace(plan.Executable) == "" || len(plan.Args) == 0 || strings.TrimSpace(plan.CWD) == "" {
		return fmt.Errorf("candidate install plan executable, argv, and cwd are required")
	}
	if projection.Requirement.Kind != DependencyLanguagePackage {
		return fmt.Errorf("workspace dependency install currently supports only explicit language packages")
	}
	if strings.TrimSpace(projection.ToolSnapshotID) == "" || len(projection.Providers) == 0 {
		return fmt.Errorf("candidate install plan lacks pinned tool/provider provenance")
	}
	for _, provider := range projection.Providers {
		if strings.TrimSpace(string(provider.ProviderID)) == "" || strings.TrimSpace(provider.ToolName) == "" || strings.TrimSpace(provider.RegistrationID) == "" {
			return fmt.Errorf("candidate install plan contains incomplete provider provenance")
		}
	}
	return nil
}

func validateBrokerGitRemoteRequest(request ExecutionBrokerRequest) error {
	projection := request.GitRemote
	if projection == nil {
		return fmt.Errorf("typed Git remote operation requires a validated projection")
	}
	if request.ProviderID != ProviderID("builtin:git_remote") {
		return fmt.Errorf("Git remote operation is reserved for builtin:git_remote")
	}
	if strings.TrimSpace(request.Command) != "" {
		return fmt.Errorf("typed Git remote operation cannot carry a command string")
	}
	if strings.TrimSpace(projection.RemoteURL) == "" || strings.TrimSpace(projection.GitExecutable) == "" ||
		strings.TrimSpace(projection.GitExecutableFingerprint) == "" || strings.TrimSpace(projection.ToolSnapshotID) == "" ||
		strings.TrimSpace(projection.EnvironmentSnapshotID) == "" || strings.TrimSpace(projection.GitExecPath) == "" ||
		strings.TrimSpace(projection.GitRemoteHelper) == "" || strings.TrimSpace(projection.GitRemoteHelperFingerprint) == "" {
		return fmt.Errorf("typed Git projection lacks remote, executable, fingerprint, or snapshot identity")
	}
	if strings.TrimSpace(request.EnvironmentSnapshotID) == "" || request.EnvironmentSnapshotID != projection.EnvironmentSnapshotID {
		return fmt.Errorf("typed Git projection is not bound to the requested environment snapshot")
	}
	if !filepath.IsAbs(projection.GitExecutable) {
		return fmt.Errorf("typed Git executable must be absolute")
	}
	if fingerprint, err := FingerprintExecutable(projection.GitExecutable); err != nil || fingerprint != projection.GitExecutableFingerprint {
		return fmt.Errorf("typed Git executable fingerprint does not match current bytes")
	}
	if !filepath.IsAbs(projection.GitExecPath) || !filepath.IsAbs(projection.GitRemoteHelper) ||
		!PathWithinRoot(projection.GitRemoteHelper, projection.GitExecPath) {
		return fmt.Errorf("typed Git helper path is not inside the trusted exec path")
	}
	if fingerprint, err := FingerprintExecutable(projection.GitRemoteHelper); err != nil || fingerprint != projection.GitRemoteHelperFingerprint {
		return fmt.Errorf("typed Git HTTPS helper fingerprint does not match current bytes")
	}
	if err := validateBrokerGitRemoteURL(projection.RemoteURL); err != nil {
		return err
	}
	if projection.Ref != "" && !validateBrokerGitRef(projection.Ref) {
		return fmt.Errorf("typed Git ref is unsafe")
	}
	if projection.ProxyRouted {
		switch projection.ProxySource {
		case "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy":
		default:
			return fmt.Errorf("typed Git proxy route lacks a recognized host-owned source")
		}
	} else if projection.ProxySource != "" {
		return fmt.Errorf("typed Git proxy source is present without proxy-routed mode")
	}
	switch projection.Operation {
	case "ls_remote":
		if request.Capability != CapabilityGitRemoteRead {
			return fmt.Errorf("ls_remote requires git.remote_read capability")
		}
		if projection.Destination != "" || projection.ReplaceExisting || len(request.WritableRoots) != 0 || !request.WorkspaceScopeDeclared {
			return fmt.Errorf("ls_remote must declare an empty publish scope")
		}
	case "clone":
		if request.Capability != CapabilityGitClone {
			return fmt.Errorf("clone requires git.clone capability")
		}
		destination, ok, err := normalizeBrokerRelativeRoot(projection.Destination)
		if err != nil || !ok || destination == "." {
			return fmt.Errorf("clone requires a non-root workspace-relative destination")
		}
		if !request.WorkspaceScopeDeclared || len(request.WritableRoots) != 1 {
			return fmt.Errorf("clone must declare exactly one writable destination")
		}
		writable, writableOK, writableErr := normalizeBrokerRelativeRoot(request.WritableRoots[0])
		if writableErr != nil || !writableOK || writable != destination {
			return fmt.Errorf("clone writable scope does not exactly match destination")
		}
	default:
		return fmt.Errorf("unsupported typed Git operation %q", projection.Operation)
	}
	return nil
}

func validateBrokerGitRemoteURL(value string) error {
	if strings.TrimSpace(value) != value || strings.Contains(value, "\\") || strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("typed Git URL contains forbidden whitespace or backslash")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Path == "" || parsed.Path == "/" {
		return fmt.Errorf("typed Git URL must be canonical HTTPS without credentials, query, or fragment")
	}
	if parsed.String() != value {
		return fmt.Errorf("typed Git URL is not canonical")
	}
	return nil
}

func validateBrokerGitRef(value string) bool {
	if strings.TrimSpace(value) != value || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.Contains(value, "//") ||
		strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.ContainsAny(value, " ~^:?*[\\\r\n\t") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	return true
}

func validateBrokerArtifactPublishRequest(request ExecutionBrokerRequest) error {
	projection := request.ArtifactPublish
	if projection == nil {
		return fmt.Errorf("host artifact publish requires a validated projection")
	}
	if strings.TrimSpace(string(request.ProviderID)) == "" || !isArtifactPublishCapability(request.Capability) {
		return fmt.Errorf("host artifact publish requires a named artifact-writing provider and capability")
	}
	if strings.TrimSpace(request.Command) != "" || request.DependencyInstall != nil || request.GitRemote != nil {
		return fmt.Errorf("host artifact publish cannot carry executable or unrelated typed projections")
	}
	relative, ok, err := normalizeBrokerRelativeRoot(projection.RelativePath)
	if err != nil || !ok || relative == "." {
		return fmt.Errorf("host artifact publish requires a non-root workspace-relative path")
	}
	if relative != filepath.Clean(projection.RelativePath) {
		return fmt.Errorf("host artifact publish relative path is not canonical")
	}
	digest := strings.TrimSpace(projection.SHA256)
	decodedDigest, err := hex.DecodeString(digest)
	if err != nil || len(decodedDigest) != sha256.Size || strings.ToLower(digest) != digest {
		return fmt.Errorf("host artifact publish requires a lowercase SHA-256 digest")
	}
	if projection.SizeBytes <= 0 || strings.TrimSpace(projection.ContentType) == "" {
		return fmt.Errorf("host artifact publish requires positive size and content type")
	}
	switch strings.TrimSpace(projection.Kind) {
	case "markdown", "pdf", "document", "presentation", "spreadsheet":
	default:
		return fmt.Errorf("host artifact publish kind %q is not publishable", projection.Kind)
	}
	if !request.WorkspaceScopeDeclared || len(request.WritableRoots) == 0 {
		return fmt.Errorf("host artifact publish requires an explicit non-empty workspace scope")
	}
	if len(request.ReferencedPaths) != 1 || filepath.Clean(request.ReferencedPaths[0]) != relative {
		return fmt.Errorf("host artifact publish referenced path must exactly match the projection")
	}
	return nil
}

func isArtifactPublishCapability(capability Capability) bool {
	switch capability {
	case CapabilityFileWrite,
		CapabilityFileEdit,
		CapabilityPDFRender,
		CapabilityDocumentCreate,
		CapabilityPresentationCreate,
		CapabilityPresentationEdit,
		CapabilityPresentationRender,
		CapabilitySpreadsheetCreate:
		return true
	default:
		return false
	}
}

func validateBrokerArtifactPublishDelta(ctx context.Context, session *workspaceExecutionSession, staged workspaceManifest, plan workspaceCommitPlan) error {
	projection := session.request.ArtifactPublish
	if projection == nil {
		return fmt.Errorf("artifact projection is missing")
	}
	relative := filepath.Clean(projection.RelativePath)
	entry, exists := staged[relative]
	if !exists || entry.Kind != workspaceEntryFile || entry.Size != projection.SizeBytes {
		return fmt.Errorf("staged artifact is not the projected regular file and size")
	}
	digest, err := sha256File(ctx, filepath.Join(session.stageWorkspace, relative))
	if err != nil {
		return fmt.Errorf("hash staged artifact: %w", err)
	}
	if digest != projection.SHA256 {
		return fmt.Errorf("staged artifact digest does not match the projection")
	}
	if len(plan.changes) == 0 {
		return nil
	}
	if len(plan.files) != 1 || plan.files[0] != relative {
		return fmt.Errorf("artifact publish must change exactly one regular file")
	}
	for path, action := range plan.changes {
		if path == relative {
			if action != "created" && action != "updated" {
				return fmt.Errorf("artifact path has unsupported action %q", action)
			}
			continue
		}
		if action != "created" || staged[path].Kind != workspaceEntryDirectory || !isStrictPathAncestor(path, relative) {
			return fmt.Errorf("artifact publish contains unrelated workspace delta %q", path)
		}
	}
	for _, path := range plan.trashRoots {
		if filepath.Clean(path) != relative {
			return fmt.Errorf("artifact publish would replace or delete unrelated path %q", path)
		}
	}
	return nil
}

func isStrictPathAncestor(parent string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != "." && !relativePathEscapes(relative)
}

func sha256File(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		read, readErr := file.Read(buffer)
		if read > 0 {
			_, _ = hash.Write(buffer[:read])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (b *WorkspaceExecutionBroker) applyWorkspaceCommit(ctx context.Context, session *workspaceExecutionSession, staged workspaceManifest, plan workspaceCommitPlan) ([]BrokerWorkspaceChange, error) {
	changes := make([]BrokerWorkspaceChange, 0, len(plan.changes))
	if len(plan.trashRoots) > 0 {
		recovered, err := b.movePreimagesToRecoveryBundle(ctx, session, plan)
		changes = append(changes, recovered...)
		if err != nil {
			return changes, err
		}
	}

	for _, relative := range plan.directories {
		if err := ctx.Err(); err != nil {
			return changes, err
		}
		destination := filepath.Join(session.workspaceRoot, relative)
		if _, err := os.Lstat(destination); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(destination, 0o700); err != nil {
				return changes, fmt.Errorf("create directory %s: %w", destination, err)
			}
			if plan.changes[relative] == "created" {
				changes = append(changes, BrokerWorkspaceChange{Action: "created", Path: destination})
			}
		} else if err != nil {
			return changes, fmt.Errorf("inspect directory destination %s: %w", destination, err)
		}
	}
	for _, relative := range plan.files {
		if err := ctx.Err(); err != nil {
			return changes, err
		}
		source := filepath.Join(session.stageWorkspace, relative)
		destination := filepath.Join(session.workspaceRoot, relative)
		if err := copyRegularFileNoReplace(ctx, source, destination, staged[relative].Mode); err != nil {
			return changes, err
		}
		if plan.changes[relative] == "created" {
			changes = append(changes, BrokerWorkspaceChange{Action: "created", Path: destination})
		}
	}
	for _, relative := range plan.directories {
		if err := os.Chmod(filepath.Join(session.workspaceRoot, relative), staged[relative].Mode.Perm()); err != nil {
			return changes, fmt.Errorf("apply directory mode %s: %w", relative, err)
		}
		if plan.changes[relative] == "updated" {
			changes = append(changes, BrokerWorkspaceChange{Action: "updated", Path: filepath.Join(session.workspaceRoot, relative)})
		}
	}
	return deduplicateBrokerWorkspaceChanges(changes), nil
}

func (b *WorkspaceExecutionBroker) movePreimagesToRecoveryBundle(ctx context.Context, session *workspaceExecutionSession, plan workspaceCommitPlan) ([]BrokerWorkspaceChange, error) {
	bundle, err := os.MkdirTemp(session.workspaceRoot, "Awaken-Shell-Recovery-")
	if err != nil {
		return nil, fmt.Errorf("create recoverable preimage bundle: %w", err)
	}
	moved := make([]string, 0, len(plan.trashRoots))
	rollback := func() error {
		var rollbackErrors []string
		for index := len(moved) - 1; index >= 0; index-- {
			relative := moved[index]
			from := filepath.Join(bundle, relative)
			to := filepath.Join(session.workspaceRoot, relative)
			if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
				continue
			}
			if err := os.Rename(from, to); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
			}
		}
		_ = os.RemoveAll(bundle)
		if len(rollbackErrors) > 0 {
			return fmt.Errorf("rollback failed: %s", strings.Join(rollbackErrors, "; "))
		}
		return nil
	}

	for _, relative := range plan.trashRoots {
		if err := ctx.Err(); err != nil {
			_ = rollback()
			return nil, err
		}
		from := filepath.Join(session.workspaceRoot, relative)
		to := filepath.Join(bundle, relative)
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			_ = rollback()
			return nil, fmt.Errorf("prepare recovery path %s: %w", relative, err)
		}
		if err := os.Rename(from, to); err != nil {
			_ = rollback()
			return nil, fmt.Errorf("stage recoverable preimage %s: %w", from, err)
		}
		moved = append(moved, relative)
	}
	trashResult, err := b.trashAdapter.TrashPath(bundle)
	if err != nil {
		rollbackErr := rollback()
		if rollbackErr != nil {
			return nil, fmt.Errorf("move recovery bundle to Trash: %w; %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("move recovery bundle to Trash: %w", err)
	}
	changes := make([]BrokerWorkspaceChange, 0, len(moved))
	for _, relative := range moved {
		recoveryPath := ""
		if strings.TrimSpace(trashResult.TrashedPath) != "" {
			recoveryPath = filepath.Join(trashResult.TrashedPath, relative)
		}
		changes = append(changes, BrokerWorkspaceChange{
			Action: plan.changes[relative], Path: filepath.Join(session.workspaceRoot, relative),
			RecoveryPath: recoveryPath, RecoveryMethod: trashResult.Method,
		})
	}
	return changes, nil
}

func (b *WorkspaceExecutionBroker) newStageRoot() (string, error) {
	parent := b.stagingParent
	if parent != "" {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return "", err
		}
	}
	return os.MkdirTemp(parent, "awaken-supervised-")
}

func normalizeWorkspaceExecutionPaths(workspaceRoot string, workingDirectory string) (string, string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", "", fmt.Errorf("workspace root is required")
	}
	absoluteRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", "", err
	}
	absoluteRoot, err = filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("workspace root must be an existing directory")
	}
	if filepath.Clean(absoluteRoot) == filepath.Clean(string(filepath.Separator)) {
		return "", "", fmt.Errorf("filesystem root cannot be staged")
	}
	if strings.TrimSpace(workingDirectory) == "" {
		workingDirectory = absoluteRoot
	}
	absoluteWorkingDirectory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", "", err
	}
	absoluteWorkingDirectory, err = filepath.EvalSymlinks(absoluteWorkingDirectory)
	if err != nil {
		return "", "", fmt.Errorf("resolve working directory: %w", err)
	}
	relative, err := filepath.Rel(absoluteRoot, absoluteWorkingDirectory)
	if err != nil || relativePathEscapes(relative) {
		return "", "", fmt.Errorf("working directory is outside workspace")
	}
	return filepath.Clean(absoluteRoot), filepath.Clean(absoluteWorkingDirectory), nil
}

func normalizeExecutionBrokerRequestScope(workspaceRoot string, request ExecutionBrokerRequest) ([]string, []string, error) {
	writableInputs := append([]string(nil), request.WritableRoots...)
	if !request.WorkspaceScopeDeclared && len(writableInputs) == 0 {
		// Match PermissionContext.Normalize: an omitted writable scope defaults
		// to the workspace root. A declared-but-empty scope means that no path
		// in this staged workspace may be published.
		writableInputs = []string{"."}
	}

	writableRoots := make([]string, 0, len(writableInputs))
	for _, input := range writableInputs {
		cleaned, ok, err := normalizeBrokerRelativeRoot(input)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		realRoot, err := ResolvePathWithExistingAncestor(filepath.Join(workspaceRoot, cleaned))
		if err != nil {
			return nil, nil, fmt.Errorf("resolve writable root %q: %w", input, err)
		}
		if !PathWithinRoot(realRoot, workspaceRoot) {
			return nil, nil, fmt.Errorf("writable root %q resolves outside workspace", input)
		}
		relative, err := filepath.Rel(workspaceRoot, realRoot)
		if err != nil || relativePathEscapes(relative) {
			return nil, nil, fmt.Errorf("writable root %q cannot be projected into workspace", input)
		}
		writableRoots = append(writableRoots, filepath.Clean(relative))
	}

	protectedRoots := make([]string, 0, len(request.ProtectedRoots)*2)
	for _, input := range request.ProtectedRoots {
		cleaned, ok, err := normalizeBrokerRelativeRoot(input)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		// Preserve the lexical location so deleting a protected symlink or a
		// not-yet-created protected root is denied.
		protectedRoots = append(protectedRoots, cleaned)
		if realRoot, resolveErr := ResolvePathWithExistingAncestor(filepath.Join(workspaceRoot, cleaned)); resolveErr == nil && PathWithinRoot(realRoot, workspaceRoot) {
			relative, relErr := filepath.Rel(workspaceRoot, realRoot)
			if relErr != nil || relativePathEscapes(relative) {
				return nil, nil, fmt.Errorf("protected root %q cannot be projected into workspace", input)
			}
			protectedRoots = append(protectedRoots, filepath.Clean(relative))
		}
	}

	protectedRoots = dedupeStrings(protectedRoots)
	sort.Strings(protectedRoots)
	return topLevelPaths(dedupeStrings(writableRoots)), protectedRoots, nil
}

func normalizeBrokerRelativeRoot(value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, nil
	}
	if filepath.IsAbs(value) {
		return "", false, fmt.Errorf("broker workspace root must be relative: %q", value)
	}
	cleaned := filepath.Clean(value)
	if relativePathEscapes(cleaned) {
		return "", false, fmt.Errorf("broker workspace root escapes workspace: %q", value)
	}
	return cleaned, true, nil
}

func relativePathEscapes(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative)
}

func workspaceScratchEnvironment(root string) map[string]string {
	cache := filepath.Join(root, "cache")
	goCache := filepath.Join(cache, "go-build")
	goModCache := filepath.Join(cache, "go-mod")
	_ = os.MkdirAll(goCache, 0o700)
	_ = os.MkdirAll(goModCache, 0o700)
	return map[string]string{
		"TMPDIR": root, "TMP": root, "TEMP": root,
		"XDG_CACHE_HOME":    cache,
		"npm_config_cache":  filepath.Join(cache, "npm"),
		"NPM_CONFIG_CACHE":  filepath.Join(cache, "npm"),
		"YARN_CACHE_FOLDER": filepath.Join(cache, "yarn"),
		"PIP_CACHE_DIR":     filepath.Join(cache, "pip"),
		"GOCACHE":           goCache, "GOMODCACHE": goModCache,
	}
}

func workspaceAuthorizationIdentity(request ExecutionBrokerRequest, stageRoot string) string {
	digest := sha256.Sum256([]byte(brokerAuthorizationIdentity(request) + "\x00" + stageRoot + "\x00" + time.Now().UTC().Format(time.RFC3339Nano)))
	return "workspace_broker_" + hex.EncodeToString(digest[:12])
}

func copyWorkspaceSnapshot(ctx context.Context, sourceRoot string, destinationRoot string) (workspaceManifest, error) {
	if err := os.MkdirAll(destinationRoot, 0o700); err != nil {
		return nil, err
	}
	manifest := make(workspaceManifest)
	var directories []string
	err := filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil || relativePathEscapes(relative) {
			return fmt.Errorf("invalid workspace path %s", path)
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		destination := filepath.Join(destinationRoot, relative)
		switch {
		case info.IsDir():
			if err := os.Mkdir(destination, 0o700); err != nil {
				return err
			}
			manifest[relative] = workspaceEntry{Kind: workspaceEntryDirectory, Mode: info.Mode()}
			directories = append(directories, relative)
		case info.Mode().IsRegular():
			if err := copyRegularFileSnapshot(ctx, path, destination, info); err != nil {
				return err
			}
			manifest[relative] = workspaceEntry{
				Kind: workspaceEntryFile, Mode: info.Mode(), Size: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(),
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, destination); err != nil {
				return err
			}
			manifest[relative] = workspaceEntry{Kind: workspaceEntrySymlink, Mode: info.Mode(), Target: target}
		default:
			return fmt.Errorf("special filesystem object is unsupported: %s", path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(directories, func(i, j int) bool { return pathDepth(directories[i]) > pathDepth(directories[j]) })
	for _, relative := range directories {
		entry := manifest[relative]
		if err := os.Chmod(filepath.Join(destinationRoot, relative), entry.Mode.Perm()); err != nil {
			return nil, err
		}
	}
	return manifest, nil
}

func copyRegularFileSnapshot(ctx context.Context, source string, destination string, info fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cloned, err := cloneWorkspaceRegularFile(source, destination)
	if err != nil {
		return err
	}
	if cloned {
		if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
			return err
		}
		_ = os.Chtimes(destination, info.ModTime(), info.ModTime())
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := copyWithContext(ctx, output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
		return err
	}
	_ = os.Chtimes(destination, info.ModTime(), info.ModTime())
	return nil
}

func scanWorkspaceManifest(ctx context.Context, root string) (workspaceManifest, error) {
	manifest := make(workspaceManifest)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relativePathEscapes(relative) {
			return fmt.Errorf("invalid workspace path %s", path)
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			manifest[relative] = workspaceEntry{Kind: workspaceEntryDirectory, Mode: info.Mode()}
		case info.Mode().IsRegular():
			manifest[relative] = workspaceEntry{
				Kind: workspaceEntryFile, Mode: info.Mode(), Size: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(),
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			manifest[relative] = workspaceEntry{Kind: workspaceEntrySymlink, Mode: info.Mode(), Target: target}
		default:
			return fmt.Errorf("special filesystem object is unsupported: %s", path)
		}
		return nil
	})
	return manifest, err
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 128*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			count, writeErr := destination.Write(buffer[:read])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != read {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func workspaceManifestsEqual(left workspaceManifest, right workspaceManifest) bool {
	if len(left) != len(right) {
		return false
	}
	for path, entry := range left {
		if other, ok := right[path]; !ok || !workspaceEntriesEqual(entry, other) {
			return false
		}
	}
	return true
}

func workspaceEntriesEqual(left workspaceEntry, right workspaceEntry) bool {
	return left.Kind == right.Kind && left.Mode.Perm() == right.Mode.Perm() && left.Size == right.Size &&
		left.ModifiedUnixNano == right.ModifiedUnixNano && left.Target == right.Target
}

func buildWorkspaceCommitPlan(original workspaceManifest, staged workspaceManifest) (workspaceCommitPlan, error) {
	plan := workspaceCommitPlan{changes: make(map[string]string)}
	for path, originalEntry := range original {
		stagedEntry, exists := staged[path]
		if exists && workspaceEntriesEqual(originalEntry, stagedEntry) {
			continue
		}
		if originalEntry.Kind == workspaceEntrySymlink && exists {
			return workspaceCommitPlan{}, fmt.Errorf("symlink replacement is unsupported: %s", path)
		}
		if originalEntry.Kind == workspaceEntryDirectory && exists && stagedEntry.Kind == workspaceEntryDirectory {
			plan.changes[path] = "updated"
			continue
		}
		plan.changes[path] = "deleted"
		if exists {
			plan.changes[path] = "updated"
		}
		plan.trashRoots = append(plan.trashRoots, path)
	}
	plan.trashRoots = topLevelPaths(plan.trashRoots)

	for path, stagedEntry := range staged {
		originalEntry, exists := original[path]
		if exists && workspaceEntriesEqual(originalEntry, stagedEntry) {
			continue
		}
		if stagedEntry.Kind == workspaceEntrySymlink {
			return workspaceCommitPlan{}, fmt.Errorf("symlink creation or replacement is unsupported: %s", path)
		}
		if !exists {
			plan.changes[path] = "created"
		} else if plan.changes[path] == "" {
			plan.changes[path] = "updated"
		}
		switch stagedEntry.Kind {
		case workspaceEntryDirectory:
			plan.directories = append(plan.directories, path)
		case workspaceEntryFile:
			plan.files = append(plan.files, path)
		default:
			return workspaceCommitPlan{}, fmt.Errorf("unsupported staged object: %s", path)
		}
	}
	sort.Slice(plan.trashRoots, func(i, j int) bool { return pathDepth(plan.trashRoots[i]) > pathDepth(plan.trashRoots[j]) })
	sort.Slice(plan.directories, func(i, j int) bool { return pathDepth(plan.directories[i]) < pathDepth(plan.directories[j]) })
	sort.Strings(plan.files)
	return plan, nil
}

type workspaceCommitScopeViolation struct {
	commitStatus string
	evidence     string
	reason       string
}

// validateWorkspaceCommitPlanScope runs after staging is sealed and scanned,
// but before the real workspace is rechecked, any recovery bundle is created,
// Trash is invoked, or a destination is published. Every changed manifest
// entry must be inside a canonical writable root and outside all protected
// roots.
func validateWorkspaceCommitPlanScope(plan workspaceCommitPlan, writableRoots []string, protectedRoots []string) *workspaceCommitScopeViolation {
	paths := make([]string, 0, len(plan.changes))
	for path := range plan.changes {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if relativePathWithinAnyRoot(path, protectedRoots) {
			return &workspaceCommitScopeViolation{
				commitStatus: "denied_protected_root",
				evidence:     "workspace_delta_targets_protected_root",
				reason:       fmt.Sprintf("workspace delta %q targets a protected root", path),
			}
		}
		if !relativePathWithinAnyRoot(path, writableRoots) {
			return &workspaceCommitScopeViolation{
				commitStatus: "denied_writable_scope",
				evidence:     "workspace_delta_outside_writable_roots",
				reason:       fmt.Sprintf("workspace delta %q is outside canonical writable roots", path),
			}
		}
	}
	return nil
}

func relativePathWithinAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) || relativePathEscapes(path) {
		return false
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if filepath.IsAbs(root) || relativePathEscapes(root) {
			continue
		}
		relative, err := filepath.Rel(root, path)
		if err == nil && !relativePathEscapes(relative) {
			return true
		}
	}
	return false
}

func topLevelPaths(paths []string) []string {
	sort.Slice(paths, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(paths[i]), pathDepth(paths[j])
		if leftDepth == rightDepth {
			return paths[i] < paths[j]
		}
		return leftDepth < rightDepth
	})
	var result []string
	for _, candidate := range paths {
		covered := false
		for _, parent := range result {
			relative, err := filepath.Rel(parent, candidate)
			if err == nil && relative != "." && !relativePathEscapes(relative) {
				covered = true
				break
			}
			if candidate == parent {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, candidate)
		}
	}
	return result
}

func pathDepth(path string) int {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return 0
	}
	return strings.Count(path, string(filepath.Separator)) + 1
}

func copyRegularFileNoReplace(ctx context.Context, source string, destination string, mode fs.FileMode) error {
	parent := filepath.Dir(destination)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("destination parent is not a stable directory: %s", parent)
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("destination unexpectedly exists: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination %s: %w", destination, err)
	}
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open staged file %s: %w", source, err)
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".awaken-commit-*")
	if err != nil {
		return fmt.Errorf("create staged destination %s: %w", destination, err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := copyWithContext(ctx, temporary, input); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("copy staged file %s: %w", destination, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync staged file %s: %w", destination, err)
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("chmod staged file %s: %w", destination, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close staged file %s: %w", destination, err)
	}
	// Link is a portable no-replace publication primitive on the supported
	// local filesystems. It cannot silently overwrite a concurrently created
	// user path the way os.Rename can on Unix.
	if err := os.Link(temporaryPath, destination); err != nil {
		return fmt.Errorf("publish staged file without overwrite %s: %w", destination, err)
	}
	return nil
}

func deduplicateBrokerWorkspaceChanges(values []BrokerWorkspaceChange) []BrokerWorkspaceChange {
	seen := make(map[string]int, len(values))
	result := make([]BrokerWorkspaceChange, 0, len(values))
	for _, value := range values {
		key := value.Action + "\x00" + value.Path
		if index, exists := seen[key]; exists {
			if result[index].RecoveryPath == "" {
				result[index].RecoveryPath = value.RecoveryPath
				result[index].RecoveryMethod = value.RecoveryMethod
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Action < result[j].Action
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func cloneBrokerReceipt(value BrokerExecutionReceipt) BrokerExecutionReceipt {
	value.Evidence = append([]string(nil), value.Evidence...)
	value.WorkspaceChanges = append([]BrokerWorkspaceChange(nil), value.WorkspaceChanges...)
	return value
}
