package dependencyinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
	"awaken/src/tool/sandbox"
)

const (
	toolName                = "dependency_install"
	providerID              = "builtin:dependency_install"
	fixedInstallTimeout     = 2 * time.Minute
	fixedOutputLimitRunes   = 32000
	brokerCompletionTimeout = 2 * time.Minute
)

type Tool struct {
	adapter sandbox.SandboxAdapter
	timeout time.Duration
}

var _ tool.OutcomeTool = (*Tool)(nil)

type input struct {
	PlanID string `json:"plan_id"`
}

type output struct {
	PlanID                       string                               `json:"plan_id"`
	Source                       string                               `json:"source"`
	ExitCode                     int                                  `json:"exit_code"`
	Stdout                       string                               `json:"stdout,omitempty"`
	Stderr                       string                               `json:"stderr,omitempty"`
	Truncated                    bool                                 `json:"truncated,omitempty"`
	BrokerReceipt                tool.BrokerExecutionReceipt          `json:"broker_receipt"`
	Providers                    []tool.InstallPlanProviderProvenance `json:"providers"`
	CurrentToolSnapshotID        string                               `json:"current_tool_snapshot_id"`
	CurrentEnvironmentSnapshotID string                               `json:"current_environment_snapshot_id"`
	RefreshedEnvironmentID       string                               `json:"refreshed_environment_snapshot_id,omitempty"`
	RefreshedDependency          *tool.DependencyObservation          `json:"refreshed_dependency,omitempty"`
	CurrentRunRemainsPinned      bool                                 `json:"current_run_remains_pinned"`
}

func (*Tool) Name() string { return toolName }

func (*Tool) Description() string {
	return "执行当前 run snapshot 中宿主生成的依赖安装计划；输入只接受 plan_id，executable、argv、cwd 与 scope 不可由模型指定"
}

func (*Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{{
		Name: "plan_id", Description: "capability/readiness 诊断返回的当前 run 精确 install_plan ID", Type: "string", Required: true,
	}}
}

func (t *Tool) InputSchema() tool.ToolParametersSchema {
	minLength := 1
	return tool.ToolParametersSchema{
		Type: "object",
		Properties: map[string]tool.ToolParam{
			"plan_id": {Type: "string", MinLength: &minLength, Description: "当前 run snapshot 中的宿主生成 plan_id"},
		},
		Required: []string{"plan_id"}, AdditionalProperties: tool.BoolPtr(false),
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	providerSchema := tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: toolName, DisplayName: "Dependency Install", Description: t.Description(),
		Purpose: "在真实平台 sandbox 与 workspace staging 内执行当前 run 固定的安全依赖计划，且只在进程 exit 0 后发布文件变化。",
		WhenToUse: []string{
			"provider readiness 返回 host_dependency_resolver_v1 candidate_install_plan，且需要准备 workspace language package 时",
			"模型已经选择诊断中某个精确 plan_id 时",
		},
		WhenNotToUse: []string{
			"不得传入命令、argv、package、URL、timeout 或安装目录；schema 不接受这些字段",
			"不得用于 user/system/admin/global 安装，也不得替代 Skill、MCP 或普通 Shell 执行",
			"平台没有真实 full sandbox 时不会启动 package manager",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"plan_id": {Type: "string"}, "source": {Type: "string"},
			"stdout": {Type: "string"}, "stderr": {Type: "string"},
			"broker_receipt":           {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
			"providers":                {Type: "array", Items: &providerSchema},
			"current_tool_snapshot_id": {Type: "string"}, "current_environment_snapshot_id": {Type: "string"},
			"refreshed_environment_snapshot_id": {Type: "string"},
			"refreshed_dependency":              {Type: "object", AdditionalProperties: tool.BoolPtr(true)},
			"current_run_remains_pinned":        {Type: "boolean"},
		}, Required: []string{
			"plan_id", "source", "broker_receipt", "providers",
			"current_tool_snapshot_id", "current_environment_snapshot_id",
			"refreshed_environment_snapshot_id", "refreshed_dependency", "current_run_remains_pinned",
		}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectExecute, Resources: []string{"pinned package manager process"}, Description: "直接执行宿主固定的 executable/argv，不经过 shell。", Reversibility: tool.ReversibilityNotApplicable},
			{Kind: tool.SideEffectNetwork, Resources: []string{"package registry selected by the pinned manager"}, Description: "安装进程可联网，但真实文件写入只允许发生在 private staging。", Reversibility: tool.ReversibilityNotApplicable},
			{Kind: tool.SideEffectUpdate, Resources: []string{"task-scoped workspace dependency files"}, Description: "exit 0 后由 broker 校验 scope 并发布；覆盖前版本进入操作系统 Trash。", Reversibility: tool.ReversibilityConditional},
		},
		Examples: []tool.ToolExample{{Scenario: "执行 readiness 诊断给出的固定计划", Input: map[string]any{"plan_id": "install_plan_0123456789abcdef01234567"}, Outcome: "返回进程事实、broker receipt、刷新后的环境/依赖观察与新 run 提示"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "install_plan_not_found", Cause: "plan_id 不属于当前 run snapshot 或已过期", Retryable: false, Recovery: []string{"重新诊断当前 run 或刷新/新建 run"}},
			{Code: "install_plan_invalid", Cause: "计划 identity、environment、scope、manager path 或安全 argv 不匹配", Retryable: false, Recovery: []string{"拒绝执行并刷新 run availability"}},
			{Code: tool.ErrorCodeSupervisedExecutionRequired, Cause: "平台无法应用真实 full sandbox/staging broker", Retryable: false, Recovery: []string{"改用支持真实 sandbox 的平台或已就绪 provider"}},
			{Code: tool.ErrorCodeProcessExitNonzero, Cause: "package manager 非零退出", Retryable: false, Recovery: []string{"检查有界 stderr；staging 已丢弃，不要盲目重试"}},
			{Code: tool.ErrorCodeProcessTimeout, Cause: "固定时限到期", Retryable: false, Recovery: []string{"检查 registry/manager 状态；staging 已丢弃"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: false, Destructive: true, OpenWorld: true,
			CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite, NetworkScope: tool.NetworkScopeOpen,
		},
		ProviderID: tool.ProviderID(providerID), Capabilities: []tool.Capability{tool.CapabilityDependencyInstall},
	})
}

