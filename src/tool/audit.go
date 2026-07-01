package tool

import (
	"sync"
	"time"
)

// ToolAuditRecord captures a complete tool invocation lifecycle for
// observability and compliance.
type ToolAuditRecord struct {
	ToolCallID     string    `json:"toolCallId"`
	RunID          string    `json:"runId"`
	ToolName       string    `json:"toolName"`
	Source         ToolSource    `json:"source"`
	Decision       string    `json:"decision"`       // "allow", "ask", "deny"
	DecisionReason string    `json:"decisionReason"` // human-readable reason
	ApprovalOrigin string    `json:"approvalOrigin"` // "local_ui", "system_policy", "auto_allow", "skill_preapproved"
	StartedAt      time.Time `json:"startedAt"`
	CompletedAt    time.Time `json:"completedAt"`
	Outcome        string    `json:"outcome"`      // "success", "failed", "needs_auth", "denied"
	ResultSummary  string    `json:"resultSummary"` // optional, truncated result
	ErrorReason    string    `json:"errorReason"`   // populated on failure
}

// AuditSink receives ToolAuditRecords for persistence.
type AuditSink interface {
	Write(record ToolAuditRecord) error
}

// NoopSink drops every record silently.
type NoopSink struct{}

func (s *NoopSink) Write(ToolAuditRecord) error { return nil }

// InMemorySink collects records in memory for test inspection.
type InMemorySink struct {
	mu      sync.Mutex
	records []ToolAuditRecord
}

func (s *InMemorySink) Write(record ToolAuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

// Records returns a snapshot of all collected records.
func (s *InMemorySink) Records() []ToolAuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ToolAuditRecord, len(s.records))
	copy(out, s.records)
	return out
}

// Reset clears the in-memory buffer.
func (s *InMemorySink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
}