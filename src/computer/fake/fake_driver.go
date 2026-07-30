// Package fake provides a programmable fake Computer Driver for domain and
// executor conformance tests. It intentionally does not depend on CUA.
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"awaken/src/computer"
)

// Driver is a thread-safe fake that records calls and returns programmed
// responses. It validates fence/epoch monotonicity.
type Driver struct {
	mu sync.RWMutex

	healthy         bool
	version         string
	protocolVersion int
	os              string

	sessions     map[computer.SessionID]*sessionRecord
	observations map[computer.ObservationID]computer.Observation
	actions      map[computer.ActionID]computer.PreparedAction

	// Programming hooks.
	OnOpenSession  func(bind computer.Binding, proposed computer.Session, policy computer.PolicySnapshot, fence computer.FenceToken) (computer.Session, error)
	OnObserve      func(sessionID computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, opts computer.ObserveOptions) (computer.Observation, error)
	OnPrepare      func(sessionID computer.SessionID, obs computer.Observation, spec computer.ActionSpec) (computer.PreparedAction, error)
	OnPerform      func(sessionID computer.SessionID, action computer.PreparedAction, fence computer.FenceToken, epoch computer.ControlEpoch) (computer.ActionResult, error)
	OnQuiesce      func(sessionID computer.SessionID, nextEpoch computer.ControlEpoch) error
	OnResume       func(sessionID computer.SessionID, policy computer.PolicySnapshot, fence computer.FenceToken, epoch computer.ControlEpoch) (computer.Session, error)
	OnCloseSession func(sessionID computer.SessionID, reason string) error

	Calls []CallRecord
}

type sessionRecord struct {
	session computer.Session
	policy  computer.PolicySnapshot
	bind    computer.Binding
}

// CallRecord records a single driver invocation.
type CallRecord struct {
	Method    string    `json:"method"`
	SessionID string    `json:"session_id,omitempty"`
	At        time.Time `json:"at"`
}

// NewDriver creates a healthy fake driver.
func NewDriver() *Driver {
	return &Driver{
		healthy:         true,
		version:         "fake-0.1.0",
		protocolVersion: 1,
		os:              "fakeos",
		sessions:        make(map[computer.SessionID]*sessionRecord),
		observations:    make(map[computer.ObservationID]computer.Observation),
		actions:         make(map[computer.ActionID]computer.PreparedAction),
	}
}

// SetHealthy controls the health check result.
func (d *Driver) SetHealthy(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.healthy = v
}

func (d *Driver) record(method string, sessionID computer.SessionID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Calls = append(d.Calls, CallRecord{Method: method, SessionID: string(sessionID), At: time.Now().UTC()})
}

func (d *Driver) Health(ctx context.Context) (computer.HealthResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return computer.HealthResult{
		Healthy:            d.healthy,
		Version:            d.version,
		ProtocolVersion:    d.protocolVersion,
		OS:                 d.os,
		TCCScreenRecording: true,
		TCCAccessibility:   true,
		Message:            "fake driver",
	}, nil
}

func (d *Driver) OpenSession(ctx context.Context, bind computer.Binding, proposed computer.Session, policy computer.PolicySnapshot, fence computer.FenceToken) (computer.Session, error) {
	d.record("OpenSession", "")
	if d.OnOpenSession != nil {
		return d.OnOpenSession(bind, proposed, policy, fence)
	}
	if !d.healthy {
		return computer.Session{}, fmt.Errorf("fake driver is unhealthy")
	}
	sess := proposed.Clone()
	sess.HostSessionRef = "host-" + string(sess.SessionID)
	if err := sess.MarkReady(); err != nil {
		return computer.Session{}, err
	}
	d.mu.Lock()
	d.sessions[sess.SessionID] = &sessionRecord{session: sess, policy: policy, bind: bind}
	d.mu.Unlock()
	return sess, nil
}

func (d *Driver) Observe(ctx context.Context, sessionID computer.SessionID, scope computer.CaptureScope, target computer.TargetRef, opts computer.ObserveOptions) (computer.Observation, error) {
	d.record("Observe", sessionID)
	if d.OnObserve != nil {
		return d.OnObserve(sessionID, scope, target, opts)
	}
	d.mu.RLock()
	rec, ok := d.sessions[sessionID]
	d.mu.RUnlock()
	if !ok {
		return computer.Observation{}, fmt.Errorf("session %s not found", sessionID)
	}
	now := time.Now().UTC()
	obs, err := computer.NewObservation(rec.session, scope, target, computer.Freshness{CapturedAt: now, FreshUntil: now.Add(30 * time.Second)})
	if err != nil {
		return computer.Observation{}, err
	}
	obs.Screenshot = &computer.MediaRef{StorageKey: "fake://screenshot/" + string(obs.ObservationID), MIMEType: "image/png", ByteSize: 1024, SHA256: "0000"}
	if opts.IncludeAccessibility {
		obs.AccessibilityTree = &computer.MediaRef{StorageKey: "fake://ax/" + string(obs.ObservationID), MIMEType: "application/json", ByteSize: 512, SHA256: "0001"}
	}
	d.mu.Lock()
	d.observations[obs.ObservationID] = obs
	rec.session.LastObservation = &obs.ObservationID
	d.mu.Unlock()
	return obs, nil
}

