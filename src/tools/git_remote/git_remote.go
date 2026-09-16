package gitremote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"awaken/src/internal/outputlimit"
	"awaken/src/managedenv"
	"awaken/src/tool"
	"awaken/src/tool/sandbox"
)

const (
	toolName                = "git_remote"
	providerID              = "builtin:git_remote"
	operationLSRemote       = "ls_remote"
	operationClone          = "clone"
	fixedTimeout            = 2 * time.Minute
	fixedOutputLimitRunes   = 32000
	maxRemoteRefs           = 1024
	brokerCompletionTimeout = 2 * time.Minute
)

type Tool struct {
	adapter     sandbox.SandboxAdapter
	timeout     time.Duration
	resolveHost func(context.Context, string) ([]net.IP, error)
	lookupEnv   func(string) (string, bool)
	trustGit    func(tool.ExecutableObservation) (gitRuntimeTrust, error)
}

type gitProxyRoute struct {
	Enabled bool
	Source  string
	URL     string
}

type gitRuntimeTrust struct {
	ExecPath          string
	RemoteHelper      string
	HelperFingerprint string
}

var _ tool.OutcomeTool = (*Tool)(nil)

type input struct {
	Operation       string `json:"operation"`
	URL             string `json:"url"`
	Ref             string `json:"ref,omitempty"`
	Destination     string `json:"destination,omitempty"`
	ReplaceExisting bool   `json:"replace_existing,omitempty"`
}

type remoteRef struct {
	ObjectID string `json:"object_id"`
	Name     string `json:"name"`
}

type output struct {
	Operation                  string                      `json:"remote_operation"`
	RemoteURL                  string                      `json:"remote_url"`
	Ref                        string                      `json:"ref,omitempty"`
	Destination                string                      `json:"destination,omitempty"`
	ReplaceExisting            bool                        `json:"replace_existing,omitempty"`
	Refs                       []remoteRef                 `json:"refs,omitempty"`
	Stdout                     string                      `json:"stdout,omitempty"`
	Stderr                     string                      `json:"stderr,omitempty"`
	Truncated                  bool                        `json:"truncated,omitempty"`
	BrokerReceipt              tool.BrokerExecutionReceipt `json:"broker_receipt"`
	ToolSnapshotID             string                      `json:"tool_snapshot_id"`
	EnvironmentSnapshotID      string                      `json:"environment_snapshot_id"`
	GitExecutable              string                      `json:"git_executable"`
	GitExecutableFingerprint   string                      `json:"git_executable_fingerprint"`
	GitExecPath                string                      `json:"git_exec_path"`
	GitRemoteHelperFingerprint string                      `json:"git_remote_helper_fingerprint"`
	ProxyRouted                bool                        `json:"proxy_routed"`
	ProxySource                string                      `json:"proxy_source,omitempty"`
}

func (*Tool) Name() string { return toolName }

func (*Tool) Description() string {
	return "通过当前 run 固定的 Git executable 安全读取公开 HTTPS remote：列出 refs，或浅克隆到 TaskScope workspace；不接受命令、凭据或任意 argv"
}

func (*Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "operation", Description: "ls_remote 或 clone", Type: "string", Required: true},
		{Name: "url", Description: "无凭据、query、fragment 的 HTTPS Git remote URL", Type: "string", Required: true},
		{Name: "ref", Description: "可选的精确 branch/tag/ref 名", Type: "string", Required: false},
		{Name: "destination", Description: "clone 的 workspace-relative 目标目录", Type: "string", Required: false},
		{Name: "replace_existing", Description: "clone 时是否以可恢复方式替换已有目标", Type: "boolean", Required: false},
	}
}

