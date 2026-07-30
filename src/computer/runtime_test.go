package computer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRuntimeObserveActVerify(t *testing.T) {
	ctx := context.Background()
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", DefaultPolicy())
	driver := NewUnavailableDriver()
	runtime := NewRuntime(bind, driver)

	if _, err := runtime.Observe(ctx, ObserveOptions{}); err == nil {
		t.Fatalf("observe without session should fail")
	}
}

func TestRuntimeWithFakeDriver(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", policy)
	fake := &fakeDriver{healthy: true}
	runtime := NewRuntime(bind, fake)

	sess, err := runtime.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if sess.State != SessionReady {
		t.Fatalf("expected ready, got %s", sess.State)
	}

	obs, err := runtime.Observe(ctx, ObserveOptions{IncludeScreenshot: true})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.Screenshot == nil {
		t.Fatalf("expected screenshot ref")
	}

	action, err := runtime.Prepare(ctx, obs, ActionSpec{
		Kind:           ActionClick,
		Target:         ElementRef{Locator: "button:OK"},
		Intent:         "confirm the dialog",
		ExpectedEffect: "dialog closes",
		IdempotencyKey: "idemp-1",
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if action.ActionDigest == "" {
		t.Fatalf("expected action digest")
	}

	res, err := runtime.Perform(ctx, action, nil)
	if err != nil {
		t.Fatalf("perform: %v", err)
	}
	if res.Status != ActionSucceeded {
		t.Fatalf("expected succeeded, got %s", res.Status)
	}
}

// fakeDriver is a minimal in-package fake for runtime tests.
type fakeDriver struct {
	healthy   bool
	session   Session
	onPerform func(context.Context, SessionID, PreparedAction, FenceToken, ControlEpoch) (ActionResult, error)
}

func (f *fakeDriver) Health(ctx context.Context) (HealthResult, error) {
	return HealthResult{Healthy: f.healthy, TCCScreenRecording: true, TCCAccessibility: true}, nil
}
func (f *fakeDriver) OpenSession(ctx context.Context, bind Binding, proposed Session, policy PolicySnapshot, fence FenceToken) (Session, error) {
	sess, err := NewSession(bind, CaptureWindow, TargetRef{AppName: "FakeApp"})
	if err != nil {
		return Session{}, err
	}
	_ = sess.MarkReady()
	f.session = sess
	return sess, nil
}
func (f *fakeDriver) Observe(ctx context.Context, sessionID SessionID, scope CaptureScope, target TargetRef, opts ObserveOptions) (Observation, error) {
	now := time.Now().UTC()
	sess := Session{SessionID: sessionID, BindingID: f.session.BindingID, FenceToken: f.session.FenceToken, ControlEpoch: f.session.ControlEpoch, PolicySnapshotID: f.session.PolicySnapshotID, Target: target}
	obs, err := NewObservation(sess, scope, target, Freshness{CapturedAt: now, FreshUntil: now.Add(time.Minute)})
	if err != nil {
		return Observation{}, err
	}
	obs.Screenshot = &MediaRef{StorageKey: "fake://screenshot/" + string(obs.ObservationID), MIMEType: "image/png", ByteSize: 1024, SHA256: "0000"}
	return obs, nil
}
func (f *fakeDriver) Prepare(ctx context.Context, sessionID SessionID, obs Observation, spec ActionSpec) (PreparedAction, error) {
	expires := time.Now().UTC().Add(time.Minute)
	sess := Session{SessionID: sessionID, BindingID: f.session.BindingID, FenceToken: f.session.FenceToken, ControlEpoch: f.session.ControlEpoch, PolicySnapshotID: f.session.PolicySnapshotID, Target: obs.Target, State: SessionReady}
	risk := RiskForAction(Binding{
		AllowedActionKinds: DefaultPolicy().AllowedActionKinds,
	}, spec.Kind, spec.Target, spec.Intent, spec.Parameters)
	return PrepareAction(sess, obs, spec, risk, expires)
}
func (f *fakeDriver) PerformChecked(ctx context.Context, sessionID SessionID, action PreparedAction, fence FenceToken, epoch ControlEpoch) (ActionResult, error) {
	if f.onPerform != nil {
		return f.onPerform(ctx, sessionID, action, fence, epoch)
	}
	res := NewActionResult(action.ActionID, sessionID, ActionSucceeded)
	res.HostRequestID = HostRequestID("req-" + string(action.ActionID))
	return res, nil
}
func (f *fakeDriver) Quiesce(ctx context.Context, sessionID SessionID, nextEpoch ControlEpoch) error {
	f.session.ControlEpoch = nextEpoch
	return nil
}
func (f *fakeDriver) Resume(ctx context.Context, sessionID SessionID, policy PolicySnapshot, fence FenceToken, epoch ControlEpoch) (Session, error) {
	f.session.ControlEpoch = epoch
	f.session.FenceToken = fence
	return f.session, nil
}
func (f *fakeDriver) CloseSession(ctx context.Context, sessionID SessionID, reason string) error {
	return nil
}

var _ Driver = (*fakeDriver)(nil)

func TestRuntimeTerminateInterruptsComputerUseWithoutCancellingRunContext(t *testing.T) {
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	started := make(chan struct{})
	fake := &fakeDriver{
		healthy: true,
		onPerform: func(ctx context.Context, _ SessionID, _ PreparedAction, _ FenceToken, _ ControlEpoch) (ActionResult, error) {
			close(started)
			<-ctx.Done()
			return ActionResult{}, ctx.Err()
		},
	}
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-stop-computer", "thread-1", "agent-1", "", policy)
	runtime := NewRuntime(bind, fake)

	if _, err := runtime.OpenSession(runCtx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	obs, err := runtime.Observe(runCtx, ObserveOptions{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	action, err := runtime.Prepare(runCtx, obs, ActionSpec{Kind: ActionWait, Intent: "wait"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	performDone := make(chan error, 1)
	go func() {
		_, performErr := runtime.Perform(runCtx, action, nil)
		performDone <- performErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("perform did not start")
	}

	terminateDone := make(chan error, 1)
	go func() {
		terminateDone <- runtime.Terminate(context.Background(), "user_stopped")
	}()
	select {
	case err := <-terminateDone:
		if err != nil {
			t.Fatalf("terminate: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("terminate blocked behind active Computer action")
	}
	select {
	case err := <-performDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("perform error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active Computer action was not interrupted")
	}

	if err := runCtx.Err(); err != nil {
		t.Fatalf("run context was cancelled: %v", err)
	}
	if _, active := runtime.Session(); active {
		t.Fatal("Computer session remains active after terminate")
	}
	if _, err := runtime.OpenSession(runCtx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err == nil ||
		!strings.Contains(err.Error(), computerUseStoppedMessage) {
		t.Fatalf("reopening Computer Use error = %v, want user-stopped error", err)
	}
}

func TestRuntimeObservationStore(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", policy)
	fake := &fakeDriver{healthy: true}
	runtime := NewRuntime(bind, fake)

	if _, err := runtime.GetObservation(ObservationID("missing")); err == nil {
		t.Fatal("expected error for missing observation without session")
	}

	sess, err := runtime.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}

	obs, err := runtime.Observe(ctx, ObserveOptions{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}

	got, err := runtime.GetObservation(obs.ObservationID)
	if err != nil {
		t.Fatalf("get observation: %v", err)
	}
	if got.ObservationID != obs.ObservationID {
		t.Fatalf("observation mismatch")
	}

	// Unknown observation returns error.
	if _, err := runtime.GetObservation(ObservationID("unknown")); err == nil {
		t.Fatal("expected error for unknown observation")
	}

	// Expired observation returns error.
	obs.Freshness.FreshUntil = time.Now().UTC().Add(-time.Second)
	runtime.observations[obs.ObservationID] = &obs
	if _, err := runtime.GetObservation(obs.ObservationID); err == nil {
		t.Fatal("expected error for expired observation")
	}

	// Takeover increases epoch and invalidates old observations.
	if err := runtime.Takeover(ctx); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if runtime.Binding().ControlEpoch == sess.ControlEpoch {
		t.Fatal("expected control epoch to increase after takeover")
	}
	if _, err := runtime.GetObservation(obs.ObservationID); err == nil {
		t.Fatal("expected error after takeover invalidates old observation")
	}
}

func TestRuntimeReviewLifecycle(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", policy)
	fake := &fakeDriver{healthy: true}
	runtime := NewRuntime(bind, fake)

	if _, err := runtime.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	obs, err := runtime.Observe(ctx, ObserveOptions{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}

	action, err := runtime.Prepare(ctx, obs, ActionSpec{
		Kind:           ActionClick,
		Target:         ElementRef{Locator: "button:Send"},
		Intent:         "send the email",
		ExpectedEffect: "email sent",
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if action.Risk != RiskReview {
		t.Fatalf("expected review risk, got %s", action.Risk)
	}

	review, err := runtime.RequestReview(ctx, RequestReviewInput{
		Action:        action,
		TargetSummary: "click Send button",
		EffectSummary: "send the email",
	})
	if err != nil {
		t.Fatalf("request review: %v", err)
	}
	if review.Decision != "" {
		t.Fatalf("expected pending review, got decision %s", review.Decision)
	}

	got, ok := runtime.GetReview(review.ReviewID)
	if !ok {
		t.Fatal("expected review in ledger")
	}
	if got.ReviewID != review.ReviewID {
		t.Fatal("review id mismatch")
	}

	// Performing without review fails.
	if _, err := runtime.Perform(ctx, action, nil); err == nil {
		t.Fatal("expected perform without review to fail")
	}

	// Resolving with an invalid decision fails.
	if _, err := runtime.ResolveReview(ctx, review.ReviewID, "invalid", "user", "req-1"); err == nil {
		t.Fatal("expected invalid decision to fail")
	}

	resolved, err := runtime.ResolveReview(ctx, review.ReviewID, ReviewApprovedOnce, "user", "req-1")
	if err != nil {
		t.Fatalf("resolve review: %v", err)
	}
	if resolved.Decision != ReviewApprovedOnce {
		t.Fatalf("expected approved_once, got %s", resolved.Decision)
	}

	// Double resolution fails.
	if _, err := runtime.ResolveReview(ctx, review.ReviewID, ReviewApprovedOnce, "user", "req-2"); err == nil {
		t.Fatal("expected double resolution to fail")
	}

	// Perform with approved review succeeds.
	res, err := runtime.Perform(ctx, action, resolved)
	if err != nil {
		t.Fatalf("perform with review: %v", err)
	}
	if res.Status != ActionSucceeded {
		t.Fatalf("expected succeeded, got %s", res.Status)
	}
}

func TestRuntimePauseAndReturnUseFreshControlEpoch(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-pause", "thread-pause", "agent-pause", "", policy)
	runtime := NewRuntime(bind, &fakeDriver{healthy: true})
	if _, err := runtime.OpenSession(ctx, CaptureDesktop, TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	before := runtime.Binding().ControlEpoch
	if err := runtime.Pause(ctx); err != nil {
		t.Fatalf("pause: %v", err)
	}
	pausedSession, ok := runtime.Session()
	if !ok {
		t.Fatal("session missing after pause")
	}
	if got := pausedSession.State; got != SessionPaused {
		t.Fatalf("state after pause = %s, want paused", got)
	}
	pausedEpoch := runtime.Binding().ControlEpoch
	if pausedEpoch == before {
		t.Fatal("pause did not advance control epoch")
	}
	if err := runtime.Return(ctx); err != nil {
		t.Fatalf("return: %v", err)
	}
	returnedSession, ok := runtime.Session()
	if !ok {
		t.Fatal("session missing after return")
	}
	if got := returnedSession.State; got != SessionReady {
		t.Fatalf("state after return = %s, want ready", got)
	}
	if runtime.Binding().ControlEpoch == pausedEpoch {
		t.Fatal("return did not advance control epoch")
	}
}

func TestRuntimeTakeoverBlocksPerform(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", policy)
	fake := &fakeDriver{healthy: true}
	runtime := NewRuntime(bind, fake)

	if _, err := runtime.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open session: %v", err)
	}
	obs, err := runtime.Observe(ctx, ObserveOptions{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	action, err := runtime.Prepare(ctx, obs, ActionSpec{Kind: ActionWait, Intent: "wait"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if err := runtime.Takeover(ctx); err != nil {
		t.Fatalf("takeover: %v", err)
	}

	if _, err := runtime.Perform(ctx, action, nil); err == nil {
		t.Fatal("expected perform to fail while taken over")
	}

	if err := runtime.Return(ctx); err != nil {
		t.Fatalf("return: %v", err)
	}

	if _, err := runtime.Perform(ctx, action, nil); err == nil {
		t.Fatal("expected pre-takeover action to remain stale after return")
	}
	fresh, err := runtime.Observe(ctx, ObserveOptions{})
	if err != nil {
		t.Fatalf("fresh observe after return: %v", err)
	}
	freshAction, err := runtime.Prepare(ctx, fresh, ActionSpec{Kind: ActionWait, Intent: "wait again"})
	if err != nil {
		t.Fatalf("prepare after return: %v", err)
	}
	if _, err := runtime.Perform(ctx, freshAction, nil); err != nil {
		t.Fatalf("perform fresh action after return: %v", err)
	}
}

func TestRuntimeSharedLeaseBlocksSecondRun(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	firstBinding, _ := NewBinding("run-1", "thread-1", "agent-1", "host://local/desktop", policy)
	secondBinding, _ := NewBinding("run-2", "thread-2", "agent-2", "host://local/desktop", policy)
	leases := NewLeaseManager()
	first := NewRuntimeWithLeaseManager(firstBinding, &fakeDriver{healthy: true}, leases)
	second := NewRuntimeWithLeaseManager(secondBinding, &fakeDriver{healthy: true}, leases)

	if _, err := first.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open first session: %v", err)
	}
	if _, err := second.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err == nil {
		t.Fatal("expected second run to be rejected while desktop lease is held")
	}
	if err := first.Close(ctx, "done"); err != nil {
		t.Fatalf("close first session: %v", err)
	}
	if _, err := second.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"}); err != nil {
		t.Fatalf("open second session after release: %v", err)
	}
}

func TestRuntimeRecordsComputerFacts(t *testing.T) {
	ctx := context.Background()
	policy, _ := NewPolicySnapshot(DefaultPolicy())
	bind, _ := NewBinding("run-1", "thread-1", "agent-1", "", policy)
	fake := &fakeDriver{healthy: true}
	runtime := NewRuntime(bind, fake)

	var recordedKinds []string
	runtime.SetRecordFact(func(ctx context.Context, kind string, payload map[string]any) (string, error) {
		recordedKinds = append(recordedKinds, kind)
		return "rec_" + kind, nil
	})

	sess, err := runtime.OpenSession(ctx, CaptureWindow, TargetRef{AppName: "FakeApp"})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	obs, err := runtime.Observe(ctx, ObserveOptions{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	action, err := runtime.Prepare(ctx, obs, ActionSpec{Kind: ActionWait, Intent: "wait"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := runtime.Perform(ctx, action, nil); err != nil {
		t.Fatalf("perform: %v", err)
	}
	if err := runtime.Takeover(ctx); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if err := runtime.Return(ctx); err != nil {
		t.Fatalf("return: %v", err)
	}
	if err := runtime.Close(ctx, "test_done"); err != nil {
		t.Fatalf("close: %v", err)
	}

	want := []string{
		computerSessionOpenedKind,
		computerObservationCapturedKind,
		computerActionPreparedKind,
		computerActionStartedKind,
		computerActionCompletedKind,
		computerStateChangedKind,
		computerControlTransferStartedKind,
		computerStateChangedKind,
		computerControlTransferCompletedKind,
		computerSessionClosedKind,
	}
	if len(recordedKinds) != len(want) {
		t.Fatalf("expected %d facts, got %d: %v", len(want), len(recordedKinds), recordedKinds)
	}
	for i, kind := range want {
		if recordedKinds[i] != kind {
			t.Fatalf("fact %d: expected %s, got %s", i, kind, recordedKinds[i])
		}
	}

	// Close should clear the session and a second close is a no-op.
	if err := runtime.Close(ctx, "again"); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, active := runtime.Session(); active {
		t.Fatal("expected session to be closed")
	}

	_ = sess.SessionID
}
