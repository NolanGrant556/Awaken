package shelltool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
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

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", params.Command)
	if strings.TrimSpace(params.Workdir) != "" {
		cmd.Dir = strings.TrimSpace(params.Workdir)
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
