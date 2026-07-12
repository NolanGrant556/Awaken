package shelltool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

const (
	defaultTimeoutMS      = 10 * 60 * 1000
	maxTimeoutMS          = 30 * 60 * 1000
	defaultMaxOutputChars = 12000
	maxOutputCharsLimit   = 50000
)

type ShellTool struct{}

type shellParams struct {
	Command        string `json:"command"`
	Workdir        string `json:"workdir"`
	TimeoutMS      int    `json:"timeout_ms"`
	MaxOutputChars int    `json:"max_output_chars"`
}

type shellResult struct {
	Command               string                       `json:"command"`
	Cwd                   string                       `json:"cwd,omitempty"`
	ExitCode              int                          `json:"exit_code"`
	TimedOut              bool                         `json:"timed_out"`
	Cancelled             bool                         `json:"cancelled,omitempty"`
	DurationMS            int64                        `json:"duration_ms"`
	Stdout                string                       `json:"stdout"`
	Stderr                string                       `json:"stderr"`
	Truncated             bool                         `json:"truncated"`
	EnvironmentSnapshotID string                       `json:"environment_snapshot_id,omitempty"`
	BrokerReceipt         *tool.BrokerExecutionReceipt `json:"broker_receipt,omitempty"`
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Description() string {
	return "使用当前登录用户的本机环境与操作系统权限直接执行 shell/CLI，允许访问用户目录、网络、包管理器、安装脚本和本机程序；返回真实 exit code、耗时、stdout、stderr 与执行 provider receipt。默认 provider 不声称 workspace 隔离、可恢复修改或完整副作用观察"
}

func (t *ShellTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "command", Description: "要执行的 shell 命令", Type: "string", Required: true},
		{Name: "workdir", Description: "命令工作目录，默认当前工作目录", Type: "string", Required: false},
		{Name: "timeout_ms", Description: "超时时间毫秒，默认 600000（10 分钟），最大 1800000（30 分钟）", Type: "integer", Required: false},
		{Name: "max_output_chars", Description: "stdout/stderr 各自最多返回字符数，默认 12000，最大 50000", Type: "integer", Required: false},
	}
}

func (t *ShellTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "shell",
		DisplayName: "Shell",
		Description: t.Description(),
		Purpose:     "以当前登录用户身份执行本机 CLI，并返回真实进程状态、stdout、stderr 与截断事实。",
		WhenToUse: []string{
			"需要运行 Skill 初始化、npm/pip 安装脚本、构建、测试、网络 CLI 或本机应用命令时",
			"需要在 workspace、用户级 Skill 目录、用户缓存或其他用户可访问目录操作时",
			"需要观察真实 exit code、stderr、timeout 或进程级失败时",
		},
		WhenNotToUse: []string{
			"只需读取或搜索时可优先使用 read_file、grep_search；精确编辑时可优先使用 text_edit",
			"需要可恢复删除或可验证文件提交时可优先使用 trash_file 等职责更窄的 typed tool",
			"不要把默认 direct provider 的 receipt 当作文件变化、网络收件人或可恢复删除证明",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"command":                 {Type: "string", Description: "实际执行命令"},
			"cwd":                     {Type: "string", Description: "实际工作目录"},
			"stdout":                  {Type: "string", Description: "有界标准输出"},
			"stderr":                  {Type: "string", Description: "有界标准错误"},
			"cancelled":               {Type: "boolean", Description: "命令是否因调用上下文取消而终止"},
			"reason":                  {Type: "string", Description: "非零退出被判为 neutral 时的机器原因"},
			"environment_snapshot_id": {Type: "string", Description: "本次命令使用的只读环境快照 ID"},
			"broker_receipt":          {Type: "object", Description: "执行 broker 的授权与强制证据", AdditionalProperties: tool.BoolPtr(true)},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectExecute, Resources: []string{"local process and command-selected resources"}, Description: "启动本机进程；具体文件、git、网络或外部副作用由命令决定。", Reversibility: tool.ReversibilityUnknown}},
		Examples:    []tool.ToolExample{{Scenario: "在仓库运行聚焦 Go 测试", Input: map[string]any{"command": "go test ./src/tool", "workdir": ".", "timeout_ms": 120000}, Outcome: "返回 exit code、stdout、stderr、duration 和 process status"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeValidationError, Cause: "命令、工作目录、timeout 或输出上限无效", Retryable: false, Recovery: []string{"修正参数"}},
			{Code: tool.ErrorCodeProcessSpawnFailed, Cause: "本机进程未能启动", Retryable: true, Recovery: []string{"检查可执行文件、工作目录和本机进程环境后重试"}},
			{Code: tool.ErrorCodeProcessExitNonzero, Cause: "进程完成但返回非零退出码", Retryable: false, Recovery: []string{"根据 stderr 和 exit code 修复原因", "不要盲目原样重试"}},
			{Code: tool.ErrorCodeProcessTimeout, Cause: "进程超过 timeout", Retryable: true, Recovery: []string{"检查是否应使用更窄命令或合理增加 timeout"}},
			{Code: tool.ErrorCodeToolDenied, Cause: "显式注入的受限 broker 拒绝命令", Retryable: false, Recovery: []string{"检查该 run 的 broker 合同或使用默认本机 provider"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     false,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilitySystemOperation,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
		Permissions: tool.ToolPermissionSpec{
			CapabilityClass: tool.CapabilitySystemOperation,
			WorkspaceScope:  tool.WorkspaceScopeNone,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeOpen,
		},
		ProviderID:   tool.ProviderID("builtin:shell"),
		Capabilities: []tool.Capability{tool.CapabilityShellExecute},
	})
}

