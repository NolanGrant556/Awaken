package agentcore_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/agentcore/executors"
	"awaken/src/computer"
	computerfake "awaken/src/computer/fake"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	bashtool "awaken/src/tools/bash"
	computertool "awaken/src/tools/computer"
)

func TestToolRuntimeKeepsComputerSessionAndBashCWDWhenCallsAlternate(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()

	policy, err := computer.NewPolicySnapshot(computer.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	binding, err := computer.NewBinding("run-computer-bash", "thread-computer-bash", "root-agent", "", policy)
	if err != nil {
		t.Fatal(err)
	}
	computerRuntime := computer.NewRuntime(binding, computerfake.NewDriver())

	hostEnvironment := tool.ResolveHostEnvironment(ctx, tool.ExecutionEnvironmentProbeOptions{
		WorkspaceRoot:    workspace,
		WorkingDirectory: workspace,
		RefreshReason:    "computer_bash_integration",
		ProbeVersions:    false,
		ComputerEnabled:  true,
	})
	environment := hostEnvironment.Snapshot
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	execProfile := profile.DangerFullAccess()
	sessionState := tool.NewToolSessionState(workspace)
	runtime := agentcore.NewToolRuntimeWithConfig(nil, agentcore.ToolRuntimeConfig{
		PermissionContext:            tool.NewPermissionContext(workspace, []string{workspace}),
		AutoToolPolicy:               allowAutoPolicy{},
		RunID:                        "run-computer-bash",
		ToolAvailabilitySnapshot:     &snapshot,
		ExecutionEnvironmentSnapshot: &environment,
		Profile:                      &execProfile,
		ToolSessionState:             sessionState,
		ComputerExecutor:             executors.NewComputerExecutor(computerRuntime),
	})

	execute := func(id, name, arguments string) agentcore.ToolExecutionResult {
		t.Helper()
		result := runtime.Execute(ctx, convcontext.ToolCall{
			ID: id,
			Function: convcontext.FunctionCall{
				Name:      name,
				Arguments: arguments,
			},
		})
		if result.IsError || result.Outcome.Status != tool.OutcomeSucceeded {
			t.Fatalf("%s failed: %+v", name, result)
		}
		if result.ToolSnapshotID != snapshot.SnapshotID || result.EnvironmentSnapshotID != environment.SnapshotID {
			t.Fatalf("%s lost pinned snapshots: tool=%q environment=%q", name, result.ToolSnapshotID, result.EnvironmentSnapshotID)
		}
		return result
	}

	firstObserve := execute("observe-1", computertool.ToolNameObserve, `{"scope":"desktop","include":["accessibility"],"reason":"capture before Bash"}`)
	var firstObservation struct {
		ObservationID string `json:"observation_id"`
		SessionID     string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(firstObserve.Content), &firstObservation); err != nil {
		t.Fatal(err)
	}
	if firstObservation.ObservationID == "" || firstObservation.SessionID == "" {
		t.Fatalf("first observation omitted identity: %s", firstObserve.Content)
	}

	actArguments, _ := json.Marshal(map[string]any{
		"observation_id":  firstObservation.ObservationID,
		"action":          "wait",
		"target":          map[string]any{},
		"parameters":      map[string]any{"duration_ms": 1},
		"intent":          "create a verifiable Computer action without changing the desktop",
		"expected_effect": "desktop remains available",
		"idempotency_key": "computer-bash-integration-wait",
	})
	action := execute("act-1", computertool.ToolNameAct, string(actArguments))
	var actionResult struct {
		ActionID string `json:"action_id"`
	}
	if err := json.Unmarshal([]byte(action.Content), &actionResult); err != nil {
		t.Fatal(err)
	}
	if actionResult.ActionID == "" {
		t.Fatalf("Computer action omitted action_id: %s", action.Content)
	}

	bashResult := execute("bash-1", bashtool.ToolName, `{"command":"pwd","description":"confirm thread cwd between Computer calls"}`)
	var bashPayload struct {
		CWD    string `json:"cwd"`
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(bashResult.Content), &bashPayload); err != nil {
		t.Fatal(err)
	}
	expectedCWD, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		expectedCWD = workspace
	}
	if bashPayload.CWD != expectedCWD || sessionState.CWD("") != expectedCWD {
		t.Fatalf("Bash cwd drifted: payload=%q session=%q want=%q", bashPayload.CWD, sessionState.CWD(""), expectedCWD)
	}

	secondObserve := execute("observe-2", computertool.ToolNameObserve, `{"scope":"desktop","include":["accessibility"],"reason":"capture after Bash"}`)
	var secondObservation struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(secondObserve.Content), &secondObservation); err != nil {
		t.Fatal(err)
	}
	if secondObservation.SessionID != firstObservation.SessionID {
		t.Fatalf("Computer session changed across Bash: first=%q second=%q", firstObservation.SessionID, secondObservation.SessionID)
	}

	verifyArguments, _ := json.Marshal(map[string]any{
		"action_id":       actionResult.ActionID,
		"expected_effect": "desktop remains available",
		"assertions":      []any{},
	})
	execute("verify-1", computertool.ToolNameVerify, string(verifyArguments))

	currentSession, active := computerRuntime.Session()
	if !active || string(currentSession.SessionID) != firstObservation.SessionID {
		t.Fatalf("Computer session was not preserved across Bash: active=%v session=%q", active, currentSession.SessionID)
	}
}
