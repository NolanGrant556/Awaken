// Package computer defines the model-independent domain for Awaken Computer Use.
// It owns binding, session, observation, action, review, lease/fence and the
// driver contract. CUA-specific concepts are isolated in adapters; this package
// is the authority for Awaken-level Computer facts.
package computer

import (
	"fmt"
	"strings"
	"time"
)

// IDs and opaque references.
type (
	BindingID        string
	SessionID        string
	ObservationID    string
	ActionID         string
	ReviewID         string
	HostRequestID    string
	PolicySnapshotID string
)

// Capability tokens.
type (
	// FenceToken is a monotonic lease boundary. A larger value always wins.
	FenceToken uint64
	// ControlEpoch identifies a distinct user/Agent control period. Takeover
	// and resume increase the epoch; old epochs must not perform input.
	ControlEpoch uint64
)

// CaptureScope defines the spatial boundary of a capture.
type CaptureScope string

const (
	// CaptureWindow is the default scope: a single authorized window with
	// window-local coordinates and (when supported) background delivery.
	CaptureWindow CaptureScope = "window"
	// CaptureDesktop is foreground-only, screen-absolute coordinates. It is an
	// explicit escalation that must be allowed by policy.
	CaptureDesktop CaptureScope = "desktop"
	// CaptureAuto is only used for probing/compat. A real session must record
	// the resolved immutable scope.
	CaptureAuto CaptureScope = "auto"
)

// ActionKind enumerates the high-level input actions the model may request.
type ActionKind string

const (
	ActionClick       ActionKind = "click"
	ActionDoubleClick ActionKind = "double_click"
	ActionScroll      ActionKind = "scroll"
	ActionDrag        ActionKind = "drag"
	ActionKey         ActionKind = "key"
	ActionTypeText    ActionKind = "type_text"
	ActionMoveCursor  ActionKind = "move_cursor"
	ActionHotkey      ActionKind = "hotkey"
	ActionWait        ActionKind = "wait"
)

// RiskLevel is the product-facing risk classification for an action.
type RiskLevel string

const (
	// RiskLow may execute directly inside the allowlist and is recorded.
	RiskLow RiskLevel = "low"
	// RiskReview requires explicit human approval of an immutable action digest.
	RiskReview RiskLevel = "review"
	// RiskHandoff pauses Agent capture/input and asks the user to complete the
	// step manually (secrets, identity, security authority).
	RiskHandoff RiskLevel = "handoff"
	// RiskForbidden must never execute, even if the user asks verbally.
	RiskForbidden RiskLevel = "forbidden"
)

// SessionState is the durable state of a Computer session.
type SessionState string

const (
	SessionOpening     SessionState = "opening"
	SessionReady       SessionState = "ready"
	SessionQuiescing   SessionState = "quiescing"
	SessionPaused      SessionState = "paused"
	SessionTakenOver   SessionState = "taken_over"
	SessionResuming    SessionState = "resuming"
	SessionReconciling SessionState = "reconciling"
	SessionNeedsUser   SessionState = "needs_user"
	SessionClosing     SessionState = "closing"
	SessionClosed      SessionState = "closed"
	SessionFailed      SessionState = "failed"
)

// ActionResultStatus distinguishes confirmed, failed and genuinely unknown
// outcomes. Unknown is the conservative default whenever input may have
// happened but confirmation was lost.
type ActionResultStatus string

const (
	ActionSucceeded              ActionResultStatus = "succeeded"
	ActionFailed                 ActionResultStatus = "failed"
	ActionUnknownOutcome         ActionResultStatus = "unknown_outcome"
	ActionCancelledBeforePerform ActionResultStatus = "cancelled_before_perform"
)

// VerificationStatus is the result of an independent verification pass.
type VerificationStatus string

const (
	VerificationVerified     VerificationStatus = "verified"
	VerificationNotVerified  VerificationStatus = "not_verified"
	VerificationInconclusive VerificationStatus = "inconclusive"
)

// ReviewDecision records how a review was resolved.
type ReviewDecision string

const (
	ReviewApprovedOnce ReviewDecision = "approved_once"
	ReviewDenied       ReviewDecision = "denied"
	ReviewExpired      ReviewDecision = "expired"
	ReviewCancelled    ReviewDecision = "cancelled"
)

