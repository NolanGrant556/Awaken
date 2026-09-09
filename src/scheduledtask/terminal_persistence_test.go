package scheduledtask

import (
	"errors"
	"testing"
)

type deepAuditTerminalFailureRepo struct{ Repository }

func (r deepAuditTerminalFailureRepo) AppendExecution(id string, e Execution) (Execution, bool, error) {
	if e.Status.Terminal() {
		return Execution{}, false, errors.New("injected disk full at terminal append")
	}
	return r.Repository.AppendExecution(id, e)
}
func TestFailedTerminalPersistenceDoesNotPublishSuccess(t *testing.T) {
	h := newSchedHarness(t)
	task := agentCronTask("persist", "* * * * *", 300)
	task.Enabled = false
	task = h.mustCreate(t, task)
	h.sched = NewScheduler(SchedulerConfig{Repo: deepAuditTerminalFailureRepo{h.repo}, Clock: h.clock, Executor: h.exec, Validator: Validator{Clock: h.clock}, NewTimer: h.timers.newTimer, Emit: h.events.emit})
	h.mustStart(t)
	if _, err := h.sched.RunManual(task, "audit"); err != nil {
		t.Fatal(err)
	}
	h.sched.waitWorkers()
	disk, err := OpenFileRepository(h.repo.genDir, h.clock)
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	executions, err := disk.ListExecutions(task.TaskID, 50)
	if err != nil {
		t.Fatal(err)
	}
	var success bool
	for _, e := range h.events.snapshot() {
		if e.Type == EventExecutionFinished && e.Status == string(ExecutionStatusSuccess) {
			success = true
		}
	}
	if h.events.count(EventNotice) != 1 {
		t.Fatal("missing persistence failure notice")
	}
	if success || executions[0].Status != ExecutionStatusRunning {
		t.Fatalf("live event says success but durable execution is %s; terminal write error was discarded", executions[0].Status)
	}
}

func TestEveryFailedTerminalWriteReportsFailure(t *testing.T) {
	for _, status := range []ExecutionStatus{ExecutionStatusSuccess, ExecutionStatusError, ExecutionStatusCancelled, ExecutionStatusSkipped} {
		t.Run(string(status), func(t *testing.T) {
			h := newSchedHarness(t)
			task := h.mustCreate(t, agentCronTask("persist", "* * * * *", 300))
			exec, _, err := h.repo.AppendExecution(task.TaskID, Execution{Trigger: TriggerManual, Status: ExecutionStatusQueued, IdempotencyKey: "one"})
			if err != nil {
				t.Fatal(err)
			}
			h.sched.repo = deepAuditTerminalFailureRepo{h.repo}
			if h.sched.transitionTerminal(task, exec, status, "", "", "", "") {
				t.Fatal("failed commit acknowledged")
			}
			events := h.events.snapshot()
			if len(events) != 1 || events[0].Type != EventNotice || events[0].Status != "error" {
				t.Fatalf("missing safe error event: %+v", events)
			}
			sealed, err := h.repo.SealInterrupted(task.TaskID, h.clock)
			if err != nil || len(sealed) != 1 || sealed[0].Status != ExecutionStatusInterrupted {
				t.Fatalf("recovery failed: %+v %v", sealed, err)
			}
		})
	}
}
