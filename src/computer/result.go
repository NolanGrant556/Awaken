package computer

import (
	"fmt"
	"time"
)

// ActionResult records what happened after a PerformChecked call.
type ActionResult struct {
	ActionID          ActionID         `json:"action_id"`
	HostRequestID     HostRequestID    `json:"host_request_id,omitempty"`
	SessionID         SessionID        `json:"session_id"`
	Status            ActionResultStatus `json:"status"`
	CUAStatus         string           `json:"cua_status,omitempty"`
	StartedAt         time.Time        `json:"started_at"`
	FinishedAt        time.Time        `json:"finished_at"`
	ErrorCode         string           `json:"error_code,omitempty"`
	ErrorCategory     string           `json:"error_category,omitempty"`
	ErrorMessage      string           `json:"error_message,omitempty"`
	AfterObservation  *ObservationID   `json:"after_observation,omitempty"`
}

// NewActionResult builds a result with the required fields.
func NewActionResult(actionID ActionID, sessionID SessionID, status ActionResultStatus) ActionResult {
	now := time.Now().UTC()
	return ActionResult{
		ActionID:   actionID,
		SessionID:  sessionID,
		Status:     status,
		StartedAt:  now,
		FinishedAt: now,
	}
}

// VerificationResult is an independent assessment of whether the expected
// effect occurred.
type VerificationResult struct {
	ActionID       ActionID           `json:"action_id"`
	ObservationID  ObservationID      `json:"observation_id"`
	SessionID      SessionID          `json:"session_id"`
	Status         VerificationStatus `json:"status"`
	Assertions     []Assertion        `json:"assertions"`
	EvidenceRefs   []string           `json:"evidence_refs,omitempty"`
	VerifiedAt     time.Time          `json:"verified_at"`
	Reason         string             `json:"reason,omitempty"`
}

// Assertion is a single machine-checkable expectation.
type Assertion struct {
	Type     string `json:"type"`
	Target   string `json:"target"`
	Expected string `json:"expected"`
	Actual   string `json:"actual,omitempty"`
	Matched  bool   `json:"matched"`
}

// NewVerificationResult creates a verification result.
func NewVerificationResult(actionID ActionID, observationID ObservationID, sessionID SessionID, status VerificationStatus) VerificationResult {
	return VerificationResult{
		ActionID:      actionID,
		ObservationID: observationID,
		SessionID:     sessionID,
		Status:        status,
		Assertions:    nil,
		VerifiedAt:    time.Now().UTC(),
	}
}

// UnknownOutcomeResult returns an action result for the cases where input may
// have happened but confirmation was lost.
func UnknownOutcomeResult(actionID ActionID, sessionID SessionID, reason string) ActionResult {
	r := NewActionResult(actionID, sessionID, ActionUnknownOutcome)
	r.ErrorCode = "unknown_outcome"
	r.ErrorMessage = fmt.Sprintf("input may have occurred but the outcome is unknown: %s", reason)
	return r
}
