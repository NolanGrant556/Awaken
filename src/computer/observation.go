package computer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Observation is a durable, immutable snapshot of the authorized capture scope
// at a moment in time. All actions must be bound to an observation.
type Observation struct {
	ObservationID     ObservationID    `json:"observation_id"`
	SessionID         SessionID        `json:"session_id"`
	BindingID         BindingID        `json:"binding_id"`
	FenceToken        FenceToken       `json:"fence_token"`
	ControlEpoch      ControlEpoch     `json:"control_epoch"`
	Scope             CaptureScope     `json:"scope"`
	Target            TargetRef        `json:"target"`
	Freshness         Freshness        `json:"freshness"`
	Completeness      string           `json:"completeness"`
	Screenshot        *MediaRef        `json:"screenshot,omitempty"`
	AccessibilityTree *MediaRef        `json:"accessibility_tree,omitempty"`
	Elements          []ElementRef     `json:"elements,omitempty"`
	AvailableWindows  []WindowRef      `json:"available_windows,omitempty"`
	CoordinateSpace   *CoordinateSpace `json:"coordinate_space,omitempty"`
	MissingReason     string           `json:"missing_reason,omitempty"`
	PolicySnapshotID  PolicySnapshotID `json:"policy_snapshot_id"`
}

// CoordinateSpace describes the exact coordinate system presented to the
// model and accepted by pointer actions. Model screenshot pixels are
// normalized to Width x Height screen points; the Host alone converts those
// points back to native backing pixels before calling CUA.
type CoordinateSpace struct {
	Unit                   string  `json:"unit"`
	Width                  int     `json:"width"`
	Height                 int     `json:"height"`
	SourceScreenshotWidth  int     `json:"source_screenshot_width,omitempty"`
	SourceScreenshotHeight int     `json:"source_screenshot_height,omitempty"`
	ScaleX                 float64 `json:"scale_x,omitempty"`
	ScaleY                 float64 `json:"scale_y,omitempty"`
	// HasOrigin is required because a window can legitimately sit at (0,0).
	HasOrigin   bool   `json:"has_origin,omitempty"`
	OriginX     int    `json:"origin_x"`
	OriginY     int    `json:"origin_y"`
	OriginSpace string `json:"origin_space,omitempty"`
}

// NewObservation creates an observation linked to a session.
func NewObservation(sess Session, scope CaptureScope, target TargetRef, freshness Freshness) (Observation, error) {
	if sess.SessionID == "" {
		return Observation{}, fmt.Errorf("session is required")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Observation{}, fmt.Errorf("generate observation id: %w", err)
	}
	return Observation{
		ObservationID:    ObservationID("obs_" + hex.EncodeToString(id)),
		SessionID:        sess.SessionID,
		BindingID:        sess.BindingID,
		FenceToken:       sess.FenceToken,
		ControlEpoch:     sess.ControlEpoch,
		Scope:            scope,
		Target:           target,
		Freshness:        freshness,
		Completeness:     "complete",
		PolicySnapshotID: sess.PolicySnapshotID,
	}, nil
}

// IsFresh reports whether the observation is still within its freshness window.
func (o Observation) IsFresh(now time.Time) bool {
	return !o.Freshness.FreshUntil.IsZero() && now.Before(o.Freshness.FreshUntil)
}

// CanAct reports whether the observation may be used as the basis for a new
// action under the current session state and epoch.
func (o Observation) CanAct(sess Session) error {
	if o.SessionID != sess.SessionID {
		return fmt.Errorf("observation %s belongs to session %s, not %s", o.ObservationID, o.SessionID, sess.SessionID)
	}
	if o.FenceToken != sess.FenceToken {
		return fmt.Errorf("observation fence %s does not match session fence %s", o.FenceToken, sess.FenceToken)
	}
	if o.ControlEpoch != sess.ControlEpoch {
		return fmt.Errorf("observation epoch %s does not match session epoch %s", o.ControlEpoch, sess.ControlEpoch)
	}
	if o.Target.Generation != sess.Target.Generation {
		return fmt.Errorf("target generation changed: observation %d vs session %d", o.Target.Generation, sess.Target.Generation)
	}
	if !o.IsFresh(time.Now().UTC()) {
		return fmt.Errorf("observation %s is no longer fresh", o.ObservationID)
	}
	if !AllowsNewActions(sess.State) {
		return fmt.Errorf("session %s in state %s cannot accept new actions", sess.SessionID, sess.State)
	}
	return nil
}

// ObserveOptions controls what is captured.
type ObserveOptions struct {
	Scope                CaptureScope `json:"scope,omitempty"`
	IncludeScreenshot    bool         `json:"include_screenshot"`
	IncludeAccessibility bool         `json:"include_accessibility"`
	Reason               string       `json:"reason,omitempty"`
}
