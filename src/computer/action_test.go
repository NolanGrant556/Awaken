package computer

import (
	"testing"
	"time"
)

func TestActionDigestChanges(t *testing.T) {
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	sess, _ := NewSession(bind, CaptureWindow, TargetRef{AppName: "Test"})
	_ = sess.MarkReady()
	obs, _ := NewObservation(sess, CaptureWindow, sess.Target, Freshness{CapturedAt: time.Now().UTC(), FreshUntil: time.Now().UTC().Add(time.Minute)})

	spec := ActionSpec{
		Kind:           ActionClick,
		Target:         ElementRef{Locator: "button:Send"},
		Intent:         "send the message",
		ExpectedEffect: "message sent",
		IdempotencyKey: "idemp-1",
	}
	action1, err := PrepareAction(sess, obs, spec, RiskLow, time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("prepare action: %v", err)
	}

	// Same inputs produce the same digest.
	action2, err := PrepareAction(sess, obs, spec, RiskLow, time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("prepare action again: %v", err)
	}
	if action1.ActionDigest != action2.ActionDigest {
		t.Fatalf("same inputs produced different digests")
	}

	// Different intent changes digest.
	spec.Intent = "send the draft"
	action3, err := PrepareAction(sess, obs, spec, RiskLow, time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("prepare action with changed intent: %v", err)
	}
	if action3.ActionDigest == action1.ActionDigest {
		t.Fatalf("changed intent should change digest")
	}

	// Source observation, policy snapshot and idempotency identity are part of
	// the reviewed/executed action and must not be mutable after preparation.
	mutations := []func(*PreparedAction){
		func(action *PreparedAction) { action.ObservationID = "obs-other" },
		func(action *PreparedAction) { action.PolicySnapshotID = "policy-other" },
		func(action *PreparedAction) { action.IdempotencyKey = "idemp-other" },
		func(action *PreparedAction) { action.Parameters = map[string]any{"x": 42} },
	}
	for i, mutate := range mutations {
		tampered := action1
		mutate(&tampered)
		if err := tampered.ValidForSession(sess, time.Now().UTC()); err == nil {
			t.Fatalf("mutation %d should invalidate action digest/session binding", i)
		}
	}
}

func TestRiskClassification(t *testing.T) {
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	cases := []struct {
		kind   ActionKind
		intent string
		params map[string]any
		want   RiskLevel
	}{
		{ActionScroll, "scroll the list", nil, RiskLow},
		{ActionClick, "click the Send button", map[string]any{"label": "Send"}, RiskReview},
		{ActionClick, "click the Send button", nil, RiskReview},
		{ActionTypeText, "enter the password", map[string]any{"secret": true}, RiskHandoff},
	}
	for _, tc := range cases {
		got := RiskForAction(bind, tc.kind, ElementRef{}, tc.intent, tc.params)
		if got != tc.want {
			t.Errorf("risk for %q (%s): got %s, want %s", tc.intent, tc.kind, got, tc.want)
		}
	}
}

func TestRiskClassificationHonorsConfiguredReviewKinds(t *testing.T) {
	policy := DefaultPolicy()
	policy.RequireReviewFor = []ActionKind{ActionKey}
	binding, err := NewBinding("run-1", "thread-1", "agent-1", "", policy)
	if err != nil {
		t.Fatal(err)
	}
	if got := RiskForAction(binding, ActionKey, ElementRef{}, "move focus", map[string]any{"key": "Tab"}); got != RiskReview {
		t.Fatalf("expected configured key action to require review, got %s", got)
	}
}
