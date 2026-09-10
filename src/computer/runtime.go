package computer

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// RecordComputerFactFunc records a durable Computer fact. The runtime layer
// provides the implementation so the computer package stays free of agentcore
// imports and avoids a circular dependency.
type RecordComputerFactFunc func(ctx context.Context, kind string, payload map[string]any) (recordID string, err error)

// Semantic kind constants for Computer Use journal records. They must stay in
// sync with agentcore.SemanticKindComputer* constants.
const (
	computerSessionOpenedKind            = "computer_session_opened"
	computerStateChangedKind             = "computer_state_changed"
	computerObservationCapturedKind      = "computer_observation_captured"
	computerActionPreparedKind           = "computer_action_prepared"
	computerReviewRequestedKind          = "computer_review_requested"
	computerReviewResolvedKind           = "computer_review_resolved"
	computerActionStartedKind            = "computer_action_started"
	computerActionCompletedKind          = "computer_action_completed"
	computerControlTransferStartedKind   = "computer_control_transfer_started"
	computerControlTransferCompletedKind = "computer_control_transfer_completed"
	computerSessionClosedKind            = "computer_session_closed"
	computerMediaAccessedKind            = "computer_media_accessed"
	computerVerificationCompletedKind    = "computer_verification_completed"
)

// Runtime is a run-scoped Computer coordinator. It owns the binding, session,
// observations, lease and driver interactions for a single root run. It is not
// global state.
type Runtime struct {
	mu              sync.RWMutex
	operationMu     sync.Mutex
	operationID     uint64
	activeOperation context.CancelFunc
	stopped         bool
	bind            Binding
	driver          Driver
	lease           *LeaseManager
	session         *Session
	observations    map[ObservationID]*Observation
	reviews         map[ReviewID]*Review
	reviewWaiters   map[ReviewID]chan struct{}
	returnWaiter    chan struct{}
	actionResults   map[ActionID]ActionResult
	preparedActions map[ActionID]PreparedAction
	recordFact      RecordComputerFactFunc
	mediaVault      MediaVault
}

const computerUseStoppedMessage = "computer use was stopped by the user for this run; continue without computer tools"

func (r *Runtime) beginDriverOperation(ctx context.Context) (context.Context, func(), error) {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()
	if r.stopped {
		return nil, nil, fmt.Errorf("%s", computerUseStoppedMessage)
	}
	r.operationID++
	operationID := r.operationID
	operationCtx, cancel := context.WithCancel(ctx)
	r.activeOperation = cancel
	return operationCtx, func() {
		cancel()
		r.operationMu.Lock()
		if r.operationID == operationID {
			r.activeOperation = nil
		}
		r.operationMu.Unlock()
	}, nil
}

func (r *Runtime) cancelActiveDriverOperation(stop bool) {
	r.operationMu.Lock()
	if stop {
		r.stopped = true
	}
	cancel := r.activeOperation
	r.operationMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Runtime) noActiveSessionError() error {
	r.operationMu.Lock()
	defer r.operationMu.Unlock()
	if r.stopped {
		return fmt.Errorf("%s", computerUseStoppedMessage)
	}
	return fmt.Errorf("no active session")
}

// NewRuntime creates a run-bound Computer runtime. A nil driver defaults to the
// unavailable driver.
func NewRuntime(bind Binding, driver Driver) *Runtime {
	if driver == nil {
		driver = NewUnavailableDriver()
	}
	return &Runtime{
		bind:            bind,
		driver:          driver,
		lease:           NewLeaseManager(),
		observations:    make(map[ObservationID]*Observation),
		reviews:         make(map[ReviewID]*Review),
		reviewWaiters:   make(map[ReviewID]chan struct{}),
		actionResults:   make(map[ActionID]ActionResult),
		preparedActions: make(map[ActionID]PreparedAction),
	}
}

// NewRuntimeWithLeaseManager creates a run-bound runtime that shares a
// process-scoped lease manager with other Computer runtimes. A shared manager
// is required to enforce exclusive control of a physical desktop across runs.
func NewRuntimeWithLeaseManager(bind Binding, driver Driver, lease *LeaseManager) *Runtime {
	runtime := NewRuntime(bind, driver)
	if lease != nil {
		runtime.lease = lease
	}
	return runtime
}

