package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/textfile"
)

const (
	ToolName             = "Bash"
	defaultTimeoutMillis = 120000
	maxTimeoutMillis     = 600000
	maxOutputBytes       = 50000
)

type Tool struct{}
type params struct {
	Command         string `json:"command"`
	Timeout         int    `json:"timeout,omitempty"`
	Description     string `json:"description,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
}

type result struct {
	Command               string `json:"command"`
	Shell                 string `json:"shell"`
	CWD                   string `json:"cwd"`
	Stdout                string `json:"stdout,omitempty"`
	Stderr                string `json:"stderr,omitempty"`
	ExitCode              *int   `json:"exit_code,omitempty"`
	TimedOut              bool   `json:"timed_out,omitempty"`
	Cancelled             bool   `json:"cancelled,omitempty"`
	Truncated             bool   `json:"truncated,omitempty"`
	DurationMS            int64  `json:"duration_ms"`
	EnvironmentSnapshotID string `json:"environment_snapshot_id,omitempty"`
	TaskID                string `json:"task_id,omitempty"`
	LogPath               string `json:"log_path,omitempty"`
	StatusPath            string `json:"status_path,omitempty"`
}

func (t *Tool) Name() string { return ToolName }
func (t *Tool) Description() string {
	return "Run a command in the current user's real shell environment without loading rc files. CWD is isolated per thread; foreground commands time out after 120 seconds by default, and background commands return task/log/status paths immediately."
}
func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "command", Description: "Command text to execute.", Type: "string", Required: true},
		{Name: "timeout", Description: "Timeout in milliseconds. Defaults to 120000 and is capped at 600000.", Type: "integer"},
		{Name: "description", Description: "Short user-facing description of the command.", Type: "string"},
		{Name: "run_in_background", Description: "Start a thread-owned background task and return immediately.", Type: "boolean"},
	}
}
func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: ToolName, DisplayName: "Bash", Description: t.Description(),
		Purpose:      "Execute current-user shell commands with real cwd, exit, timeout, output, cancellation, and background-task facts; Bash remains independent from the Computer lane.",
		WhenToUse:    []string{"Running Python, Typst, builds, tests, Skills, package tools, converters, network CLIs, or local applications", "Creating non-text files with installed user tools before artifact_publish", "Running terminal-driven GUI automation only when the user explicitly requests that method"},
		WhenNotToUse: []string{"Use Read, Edit, Write, Glob, or Grep for their narrower operations", "Use computer_observe, computer_act, and computer_verify for direct GUI observation or input", "Do not silently use AppleScript, xdotool, coordinate scripts, or similar terminal automation to impersonate unavailable Computer Use", "Use trash_file for an explicit recoverable deletion"},
		SideEffects:  []tool.ToolSideEffect{{Kind: tool.SideEffectExecute, Resources: []string{"current-user process and command-selected resources"}, Description: "Starts a process with current-user OS and network authority.", Reversibility: tool.ReversibilityUnknown}},
		Examples:     []tool.ToolExample{{Scenario: "Run Go tests", Input: map[string]any{"command": "go test ./...", "timeout": 120000, "description": "Run Go tests"}, Outcome: "Returns shell, cwd, stdout/stderr, exit code, timeout, and truncation facts."}},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeProcessSpawnFailed, Cause: "The selected shell process could not start", Retryable: true},
			{Code: tool.ErrorCodeProcessExitNonzero, Cause: "The process exited nonzero", Retryable: false},
			{Code: tool.ErrorCodeProcessTimeout, Cause: "The process exceeded timeout", Retryable: true},
		},
		OutputSchema: tool.ToolParam{Type: "object", Description: "Shell, cwd, streams, exit, timeout, truncation, and background-task details.", AdditionalProperties: tool.BoolPtr(true)},
		Annotations:  tool.ToolAnnotations{ReadOnly: false, OpenWorld: true, CapabilityClass: tool.CapabilitySystemOperation, TrustedOrigin: tool.TrustedOriginBuiltin},
		Permissions:  tool.ToolPermissionSpec{CapabilityClass: tool.CapabilitySystemOperation, WorkspaceScope: tool.WorkspaceScopeNone, FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeOpen},
		ProviderID:   tool.ProviderID("builtin:bash"), Capabilities: []tool.Capability{tool.CapabilityShellExecute},
	})
}

func (t *Tool) CheckPermission(ctx context.Context, input string, _ tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}
	if _, err := parseParams(input); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermission()
}

func (t *Tool) DescribeActivity(input string) tool.ToolActivity {
	p, _ := parseParams(input)
	title := strings.TrimSpace(p.Description)
	if title == "" {
		title = "Running command"
	}
	detail := strings.TrimSpace(p.Command)
	if len(detail) > 180 {
		detail = detail[:180] + "…"
	}
	return tool.ToolActivity{Stage: "started", Title: title, Detail: detail}
}

func (t *Tool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	return outcome.Content, err
}
func (t *Tool) ExecuteWithObserver(ctx context.Context, input string, observer tool.ToolObserver) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, observer)
	return outcome.Content, err
}

func (t *Tool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	p, err := parseParams(input)
	if err != nil {
		return tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC()), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	state, _ := tool.ToolSessionStateFromContext(ctx)
	cwd := permissionContext.WorkspaceRootOrCWD()
	if state != nil {
		cwd = state.CWD(cwd)
	}
	if info, statErr := os.Stat(cwd); statErr != nil || !info.IsDir() {
		return tool.FailedOutcome(fmt.Sprintf("Bash cwd is unavailable: %s", cwd), "bash_cwd_unavailable", tool.ErrorCategoryRuntime, true).WithTiming(started, time.Now().UTC()), nil
	}
	hostEnvironment, err := pinnedHostEnvironment(ctx, cwd)
	if err != nil {
		return tool.FailedOutcome(err.Error(), tool.ErrorCodeNeedsRuntime, tool.ErrorCategoryRuntime, true).WithTiming(started, time.Now().UTC()), nil
	}
	shell, args, kind, err := selectShell(hostEnvironment, p.Command)
	if err != nil {
		return tool.FailedOutcome(err.Error(), tool.ErrorCodeProcessSpawnFailed, tool.ErrorCategoryRuntime, true).WithTiming(started, time.Now().UTC()), nil
	}
	if p.RunInBackground {
		backgroundResult, backgroundErr := startBackgroundTask(ctx, p, cwd, shell, args, kind, hostEnvironment, state)
		if backgroundErr != nil {
			return tool.FailedOutcome(backgroundErr.Error(), tool.ErrorCodeProcessSpawnFailed, tool.ErrorCategoryProcess, true).WithTiming(started, time.Now().UTC()), nil
		}
		payload, _ := json.MarshalIndent(backgroundResult, "", "  ")
		outcome := tool.SucceededOutcome(string(payload)).WithTiming(started, time.Now().UTC())
		outcome.Summary = fmt.Sprintf("Started background task %s.", backgroundResult.TaskID)
		outcome.Details = map[string]any{"task_id": backgroundResult.TaskID, "log_path": backgroundResult.LogPath, "status_path": backgroundResult.StatusPath, "cwd": cwd, "shell": shell}
		return outcome, nil
	}
	execution := runForeground(ctx, p, cwd, shell, args, kind, hostEnvironment, observer, state)
	return outcomeFromResult(execution, started), nil
}

func parseParams(input string) (params, error) {
	var p params
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(input)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return params{}, fmt.Errorf("invalid Bash input: %w", err)
	}
	p.Command = strings.TrimSpace(p.Command)
	if p.Command == "" {
		return params{}, fmt.Errorf("command is required")
	}
	if p.Timeout == 0 {
		p.Timeout = defaultTimeoutMillis
	}
	if p.Timeout < 1 || p.Timeout > maxTimeoutMillis {
		return params{}, fmt.Errorf("timeout must be between 1 and %d milliseconds", maxTimeoutMillis)
	}
	return p, nil
}

func pinnedHostEnvironment(ctx context.Context, cwd string) (tool.HostEnvironment, error) {
	if snapshot, ok := tool.ExecutionEnvironmentSnapshotFromContext(ctx); ok {
		if snapshot.PATHTruncated && snapshot.PinnedTransportPATH() == "" {
			return tool.HostEnvironment{}, fmt.Errorf("pinned environment PATH is unavailable")
		}
		return tool.HostEnvironmentFromSnapshot(snapshot), nil
	}
	return tool.ResolveHostEnvironment(ctx, tool.ExecutionEnvironmentProbeOptions{WorkingDirectory: cwd, WorkspaceRoot: cwd, RefreshReason: "bash_direct", ProbeVersions: false}), nil
}

func selectShell(environment tool.HostEnvironment, command string) (string, []string, string, error) {
	if runtime.GOOS == "windows" {
		for _, candidate := range []struct{ name, kind string }{{"bash", "posix"}, {"pwsh", "powershell"}, {"powershell", "powershell"}, {"cmd", "cmd"}} {
			path := findExecutable(environment, candidate.name)
			if path == "" {
				continue
			}
			return path, invocationArgs(path, candidate.kind, command), candidate.kind, nil
		}
		return "", nil, "", fmt.Errorf("no supported shell found (Git Bash, pwsh, powershell, cmd)")
	}
	shell := strings.TrimSpace(environment.Shell)
	if shell == "" {
		shell = findExecutable(environment, "bash")
	}
	if shell == "" {
		return "", nil, "", fmt.Errorf("no current-user shell is available")
	}
	kind := "posix"
	return shell, invocationArgs(shell, kind, command), kind, nil
}

func invocationArgs(shell, kind, command string) []string {
	switch kind {
	case "powershell":
		return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command}
	case "cmd":
		return []string{"/D", "/V:ON", "/S", "/C", command}
	}
	switch strings.ToLower(filepath.Base(shell)) {
	case "bash", "bash.exe":
		return []string{"--noprofile", "--norc", "-c", command}
	case "zsh", "zsh.exe":
		return []string{"-f", "-c", command}
	case "fish", "fish.exe":
		return []string{"--no-config", "-c", command}
	default:
		return []string{"-c", command}
	}
}

func findExecutable(environment tool.HostEnvironment, name string) string {
	if observed, ok := environment.Snapshot.Executable(name); ok && observed.Path != "" {
		return observed.Path
	}
	for _, directory := range filepath.SplitList(environment.PATH) {
		for _, suffix := range []string{"", ".exe"} {
			candidate := filepath.Join(directory, name+suffix)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate
			}
		}
	}
	return ""
}

func runForeground(ctx context.Context, p params, cwd, shell string, args []string, kind string, environment tool.HostEnvironment, observer tool.ToolObserver, state *tool.ToolSessionState) result {
	started := time.Now()
	timeoutContext, cancel := context.WithTimeout(ctx, time.Duration(p.Timeout)*time.Millisecond)
	defer cancel()
	marker, _ := os.CreateTemp("", "awaken-bash-cwd-*")
	markerPath := ""
	if marker != nil {
		markerPath = marker.Name()
		_ = marker.Close()
		_ = os.Remove(markerPath)
		defer os.Remove(markerPath)
	}
	wrapped := wrapCommand(p.Command, markerPath, kind)
	args = invocationArgs(shell, kind, wrapped)
	command := exec.CommandContext(timeoutContext, shell, args...)
	configureProcessTree(command)
	command.Dir = cwd
	command.Env = append(append([]string(nil), environment.Environment...), "AWAKEN_CWD_RESULT="+markerPath)
	stdout := newLimitedWriter(maxOutputBytes, observer, "stdout")
	stderr := newLimitedWriter(maxOutputBytes, observer, "stderr")
	command.Stdout, command.Stderr = stdout, stderr
	result := result{Command: p.Command, Shell: shell, CWD: cwd, EnvironmentSnapshotID: environment.Snapshot.SnapshotID}
	err := command.Run()
	result.DurationMS = time.Since(started).Milliseconds()
	result.Stdout, result.Stderr = stdout.String(), stderr.String()
	result.Truncated = stdout.Truncated() || stderr.Truncated()
	result.TimedOut = errors.Is(timeoutContext.Err(), context.DeadlineExceeded)
	result.Cancelled = errors.Is(timeoutContext.Err(), context.Canceled) && !result.TimedOut
	if command.ProcessState != nil {
		exitCode := command.ProcessState.ExitCode()
		result.ExitCode = &exitCode
	}
	if err == nil && result.ExitCode == nil {
		exitCode := 0
		result.ExitCode = &exitCode
	}
	if actualCWD := readCWDMarker(markerPath); actualCWD != "" {
		result.CWD = actualCWD
		if state != nil {
			_ = state.SetCWD(actualCWD)
		}
	}
	return result
}

func wrapCommand(command, markerPath, kind string) string {
	if markerPath == "" {
		return command
	}
	switch kind {
	case "powershell":
		return fmt.Sprintf("try { %s } finally { (Get-Location).Path | Set-Content -LiteralPath $env:AWAKEN_CWD_RESULT }", command)
	case "cmd":
		return fmt.Sprintf("%s & set AWAKEN_EXIT=!ERRORLEVEL! & cd > \"%%AWAKEN_CWD_RESULT%%\" & exit /b !AWAKEN_EXIT!", command)
	default:
		return "trap 'pwd > \"$AWAKEN_CWD_RESULT\"' EXIT\n" + command
	}
}

func readCWDMarker(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	cwd := strings.TrimSpace(string(data))
	if info, err := os.Stat(cwd); err == nil && info.IsDir() {
		return filepath.Clean(cwd)
	}
	return ""
}

func outcomeFromResult(execution result, started time.Time) tool.ExecutionOutcome {
	payload, _ := json.MarshalIndent(execution, "", "  ")
	finished := time.Now().UTC()
	if execution.TimedOut {
		outcome := tool.FailedOutcome(string(payload), tool.ErrorCodeProcessTimeout, tool.ErrorCategoryProcess, true)
		outcome.Content, outcome.ProcessStatus, outcome.TimedOut = string(payload), tool.ProcessTimedOut, true
		outcome.TransportStatus = tool.TransportCancelled
		outcome.Details = resultDetails(execution)
		return outcome.WithTiming(started, finished)
	}
	if execution.Cancelled {
		outcome := tool.FailedOutcome(string(payload), tool.ErrorCodeToolCancelled, tool.ErrorCategoryCancelled, true)
		outcome.Content, outcome.Status, outcome.ProcessStatus = string(payload), tool.OutcomeCancelled, tool.ProcessSignaled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.Details = resultDetails(execution)
		return outcome.WithTiming(started, finished)
	}
	if execution.ExitCode == nil {
		outcome := tool.FailedOutcome(string(payload), tool.ErrorCodeProcessSpawnFailed, tool.ErrorCategoryProcess, true)
		outcome.Content, outcome.ProcessStatus = string(payload), tool.ProcessSpawnFailed
		outcome.Details = resultDetails(execution)
		return outcome.WithTiming(started, finished)
	}
	if *execution.ExitCode != 0 {
		outcome := tool.FailedOutcome(string(payload), tool.ErrorCodeProcessExitNonzero, tool.ErrorCategoryProcess, false)
		outcome.Content, outcome.ProcessStatus, outcome.ExitCode = string(payload), tool.ProcessExited, execution.ExitCode
		outcome.TransportStatus = tool.TransportCompleted
		outcome.Details, outcome.Truncated = resultDetails(execution), execution.Truncated
		return outcome.WithTiming(started, finished)
	}
	outcome := tool.SucceededOutcome(string(payload))
	outcome.ProcessStatus, outcome.ExitCode, outcome.Truncated = tool.ProcessExited, execution.ExitCode, execution.Truncated
	outcome.Details = resultDetails(execution)
	outcome.Summary = fmt.Sprintf("Command completed in %s with exit code 0.", execution.CWD)
	return outcome.WithTiming(started, finished)
}

func resultDetails(execution result) map[string]any {
	return map[string]any{"shell": execution.Shell, "cwd": execution.CWD, "stdout": execution.Stdout, "stderr": execution.Stderr, "exit_code": execution.ExitCode, "timed_out": execution.TimedOut, "cancelled": execution.Cancelled, "truncated": execution.Truncated, "duration_ms": execution.DurationMS, "environment_snapshot_id": execution.EnvironmentSnapshotID}
}

type limitedWriter struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
	observer  tool.ToolObserver
	stream    string
	observeMu sync.Mutex
}

func newLimitedWriter(limit int, observer tool.ToolObserver, stream string) *limitedWriter {
	return &limitedWriter{limit: limit, observer: observer, stream: stream}
}
func (w *limitedWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		portion := value
		if len(portion) > remaining {
			portion = portion[:remaining]
			w.truncated = true
		}
		_, _ = w.buffer.Write(portion)
	} else if len(value) > 0 {
		w.truncated = true
	}
	w.mu.Unlock()
	if w.observer != nil && len(value) > 0 {
		w.observeMu.Lock()
		w.observer.OnCommandOutput(tool.CommandOutput{Stream: w.stream, Delta: string(value)})
		w.observeMu.Unlock()
	}
	return len(value), nil
}
func (w *limitedWriter) String() string  { w.mu.Lock(); defer w.mu.Unlock(); return w.buffer.String() }
func (w *limitedWriter) Truncated() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.truncated }

type backgroundStatus struct {
	TaskID     string     `json:"task_id"`
	ThreadID   string     `json:"thread_id"`
	State      string     `json:"state"`
	Command    string     `json:"command"`
	Shell      string     `json:"shell"`
	CWD        string     `json:"cwd"`
	PID        int        `json:"pid,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	TimedOut   bool       `json:"timed_out,omitempty"`
	Cancelled  bool       `json:"cancelled,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type backgroundTask struct {
	threadID string
	cancel   context.CancelFunc
}

var taskRegistry = struct {
	sync.Mutex
	tasks map[string]*backgroundTask
}{tasks: make(map[string]*backgroundTask)}

func startBackgroundTask(runContext context.Context, p params, cwd, shell string, args []string, kind string, environment tool.HostEnvironment, state *tool.ToolSessionState) (result, error) {
	threadID := tool.ToolThreadIDFromContext(runContext)
	identity, _ := tool.ToolExecutionIdentityFromContext(runContext)
	taskID := identity.ToolCallID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	taskRoot := backgroundTaskRoot(threadID, taskID)
	if err := os.MkdirAll(taskRoot, 0o700); err != nil {
		return result{}, err
	}
	logPath := filepath.Join(taskRoot, "combined.log")
	statusPath := filepath.Join(taskRoot, "status.json")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return result{}, err
	}
	markerPath := filepath.Join(taskRoot, "cwd.txt")
	wrapped := wrapCommand(p.Command, markerPath, kind)
	args = invocationArgs(shell, kind, wrapped)
	taskContext, cancel := context.WithTimeout(context.Background(), time.Duration(p.Timeout)*time.Millisecond)
	command := exec.CommandContext(taskContext, shell, args...)
	configureProcessTree(command)
	command.Dir = cwd
	command.Env = append(append([]string(nil), environment.Environment...), "AWAKEN_CWD_RESULT="+markerPath)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		cancel()
		_ = logFile.Close()
		return result{}, err
	}
	status := backgroundStatus{TaskID: taskID, ThreadID: threadID, State: "running", Command: p.Command, Shell: shell, CWD: cwd, PID: command.Process.Pid, StartedAt: time.Now().UTC()}
	if err := writeBackgroundStatus(statusPath, status); err != nil {
		cancel()
		_ = command.Wait()
		_ = logFile.Close()
		return result{}, err
	}
	task := &backgroundTask{threadID: threadID, cancel: cancel}
	taskRegistry.Lock()
	taskRegistry.tasks[taskID] = task
	taskRegistry.Unlock()
	go func() {
		err := command.Wait()
		_ = logFile.Sync()
		_ = logFile.Close()
		finished := time.Now().UTC()
		status.FinishedAt = &finished
		if command.ProcessState != nil {
			exit := command.ProcessState.ExitCode()
			status.ExitCode = &exit
		}
		status.TimedOut = errors.Is(taskContext.Err(), context.DeadlineExceeded)
		status.Cancelled = errors.Is(taskContext.Err(), context.Canceled) && !status.TimedOut
		switch {
		case status.TimedOut:
			status.State = "timed_out"
		case status.Cancelled:
			status.State = "cancelled"
		case err != nil:
			status.State = "failed"
		default:
			status.State = "completed"
		}
		if actualCWD := readCWDMarker(markerPath); actualCWD != "" {
			status.CWD = actualCWD
			if state != nil {
				_ = state.SetCWD(actualCWD)
			}
		}
		_ = writeBackgroundStatus(statusPath, status)
		taskRegistry.Lock()
		delete(taskRegistry.tasks, taskID)
		taskRegistry.Unlock()
	}()
	return result{Command: p.Command, Shell: shell, CWD: cwd, EnvironmentSnapshotID: environment.Snapshot.SnapshotID, TaskID: taskID, LogPath: logPath, StatusPath: statusPath}, nil
}

func backgroundTaskRoot(threadID, taskID string) string {
	cache, err := os.UserCacheDir()
	if err != nil || cache == "" {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "Awaken", "tasks", safeName(threadID), safeName(taskID))
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func writeBackgroundStatus(path string, status backgroundStatus) error {
	encoded, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return textfile.WriteAtomic(path, append(encoded, '\n'), 0o600)
}

func CancelThreadTasks(threadID string) {
	taskRegistry.Lock()
	defer taskRegistry.Unlock()
	for _, task := range taskRegistry.tasks {
		if task.threadID == threadID {
			task.cancel()
		}
	}
}

func CancelAllTasks() {
	taskRegistry.Lock()
	defer taskRegistry.Unlock()
	for _, task := range taskRegistry.tasks {
		task.cancel()
	}
}

var _ io.Writer = (*limitedWriter)(nil)

func init() { tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin) }