// HealthResult reports driver/host readiness at a point in time.
type HealthResult struct {
	Healthy            bool   `json:"healthy"`
	Version            string `json:"version"`
	ProtocolVersion    int    `json:"protocol_version"`
	OS                 string `json:"os"`
	TCCScreenRecording bool   `json:"tcc_screen_recording"`
	TCCAccessibility   bool   `json:"tcc_accessibility"`
	Message            string `json:"message,omitempty"`
}

// TargetRef identifies an application/window within a session. It is scoped to
// a session and a monotonic generation; navigation, window replacement or
// process recycle invalidates older refs.
type TargetRef struct {
	PID             int32  `json:"pid"`
	AppBundleID     string `json:"app_bundle_id,omitempty"`
	AppName         string `json:"app_name"`
	WindowID        string `json:"window_id,omitempty"`
	WindowTitle     string `json:"window_title,omitempty"`
	Generation      uint64 `json:"generation"`
	CoordinateSpace string `json:"coordinate_space"`
}

// ElementRef binds an action target to a specific observation and session.
type ElementRef struct {
	ObservationID    string `json:"observation_id"`
	SessionID        string `json:"session_id"`
	TargetGeneration uint64 `json:"target_generation"`
	// Locator is preferred as an accessibility locator; PixelFallback is only
	// used when accessibility is insufficient.
	Locator       string `json:"locator,omitempty"`
	PixelFallback *Point `json:"pixel_fallback,omitempty"`
	ElementRole   string `json:"element_role,omitempty"`
	ElementLabel  string `json:"element_label,omitempty"`
}

// Point is a window-local or desktop pixel coordinate.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Rect is a window-local or desktop pixel rectangle.
type Rect struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

// MediaRef is a reference to screenshot/tree media stored under the computer
// policy namespace. It never contains media bytes.
type MediaRef struct {
	StorageKey string    `json:"storage_key"`
	MIMEType   string    `json:"mime_type"`
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
	ByteSize   int64     `json:"byte_size"`
	SHA256     string    `json:"sha256"`
	Redacted   bool      `json:"redacted"`
	SessionID  SessionID `json:"session_id,omitempty"`
}

// Freshness bounds how long an observation may be used as the basis for action.
type Freshness struct {
	CapturedAt time.Time `json:"captured_at"`
	FreshUntil time.Time `json:"fresh_until"`
}

// ValidCaptureScope rejects auto once a session has resolved its scope.
func ValidCaptureScope(s CaptureScope, resolved bool) error {
	switch s {
	case CaptureWindow, CaptureDesktop:
		return nil
	case CaptureAuto:
		if resolved {
			return fmt.Errorf("capture scope %q is only allowed during probing", s)
		}
		return nil
	default:
		return fmt.Errorf("unknown capture scope %q", s)
	}
}

// IsTerminalState reports whether a session may still accept new actions.
func IsTerminalState(s SessionState) bool {
	switch s {
	case SessionClosed, SessionFailed:
		return true
	default:
		return false
	}
}

// AllowsNewActions reports whether the session state permits prepare/perform.
func AllowsNewActions(s SessionState) bool {
	return s == SessionReady
}

// ActionSpec is the raw action request from the model. It is not yet bound to
// an immutable observation or evaluated by policy.
type ActionSpec struct {
	Kind            ActionKind     `json:"action"`
	Target          ElementRef     `json:"target"`
	Parameters      map[string]any `json:"parameters"`
	ExpectedContext map[string]any `json:"expected_context,omitempty"`
	Intent          string         `json:"intent"`
	ExpectedEffect  string         `json:"expected_effect"`
	IdempotencyKey  string         `json:"idempotency_key"`
}

// NewFenceToken returns the next fence token. Zero is reserved for "unset".
func NewFenceToken(current FenceToken) FenceToken {
	if current == 0 {
		return 1
	}
	return current + 1
}

// NewControlEpoch returns the next control epoch. Zero is reserved for "unset".
func NewControlEpoch(current ControlEpoch) ControlEpoch {
	if current == 0 {
		return 1
	}
	return current + 1
}

// String helpers.
func (t FenceToken) String() string   { return fmt.Sprintf("fence_%d", uint64(t)) }
func (e ControlEpoch) String() string { return fmt.Sprintf("epoch_%d", uint64(e)) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
