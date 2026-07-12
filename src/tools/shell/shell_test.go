package shelltool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"awaken/src/tool"
	"awaken/src/tool/sandbox"
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

func TestShellDirectNonzeroOutcomePreservesProcessAndUnknownSideEffects(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, nil))
	outcome, err := (&ShellTool{}).ExecuteOutcome(
		ctx, `{"command":"printf partial > direct-partial.txt; exit 7"}`, nil,
	)
	if err != nil {
		t.Fatalf("ExecuteOutcome transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ProcessStatus != tool.ProcessExited || outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero || outcome.ExitCode == nil || *outcome.ExitCode != 7 {
		t.Fatalf("real process failure facts were lost: %+v", outcome)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown || len(outcome.Changes) != 0 {
		t.Fatalf("direct provider fabricated side-effect certainty: operation=%+v changes=%+v", outcome.Operation, outcome.Changes)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, "direct-partial.txt")); readErr != nil || string(content) != "partial" {
		t.Fatalf("real partial side effect was lost: content=%q err=%v", content, readErr)
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

	// workspace_mutation: echo "hello" > file.txt → Allow with RiskTone="warning"
	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"echo \"hello\" > file.txt"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone != "warning" {
		t.Fatalf("expected workspace_mutation command to be allowed with warning risk, got %+v", decision)
	}

	// open_world_system: curl https://example.com → Allow. Shell is a
	// local-computer surface; network/dependency/system commands are not
	// constrained to the project workspace.
	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"curl https://example.com"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("expected open_world_system command to be allowed, got %+v", decision)
	}
}