func (t *ShellTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}

	params, err := parseParams(input)
	if err != nil {
		return tool.AllowPermission()
	}
	params.Command = strings.TrimSpace(params.Command)
	if params.Command == "" {
		return tool.AllowPermission()
	}
	cwd := resolveExecutorCwd(params.Workdir, permissionContext)
	policy := EvaluateShellPolicy(params.Command, cwd, permissionContext)
	if policy.DenyReason != "" {
		return tool.DenyPermission(policy.DenyReason)
	}
	if policy.DirectAllow {
		if policy.Lane == ShellLaneInspectReadOnly {
			return tool.AllowPermission()
		}
		if policy.Lane != ShellLaneUnknown {
			return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
				Title:       "Shell " + string(policy.Lane),
				Description: shellApprovalDescription(params, "allowed shell lane: "+string(policy.Lane)),
				RiskTone:    "warning",
			})
		}
		return tool.AllowPermission()
	}
	return tool.AllowPermission()
}

func (t *ShellTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	return outcome.Content, err
}

func (t *ShellTool) ExecuteWithObserver(ctx context.Context, input string, observer tool.ToolObserver) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, observer)
	return outcome.Content, err
}

func (t *ShellTool) DescribeActivity(input string) tool.ToolActivity {
	params, err := parseParams(input)
	if err != nil {
		return tool.ToolActivity{Stage: "started", Title: "正在执行 shell"}
	}
	command := redactShellSecrets(strings.TrimSpace(params.Command))
	if title, detail, ok := installActivity(command); ok {
		return tool.ToolActivity{Stage: "started", Title: title, Detail: detail}
	}
	detail := command
	if len([]rune(detail)) > 160 {
		detail = string([]rune(detail)[:160]) + "…"
	}
	return tool.ToolActivity{
		Stage:  "started",
		Title:  "正在执行命令",
		Detail: detail,
	}
}

