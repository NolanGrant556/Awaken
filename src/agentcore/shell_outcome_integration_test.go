package agentcore_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	shelltool "awaken/src/tools/shell"
)

type registeredShellTool struct {
	shelltool.ShellTool
	name string
}

func TestShellDirectDeleteUsesCurrentLoginUserProvider(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "obsolete.txt")
	if err := os.WriteFile(target, []byte("direct shell fixture"), 0o644); err != nil {
		t.Fatal(err)
	}

	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
	})
	result := runtime.Execute(t.Context(), convcontext.ToolCall{
		ID: "call_delete_transform",
		Function: convcontext.FunctionCall{
			Name: "shell", Arguments: `{"command":"rm -f obsolete.txt"}`,
		},
	})
	if result.IsError || result.Decision == nil || result.Decision.Action != profile.DecisionAllow {
		t.Fatalf("direct delete was not executed successfully: %+v", result)
	}
	if result.TransformedFrom != "" {
		t.Fatalf("raw shell must not be transformed to a typed delete: %+v", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("original target still exists or stat failed unexpectedly: %v", err)
	}
	if result.Outcome.Operation == nil || result.Outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("direct shell must preserve unobserved side-effect semantics: %+v", result.Outcome)
	}
	if len(result.Outcome.Changes) != 0 || !strings.Contains(result.Outcome.Content, `"broker_id": "local_user_direct_v1"`) {
		t.Fatalf("direct provider must not invent Trash changes and must expose its receipt: %+v", result.Outcome)
	}
}

func (t registeredShellTool) Name() string { return t.name }

func (t registeredShellTool) Definition() tool.ToolDefinition {
	definition := t.ShellTool.Definition()
	definition.Name = t.name
	definition.DisplayName = t.name
	return definition
}

type installActivityProbeTool struct {
	name string
}

type allowAutoPolicy struct{}

func (allowAutoPolicy) Decide(context.Context, agentcore.ToolDecisionRequest) agentcore.ToolDecision {
	return agentcore.ToolDecision{
		Action: agentcore.ToolDecisionAllow,
		Source: agentcore.ToolDecisionSourceAutoAllowlist,
		Reason: "phase8 integration allow",
	}
}

func (allowAutoPolicy) DecideExecution(context.Context, profile.ExecutionIntent, agentcore.ToolDecisionRequest) profile.ExecutionDecision {
	return profile.AllowDecision(string(profile.DecisionSourceAutoAllowlist), "phase8 integration allow")
}

func (t installActivityProbeTool) Name() string        { return t.name }
func (t installActivityProbeTool) Description() string { return "install activity probe" }
func (t installActivityProbeTool) Parameters() []tool.ToolParameter {
	return (&shelltool.ShellTool{}).Parameters()
}
func (t installActivityProbeTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.name,
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			CapabilityClass: tool.CapabilitySystemOperation,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
}
func (t installActivityProbeTool) CheckPermission(context.Context, string, tool.PermissionContext) tool.PermissionDecision {
	return tool.AllowPermission()
}
func (t installActivityProbeTool) Execute(context.Context, string) (string, error) {
	return "probe completed", nil
}
func (t installActivityProbeTool) DescribeActivity(input string) tool.ToolActivity {
	return (&shelltool.ShellTool{}).DescribeActivity(input)
}

