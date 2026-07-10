package shelltool

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

const (
	defaultTimeoutMS      = 10000
	maxTimeoutMS          = 120000
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
	Command    string `json:"command"`
	Cwd        string `json:"cwd,omitempty"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMS int64  `json:"duration_ms"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Truncated  bool   `json:"truncated"`
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Description() string {
	return "执行本机 shell/CLI 命令并返回 exit code、耗时、stdout 和 stderr；可读取和操作项目内外本机路径，可调用本机已安装 CLI（如 dws、gh、feishu、obsidian）。删除本地文件必须走 trash_file/Trash 工具，不要用 rm/rmdir/unlink；涉及群发、批量通知或机器人广播必须走人工审核能力"
}

func (t *ShellTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "command", Description: "要执行的 shell 命令", Type: "string", Required: true},
		{Name: "workdir", Description: "命令工作目录，默认当前工作目录", Type: "string", Required: false},
		{Name: "timeout_ms", Description: "超时时间毫秒，默认 10000，最大 120000", Type: "integer", Required: false},
		{Name: "max_output_chars", Description: "stdout/stderr 各自最多返回字符数，默认 12000，最大 50000", Type: "integer", Required: false},
	}
}

func (t *ShellTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        "shell",
		DisplayName: "Shell",
		Source:      tool.SourceBuiltin,
		Description: "执行本机 shell/CLI 命令，可操作项目内外本机路径；本地删除必须走回收站，群发/批量通知必须走人工审核能力",
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     true,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilitySystemOperation,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
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
	if metadata, ok := MassMessageReviewMetadataForShellCommand(params.Command); ok {
		return tool.NeedsMassMessageReviewPermission("mass message CLI command requires human review", tool.DecisionMetadata{
			Title:       "Shell mass message review",
			Description: shellApprovalDescription(params, "mass message CLI command"),
			RiskTone:    "danger",
			MassMessage: metadata,
		})
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
		if strings.Contains(strings.ToLower(msg), "denied") || strings.Contains(msg, "policy") {
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
		Command:    redactShellSecrets(params.Command),
		Cwd:        cwd,
		ExitCode:   result.ExitCode,
		TimedOut:   result.TimedOut,
		DurationMS: result.DurationMS,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		Truncated:  result.Truncated,
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

	switch {
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
		} else {
			outcome.Status = tool.OutcomeFailed
			outcome.TransportStatus = tool.TransportCompleted
			outcome.SemanticStatus = tool.SemanticFailure
			outcome.ProcessStatus = tool.ProcessExited
			outcome.ErrorCode = tool.ErrorCodeProcessExitNonzero
			outcome.ErrorCategory = tool.ErrorCategoryProcess
			outcome.ErrorMessage = fmt.Sprintf("process exited with code %d", result.ExitCode)
			outcome.Retryable = true
			outcome.RecoveryOptions = []string{"inspect_exit_code_and_stderr", "change_command_or_stop"}
		}
	}
	return outcome.WithTiming(started, finished), nil
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
	tool.MustRegisterTool(&ShellTool{})
}