// SetRecordFact installs the semantic-journal callback. It must be called before
// the first Computer fact is recorded; for formal runs RunService sets it after
// creating the runtime and binding the session recorder.
func (r *Runtime) SetRecordFact(fn RecordComputerFactFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordFact = fn
}

// SetMediaVault installs the session media store used for screenshots and
// accessibility trees. It must be set before Observe stores media.
func (r *Runtime) SetMediaVault(vault MediaVault) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mediaVault = vault
}

// recordComputerFact writes a durable fact when the caller already holds the
// runtime lock. I/O happens under the lock in this phase; a later pass can
// refine the locking if contention becomes measurable.
func (r *Runtime) recordComputerFact(ctx context.Context, kind string, payload map[string]any) error {
	if r.recordFact == nil || ctx == nil {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	_, err := r.recordFact(ctx, kind, payload)
	return err
}

// Binding returns the runtime's immutable binding.
func (r *Runtime) Binding() Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bind.Clone()
}

// Driver exposes the underlying driver for tests.
func (r *Runtime) Driver() Driver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.driver
}

// Ready reports whether the bound driver can create a usable Computer session.
// It is evaluated at the run availability snapshot boundary; an unhealthy
// driver keeps the Computer tools out of that pinned snapshot.
func (r *Runtime) Ready(ctx context.Context) bool {
	r.mu.RLock()
	driver := r.driver
	r.mu.RUnlock()
	if driver == nil {
		return false
	}
	health, err := driver.Health(ctx)
	return err == nil && health.Healthy && health.TCCScreenRecording && health.TCCAccessibility
}

// SetDriver replaces the driver (e.g. after Host restart). It increments the
// fence so any in-flight authorizations are invalidated.
func (r *Runtime) SetDriver(driver Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.driver = driver
	r.bind = r.bind.NextFence()
}

// OpenSession creates the first session and acquires the resource lease.
func (r *Runtime) OpenSession(ctx context.Context, scope CaptureScope, target TargetRef) (Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	operationCtx, finishOperation, err := r.beginDriverOperation(ctx)
	if err != nil {
		return Session{}, err
	}
	defer finishOperation()
	if r.session != nil && !IsTerminalState(r.session.State) {
		return Session{}, fmt.Errorf("session %s already active", r.session.SessionID)
	}
	// A new session invalidates all previous observations.
	r.observations = make(map[ObservationID]*Observation)
	if !r.bind.ScopeAllowed(scope) {
		return Session{}, fmt.Errorf("scope %s not allowed by binding", scope)
	}
	sess, err := NewSession(r.bind, scope, target)
	if err != nil {
		return Session{}, err
	}
	var lease ResourceLease
	if r.bind.WaitForResource {
		lease, err = r.lease.AcquireContext(operationCtx, r.bind.ResourceKey, r.bind, sess.SessionID, string(r.bind.RootAgentID), r.bind.MaxDuration())
	} else {
		lease, err = r.lease.Acquire(r.bind.ResourceKey, r.bind, sess.SessionID, string(r.bind.RootAgentID), r.bind.MaxDuration())
	}
	if err != nil {
		return Session{}, fmt.Errorf("acquire resource lease: %w", err)
	}
	r.bind.FenceToken = lease.FenceToken
	sess.FenceToken = lease.FenceToken
	opened, err := r.driver.OpenSession(operationCtx, r.bind, sess, PolicySnapshot{ID: r.bind.PolicySnapshotID}, r.bind.FenceToken)
	if err != nil {
		_ = r.lease.Release(r.bind.ResourceKey, r.bind.BindingID)
		return Session{}, err
	}
	// Preserve Awaken IDs; copy driver-provided host ref and any state updates.
	sess.HostSessionRef = opened.HostSessionRef
	if err := sess.MarkReady(); err != nil {
		_ = r.driver.CloseSession(ctx, sess.SessionID, "open_failed")
		_ = r.lease.Release(r.bind.ResourceKey, r.bind.BindingID)
		return Session{}, err
	}
	r.session = &sess
	if err := r.recordComputerFact(ctx, computerSessionOpenedKind, map[string]any{
		"session_id":         string(sess.SessionID),
		"binding_id":         string(r.bind.BindingID),
		"environment_id":     r.bind.EnvironmentID,
		"resource_key":       r.bind.ResourceKey,
		"policy_snapshot_id": string(r.bind.PolicySnapshotID),
		"capture_scope":      string(sess.CaptureScope),
		"host_session_ref":   sess.HostSessionRef,
	}); err != nil {
		_ = r.driver.CloseSession(ctx, sess.SessionID, "journal_failed")
		_ = r.lease.Release(r.bind.ResourceKey, r.bind.BindingID)
		r.session = nil
		return Session{}, fmt.Errorf("record computer session open: %w", err)
	}
	return sess, nil
}

