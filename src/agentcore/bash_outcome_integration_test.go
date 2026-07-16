package agentcore_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	bashtool "awaken/src/tools/bash"
)

type registeredBashTool struct {
	bashtool.Tool
	name string
}

func (t registeredBashTool) Name() string { return t.name }
func (t registeredBashTool) Definition() tool.ToolDefinition {
	definition := t.Tool.Definition()
	definition.Name, definition.DisplayName = t.name, t.name
	return definition
}

type allowAutoPolicy struct{}

func (allowAutoPolicy) Decide(context.Context, agentcore.ToolDecisionRequest) agentcore.ToolDecision {
	return agentcore.ToolDecision{Action: agentcore.ToolDecisionAllow, Source: agentcore.ToolDecisionSourceAutoAllowlist, Reason: "Bash integration allow"}
}
func (allowAutoPolicy) DecideExecution(context.Context, profile.ExecutionIntent, agentcore.ToolDecisionRequest) profile.ExecutionDecision {
	return profile.AllowDecision(string(profile.DecisionSourceAutoAllowlist), "Bash integration allow")
}

func TestBashExit7SurvivesRuntimeJournalAndAGUIProjection(t *testing.T) {
	const toolName = "bash_exit7_integration"
	if err := tool.RegisterToolForce(&registeredBashTool{name: toolName}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tool.UnregisterTool(toolName) })

	execProfile := profile.DangerFullAccess()
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext: tool.NewPermissionContext(t.TempDir(), nil),
		RunID:             "run-bash-exit7", Profile: &execProfile, AutoToolPolicy: allowAutoPolicy{},
	})
	result := runtime.Execute(context.Background(), convcontext.ToolCall{ID: "call-bash-exit7", Function: convcontext.FunctionCall{Name: toolName, Arguments: `{"command":"exit 7"}`}})
	if result.Outcome.Status != tool.OutcomeFailed || result.Outcome.ProcessStatus != tool.ProcessExited || result.Outcome.ExitCode == nil || *result.Outcome.ExitCode != 7 {
		t.Fatalf("runtime outcome lost exit-7 semantics: %+v", result)
	}
	if result.Outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero || !strings.Contains(result.Outcome.ModelPayload(), `"exit_code":7`) {
		t.Fatalf("structured Bash failure drifted: %+v", result.Outcome)
	}

	journal, err := awruntime.NewExecutionJournal(filepath.Join(t.TempDir(), "thread.jsonl"), "thread-bash-exit7")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	record := agentcore.ToolResultSemanticRecord("thread-bash-exit7", "run-bash-exit7", "turn-1", "session-1", result)
	if err := journal.Append(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	records, err := journal.SnapshotRecords()
	if err != nil {
		t.Fatal(err)
	}
	frames := agui.ProjectSemanticRecords(records)
	if len(frames) != 1 || frames[0].Type != agui.EventToolCallResult || frames[0].ProcessExitCode == nil || *frames[0].ProcessExitCode != 7 || frames[0].ErrorCode != tool.ErrorCodeProcessExitNonzero {
		t.Fatalf("AG-UI Bash projection drifted: %+v", frames)
	}
}
