package bash

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"
)

func bashContext(root, threadID, callID string) context.Context {
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	ctx = tool.ContextWithToolSessionState(ctx, tool.NewToolSessionState(root))
	ctx = tool.ContextWithToolThreadID(ctx, threadID)
	return tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{ToolCallID: callID, ToolName: ToolName})
}

func TestBashThreadCWDAndTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fixture")
	}
	root := t.TempDir()
	ctx := bashContext(root, "thread-cwd", "call-cwd")
	first, err := (&Tool{}).ExecuteOutcome(ctx, `{"command":"mkdir sub && cd sub && pwd","timeout":30000}`, nil)
	if err != nil || first.IsError() {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, _ := (&Tool{}).ExecuteOutcome(ctx, `{"command":"pwd","timeout":30000}`, nil)
	stdout, _ := second.Details["stdout"].(string)
	wantCWD, _ := filepath.EvalSymlinks(filepath.Join(root, "sub"))
	if second.IsError() || strings.TrimSpace(stdout) != wantCWD {
		t.Fatalf("second=%+v", second)
	}
	timedOut, _ := (&Tool{}).ExecuteOutcome(ctx, `{"command":"sleep 1","timeout":10}`, nil)
	if timedOut.ErrorCode != tool.ErrorCodeProcessTimeout || !timedOut.TimedOut {
		t.Fatalf("timeout=%+v", timedOut)
	}
}

func TestBashBackgroundTaskCanBeCancelledByThread(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fixture")
	}
	ctx := bashContext(t.TempDir(), "thread-background", "task-background")
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"command":"sleep 30","timeout":60000,"run_in_background":true}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("start=%+v err=%v", outcome, err)
	}
	statusPath, _ := outcome.Details["status_path"].(string)
	logPath, _ := outcome.Details["log_path"].(string)
	if statusPath == "" || logPath == "" {
		t.Fatalf("paths=%+v", outcome.Details)
	}
	CancelThreadTasks("thread-background")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(statusPath)
		var status backgroundStatus
		if readErr == nil && json.Unmarshal(data, &status) == nil && status.State == "cancelled" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(statusPath)
	t.Fatalf("background task did not become cancelled: %s", data)
}

