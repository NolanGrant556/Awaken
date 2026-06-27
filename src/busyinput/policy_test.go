package busyinput

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeModeDefaultsToInterrupt(t *testing.T) {
	tests := []struct {
		raw  string
		want Mode
	}{
		{raw: "", want: ModeInterrupt},
		{raw: "bogus", want: ModeInterrupt},
		{raw: " interrupt ", want: ModeInterrupt},
		{raw: "QUEUE", want: ModeQueue},
		{raw: "steer", want: ModeSteer},
	}

	for _, test := range tests {
		if got := NormalizeMode(test.raw); got != test.want {
			t.Fatalf("NormalizeMode(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestDecideDrainingQueuesQueueAndSteerButRejectsInterrupt(t *testing.T) {
	queued := Decide(Request{Mode: ModeQueue, State: StateDraining})
	if !queued.Accept || !queued.QueuePending || queued.InterruptRequested {
		t.Fatalf("expected draining queue mode to queue without interrupt: %+v", queued)
	}
	if queued.Action != ActionDrainingQueued || queued.Reason != "draining_queued" {
		t.Fatalf("unexpected draining queue decision: %+v", queued)
	}

	restarting := Decide(Request{Mode: ModeSteer, State: StateRestarting})
	if !restarting.Accept || !restarting.QueuePending || restarting.InterruptRequested {
		t.Fatalf("expected restarting steer mode to queue after recovery: %+v", restarting)
	}
	if restarting.Action != ActionDrainingQueued || restarting.Reason != "restarting_queued" {
		t.Fatalf("unexpected restarting queue decision: %+v", restarting)
	}

	rejected := Decide(Request{Mode: ModeInterrupt, State: StateDraining})
	if rejected.Accept || !rejected.Rejected || rejected.QueuePending {
		t.Fatalf("expected draining interrupt mode to reject: %+v", rejected)
	}
	if rejected.Action != ActionDrainingRejected || rejected.Reason != "draining_rejected" {
		t.Fatalf("unexpected draining reject decision: %+v", rejected)
	}
}

func TestDecideSteerSuccessAndFallbackQueue(t *testing.T) {
	success := Decide(Request{
		Mode:           ModeSteer,
		State:          StateBusy,
		CanSteer:       true,
		SteerSucceeded: true,
	})
	if !success.SteerAttempted || !success.SteerSucceeded {
		t.Fatalf("expected steer to be attempted and succeed: %+v", success)
	}
	if success.QueuePending || success.InterruptRequested || success.SteerFallbackQueued {
		t.Fatalf("expected successful steer to avoid queue and interrupt: %+v", success)
	}
	if success.Action != ActionSteerSucceeded || success.Reason != "steer_succeeded" {
		t.Fatalf("unexpected steer success decision: %+v", success)
	}

	rejected := Decide(Request{
		Mode:           ModeSteer,
		State:          StateBusy,
		CanSteer:       true,
		SteerSucceeded: false,
	})
	if !rejected.SteerAttempted || !rejected.SteerFallbackQueued || !rejected.QueuePending {
		t.Fatalf("expected rejected steer to fall back to queue: %+v", rejected)
	}
	if rejected.InterruptRequested || rejected.Action != ActionSteerFallbackQueued {
		t.Fatalf("expected rejected steer fallback without interrupt: %+v", rejected)
	}

	unavailable := Decide(Request{Mode: ModeSteer, State: StateBusy})
	if unavailable.SteerAttempted || !unavailable.SteerFallbackQueued || !unavailable.QueuePending {
		t.Fatalf("expected unavailable steer to queue without attempt: %+v", unavailable)
	}
}

func TestDecideQueueDoesNotInterrupt(t *testing.T) {
	decision := Decide(Request{Mode: ModeQueue, State: StateBusy})
	if !decision.QueuePending {
		t.Fatalf("expected queue mode to mark pending queue: %+v", decision)
	}
	if decision.InterruptRequested {
		t.Fatalf("queue mode must not request interrupt: %+v", decision)
	}
	if decision.Action != ActionQueuePending || decision.Reason != "queue_pending" {
		t.Fatalf("unexpected queue decision: %+v", decision)
	}
}

func TestDecideInterruptRequestsInterrupt(t *testing.T) {
	decision := Decide(Request{Mode: ModeInterrupt, State: StateBusy})
	if !decision.InterruptRequested {
		t.Fatalf("expected interrupt mode to request interrupt: %+v", decision)
	}
	if !decision.QueuePending {
		t.Fatalf("expected interrupt mode to preserve pending input for next turn: %+v", decision)
	}
	if decision.Action != ActionInterruptRequested || decision.Reason != "interrupt_requested" {
		t.Fatalf("unexpected interrupt decision: %+v", decision)
	}
}

func TestShouldAckCooldownAndSuppressedDecision(t *testing.T) {
	lastAck := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	if !ShouldAck(time.Time{}, lastAck, DefaultAckCooldown) {
		t.Fatalf("expected zero last ack to allow ack")
	}
	if ShouldAck(lastAck, lastAck.Add(29*time.Second), DefaultAckCooldown) {
		t.Fatalf("expected ack within cooldown to be suppressed")
	}
	if !ShouldAck(lastAck, lastAck.Add(30*time.Second), DefaultAckCooldown) {
		t.Fatalf("expected ack at cooldown boundary to be allowed")
	}

	decision := Decide(Request{
		Mode:        ModeQueue,
		State:       StateBusy,
		LastAck:     lastAck,
		Now:         lastAck.Add(10 * time.Second),
		AckCooldown: DefaultAckCooldown,
	})
	if !decision.AckSuppressed {
		t.Fatalf("expected decision to express ack suppression: %+v", decision)
	}
	ack := BuildAck(decision, ActivitySummary{})
	if !ack.Suppressed || ack.Reason != "ack_suppressed" {
		t.Fatalf("unexpected suppressed ack: %+v", ack)
	}
}

func TestBuildAckIncludesActivityDetail(t *testing.T) {
	decision := Decide(Request{Mode: ModeQueue, State: StateBusy})
	ack := BuildAck(decision, ActivitySummary{
		RunningFor:  3*time.Minute + 20*time.Second,
		TurnIndex:   2,
		MaxTurns:    5,
		CurrentTool: "shell",
	})

	if ack.Reason != "queue_pending" {
		t.Fatalf("unexpected ack reason: %+v", ack)
	}
	if ack.Activity.RunningMinutes != 3 {
		t.Fatalf("expected running minutes to be normalized, got %+v", ack.Activity)
	}
	assertDetail(t, ack, "running 3 minutes")
	assertDetail(t, ack, "turn 2/5")
	assertDetail(t, ack, "tool shell")
	if !strings.Contains(ack.Status, "running 3 minutes") {
		t.Fatalf("expected status to include activity details, got %q", ack.Status)
	}
}

func assertDetail(t *testing.T, ack Ack, detail string) {
	t.Helper()
	for _, got := range ack.Details {
		if got == detail {
			return
		}
	}
	t.Fatalf("expected ack details to contain %q, got %+v", detail, ack.Details)
}
