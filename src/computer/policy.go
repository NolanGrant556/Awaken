package computer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// PolicySnapshot is an immutable version of the Computer policy at run start.
type PolicySnapshot struct {
	ID                           PolicySnapshotID `json:"id"`
	Version                      string           `json:"version"`
	EnvironmentID                string           `json:"environment_id"`
	AllowedApps                  []string         `json:"allowed_apps,omitempty"`
	AllowedWindows               []string         `json:"allowed_windows,omitempty"`
	AllowedScopes                []CaptureScope   `json:"allowed_scopes,omitempty"`
	AllowedActionKinds           []ActionKind     `json:"allowed_action_kinds,omitempty"`
	AllowBackgroundWindowActions bool             `json:"allow_background_window_actions"`
	DefaultScope                 CaptureScope     `json:"default_scope"`
	RequireReviewFor             []ActionKind     `json:"require_review_for,omitempty"`
	ForbiddenKinds               []ActionKind     `json:"forbidden_kinds,omitempty"`
	MaxDuration                  time.Duration    `json:"max_duration"`
	CreatedAt                    time.Time        `json:"created_at"`
}

// DefaultPolicy returns a restrictive default policy for development/testing.
func DefaultPolicy() PolicySnapshot {
	return PolicySnapshot{
		Version:                      "0.1.0",
		EnvironmentID:                "host://local/desktop",
		AllowedApps:                  []string{},
		AllowedWindows:               []string{},
		AllowedScopes:                []CaptureScope{CaptureWindow, CaptureDesktop},
		AllowedActionKinds:           []ActionKind{ActionClick, ActionDoubleClick, ActionScroll, ActionDrag, ActionKey, ActionTypeText, ActionMoveCursor, ActionHotkey, ActionWait},
		AllowBackgroundWindowActions: false,
		DefaultScope:                 CaptureWindow,
		MaxDuration:                  30 * time.Minute,
		CreatedAt:                    time.Now().UTC(),
	}
}

// PolicyFromSettings builds a runtime policy snapshot from user settings.
// Scope and review policy are host-owned fixed defaults; the user settings
// surface only controls global enablement and background window delivery.
func PolicyFromSettings(settings Settings) PolicySnapshot {
	defaultPolicy := DefaultPolicy()
	return PolicySnapshot{
		Version:                      defaultPolicy.Version,
		EnvironmentID:                defaultPolicy.EnvironmentID,
		AllowedApps:                  defaultPolicy.AllowedApps,
		AllowedWindows:               defaultPolicy.AllowedWindows,
		AllowedScopes:                append([]CaptureScope(nil), defaultPolicy.AllowedScopes...),
		AllowedActionKinds:           defaultPolicy.AllowedActionKinds,
		AllowBackgroundWindowActions: settings.AllowBackgroundWindowActions,
		DefaultScope:                 CaptureWindow,
		RequireReviewFor:             append([]ActionKind(nil), defaultPolicy.RequireReviewFor...),
		MaxDuration:                  defaultPolicy.MaxDuration,
		CreatedAt:                    time.Now().UTC(),
	}
}

// NewPolicySnapshot creates a new policy snapshot with a unique ID.
func NewPolicySnapshot(policy PolicySnapshot) (PolicySnapshot, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return PolicySnapshot{}, fmt.Errorf("generate policy id: %w", err)
	}
	policy.ID = PolicySnapshotID("pol_" + hex.EncodeToString(id))
	if policy.Version == "" {
		policy.Version = "0.1.0"
	}
	if policy.MaxDuration == 0 {
		policy.MaxDuration = 30 * time.Minute
	}
	if policy.DefaultScope == "" {
		policy.DefaultScope = CaptureWindow
	}
	policy.CreatedAt = time.Now().UTC()
	return policy, nil
}

// ScopeAllowed reports whether a capture scope is permitted by policy.
func (p PolicySnapshot) ScopeAllowed(scope CaptureScope) bool {
	for _, allowed := range p.AllowedScopes {
		if allowed == scope {
			return true
		}
	}
	return false
}

// ActionAllowed reports whether an action kind is permitted by policy.
func (p PolicySnapshot) ActionAllowed(kind ActionKind) bool {
	for _, forbidden := range p.ForbiddenKinds {
		if forbidden == kind {
			return false
		}
	}
	for _, allowed := range p.AllowedActionKinds {
		if allowed == kind {
			return true
		}
	}
	return false
}

// RiskForKind returns the policy-mandated risk level for an action kind.
func (p PolicySnapshot) RiskForKind(kind ActionKind) RiskLevel {
	for _, forbidden := range p.ForbiddenKinds {
		if forbidden == kind {
			return RiskForbidden
		}
	}
	for _, reviewed := range p.RequireReviewFor {
		if reviewed == kind {
			return RiskReview
		}
	}
	return RiskLow
}