func (t *ShellTool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	cwd := resolveExecutorCwd(params.Workdir, permissionContext)
	executor := NewShellExecutor()
	result, err := executor.Execute(ctx, params, permissionContext, observer)
	finished := time.Now().UTC()
	if err != nil {
		msg := err.Error()
		outcome := tool.ExecutionOutcome{
			Content:         msg,
			Status:          tool.OutcomeFailed,
			TransportStatus: tool.TransportFailed,
			SemanticStatus:  tool.SemanticFailure,
			ProcessStatus:   tool.ProcessSpawnFailed,
			ErrorCode:       tool.ErrorCodeProcessSpawnFailed,
			ErrorCategory:   tool.ErrorCategoryProcess,
			ErrorMessage:    msg,
			Retryable:       true,
			Diagnostic: map[string]any{
				"command": redactShellSecrets(params.Command),
				"cwd":     cwd,
			},
		}
		if result.EnvironmentSnapshot.SnapshotID != "" {
			outcome.Diagnostic["environment_snapshot_id"] = result.EnvironmentSnapshot.SnapshotID
		}
		if result.BrokerAuthorization.AuthorizationID != "" {
			outcome.Diagnostic["broker_authorization"] = result.BrokerAuthorization
		}
		if result.BrokerReceipt.AuthorizationID != "" {
			outcome.Diagnostic["broker_receipt"] = result.BrokerReceipt
		}
		switch {
		case errors.Is(err, tool.ErrExecutionBrokerUnavailable), errors.Is(err, tool.ErrExecutionSandboxRequired):
			outcome.Status = tool.OutcomeUnsupported
			outcome.TransportStatus = tool.TransportNotStarted
			outcome.ProcessStatus = tool.ProcessNotApplicable
			outcome.ErrorCode = tool.ErrorCodeExecutionBrokerUnavailable
			outcome.ErrorCategory = tool.ErrorCategoryRuntime
			outcome.Retryable = false
			outcome.RecoveryOptions = []string{"install_or_restore_execution_broker"}
		case errors.Is(err, tool.ErrExecutionBrokerCommit), errors.Is(err, tool.ErrExecutionWorkspaceChanged):
			outcome.Status = tool.OutcomeFailed
			outcome.TransportStatus = tool.TransportCompleted
			outcome.ProcessStatus = tool.ProcessExited
			outcome.ErrorCode = tool.ErrorCodeToolExecutionFailed
			outcome.ErrorCategory = tool.ErrorCategoryTool
			outcome.Retryable = false
			exitCode := result.ExitCode
			outcome.ExitCode = &exitCode
			outcome.RecoveryOptions = []string{"inspect_workspace_and_trash_recovery_bundle", "refresh_workspace_before_retry"}
		case errors.Is(err, tool.ErrExecutionBrokerDenied):
			if result.BrokerReceipt.CommitStatus == "denied_writable_scope" || result.BrokerReceipt.CommitStatus == "denied_protected_root" {
				// The process completed only inside the sealed staging workspace;
				// commit-time scope validation then denied publication before Trash
				// or any live-workspace mutation.
				outcome.Status = tool.OutcomeDenied
				outcome.TransportStatus = tool.TransportCompleted
				outcome.ProcessStatus = tool.ProcessExited
				outcome.ErrorCode = tool.ErrorCodeToolDenied
				outcome.ErrorCategory = tool.ErrorCategoryPolicy
				exitCode := result.ExitCode
				outcome.ExitCode = &exitCode
				outcome.Retryable = false
				outcome.RecoveryOptions = []string{"restrict_command_to_writable_roots", "avoid_protected_roots"}
			} else {
				outcome.Status = tool.OutcomeUnsupported
				outcome.TransportStatus = tool.TransportNotStarted
				outcome.ProcessStatus = tool.ProcessNotApplicable
				outcome.ErrorCode = tool.ErrorCodeSupervisedExecutionRequired
				outcome.ErrorCategory = tool.ErrorCategoryPolicy
				outcome.Retryable = false
				outcome.RecoveryOptions = []string{"use_typed_tool", "prepare_staging_or_overlay_broker"}
			}
		case errors.Is(err, tool.ErrExecutionTransformRequired):
			outcome.Status = tool.OutcomeUnsupported
			outcome.TransportStatus = tool.TransportNotStarted
			outcome.ProcessStatus = tool.ProcessNotApplicable
			outcome.ErrorCode = tool.ErrorCodeSupervisedExecutionRequired
			outcome.ErrorCategory = tool.ErrorCategoryPolicy
			outcome.Retryable = false
			outcome.RecoveryOptions = []string{"use_typed_tool", "prepare_staging_or_overlay_broker"}
		}
		applyBrokerReceiptToOutcome(&outcome, result.BrokerReceipt)
		if outcome.Status == tool.OutcomeFailed && (strings.Contains(strings.ToLower(msg), "denied") || strings.Contains(msg, "policy")) {
			outcome.Status = tool.OutcomeDenied
			outcome.TransportStatus = tool.TransportNotStarted
			outcome.ProcessStatus = tool.ProcessNotApplicable
			outcome.ErrorCode = tool.ErrorCodeToolDenied
			outcome.ErrorCategory = tool.ErrorCategoryPolicy
			outcome.Retryable = false
		}
		return outcome.WithTiming(started, finished), nil
	}

	shellRes := shellResult{
		Command:               redactShellSecrets(params.Command),
		Cwd:                   cwd,
		ExitCode:              result.ExitCode,
		TimedOut:              result.TimedOut,
		Cancelled:             result.Cancelled,
		DurationMS:            result.DurationMS,
		Stdout:                result.Stdout,
		Stderr:                result.Stderr,
		Truncated:             result.Truncated,
		EnvironmentSnapshotID: result.EnvironmentSnapshot.SnapshotID,
	}
	if result.BrokerReceipt.AuthorizationID != "" {
		receipt := result.BrokerReceipt
		shellRes.BrokerReceipt = &receipt
	}
	data, marshalErr := json.MarshalIndent(shellRes, "", "  ")
	if marshalErr != nil {
		return tool.ExecutionOutcome{}, marshalErr
	}
	content := string(data)
	exitCode := result.ExitCode
	outcome := tool.ExecutionOutcome{
		Content:    content,
		ExitCode:   &exitCode,
		TimedOut:   result.TimedOut,
		Truncated:  result.Truncated,
		DurationMs: result.DurationMS,
		Diagnostic: map[string]any{
			"command": shellRes.Command,
			"cwd":     cwd,
			"stdout":  result.Stdout,
			"stderr":  result.Stderr,
		},
	}
	outcome.Details = map[string]any{
		"command": shellRes.Command, "cwd": cwd,
		"stdout": result.Stdout, "stderr": result.Stderr,
		"environment_snapshot_id": result.EnvironmentSnapshot.SnapshotID,
	}
	if shellRes.BrokerReceipt != nil {
		outcome.Details["broker_receipt"] = shellRes.BrokerReceipt
	}
	outcome.Environment = &tool.EnvironmentObservation{
		Observation: tool.ObservationPartial, ObservedAt: finished, Scope: cwd,
		Source:        "execution_environment_snapshot:" + result.EnvironmentSnapshot.SnapshotID,
		WorkspaceRoot: permissionContext.WorkspaceRoot, WorkingDirectory: cwd,
		Processes:  []tool.ProcessObservation{{Status: tool.ProcessExited, ExitCode: &exitCode, CWD: cwd}},
		Unobserved: []string{"git", "environment_variable_values"},
	}

	switch {
	case result.Cancelled:
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.SemanticStatus = tool.SemanticFailure
		outcome.ProcessStatus = tool.ProcessSignaled
		outcome.ErrorCode = tool.ErrorCodeToolCancelled
		outcome.ErrorCategory = tool.ErrorCategoryCancelled
		outcome.ErrorMessage = "command cancelled"
		outcome.Retryable = false
	case result.TimedOut:
		outcome.Status = tool.OutcomeFailed
		outcome.TransportStatus = tool.TransportFailed
		outcome.SemanticStatus = tool.SemanticFailure
		outcome.ProcessStatus = tool.ProcessTimedOut
		outcome.ErrorCode = tool.ErrorCodeProcessTimeout
		outcome.ErrorCategory = tool.ErrorCategoryProcess
		outcome.ErrorMessage = "command timed out"
		outcome.Retryable = true
		outcome.RecoveryOptions = []string{"increase_timeout", "change_command"}
	case result.ExitCode == 0:
		outcome.Status = tool.OutcomeSucceeded
		outcome.TransportStatus = tool.TransportCompleted
		outcome.SemanticStatus = tool.SemanticSuccess
		outcome.ProcessStatus = tool.ProcessExited
	default:
		if reason, ok := neutralNonzeroReason(params.Command, result.ExitCode); ok {
			outcome.Status = tool.OutcomeSucceeded
			outcome.TransportStatus = tool.TransportCompleted
			outcome.SemanticStatus = tool.SemanticNeutral
			outcome.ProcessStatus = tool.ProcessExited
			outcome.Diagnostic["reason"] = reason
			outcome.Details["reason"] = reason
		} else {
			outcome.Status = tool.OutcomeFailed
			outcome.TransportStatus = tool.TransportCompleted
			outcome.SemanticStatus = tool.SemanticFailure
			outcome.ProcessStatus = tool.ProcessExited
			outcome.ErrorCode = tool.ErrorCodeProcessExitNonzero
			outcome.ErrorCategory = tool.ErrorCategoryProcess
			outcome.ErrorMessage = fmt.Sprintf("process exited with code %d", result.ExitCode)
			outcome.Retryable = false
			outcome.RecoveryOptions = []string{"inspect_exit_code_and_stderr", "change_command_or_stop"}
		}
	}
	applyBrokerReceiptToOutcome(&outcome, result.BrokerReceipt)
	return outcome.WithTiming(started, finished), nil
}

