package computer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Review captures a human decision about a single prepared action digest.
type Review struct {
	ReviewID        ReviewID       `json:"review_id"`
	ActionID        ActionID       `json:"action_id"`
	ActionDigest    string         `json:"action_digest"`
	PolicySnapshotID PolicySnapshotID `json:"policy_snapshot_id"`
	SessionID       SessionID      `json:"session_id"`
	BindingID       BindingID      `json:"binding_id"`
	FenceToken      FenceToken     `json:"fence_token"`
	ControlEpoch    ControlEpoch   `json:"control_epoch"`
	TargetSummary   string         `json:"target_summary"`
	EffectSummary   string         `json:"effect_summary"`
	EvidenceRefs    []string       `json:"evidence_refs,omitempty"`
	RequestedAt     time.Time      `json:"requested_at"`
	DecidedAt       *time.Time     `json:"decided_at,omitempty"`
	ExpiresAt       time.Time      `json:"expires_at"`
	Decision        ReviewDecision `json:"decision"`
	Actor           string         `json:"actor,omitempty"`
	ControlRouteRequestID string   `json:"control_route_request_id,omitempty"`
}

// NewReview creates a pending review for a prepared action.
func NewReview(action PreparedAction, targetSummary, effectSummary string, evidence []string, ttl time.Duration) (Review, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Review{}, fmt.Errorf("generate review id: %w", err)
	}
	now := time.Now().UTC()
	return Review{
		ReviewID:         ReviewID("rev_" + hex.EncodeToString(id)),
		ActionID:         action.ActionID,
		ActionDigest:     action.ActionDigest,
		PolicySnapshotID: action.PolicySnapshotID,
		SessionID:        action.SessionID,
		BindingID:        action.BindingID,
		FenceToken:       action.FenceToken,
		ControlEpoch:     action.ControlEpoch,
		TargetSummary:    targetSummary,
		EffectSummary:    effectSummary,
		EvidenceRefs:     append([]string(nil), evidence...),
		RequestedAt:      now,
		ExpiresAt:        now.Add(ttl),
		Decision:         "", // pending
	}, nil
}

// IsPending reports whether the review is awaiting a decision.
func (r Review) IsPending() bool {
	return r.Decision == ""
}

// Resolve records a human decision. The decision must match the digest,
// session, fence and epoch of the action it was created for.
func (r *Review) Resolve(decision ReviewDecision, actor, requestID string, now time.Time) error {
	if r == nil {
		return fmt.Errorf("nil review")
	}
	if !r.IsPending() {
		return fmt.Errorf("review %s already resolved as %s", r.ReviewID, r.Decision)
	}
	if decision == "" {
		return fmt.Errorf("decision is required")
	}
	switch decision {
	case ReviewApprovedOnce, ReviewDenied, ReviewExpired, ReviewCancelled:
	default:
		return fmt.Errorf("invalid review decision %q", decision)
	}
	if now.After(r.ExpiresAt) {
		r.Decision = ReviewExpired
		return fmt.Errorf("review %s expired", r.ReviewID)
	}
	r.Decision = decision
	nowUTC := now.UTC()
	r.DecidedAt = &nowUTC
	r.Actor = actor
	r.ControlRouteRequestID = requestID
	return nil
}

// ValidateForAction checks that the review authorizes exactly this action.
func (r Review) ValidateForAction(action PreparedAction, now time.Time) error {
	if r.ActionID != action.ActionID {
		return fmt.Errorf("review action mismatch")
	}
	if r.ActionDigest != action.ActionDigest {
		return fmt.Errorf("review digest mismatch")
	}
	if r.SessionID != action.SessionID {
		return fmt.Errorf("review session mismatch")
	}
	if r.FenceToken != action.FenceToken {
		return fmt.Errorf("review fence mismatch")
	}
	if r.ControlEpoch != action.ControlEpoch {
		return fmt.Errorf("review epoch mismatch")
	}
	if r.Decision != ReviewApprovedOnce {
		return fmt.Errorf("review decision %q does not authorize perform", r.Decision)
	}
	if now.After(r.ExpiresAt) {
		return fmt.Errorf("review expired")
	}
	return nil
}
