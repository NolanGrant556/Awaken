package computer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// PreparedAction is an immutable, policy-evaluated action ready for execution.
// It is bound to an observation and includes a digest covering every field that
// affects the user decision or actual input.
type PreparedAction struct {
	ActionID         ActionID         `json:"action_id"`
	IdempotencyKey   string           `json:"idempotency_key"`
	ObservationID    ObservationID    `json:"observation_id"`
	SessionID        SessionID        `json:"session_id"`
	BindingID        BindingID        `json:"binding_id"`
	PolicySnapshotID PolicySnapshotID `json:"policy_snapshot_id"`
	FenceToken       FenceToken       `json:"fence_token"`
	ControlEpoch     ControlEpoch     `json:"control_epoch"`
	TargetGeneration uint64           `json:"target_generation"`
	Kind             ActionKind       `json:"action"`
	Target           ElementRef       `json:"target"`
	Parameters       map[string]any   `json:"parameters"`
	ExpectedContext  map[string]any   `json:"expected_context,omitempty"`
	Intent           string           `json:"intent"`
	ExpectedEffect   string           `json:"expected_effect"`
	ActionDigest     string           `json:"action_digest"`
	Risk             RiskLevel        `json:"risk"`
	RequiresReview   bool             `json:"requires_review"`
	PreparedAt       time.Time        `json:"prepared_at"`
	ExpiresAt        time.Time        `json:"expires_at"`
}

// PrepareAction creates a PreparedAction from an observation and an action spec.
// It computes the digest and records the policy decision.
func PrepareAction(sess Session, obs Observation, spec ActionSpec, risk RiskLevel, expires time.Time) (PreparedAction, error) {
	if err := obs.CanAct(sess); err != nil {
		return PreparedAction{}, fmt.Errorf("cannot prepare action: %w", err)
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return PreparedAction{}, fmt.Errorf("generate action id: %w", err)
	}
	if spec.IdempotencyKey == "" {
		spec.IdempotencyKey = hex.EncodeToString(id)
	}
	prepared := PreparedAction{
		ActionID:         ActionID("act_" + hex.EncodeToString(id)),
		IdempotencyKey:   spec.IdempotencyKey,
		ObservationID:    obs.ObservationID,
		SessionID:        sess.SessionID,
		BindingID:        sess.BindingID,
		PolicySnapshotID: sess.PolicySnapshotID,
		FenceToken:       sess.FenceToken,
		ControlEpoch:     sess.ControlEpoch,
		TargetGeneration: sess.Target.Generation,
		Kind:             spec.Kind,
		Target:           spec.Target,
		Parameters:       cloneAnyMap(spec.Parameters),
		ExpectedContext:  cloneAnyMap(spec.ExpectedContext),
		Intent:           spec.Intent,
		ExpectedEffect:   spec.ExpectedEffect,
		Risk:             risk,
		RequiresReview:   risk == RiskReview || risk == RiskHandoff,
		PreparedAt:       time.Now().UTC(),
		ExpiresAt:        expires,
	}
	prepared.ActionDigest = prepared.ComputeDigest()
	return prepared, nil
}