func (*Tool) InputSchema() tool.ToolParametersSchema {
	minLength, maxURL, maxRef, maxDestination := 1, 2048, 255, 512
	return tool.ToolParametersSchema{
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"operation":        {Type: "string", Enum: []any{operationLSRemote, operationClone}, Description: "ls_remote 或 clone"},
			"url":              {Type: "string", MinLength: &minLength, MaxLength: &maxURL, Description: "无凭据、query、fragment 的 HTTPS Git remote URL"},
			"ref":              {Type: "string", MinLength: &minLength, MaxLength: &maxRef, Description: "可选的精确 branch/tag/ref 名"},
			"destination":      {Type: "string", MinLength: &minLength, MaxLength: &maxDestination, Description: "clone 的 workspace-relative 目标目录"},
			"replace_existing": {Type: "boolean", Description: "是否以 Trash 可恢复方式替换已有 clone 目标"},
		},
		Required: []string{"operation", "url"}, AdditionalProperties: tool.BoolPtr(false),
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	refSchema := tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
		"object_id": {Type: "string"}, "name": {Type: "string"},
	}, Required: []string{"object_id", "name"}, AdditionalProperties: tool.BoolPtr(false)}
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: toolName, DisplayName: "Git Remote", Description: t.Description(),
		Purpose: "为普通公开 Git 外部读取提供宿主拥有、零审批、HTTPS-only 的 typed 路径，同时保持 raw Shell network fail-closed。",
		WhenToUse: []string{
			"需要读取公开 HTTPS Git remote 的 refs，且不需要认证时",
			"需要把公开 repository 的单分支浅克隆安全发布到 TaskScope workspace 时",
		},
		WhenNotToUse: []string{
			"不得用于 SSH、git、file、HTTP remote、inline credentials、query/fragment 或私有仓库认证",
			"不得用于 push、fetch 到现有 repository、submodule、任意 Git argv 或修改已有 repository 配置",
			"平台没有真实 full sandbox 时不会启动 Git；不要回退到 raw Shell network",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"remote_operation": {Type: "string"}, "remote_url": {Type: "string"}, "ref": {Type: "string"},
			"destination": {Type: "string"}, "replace_existing": {Type: "boolean"},
			"refs": {Type: "array", Items: &refSchema}, "stdout": {Type: "string"}, "stderr": {Type: "string"},
			"broker_receipt":   {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
			"tool_snapshot_id": {Type: "string"}, "environment_snapshot_id": {Type: "string"},
			"git_executable": {Type: "string"}, "git_executable_fingerprint": {Type: "string"},
			"git_exec_path": {Type: "string"}, "git_remote_helper_fingerprint": {Type: "string"},
			"proxy_routed": {Type: "boolean"}, "proxy_source": {Type: "string"},
		}, Required: []string{
			"remote_operation", "remote_url", "broker_receipt", "tool_snapshot_id", "environment_snapshot_id",
			"git_executable", "git_executable_fingerprint", "git_exec_path", "git_remote_helper_fingerprint", "proxy_routed",
		}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectExecute, Resources: []string{"pinned Git process"}, Description: "以 direct argv 执行当前 run 快照中的 Git，不经过 shell。", Reversibility: tool.ReversibilityNotApplicable},
			{Kind: tool.SideEffectNetwork, Resources: []string{"one explicit HTTPS Git remote"}, Description: "只允许输入中明确的无凭据 HTTPS remote；禁用交互、credential helper、非 HTTPS protocol 和 redirect。", Reversibility: tool.ReversibilityNotApplicable},
			{Kind: tool.SideEffectUpdate, Resources: []string{"one task-scoped clone destination"}, Description: "仅 clone 可写 private staging 中的单一目标；成功后 broker 发布，覆盖前版本进入 Trash。", Reversibility: tool.ReversibilityConditional},
		},
		Examples: []tool.ToolExample{
			{Scenario: "列出公开 remote 的 main branch", Input: map[string]any{"operation": operationLSRemote, "url": "https://github.com/example/project.git", "ref": "refs/heads/main"}, Outcome: "返回有界、结构化 refs，不修改 workspace"},
			{Scenario: "浅克隆公开 repository", Input: map[string]any{"operation": operationClone, "url": "https://github.com/example/project.git", "destination": "vendor/project"}, Outcome: "成功时仅发布 vendor/project；失败时 live workspace 不变"},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeValidationError, Cause: "operation、URL、ref 或 destination 不符合严格边界", Retryable: false, Recovery: []string{"使用公开 HTTPS URL 和 TaskScope-relative destination"}},
			{Code: "git_snapshot_invalid", Cause: "当前 run 没有匹配的 Git path/fingerprint/provider snapshot", Retryable: false, Recovery: []string{"显式刷新 run availability"}},
			{Code: tool.ErrorCodeSupervisedExecutionRequired, Cause: "平台不能应用真实 full sandbox/staging", Retryable: false, Recovery: []string{"使用支持 full sandbox 的平台；不得改用 raw Shell network"}},
			{Code: tool.ErrorCodeProcessExitNonzero, Cause: "remote/ref 不存在、TLS/network 失败或 Git 非零退出", Retryable: false, Recovery: []string{"检查有界 stderr 和 remote/ref；clone staging 已丢弃"}},
			{Code: tool.ErrorCodeProcessTimeout, Cause: "固定时限到期", Retryable: false, Recovery: []string{"检查 remote 可达性或 repository 大小；staging 已丢弃"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: false, Destructive: true, OpenWorld: true,
			CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite, NetworkScope: tool.NetworkScopeRestricted,
		},
		ProviderID:   tool.ProviderID(providerID),
		Capabilities: []tool.Capability{tool.CapabilityGitRemoteRead, tool.CapabilityGitClone},
		Dependencies: []tool.DependencyRequirement{{Kind: tool.DependencyOSExecutable, Name: "git", Source: "host_environment"}},
	})
}

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, raw, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return outcome.Content, tool.ToolError{
			Code: outcome.ErrorCode, Category: outcome.ErrorCategory, Message: outcome.ErrorMessage,
			Retryable: outcome.Retryable, RecoveryOptions: outcome.RecoveryOptions, Diagnostic: outcome.Diagnostic,
		}
	}
	return outcome.Content, nil
}