// Observe captures a fresh observation. If opts.Scope is set and allowed by
// the binding, it overrides the session's default scope.
func (r *Runtime) Observe(ctx context.Context, opts ObserveOptions) (Observation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil {
		return Observation{}, r.noActiveSessionError()
	}
	if err := r.session.RequireState(SessionReady); err != nil {
		return Observation{}, err
	}
	scope := r.session.CaptureScope
	if opts.Scope != "" {
		if !r.bind.ScopeAllowed(opts.Scope) {
			return Observation{}, fmt.Errorf("scope %s not allowed by binding", opts.Scope)
		}
		scope = opts.Scope
	}
	operationCtx, finishOperation, err := r.beginDriverOperation(ctx)
	if err != nil {
		return Observation{}, err
	}
	defer finishOperation()
	obs, err := r.driver.Observe(operationCtx, r.session.SessionID, scope, r.session.Target, opts)
	if err != nil {
		return Observation{}, err
	}
	r.session.LastObservation = &obs.ObservationID
	r.observations[obs.ObservationID] = &obs
	if err := r.recordComputerFact(ctx, computerObservationCapturedKind, map[string]any{
		"observation_id":    string(obs.ObservationID),
		"session_id":        string(obs.SessionID),
		"scope":             string(obs.Scope),
		"target_app_name":   obs.Target.AppName,
		"target_pid":        obs.Target.PID,
		"target_generation": obs.Target.Generation,
		"has_screenshot":    obs.Screenshot != nil,
		"has_tree":          obs.AccessibilityTree != nil,
	}); err != nil {
		delete(r.observations, obs.ObservationID)
		r.session.LastObservation = nil
		return Observation{}, fmt.Errorf("record computer observation: %w", err)
	}
	return obs, nil
}

// ReadMedia validates and returns session media bytes, recording an audit fact
// when a semantic-journal callback is installed.
func (r *Runtime) ReadMedia(ctx context.Context, ref MediaRef) ([]byte, error) {
	r.mu.RLock()
	vault := r.mediaVault
	var sessionID SessionID
	if r.session != nil {
		sessionID = r.session.SessionID
	}
	r.mu.RUnlock()

	if sessionID == "" {
		return nil, r.noActiveSessionError()
	}

	if vault == nil {
		// Fallback for tests and unvaulted drivers: read the storage key as a
		// local path.
		return os.ReadFile(ref.StorageKey)
	}
	data, err := vault.Read(ctx, sessionID, ref)
	if err != nil {
		return nil, err
	}
	_ = r.recordComputerFact(ctx, computerMediaAccessedKind, map[string]any{
		"session_id":  string(sessionID),
		"storage_key": ref.StorageKey,
		"sha256":      ref.SHA256,
		"byte_size":   ref.ByteSize,
	})
	return data, nil
}

