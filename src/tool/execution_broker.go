package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type BrokerOperation string

const (
	BrokerOperationInspect           BrokerOperation = "inspect_read_only"
	BrokerOperationMutation          BrokerOperation = "workspace_mutation"
	BrokerOperationDelete            BrokerOperation = "file_delete"
	BrokerOperationNetwork           BrokerOperation = "network"
	BrokerOperationDependencyInstall BrokerOperation = "dependency_install"
	BrokerOperationGitRemote         BrokerOperation = "git_remote"
	BrokerOperationArtifactPublish   BrokerOperation = "host_artifact_publish"
	BrokerOperationUnknown           BrokerOperation = "unknown"
)

// BrokerDependencyInstallPlan is an immutable, host-validated projection of
// a CandidateInstallPlan. It is populated only by the builtin typed installer;
// model input never controls these fields.
type BrokerDependencyInstallPlan struct {
	Plan           CandidateInstallPlan            `json:"plan"`
	Requirement    DependencyRequirement           `json:"requirement"`
	Providers      []InstallPlanProviderProvenance `json:"providers"`
	ToolSnapshotID string                          `json:"tool_snapshot_id"`
}

// BrokerGitRemotePlan is an immutable projection authored by the builtin
// typed Git provider. Model input never controls executable identity or argv;
// the broker receives only the already-normalized operation subject.
type BrokerGitRemotePlan struct {
	Operation                  string `json:"operation"`
	RemoteURL                  string `json:"remote_url"`
	Ref                        string `json:"ref,omitempty"`
	Destination                string `json:"destination,omitempty"`
	ReplaceExisting            bool   `json:"replace_existing,omitempty"`
	GitExecutable              string `json:"git_executable"`
	GitExecutableFingerprint   string `json:"git_executable_fingerprint"`
	GitExecPath                string `json:"git_exec_path"`
	GitRemoteHelper            string `json:"git_remote_helper"`
	GitRemoteHelperFingerprint string `json:"git_remote_helper_fingerprint"`
	ToolSnapshotID             string `json:"tool_snapshot_id"`
	EnvironmentSnapshotID      string `json:"environment_snapshot_id"`
	ProxyRouted                bool   `json:"proxy_routed,omitempty"`
	ProxySource                string `json:"proxy_source,omitempty"`
}

// BrokerArtifactPublishPlan binds a host-mediated artifact commit to the
// exact provider bytes already validated by the MCP adapter. A remote provider
// cannot populate a local path and claim publication: only the host writes the
// declared digest into the broker's private workspace snapshot.
type BrokerArtifactPublishPlan struct {
	RelativePath string `json:"relative_path"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"size_bytes"`
	ContentType  string `json:"content_type"`
	Kind         string `json:"kind"`
}

type BrokerAction string

const (
	BrokerAllow     BrokerAction = "allow"
	BrokerDeny      BrokerAction = "deny"
	BrokerTransform BrokerAction = "transform"
)

type BrokerTransformKind string

const (
	BrokerTransformTrashPaths BrokerTransformKind = "trash_paths"
)

type BrokerTransformProposal struct {
	Kind     BrokerTransformKind `json:"kind"`
	Paths    []string            `json:"paths"`
	ToolName string              `json:"tool_name"`
}

