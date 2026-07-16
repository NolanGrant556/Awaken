package bash

import (
	"context"
	"encoding/json"
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
