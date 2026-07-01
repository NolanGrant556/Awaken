package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ToolAuditRecord captures a complete tool invocation lifecycle for
// observability and compliance.
type ToolAuditRecord struct {
	ToolCallID     string          `json:"toolCallId"`
	RunID          string          `json:"runId"`
	ToolName       string          `json:"toolName"`
	Source         ToolSource      `json:"source"`
	Annotations    ToolAnnotations `json:"annotations"`
	Decision       string          `json:"decision"`       // "allow", "ask", "deny"
	DecisionReason string          `json:"decisionReason"` // human-readable reason
	ApprovalOrigin string          `json:"approvalOrigin"` // "local_ui", "system_policy", "auto_allow", "skill_preapproved"
	StartedAt      time.Time       `json:"startedAt"`
	CompletedAt    time.Time       `json:"completedAt"`
	Outcome        string          `json:"outcome"`      // "success", "failed", "needs_auth", "denied"
	ResultSummary  string          `json:"resultSummary"` // optional, truncated result
	ErrorReason    string          `json:"errorReason"`   // populated on failure
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

// FileSink writes audit records as JSONL to daily files under the given
// directory. Writes are asynchronous; queue overflow drops the oldest
// records with a warning.
type FileSink struct {
	dir   string
	mu    sync.Mutex
	queue chan ToolAuditRecord
	done  chan struct{}
}

// NewFileSink creates a FileSink writing to dir. It starts a background
// goroutine that must be stopped via Close.
func NewFileSink(dir string, queueSize int) *FileSink {
	if queueSize <= 0 {
		queueSize = 256
	}
	s := &FileSink{
		dir:   dir,
		queue: make(chan ToolAuditRecord, queueSize),
		done:  make(chan struct{}),
	}
	go s.loop()
	return s
}

func (s *FileSink) Write(record ToolAuditRecord) error {
	select {
	case s.queue <- record:
		return nil
	default:
		// Queue full: drop oldest
		select {
		case <-s.queue:
		default:
		}
		select {
		case s.queue <- record:
		default:
		}
		return nil
	}
}

func (s *FileSink) loop() {
	for {
		select {
		case <-s.done:
			return
		case record := <-s.queue:
			s.writeRecord(record)
		}
	}
}

func (s *FileSink) writeRecord(record ToolAuditRecord) {
	date := time.Now().Format("2006-01-02")
	filename := filepath.Join(s.dir, date+".jsonl")
	_ = os.MkdirAll(s.dir, 0755)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	data, _ := json.Marshal(record)
	f.Write(append(data, '\n'))
}

// Close stops the background writer goroutine.
func (s *FileSink) Close() error {
	close(s.done)
	return nil
}