func (d *Driver) Prepare(ctx context.Context, sessionID computer.SessionID, obs computer.Observation, spec computer.ActionSpec) (computer.PreparedAction, error) {
	d.record("Prepare", sessionID)
	if d.OnPrepare != nil {
		return d.OnPrepare(sessionID, obs, spec)
	}
	d.mu.RLock()
	rec, ok := d.sessions[sessionID]
	d.mu.RUnlock()
	if !ok {
		return computer.PreparedAction{}, fmt.Errorf("session %s not found", sessionID)
	}
	risk := computer.RiskForAction(rec.bind, spec.Kind, spec.Target, spec.Intent, spec.Parameters)
	expires := time.Now().UTC().Add(5 * time.Minute)
	prepared, err := computer.PrepareAction(rec.session, obs, spec, risk, expires)
	if err != nil {
		return computer.PreparedAction{}, err
	}
	d.mu.Lock()
	d.actions[prepared.ActionID] = prepared
	d.mu.Unlock()
	return prepared, nil
}

func (d *Driver) PerformChecked(ctx context.Context, sessionID computer.SessionID, action computer.PreparedAction, fence computer.FenceToken, epoch computer.ControlEpoch) (computer.ActionResult, error) {
	d.record("PerformChecked", sessionID)
	if d.OnPerform != nil {
		return d.OnPerform(sessionID, action, fence, epoch)
	}
	d.mu.RLock()
	rec, ok := d.sessions[sessionID]
	d.mu.RUnlock()
	if !ok {
		return computer.ActionResult{}, fmt.Errorf("session %s not found", sessionID)
	}
	if fence != rec.session.FenceToken {
		return computer.ActionResult{}, fmt.Errorf("stale fence")
	}
	if epoch != rec.session.ControlEpoch {
		return computer.ActionResult{}, fmt.Errorf("stale epoch")
	}
	res := computer.NewActionResult(action.ActionID, sessionID, computer.ActionSucceeded)
	res.HostRequestID = computer.HostRequestID("req-" + string(action.ActionID))
	return res, nil
}

func (d *Driver) Quiesce(ctx context.Context, sessionID computer.SessionID, nextEpoch computer.ControlEpoch) (err error) {
	d.record("Quiesce", sessionID)
	if d.OnQuiesce != nil {
		return d.OnQuiesce(sessionID, nextEpoch)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if nextEpoch <= rec.session.ControlEpoch {
		return fmt.Errorf("next epoch must be greater than current epoch")
	}
	rec.session.ControlEpoch = nextEpoch
	_ = rec.session.Transition(computer.SessionTakenOver)
	return nil
}

func (d *Driver) Resume(ctx context.Context, sessionID computer.SessionID, policy computer.PolicySnapshot, fence computer.FenceToken, epoch computer.ControlEpoch) (computer.Session, error) {
	d.record("Resume", sessionID)
	if d.OnResume != nil {
		return d.OnResume(sessionID, policy, fence, epoch)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.sessions[sessionID]
	if !ok {
		return computer.Session{}, fmt.Errorf("session %s not found", sessionID)
	}
	rec.policy = policy
	rec.session.FenceToken = fence
	rec.session.ControlEpoch = epoch
	_ = rec.session.Transition(computer.SessionResuming)
	_ = rec.session.Transition(computer.SessionReady)
	return rec.session, nil
}

func (d *Driver) CloseSession(ctx context.Context, sessionID computer.SessionID, reason string) error {
	d.record("CloseSession", sessionID)
	if d.OnCloseSession != nil {
		return d.OnCloseSession(sessionID, reason)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, ok := d.sessions[sessionID]
	if !ok {
		return nil
	}
	_ = rec.session.Transition(computer.SessionClosing)
	_ = rec.session.Transition(computer.SessionClosed)
	delete(d.sessions, sessionID)
	return nil
}

var _ computer.Driver = (*Driver)(nil)