func (t *Tool) ExecuteOutcome(ctx context.Context, raw string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseInput(raw)
	if err != nil {
		return failedBeforeTransport(started, err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, tool.OutcomeFailed, []string{"provide_only_strict_git_remote_fields"}), nil
	}
	params.URL, err = normalizeRemoteURL(params.URL)
	if err != nil {
		return failedBeforeTransport(started, err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, tool.OutcomeDenied, []string{"use_public_https_url_without_credentials_query_or_fragment"}), nil
	}
	proxyRoute, err := t.validateRemoteRoute(ctx, params.URL)
	if err != nil {
		return failedBeforeTransport(started, err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"use_a_public_remote_host_whose_current_DNS_answers_are_all_public"}), nil
	}
	if params.Ref != "" && !validGitRef(params.Ref) {
		return failedBeforeTransport(started, "ref is not a safe exact Git branch/tag/ref name", tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, tool.OutcomeDenied, []string{"use_an_exact_branch_tag_or_refs_path"}), nil
	}

	snapshot, environment, ok := tool.RunAvailabilityFromContext(ctx)
	if !ok {
		return failedBeforeTransport(started, "run_availability_snapshot_missing: typed Git requires pinned tool and environment snapshots", "run_availability_snapshot_missing", tool.ErrorCategoryRuntime, tool.OutcomeUnsupported, []string{"start_or_refresh_run_availability"}), nil
	}
	gitObservation, gitTrust, err := t.validatePinnedGit(ctx, snapshot, environment)
	if err != nil {
		return failedBeforeTransport(started, err.Error(), "git_snapshot_invalid", tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"refresh_run_availability", "restore_pinned_git_executable"}), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx).Normalize()
	destinationRelative := ""
	if params.Operation == operationClone {
		resolved, relative, destinationErr := validateCloneDestination(params.Destination, permissionContext)
		if destinationErr != nil {
			return failedBeforeTransport(started, destinationErr.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"choose_a_non_sensitive_task_scope_destination_with_existing_parent"}), nil
		}
		params.Destination = resolved
		destinationRelative = relative
	}

	activeSandbox := t.adapter
	if activeSandbox == nil {
		activeSandbox = sandbox.DefaultAdapter()
	}
	if activeSandbox == nil || activeSandbox.SupportLevel() != sandbox.SandboxFull {
		return failedBeforeTransport(started, "supervised_execution_required: typed Git remote access requires a real full platform sandbox", tool.ErrorCodeSupervisedExecutionRequired, tool.ErrorCategoryUnsupported, tool.OutcomeUnsupported, []string{"use_supported_full_sandbox_platform", "do_not_use_raw_shell_network"}), nil
	}

	broker, explicitBroker := tool.ExecutionBrokerFromContext(ctx)
	if !explicitBroker {
		broker = tool.NewWorkspaceExecutionBroker()
	}
	if broker == nil {
		return failedBeforeTransport(started, "execution_broker_unavailable: typed Git requires workspace staging", tool.ErrorCodeExecutionBrokerUnavailable, tool.ErrorCategoryRuntime, tool.OutcomeUnsupported, []string{"restore_workspace_execution_broker"}), nil
	}
	brokerScope, err := tool.CanonicalBrokerWorkspaceScope(permissionContext)
	if err != nil {
		return failedBeforeTransport(started, "canonicalize Git scope: "+err.Error(), "git_scope_invalid", tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"correct_task_scope"}), nil
	}
	writableRoots := []string(nil)
	capability := tool.CapabilityGitRemoteRead
	if params.Operation == operationClone {
		writableRoots = []string{destinationRelative}
		capability = tool.CapabilityGitClone
	}
	projection := &tool.BrokerGitRemotePlan{
		Operation: params.Operation, RemoteURL: params.URL, Ref: params.Ref, Destination: destinationRelative,
		ReplaceExisting: params.ReplaceExisting, GitExecutable: gitObservation.Path,
		GitExecutableFingerprint: gitObservation.Fingerprint, ToolSnapshotID: snapshot.SnapshotID,
		GitExecPath: gitTrust.ExecPath, GitRemoteHelper: gitTrust.RemoteHelper,
		GitRemoteHelperFingerprint: gitTrust.HelperFingerprint,
		EnvironmentSnapshotID:      environment.SnapshotID, ProxyRouted: proxyRoute.Enabled, ProxySource: proxyRoute.Source,
	}
	identity, _ := tool.ToolExecutionIdentityFromContext(ctx)
	authorization, authorizeErr := broker.Authorize(ctx, tool.ExecutionBrokerRequest{
		RunID: identity.RunID, ToolCallID: identity.ToolCallID,
		ProviderID: tool.ProviderID(providerID), Capability: capability, Operation: tool.BrokerOperationGitRemote,
		WorkingDirectory: permissionContext.WorkspaceRoot, WorkspaceRoot: permissionContext.WorkspaceRoot,
		WorkspaceScopeDeclared: true, WritableRoots: writableRoots, ProtectedRoots: brokerScope.ProtectedRoots,
		EnvironmentSnapshotID: environment.SnapshotID, GitRemote: projection,
	})
	if authorizeErr != nil || authorization.Action != tool.BrokerAllow {
		message := "execution broker denied typed Git remote access"
		if authorizeErr != nil {
			message += ": " + authorizeErr.Error()
		}
		return failedBeforeTransport(started, message, tool.ErrorCodeToolDenied, tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"inspect_task_scope_and_broker_evidence"}), nil
	}

	gitWork, hooks, templates, environmentValues, prepareErr := prepareGitExecutionEnvironment(authorization, environment, gitTrust, proxyRoute)
	if prepareErr != nil {
		receipt, _ := completeBroker(broker, authorization, discardResult("git_environment_prepare_failed"))
		outcome := failedBeforeTransport(started, "prepare isolated Git environment: "+prepareErr.Error(), tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryRuntime, tool.OutcomeFailed, []string{"inspect_temporary_workspace_permissions"})
		outcome.Diagnostic = map[string]any{"broker_receipt": receipt}
		return outcome, nil
	}

	if params.Operation == operationClone {
		stageDestination := filepath.Join(authorization.ExecutionWorkspaceRoot, destinationRelative)
		if prepareErr = prepareCloneDestination(stageDestination, params.ReplaceExisting); prepareErr != nil {
			receipt, _ := completeBroker(broker, authorization, discardResult("clone_destination_not_replaceable"))
			outcome := failedBeforeTransport(started, prepareErr.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"choose_an_absent_destination_or_set_replace_existing"})
			outcome.Diagnostic = map[string]any{"broker_receipt": receipt}
			return outcome, nil
		}
	}

	args := gitArgs(params, authorization, destinationRelative, hooks, templates)
	timeout := t.timeout
	if timeout <= 0 || timeout > fixedTimeout {
		timeout = fixedTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, gitObservation.Path, args...)
	cmd.Dir = gitWork
	cmd.Env = environmentValues
	sandboxResult, sandboxErr := activeSandbox.ApplySandbox(cmd, authorization.SandboxTier, authorization.SandboxRoot)
	if sandboxErr != nil || !sandboxResult.Applied || sandboxResult.Support != sandbox.SandboxFull {
		receipt, _ := completeBroker(broker, authorization, discardResult("sandbox_not_applied"))
		message := "supervised_execution_required: platform sandbox was not applied"
		if sandboxErr != nil {
			message += ": " + sandboxErr.Error()
		}
		outcome := failedBeforeTransport(started, message, tool.ErrorCodeSupervisedExecutionRequired, tool.ErrorCategoryUnsupported, tool.OutcomeUnsupported, []string{"restore_full_platform_sandbox", "do_not_use_raw_shell_network"})
		outcome.Diagnostic = map[string]any{"broker_receipt": receipt, "sandbox_support": sandboxResult.Support, "sandbox_method": sandboxResult.Method}
		return outcome, nil
	}

	configureProcessTree(cmd)
	stdout := newBoundedOutput("stdout", observer)
	stderr := newBoundedOutput("stderr", observer)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	runErr := cmd.Run()
	terminateProcessTree(cmd)
	stdoutText, stdoutTruncated := stdout.Result()
	stderrText, stderrTruncated := stderr.Result()
	stdoutText, stderrText = sanitizeOutput(stdoutText), sanitizeOutput(stderrText)
	stdout.Emit(stdoutText, stdoutTruncated)
	stderr.Emit(stderrText, stderrTruncated)
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	timedOut := runCtx.Err() == context.DeadlineExceeded
	cancelled := runCtx.Err() == context.Canceled || errors.Is(ctx.Err(), context.Canceled)
	transport, process := tool.TransportCompleted, tool.ProcessExited
	discardReason := ""
	if timedOut {
		transport, process, discardReason = tool.TransportFailed, tool.ProcessTimedOut, "process_timeout"
	} else if cancelled {
		transport, process, discardReason = tool.TransportCancelled, tool.ProcessSignaled, "process_cancelled"
	} else if runErr != nil {
		if exitCode >= 0 {
			discardReason = "process_exit_nonzero"
		} else {
			transport, process, discardReason = tool.TransportFailed, tool.ProcessSpawnFailed, "process_spawn_failed"
		}
	}
	discard := runErr != nil || timedOut || cancelled
	receipt, completeErr := completeBroker(broker, authorization, tool.BrokerExecutionResult{
		TransportStatus: transport, ProcessStatus: process, ExitCode: &exitCode,
		SideEffectState: tool.OutcomeSideEffectUnknown, DiscardWorkspace: discard, DiscardReason: discardReason,
	})
	if completeErr != nil {
		return brokerCompletionFailure(started, params, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, completeErr), nil
	}
	if discard {
		return processFailure(started, params, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, timedOut, cancelled, process), nil
	}
	if receipt.CommitStatus != "committed" && receipt.CommitStatus != "no_workspace_changes" {
		return brokerCompletionFailure(started, params, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, fmt.Errorf("unexpected broker commit status %s", receipt.CommitStatus)), nil
	}

	refs := []remoteRef(nil)
	if params.Operation == operationLSRemote {
		refs, err = parseRemoteRefs(stdoutText)
		if err != nil {
			return semanticOutputFailure(started, params, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, err), nil
		}
	}
	payload := output{
		Operation: params.Operation, RemoteURL: params.URL, Ref: params.Ref,
		Destination: destinationForOutput(params, permissionContext.WorkspaceRoot), ReplaceExisting: params.ReplaceExisting,
		Refs: refs, Stdout: stdoutText, Stderr: stderrText, Truncated: stdoutTruncated || stderrTruncated,
		BrokerReceipt: receipt, ToolSnapshotID: snapshot.SnapshotID, EnvironmentSnapshotID: environment.SnapshotID,
		GitExecutable: gitObservation.Path, GitExecutableFingerprint: gitObservation.Fingerprint,
		GitExecPath: gitTrust.ExecPath, GitRemoteHelperFingerprint: gitTrust.HelperFingerprint,
		ProxyRouted: proxyRoute.Enabled, ProxySource: proxyRoute.Source,
	}
	content, _ := json.Marshal(payload)
	outcome := tool.SucceededOutcome(string(content)).WithTiming(started, time.Now().UTC())
	outcome.TransportStatus, outcome.ProcessStatus, outcome.ExitCode = tool.TransportCompleted, tool.ProcessExited, intPointer(exitCode)
	outcome.Summary = "Listed bounded refs from the pinned HTTPS Git remote."
	if params.Operation == operationClone {
		outcome.Summary = "Cloned the pinned HTTPS Git remote into the task workspace through supervised staging."
	}
	outcome.Operation = &tool.OperationDescriptor{Type: toolName + "." + params.Operation, Target: params.URL, SideEffectState: receipt.SideEffectState}
	outcome.AffectedPaths, outcome.Changes = brokerOutcomeChanges(receipt.WorkspaceChanges)
	outcome.Environment = &tool.EnvironmentObservation{
		Observation: tool.ObservationComplete, ObservedAt: time.Now().UTC(), Scope: permissionContext.ScopeID,
		Source: "pinned_execution_environment_snapshot", WorkspaceRoot: permissionContext.WorkspaceRoot,
		WorkingDirectory: permissionContext.WorkspaceRoot,
	}
	outcome.Details = outputDetails(payload)
	outcome.Diagnostic = map[string]any{
		"tool_snapshot_id": snapshot.SnapshotID, "environment_snapshot_id": environment.SnapshotID,
		"git_executable_fingerprint": gitObservation.Fingerprint, "sandbox_method": sandboxResult.Method,
		"git_remote_helper_fingerprint": gitTrust.HelperFingerprint,
		"git_policy":                    "https_only_public_noninteractive_isolated_config_no_submodule_direct_argv",
		"proxy_routed":                  proxyRoute.Enabled, "proxy_source": proxyRoute.Source,
	}
	return outcome, nil
}

