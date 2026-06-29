package shelltool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/powershellpolicy"
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

	stdout := newShellOutputWriter("stdout", maxOutputChars, observer)
	stderr := newShellOutputWriter("stderr", maxOutputChars, observer)
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

func shellPermissionReason(params shellParams, permissionContext tool.PermissionContext) (string, string) {
	if workdirOutsideWorkspace(params.Workdir, permissionContext) {
		return "shell command uses a working directory outside the workspace root", "danger"
	}
	if decision := powershellpolicy.Analyze(params.Command); decision.Behavior == powershellpolicy.BehaviorAsk {
		return "PowerShell policy: " + decision.Message, "danger"
	}
	if isDangerousShellCommand(params.Command) {
		return "shell command may mutate files, change repository state, or access external systems", "danger"
	}
	if hasShellControlOperator(params.Command) {
		return "shell command contains operators, redirections, substitutions, or multiple operations", "warning"
	}
	if isSafeReadOnlyShellCommand(params.Command) {
		paths, ok := safeReadOnlyCommandPaths(params.Command)
		if !ok {
			return "shell command uses an unrecognized argument pattern and may access files outside the workspace", "warning"
		}
		for _, p := range paths {
			if !permissionContext.IsWithinWorkspaceResolved(p) {
				return fmt.Sprintf("shell command would read path outside workspace: %s", p), "warning"
			}
		}
		return "", ""
	}
	return "shell command is not classified as read-only", "warning"
}

// safeReadOnlyCommandPaths extracts filesystem path arguments from a simple read-only shell command.
// It returns ok=false when the argument pattern is too complex to analyze safely (e.g. unknown options),
// in which case the caller should request approval instead of allowing the command.
func safeReadOnlyCommandPaths(command string) ([]string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return nil, true
	}
	base := filepath.Base(fields[0])
	args := fields[1:]

	switch base {
	case "pwd", "echo", "printf", "which", "whereis", "whoami", "date":
		return nil, true
	case "cat", "head", "tail", "wc", "stat", "file", "strings":
		return trailingFileArgs(args, map[string]bool{"-n": true, "-c": true, "-w": true, "-l": true, "-m": true, "-L": true})
	case "ls", "tree", "du":
		return trailingFileArgs(args, map[string]bool{"-a": false, "-l": false, "-h": false, "-r": false, "-s": false, "-d": true, "-L": false})
	case "grep":
		return grepPathArgs(args)
	case "rg":
		return rgPathArgs(args)
	case "find":
		return findPathArgs(args)
	case "git":
		// Read-only git commands (status/diff/log/...) do not read arbitrary external paths.
		return nil, true
	default:
		return trailingFileArgs(args, nil)
	}
}

func trailingFileArgs(args []string, optionArgs map[string]bool) ([]string, bool) {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}
		if takesOptionArgument(arg, optionArgs) {
			i += 2
			if i > len(args) {
				return nil, false
			}
		} else {
			i++
		}
	}
	var paths []string
	for ; i < len(args); i++ {
		if args[i] != "-" {
			paths = append(paths, args[i])
		}
	}
	return paths, true
}

func takesOptionArgument(arg string, optionArgs map[string]bool) bool {
	if optionArgs == nil {
		return false
	}
	// Short option stack like -n10 or -abc is not supported; treat as no-argument.
	if _, ok := optionArgs[arg]; ok {
		return ok
	}
	return false
}

func grepPathArgs(args []string) ([]string, bool) {
	i := 0
	var paths []string
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}
		switch arg {
		case "-e", "--regexp":
			i += 2
			if i > len(args) {
				return nil, false
			}
		case "-f", "--file":
			i++
			if i >= len(args) {
				return nil, false
			}
			paths = append(paths, args[i])
			i++
		case "-d", "-D", "--binary-files", "--color", "-C", "-B", "-A", "--context", "--group-separator", "--line-buffered":
			i += 2
			if i > len(args) {
				return nil, false
			}
		default:
			if strings.HasPrefix(arg, "--file=") {
				paths = append(paths, strings.TrimPrefix(arg, "--file="))
				i++
				continue
			}
			// Unknown grep option; argument pattern is too complex.
			if strings.HasPrefix(arg, "--") {
				return nil, false
			}
			// Short options we don't recognize; conservative.
			return nil, false
		}
	}
	if i >= len(args) {
		return paths, true
	}
	// First non-option is the pattern; the rest are files.
	for j := i + 1; j < len(args); j++ {
		if args[j] != "-" {
			paths = append(paths, args[j])
		}
	}
	return paths, true
}

func rgPathArgs(args []string) ([]string, bool) {
	i := 0
	var paths []string
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}
		switch arg {
		case "-e", "--regexp":
			i += 2
			if i > len(args) {
				return nil, false
			}
		case "-f", "--file":
			i++
			if i >= len(args) {
				return nil, false
			}
			paths = append(paths, args[i])
			i++
		case "-g", "--glob", "-t", "--type", "-T", "--type-not", "-C", "-B", "-A", "--context", "--sort", "--sortr", "--binary-files", "--color", "--colors", "--line-buffered":
			i += 2
			if i > len(args) {
				return nil, false
			}
		default:
			if strings.HasPrefix(arg, "--file=") {
				paths = append(paths, strings.TrimPrefix(arg, "--file="))
				i++
				continue
			}
			if strings.HasPrefix(arg, "--") {
				return nil, false
			}
			return nil, false
		}
	}
	if i >= len(args) {
		return paths, true
	}
	// First non-option is the pattern; the rest are paths.
	for j := i + 1; j < len(args); j++ {
		paths = append(paths, args[j])
	}
	return paths, true
}

func findPathArgs(args []string) ([]string, bool) {
	expressionMarkers := map[string]bool{
		"-name": true, "-iname": true, "-type": true, "-mtime": true,
		"-exec": true, "-execdir": true, "-ok": true, "-delete": true,
		"-print": true, "-print0": true, "-printf": true,
		"-and": true, "-or": true, "-not": true, "!": true,
		"-maxdepth": true, "-mindepth": true, "-path": true, "-ipath": true,
		"-newer": true, "-size": true, "-empty": true, "-perm": true,
		"-user": true, "-group": true, "-links": true,
	}
	globalOptions := map[string]bool{"-L": true, "-P": true, "-H": true, "-D": true, "-O": true}

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if globalOptions[arg] {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") || expressionMarkers[arg] {
			break
		}
		i++
	}
	// Everything before i is a path argument.
	return args[:i], true
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
