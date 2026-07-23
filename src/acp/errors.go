package acp

import "errors"

// Error codes from the ACP design document §16.
const (
	ErrCodeRunnerNotConfigured     = "acp_runner_not_configured"
	ErrCodeRunnerNotReady          = "acp_runner_not_ready"
	ErrCodeStartTimeout            = "acp_start_timeout"
	ErrCodeProtocolVersionMismatch = "acp_protocol_version_mismatch"
	ErrCodeCapabilityUnsupported   = "acp_capability_unsupported"
	ErrCodeSessionConflict         = "acp_session_conflict"
	ErrCodeSessionMissing          = "acp_session_missing"
	ErrCodePermissionDenied        = "acp_permission_denied"
	ErrCodePermissionExpired       = "acp_permission_expired"
	ErrCodeTurnTimeout             = "acp_turn_timeout"
	ErrCodeTurnCancelled           = "acp_turn_cancelled"
	ErrCodeTransportClosed         = "acp_transport_closed"
	ErrCodeOutputLimitExceeded     = "acp_output_limit_exceeded"
)

// Common errors.
var (
	ErrSessionConflict  = errors.New("acp session conflict")
	ErrSessionMissing   = errors.New("acp session missing")
	ErrTurnInProgress   = errors.New("acp turn in progress")
	ErrRunnerNotEnabled = errors.New("acp runner not enabled")
	ErrProtocolMismatch = errors.New("acp protocol version mismatch")
)