// ComputeDigest returns a stable hash of all fields that affect the user
// decision or input. Any change invalidates prior reviews.
func (a PreparedAction) ComputeDigest() string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(struct {
		IdempotencyKey   string
		ObservationID    string
		SessionID        string
		BindingID        string
		PolicySnapshotID string
		FenceToken       uint64
		ControlEpoch     uint64
		TargetGeneration uint64
		Kind             string
		Target           ElementRef
		Parameters       map[string]any
		ExpectedContext  map[string]any
		Intent           string
		ExpectedEffect   string
		Risk             string
		RequiresReview   bool
	}{
		IdempotencyKey:   a.IdempotencyKey,
		ObservationID:    string(a.ObservationID),
		SessionID:        string(a.SessionID),
		BindingID:        string(a.BindingID),
		PolicySnapshotID: string(a.PolicySnapshotID),
		FenceToken:       uint64(a.FenceToken),
		ControlEpoch:     uint64(a.ControlEpoch),
		TargetGeneration: a.TargetGeneration,
		Kind:             string(a.Kind),
		Target:           a.Target,
		Parameters:       a.Parameters,
		ExpectedContext:  a.ExpectedContext,
		Intent:           a.Intent,
		ExpectedEffect:   a.ExpectedEffect,
		Risk:             string(a.Risk),
		RequiresReview:   a.RequiresReview,
	})
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// ValidForSession checks that the prepared action matches the current session
// state and has not expired.
func (a PreparedAction) ValidForSession(sess Session, now time.Time) error {
	if a.SessionID != sess.SessionID {
		return fmt.Errorf("action session mismatch")
	}
	if a.BindingID != sess.BindingID {
		return fmt.Errorf("action binding mismatch")
	}
	if a.PolicySnapshotID != sess.PolicySnapshotID {
		return fmt.Errorf("action policy snapshot mismatch")
	}
	if a.FenceToken != sess.FenceToken {
		return fmt.Errorf("action fence mismatch")
	}
	if a.ControlEpoch != sess.ControlEpoch {
		return fmt.Errorf("action control epoch mismatch")
	}
	if a.TargetGeneration != sess.Target.Generation {
		return fmt.Errorf("action target generation mismatch")
	}
	if a.ActionDigest == "" || a.ActionDigest != a.ComputeDigest() {
		return fmt.Errorf("action digest mismatch")
	}
	if !now.Before(a.ExpiresAt) {
		return fmt.Errorf("action digest expired")
	}
	if !AllowsNewActions(sess.State) {
		return fmt.Errorf("session cannot perform actions")
	}
	return nil
}

// cloneAnyMap returns a shallow copy of a map[string]any.
func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	clone := make(map[string]any, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

// CloneActionParameters returns a shallow copy suitable for adapter-specific
// normalization without mutating the immutable PreparedAction.
func CloneActionParameters(parameters map[string]any) map[string]any {
	cloned := cloneAnyMap(parameters)
	if cloned == nil {
		cloned = make(map[string]any)
	}
	return cloned
}

// RiskForAction evaluates a simple policy. Real policy will be richer.
func RiskForAction(bind Binding, kind ActionKind, target ElementRef, intent string, params map[string]any) RiskLevel {
	if !bind.ActionKindAllowed(kind) {
		return RiskForbidden
	}
	for _, forbidden := range bind.ForbiddenKinds {
		if forbidden == kind {
			return RiskForbidden
		}
	}
	// Handoff: anything that smells like authentication, secrets, or security.
	if isSensitiveAction(kind, params) || isSensitiveIntent(intent) {
		return RiskHandoff
	}
	for _, reviewed := range bind.RequireReviewFor {
		if reviewed == kind {
			return RiskReview
		}
	}
	// Review: actions with external consequences.
	if hasExternalConsequence(kind, params, intent) {
		return RiskReview
	}
	return RiskLow
}

func isSensitiveAction(kind ActionKind, params map[string]any) bool {
	if kind == ActionTypeText {
		if secret, ok := params["secret"].(bool); ok && secret {
			return true
		}
	}
	return false
}

func isSensitiveIntent(intent string) bool {
	lower := ""
	for _, r := range intent {
		if r >= 'A' && r <= 'Z' {
			r += 32
		}
		lower += string(r)
	}
	for _, keyword := range []string{"password", "otp", "login", "authenticate", "admin", "payment", "credit card", "security"} {
		if containsSubstring(lower, keyword) {
			return true
		}
	}
	return false
}

func hasExternalConsequence(kind ActionKind, params map[string]any, intent string) bool {
	checkText := ""
	if params != nil {
		switch kind {
		case ActionKey:
			if key, ok := params["key"].(string); ok {
				checkText = key
			}
		case ActionClick, ActionDoubleClick:
			if label, ok := params["label"].(string); ok {
				checkText = label
			}
		}
	}
	// Fall back to intent when explicit parameter label is absent.
	if checkText == "" {
		checkText = intent
	}
	checkText = toLowerASCII(checkText)
	for _, ent := range []string{"send", "submit", "delete", "buy", "purchase", "publish"} {
		if containsSubstring(checkText, ent) {
			return true
		}
	}
	return false
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func containsSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