func TestBashBackgroundTaskSurvivesNormalRunContextEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fixture")
	}
	ctx, finishRun := context.WithCancel(bashContext(t.TempDir(), "thread-survives-run", "task-survives-run"))
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"command":"sleep 0.1; echo background-ok","timeout":5000,"run_in_background":true}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("start=%+v err=%v", outcome, err)
	}
	statusPath, _ := outcome.Details["status_path"].(string)
	logPath, _ := outcome.Details["log_path"].(string)
	finishRun()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(statusPath)
		var status backgroundStatus
		if readErr == nil && json.Unmarshal(data, &status) == nil && status.State == "completed" {
			logData, logErr := os.ReadFile(logPath)
			if logErr != nil || !strings.Contains(string(logData), "background-ok") {
				t.Fatalf("background log=%q err=%v", logData, logErr)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, _ := os.ReadFile(statusPath)
	t.Fatalf("background task did not survive normal run completion: %s", data)
}

func TestWindowsShellSelectionPrefersDerivedGitBashAndRejectsSystemLauncher(t *testing.T) {
	root := t.TempDir()
	gitPath := writeExecutableFixture(t, filepath.Join(root, "Git", "cmd", "git.exe"))
	bashPath := writeExecutableFixture(t, filepath.Join(root, "Git", "bin", "bash.exe"))
	pwshPath := writeExecutableFixture(t, filepath.Join(root, "PowerShell", "pwsh.exe"))
	environment := tool.HostEnvironment{
		Shell: `C:\Windows\System32\bash.exe`,
		Snapshot: tool.ExecutionEnvironmentSnapshot{Executables: []tool.ExecutableObservation{
			{Name: "bash", Path: `C:\Windows\System32\bash.exe`},
			{Name: "git", Path: gitPath},
			{Name: "pwsh", Path: pwshPath},
		}},
	}
	var probed []string
	shell, args, kind, err := selectWindowsShell(context.Background(), environment, "echo ready", func(_ context.Context, path, _ string, _ tool.HostEnvironment) error {
		probed = append(probed, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if shell != bashPath || kind != "posix" || len(probed) != 1 || probed[0] != bashPath {
		t.Fatalf("shell=%q kind=%q probed=%v", shell, kind, probed)
	}
	if strings.Join(args, " ") != "--noprofile --norc -c echo ready" {
		t.Fatalf("args=%v", args)
	}
}

func TestWindowsShellSelectionFallsBackAfterHealthCheckFailure(t *testing.T) {
	root := t.TempDir()
	gitPath := writeExecutableFixture(t, filepath.Join(root, "Git", "cmd", "git.exe"))
	bashPath := writeExecutableFixture(t, filepath.Join(root, "Git", "bin", "bash.exe"))
	pwshPath := writeExecutableFixture(t, filepath.Join(root, "PowerShell", "pwsh.exe"))
	environment := tool.HostEnvironment{Snapshot: tool.ExecutionEnvironmentSnapshot{Executables: []tool.ExecutableObservation{
		{Name: "git", Path: gitPath},
		{Name: "pwsh", Path: pwshPath},
	}}}
	shell, args, kind, err := selectWindowsShell(context.Background(), environment, "Get-Location", func(_ context.Context, path, _ string, _ tool.HostEnvironment) error {
		if path == bashPath {
			return errors.New("unhealthy")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if shell != pwshPath || kind != "powershell" {
		t.Fatalf("shell=%q kind=%q", shell, kind)
	}
	if strings.Join(args, " ") != "-NoLogo -NoProfile -NonInteractive -Command Get-Location" {
		t.Fatalf("args=%v", args)
	}
}

func TestWindowsShellSelectionUsesCOMSPECWhenOtherShellsAreUnavailable(t *testing.T) {
	cmdPath := writeExecutableFixture(t, filepath.Join(t.TempDir(), "cmd.exe"))
	environment := tool.HostEnvironment{Environment: []string{"COMSPEC=" + cmdPath}}
	shell, args, kind, err := selectWindowsShell(context.Background(), environment, "echo ready", func(_ context.Context, _ string, _ string, _ tool.HostEnvironment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if shell != cmdPath || kind != "cmd" {
		t.Fatalf("shell=%q kind=%q", shell, kind)
	}
	if strings.Join(args, " ") != "/D /V:ON /S /C echo ready" {
		t.Fatalf("args=%v", args)
	}
}

func TestWindowsGitBashRecognitionRejectsSystemLauncher(t *testing.T) {
	if looksLikeGitBash(`C:\Windows\System32\bash.exe`) {
		t.Fatal("system launcher must not be accepted as Git Bash")
	}
	if !looksLikeGitBash(`D:\Tools\GitForWindows\bin\bash.exe`) {
		t.Fatal("recognized Git Bash path was rejected")
	}
}

func TestCancelledShellHealthProbeIsNotCached(t *testing.T) {
	shellHealthCache.Lock()
	shellHealthCache.results = make(map[string]string)
	shellHealthCache.order = nil
	shellHealthCache.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	environment := tool.HostEnvironment{Snapshot: tool.ExecutionEnvironmentSnapshot{SnapshotID: "cancelled-probe"}}
	err := cachedShellHealthProbe(ctx, writeExecutableFixture(t, filepath.Join(t.TempDir(), "bash.exe")), "posix", environment)
	if err == nil {
		t.Fatal("cancelled probe unexpectedly succeeded")
	}
	shellHealthCache.Lock()
	defer shellHealthCache.Unlock()
	if len(shellHealthCache.results) != 0 {
		t.Fatalf("cancelled probe was cached: %v", shellHealthCache.results)
	}
}

func writeExecutableFixture(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSubagentCleanupDrainsOnlyOwnedBackgroundCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fixture")
	}
	var paths []string
	for _, id := range []string{"child-a", "child-b"} {
		ctx := bashContext(t.TempDir(), "thread-child-cleanup", id)
		ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run", SubagentTaskID: id, ToolCallID: id, ToolName: ToolName})
		outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"command":"sleep 30","timeout":60000,"run_in_background":true}`, nil)
		if err != nil || outcome.IsError() {
			t.Fatalf("%+v %v", outcome, err)
		}
		paths = append(paths, outcome.Details["status_path"].(string))
	}
	defer FinishSubagentTasks("child-b")
	FinishSubagentTasks("child-a")
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var status backgroundStatus
		if err := json.Unmarshal(data, &status); err != nil {
			t.Fatal(err)
		}
		want := "cancelled"
		if i == 1 {
			want = "running"
		}
		if status.State != want {
			t.Fatalf("%s: got %s want %s", path, status.State, want)
		}
	}
}

func TestDeletedTemporaryCWDNeedsExplicitRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	root := t.TempDir()
	ctx := bashContext(root, "cwd-recovery", "call-recovery")
	tmp := filepath.Join(root, "temporary")
	if err := os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	state, _ := tool.ToolSessionStateFromContext(ctx)
	if err := state.SetCWD(tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tmp); err != nil {
		t.Fatal(err)
	}
	first, err := (&Tool{}).ExecuteOutcome(ctx, `{"command":"touch should-not-exist"}`, nil)
	if err != nil || first.ErrorCode != "bash_cwd_unavailable" || len(first.NextActions) == 0 {
		t.Fatalf("missing recovery: %+v %v", first, err)
	}
	if _, err = os.Stat(filepath.Join(root, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatal("silently executed in another directory")
	}
	raw, _ := json.Marshal(map[string]any{"command": "pwd", "cwd": root})
	second, err := (&Tool{}).ExecuteOutcome(ctx, string(raw), nil)
	if err != nil || second.IsError() {
		t.Fatalf("explicit recovery failed: %+v %v", second, err)
	}
	actual, _ := second.Details["stdout"].(string)
	want, _ := filepath.EvalSymlinks(root)
	if strings.TrimSpace(actual) != want {
		t.Fatalf("wrong cwd %q", actual)
	}
}