func parseInput(raw string) (input, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var params input
	if err := decoder.Decode(&params); err != nil {
		return input{}, fmt.Errorf("git_remote input must match the strict schema: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return input{}, fmt.Errorf("git_remote accepts exactly one JSON object")
		}
		return input{}, fmt.Errorf("invalid trailing JSON: %w", err)
	}
	params.Operation = strings.TrimSpace(params.Operation)
	params.URL = strings.TrimSpace(params.URL)
	params.Ref = strings.TrimSpace(params.Ref)
	params.Destination = strings.TrimSpace(params.Destination)
	switch params.Operation {
	case operationLSRemote:
		if params.Destination != "" || params.ReplaceExisting {
			return input{}, fmt.Errorf("ls_remote does not accept destination or replace_existing")
		}
	case operationClone:
		if params.Destination == "" {
			return input{}, fmt.Errorf("clone requires destination")
		}
	default:
		return input{}, fmt.Errorf("operation must be ls_remote or clone")
	}
	if params.URL == "" {
		return input{}, fmt.Errorf("url is required")
	}
	return params, nil
}

func normalizeRemoteURL(raw string) (string, error) {
	if len(raw) > 2048 || strings.Contains(raw, "\\") || strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("Git remote URL contains forbidden length, whitespace, control, or backslash")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "https") {
		return "", fmt.Errorf("Git remote URL must use hierarchical HTTPS")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("inline Git credentials or usernames are forbidden")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("Git remote URL query and fragment are forbidden")
	}
	hostname := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if hostname == "" || strings.Contains(parsed.Host, "@") || strings.Contains(hostname, "%") {
		return "", fmt.Errorf("Git remote URL requires an unambiguous host")
	}
	for _, char := range hostname {
		if char > unicode.MaxASCII {
			return "", fmt.Errorf("Git remote host must use an ASCII DNS name or IP literal")
		}
	}
	if isLocalOrPrivateHost(hostname) {
		return "", fmt.Errorf("Git remote host must not target loopback, private, link-local, or local-name address space")
	}
	if parsed.Path == "" || parsed.Path == "/" || strings.ContainsRune(parsed.Path, '\x00') {
		return "", fmt.Errorf("Git remote URL requires a non-root repository path")
	}
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String(), nil
}

func isLocalOrPrivateHost(host string) bool {
	trimmed := strings.TrimSuffix(strings.ToLower(host), ".")
	if trimmed == "localhost" || strings.HasSuffix(trimmed, ".localhost") || strings.HasSuffix(trimmed, ".local") ||
		strings.HasSuffix(trimmed, ".internal") || !strings.Contains(trimmed, ".") {
		return true
	}
	if address := net.ParseIP(trimmed); address != nil {
		return isForbiddenRemoteIP(address)
	}
	return false
}

