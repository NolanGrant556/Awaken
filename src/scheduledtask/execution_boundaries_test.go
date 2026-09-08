package scheduledtask

import (
	"context"
	"errors"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/runtime"
)

func TestExecutorAlreadyCompletedRunDoesNotHang(t *testing.T) {
	f := newExecutorFixture(t)
	task := f.agentTask("fast-completion")
	f.executor.SetExecutionStartedHandler(func(_ Task, _ Execution, runID, threadID string) {
		waitForServiceRunStatus(t, f.service, runID, runtime.RunStatusFinished)
		waitForAllThreadsIdle(t, f.service)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan *ExecError, 1)
	go func() {
		_, _, err := f.executor.ExecuteAgent(ctx, task, Execution{ExecutionID: "audit-fast"})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("already-finished run failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		cancel()
		select {
		case err := <-done:
			t.Errorf("finished run only ended after cancellation: %v", err)
		case <-time.After(200 * time.Millisecond):
			t.Errorf("finished run hangs even after cancellation; nil events drain")
		}
	}
}

func TestRunManualQueueSkipDoesNotConsumeSchedule(t *testing.T) {
	h := newSchedHarness(t)
	task := agentCronTask("manual", "* * * * *", 0)
	task.Enabled = false
	runAt := h.clock.Now().Add(time.Hour)
	task.Schedule = Schedule{Kind: ScheduleKindCalendar, Timezone: "UTC", RunAt: &runAt, RepeatEveryDays: 1, RepeatEnd: &RepeatEnd{Kind: RepeatEndCount, Count: 1}}
	task = h.mustCreate(t, task)
	h.mustStart(t)
	h.sched.agentSem <- struct{}{}
	h.sched.agentSem <- struct{}{}
	defer func() { <-h.sched.agentSem; <-h.sched.agentSem }()
	_, err := h.sched.RunManual(task, "audit-key")
	if err != nil {
		t.Fatal(err)
	}
	h.sched.waitWorkers()
	got, err := h.repo.Get(task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("manual skip consumed=%d completed=%v last=%v", got.ConsumedOccurrences, got.Completed, got.LastProcessedOccurrence)
	if got.ConsumedOccurrences != 0 || got.Completed || got.LastProcessedOccurrence != nil {
		t.Errorf("manual execution consumed future schedule")
	}
}

func TestCronSteppedSundayRange(t *testing.T) {
	sched := Schedule{Kind: ScheduleKindCron, Cron: "0 9 * * 2-7/2", Timezone: "UTC"}
	after := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC) // Saturday, next expected Tuesday
	got, ok := NextOccurrence(sched, after)
	want := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	t.Logf("cron=%s normalized=%s next=%s", sched.Cron, normalizeCronExpression(sched.Cron), got)
	if !ok || !got.Equal(want) {
		t.Errorf("next=%s want=%s", got, want)
	}
}

func TestExecutorAlreadyFailedOrGatedRunReturnsRecordedOutcome(t *testing.T) {
	for _, gate := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "interaction"}[gate], func(t *testing.T) {
			f := newExecutorFixture(t)
			f.probe.setRun(func(_ context.Context, _ string, sink agentcore.EventSink) (string, error) {
				if gate {
					sink.Emit(agentcore.AwakenEvent{Type: agentcore.UserQuestionRequestedType})
				}
				return "", errors.New("controlled failure")
			})
			f.executor.SetExecutionStartedHandler(func(_ Task, _ Execution, runID, threadID string) {
				waitForServiceRunStatus(t, f.service, runID, runtime.RunStatusError)
				waitForAllThreadsIdle(t, f.service)
			})
			done := make(chan *ExecError, 1)
			go func() {
				_, _, err := f.executor.ExecuteAgent(context.Background(), f.agentTask("early-error"), Execution{ExecutionID: "early"})
				done <- err
			}()
			select {
			case err := <-done:
				want := ErrorCodeRunFailed
				if gate {
					want = ErrorCodeScheduledInteractionRequired
				}
				if err == nil || err.Code != want {
					t.Fatalf("outcome=%v want=%s", err, want)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("terminal history left executor blocked")
			}
		})
	}
}
