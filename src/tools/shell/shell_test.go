package shelltool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"awaken/src/tool"
)

func TestShellSuccess(t *testing.T) {
	out, err := (&ShellTool{}).Execute(context.Background(), `{"command":"printf hello"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	result := decodeResult(t, out)
	if result.ExitCode != 0 || result.Stdout != "hello" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestShellNonZeroExitIsReturned(t *testing.T) {
	out, err := (&ShellTool{}).Execute(context.Background(), `{"command":"printf nope >&2; exit 7"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	result := decodeResult(t, out)
	if result.ExitCode != 7 || result.Stderr != "nope" {
		t.Fatalf("unexpected nonzero result: %+v", result)
	}
}

func TestShellTimeout(t *testing.T) {
	out, err := (&ShellTool{}).Execute(context.Background(), `{"command":"sleep 1","timeout_ms":10}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	result := decodeResult(t, out)
	if !result.TimedOut || result.ExitCode != -1 {
		t.Fatalf("expected timeout result, got %+v", result)
	}
}

func TestShellOutputTruncation(t *testing.T) {
	out, err := (&ShellTool{}).Execute(context.Background(), `{"command":"printf 123456789","max_output_chars":4}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	result := decodeResult(t, out)
	if !result.Truncated || !strings.Contains(result.Stdout, "truncated") {
		t.Fatalf("expected truncated output, got %+v", result)
	}
}

func TestShellExecuteWithObserverStreamsOutput(t *testing.T) {
	var mu sync.Mutex
	var outputs []tool.CommandOutput
	observer := shellObserverFunc(func(output tool.CommandOutput) {
		mu.Lock()
		defer mu.Unlock()
		outputs = append(outputs, output)
	})

	out, err := (&ShellTool{}).ExecuteWithObserver(context.Background(), `{"command":"printf hello; printf err >&2"}`, observer)
	if err != nil {
		t.Fatalf("ExecuteWithObserver returned error: %v", err)
	}
	result := decodeResult(t, out)
	if result.Stdout != "hello" || result.Stderr != "err" {
		t.Fatalf("unexpected result: %+v", result)
	}

	joined := map[string]string{}
	for _, output := range outputs {
		joined[output.Stream] += output.Delta
	}
	if joined["stdout"] != "hello" || joined["stderr"] != "err" {
		t.Fatalf("unexpected streamed output: %+v", outputs)
	}
}

func TestShellExecuteWithObserverCapsOutputDelta(t *testing.T) {
	var mu sync.Mutex
	var outputs []tool.CommandOutput
	observer := shellObserverFunc(func(output tool.CommandOutput) {
		mu.Lock()
		defer mu.Unlock()
		outputs = append(outputs, output)
	})

	_, err := (&ShellTool{}).ExecuteWithObserver(context.Background(), `{"command":"printf 123456789","max_output_chars":4}`, observer)
	if err != nil {
		t.Fatalf("ExecuteWithObserver returned error: %v", err)
	}

	var stdout string
	var sawTruncated bool
	for _, output := range outputs {
		if output.Stream != "stdout" {
			continue
		}
		stdout += output.Delta
		sawTruncated = sawTruncated || output.Truncated
	}
	if !sawTruncated || !strings.Contains(stdout, "output truncated") {
		t.Fatalf("expected truncated stdout delta, got outputs=%+v joined=%q", outputs, stdout)
	}
	if strings.Contains(stdout, "123456789") {
		t.Fatalf("expected streamed delta to be capped, got %q", stdout)
	}
}

func TestShellPermissionAllowsSimpleReadOnlyCommands(t *testing.T) {
	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"ls -la"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected read-only command to be allowed, got %+v", decision)
	}

	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"git status --short"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected read-only git command to be allowed, got %+v", decision)
	}
}

func TestShellCheckPermission(t *testing.T) {
	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)

	// inspect_read_only: ls → Allow
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"ls"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected inspect_read_only command (ls) to be allowed, got %+v", decision)
	}

	// workspace_mutation: echo "hello" > file.txt → Ask with RiskTone="warning"
	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"echo \"hello\" > file.txt"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone != "warning" {
		t.Fatalf("expected workspace_mutation command to ask with warning risk, got %+v", decision)
	}

	// open_world_system: curl https://example.com → Deny. Network/dependency
	// commands need a project profile or typed executor; they no longer become
	// a generic approval-style warning.
	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"curl https://example.com"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected open_world_system command to be denied, got %+v", decision)
	}
}