func TestShellExit7SurvivesRuntimeJournalAndAGUIProjection(t *testing.T) {
	const toolName = "phase8_shell_exit7_integration"
	if err := tool.RegisterToolForce(&registeredShellTool{name: toolName}); err != nil {
		t.Fatalf("register shell tool: %v", err)
	}
	t.Cleanup(func() { tool.UnregisterTool(toolName) })

	root := t.TempDir()
	execProfile := profile.DangerFullAccess()
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
		RunID:             "run-shell-exit7",
		Profile:           &execProfile,
		AutoToolPolicy:    allowAutoPolicy{},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call-shell-exit7",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"command":"exit 7"}`,
		},
	})
	if result.Outcome.Status != tool.OutcomeFailed || result.Outcome.ProcessStatus != tool.ProcessExited || result.Outcome.ExitCode == nil || *result.Outcome.ExitCode != 7 {
		t.Fatalf("runtime outcome lost exit-7 semantics: %+v", result)
	}
	if result.Outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero {
		t.Fatalf("runtime error code=%q", result.Outcome.ErrorCode)
	}
	modelPayload := result.Outcome.ModelPayload()
	if !strings.Contains(modelPayload, string(tool.ErrorCodeProcessExitNonzero)) || !strings.Contains(modelPayload, `"exit_code":7`) {
		t.Fatalf("model payload lost structured failure: %s", modelPayload)
	}

	journal, err := awruntime.NewExecutionJournal(filepath.Join(t.TempDir(), "thread.jsonl"), "thread-shell-exit7")
	if err != nil {
		t.Fatalf("new journal: %v", err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	record := agentcore.ToolResultSemanticRecord("thread-shell-exit7", "run-shell-exit7", "turn-1", "session-1", result)
	if err := journal.Append(context.Background(), record); err != nil {
		t.Fatalf("append result: %v", err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatalf("flush result: %v", err)
	}
	records, err := journal.SnapshotRecords()
	if err != nil {
		t.Fatalf("snapshot journal: %v", err)
	}
	if len(records) != 1 || records[0].Seq == 0 {
		t.Fatalf("unexpected durable records: %+v", records)
	}
	frames := agui.ProjectSemanticRecord(records[0])
	if len(frames) != 1 {
		t.Fatalf("expected one ToolResult frame, got %+v", frames)
	}
	frame := frames[0]
	if frame.Type != agui.EventToolCallResult || frame.ToolStatus != string(tool.OutcomeFailed) || frame.TransportStatus != string(tool.TransportCompleted) || frame.SemanticStatus != string(tool.SemanticFailure) || frame.ProcessStatus != string(tool.ProcessExited) {
		t.Fatalf("AG-UI status drift: %+v", frame)
	}
	if frame.ProcessExitCode == nil || *frame.ProcessExitCode != 7 || frame.ErrorCode != tool.ErrorCodeProcessExitNonzero {
		t.Fatalf("AG-UI exit/error drift: %+v", frame)
	}
}

func TestInstallActivityIsHostGeneratedRedactedAndMappedWithoutNarrative(t *testing.T) {
	const toolName = "phase8_install_activity_probe"
	if err := tool.RegisterToolForce(installActivityProbeTool{name: toolName}); err != nil {
		t.Fatalf("register activity tool: %v", err)
	}
	t.Cleanup(func() { tool.UnregisterTool(toolName) })

	var events []agentcore.AwakenEvent
	execProfile := profile.DangerFullAccess()
	runtime := agentcore.NewToolRuntimeWithConfig(agentcore.EventSinkFunc(func(event agentcore.AwakenEvent) {
		events = append(events, event)
	}), agentcore.ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(t.TempDir(), nil),
		Profile:           &execProfile,
		AutoToolPolicy:    allowAutoPolicy{},
	})
	secret := "sk-abcdefghijklmnopqrstuvwxyz"
	result := runtime.Execute(context.Background(), convcontext.ToolCall{
		ID: "call-install-activity",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"command":"pip install requests --index-url ` + secret + `"}`,
		},
	})
	if result.IsError {
		t.Fatalf("probe execution failed: %+v", result)
	}

	var started *agentcore.AwakenEvent
	for i := range events {
		if events[i].Type == agentcore.ToolProgressEventType && events[i].ToolProgressStage == "started" {
			started = &events[i]
			break
		}
	}
	if started == nil {
		t.Fatalf("host activity missing without assistant narrative: %+v", events)
	}
	if !strings.Contains(started.ToolProgressMessage, "正在安装依赖") || !strings.Contains(started.ToolProgressMessage, "[redacted]") || strings.Contains(started.ToolProgressMessage, secret) {
		t.Fatalf("activity was not descriptive and redacted: %+v", started)
	}
	mapped := agui.MapEvent(*started)
	if len(mapped) != 1 || mapped[0].Type != agui.EventToolProgress || mapped[0].ToolProgressMessage != started.ToolProgressMessage {
		t.Fatalf("host activity did not survive AG-UI mapping: %+v", mapped)
	}
}

func TestShellNeutralTimeoutAndSpawnOutcomesSurviveJournalProjection(t *testing.T) {
	const toolName = "phase8_shell_matrix_integration"
	if err := tool.RegisterToolForce(&registeredShellTool{name: toolName}); err != nil {
		t.Fatalf("register shell tool: %v", err)
	}
	t.Cleanup(func() { tool.UnregisterTool(toolName) })

	binDir := t.TempDir()
	writeExternalExitOneCommand(t, binDir, "rg")
	writeExternalExitOneCommand(t, binDir, "diff")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	execProfile := profile.DangerFullAccess()
	toolRuntime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(root, []string{root}),
		Profile:           &execProfile,
		AutoToolPolicy:    allowAutoPolicy{},
	})

	t.Run("rg no match", func(t *testing.T) {
		result := toolRuntime.Execute(context.Background(), shellCall("call-rg-neutral", toolName, map[string]any{
			"command": "rg missing-token sample.txt",
		}))
		frame := durableToolResultFrame(t, result, "rg-neutral")
		if frame.ToolStatus != string(tool.OutcomeSucceeded) || frame.SemanticStatus != string(tool.SemanticNeutral) || frame.ProcessExitCode == nil || *frame.ProcessExitCode != 1 || frame.Diagnostic["reason"] != "no_match" {
			t.Fatalf("neutral rg drifted across journal projection: result=%+v frame=%+v", result, frame)
		}
	})

	t.Run("diff different", func(t *testing.T) {
		result := toolRuntime.Execute(context.Background(), shellCall("call-diff-neutral", toolName, map[string]any{
			"command": "diff before.txt after.txt",
		}))
		frame := durableToolResultFrame(t, result, "diff-neutral")
		if frame.ToolStatus != string(tool.OutcomeSucceeded) || frame.SemanticStatus != string(tool.SemanticNeutral) || frame.Diagnostic["reason"] != "different" {
			t.Fatalf("neutral diff drifted across journal projection: result=%+v frame=%+v", result, frame)
		}
	})

	t.Run("timeout retains truncated streams", func(t *testing.T) {
		command := "printf 123456789; printf abcdefghi >&2; sleep 1"
		if runtime.GOOS == "windows" {
			command = "echo 123456789 & echo abcdefghi 1>&2 & ping -n 3 127.0.0.1 >nul"
		}
		result := toolRuntime.Execute(context.Background(), shellCall("call-timeout", toolName, map[string]any{
			"command":          command,
			"timeout_ms":       20,
			"max_output_chars": 4,
		}))
		frame := durableToolResultFrame(t, result, "timeout")
		stdout, _ := frame.Diagnostic["stdout"].(string)
		stderr, _ := frame.Diagnostic["stderr"].(string)
		if frame.ToolStatus != string(tool.OutcomeFailed) || frame.TransportStatus != string(tool.TransportFailed) || frame.ProcessStatus != string(tool.ProcessTimedOut) || frame.ErrorCode != tool.ErrorCodeProcessTimeout || stdout == "" || stderr == "" {
			t.Fatalf("timeout drifted across journal projection: result=%+v frame=%+v", result, frame)
		}
	})

	t.Run("spawn failure", func(t *testing.T) {
		result := toolRuntime.Execute(context.Background(), shellCall("call-spawn-failure", toolName, map[string]any{
			"command": "printf never-runs",
			"workdir": "missing/child",
		}))
		frame := durableToolResultFrame(t, result, "spawn-failure")
		if frame.ToolStatus != string(tool.OutcomeFailed) || frame.TransportStatus != string(tool.TransportFailed) || frame.ProcessStatus != string(tool.ProcessSpawnFailed) || frame.ErrorCode != tool.ErrorCodeProcessSpawnFailed {
			t.Fatalf("spawn failure drifted across journal projection: result=%+v frame=%+v", result, frame)
		}
	})
}

func shellCall(id, toolName string, input map[string]any) convcontext.ToolCall {
	arguments, _ := json.Marshal(input)
	return convcontext.ToolCall{
		ID: id,
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: string(arguments),
		},
	}
}

func durableToolResultFrame(t *testing.T, result agentcore.ToolExecutionResult, suffix string) agui.Event {
	t.Helper()
	threadID := "thread-" + suffix
	journal, err := awruntime.NewExecutionJournal(filepath.Join(t.TempDir(), "thread.jsonl"), threadID)
	if err != nil {
		t.Fatalf("new journal: %v", err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	record := agentcore.ToolResultSemanticRecord(threadID, "run-"+suffix, "turn-1", "session-1", result)
	if err := journal.Append(context.Background(), record); err != nil {
		t.Fatalf("append result: %v", err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatalf("flush result: %v", err)
	}
	records, err := journal.SnapshotRecords()
	if err != nil {
		t.Fatalf("snapshot journal: %v", err)
	}
	frames := agui.ProjectSemanticRecords(records)
	if len(frames) != 1 || frames[0].Type != agui.EventToolCallResult {
		t.Fatalf("unexpected projected frames: %+v", frames)
	}
	return frames[0]
}

func writeExternalExitOneCommand(t *testing.T, dir, name string) {
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