func (*Tool) CheckPermission(_ context.Context, raw string, _ tool.PermissionContext) tool.PermissionDecision {
	params, err := parseInput(raw)
	if err != nil {
		// Execution returns the canonical validation outcome. Permission must not
		// invent a broader fallback or generic review gate.
		return tool.AllowPermission()
	}
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title: "Install pinned dependency", Description: params.PlanID, RiskTone: "warning",
	})
}

func (t *Tool) Execute(ctx context.Context, raw string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, raw, nil)
	if err != nil {
		return outcome.Content, err
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
		return failedBeforeTransport(started, err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, tool.OutcomeFailed, []string{"provide_only_current_plan_id"}), nil
	}
	snapshot, environment, ok := tool.RunAvailabilityFromContext(ctx)
	if !ok || snapshot.EnvironmentSnapshotID == "" || environment.SnapshotID == "" {
		return failedBeforeTransport(started, "run_availability_snapshot_missing: dependency installation requires pinned tool and environment snapshots", "run_availability_snapshot_missing", tool.ErrorCategoryRuntime, tool.OutcomeUnsupported, []string{"start_or_refresh_run_availability"}), nil
	}
	resolution, err := snapshot.LookupCandidateInstallPlan(params.PlanID)
	if err != nil {
		code := "install_plan_not_found"
		if errors.Is(err, tool.ErrCandidateInstallPlanInvalid) {
			code = "install_plan_invalid"
		}
		return failedBeforeTransport(started, err.Error(), code, tool.ErrorCategoryValidation, tool.OutcomeDenied, []string{"refresh_run_availability", "select_exact_current_plan_id"}), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx).Normalize()
	if err := validatePinnedPlan(resolution, snapshot, environment, permissionContext); err != nil {
		return failedBeforeTransport(started, err.Error(), "install_plan_invalid", tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"refresh_run_availability", "do_not_modify_plan_fields"}), nil
	}

	activeSandbox := t.adapter
	if activeSandbox == nil {
		activeSandbox = sandbox.DefaultAdapter()
	}
	if activeSandbox == nil || activeSandbox.SupportLevel() != sandbox.SandboxFull {
		return failedBeforeTransport(started, "supervised_execution_required: dependency installation requires a real full platform sandbox", tool.ErrorCodeSupervisedExecutionRequired, tool.ErrorCategoryUnsupported, tool.OutcomeUnsupported, []string{"use_supported_full_sandbox_platform", "select_ready_provider"}), nil
	}

	broker, explicitBroker := tool.ExecutionBrokerFromContext(ctx)
	if !explicitBroker {
		broker = tool.NewWorkspaceExecutionBroker()
	}
	if broker == nil {
		return failedBeforeTransport(started, "execution_broker_unavailable: typed dependency installation requires workspace staging", tool.ErrorCodeExecutionBrokerUnavailable, tool.ErrorCategoryRuntime, tool.OutcomeUnsupported, []string{"restore_workspace_execution_broker"}), nil
	}
	brokerScope, err := tool.CanonicalBrokerWorkspaceScope(permissionContext)
	if err != nil {
		return failedBeforeTransport(started, fmt.Sprintf("canonicalize dependency install scope: %v", err), "install_scope_invalid", tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"correct_task_scope"}), nil
	}
	projection := &tool.BrokerDependencyInstallPlan{
		Plan: resolution.Plan, Requirement: resolution.Requirement,
		Providers: resolution.Providers, ToolSnapshotID: resolution.ToolSnapshotID,
	}
	authorization, authorizeErr := broker.Authorize(ctx, tool.ExecutionBrokerRequest{
		ProviderID: tool.ProviderID(providerID), Capability: tool.CapabilityDependencyInstall,
		Operation:        tool.BrokerOperationDependencyInstall,
		WorkingDirectory: resolution.Plan.CWD, WorkspaceRoot: permissionContext.WorkspaceRoot,
		WorkspaceScopeDeclared: true, WritableRoots: brokerScope.WritableRoots, ProtectedRoots: brokerScope.ProtectedRoots,
		EnvironmentSnapshotID: environment.SnapshotID, DependencyInstall: projection,
	})
	if authorizeErr != nil || authorization.Action != tool.BrokerAllow {
		message := "execution broker denied typed dependency installation"
		if authorizeErr != nil {
			message += ": " + authorizeErr.Error()
		}
		return failedBeforeTransport(started, message, tool.ErrorCodeToolDenied, tool.ErrorCategoryPolicy, tool.OutcomeDenied, []string{"inspect_task_scope_and_broker_evidence"}), nil
	}

	timeout := t.timeout
	if timeout <= 0 || timeout > fixedInstallTimeout {
		timeout = fixedInstallTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, resolution.Plan.Executable, append([]string(nil), resolution.Plan.Args...)...)
	cmd.Dir = authorization.ExecutionWorkingDirectory
	cmd.Env = installEnvironment(os.Environ(), resolution.Plan.EnvDelta, authorization.EnvironmentOverrides, filteredInstallPATH(environment, permissionContext))
	sandboxResult, sandboxErr := activeSandbox.ApplySandbox(cmd, authorization.SandboxTier, authorization.SandboxRoot)
	if sandboxErr != nil || !sandboxResult.Applied || sandboxResult.Support != sandbox.SandboxFull {
		receipt, _ := completeBroker(broker, authorization, tool.BrokerExecutionResult{
			TransportStatus: tool.TransportNotStarted, ProcessStatus: tool.ProcessNotApplicable,
			SideEffectState: tool.OutcomeSideEffectNone, DiscardWorkspace: true, DiscardReason: "sandbox_not_applied",
		})
		message := "supervised_execution_required: platform sandbox was not applied"
		if sandboxErr != nil {
			message += ": " + sandboxErr.Error()
		}
		outcome := failedBeforeTransport(started, message, tool.ErrorCodeSupervisedExecutionRequired, tool.ErrorCategoryUnsupported, tool.OutcomeUnsupported, []string{"restore_full_platform_sandbox"})
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
	stdoutText = sanitizeOutput(stdoutText)
	stderrText = sanitizeOutput(stderrText)
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
		return brokerCompletionFailure(started, params.PlanID, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, completeErr), nil
	}
	if discard {
		return processFailure(started, params.PlanID, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, timedOut, cancelled, process), nil
	}
	if receipt.CommitStatus != "committed" && receipt.CommitStatus != "no_workspace_changes" {
		return brokerCompletionFailure(started, params.PlanID, exitCode, stdoutText, stderrText, stdoutTruncated || stderrTruncated, receipt, fmt.Errorf("unexpected broker commit status %s", receipt.CommitStatus)), nil
	}

	refreshedEnvironment := refreshEnvironmentSnapshot(ctx, environment, permissionContext, params.PlanID)
	refreshedDependency := tool.ObserveDependencyRequirementWithSandbox(resolution.Requirement, refreshedEnvironment, activeSandbox)
	payload := output{
		PlanID: params.PlanID, Source: resolution.Plan.Source, ExitCode: exitCode,
		Stdout: stdoutText, Stderr: stderrText, Truncated: stdoutTruncated || stderrTruncated,
		BrokerReceipt: receipt, Providers: resolution.Providers,
		CurrentToolSnapshotID: snapshot.SnapshotID, CurrentEnvironmentSnapshotID: environment.SnapshotID,
		RefreshedEnvironmentID: refreshedEnvironment.SnapshotID, RefreshedDependency: &refreshedDependency,
		CurrentRunRemainsPinned: true,
	}
	content, _ := json.Marshal(payload)
	if refreshedDependency.Status != tool.DependencyReady {
		outcome := tool.FailedOutcome(string(content), tool.ErrorCodeNeedsRuntime, tool.ErrorCategoryRuntime, false).WithTiming(started, time.Now().UTC())
		outcome.Status, outcome.TransportStatus, outcome.ProcessStatus = tool.OutcomeNeedsRuntime, tool.TransportCompleted, tool.ProcessExited
		outcome.ExitCode = intPointer(exitCode)
		outcome.ErrorMessage = "package manager exited zero, but the bounded post-install probe did not verify the dependency as ready"
		outcome.Summary = "Dependency installation was not verified ready after the manager exited zero."
		outcome.Operation = &tool.OperationDescriptor{Type: toolName, Target: params.PlanID, SideEffectState: receipt.SideEffectState}
		outcome.AffectedPaths, outcome.Changes = brokerOutcomeChanges(receipt.WorkspaceChanges)
		outcome.Environment = &tool.EnvironmentObservation{
			Observation: tool.ObservationComplete, ObservedAt: refreshedEnvironment.GeneratedAt,
			Scope: "refreshed_next_run_candidate", Source: "execution_environment_snapshot",
			WorkspaceRoot: refreshedEnvironment.WorkspaceRoot, WorkingDirectory: refreshedEnvironment.WorkingDirectory,
		}
		outcome.Details = outputDetails(payload)
		outcome.Diagnostic = map[string]any{
			"plan_id": params.PlanID, "plan_provenance": resolution.Plan.Provenance,
			"tool_snapshot_id": snapshot.SnapshotID, "environment_snapshot_id": environment.SnapshotID,
			"refreshed_environment_snapshot_id":    refreshedEnvironment.SnapshotID,
			"refreshed_environment_refresh_reason": refreshedEnvironment.RefreshReason,
			"refreshed_dependency_status":          refreshedDependency.Status,
			"current_run_snapshot_remains_pinned":  true, "sandbox_method": sandboxResult.Method,
			"environment_policy": "pinned_path_plus_explicit_current_package_auth_proxy_allowlist",
		}
		outcome.RecoveryOptions = []string{"inspect_refreshed_dependency_observation", "start_new_run_or_refresh_availability", "do_not_blindly_retry_install"}
		outcome.NextActions = []tool.NextAction{{
			ID: "inspect_dependency_state", Description: "Inspect the refreshed dependency observation and committed workspace changes before deciding remediation.",
			Preconditions: []string{"Do not claim the dependency is installed from exit code alone."},
		}, {
			ID: "refresh_run_availability", Description: "Start a new run or explicitly refresh run availability after the dependency state is verified.",
			Preconditions: []string{"Do not replace the current run's pinned snapshot in place."},
		}}
		return outcome, nil
	}
	outcome := tool.SucceededOutcome(string(content)).WithTiming(started, time.Now().UTC())
	outcome.TransportStatus, outcome.ProcessStatus, outcome.ExitCode = tool.TransportCompleted, tool.ProcessExited, intPointer(exitCode)
	outcome.Summary = "Installed the pinned workspace dependency plan and committed the verified staged delta."
	outcome.Operation = &tool.OperationDescriptor{Type: toolName, Target: params.PlanID, SideEffectState: receipt.SideEffectState}
	outcome.AffectedPaths, outcome.Changes = brokerOutcomeChanges(receipt.WorkspaceChanges)
	outcome.Environment = &tool.EnvironmentObservation{
		Observation: tool.ObservationComplete, ObservedAt: refreshedEnvironment.GeneratedAt,
		Scope: "refreshed_next_run_candidate", Source: "execution_environment_snapshot",
		WorkspaceRoot: refreshedEnvironment.WorkspaceRoot, WorkingDirectory: refreshedEnvironment.WorkingDirectory,
	}
	outcome.Details = outputDetails(payload)
	outcome.Diagnostic = map[string]any{
		"plan_id": params.PlanID, "plan_provenance": resolution.Plan.Provenance,
		"tool_snapshot_id": snapshot.SnapshotID, "environment_snapshot_id": environment.SnapshotID,
		"refreshed_environment_snapshot_id":    refreshedEnvironment.SnapshotID,
		"refreshed_environment_refresh_reason": refreshedEnvironment.RefreshReason,
		"current_run_snapshot_remains_pinned":  true, "sandbox_method": sandboxResult.Method,
		"environment_policy": "pinned_path_plus_explicit_current_package_auth_proxy_allowlist",
	}
	outcome.NextActions = []tool.NextAction{{
		ID: "refresh_run_availability", Description: "Start a new run or explicitly refresh run availability so providers can consume the refreshed dependency state.",
		Preconditions: []string{"Do not replace the current run's pinned tool/environment snapshot in place."},
	}}
	return outcome, nil
}

