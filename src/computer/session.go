package computer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Session is the runtime handle for an open Computer session. It carries the
// current state, target generation and control epoch. The CUA opaque handle is
// intentionally not exposed; Awaken IDs are the durable association keys.
type Session struct {
	SessionID        SessionID        `json:"session_id"`
	BindingID        BindingID        `json:"binding_id"`
	HostSessionRef   string           `json:"host_session_ref,omitempty"`
	CaptureScope     CaptureScope     `json:"capture_scope"`
	Target           TargetRef        `json:"target"`
	State            SessionState     `json:"state"`
	FenceToken       FenceToken       `json:"fence_token"`
	ControlEpoch     ControlEpoch     `json:"control_epoch"`
	PolicySnapshotID PolicySnapshotID `json:"policy_snapshot_id"`
	DriverVersion    string           `json:"driver_version"`
	HostVersion      string           `json:"host_version"`
	LastObservation  *ObservationID   `json:"last_observation,omitempty"`
	LastAction       *ActionID        `json:"last_action,omitempty"`
	HeartbeatAt      time.Time        `json:"heartbeat_at"`
	CreatedAt        time.Time        `json:"created_at"`
	ClosedAt         *time.Time       `json:"closed_at,omitempty"`
	CloseReason      string           `json:"close_reason,omitempty"`
}

// NewSession creates an opening session bound to a binding.
func NewSession(bind Binding, scope CaptureScope, target TargetRef) (Session, error) {
	if bind.BindingID == "" {
		return Session{}, fmt.Errorf("binding is required")
	}
	if err := ValidCaptureScope(scope, false); err != nil {
		return Session{}, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Session{}, fmt.Errorf("generate session id: %w", err)
	}
	now := time.Now().UTC()
	return Session{
		SessionID:        SessionID("sess_" + hex.EncodeToString(id)),
		BindingID:        bind.BindingID,
		CaptureScope:     scope,
		Target:           target,
		State:            SessionOpening,
		FenceToken:       bind.FenceToken,
		ControlEpoch:     bind.ControlEpoch,
		PolicySnapshotID: bind.PolicySnapshotID,
		DriverVersion:    bind.DriverVersion,
		HostVersion:      bind.HostVersion,
		HeartbeatAt:      now,
		CreatedAt:        now,
	}, nil
}

// Clone returns a deep copy.
func (s Session) Clone() Session {
	clone := s
	return clone
}

// Transition moves the session to a new state, validating the allowed edges.
// Unknown states and illegal transitions fail closed.
func (s *Session) Transition(to SessionState) error {
	if s == nil {
		return fmt.Errorf("nil session")
	}
	if IsTerminalState(s.State) && !IsTerminalState(to) {
		return fmt.Errorf("cannot transition terminal session %s from %s to %s", s.SessionID, s.State, to)
	}
	allowed := allowedTransitions(s.State)
	for _, candidate := range allowed {
		if candidate == to {
			s.State = to
			now := time.Now().UTC()
			s.HeartbeatAt = now
			if IsTerminalState(to) && s.ClosedAt == nil {
				s.ClosedAt = &now
			}
			return nil
		}
	}
	return fmt.Errorf("invalid session transition: %s -> %s", s.State, to)
}

// allowedTransitions encodes the state machine from the target design.
func allowedTransitions(from SessionState) []SessionState {
	switch from {
	case SessionOpening:
		return []SessionState{SessionReady, SessionFailed}
	case SessionReady:
		return []SessionState{SessionQuiescing, SessionReconciling, SessionClosing}
	case SessionQuiescing:
		return []SessionState{SessionPaused, SessionTakenOver, SessionReady, SessionNeedsUser, SessionClosing}
	case SessionPaused:
		return []SessionState{SessionResuming, SessionClosing}
	case SessionTakenOver:
		return []SessionState{SessionResuming, SessionClosing}
	case SessionResuming:
		return []SessionState{SessionReady, SessionNeedsUser, SessionFailed}
	case SessionReconciling:
		return []SessionState{SessionReady, SessionNeedsUser}
	case SessionNeedsUser:
		return []SessionState{SessionClosing}
	case SessionClosing:
		return []SessionState{SessionClosed}
	case SessionFailed:
		return []SessionState{SessionClosed}
	case SessionClosed:
		return nil
	default:
		return nil
	}
}

// RequireState returns an error if the session is not in one of the required
// states.
func (s Session) RequireState(required ...SessionState) error {
	for _, r := range required {
		if s.State == r {
			return nil
		}
	}
	return fmt.Errorf("session %s in state %s, required one of %v", s.SessionID, s.State, required)
}

// MarkReady transitions an opening session to ready.
func (s *Session) MarkReady() error {
	return s.Transition(SessionReady)
}

// MarkQuiescing begins takeover or safe pause.
func (s *Session) MarkQuiescing() error {
	return s.Transition(SessionQuiescing)
}

// MarkTakenOver confirms the user has control.
func (s *Session) MarkTakenOver() error {
	return s.Transition(SessionTakenOver)
}

// MarkPaused confirms that Agent capture and input are quiesced without
// transferring control ownership to the user.
func (s *Session) MarkPaused() error {
	return s.Transition(SessionPaused)
}

// MarkResuming begins return-to-Agent flow.
func (s *Session) MarkResuming() error {
	return s.Transition(SessionResuming)
}

// MarkClosed closes the session with a reason.
func (s *Session) MarkClosed(reason string) error {
	if reason == "" {
		reason = "closed"
	}
	s.CloseReason = reason
	return s.Transition(SessionClosing)
}

// UpdateTarget increments the target generation and records the new target.
// Old observations and prepared actions become invalid.
func (s *Session) UpdateTarget(target TargetRef) {
	s.Target = target
	s.Target.Generation++
}
