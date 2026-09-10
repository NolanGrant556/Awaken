package subagentruntime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRecursiveWaitReleasesOnlyExecutionSlot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	policy := Policy{MaxActiveAgents: 1, MaxDepth: 2}
	var m *Manager
	m = NewManager(ctx, Config{Policy: policy, Starter: func(ctx context.Context, request StartRequest) (string, error) {
		child := m.Bind(ctx, request.Task.AgentID, 1, Config{Starter: func(_ context.Context, _ StartRequest) (string, error) { return "grandchild result", nil }})
		grandchild, err := child.Spawn(ctx, SpawnRequest{Description: "grandchild", Prompt: "work"})
		if err != nil {
			return "", err
		}
		result, err := child.Wait(ctx, WaitRequest{TaskIDs: []string{grandchild.TaskID}, EventDriven: true})
		if err != nil {
			return "", err
		}
		return result.Tasks[0].ResultPreview, nil
	}})
	defer m.Close(context.Background())
	parent, err := m.Spawn(ctx, SpawnRequest{Description: "parent", Prompt: "delegate"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Wait(ctx, WaitRequest{TaskIDs: []string{parent.TaskID}, EventDriven: true})
	if err != nil || result.Tasks[0].ResultPreview != "grandchild result" {
		t.Fatalf("recursive wait: %v %+v", err, result)
	}
}

func TestFollowupReusesAgentAndKeepsAssignmentsDistinct(t *testing.T) {
	ctx := context.Background()
	policy := Policy{MaxActiveAgents: 1}
	m := NewManager(ctx, Config{Policy: policy, Starter: func(_ context.Context, req StartRequest) (string, error) { return req.Prompt, nil }})
	defer m.Close(ctx)
	first, err := m.Spawn(ctx, SpawnRequest{Description: "worker", Prompt: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Wait(ctx, WaitRequest{TaskIDs: []string{first.TaskID}, EventDriven: true}); err != nil {
		t.Fatal(err)
	}
	second, err := m.Followup(ctx, first.TaskID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if second.AgentID != first.AgentID || second.TaskID == first.TaskID || second.AttemptID == first.AttemptID {
		t.Fatal("assignment identity was not separated from Agent identity")
	}
	if _, err = m.Wait(ctx, WaitRequest{TaskIDs: []string{second.TaskID}, EventDriven: true}); err != nil {
		t.Fatal(err)
	}
	old, _ := m.Status(first.TaskID)
	if old.ResultPreview != "first" {
		t.Fatal("follow-up rewrote previous result")
	}
	if _, err = m.CloseAgent(ctx, second.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Followup(ctx, second.TaskID, "must reject"); err == nil {
		t.Fatal("closed Agent restarted")
	}
}

func TestHostTypedResultIsNotTheUIPreview(t *testing.T) {
	full := `{"value":"` + strings.Repeat("x", maxResultBytes+1000) + `"}`
	m := NewManager(context.Background(), Config{Starter: func(context.Context, StartRequest) (string, error) { return full, nil }})
	defer m.Close(context.Background())
	task, err := m.Spawn(context.Background(), SpawnRequest{Description: "JSON", Prompt: "structured result"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Wait(context.Background(), WaitRequest{TaskIDs: []string{task.TaskID}, EventDriven: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tasks[0].ResultContent != full || len(result.Tasks[0].ResultPreview) > maxResultBytes {
		t.Fatal("typed result was truncated or public preview is unbounded")
	}
}