func parseInput(raw string) (input, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var params input
	if err := decoder.Decode(&params); err != nil {
		return input{}, fmt.Errorf("dependency_install input must be an object containing only plan_id: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return input{}, err
	}
	params.PlanID = strings.TrimSpace(params.PlanID)
	if params.PlanID == "" {
		return input{}, fmt.Errorf("plan_id is required")
	}
	return params, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return fmt.Errorf("dependency_install accepts exactly one JSON object")
}

func validatePinnedPlan(resolution tool.CandidateInstallPlanResolution, snapshot tool.RunToolAvailabilitySnapshot, environment tool.ExecutionEnvironmentSnapshot, permissionContext tool.PermissionContext) error {
	plan := resolution.Plan
	if resolution.ToolSnapshotID != snapshot.SnapshotID || plan.EnvironmentID != environment.SnapshotID || snapshot.EnvironmentSnapshotID != environment.SnapshotID {
		return fmt.Errorf("install plan is stale or bound to a different run/environment snapshot")
	}
	if plan.InstallScope != tool.InstallWorkspace || plan.RequiresAdmin || plan.Provenance != "host_dependency_resolver_v1" {
		return fmt.Errorf("only host-generated, non-admin workspace install plans are supported")
	}
	if len(plan.EnvDelta) != 0 {
		return fmt.Errorf("host_dependency_resolver_v1 does not authorize free environment overrides")
	}
	if strings.TrimSpace(environment.PinnedTransportPATH()) == "" {
		return fmt.Errorf("pinned environment PATH is required for deterministic package-manager execution")
	}
	if !containsFold(environment.AvailablePackageManagers, plan.Source) {
		return fmt.Errorf("package manager %q was not observed in the pinned environment", plan.Source)
	}
	manager, ok := environment.Executable(plan.Source)
	if !ok || !sameExecutablePath(manager.Path, plan.Executable) {
		return fmt.Errorf("plan executable does not match the pinned package-manager observation")
	}
	if environment.IsTaskControlledPath(plan.Executable) || pathWithinPermissionControlledRoot(plan.Executable, permissionContext) {
		return fmt.Errorf("pinned package manager is inside a task-controlled workspace, temporary, or Skill root")
	}
	currentFingerprint, fingerprintErr := tool.FingerprintExecutable(plan.Executable)
	if fingerprintErr != nil || currentFingerprint != plan.ExecutableFingerprint ||
		(strings.TrimSpace(manager.Fingerprint) != "" && manager.Fingerprint != plan.ExecutableFingerprint) {
		return fmt.Errorf("pinned package manager fingerprint changed before transport")
	}
	workspaceRoot := permissionContext.WorkspaceRoot
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(plan.CWD) == "" || !tool.PathWithinRoot(plan.CWD, workspaceRoot) {
		return fmt.Errorf("install plan cwd is outside the current task workspace")
	}
	if environment.WorkspaceRoot != "" && !sameExecutablePath(environment.WorkspaceRoot, workspaceRoot) {
		return fmt.Errorf("pinned environment workspace does not match current TaskScope")
	}
	if len(resolution.Providers) == 0 || resolution.Requirement.Kind != tool.DependencyLanguagePackage {
		return fmt.Errorf("install plan lacks language dependency/provider provenance")
	}
	if err := validateManagerArgv(plan); err != nil {
		return err
	}
	return nil
}

func validateManagerArgv(plan tool.CandidateInstallPlan) error {
	if len(plan.Args) == 0 {
		return fmt.Errorf("install plan argv is empty")
	}
	for _, arg := range plan.Args {
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("install plan argv contains NUL")
		}
	}
	switch strings.ToLower(plan.Source) {
	case "pip", "pip3":
		if !containsExact(plan.Args, "--no-input") || !containsExact(plan.Args, "--only-binary=:all:") || !containsExact(plan.Args, "--target") {
			return fmt.Errorf("Python install plan lost no-input/binary-only/workspace-target guards")
		}
	case "uv":
		if len(plan.Args) < 2 || plan.Args[0] != "pip" || plan.Args[1] != "install" || !containsExact(plan.Args, "--only-binary") || !containsExact(plan.Args, ":all:") || !containsExact(plan.Args, "--target") {
			return fmt.Errorf("uv install plan lost pip/binary-only/workspace-target guards")
		}
	case "npm":
		if !containsExact(plan.Args, "--ignore-scripts") || !containsExact(plan.Args, "--no-audit") || !containsExact(plan.Args, "--no-fund") {
			return fmt.Errorf("npm install plan lost lifecycle/audit/fund guards")
		}
	case "pnpm":
		if !containsExact(plan.Args, "--ignore-scripts") {
			return fmt.Errorf("pnpm install plan lost lifecycle-script guard")
		}
	case "yarn":
		if !containsExact(plan.Args, "--ignore-scripts") || !containsExact(plan.Args, "--non-interactive") {
			return fmt.Errorf("yarn install plan lost lifecycle/non-interactive guards")
		}
	default:
		return fmt.Errorf("unsupported workspace package manager %q", plan.Source)
	}
	return nil
}