type ExecutionBrokerRequest struct {
	RunID            string          `json:"run_id,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	ProviderID       ProviderID      `json:"provider_id"`
	Capability       Capability      `json:"capability"`
	Operation        BrokerOperation `json:"operation"`
	Command          string          `json:"command,omitempty"`
	WorkingDirectory string          `json:"working_directory,omitempty"`
	WorkspaceRoot    string          `json:"workspace_root,omitempty"`
	// WorkspaceScopeDeclared distinguishes a deliberately empty set of
	// workspace-local writable roots from legacy/direct callers that omit the
	// scope. Omitted scope retains PermissionContext's established default of
	// allowing the workspace root. When declared, WritableRoots are canonical,
	// symlink-resolved paths relative to WorkspaceRoot. ProtectedRoots contain
	// the protected lexical location plus any workspace-local canonical target.
	WorkspaceScopeDeclared bool                         `json:"workspace_scope_declared,omitempty"`
	WritableRoots          []string                     `json:"writable_roots,omitempty"`
	ProtectedRoots         []string                     `json:"protected_roots,omitempty"`
	ReferencedPaths        []string                     `json:"referenced_paths,omitempty"`
	EnvironmentSnapshotID  string                       `json:"environment_snapshot_id,omitempty"`
	DependencyInstall      *BrokerDependencyInstallPlan `json:"dependency_install,omitempty"`
	GitRemote              *BrokerGitRemotePlan         `json:"git_remote,omitempty"`
	ArtifactPublish        *BrokerArtifactPublishPlan   `json:"artifact_publish,omitempty"`
}

type BrokerAuthorization struct {
	AuthorizationID           string                   `json:"authorization_id"`
	Action                    BrokerAction             `json:"action"`
	Reason                    string                   `json:"reason,omitempty"`
	EnforcementMode           string                   `json:"enforcement_mode"`
	Evidence                  []string                 `json:"evidence,omitempty"`
	Transform                 *BrokerTransformProposal `json:"transform,omitempty"`
	RequiresAppliedSandbox    bool                     `json:"requires_applied_sandbox,omitempty"`
	SandboxRoot               string                   `json:"sandbox_root,omitempty"`
	SandboxTier               string                   `json:"sandbox_tier,omitempty"`
	ExecutionWorkspaceRoot    string                   `json:"execution_workspace_root,omitempty"`
	ExecutionWorkingDirectory string                   `json:"execution_working_directory,omitempty"`
	EnvironmentOverrides      map[string]string        `json:"environment_overrides,omitempty"`
	IssuedAt                  time.Time                `json:"issued_at"`
}

type BrokerExecutionResult struct {
	TransportStatus TransportStatus `json:"transport_status"`
	ProcessStatus   ProcessStatus   `json:"process_status"`
	ExitCode        *int            `json:"exit_code,omitempty"`
	SideEffectState SideEffectState `json:"side_effect_state"`
	// DiscardWorkspace prevents a completed process from publishing its staged
	// delta. Typed dependency installation sets this for non-zero exits so a
	// partially written package tree never reaches the live workspace.
	DiscardWorkspace bool   `json:"discard_workspace,omitempty"`
	DiscardReason    string `json:"discard_reason,omitempty"`
}

type BrokerExecutionReceipt struct {
	AuthorizationID  string                  `json:"authorization_id"`
	BrokerID         string                  `json:"broker_id"`
	EnforcementMode  string                  `json:"enforcement_mode"`
	Action           BrokerAction            `json:"action"`
	Evidence         []string                `json:"evidence,omitempty"`
	CommitStatus     string                  `json:"commit_status,omitempty"`
	WorkspaceRoot    string                  `json:"workspace_root,omitempty"`
	SideEffectState  SideEffectState         `json:"side_effect_state,omitempty"`
	WorkspaceChanges []BrokerWorkspaceChange `json:"workspace_changes,omitempty"`
	CompletedAt      time.Time               `json:"completed_at"`
}

// BrokerWorkspaceChange is the broker-observed, committed workspace delta.
// Updated and deleted entries carry the Trash destination of their preimage,
// proving that replacement never permanently discards the user's old bytes.
type BrokerWorkspaceChange struct {
	Action         string `json:"action"`
	Path           string `json:"path"`
	RecoveryPath   string `json:"recovery_path,omitempty"`
	RecoveryMethod string `json:"recovery_method,omitempty"`
}

type ExecutionBroker interface {
	ID() string
	Authorize(ctx context.Context, request ExecutionBrokerRequest) (BrokerAuthorization, error)
	Complete(ctx context.Context, authorization BrokerAuthorization, result BrokerExecutionResult) (BrokerExecutionReceipt, error)
}

// WorkspaceScopeExecutionBroker marks a broker that actually enforces the
// canonical workspace writable/protected scope for reserved typed operations.
// Bash never consumes or projects into this broker contract.
type WorkspaceScopeExecutionBroker interface {
	ExecutionBroker
	EnforcesWorkspaceScope() bool
}

var (
	ErrExecutionBrokerUnavailable = errors.New("execution broker unavailable")
	ErrExecutionBrokerDenied      = errors.New("execution broker denied execution")
	ErrExecutionTransformRequired = errors.New("execution requires a typed transform")
	ErrExecutionSandboxRequired   = errors.New("execution broker requires an applied platform sandbox")
	ErrExecutionBrokerCommit      = errors.New("execution broker could not safely commit workspace changes")
	ErrExecutionWorkspaceChanged  = errors.New("workspace changed outside the supervised execution")
)

type UnavailableExecutionBroker struct{}

func (*UnavailableExecutionBroker) ID() string { return "unavailable" }
func (*UnavailableExecutionBroker) Authorize(context.Context, ExecutionBrokerRequest) (BrokerAuthorization, error) {
	return BrokerAuthorization{}, ErrExecutionBrokerUnavailable
}
func (*UnavailableExecutionBroker) Complete(context.Context, BrokerAuthorization, BrokerExecutionResult) (BrokerExecutionReceipt, error) {
	return BrokerExecutionReceipt{}, ErrExecutionBrokerUnavailable
}

func brokerAuthorizationIdentity(request ExecutionBrokerRequest) string {
	copyRequest := request
	copyRequest.Command = sanitizeProbeText(copyRequest.Command, 4096)
	data, _ := json.Marshal(copyRequest)
	digest := sha256.Sum256(data)
	return "broker_" + hex.EncodeToString(digest[:12])
}

type executionBrokerContextValue struct {
	Broker ExecutionBroker
}
type executionBrokerContextKey struct{}

// ContextWithExecutionBroker explicitly installs a broker. A nil broker is a
// meaningful fail-closed value and is distinguishable from no override.
func ContextWithExecutionBroker(ctx context.Context, broker ExecutionBroker) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, executionBrokerContextKey{}, executionBrokerContextValue{Broker: broker})
}

func ExecutionBrokerFromContext(ctx context.Context) (ExecutionBroker, bool) {
	if ctx == nil {
		return nil, false
	}
	value, ok := ctx.Value(executionBrokerContextKey{}).(executionBrokerContextValue)
	if !ok {
		return nil, false
	}
	return value.Broker, true
}

func cloneBrokerAuthorization(value BrokerAuthorization) BrokerAuthorization {
	value.Evidence = append([]string(nil), value.Evidence...)
	if value.EnvironmentOverrides != nil {
		value.EnvironmentOverrides = cloneStringMap(value.EnvironmentOverrides)
	}
	if value.Transform != nil {
		transform := *value.Transform
		transform.Paths = append([]string(nil), value.Transform.Paths...)
		value.Transform = &transform
	}
	return value
}

func cloneExecutionBrokerRequest(value ExecutionBrokerRequest) ExecutionBrokerRequest {
	value.WritableRoots = append([]string(nil), value.WritableRoots...)
	value.ProtectedRoots = append([]string(nil), value.ProtectedRoots...)
	value.ReferencedPaths = append([]string(nil), value.ReferencedPaths...)
	if value.DependencyInstall != nil {
		projection := *value.DependencyInstall
		projection.Plan = cloneCandidateInstallPlans([]CandidateInstallPlan{projection.Plan})[0]
		projection.Providers = make([]InstallPlanProviderProvenance, len(value.DependencyInstall.Providers))
		for index, provider := range value.DependencyInstall.Providers {
			provider.Capabilities = append([]Capability(nil), provider.Capabilities...)
			projection.Providers[index] = provider
		}
		value.DependencyInstall = &projection
	}
	if value.GitRemote != nil {
		projection := *value.GitRemote
		value.GitRemote = &projection
	}
	if value.ArtifactPublish != nil {
		projection := *value.ArtifactPublish
		value.ArtifactPublish = &projection
	}
	return value
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
}

func normalizeBrokerPaths(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