func (t *Tool) validateRemoteRoute(ctx context.Context, remoteURL string) (gitProxyRoute, error) {
	proxyRoute, found, err := t.explicitProxyRoute(ctx)
	if err != nil {
		return gitProxyRoute{}, err
	}
	if found {
		return proxyRoute, nil
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil {
		return gitProxyRoute{}, fmt.Errorf("parse normalized Git remote URL: %w", err)
	}
	host := parsed.Hostname()
	if address := net.ParseIP(host); address != nil {
		if isForbiddenRemoteIP(address) {
			return gitProxyRoute{}, fmt.Errorf("Git remote resolved to forbidden local/private address space")
		}
		return gitProxyRoute{}, nil
	}
	resolver := t.hostResolver()
	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addresses, err := resolver(resolveCtx, host)
	if err != nil || len(addresses) == 0 {
		if err == nil {
			err = fmt.Errorf("no addresses")
		}
		return gitProxyRoute{}, fmt.Errorf("resolve Git remote host before transport: %w", err)
	}
	for _, address := range addresses {
		if address == nil || isForbiddenRemoteIP(address) {
			return gitProxyRoute{}, fmt.Errorf("Git remote DNS includes loopback, private, CGNAT, link-local, reserved, or multicast address space")
		}
	}
	return gitProxyRoute{}, nil
}

func (t *Tool) explicitProxyRoute(ctx context.Context) (gitProxyRoute, bool, error) {
	lookup := t.lookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	for _, source := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		raw, exists := lookup(source)
		if !exists || strings.TrimSpace(raw) == "" {
			continue
		}
		parsed, err := parseExplicitGitProxy(raw)
		if err != nil {
			return gitProxyRoute{}, false, fmt.Errorf("explicit Git proxy %s is invalid: %w", source, err)
		}
		if err := t.validateProxyEndpoint(ctx, parsed.Hostname()); err != nil {
			return gitProxyRoute{}, false, fmt.Errorf("explicit Git proxy %s endpoint is invalid: %w", source, err)
		}
		return gitProxyRoute{Enabled: true, Source: source, URL: parsed.String()}, true, nil
	}
	return gitProxyRoute{}, false, nil
}

func parseExplicitGitProxy(raw string) (*url.URL, error) {
	if raw != strings.TrimSpace(raw) || len(raw) > 2048 || strings.Contains(raw, "\\") ||
		strings.IndexFunc(raw, unicode.IsControl) >= 0 || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return nil, fmt.Errorf("proxy contains forbidden length, whitespace, control, or backslash")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("proxy must be a hierarchical URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5h":
	default:
		return nil, fmt.Errorf("proxy scheme must be http, https, or socks5h")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, fmt.Errorf("proxy path, query, and fragment are forbidden")
	}
	port := parsed.Port()
	if port == "" {
		return nil, fmt.Errorf("proxy requires an explicit port")
	}
	var portNumber int
	if _, err := fmt.Sscanf(port, "%d", &portNumber); err != nil || portNumber < 1 || portNumber > 65535 || fmt.Sprintf("%d", portNumber) != port {
		return nil, fmt.Errorf("proxy port is invalid")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	return parsed, nil
}

func (t *Tool) validateProxyEndpoint(ctx context.Context, host string) error {
	if address := net.ParseIP(host); address != nil {
		if address.IsUnspecified() || address.IsMulticast() {
			return fmt.Errorf("proxy endpoint is unspecified or multicast")
		}
		return nil
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addresses, err := t.hostResolver()(resolveCtx, host)
	if err != nil || len(addresses) == 0 {
		if err == nil {
			err = fmt.Errorf("no addresses")
		}
		return fmt.Errorf("resolve explicit proxy endpoint: %w", err)
	}
	for _, address := range addresses {
		if address == nil || address.IsUnspecified() || address.IsMulticast() {
			return fmt.Errorf("proxy endpoint resolves to unspecified or multicast address")
		}
	}
	return nil
}

func (t *Tool) hostResolver() func(context.Context, string) ([]net.IP, error) {
	if t.resolveHost != nil {
		return t.resolveHost
	}
	return func(resolveCtx context.Context, hostname string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(resolveCtx, "ip", hostname)
	}
}

func isForbiddenRemoteIP(address net.IP) bool {
	parsed, ok := netip.AddrFromSlice(address)
	if !ok {
		return true
	}
	parsed = parsed.Unmap()
	if !parsed.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range forbiddenRemotePrefixes {
		if prefix.Contains(parsed) {
			return true
		}
	}
	return false
}

var forbiddenRemotePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func validGitRef(value string) bool {
	if value == "" || len(value) > 255 || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.Contains(value, "//") ||
		strings.Contains(value, "..") || strings.Contains(value, "@{") {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) || unicode.IsSpace(char) || strings.ContainsRune(`~^:?*[\\`, char) {
			return false
		}
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	return true
}

func (t *Tool) validatePinnedGit(ctx context.Context, snapshot tool.RunToolAvailabilitySnapshot, environment tool.ExecutionEnvironmentSnapshot) (tool.ExecutableObservation, gitRuntimeTrust, error) {
	if snapshot.EnvironmentSnapshotID == "" || snapshot.EnvironmentSnapshotID != environment.SnapshotID {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("Git tool/environment snapshot binding is stale")
	}
	entry, err := snapshot.Lookup(toolName)
	if err != nil || entry.Provider.ProviderID != tool.ProviderID(providerID) ||
		!tool.HasCapability(entry.Provider.Capabilities, tool.CapabilityGitRemoteRead) ||
		!tool.HasCapability(entry.Provider.Capabilities, tool.CapabilityGitClone) {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("current run snapshot does not contain the builtin typed Git provider")
	}
	managed, hasManaged := tool.ManagedExecutable(ctx, "git")
	if entry.Readiness.Readiness != tool.ProviderReady && !(entry.Readiness.Readiness == tool.ProviderNeedsRuntime && hasManaged) {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("typed Git provider is not ready in the current run snapshot")
	}
	gitObservation, ok := environment.Executable("git")
	if (!ok || gitObservation.Path == "") && hasManaged {
		fingerprint, err := tool.FingerprintExecutable(managed.Executable)
		if err != nil {
			return tool.ExecutableObservation{}, gitRuntimeTrust{}, err
		}
		gitObservation = tool.ExecutableObservation{Name: "git", Path: managed.Executable, Fingerprint: fingerprint, Provenance: "awaken-managed"}
		ok = true
	}
	if !ok || !filepath.IsAbs(gitObservation.Path) || strings.TrimSpace(gitObservation.Fingerprint) == "" {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("pinned environment lacks an absolute fingerprinted Git executable")
	}
	fingerprint, fingerprintErr := tool.FingerprintExecutable(gitObservation.Path)
	if fingerprintErr != nil || fingerprint != gitObservation.Fingerprint {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("pinned Git executable fingerprint changed before transport")
	}
	trustValidator := t.trustGit
	if trustValidator == nil {
		trustValidator = validateHostTrustedGitRuntime
	}
	trust, err := trustValidator(gitObservation)
	if gitObservation.Provenance == "awaken-managed" {
		if m := managedenv.FromContext(ctx); m != nil {
			if execPath, helper, ok := m.ManagedGitFiles(gitObservation.Path); ok {
				fingerprint, e := tool.FingerprintExecutable(helper)
				trust = gitRuntimeTrust{ExecPath: execPath, RemoteHelper: helper, HelperFingerprint: fingerprint}
				err = e
			}
		}
	}
	if err != nil {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, err
	}
	if !filepath.IsAbs(trust.ExecPath) || !filepath.IsAbs(trust.RemoteHelper) || !tool.PathWithinRoot(trust.RemoteHelper, trust.ExecPath) || strings.TrimSpace(trust.HelperFingerprint) == "" {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("host-trusted Git helper runtime is incomplete")
	}
	if fingerprint, err := tool.FingerprintExecutable(trust.RemoteHelper); err != nil || fingerprint != trust.HelperFingerprint {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("pinned Git HTTPS helper fingerprint changed before transport")
	}
	if gitRuntimeIntersectsTaskRoots(gitObservation.Path, trust.ExecPath, environment) {
		return tool.ExecutableObservation{}, gitRuntimeTrust{}, fmt.Errorf("host-trusted Git runtime intersects task-controlled roots")
	}
	return gitObservation, trust, nil
}

func validateCloneDestination(value string, permissionContext tool.PermissionContext) (string, string, error) {
	if len(value) > 512 || filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return "", "", fmt.Errorf("clone destination must be a bounded workspace-relative path")
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("clone destination cannot be the workspace root or escape it")
	}
	resolved, err := permissionContext.ResolveMutationPathChecked(cleaned)
	if err != nil {
		return "", "", fmt.Errorf("clone destination is outside writable TaskScope or sensitive: %w", err)
	}
	if info, lstatErr := os.Lstat(resolved); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", "", fmt.Errorf("clone destination cannot be an existing symlink")
	} else if lstatErr != nil && !errors.Is(lstatErr, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect clone destination: %w", lstatErr)
	}
	parent := filepath.Dir(resolved)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return "", "", fmt.Errorf("clone destination parent must already be a directory")
	}
	relative, err := filepath.Rel(permissionContext.WorkspaceRoot, resolved)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", "", fmt.Errorf("clone destination cannot be projected into TaskScope workspace")
	}
	return resolved, filepath.Clean(relative), nil
}