func completeBroker(broker tool.ExecutionBroker, authorization tool.BrokerAuthorization, result tool.BrokerExecutionResult) (tool.BrokerExecutionReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), brokerCompletionTimeout)
	defer cancel()
	return broker.Complete(ctx, authorization, result)
}

func installEnvironment(base []string, planDelta map[string]string, brokerOverrides map[string]string, pinnedPATH []string) []string {
	values := make(map[string]string, len(base)+len(planDelta)+len(brokerOverrides))
	keys := make(map[string]string, len(values))
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" || !installEnvironmentKeyAllowed(key) {
			continue
		}
		canonical := key
		if runtime.GOOS == "windows" {
			canonical = strings.ToUpper(key)
		}
		keys[canonical], values[canonical] = key, value
	}
	apply := func(delta map[string]string) {
		for key, value := range delta {
			canonical := key
			if runtime.GOOS == "windows" {
				canonical = strings.ToUpper(key)
			}
			keys[canonical], values[canonical] = key, value
		}
	}
	apply(planDelta)
	apply(brokerOverrides)
	// PATH is execution identity, not ambient convenience: package-manager
	// shebangs commonly use /usr/bin/env. Force the exact run-snapshot PATH
	// after all current-process/plan/broker values so a later os.Environ change
	// cannot select a different interpreter.
	pathKey := "PATH"
	if runtime.GOOS == "windows" {
		pathKey = strings.ToUpper(pathKey)
	}
	keys[pathKey] = "PATH"
	values[pathKey] = strings.Join(append([]string(nil), pinnedPATH...), string(os.PathListSeparator))
	canonicalKeys := make([]string, 0, len(values))
	for key := range values {
		canonicalKeys = append(canonicalKeys, key)
	}
	sort.Strings(canonicalKeys)
	result := make([]string, 0, len(canonicalKeys))
	for _, canonical := range canonicalKeys {
		result = append(result, keys[canonical]+"="+values[canonical])
	}
	return result
}