func applyBrokerReceiptToOutcome(outcome *tool.ExecutionOutcome, receipt tool.BrokerExecutionReceipt) {
	if outcome == nil || receipt.AuthorizationID == "" {
		return
	}
	if receipt.SideEffectState != "" {
		if outcome.Operation == nil {
			outcome.Operation = &tool.OperationDescriptor{Type: "shell", Target: receipt.WorkspaceRoot}
		}
		outcome.Operation.SideEffectState = receipt.SideEffectState
	}
	for _, change := range receipt.WorkspaceChanges {
		details := map[string]any{"broker_id": receipt.BrokerID, "commit_status": receipt.CommitStatus}
		if change.RecoveryPath != "" {
			details["recovery_path"] = change.RecoveryPath
			details["recovery_method"] = change.RecoveryMethod
		}
		reversibility := tool.ChangeConditional
		summary := "Workspace change committed by the supervised execution broker."
		if change.Action == "deleted" {
			summary = "Workspace resource moved into a recoverable Trash bundle."
		}
		outcome.Changes = append(outcome.Changes, tool.ResourceChange{
			Kind: "file", Action: change.Action, Resource: change.Path, Summary: summary,
			Certainty: tool.ChangeObserved, Reversibility: reversibility, Details: details,
		})
		outcome.AffectedPaths = append(outcome.AffectedPaths, change.Path)
	}
}

