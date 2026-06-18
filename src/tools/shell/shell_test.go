package shelltool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestShellPermissionAsksForComplexOrDangerousCommands(t *testing.T) {
	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"printf hello > out.txt"}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "warning" {
		t.Fatalf("expected complex command to ask with warning risk, got %+v", decision)
	}

	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"rm -rf build"}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "danger" {
		t.Fatalf("expected dangerous command to ask with danger risk, got %+v", decision)
	}
}

func TestShellPermissionUsesWorkspaceRootForWorkdir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	permissionContext := tool.NewPermissionContext(root, nil)

	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"ls","workdir":"`+outside+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionAsk || decision.ApprovalRequest.RiskTone != "danger" {
		t.Fatalf("expected outside workdir to ask with danger risk, got %+v", decision)
	}

	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"ls","workdir":"`+root+`"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected inside workspace workdir to be allowed for read-only command, got %+v", decision)
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

func TestShellPermissionAsksWhenReadOnlyCommandAccessesExternalPath(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, nil)
	shellTool := &ShellTool{}

	cases := []string{
		`{"command":"cat /etc/hosts"}`,
		`{"command":"head -1 /etc/passwd"}`,
		`{"command":"tail -n 5 /var/log/system.log"}`,
		`{"command":"ls /tmp"}`,
		`{"command":"grep root /etc/passwd"}`,
		`{"command":"cat README.md /etc/hosts"}`,
	}
	for _, input := range cases {
		decision := shellTool.CheckPermission(context.Background(), input, ctx)
		if decision.Behavior != tool.PermissionAsk {
			t.Fatalf("expected ask for %q, got %+v", input, decision)
		}
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

func TestShellPermissionAsksForSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	ctx := tool.NewPermissionContext(root, nil)
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"cat link"}`, ctx)
	if decision.Behavior != tool.PermissionAsk {
		t.Fatalf("expected ask for symlink escape, got %+v", decision)
	}
}

func TestShellPermissionControlOperatorsStillAsk(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, nil)
	shellTool := &ShellTool{}

	cases := []string{
		`{"command":"cat README.md | head"}`,
		`{"command":"cat README.md > out.txt"}`,
		`{"command":"cat README.md && echo done"}`,
	}
	for _, input := range cases {
		decision := shellTool.CheckPermission(context.Background(), input, ctx)
		if decision.Behavior != tool.PermissionAsk {
			t.Fatalf("expected ask for control operator in %q, got %+v", input, decision)
		}
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
