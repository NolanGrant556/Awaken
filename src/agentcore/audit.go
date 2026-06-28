package agentcore

import (
	"regexp"
	"strings"
	"time"
)

const defaultAuditPreviewMaxRunes = 240

type SecurityAuditEventType string

const (
	SecurityAuditRunStarted            SecurityAuditEventType = "run_started"
	SecurityAuditRunFinished           SecurityAuditEventType = "run_finished"
	SecurityAuditRunError              SecurityAuditEventType = "run_error"
	SecurityAuditRunCancelled          SecurityAuditEventType = "run_cancelled"
	SecurityAuditApprovalRequested     SecurityAuditEventType = "approval_requested"
	SecurityAuditApprovalResolved      SecurityAuditEventType = "approval_resolved"
	SecurityAuditToolExecutionStarted  SecurityAuditEventType = "tool_execution_started"
	SecurityAuditToolExecutionFinished SecurityAuditEventType = "tool_execution_finished"
)

type SecurityAuditAction string

const (
	SecurityAuditActionRun          SecurityAuditAction = "run"
	SecurityAuditActionApproval     SecurityAuditAction = "approval"
	SecurityAuditActionTool         SecurityAuditAction = "tool"
	SecurityAuditActionFileMutation SecurityAuditAction = "file_mutation"
	SecurityAuditActionShellCommand SecurityAuditAction = "shell_command"
)

type SecurityAuditOutcome string

const (
	SecurityAuditOutcomeStarted   SecurityAuditOutcome = "started"
	SecurityAuditOutcomeFinished  SecurityAuditOutcome = "finished"
	SecurityAuditOutcomeError     SecurityAuditOutcome = "error"
	SecurityAuditOutcomeApproved  SecurityAuditOutcome = "approved"
	SecurityAuditOutcomeRejected  SecurityAuditOutcome = "rejected"
	SecurityAuditOutcomeCancelled SecurityAuditOutcome = "cancelled"
)

type SecurityAuditEvent struct {
	ID               string                 `json:"id"`
	Seq              uint64                 `json:"seq"`
	Type             SecurityAuditEventType `json:"type"`
	Timestamp        time.Time              `json:"timestamp"`
	ThreadID         string                 `json:"thread_id,omitempty"`
	RunID            string                 `json:"run_id,omitempty"`
	ToolCallID       string                 `json:"tool_call_id,omitempty"`
	ToolName         string                 `json:"tool_name,omitempty"`
	ApprovalID       string                 `json:"approval_id,omitempty"`
	Action           SecurityAuditAction    `json:"action"`
	Outcome          SecurityAuditOutcome   `json:"outcome,omitempty"`
	ApprovalDecision ApprovalDecision       `json:"approval_decision,omitempty"`
	RiskTone         ApprovalRiskTone       `json:"risk_tone,omitempty"`
	Summary          string                 `json:"summary,omitempty"`
	InputPreview     string                 `json:"input_preview,omitempty"`
	AffectedPaths    []string               `json:"affected_paths,omitempty"`
	Error            string                 `json:"error,omitempty"`
	Truncated        bool                   `json:"truncated,omitempty"`
}

type SecurityAuditRecorder interface {
	RecordSecurityAuditEvent(event SecurityAuditEvent)
}

var auditSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)(["']?(?:authorization|x-api-key|api-key|x-auth-token|api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|id[_-]?token|auth[_-]?token|token|secret|password|passwd|pwd)["']?\s*[:=]\s*["']?)[^"',\s}\]]+`),
	regexp.MustCompile(`sk-proj-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{16,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{16,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
}

func SanitizeSecurityAuditEvent(event SecurityAuditEvent) SecurityAuditEvent {
	event.Summary, event.Truncated = redactAuditField(event.Summary, event.Truncated)
	event.InputPreview, event.Truncated = redactAuditField(event.InputPreview, event.Truncated)
	event.Error, event.Truncated = redactAuditField(event.Error, event.Truncated)
	for i := range event.AffectedPaths {
		event.AffectedPaths[i], event.Truncated = redactAuditField(event.AffectedPaths[i], event.Truncated)
	}
	return event
}

func RedactAuditText(value string, maxRunes int) (string, bool) {
	value = strings.TrimSpace(value)
	truncated := false
	for _, pattern := range auditSecretPatterns {
		value = pattern.ReplaceAllString(value, "${1}[REDACTED]")
	}
	if maxRunes <= 0 {
		maxRunes = defaultAuditPreviewMaxRunes
	}
	value, truncated = truncateAuditText(value, maxRunes)
	return value, truncated
}

func redactAuditField(value string, alreadyTruncated bool) (string, bool) {
	redacted, truncated := RedactAuditText(value, defaultAuditPreviewMaxRunes)
	return redacted, alreadyTruncated || truncated
}

func truncateAuditText(value string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return "", value != ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value, false
	}
	if maxRunes <= len("...[truncated]") {
		return string(runes[:maxRunes]), true
	}
	suffix := "...[truncated]"
	return string(runes[:maxRunes-len([]rune(suffix))]) + suffix, true
}
