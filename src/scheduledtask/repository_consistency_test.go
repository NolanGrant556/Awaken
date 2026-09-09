package scheduledtask

import (
	"errors"
	"testing"
	"time"
)

func TestRepositoryCASAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	clock := NewFakeClock(time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC))
	a, err := OpenFileRepository(dir, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	task, err := a.Create(agentCronTask("original", "* * * * *", 300))
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenFileRepository(dir, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err = a.Update(task.TaskID, task.Version, func(v Task) (Task, error) { v.Name = "A saved"; return v, nil }); err != nil {
		t.Fatal(err)
	}
	_, staleErr := b.Update(task.TaskID, task.Version, func(v Task) (Task, error) { v.Name = "B stale overwrite"; return v, nil })
	reopened, openErr := OpenFileRepository(dir, clock)
	if openErr != nil {
		t.Fatalf("stale write err=%v; independent reopen failed: %v", staleErr, openErr)
	}
	defer reopened.Close()
	latest, err := reopened.Get(task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Name != "A saved" || latest.Version != task.Version+1 {
		t.Fatalf("independent disk readback lost accepted update: %+v", latest)
	}
	if !errors.Is(staleErr, ErrVersionConflict) {
		t.Fatalf("stale version %d was accepted: err=%v; disk now name=%q version=%d", task.Version, staleErr, latest.Name, latest.Version)
	}
}

func TestRepositoryInstancesShareExecutionIdentityAndTombstones(t *testing.T) {
	dir := t.TempDir()
	a, err := OpenFileRepository(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	task, err := a.Create(agentCronTask("shared", "* * * * *", 300))
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenFileRepository(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	first, _, err := a.AppendExecution(task.TaskID, Execution{Trigger: TriggerManual, IdempotencyKey: "same", Status: ExecutionStatusQueued})
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := b.AppendExecution(task.TaskID, Execution{Trigger: TriggerManual, IdempotencyKey: "same", Status: ExecutionStatusQueued})
	if err != nil || created || second.ExecutionID != first.ExecutionID {
		t.Fatalf("duplicate execution: %+v created=%v err=%v", second, created, err)
	}
	first.Status = ExecutionStatusSuccess
	if _, _, err = a.AppendExecution(task.TaskID, first); err != nil {
		t.Fatal(err)
	}
	second.Status = ExecutionStatusRunning
	if _, _, err = b.AppendExecution(task.TaskID, second); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("stale transition accepted: %v", err)
	}
	at := time.Now().UTC()
	if _, err = a.ConsumeOccurrence(task.TaskID, at); err != nil {
		t.Fatal(err)
	}
	consumed, err := b.ConsumeOccurrence(task.TaskID, at)
	if err != nil || consumed.ConsumedOccurrences != 1 {
		t.Fatalf("occurrence counted twice: %+v %v", consumed, err)
	}
	task = consumed
	if err = a.Delete(task.TaskID, task.Version, "delete"); err != nil {
		t.Fatal(err)
	}
	if _, err = b.SetResumed(task.TaskID, task.Version); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("deleted task revived: %v", err)
	}
	if err = b.Delete(task.TaskID, task.Version, "delete"); err != nil {
		t.Fatalf("delete retry lost receipt: %v", err)
	}
	if tasks, err := b.List(); err != nil || len(tasks) != 0 {
		t.Fatalf("stale list: %+v %v", tasks, err)
	}
}