func filteredInstallPATH(environment tool.ExecutionEnvironmentSnapshot, permissionContext tool.PermissionContext) []string {
	transportPATH := environment.PinnedTransportPATHEntries()
	result := make([]string, 0, len(transportPATH))
	seen := map[string]bool{}
	for _, entry := range transportPATH {
		entry, ok := tool.AbsoluteCleanPath(entry, "")
		if !ok {
			continue
		}
		if realEntry, err := filepath.EvalSymlinks(entry); err == nil {
			entry = filepath.Clean(realEntry)
		} else {
			// A missing/unresolvable PATH directory can be created or redirected
			// after the run snapshot. It is not safe for /usr/bin/env shebangs.
			continue
		}
		info, err := os.Stat(entry)
		if err != nil || !info.IsDir() || environment.IsTaskControlledPath(entry) ||
			pathWithinPermissionControlledRoot(entry, permissionContext) || seen[entry] {
			continue
		}
		seen[entry] = true
		result = append(result, entry)
	}
	return result
}

func pathWithinPermissionControlledRoot(path string, permissionContext tool.PermissionContext) bool {
	path, ok := tool.AbsoluteCleanPath(path, "")
	if !ok {
		return true
	}
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(realPath)
	}
	for _, root := range []string{permissionContext.WorkspaceRoot, permissionContext.TemporaryWorkRoot} {
		root, ok := tool.AbsoluteCleanPath(root, "")
		if !ok {
			continue
		}
		if tool.PathWithinRoot(path, root) {
			return true
		}
		if realRoot, err := filepath.EvalSymlinks(root); err == nil && tool.PathWithinRoot(path, realRoot) {
			return true
		}
	}
	return false
}