// GetObservation returns a stored observation if it is still valid for the
// current session state. Actions must be bound to observations returned by this
// method.
func (r *Runtime) GetObservation(id ObservationID) (Observation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.session == nil {
		return Observation{}, r.noActiveSessionError()
	}
	if IsTerminalState(r.session.State) {
		return Observation{}, fmt.Errorf("session %s is in terminal state %s", r.session.SessionID, r.session.State)
	}
	obs, ok := r.observations[id]
	if !ok {
		return Observation{}, fmt.Errorf("observation %s not found", id)
	}
	if err := obs.CanAct(*r.session); err != nil {
		return Observation{}, fmt.Errorf("observation %s is no longer valid: %w", id, err)
	}
	return *obs, nil
}

// Prepare evaluates policy and returns a prepared action.
func (r *Runtime) Prepare(ctx context.Context, obs Observation, spec ActionSpec) (PreparedAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ValidateActionSpec(spec); err != nil {
		return PreparedAction{}, fmt.Errorf("invalid action parameters: %w", err)
	}
	if r.session == nil {
		return PreparedAction{}, r.noActiveSessionError()
	}
	if err := r.session.RequireState(SessionReady); err != nil {
		return PreparedAction{}, err
	}
	expectedRisk := RiskForAction(r.bind, spec.Kind, spec.Target, spec.Intent, spec.Parameters)
	operationCtx, finishOperation, err := r.beginDriverOperation(ctx)
	if err != nil {
		return PreparedAction{}, err
	}
	defer finishOperation()
	prepared, err := r.driver.Prepare(operationCtx, r.session.SessionID, obs, spec)
	if err != nil {
		return PreparedAction{}, err
	}
	if prepared.Risk != expectedRisk {
		return PreparedAction{}, fmt.Errorf("driver risk decision %s does not match runtime policy %s", prepared.Risk, expectedRisk)
	}
	if err := prepared.ValidForSession(*r.session, time.Now().UTC()); err != nil {
		return PreparedAction{}, fmt.Errorf("driver returned invalid prepared action: %w", err)
	}
	if err := r.recordComputerFact(ctx, computerActionPreparedKind, map[string]any{
		"action_id":         string(prepared.ActionID),
		"observation_id":    string(prepared.ObservationID),
		"session_id":        string(prepared.SessionID),
		"binding_id":        string(prepared.BindingID),
		"action":            string(prepared.Kind),
		"risk":              string(prepared.Risk),
		"requires_review":   prepared.RequiresReview,
		"action_digest":     prepared.ActionDigest,
		"target_generation": prepared.TargetGeneration,
	}); err != nil {
		return PreparedAction{}, fmt.Errorf("record computer action preparation: %w", err)
	}
	r.preparedActions[prepared.ActionID] = prepared
	return prepared, nil
}

// Perform executes a prepared action after optional review.
func (r *Runtime) Perform(ctx context.Context, action PreparedAction, review *Review) (ActionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil {
		return ActionResult{}, r.noActiveSessionError()
	}
	if r.session.State == SessionTakenOver {
		return ActionResult{}, fmt.Errorf("session is under user control; return control before performing actions")
	}
	if err := r.session.RequireState(SessionReady); err != nil {
		return ActionResult{}, err
	}
	if err := action.ValidForSession(*r.session, time.Now().UTC()); err != nil {
		return ActionResult{}, err
	}
	if action.RequiresReview {
		if review == nil {
			return ActionResult{}, fmt.Errorf("action %s requires review", action.ActionID)
		}
		if err := review.ValidateForAction(action, time.Now().UTC()); err != nil {
			return ActionResult{}, err
		}
	}
	if err := r.lease.ValidateFence(r.bind.ResourceKey, action.FenceToken); err != nil {
		return ActionResult{}, err
	}
	if previous, ok := r.actionResults[action.ActionID]; ok {
		return previous, nil
	}
	started := map[string]any{
		"action_id":      string(action.ActionID),
		"observation_id": string(action.ObservationID),
		"session_id":     string(action.SessionID),
		"action":         string(action.Kind),
		"risk":           string(action.Risk),
		"has_review":     review != nil,
	}
	obs := r.observations[action.ObservationID]
	for key, value := range PointerHintForAction(action, obs).Payload() {
		started[key] = value
	}
	if err := r.recordComputerFact(ctx, computerActionStartedKind, started); err != nil {
		return ActionResult{}, fmt.Errorf("record computer action start: %w", err)
	}
	operationCtx, finishOperation, err := r.beginDriverOperation(ctx)
	if err != nil {
		return ActionResult{}, err
	}
	defer finishOperation()
	res, err := r.driver.PerformChecked(operationCtx, r.session.SessionID, action, r.session.FenceToken, r.session.ControlEpoch)
	status := string(res.Status)
	if status == "" {
		status = string(ActionSucceeded)
	}
	if err != nil {
		status = string(ActionUnknownOutcome)
	}
	completionError := errString(err)
	if completionError == "" {
		completionError = res.ErrorMessage
	}
	if recordErr := r.recordComputerFact(ctx, computerActionCompletedKind, map[string]any{
		"action_id":      string(action.ActionID),
		"observation_id": string(action.ObservationID),
		"session_id":     string(action.SessionID),
		"status":         status,
		"error":          completionError,
	}); recordErr != nil {
		return ActionResult{}, fmt.Errorf("record computer action completion: %w", recordErr)
	}
	if err != nil {
		return ActionResult{}, err
	}
	r.session.LastAction = &action.ActionID
	r.actionResults[action.ActionID] = res
	return res, nil
}

