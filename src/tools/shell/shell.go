package shelltool

import (
	"context"
	"encoding/json"
	"fmt"
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
	return "执行本机 shell/CLI 命令并返回 exit code、耗时、stdout 和 stderr；可调用本机已安装 CLI（如 dws、gh、feishu、obsidian）。普通文件增删改应优先使用 apply_patch；删除本地文件必须走回收站/Trash 工具，不要用 rm/rmdir；涉及群发、批量通知或机器人广播必须走人工审核能力"
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
		Description: "执行本机 shell/CLI 命令；本地文件删除必须走回收站，群发/批量通知必须走人工审核能力",
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
	if policy.DirectAllow && policy.Lane != ShellLaneInspectReadOnly {
		return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
			Title:       "Shell " + string(policy.Lane),
			Description: shellApprovalDescription(params, "allowed shell lane: "+string(policy.Lane)),
			RiskTone:    "warning",
		})
	}
	tier := ClassifyShellCommand(params.Command, cwd)
	switch tier {
	case ShellTierInspectReadOnly:
		// Safe read-only: auto-allow. DecideExecution will hit the
		// StaticBehavior==Allow branch and allow without transform.
		return tool.AllowPermission()
	case ShellTierLocalCLI:
		return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
			Title:       "Shell local_cli",
			Description: shellApprovalDescription(params, "local CLI command"),
			RiskTone:    "warning",
		})
	case ShellTierWorkspaceMutation:
		// Internal risk signal: routes to decideShellTransform, which
		// rewrites simple file writes to apply_patch or structurally denies
		// unsupported workspace mutation. This is metadata, not approval.
		return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
			Title:       "Shell workspace mutation",
			Description: shellApprovalDescription(params, "workspace mutation"),
			RiskTone:    "warning",
		})
	case ShellTierOpenWorldSystem:
		// Open-world shell is not sent to approval. The policy layer denies
		// it or transforms a narrow file-write pattern when possible.
		return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
			Title:       "Shell open-world command",
			Description: shellApprovalDescription(params, "open-world system command"),
			RiskTone:    "danger",
		})
	}
	return tool.AllowPermission()
}

func (t *ShellTool) Execute(ctx context.Context, input string) (string, error) {
	return t.execute(ctx, input, nil)
}

func (t *ShellTool) ExecuteWithObserver(ctx context.Context, input string, observer tool.ToolObserver) (string, error) {
	return t.execute(ctx, input, observer)
}

func (t *ShellTool) execute(ctx context.Context, input string, observer tool.ToolObserver) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	executor := NewShellExecutor()
	result, err := executor.Execute(ctx, params, permissionContext, observer)
	if err != nil {
		return "", err
	}

	shellRes := shellResult{
		ExitCode:   result.ExitCode,
		TimedOut:   result.TimedOut,
		DurationMS: result.DurationMS,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		Truncated:  result.Truncated,
	}
	data, err := json.MarshalIndent(shellRes, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
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
