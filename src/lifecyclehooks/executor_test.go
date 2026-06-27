package lifecyclehooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutorRunsConfiguredHook(t *testing.T) {
	root := t.TempDir()
	outPath := filepath.Join(root, "hooks.log")
	script := writeHookScript(t, root, `#!/bin/sh
printf '%s:%s:%s:%s\n' "$1" "$AWAKEN_HOOK_EVENT" "$AWAKEN_RUN_ID" "$AWAKEN_WORKSPACE_ROOT" >> "$2"
`)

	executor := NewExecutorWithLogger(Config{Hooks: map[Event][]CommandConfig{
		EventBeforeRun: {{
			Command: script,
			Args:    []string{"mark", outPath},
		}},
	}}, discardLogger{})

	results := executor.Execute(context.Background(), EventBeforeRun, map[string]string{
		"run_id":         "run_123",
		"workspace_root": root,
	})

	if len(results) != 1 || results[0].ExitCode != 0 || results[0].TimedOut {
		t.Fatalf("unexpected hook results: %+v", results)
	}
	got := readFile(t, outPath)
	want := "mark:before_run:run_123:" + root + "\n"
	if got != want {
		t.Fatalf("unexpected hook output file:\n got %q\nwant %q", got, want)
	}
}

func TestExecutorRecordsTimeoutAndError(t *testing.T) {
	var logs bufferLogger
	executor := NewExecutorWithLogger(Config{Hooks: map[Event][]CommandConfig{
		EventBeforeRun: {
			{Command: "/bin/sh", Args: []string{"-c", "sleep 1"}, TimeoutMS: 10},
			{Command: "/bin/sh", Args: []string{"-c", "exit 7"}, TimeoutMS: 1000},
		},
	}}, &logs)

	results := executor.Execute(context.Background(), EventBeforeRun, nil)

	if len(results) != 2 {
		t.Fatalf("expected two hook results, got %+v", results)
	}
	if !results[0].TimedOut || results[0].ExitCode != -1 {
		t.Fatalf("expected timeout result, got %+v", results[0])
	}
	if results[1].ExitCode != 7 {
		t.Fatalf("expected non-zero exit result, got %+v", results[1])
	}
	gotLogs := logs.String()
	if !strings.Contains(gotLogs, "lifecycle hook failed") || !strings.Contains(gotLogs, "timed_out=true") || !strings.Contains(gotLogs, "exit_code=7") {
		t.Fatalf("expected timeout and exit failure logs, got %q", gotLogs)
	}
}

func TestExecutorTruncatesHookOutput(t *testing.T) {
	var logs bufferLogger
	executor := NewExecutorWithLogger(Config{Hooks: map[Event][]CommandConfig{
		EventAfterRun: {{
			Command: "/bin/sh",
			Args:    []string{"-c", `i=0; while [ "$i" -lt 13000 ]; do printf x; i=$((i+1)); done`},
		}},
	}}, &logs)

	results := executor.Execute(context.Background(), EventAfterRun, nil)

	if len(results) != 1 {
		t.Fatalf("expected one hook result, got %+v", results)
	}
	if !results[0].Truncated || !strings.Contains(results[0].Stdout, "truncated") {
		t.Fatalf("expected truncated stdout, got %+v", results[0])
	}
	if !strings.Contains(logs.String(), "lifecycle hook output truncated") {
		t.Fatalf("expected truncation log, got %q", logs.String())
	}
}

func TestExecutorRedactsSecretsFromCommandArgsInLogs(t *testing.T) {
	var logs bufferLogger
	executor := NewExecutorWithLogger(Config{Hooks: map[Event][]CommandConfig{
		EventBeforeTool: {{
			Command: "/bin/sh",
			Args:    []string{"-c", "exit 7", "token=sk-test-secret"},
		}},
	}}, &logs)

	results := executor.Execute(context.Background(), EventBeforeTool, nil)

	if len(results) != 1 || results[0].ExitCode != 7 {
		t.Fatalf("expected failed hook result, got %+v", results)
	}
	gotLogs := logs.String()
	if strings.Contains(gotLogs, "sk-test-secret") {
		t.Fatalf("expected secret to be redacted from logs, got %q", gotLogs)
	}
	if !strings.Contains(gotLogs, "token=[REDACTED]") {
		t.Fatalf("expected redacted token marker, got %q", gotLogs)
	}
}

type discardLogger struct{}

func (discardLogger) Printf(string, ...interface{}) {}

type bufferLogger struct {
	bytes.Buffer
}

func (l *bufferLogger) Printf(format string, values ...interface{}) {
	fmt.Fprintf(&l.Buffer, format+"\n", values...)
}

func writeHookScript(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}
