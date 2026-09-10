package computer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Binding is created once per root run. It captures the immutable policy,
// resource identity and media readiness that the run's tool snapshot pins.
type Binding struct {
	WaitForResource              bool             `json:"-"`
	BindingID                    BindingID        `json:"binding_id"`
	ThreadID                     string           `json:"thread_id"`
	RunID                        string           `json:"run_id"`
	RootAgentID                  string           `json:"root_agent_id"`
	EnvironmentID                string           `json:"environment_id"`
	ResourceKey                  string           `json:"resource_key"`
	HostVersion                  string           `json:"host_version"`
	ProtocolVersion              int              `json:"protocol_version"`
	DriverVersion                string           `json:"driver_version"`
	PolicySnapshotID             PolicySnapshotID `json:"policy_snapshot_id"`
	AllowedApps                  []string         `json:"allowed_apps,omitempty"`
	AllowedWindows               []string         `json:"allowed_windows,omitempty"`
	AllowedScopes                []CaptureScope   `json:"allowed_scopes,omitempty"`
	AllowedActionKinds           []ActionKind     `json:"allowed_action_kinds,omitempty"`
	AllowBackgroundWindowActions bool             `json:"allow_background_window_actions"`
	RequireReviewFor             []ActionKind     `json:"require_review_for,omitempty"`
	ForbiddenKinds               []ActionKind     `json:"forbidden_kinds,omitempty"`
	FenceToken                   FenceToken       `json:"fence_token"`
	ControlEpoch                 ControlEpoch     `json:"control_epoch"`
	MediaPolicyReady             bool             `json:"media_policy_ready"`
	ToolCallingReady             bool             `json:"tool_calling_ready"`
	VisionReady                  bool             `json:"vision_ready"`
	CreatedAt                    time.Time        `json:"created_at"`
	ExpiresAt                    time.Time        `json:"expires_at"`
}

// NewBinding creates a run-scoped binding. It does not contact the Host.
func NewBinding(runID, threadID, rootAgentID, resourceKey string, policy PolicySnapshot) (Binding, error) {
	if runID == "" {
		return Binding{}, fmt.Errorf("run_id is required")
	}
	if resourceKey == "" {
		resourceKey = "host://local/desktop"
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Binding{}, fmt.Errorf("generate binding id: %w", err)
	}
	now := time.Now().UTC()
	return Binding{
		BindingID:                    BindingID("bind_" + hex.EncodeToString(id)),
		ThreadID:                     threadID,
		RunID:                        runID,
		RootAgentID:                  rootAgentID,
		EnvironmentID:                policy.EnvironmentID,
		ResourceKey:                  resourceKey,
		PolicySnapshotID:             policy.ID,
		AllowedApps:                  append([]string(nil), policy.AllowedApps...),
		AllowedWindows:               append([]string(nil), policy.AllowedWindows...),
		AllowedScopes:                append([]CaptureScope(nil), policy.AllowedScopes...),
		AllowedActionKinds:           append([]ActionKind(nil), policy.AllowedActionKinds...),
		AllowBackgroundWindowActions: policy.AllowBackgroundWindowActions,
		RequireReviewFor:             append([]ActionKind(nil), policy.RequireReviewFor...),
		ForbiddenKinds:               append([]ActionKind(nil), policy.ForbiddenKinds...),
		FenceToken:                   NewFenceToken(0),
		ControlEpoch:                 NewControlEpoch(0),
		CreatedAt:                    now,
		ExpiresAt:                    now.Add(policy.MaxDuration),
	}, nil
}

// Clone returns a deep copy.
func (b Binding) Clone() Binding {
	clone := b
	clone.AllowedApps = append([]string(nil), b.AllowedApps...)
	clone.AllowedWindows = append([]string(nil), b.AllowedWindows...)
	clone.AllowedScopes = append([]CaptureScope(nil), b.AllowedScopes...)
	clone.AllowedActionKinds = append([]ActionKind(nil), b.AllowedActionKinds...)
	clone.RequireReviewFor = append([]ActionKind(nil), b.RequireReviewFor...)
	clone.ForbiddenKinds = append([]ActionKind(nil), b.ForbiddenKinds...)
	return clone
}

// ScopeAllowed reports whether a capture scope is permitted by this binding.
func (b Binding) ScopeAllowed(scope CaptureScope) bool {
	for _, allowed := range b.AllowedScopes {
		if allowed == scope {
			return true
		}
	}
	return false
}

// ActionKindAllowed reports whether an action kind is permitted.
func (b Binding) ActionKindAllowed(kind ActionKind) bool {
	for _, allowed := range b.AllowedActionKinds {
		if allowed == kind {
			return true
		}
	}
	return false
}

// AppAllowed reports whether an app is in the allowlist.
func (b Binding) AppAllowed(name string) bool {
	for _, allowed := range b.AllowedApps {
		if strings.EqualFold(allowed, name) {
			return true
		}
	}
	return false
}

// NextFence returns a new binding with an incremented fence token. Used for
// takeover, Host restart and lease refresh.
func (b Binding) NextFence() Binding {
	b.FenceToken = NewFenceToken(b.FenceToken)
	return b
}

// NextEpoch returns a new binding with an incremented control epoch.
func (b Binding) NextEpoch() Binding {
	b.ControlEpoch = NewControlEpoch(b.ControlEpoch)
	return b
}