func installEnvironmentKeyAllowed(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	if strings.HasPrefix(key, "LC_") {
		return true
	}
	switch key {
	case "PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "TERM", "SYSTEMROOT", "WINDIR", "PATHEXT",
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
		"SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE",
		"PIP_CONFIG_FILE", "PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL", "PIP_TRUSTED_HOST", "PIP_CERT", "PIP_CLIENT_CERT",
		"UV_INDEX_URL", "UV_EXTRA_INDEX_URL", "UV_INDEX", "UV_NATIVE_TLS",
		"NPM_CONFIG_USERCONFIG", "NPM_CONFIG_REGISTRY", "NPM_CONFIG_CAFILE", "NPM_CONFIG_CERT", "NPM_CONFIG_KEY",
		"NPM_TOKEN", "NODE_AUTH_TOKEN", "YARN_NPM_AUTH_TOKEN", "YARN_NPM_AUTH_IDENT", "YARN_NPM_REGISTRY_SERVER":
		return true
	default:
		return false
	}
}

func refreshEnvironmentSnapshot(ctx context.Context, previous tool.ExecutionEnvironmentSnapshot, permissionContext tool.PermissionContext, planID string) tool.ExecutionEnvironmentSnapshot {
	names := make([]string, 0, len(previous.Executables))
	pathEntries := append([]string(nil), previous.PATH...)
	for _, executable := range previous.Executables {
		names = append(names, executable.Name)
		directory := filepath.Dir(executable.Path)
		if directory != "." && !containsFold(pathEntries, directory) {
			pathEntries = append(pathEntries, directory)
		}
	}
	return tool.ProbeExecutionEnvironment(ctx, tool.ExecutionEnvironmentProbeOptions{
		WorkspaceRoot:     permissionContext.WorkspaceRoot,
		WorkingDirectory:  previous.WorkingDirectory,
		SkillRoots:        append([]tool.SkillRootObservation(nil), previous.SkillRoots...),
		TemporaryWorkRoot: permissionContext.TemporaryWorkRoot,
		RefreshReason:     "dependency_install:" + strings.TrimSpace(planID),
		PATH:              strings.Join(pathEntries, string(os.PathListSeparator)), PATHSource: "post_dependency_install_refresh",
		Shell: previous.Shell, Executables: names, PackageManagers: append([]string(nil), previous.AvailablePackageManagers...),
		ProbeVersions: false,
	})
}