func prepareGitExecutionEnvironment(authorization tool.BrokerAuthorization, environment tool.ExecutionEnvironmentSnapshot, gitTrust gitRuntimeTrust, proxyRoute gitProxyRoute) (string, string, string, []string, error) {
	if authorization.SandboxRoot == "" || strings.TrimSpace(environment.PinnedTransportPATH()) == "" {
		return "", "", "", nil, fmt.Errorf("broker sandbox root and pinned PATH are required")
	}
	gitRoot := filepath.Join(authorization.SandboxRoot, "git-runtime")
	work := filepath.Join(gitRoot, "work")
	home := filepath.Join(gitRoot, "home")
	xdg := filepath.Join(gitRoot, "xdg")
	hooks := filepath.Join(gitRoot, "hooks")
	templates := filepath.Join(gitRoot, "templates")
	for _, directory := range []string{work, home, xdg, hooks, templates} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", "", "", nil, err
		}
	}
	globalConfig := filepath.Join(gitRoot, "empty-global-config")
	if err := os.WriteFile(globalConfig, []byte{}, 0o600); err != nil {
		return "", "", "", nil, err
	}
	values := map[string]string{
		"PATH": sanitizedGitPATH(environment, gitTrust), "GIT_EXEC_PATH": gitTrust.ExecPath,
		"HOME": home, "XDG_CONFIG_HOME": xdg, "GIT_CONFIG_GLOBAL": globalConfig, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0", "GCM_INTERACTIVE": "Never", "GIT_ASKPASS": "", "SSH_ASKPASS": "",
		"GIT_PROTOCOL_FROM_USER": "0", "GIT_OPTIONAL_LOCKS": "0", "GIT_ATTR_NOSYSTEM": "1",
		"GIT_LFS_SKIP_SMUDGE": "1", "GIT_TEMPLATE_DIR": templates, "LANG": "C", "LC_ALL": "C",
	}
	for key, value := range authorization.EnvironmentOverrides {
		values[key] = value
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && gitEnvironmentKeyAllowed(key) {
			values[key] = value
		}
	}
	if proxyRoute.Enabled {
		values["HTTPS_PROXY"] = proxyRoute.URL
		values["https_proxy"] = proxyRoute.URL
		values["ALL_PROXY"] = proxyRoute.URL
		values["all_proxy"] = proxyRoute.URL
		values["NO_PROXY"] = ""
		values["no_proxy"] = ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environmentValues := make([]string, 0, len(keys))
	for _, key := range keys {
		environmentValues = append(environmentValues, key+"="+values[key])
	}
	return work, hooks, templates, environmentValues, nil
}

func sanitizedGitPATH(environment tool.ExecutionEnvironmentSnapshot, gitTrust gitRuntimeTrust) string {
	roots := taskControlledEnvironmentRoots(environment)
	transportPATH := environment.PinnedTransportPATHEntries()
	values := make([]string, 0, len(transportPATH)+2)
	for _, value := range append([]string{gitTrust.ExecPath}, transportPATH...) {
		value = strings.TrimSpace(value)
		if value == "" || !filepath.IsAbs(value) || pathIntersectsAnyRoot(value, roots) {
			continue
		}
		duplicate := false
		for _, existing := range values {
			if samePath(existing, value) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			values = append(values, filepath.Clean(value))
		}
	}
	return strings.Join(values, string(os.PathListSeparator))
}

func gitRuntimeIntersectsTaskRoots(gitExecutable, gitExecPath string, environment tool.ExecutionEnvironmentSnapshot) bool {
	return pathIntersectsAnyRoot(gitExecutable, taskControlledEnvironmentRoots(environment)) ||
		pathIntersectsAnyRoot(gitExecPath, taskControlledEnvironmentRoots(environment))
}

func taskControlledEnvironmentRoots(environment tool.ExecutionEnvironmentSnapshot) []string {
	values := []string{environment.WorkspaceRoot, environment.TemporaryWorkRoot}
	for _, root := range environment.SkillRoots {
		values = append(values, root.Path)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !filepath.IsAbs(value) {
			continue
		}
		if real, err := filepath.EvalSymlinks(value); err == nil {
			value = real
		}
		result = append(result, filepath.Clean(value))
	}
	return result
}