func TestShellPermissionAllowsForComplexOrDangerousCommands(t *testing.T) {
	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)

	// Redirect to workspace path → workspace_mutation → Ask warning.
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"printf hello > out.txt"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone != "warning" {
		t.Fatalf("expected redirect command to ask with warning risk, got %+v", decision)
	}

	// rm is denied at shell level because deletion must go through TrashAdapter.
	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"rm -rf build"}`, permissionContext)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected shell delete command to be denied, got %+v", decision)
	}
}

func TestShellPermissionAllowsSafeLanes(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	shellTool := &ShellTool{}

	cases := []struct {
		name string
		cmd  string
		lane string
	}{
		{"mkdir", "mkdir -p reports", "workspace_fs"},
		{"touch", "touch reports.md", "workspace_fs"},
		{"go test", "go test ./...", "test_build_codegen"},
		{"npm build", "npm run build", "test_build_codegen"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := shellTool.CheckPermission(context.Background(), `{"command":`+quoteJSON(tc.cmd)+`}`, ctx)
			if decision.Behavior != tool.PermissionAllow {
				t.Fatalf("expected allow for %q, got %+v", tc.cmd, decision)
			}
			if !strings.Contains(decision.Metadata.Title, tc.lane) {
				t.Fatalf("expected title to include lane %q, got %+v", tc.lane, decision.Metadata)
			}
		})
	}
}

func TestShellPermissionAllowsNullDeviceRedirectForReadOnlyProbe(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"command -v pdftotext >/dev/null 2>&1"}`, ctx)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected allow for /dev/null redirect probe, got %+v", decision)
	}
}

func TestShellPermissionDeniesSensitiveReadAndPipeToExecutor(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	shellTool := &ShellTool{}

	cases := []string{
		"cat ~/.ssh/id_rsa",
		"find / -type f",
		"cat README.md | sh",
		"touch -m file.txt",
		"command -v pdftotext >/dev/nullo 2>&1",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			decision := shellTool.CheckPermission(context.Background(), `{"command":`+quoteJSON(cmd)+`}`, ctx)
			if decision.Behavior != tool.PermissionDeny {
				t.Fatalf("expected deny for %q, got %+v", cmd, decision)
			}
		})
	}
}

func TestShellExecutesFromWorkspaceRootByDefault(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))

	out, err := (&ShellTool{}).Execute(ctx, `{"command":"pwd"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	result := decodeResult(t, out)
	if strings.TrimSpace(result.Stdout) != root {
		t.Fatalf("expected pwd to run from workspace root %q, got %+v", root, result)
	}
}

func TestShellPermissionAllowsReadOnlyCommandInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	ctx := tool.NewPermissionContext(root, nil)
	shellTool := &ShellTool{}

	cases := []string{
		`{"command":"cat README.md"}`,
		`{"command":"ls ./src"}`,
		`{"command":"grep hello README.md"}`,
		`{"command":"head -n 1 README.md"}`,
		`{"command":"wc -l README.md"}`,
	}
	for _, input := range cases {
		decision := shellTool.CheckPermission(context.Background(), input, ctx)
		if decision.Behavior != tool.PermissionAllow {
			t.Fatalf("expected allow for %q, got %+v", input, decision)
		}
	}
}

func TestShellPermissionControlOperatorsStillAsk(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, nil)
	shellTool := &ShellTool{}

	// Redirect to a workspace path raises the tier to workspace_mutation.
	// Pipe and && of read-only commands stay inspect_read_only under the
	// new classifier, so only the redirect case remains an Ask.
	decision := shellTool.CheckPermission(context.Background(), `{"command":"cat README.md > out.txt"}`, ctx)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone != "warning" {
		t.Fatalf("expected redirect command to ask with warning risk, got %+v", decision)
	}
}

func decodeResult(t *testing.T, out string) shellResult {
	t.Helper()
	var result shellResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to decode shell result %q: %v", out, err)
	}
	return result
}

func quoteJSON(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

type shellObserverFunc func(tool.CommandOutput)

func (fn shellObserverFunc) OnCommandOutput(output tool.CommandOutput) {
	fn(output)
}