func TestShellPermissionAllowsForComplexOrDangerousCommands(t *testing.T) {
	permissionContext := tool.NewPermissionContext(t.TempDir(), nil)

	// Redirect to workspace path → workspace_mutation → Allow warning.
	decision := (&ShellTool{}).CheckPermission(context.Background(), `{"command":"printf hello > out.txt"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone != "warning" {
		t.Fatalf("expected redirect command to be allowed with warning risk, got %+v", decision)
	}

	// A direct rm is allowed by the current-user shell; no Trash guarantee is
	// fabricated at permission time.
	decision = (&ShellTool{}).CheckPermission(context.Background(), `{"command":"rm -rf build"}`, permissionContext)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone == "recoverable_delete_transform" || len(decision.Metadata.AffectedPaths) != 0 {
		t.Fatalf("expected direct current-user execution without Trash claim, got %+v", decision)
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
		{"mkdir", "mkdir -p reports", "local_shell"},
		{"touch", "touch reports.md", "local_shell"},
		{"go test", "go test ./...", "local_shell"},
		{"npm build", "npm run build", "local_shell"},
		{"dws search", `dws aitable base search --query "DVF" --format json`, "local_shell"},
		{"gh issue list", "gh issue list --limit 10", "local_shell"},
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

func TestShellPermissionDoesNotClaimUniversalMassMessageGate(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	shellTool := &ShellTool{}

	cases := []string{
		`feishu message send --chat-id all --text hello`,
		`lark-cli im +messages-send --chat-id oc_xxx --text hello`,
		`dws chat robot send --chat-id group --text hello`,
		`dws message broadcast --text hello`,
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			decision := shellTool.CheckPermission(context.Background(), `{"command":`+quoteJSON(cmd)+`}`, ctx)
			if decision.Behavior != tool.PermissionAllow || decision.Metadata.MassMessage != nil {
				t.Fatalf("default host shell must not pretend it can universally intercept %q: %+v", cmd, decision)
			}
		})
	}
}

func TestShellExecutionPolicyAllowsCollaborationCLI(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	policy := EvaluateShellExecutionPolicy(`feishu message send --chat-id all --text hello`, root, ctx)
	if policy.DenyReason != "" || !policy.DirectAllow {
		t.Fatalf("expected current-user execution policy to allow collaboration CLI, got %+v", policy)
	}
}

func TestShellPolicyHasNoOfficeExecutableNameDeny(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	commands := []string{
		`soffice --headless --convert-to pdf deck.pptx`,
		`libreoffice --headless --convert-to pdf deck.pptx`,
		`"/Applications/LibreOffice.app/Contents/MacOS/soffice" --headless deck.pptx`,
		`'C:\\Program Files\\LibreOffice\\program\\soffice.exe' --headless deck.pptx`,
		`env SAL_USE_VCLPLUGIN=svp soffice --headless deck.pptx`,
		`/usr/bin/env -i HOME=/tmp /opt/homebrew/bin/soffice --headless deck.pptx`,
		`env -S 'soffice --headless --convert-to pdf' deck.pptx`,
		`env env env env env env env soffice --headless deck.pptx`,
		`command -- soffice --headless deck.pptx`,
		`command -p libreoffice --headless deck.pptx`,
		`printf '%s\\n' deck.pptx | xargs -n 1 soffice --headless`,
		`xargs -I {} /opt/homebrew/bin/soffice --headless {}`,
		`xargs -J % /opt/homebrew/bin/soffice --headless %`,
		`xargs env SAL_USE_VCLPLUGIN=svp soffice --headless`,
		`sh -c 'soffice --headless --convert-to pdf deck.pptx'`,
		`nohup /opt/homebrew/bin/soffice --headless deck.pptx`,
		`exec libreoffice --headless deck.pptx`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			for _, policy := range []ShellPolicyResult{
				EvaluateShellPolicy(command, root, ctx),
				EvaluateShellExecutionPolicy(command, root, ctx),
			} {
				if strings.Contains(policy.DenyReason, "OfficeRenderWorker") {
					t.Fatalf("Office executable names must not be a policy hard deny for %q: %+v", command, policy)
				}
			}
			decision := (&ShellTool{}).CheckPermission(
				context.Background(),
				`{"command":`+quoteJSON(command)+`}`,
				ctx,
			)
			if decision.Behavior != tool.PermissionAllow {
				t.Fatalf("expected Office command to reach the broker for %q, got %+v", command, decision)
			}
		})
	}
}

func TestShellPolicyAllowsOfficeRuntimeDiagnosticsWithoutLaunching(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	commands := []string{
		`command -v soffice`,
		`command -V libreoffice`,
		`rg -n soffice src`,
		`echo soffice`,
		`printf '%s\\n' soffice | xargs echo`,
		`plutil -extract LSBackgroundOnly raw -o - LibreOffice.app/Contents/Info.plist`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			policy := EvaluateShellExecutionPolicy(command, root, ctx)
			if policy.DenyReason != "" {
				t.Fatalf("expected diagnostic command to remain allowed for %q, got %+v", command, policy)
			}
		})
	}
}

func TestShellExecutorRunsOfficeNamedCLIThroughDefaultProvider(t *testing.T) {
	sandbox.SetAdapter(degradedSandboxMock{})
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	root := t.TempDir()
	path := filepath.Join(root, "soffice")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf office-direct\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := NewShellExecutor().Execute(
		context.Background(),
		shellParams{Command: "./soffice --headless --convert-to pdf deck.pptx"},
		tool.NewPermissionContext(root, []string{root}),
		nil,
	)
	if err != nil || result.ExitCode != 0 || result.Stdout != "office-direct" || result.BrokerReceipt.BrokerID != "local_user_direct_v1" {
		t.Fatalf("expected direct Office CLI execution, result=%+v err=%v", result, err)
	}
}

func TestShellExecutorCanRunOfficeNamedCLIThroughSupervisingBroker(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "soffice")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf office-ok\n"), 0o755); err != nil {
		t.Fatalf("write fake Office CLI: %v", err)
	}
	broker := &allowExecutionBroker{}
	ctx := tool.ContextWithExecutionBroker(context.Background(), broker)
	result, err := NewShellExecutor().Execute(
		ctx,
		shellParams{Command: "./soffice --headless --convert-to pdf deck.pptx"},
		tool.NewPermissionContext(root, []string{root}),
		nil,
	)
	if err != nil {
		t.Fatalf("expected supervising broker to authorize Office-named CLI: %v", err)
	}
	if result.Stdout != "office-ok" || result.BrokerReceipt.BrokerID != broker.ID() {
		t.Fatalf("unexpected execution facts: %+v", result)
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

func TestShellPermissionAllowsLocalComputerCommands(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	shellTool := &ShellTool{}

	cases := []string{
		"cat ~/.ssh/id_rsa",
		"find / -type f",
		"cat README.md | sh",
		"touch -m file.txt",
		"command -v pdftotext >/dev/nullo 2>&1",
		"sudo true",
		"chmod -R 755 /tmp/example",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			decision := shellTool.CheckPermission(context.Background(), `{"command":`+quoteJSON(cmd)+`}`, ctx)
			if decision.Behavior != tool.PermissionAllow {
				t.Fatalf("expected allow for %q, got %+v", cmd, decision)
			}
		})
	}
}

func TestShellPermissionAllowsDirectDeleteCommandsWithoutTrashClaim(t *testing.T) {
	root := t.TempDir()
	ctx := tool.NewPermissionContext(root, []string{root})
	shellTool := &ShellTool{}

	cases := []string{
		"rm -rf build",
		"rmdir build",
		"unlink file.txt",
		"del file.txt",
		"Remove-Item file.txt",
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			decision := shellTool.CheckPermission(context.Background(), `{"command":`+quoteJSON(cmd)+`}`, ctx)
			if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone == "recoverable_delete_transform" || len(decision.Metadata.AffectedPaths) != 0 {
				t.Fatalf("expected current-user direct delete for %q without recoverability claim, got %+v", cmd, decision)
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
	expectedRoot := resolveTestPath(t, root)
	gotRoot := resolveTestPath(t, strings.TrimSpace(result.Stdout))
	if gotRoot != expectedRoot {
		t.Fatalf("expected pwd to run from workspace root %q, got %+v", expectedRoot, result)
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

	// Redirect writes are allowed as local shell commands; they should still
	// carry warning metadata for observability, not trigger approval.
	decision := shellTool.CheckPermission(context.Background(), `{"command":"cat README.md > out.txt"}`, ctx)
	if decision.Behavior != tool.PermissionAllow || decision.Metadata.RiskTone != "warning" {
		t.Fatalf("expected redirect command to be allowed with warning risk, got %+v", decision)
	}
}

func TestShellOutcomeExit7IsFailed(t *testing.T) {
	outcome, err := (&ShellTool{}).ExecuteOutcome(context.Background(), `{"command":"exit 7"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed {
		t.Fatalf("status=%q", outcome.Status)
	}
	if outcome.ProcessStatus != tool.ProcessExited || outcome.ExitCode == nil || *outcome.ExitCode != 7 {
		t.Fatalf("process fields: %+v", outcome)
	}
	if outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero {
		t.Fatalf("error_code=%q", outcome.ErrorCode)
	}
	if outcome.Retryable {
		t.Fatalf("nonzero exit must require inspecting/changing the command, not blind retry: %+v", outcome)
	}
	if !outcome.IsError() {
		t.Fatal("expected IsError")
	}
}

func TestShellOutcomeGrepNoMatchIsNeutral(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	args, err := json.Marshal(map[string]any{
		"command": "grep definitely-missing-token-xyz " + target,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outcome, err := (&ShellTool{}).ExecuteOutcome(context.Background(), string(args), nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.SemanticStatus != tool.SemanticNeutral {
		t.Fatalf("expected neutral success, got status=%q semantic=%q err=%q content=%s", outcome.Status, outcome.SemanticStatus, outcome.ErrorCode, outcome.Content)
	}
	if reason, _ := outcome.Diagnostic["reason"].(string); reason != "no_match" {
		t.Fatalf("diagnostic=%v", outcome.Diagnostic)
	}
}

func TestShellOutcomeRgNoMatchIsNeutral(t *testing.T) {
	binDir := t.TempDir()
	writeExitOneCommand(t, binDir, "rg")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	outcome, err := (&ShellTool{}).ExecuteOutcome(context.Background(), `{"command":"rg definitely-missing-token sample.txt"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.TransportStatus != tool.TransportCompleted || outcome.SemanticStatus != tool.SemanticNeutral || outcome.ProcessStatus != tool.ProcessExited {
		t.Fatalf("expected neutral completed process, got %+v", outcome)
	}
	if outcome.ExitCode == nil || *outcome.ExitCode != 1 {
		t.Fatalf("exit code=%v", outcome.ExitCode)
	}
	if reason, _ := outcome.Diagnostic["reason"].(string); reason != "no_match" {
		t.Fatalf("diagnostic=%v", outcome.Diagnostic)
	}
}

func TestShellOutcomeDiffIsNeutralDifferent(t *testing.T) {
	binDir := t.TempDir()
	writeExitOneCommand(t, binDir, "diff")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	outcome, err := (&ShellTool{}).ExecuteOutcome(context.Background(), `{"command":"diff before.txt after.txt"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeSucceeded || outcome.SemanticStatus != tool.SemanticNeutral || outcome.ProcessStatus != tool.ProcessExited {
		t.Fatalf("expected neutral difference, got %+v", outcome)
	}
	if outcome.ExitCode == nil || *outcome.ExitCode != 1 {
		t.Fatalf("exit code=%v", outcome.ExitCode)
	}
	if reason, _ := outcome.Diagnostic["reason"].(string); reason != "different" {
		t.Fatalf("diagnostic=%v", outcome.Diagnostic)
	}
}

func TestShellOutcomeTimeoutRetainsTruncatedStdoutAndStderr(t *testing.T) {
	command := "printf 123456789; printf abcdefghi >&2; sleep 1"
	if runtime.GOOS == "windows" {
		command = "echo 123456789 & echo abcdefghi 1>&2 & ping -n 3 127.0.0.1 >nul"
	}
	input, err := json.Marshal(map[string]any{
		"command":          command,
		"timeout_ms":       20,
		"max_output_chars": 4,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outcome, err := (&ShellTool{}).ExecuteOutcome(context.Background(), string(input), nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.TransportStatus != tool.TransportFailed || outcome.SemanticStatus != tool.SemanticFailure || outcome.ProcessStatus != tool.ProcessTimedOut {
		t.Fatalf("expected timed-out failure, got %+v", outcome)
	}
	if outcome.ErrorCode != tool.ErrorCodeProcessTimeout || !outcome.TimedOut || !outcome.Truncated {
		t.Fatalf("timeout fields: %+v", outcome)
	}
	stdout, _ := outcome.Diagnostic["stdout"].(string)
	stderr, _ := outcome.Diagnostic["stderr"].(string)
	if stdout == "" || stderr == "" || !strings.Contains(stdout, "truncated") || !strings.Contains(stderr, "truncated") {
		t.Fatalf("expected retained truncated streams, stdout=%q stderr=%q diagnostic=%v", stdout, stderr, outcome.Diagnostic)
	}
}

func TestShellOutcomeSpawnFailureIsStructured(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	outcome, err := (&ShellTool{}).ExecuteOutcome(ctx, `{"command":"printf never-runs","workdir":"missing/child"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome returned transport error: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.TransportStatus != tool.TransportFailed || outcome.SemanticStatus != tool.SemanticFailure || outcome.ProcessStatus != tool.ProcessSpawnFailed {
		t.Fatalf("expected structured spawn failure, got %+v", outcome)
	}
	if outcome.ErrorCode != tool.ErrorCodeProcessSpawnFailed || !outcome.Retryable {
		t.Fatalf("spawn failure fields: %+v", outcome)
	}
}

func TestNeutralNonzeroReasonHelpers(t *testing.T) {
	cases := []struct {
		command string
		want    string
		ok      bool
	}{
		{"rg pattern file", "no_match", true},
		{"grep pattern file", "no_match", true},
		{"diff a b", "different", true},
		{"test -f missing", "false_condition", true},
		{"[ -f missing ]", "false_condition", true},
		{"rg a; rg b", "", false},
		{"exit 1", "", false},
	}
	for _, tc := range cases {
		got, ok := neutralNonzeroReason(tc.command, 1)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("command=%q got=(%q,%v) want=(%q,%v)", tc.command, got, ok, tc.want, tc.ok)
		}
	}
}

func TestShellDescribeActivityInstall(t *testing.T) {
	activity := (&ShellTool{}).DescribeActivity(`{"command":"pip install requests --index-url sk-abcdefghijklmnopqrstuvwxyz"}`)
	if activity.Title != "正在安装依赖" {
		t.Fatalf("title=%q", activity.Title)
	}
	if !strings.Contains(activity.Detail, "pip install") {
		t.Fatalf("detail=%q", activity.Detail)
	}
	if strings.Contains(activity.Detail, "sk-abcdefghijklmnopqrstuvwxyz") || !strings.Contains(activity.Detail, "[redacted]") {
		t.Fatalf("expected redacted install activity, detail=%q", activity.Detail)
	}
}

func writeExitOneCommand(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\nexit 1\n"
	if runtime.GOOS == "windows" {
		path += ".bat"
		content = "@exit /b 1\r\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
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