func installActivity(command string) (title, detail string, ok bool) {
	lower := strings.ToLower(command)
	switch {
	case strings.Contains(lower, "python -m pip install"), strings.Contains(lower, "pip install"), strings.Contains(lower, "pip3 install"):
		return "正在安装依赖", redactShellSecrets(command), true
	case strings.Contains(lower, "npm install"), strings.Contains(lower, "npm i "):
		return "正在安装依赖", redactShellSecrets(command), true
	case strings.Contains(lower, "pnpm install"), strings.Contains(lower, "pnpm add"):
		return "正在安装依赖", redactShellSecrets(command), true
	case strings.Contains(lower, "yarn add"), strings.Contains(lower, "yarn install"):
		return "正在安装依赖", redactShellSecrets(command), true
	case strings.Contains(lower, "brew install"):
		return "正在安装依赖", redactShellSecrets(command), true
	case strings.Contains(lower, "winget install"):
		return "正在安装依赖", redactShellSecrets(command), true
	default:
		return "", "", false
	}
}

func neutralNonzeroReason(command string, exitCode int) (string, bool) {
	if exitCode != 1 {
		return "", false
	}
	segments := splitCompoundSegments(command)
	if len(segments) != 1 {
		return "", false
	}
	tokens := shellTokens(segments[0])
	if len(tokens) == 0 {
		return "", false
	}
	base := strings.ToLower(filepath.Base(tokens[0]))
	switch base {
	case "rg", "grep", "egrep", "fgrep":
		return "no_match", true
	case "diff":
		return "different", true
	case "test", "[":
		return "false_condition", true
	default:
		return "", false
	}
}

