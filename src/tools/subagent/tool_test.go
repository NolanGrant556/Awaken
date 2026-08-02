package subagenttool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"awaken/src/subagentruntime"
)

func TestWaitTimeoutReturnsNeutralRetrievalStatus(t *testing.T) {
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
	ctx := subagentruntime.WithController(context.Background(), manager)
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