func outputDetails(value output) map[string]any {
	details := map[string]any{
		"plan_id": value.PlanID, "source": value.Source,
		"broker_receipt": value.BrokerReceipt, "providers": value.Providers,
		"current_tool_snapshot_id":          value.CurrentToolSnapshotID,
		"current_environment_snapshot_id":   value.CurrentEnvironmentSnapshotID,
		"refreshed_environment_snapshot_id": value.RefreshedEnvironmentID,
		"refreshed_dependency":              value.RefreshedDependency,
		"current_run_remains_pinned":        value.CurrentRunRemainsPinned,
	}
	if value.Stdout != "" {
		details["stdout"] = value.Stdout
	}
	if value.Stderr != "" {
		details["stderr"] = value.Stderr
	}
	return details
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
			Summary:   "Workspace dependency change committed by the staging broker.",
			Certainty: tool.ChangeObserved, Reversibility: reversibility, Details: details,
		})
	}
	return dedupe(paths), changes
}

func failedBeforeTransport(started time.Time, message, code string, category tool.ErrorCategory, status tool.OutcomeStatus, recovery []string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(sanitizeOutput(message), code, category, false)
	outcome.Status, outcome.TransportStatus, outcome.ProcessStatus = status, tool.TransportNotStarted, tool.ProcessNotApplicable
	outcome.Operation = &tool.OperationDescriptor{Type: toolName, SideEffectState: tool.OutcomeSideEffectNotStarted}
	outcome.RecoveryOptions = recovery
	return outcome.WithTiming(started, time.Now().UTC())
}