// AttachAfterObservation links the fresh post-action observation to the
// confirmed result. It never changes the action status or re-executes input.
func (r *Runtime) AttachAfterObservation(actionID ActionID, observationID ObservationID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	result, ok := r.actionResults[actionID]
	if !ok {
		return fmt.Errorf("action result %s not found", actionID)
	}
	if _, ok := r.observations[observationID]; !ok {
		return fmt.Errorf("observation %s not found", observationID)
	}
	result.AfterObservation = &observationID
	r.actionResults[actionID] = result
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Verify runs an independent verification against a post-action observation.
func (r *Runtime) Verify(ctx context.Context, actionID ActionID, obs Observation, assertions []Assertion) (VerificationResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.session == nil {
		return VerificationResult{}, r.noActiveSessionError()
	}
	if err := r.session.RequireState(SessionReady, SessionNeedsUser); err != nil {
		return VerificationResult{}, err
	}
	action, ok := r.preparedActions[actionID]
	if !ok {
		return VerificationResult{}, fmt.Errorf("prepared action %s not found", actionID)
	}
	result, ok := r.actionResults[actionID]
	if !ok {
		return VerificationResult{}, fmt.Errorf("action result %s not found", actionID)
	}
	if obs.SessionID != action.SessionID || obs.ObservationID == action.ObservationID {
		return VerificationResult{}, fmt.Errorf("verification requires a fresh post-action observation")
	}
	if result.Status == ActionUnknownOutcome {
		return VerificationResult{}, fmt.Errorf("action %s has unknown outcome and requires reconciliation", actionID)
	}
	verified := VerificationInconclusive
	evaluatedCount := 0
	matchedCount := 0
	for i := range assertions {
		assertions[i].Matched = assertions[i].Actual != "" && assertions[i].Expected == assertions[i].Actual
		if assertions[i].Actual != "" {
			evaluatedCount++
		}
		if assertions[i].Matched {
			matchedCount++
		}
	}
	if len(assertions) > 0 && evaluatedCount == len(assertions) {
		if matchedCount == len(assertions) {
			verified = VerificationVerified
		} else {
			verified = VerificationNotVerified
		}
	}
	res := NewVerificationResult(actionID, obs.ObservationID, r.session.SessionID, verified)
	res.Assertions = assertions
	if err := r.recordComputerFact(ctx, computerVerificationCompletedKind, map[string]any{
		"action_id":      string(actionID),
		"observation_id": string(obs.ObservationID),
		"session_id":     string(r.session.SessionID),
		"status":         string(res.Status),
		"assertions":     res.Assertions,
	}); err != nil {
		return VerificationResult{}, fmt.Errorf("record computer verification: %w", err)
	}
	return res, nil
}

func (r *Runtime) quiesce(ctx context.Context, target SessionState, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil {
		return r.noActiveSessionError()
	}
	previousState := r.session.State
	if err := r.session.MarkQuiescing(); err != nil {
		return err
	}
	_ = r.recordComputerFact(ctx, computerStateChangedKind, map[string]any{
		"session_id": string(r.session.SessionID),
		"from_state": string(previousState),
		"to_state":   string(r.session.State),
		"reason":     reason,
	})
	r.bind = r.bind.NextEpoch()
	operationCtx, finishOperation, err := r.beginDriverOperation(ctx)
	if err != nil {
		return err
	}
	defer finishOperation()
	if err := r.driver.Quiesce(operationCtx, r.session.SessionID, r.bind.ControlEpoch); err != nil {
		r.session.ControlEpoch = r.bind.ControlEpoch
		_ = r.session.Transition(SessionNeedsUser)
		_ = r.recordComputerFact(ctx, computerStateChangedKind, map[string]any{
			"session_id": string(r.session.SessionID),
			"from_state": string(SessionQuiescing),
			"to_state":   string(r.session.State),
			"reason":     "quiesce_failed",
			"error":      err.Error(),
		})
		return err
	}
	r.session.ControlEpoch = r.bind.ControlEpoch
	r.observations = make(map[ObservationID]*Observation)
	r.preparedActions = make(map[ActionID]PreparedAction)
	if err := r.session.Transition(target); err != nil {
		return err
	}
	eventKind := computerStateChangedKind
	if target == SessionTakenOver {
		eventKind = computerControlTransferStartedKind
		r.returnWaiter = make(chan struct{})
	}
	_ = r.recordComputerFact(ctx, eventKind, map[string]any{
		"session_id":    string(r.session.SessionID),
		"from_state":    string(SessionQuiescing),
		"to_state":      string(target),
		"reason":        reason,
		"control_epoch": r.bind.ControlEpoch.String(),
		"fence_token":   r.bind.FenceToken.String(),
	})
	return nil
}

// Pause drains in-flight input and stops Agent capture/input while preserving
// the current session for a later Return.
func (r *Runtime) Pause(ctx context.Context) error {
	return r.quiesce(ctx, SessionPaused, "pause")
}

// Takeover drains in-flight input and moves the session to user control.
func (r *Runtime) Takeover(ctx context.Context) error {
	return r.quiesce(ctx, SessionTakenOver, "takeover")
}

// Return returns control to the Agent after pause or takeover.
func (r *Runtime) Return(ctx context.Context) error {
	return r.ReturnToSession(ctx, "")
}

// ReturnToSession rejects stale handoffs before changing the current session.
// An empty ID preserves the legacy run-level control contract.
func (r *Runtime) ReturnToSession(ctx context.Context, expected SessionID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil {
		return r.noActiveSessionError()
	}
	if expected != "" && r.session.SessionID != expected {
		return fmt.Errorf("computer session %s is no longer active", expected)
	}
	previousState := r.session.State
	if err := r.session.MarkResuming(); err != nil {
		return err
	}
	_ = r.recordComputerFact(ctx, computerStateChangedKind, map[string]any{
		"session_id": string(r.session.SessionID),
		"from_state": string(previousState),
		"to_state":   string(r.session.State),
		"reason":     "return",
	})
	r.bind = r.bind.NextEpoch()
	operationCtx, finishOperation, err := r.beginDriverOperation(ctx)
	if err != nil {
		return err
	}
	defer finishOperation()
	resumed, err := r.driver.Resume(operationCtx, r.session.SessionID, PolicySnapshot{ID: r.bind.PolicySnapshotID}, r.bind.FenceToken, r.bind.ControlEpoch)
	if err != nil {
		r.session.ControlEpoch = r.bind.ControlEpoch
		_ = r.session.Transition(SessionNeedsUser)
		return err
	}
	r.session.ControlEpoch = resumed.ControlEpoch
	r.session.FenceToken = resumed.FenceToken
	r.session.Target = resumed.Target
	r.observations = make(map[ObservationID]*Observation)
	r.preparedActions = make(map[ActionID]PreparedAction)
	if err := r.session.MarkReady(); err != nil {
		return err
	}
	if r.returnWaiter != nil {
		close(r.returnWaiter)
		r.returnWaiter = nil
	}
	_ = r.recordComputerFact(ctx, computerControlTransferCompletedKind, map[string]any{
		"session_id":    string(r.session.SessionID),
		"control_epoch": r.bind.ControlEpoch.String(),
		"fence_token":   r.bind.FenceToken.String(),
	})
	return nil
}

// WaitForReturn blocks a handoff tool call until the user returns control.
func (r *Runtime) WaitForReturn(ctx context.Context) error {
	r.mu.RLock()
	if r.session == nil {
		r.mu.RUnlock()
		return r.noActiveSessionError()
	}
	if r.session.State == SessionReady {
		r.mu.RUnlock()
		return nil
	}
	if r.session.State != SessionTakenOver || r.returnWaiter == nil {
		state := r.session.State
		r.mu.RUnlock()
		return fmt.Errorf("session is not waiting for handoff return: %s", state)
	}
	waiter := r.returnWaiter
	r.mu.RUnlock()
	select {
	case <-waiter:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close terminates the session and releases the lease. It is used for normal
// run cleanup and does not by itself disable Computer Use for a still-running
// model turn.
func (r *Runtime) Close(ctx context.Context, reason string) error {
	r.cancelActiveDriverOperation(false)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeLocked(ctx, reason)
}

// Terminate stops Computer Use for the remainder of this run without
// cancelling the run itself. Any in-flight driver request is cancelled before
// waiting for the runtime lock, so the control request cannot deadlock behind
// a long GUI action.
func (r *Runtime) Terminate(ctx context.Context, reason string) error {
	r.cancelActiveDriverOperation(true)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeLocked(ctx, reason)
}

func (r *Runtime) closeLocked(ctx context.Context, reason string) error {
	if r.session == nil {
		return nil
	}
	sessionID := r.session.SessionID
	_ = r.session.MarkClosed(reason)
	_ = r.driver.CloseSession(ctx, sessionID, reason)
	_ = r.lease.Release(r.bind.ResourceKey, r.bind.BindingID)
	if r.mediaVault != nil {
		_ = r.mediaVault.PurgeSession(ctx, sessionID)
	}
	r.session = nil
	r.observations = make(map[ObservationID]*Observation)
	r.reviews = make(map[ReviewID]*Review)
	r.actionResults = make(map[ActionID]ActionResult)
	r.preparedActions = make(map[ActionID]PreparedAction)
	for reviewID, ch := range r.reviewWaiters {
		close(ch)
		delete(r.reviewWaiters, reviewID)
	}
	if r.returnWaiter != nil {
		close(r.returnWaiter)
		r.returnWaiter = nil
	}
	_ = r.recordComputerFact(ctx, computerSessionClosedKind, map[string]any{
		"session_id": string(sessionID),
		"reason":     reason,
	})
	return nil
}

// RequestReviewInput carries the prepared action and human-readable summaries
// needed to create a review.
type RequestReviewInput struct {
	Action        PreparedAction
	TargetSummary string
	EffectSummary string
	EvidenceRefs  []string
	TTL           time.Duration
}

// RequestReview creates a pending review for a prepared action and records the
// fact in the semantic journal. The returned review must be resolved via
// ResolveReview before Perform will accept the action.
func (r *Runtime) RequestReview(ctx context.Context, req RequestReviewInput) (*Review, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil {
		return nil, r.noActiveSessionError()
	}
	if err := r.session.RequireState(SessionReady); err != nil {
		return nil, err
	}
	if err := req.Action.ValidForSession(*r.session, time.Now().UTC()); err != nil {
		return nil, err
	}
	if req.TTL <= 0 {
		req.TTL = 5 * time.Minute
	}
	review, err := NewReview(req.Action, req.TargetSummary, req.EffectSummary, req.EvidenceRefs, req.TTL)
	if err != nil {
		return nil, err
	}
	r.reviews[review.ReviewID] = &review
	r.reviewWaiters[review.ReviewID] = make(chan struct{})
	if err := r.recordComputerFact(ctx, computerReviewRequestedKind, map[string]any{
		"review_id":      string(review.ReviewID),
		"action_id":      string(review.ActionID),
		"action_digest":  review.ActionDigest,
		"session_id":     string(review.SessionID),
		"binding_id":     string(review.BindingID),
		"observation_id": string(req.Action.ObservationID),
		"risk":           string(req.Action.Risk),
		"target_summary": review.TargetSummary,
		"effect_summary": review.EffectSummary,
		"evidence_refs":  review.EvidenceRefs,
		"expires_at":     review.ExpiresAt.Format(time.RFC3339Nano),
	}); err != nil {
		delete(r.reviews, review.ReviewID)
		delete(r.reviewWaiters, review.ReviewID)
		return nil, fmt.Errorf("record computer review request: %w", err)
	}
	return &review, nil
}

// ResolveReview applies a human decision to a pending review and records the
// resolution. Only pending reviews may be resolved.
func (r *Runtime) ResolveReview(ctx context.Context, reviewID ReviewID, decision ReviewDecision, actor, requestID string) (*Review, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session == nil {
		return nil, r.noActiveSessionError()
	}
	review, ok := r.reviews[reviewID]
	if !ok {
		return nil, fmt.Errorf("review %s not found", reviewID)
	}
	original := *review
	if err := review.Resolve(decision, actor, requestID, time.Now().UTC()); err != nil {
		return nil, err
	}
	if err := r.recordComputerFact(ctx, computerReviewResolvedKind, map[string]any{
		"review_id":                string(review.ReviewID),
		"action_id":                string(review.ActionID),
		"action_digest":            review.ActionDigest,
		"session_id":               string(review.SessionID),
		"decision":                 string(review.Decision),
		"actor":                    review.Actor,
		"control_route_request_id": review.ControlRouteRequestID,
		"decided_at":               review.DecidedAt.Format(time.RFC3339Nano),
	}); err != nil {
		*review = original
		return nil, fmt.Errorf("record computer review resolution: %w", err)
	}
	if ch, ok := r.reviewWaiters[reviewID]; ok {
		close(ch)
		delete(r.reviewWaiters, reviewID)
	}
	snapshot := *review
	return &snapshot, nil
}

// WaitForReview blocks until the review is resolved or the context is done.
// It returns the resolved review. If the review does not exist, it returns
// immediately with an error. If the runtime is closed while waiting, the
// context is cancelled or the channel is closed and WaitForReview returns an
// error.
func (r *Runtime) WaitForReview(ctx context.Context, reviewID ReviewID) (*Review, error) {
	r.mu.Lock()
	if r.session == nil {
		r.mu.Unlock()
		return nil, r.noActiveSessionError()
	}
	review, ok := r.reviews[reviewID]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("review %s not found", reviewID)
	}
	if !review.IsPending() {
		r.mu.Unlock()
		snapshot := *review
		return &snapshot, nil
	}
	ch, ok := r.reviewWaiters[reviewID]
	if !ok {
		ch = make(chan struct{})
		r.reviewWaiters[reviewID] = ch
	}
	r.mu.Unlock()

	select {
	case <-ch:
		r.mu.RLock()
		defer r.mu.RUnlock()
		resolved, ok := r.reviews[reviewID]
		if !ok {
			return nil, fmt.Errorf("review %s removed while waiting", reviewID)
		}
		snapshot := *resolved
		return &snapshot, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetReview returns a review snapshot if it exists in the runtime ledger.
func (r *Runtime) GetReview(reviewID ReviewID) (Review, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	review, ok := r.reviews[reviewID]
	if !ok {
		return Review{}, false
	}
	return *review, true
}

// Session returns the current session snapshot.
func (r *Runtime) Session() (Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.session == nil {
		return Session{}, false
	}
	return r.session.Clone(), true
}

// MaxDuration returns the binding's remaining lifetime.
func (b Binding) MaxDuration() time.Duration {
	return b.ExpiresAt.Sub(b.CreatedAt)
}