func redactShellSecrets(command string) string {
	redacted := command
	patterns := []struct {
		prefix string
		minLen int
	}{
		{"sk-", 8},
		{"ak-", 8},
		{"Bearer ", 8},
	}
	for _, pattern := range patterns {
		for {
			idx := strings.Index(redacted, pattern.prefix)
			if idx < 0 {
				break
			}
			start := idx + len(pattern.prefix)
			end := start
			for end < len(redacted) {
				ch := redacted[end]
				if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
					end++
					continue
				}
				break
			}
			if end-start >= pattern.minLen {
				redacted = redacted[:start] + "[redacted]" + redacted[end:]
			} else {
				break
			}
		}
	}
	return redacted
}

type shellOutputWriter struct {
	stream        string
	buffer        *outputlimit.CappedBuffer
	observer      tool.ToolObserver
	maxChars      int
	emittedChars  int
	truncatedSent bool
	mu            sync.Mutex
}

func newShellOutputWriter(stream string, maxChars int, observer tool.ToolObserver) *shellOutputWriter {
	return &shellOutputWriter{
		stream:   stream,
		buffer:   outputlimit.NewCappedBuffer(maxChars),
		observer: observer,
		maxChars: maxChars,
	}
}

func (w *shellOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.buffer.Write(p)
	w.emitDelta(string(p))
	return n, err
}

func (w *shellOutputWriter) Result() outputlimit.Result {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Result()
}

func (w *shellOutputWriter) emitDelta(delta string) {
	if w.observer == nil || delta == "" || w.maxChars <= 0 {
		return
	}

	runes := []rune(delta)
	remaining := w.maxChars - w.emittedChars
	if remaining > 0 {
		if remaining > len(runes) {
			remaining = len(runes)
		}
		chunk := string(runes[:remaining])
		w.emittedChars += remaining
		w.observer.OnCommandOutput(tool.CommandOutput{
			Stream: w.stream,
			Delta:  chunk,
		})
	}

	if w.emittedChars >= w.maxChars && len(runes) > remaining && !w.truncatedSent {
		w.truncatedSent = true
		w.observer.OnCommandOutput(tool.CommandOutput{
			Stream:    w.stream,
			Delta:     "\n[... output truncated ...]\n",
			Truncated: true,
		})
	}
}

func shellApprovalDescription(params shellParams, reason string) string {
	var builder strings.Builder
	builder.WriteString(reason)
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(params.Command))
	if strings.TrimSpace(params.Workdir) != "" {
		builder.WriteString("\n\nworkdir: ")
		builder.WriteString(strings.TrimSpace(params.Workdir))
	}
	return builder.String()
}

func containsAnyShellToken(values []string, candidates ...string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func parseParams(input string) (shellParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return shellParams{}, nil
	}

	var params shellParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return shellParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	return params, nil
}

func normalizeTimeout(value int) time.Duration {
	if value <= 0 {
		value = defaultTimeoutMS
	}
	if value > maxTimeoutMS {
		value = maxTimeoutMS
	}
	return time.Duration(value) * time.Millisecond
}

func normalizeMaxOutputChars(value int) int {
	if value <= 0 {
		return defaultMaxOutputChars
	}
	if value > maxOutputCharsLimit {
		return maxOutputCharsLimit
	}
	return value
}

func init() {
	tool.MustRegisterToolWithMode(&ShellTool{}, tool.DefinitionValidationStrictBuiltin)
}
