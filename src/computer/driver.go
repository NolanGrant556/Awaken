package computer

import (
	"context"
	"fmt"
)

// Driver is the minimal Awaken-facing contract for a Computer Host. It does
// not expose CUA handles, MCP methods or raw SDK surfaces.
type Driver interface {
	// Health reports whether the Host is ready to accept sessions.
	Health(ctx context.Context) (HealthResult, error)

	// OpenSession creates a new CUA session bound to the binding and policy.
	// The proposed Session carries the authoritative Awaken session ID; the
	// driver must honor it and return a Session with the same SessionID and
	// BindingID.
	OpenSession(ctx context.Context, bind Binding, proposed Session, policy PolicySnapshot, fence FenceToken) (Session, error)

	// Observe returns a fresh observation of the authorized scope.
	Observe(ctx context.Context, sessionID SessionID, scope CaptureScope, target TargetRef, opts ObserveOptions) (Observation, error)

	// Prepare parses an action and returns a prepared action without side effects.
	Prepare(ctx context.Context, sessionID SessionID, obs Observation, spec ActionSpec) (PreparedAction, error)

	// PerformChecked atomically re-validates the action and injects input.
	PerformChecked(ctx context.Context, sessionID SessionID, action PreparedAction, fence FenceToken, epoch ControlEpoch) (ActionResult, error)

	// Quiesce stops new input and returns when the old epoch is drained.
	Quiesce(ctx context.Context, sessionID SessionID, nextEpoch ControlEpoch) error

	// Resume refreshes policy and target and returns an updated session.
	Resume(ctx context.Context, sessionID SessionID, policy PolicySnapshot, fence FenceToken, epoch ControlEpoch) (Session, error)

	// CloseSession closes the session and releases the Host handle.
	CloseSession(ctx context.Context, sessionID SessionID, reason string) error
}

// UnavailableDriver is a fail-closed driver that is never healthy. It is the
// default before a real Host is bound to a run.
type UnavailableDriver struct{}

// NewUnavailableDriver returns a driver that always reports unavailable.
func NewUnavailableDriver() *UnavailableDriver {
	return &UnavailableDriver{}
}

func (d *UnavailableDriver) Health(ctx context.Context) (HealthResult, error) {
	return HealthResult{Healthy: false, Message: "Computer Host is not available"}, nil
}

func (d *UnavailableDriver) OpenSession(ctx context.Context, bind Binding, proposed Session, policy PolicySnapshot, fence FenceToken) (Session, error) {
	return Session{}, fmt.Errorf("computer driver unavailable")
}

func (d *UnavailableDriver) Observe(ctx context.Context, sessionID SessionID, scope CaptureScope, target TargetRef, opts ObserveOptions) (Observation, error) {
	return Observation{}, fmt.Errorf("computer driver unavailable")
}

func (d *UnavailableDriver) Prepare(ctx context.Context, sessionID SessionID, obs Observation, spec ActionSpec) (PreparedAction, error) {
	return PreparedAction{}, fmt.Errorf("computer driver unavailable")
}

func (d *UnavailableDriver) PerformChecked(ctx context.Context, sessionID SessionID, action PreparedAction, fence FenceToken, epoch ControlEpoch) (ActionResult, error) {
	return ActionResult{}, fmt.Errorf("computer driver unavailable")
}

func (d *UnavailableDriver) Quiesce(ctx context.Context, sessionID SessionID, nextEpoch ControlEpoch) error {
	return fmt.Errorf("computer driver unavailable")
}

func (d *UnavailableDriver) Resume(ctx context.Context, sessionID SessionID, policy PolicySnapshot, fence FenceToken, epoch ControlEpoch) (Session, error) {
	return Session{}, fmt.Errorf("computer driver unavailable")
}

func (d *UnavailableDriver) CloseSession(ctx context.Context, sessionID SessionID, reason string) error {
	return nil
}

var _ Driver = (*UnavailableDriver)(nil)