func processFailure(started time.Time, planID string, exitCode int, stdout, stderr string, truncated bool, receipt tool.BrokerExecutionReceipt, timedOut, cancelled bool, process tool.ProcessStatus) tool.ExecutionOutcome {
	code, category, status, transport := tool.ErrorCodeProcessExitNonzero, tool.ErrorCategoryProcess, tool.OutcomeFailed, tool.TransportCompleted
	message := fmt.Sprintf("dependency manager exited with code %d; staged changes were discarded", exitCode)
	if timedOut {
		code, transport, message = tool.ErrorCodeProcessTimeout, tool.TransportFailed, "dependency installation timed out; staged changes were discarded"
	} else if cancelled {
		code, category, status, transport, message = tool.ErrorCodeToolCancelled, tool.ErrorCategoryCancelled, tool.OutcomeCancelled, tool.TransportCancelled, "dependency installation was cancelled; staged changes were discarded"
	} else if process == tool.ProcessSpawnFailed {
		code, transport, message = tool.ErrorCodeProcessSpawnFailed, tool.TransportFailed, "dependency manager failed to start; staged changes were discarded"
	}
	outcome := tool.FailedOutcome(message, code, category, false).WithTiming(started, time.Now().UTC())
	outcome.Status, outcome.TransportStatus, outcome.ProcessStatus = status, transport, process
	outcome.ExitCode, outcome.TimedOut = intPointer(exitCode), timedOut
	outcome.Operation = &tool.OperationDescriptor{Type: toolName, Target: planID, SideEffectState: tool.OutcomeSideEffectNone}
	outcome.Details = map[string]any{"plan_id": planID, "exit_code": exitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated, "broker_receipt": receipt}
	outcome.Diagnostic = map[string]any{"broker_receipt": receipt, "staging_discarded": true}
	outcome.RecoveryOptions = []string{"inspect_bounded_manager_output", "refresh_dependency_state_before_any_retry"}
	return outcome
}

func brokerCompletionFailure(started time.Time, planID string, exitCode int, stdout, stderr string, truncated bool, receipt tool.BrokerExecutionReceipt, err error) tool.ExecutionOutcome {
	code, category, status := tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, tool.OutcomeFailed
	if errors.Is(err, tool.ErrExecutionBrokerDenied) {
		code, category, status = tool.ErrorCodeToolDenied, tool.ErrorCategoryPolicy, tool.OutcomeDenied
	}
	outcome := tool.FailedOutcome("dependency install broker completion failed: "+sanitizeOutput(err.Error()), code, category, false).WithTiming(started, time.Now().UTC())
	outcome.Status = status
	outcome.TransportStatus, outcome.ProcessStatus, outcome.ExitCode = tool.TransportCompleted, tool.ProcessExited, intPointer(exitCode)
	outcome.Operation = &tool.OperationDescriptor{Type: toolName, Target: planID, SideEffectState: receipt.SideEffectState}
	outcome.Details = map[string]any{"plan_id": planID, "exit_code": exitCode, "stdout": stdout, "stderr": stderr, "truncated": truncated, "broker_receipt": receipt}
	outcome.Diagnostic = map[string]any{"broker_receipt": receipt}
	outcome.RecoveryOptions = []string{"inspect_live_workspace_and_trash_before_retry", "do_not_blindly_replay"}
	return outcome
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

// Emit is intentionally called only after all chunks have been assembled and
// redacted. Streaming raw Write chunks would leak a token split across chunk
// boundaries into Desktop/journal before the redactor could recognize it.
func (w *boundedOutput) Emit(redacted string, truncated bool) {
	if w.observer == nil || (redacted == "" && !truncated) {
		return
	}
	w.observer.OnCommandOutput(tool.CommandOutput{Stream: w.stream, Delta: redacted, Truncated: truncated})
}

var (
	urlUserInfoPattern    = regexp.MustCompile(`(?i)(https?://)[^/@\s:]+:[^/@\s]+@`)
	urlSecretQueryPattern = regexp.MustCompile(`(?i)([?&](?:token|access_token|auth|api_key|key)=)[^&\s]+`)
	authAssignmentPattern = regexp.MustCompile(`(?i)((?:_authToken|authToken|npm_token|node_auth_token|password|passwd)\s*[=:]\s*)[^\s]+`)
)

func sanitizeOutput(value string) string {
	value = strings.TrimSpace(value)
	value = urlUserInfoPattern.ReplaceAllString(value, `${1}[REDACTED]@`)
	value = urlSecretQueryPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = authAssignmentPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	for _, marker := range []string{"Bearer ", "sk-", "ghp_", "github_pat_", "xoxb-", "xoxp-", "npm_"} {
		for {
			index := strings.Index(value, marker)
			if index < 0 {
				break
			}
			start, end := index+len(marker), index+len(marker)
			for end < len(value) {
				char := value[end]
				if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-_.", rune(char)) {
					end++
					continue
				}
				break
			}
			if end == start {
				break
			}
			value = value[:start] + "[REDACTED]" + value[end:]
		}
	}
	if len([]rune(value)) > fixedOutputLimitRunes {
		value = string([]rune(value)[:fixedOutputLimitRunes]) + "…"
	}
	return value
}

func sameExecutablePath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if resolved, err := filepath.EvalSymlinks(left); err == nil {
		left = resolved
	}
	if resolved, err := filepath.EvalSymlinks(right); err == nil {
		right = resolved
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func containsExact(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func intPointer(value int) *int { return &value }

func init() {
	tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin)
}
