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
//
// The Intent and SubagentOrigin fields are typed as `any` (rather than
// `*profile.ExecutionIntent` / `*profile.SubagentOrigin`) to avoid an import
// cycle: the `tool/profile` package imports `tool` for the Tool /
// ToolAnnotations / ToolParameter types it needs to build intents, so `tool`
// cannot import `tool/profile` back. The `agentcore` package, which constructs
// audit records, has access to both packages and populates these fields with
// the concrete `*profile.ExecutionIntent` / `*profile.SubagentOrigin` values.
// JSON serialization is unaffected: the concrete types marshal exactly as
// declared.
type ToolAuditRecord struct {
	ToolCallID     string          `json:"toolCallId"`
	RunID          string          `json:"runId"`
	ToolName       string          `json:"toolName"`
	Source         ToolSource      `json:"source"`
	Annotations    ToolAnnotations `json:"annotations"`
	Decision       string          `json:"decision"`
	DecisionReason string          `json:"decisionReason"` // human-readable reason
	DecisionOrigin string          `json:"decisionOrigin"` // "system_policy", "auto_allow", "skill_preapproved"
	StartedAt      time.Time       `json:"startedAt"`
	CompletedAt    time.Time       `json:"completedAt"`
	Outcome        string          `json:"outcome"`       // "success", "failed", "needs_auth", "denied"
	ResultSummary  string          `json:"resultSummary"` // optional, truncated result
	ErrorReason    string          `json:"errorReason"`   // populated on failure
	// ProfileName is the name of the ExecutionProfile applied to this call
	// (e.g. "workspace_write", "guardian_read_only"). It is the human-readable
	// handle that the UI and audit log surface to identify which profile the
	// call landed on.
	ProfileName string `json:"profileName,omitempty"`
	// Intent is the unified ExecutionIntent that was evaluated. It carries the
	// tool kind, requested action, paths, network scope, and subagent origin
	// in a single structured value. nil when the intent was not computed
	// (e.g. unknown tool). Concrete type is `*profile.ExecutionIntent`; see
	// the package doc comment for why this is declared as `any`.
	Intent any `json:"intent,omitempty"`
	// DecisionAction is the unified decision action: "allow", "deny", or
	// "transform". It mirrors the legacy Decision field but uses the unified
	// ExecutionDecisionAction vocabulary.
	DecisionAction string `json:"decisionAction,omitempty"`
	// DecisionSource is the unified decision source (e.g.
	// "workspace_boundary", "sensitive_path", "transform", "policy_error").
	// It mirrors the legacy DecisionReason with structured source attribution.
	DecisionSource string `json:"decisionSource,omitempty"`
	// TransformedFrom is the original tool name when DecisionAction ==
	// "transform" (e.g. "shell"). Empty for allow/deny.
	TransformedFrom string `json:"transformedFrom,omitempty"`
	// TransformedTo is the rewritten tool name when DecisionAction ==
	// "transform" (e.g. "apply_patch"). Empty for allow/deny.
	TransformedTo string `json:"transformedTo,omitempty"`
	// TrashedPaths lists the absolute paths that were moved to the OS recycle
	// bin during this call (for delete operations). Empty when no files were
	// trashed.
	TrashedPaths []string `json:"trashedPaths,omitempty"`
	// SubagentOrigin, when set, attributes this call to a spawning sub-agent.
	// nil for the root agent. Concrete type is `*profile.SubagentOrigin`;
	// see the package doc comment for why this is declared as `any`.
	SubagentOrigin any `json:"subagentOrigin,omitempty"`
	// GuardianViolation is populated when a guardian constraint was violated
	// (i.e. the call was denied because a guardian agent attempted a
	// non-read-only action). Empty for non-guardian denials.
	GuardianViolation string `json:"guardianViolation,omitempty"`
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
