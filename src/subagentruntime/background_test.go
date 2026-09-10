package subagentruntime

import (
	"context"
	"testing"
)

func TestLegacyBackgroundCannotOutliveRoot(t *testing.T) {
	root, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	started := make(chan struct{})
	cfg := Config{RootContext: root, RunID: "first", Starter: func(ctx context.Context, request StartRequest) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}}
	m := NewManager(context.Background(), cfg)
	defer m.Close(context.Background())
	task, err := m.Spawn(root, SpawnRequest{Description: "legacy background", Prompt: "work", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	if task.Background {
		t.Fatal("legacy flag detached child from parent")
	}
	<-started
	if err := m.EndAttempt(context.Background(), "root", "first"); err != nil {
		t.Fatal(err)
	}
	if m.HasLiveWork("first") {
		t.Fatal("root finished before child exited")
	}
	if task, _ := m.Status(task.TaskID); task.Status != StatusCancelled {
		t.Fatal(task.Status)
	}
	if _, err := m.Followup(context.Background(), task.TaskID, "must not revive"); err == nil {
		t.Fatal("ended parent revived child")
	}
	if _, err := m.Spawn(context.Background(), SpawnRequest{Description: "late", Prompt: "late"}); err == nil {
		t.Fatal("ended parent admitted child")
	}
}

func TestInboxRequiresContextCommitAndIsRetainedAcrossAssignments(t *testing.T) {
	policy := Policy{MaxActiveAgents: 1}
	started, release := make(chan struct{}), make(chan struct{})
	m := NewManager(context.Background(), Config{Policy: policy, Starter: func(ctx context.Context, request StartRequest) (string, error) {
		close(started)
		<-release
		return "saved", nil
	}})
	defer m.Close(context.Background())
	task, err := m.Spawn(context.Background(), SpawnRequest{Description: "worker", Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	receipt, err := m.Message(context.Background(), task.TaskID, "retain this correction")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.takeMessages(task.TaskID)) != 1 || len(m.takeMessages(task.TaskID)) != 1 {
		t.Fatal("admission destructively consumed inbox")
	}
	m.incorporateMessages(task.AgentID, []string{receipt.MessageID})
	if len(m.takeMessages(task.TaskID)) != 0 {
		t.Fatal("incorporated message was replayed")
	}
	close(release)
	if _, err := m.Wait(context.Background(), WaitRequest{TaskIDs: []string{task.TaskID}, EventDriven: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Message(context.Background(), task.TaskID, "for the next assignment"); err != nil {
		t.Fatal("idle Agent rejected inbox", err)
	}
	if _, err := m.CloseAgent(context.Background(), task.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Message(context.Background(), task.TaskID, "closed"); err == nil {
		t.Fatal("closed Agent accepted message")
	}
}