func pathIntersectsAnyRoot(value string, roots []string) bool {
	if real, err := filepath.EvalSymlinks(value); err == nil {
		value = real
	}
	for _, root := range roots {
		if tool.PathWithinRoot(value, root) {
			return true
		}
	}
	return false
}

func samePath(left, right string) bool {
	return tool.PathWithinRoot(left, right) && tool.PathWithinRoot(right, left)
}

func gitEnvironmentKeyAllowed(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "SSL_CERT_FILE", "SSL_CERT_DIR", "GIT_SSL_CAINFO", "GIT_SSL_CAPATH":
		return true
	default:
		return false
	}
}

func prepareCloneDestination(stageDestination string, replace bool) error {
	info, err := os.Lstat(stageDestination)
	if err == nil {
		if !replace {
			return fmt.Errorf("clone destination already exists; set replace_existing=true for recoverable replacement")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("clone destination cannot replace a symlink")
		}
		if err := os.RemoveAll(stageDestination); err != nil {
			return fmt.Errorf("remove staged clone destination: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect staged clone destination: %w", err)
	}
	return nil
}

func gitArgs(params input, authorization tool.BrokerAuthorization, destinationRelative, hooks, templates string) []string {
	args := []string{
		"-c", "credential.helper=", "-c", "credential.interactive=never", "-c", "core.askPass=",
		"-c", "core.hooksPath=" + hooks, "-c", "init.templateDir=" + templates,
		"-c", "protocol.allow=never", "-c", "protocol.https.allow=always",
		"-c", "submodule.recurse=false", "-c", "fetch.recurseSubmodules=false",
		"-c", "http.followRedirects=false", "-c", "core.fsmonitor=false",
	}
	if params.Operation == operationLSRemote {
		args = append(args, "ls-remote", "--refs")
		if params.Ref != "" {
			args = append(args, "--exit-code")
		}
		args = append(args, "--", params.URL)
		if params.Ref != "" {
			args = append(args, params.Ref)
		}
		return args
	}
	args = append(args, "clone", "--depth=1", "--single-branch", "--no-recurse-submodules")
	if params.Ref != "" {
		args = append(args, "--branch", params.Ref)
	}
	stageDestination := filepath.Join(authorization.ExecutionWorkspaceRoot, destinationRelative)
	return append(args, "--", params.URL, stageDestination)
}

func parseRemoteRefs(value string) ([]remoteRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []remoteRef{}, nil
	}
	lines := strings.Split(value, "\n")
	if len(lines) > maxRemoteRefs {
		return nil, fmt.Errorf("Git remote returned more than %d refs", maxRemoteRefs)
	}
	result := make([]remoteRef, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) != 2 || !validObjectID(fields[0]) || !validRemoteRefName(fields[1]) {
			return nil, fmt.Errorf("Git remote returned malformed ref output")
		}
		result = append(result, remoteRef{ObjectID: fields[0], Name: fields[1]})
	}
	return result, nil
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func validRemoteRefName(value string) bool {
	return strings.HasPrefix(value, "refs/") && validGitRef(value)
}

type boundedOutput struct {
	stream   string
	buffer   *outputlimit.CappedBuffer
	observer tool.ToolObserver
	mu       sync.Mutex
}

func newBoundedOutput(stream string, observer tool.ToolObserver) *boundedOutput {
	return &boundedOutput{stream: stream, buffer: outputlimit.NewCappedBuffer(fixedOutputLimitRunes), observer: observer}
}

func (w *boundedOutput) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(value)
}

func (w *boundedOutput) Result() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := w.buffer.Result()
	return result.Content, result.Truncated
}

func (w *boundedOutput) Emit(redacted string, truncated bool) {
	if w.observer != nil && (redacted != "" || truncated) {
		w.observer.OnCommandOutput(tool.CommandOutput{Stream: w.stream, Delta: redacted, Truncated: truncated})
	}
}

var (
	urlUserInfoPattern    = regexp.MustCompile(`(?i)((?:https?|socks5h)://)[^/@\s:]+:[^/@\s]+@`)
	urlSecretQueryPattern = regexp.MustCompile(`(?i)([?&](?:token|access_token|auth|api_key|key)=)[^&\s]+`)
)

func sanitizeOutput(value string) string {
	value = strings.TrimSpace(value)
	value = urlUserInfoPattern.ReplaceAllString(value, `${1}[REDACTED]@`)
	value = urlSecretQueryPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	for _, marker := range []string{"Bearer ", "sk-", "ghp_", "github_pat_"} {
		for {
			index := strings.Index(value, marker)
			if index < 0 {
				break
			}
			end := index + len(marker)
			for end < len(value) {
				char := value[end]
				if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-_.", rune(char)) {
					end++
					continue
				}
				break
			}
			value = value[:index+len(marker)] + "[REDACTED]" + value[end:]
		}
	}
	return value
}

func completeBroker(broker tool.ExecutionBroker, authorization tool.BrokerAuthorization, result tool.BrokerExecutionResult) (tool.BrokerExecutionReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), brokerCompletionTimeout)
	defer cancel()
	return broker.Complete(ctx, authorization, result)
}

func discardResult(reason string) tool.BrokerExecutionResult {
	return tool.BrokerExecutionResult{
		TransportStatus: tool.TransportNotStarted, ProcessStatus: tool.ProcessNotApplicable,
		SideEffectState: tool.OutcomeSideEffectNone, DiscardWorkspace: true, DiscardReason: reason,
	}
}

func failedBeforeTransport(started time.Time, message, code string, category tool.ErrorCategory, status tool.OutcomeStatus, recovery []string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(sanitizeOutput(message), code, category, false)
	outcome.Status, outcome.TransportStatus, outcome.ProcessStatus = status, tool.TransportNotStarted, tool.ProcessNotApplicable
	outcome.Operation = &tool.OperationDescriptor{Type: toolName, SideEffectState: tool.OutcomeSideEffectNotStarted}
	outcome.RecoveryOptions = recovery
	return outcome.WithTiming(started, time.Now().UTC())
}

