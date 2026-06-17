package shelltool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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
	return "执行高级 shell 命令并返回 exit code、耗时、stdout 和 stderr；普通文件增删改应优先使用 apply_patch"
}

func (t *ShellTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "command", Description: "要执行的 shell 命令", Type: "string", Required: true},
		{Name: "workdir", Description: "命令工作目录，默认当前工作目录", Type: "string", Required: false},
		{Name: "timeout_ms", Description: "超时时间毫秒，默认 10000，最大 120000", Type: "integer", Required: false},
		{Name: "max_output_chars", Description: "stdout/stderr 各自最多返回字符数，默认 12000，最大 50000", Type: "integer", Required: false},
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

	reason, riskTone := shellPermissionReason(params, permissionContext)
	if reason == "" {
		return tool.AllowPermission()
	}
	return tool.AskPermission(reason, tool.ApprovalRequest{
		Title:       "Approve shell command",
		Description: shellApprovalDescription(params, reason),
		RiskTone:    riskTone,
	})
}

func (t *ShellTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}
	params.Command = strings.TrimSpace(params.Command)
	if params.Command == "" {
		return "", fmt.Errorf("缺少必需参数: command")
	}

	timeout := normalizeTimeout(params.TimeoutMS)
	maxOutputChars := normalizeMaxOutputChars(params.MaxOutputChars)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	permissionContext := tool.PermissionContextFromContext(ctx)

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", params.Command)
	if strings.TrimSpace(params.Workdir) != "" {
		cmd.Dir = permissionContext.ResolvePath(params.Workdir)
	} else {
		cmd.Dir = permissionContext.WorkspaceRootOrCWD()
	}

	stdout := outputlimit.NewCappedBuffer(maxOutputChars)
	stderr := outputlimit.NewCappedBuffer(maxOutputChars)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err = cmd.Run()
	duration := time.Since(started)
	timedOut := runCtx.Err() == context.DeadlineExceeded
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if timedOut {
			exitCode = -1
		} else {
			return "", fmt.Errorf("执行命令失败: %w", err)
		}
	}

	stdoutResult := stdout.Result()
	stderrResult := stderr.Result()
	result := shellResult{
		ExitCode:   exitCode,
		TimedOut:   timedOut,
		DurationMS: duration.Milliseconds(),
		Stdout:     stdoutResult.Content,
		Stderr:     stderrResult.Content,
		Truncated:  stdoutResult.Truncated || stderrResult.Truncated,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func shellPermissionReason(params shellParams, permissionContext tool.PermissionContext) (string, string) {
	if workdirOutsideWorkspace(params.Workdir, permissionContext) {
		return "shell command uses a working directory outside the workspace root", "danger"
	}
	if isDangerousShellCommand(params.Command) {
		return "shell command may mutate files, change repository state, or access external systems", "danger"
	}
	if hasShellControlOperator(params.Command) {
		return "shell command contains operators, redirections, substitutions, or multiple operations", "warning"
	}
	if isSafeReadOnlyShellCommand(params.Command) {
		return "", ""
	}
	return "shell command is not classified as read-only", "warning"
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

func hasShellControlOperator(command string) bool {
	for _, marker := range []string{"\n", "&&", "||", ";", "|", ">", "<", "`", "$("} {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func isSafeReadOnlyShellCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return true
	}
	base := filepath.Base(fields[0])
	switch base {
	case "pwd", "ls", "cat", "head", "tail", "wc", "stat", "file", "strings", "rg", "grep", "tree", "du", "echo", "printf", "which", "whereis", "whoami", "date":
		return true
	case "find":
		return !containsAnyShellToken(fields[1:], "-delete", "-exec", "-execdir")
	case "git":
		return isSafeReadOnlyGitCommand(fields[1:])
	default:
		return false
	}
}

func isSafeReadOnlyGitCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "status", "diff", "log", "show", "rev-parse", "ls-files":
		return true
	case "branch":
		return len(args) == 2 && args[1] == "--show-current"
	default:
		return false
	}
}

func isDangerousShellCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	switch base {
	case "rm", "mv", "cp", "mkdir", "rmdir", "chmod", "chown", "chgrp", "touch", "ln", "sudo", "doas", "pkexec", "curl", "wget", "ssh", "scp", "rsync":
		return true
	case "git":
		return containsAnyShellToken(fields[1:], "push", "reset", "clean", "checkout", "switch", "commit", "merge", "rebase", "pull", "fetch", "restore")
	default:
		return false
	}
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

func workdirOutsideWorkspace(workdir string, permissionContext tool.PermissionContext) bool {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return false
	}
	return !permissionContext.IsWithinWorkspace(workdir)
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
