package subagenttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

func TestDurableStatusAndWaitReturnFullBodyWithoutEarlyAcknowledgement(t *testing.T) {
	for _, action := range []string{"status", "wait"} {
		t.Run(action, func(t *testing.T) {
			policy := subagentruntime.Policy{}.Effective()
			manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{Policy: policy})
			defer manager.Close(context.Background())
			full := strings.Repeat("verified-result-", 4000) + "END-742"
			manager.Restore([]subagentruntime.RestoredTask{{Snapshot: subagentruntime.TaskSnapshot{TaskID: "task", AgentID: "child", ParentAgentID: "root", Status: subagentruntime.StatusCompleted, ResultContent: full, ResultPreview: "short preview"}}})
			input := `{"action":"status","task_id":"task"}`
			if action == "wait" {
				input = `{"action":"wait","task_ids":["task"],"return_when":"all"}`
			}
			output, err := (&SubagentTool{}).Execute(subagentruntime.WithController(context.Background(), manager), input)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, full) {
				t.Fatal("explicit retrieval returned only the UI preview")
			}
			if len(manager.DrainTerminalNotifications()) != 1 {
				t.Fatal("tool retrieval acknowledged before Runner persistence")
			}
		})
	}
}

func TestWaitHostDeadlineReturnsNeutralRetrievalStatus(t *testing.T) {
	release := make(chan struct{})
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(_ context.Context, _ subagentruntime.StartRequest) (string, error) {
			<-release
			return "done", nil
		},
	})
	t.Cleanup(func() {
		close(release)
		_ = manager.Close(context.Background())
	})
	task, err := manager.Spawn(context.Background(), subagentruntime.SpawnRequest{
		Description: "slow", Prompt: "slow", AgentType: "explore", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	ctx := subagentruntime.WithController(deadlineCtx, manager)
	raw, _ := json.Marshal(map[string]any{
		"action": "wait", "task_ids": []string{task.TaskID}, "return_when": "all", "timeout_ms": 1,
	})
	outcome, err := (&SubagentTool{}).ExecuteOutcome(ctx, string(raw), nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("neutral wait outcome=%+v err=%v", outcome, err)
	}
	var payload struct {
		Tasks           []subagentruntime.TaskSnapshot `json:"tasks"`
		RetrievalStatus string                         `json:"retrieval_status"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &payload); err != nil {
		t.Fatalf("decode outcome: %v", err)
	}
	if payload.RetrievalStatus != subagentruntime.RetrievalTimeout || len(payload.Tasks) != 1 || payload.Tasks[0].Terminal() {
		t.Fatalf("payload = %+v", payload)
	}
	if outcome.DurationMs < 0 || outcome.DurationMs > int64(time.Second/time.Millisecond) {
		t.Fatalf("unexpected duration: %dms", outcome.DurationMs)
	}
}

func TestSpawnInheritsHostTurnAndModelProfileIdentity(t *testing.T) {
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		ModelProfileID: "profile-1",
		Starter: func(context.Context, subagentruntime.StartRequest) (string, error) {
			return "done", nil
		},
	})
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	ctx := subagentruntime.WithController(context.Background(), manager)
	ctx = tool.ContextWithToolThreadID(ctx, "thread-1")
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{
		RunID: "run-1", TurnID: "turn-1", ToolCallID: "call-1", ModelProfileID: "profile-1",
	})
	output, err := (&SubagentTool{}).Execute(ctx, `{"action":"spawn","description":"inspect","prompt":"inspect","agent_type":"explore","workspace_mode":"shared_readonly"}`)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Task subagentruntime.TaskSnapshot `json:"task"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Task.TurnID != "turn-1" || payload.Task.ModelProfileID != "profile-1" {
		t.Fatalf("spawn identity was not inherited: %+v", payload.Task)
	}
}

func TestMessageActionQueuesPrompt(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(ctx context.Context, _ subagentruntime.StartRequest) (string, error) {
			close(started)
			<-release
			return "done", nil
		},
	})
	t.Cleanup(func() {
		close(release)
		_ = manager.Close(context.Background())
	})
	task, err := manager.Spawn(context.Background(), subagentruntime.SpawnRequest{
		Description: "inspect", Prompt: "inspect", AgentType: "explore", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	ctx := subagentruntime.WithController(context.Background(), manager)
	raw, _ := json.Marshal(map[string]any{"action": "message", "task_id": task.TaskID, "prompt": "also check errors"})
	output, err := (&SubagentTool{}).Execute(ctx, string(raw))
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	var payload struct {
		Receipt subagentruntime.MessageReceipt `json:"receipt"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil || payload.Receipt.MessageID == "" {
		t.Fatalf("receipt = %s err=%v", output, err)
	}
}

func TestTerminalStatusRetainsNotificationUntilRunnerAcknowledges(t *testing.T) {
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Starter: func(context.Context, subagentruntime.StartRequest) (string, error) {
			return "done", nil
		},
	})
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	task, err := manager.Spawn(context.Background(), subagentruntime.SpawnRequest{
		Description: "inspect", Prompt: "inspect", AgentType: "explore", WorkspaceMode: "shared_readonly",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		snapshot, statusErr := manager.Status(task.TaskID)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if snapshot.Terminal() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task did not reach terminal status")
		}
		time.Sleep(time.Millisecond)
	}

	ctx := subagentruntime.WithController(context.Background(), manager)
	statusInput, _ := json.Marshal(map[string]any{"action": "status", "task_id": task.TaskID})
	if _, err := (&SubagentTool{}).Execute(ctx, string(statusInput)); err != nil {
		t.Fatalf("status: %v", err)
	}
	if notifications := manager.DrainTerminalNotifications(); len(notifications) != 1 {
		t.Fatalf("status cannot acknowledge before Runner persistence: %+v", notifications)
	}
}