func processFailure(started time.Time, params input, exitCode int, stdout, stderr string, truncated bool, receipt tool.BrokerExecutionReceipt, timedOut, cancelled bool, process tool.ProcessStatus) tool.ExecutionOutcome {
	code, category, status, transport := tool.ErrorCodeProcessExitNonzero, tool.ErrorCategoryProcess, tool.OutcomeFailed, tool.TransportCompleted
	message := fmt.Sprintf("Git exited with code %d; staged changes were discarded", exitCode)
	if timedOut {
		code, transport, message = tool.ErrorCodeProcessTimeout, tool.TransportFailed, "Git remote operation timed out; staged changes were discarded"
	} else if cancelled {
		code, category, status, transport, message = tool.ErrorCodeToolCancelled, tool.ErrorCategoryCancelled, tool.OutcomeCancelled, tool.TransportCancelled, "Git remote operation was cancelled; staged changes were discarded"
	} else if process == tool.ProcessSpawnFailed {
		code, transport, message = tool.ErrorCodeProcessSpawnFailed, tool.TransportFailed, "Git failed to start; staged changes were discarded"
	}
	outcome := tool.FailedOutcome(message, code, category, false).WithTiming(started, time.Now().UTC())
	outcome.Status, outcome.TransportStatus, outcome.ProcessStatus = status, transport, process
	outcome.ExitCode, outcome.TimedOut = intPointer(exitCode), timedOut
	outcome.Operation = &tool.OperationDescriptor{Type: toolName + "." + params.Operation, Target: params.URL, SideEffectState: tool.OutcomeSideEffectNone}
	outcome.Details = map[string]any{"remote_operation": params.Operation, "remote_url": params.URL, "exit_code": exitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated, "broker_receipt": receipt}
	outcome.Diagnostic = map[string]any{"broker_receipt": receipt, "staging_discarded": true}
	outcome.RecoveryOptions = []string{"inspect_bounded_git_output", "verify_remote_and_ref_before_a_new_call", "do_not_fall_back_to_raw_shell_network"}
	return outcome
}

func brokerCompletionFailure(started time.Time, params input, exitCode int, stdout, stderr string, truncated bool, receipt tool.BrokerExecutionReceipt, err error) tool.ExecutionOutcome {
	code, category, status := tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, tool.OutcomeFailed
	if errors.Is(err, tool.ErrExecutionBrokerDenied) {
		code, category, status = tool.ErrorCodeToolDenied, tool.ErrorCategoryPolicy, tool.OutcomeDenied
	}
	outcome := tool.FailedOutcome("typed Git broker completion failed: "+sanitizeOutput(err.Error()), code, category, false).WithTiming(started, time.Now().UTC())
	outcome.Status, outcome.TransportStatus, outcome.ProcessStatus = status, tool.TransportCompleted, tool.ProcessExited
	outcome.ExitCode = intPointer(exitCode)
	outcome.Operation = &tool.OperationDescriptor{Type: toolName + "." + params.Operation, Target: params.URL, SideEffectState: receipt.SideEffectState}
	outcome.Details = map[string]any{"remote_operation": params.Operation, "remote_url": params.URL, "exit_code": exitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated, "broker_receipt": receipt}
	outcome.Diagnostic = map[string]any{"broker_receipt": receipt}
	outcome.RecoveryOptions = []string{"inspect_live_workspace_and_trash_before_retry", "do_not_blindly_replay", "do_not_fall_back_to_raw_shell_network"}
	return outcome
}

func semanticOutputFailure(started time.Time, params input, exitCode int, stdout, stderr string, truncated bool, receipt tool.BrokerExecutionReceipt, err error) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome("Git remote output validation failed: "+err.Error(), "git_output_invalid", tool.ErrorCategoryDomain, false).WithTiming(started, time.Now().UTC())
	outcome.TransportStatus, outcome.ProcessStatus, outcome.ExitCode = tool.TransportCompleted, tool.ProcessExited, intPointer(exitCode)
	outcome.Operation = &tool.OperationDescriptor{Type: toolName + "." + params.Operation, Target: params.URL, SideEffectState: receipt.SideEffectState}
	outcome.Details = map[string]any{"remote_operation": params.Operation, "remote_url": params.URL, "stdout": stdout, "stderr": stderr, "truncated": truncated, "broker_receipt": receipt}
	outcome.RecoveryOptions = []string{"treat_remote_output_as_untrusted", "do_not_retry_without_inspection"}
	return outcome
}

func outputDetails(value output) map[string]any {
	details := map[string]any{
		"remote_operation": value.Operation, "remote_url": value.RemoteURL, "broker_receipt": value.BrokerReceipt,
		"tool_snapshot_id": value.ToolSnapshotID, "environment_snapshot_id": value.EnvironmentSnapshotID,
		"git_executable": value.GitExecutable, "git_executable_fingerprint": value.GitExecutableFingerprint,
		"git_exec_path": value.GitExecPath, "git_remote_helper_fingerprint": value.GitRemoteHelperFingerprint,
		"proxy_routed": value.ProxyRouted,
	}
	if value.ProxySource != "" {
		details["proxy_source"] = value.ProxySource
	}
	if value.Ref != "" {
		details["ref"] = value.Ref
	}
	if value.Destination != "" {
		details["destination"] = value.Destination
	}
	if value.ReplaceExisting {
		details["replace_existing"] = true
	}
	if value.Refs != nil {
		details["refs"] = value.Refs
	}
	if value.Stdout != "" {
		details["stdout"] = value.Stdout
	}
	if value.Stderr != "" {
		details["stderr"] = value.Stderr
	}
	if value.Truncated {
		details["truncated"] = true
	}
	return details
}

func destinationForOutput(params input, workspaceRoot string) string {
	if params.Operation != operationClone || params.Destination == "" {
		return ""
	}
	relative, err := filepath.Rel(workspaceRoot, params.Destination)
	if err != nil {
		return ""
	}
	return filepath.Clean(relative)
}

func brokerOutcomeChanges(values []tool.BrokerWorkspaceChange) ([]string, []tool.ResourceChange) {
	paths := make([]string, 0, len(values))
	changes := make([]tool.ResourceChange, 0, len(values))
	for _, value := range values {
		paths = append(paths, value.Path)
		reversibility := tool.ChangeConditional
		if value.RecoveryPath != "" {
			reversibility = tool.ChangeReversible
		}
		details := map[string]any{"broker_observed": true}
		if value.RecoveryPath != "" {
			details["recovery_path"] = value.RecoveryPath
		}
		if value.RecoveryMethod != "" {
			details["recovery_method"] = value.RecoveryMethod
		}
		changes = append(changes, tool.ResourceChange{
			Kind: "file", Action: value.Action, Resource: value.Path,
			Summary:   "Git clone workspace change committed by the staging broker.",
			Certainty: tool.ChangeObserved, Reversibility: reversibility, Details: details,
		})
	}
	return dedupe(paths), changes
}

func dedupe(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func intPointer(value int) *int { return &value }

func init() {
	tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin)
}
