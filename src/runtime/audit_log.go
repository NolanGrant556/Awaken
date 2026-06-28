package runtime

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/agentcore"
)

const defaultAuditLogCapacity = 512

type AuditEventFilter struct {
	RunID string
	Limit int
}

type AuditLog struct {
	mu       sync.RWMutex
	capacity int
	events   []agentcore.SecurityAuditEvent
	nextSeq  uint64
}

func NewAuditLog(capacity int) *AuditLog {
	if capacity <= 0 {
		capacity = defaultAuditLogCapacity
	}
	return &AuditLog{
		capacity: capacity,
		events:   make([]agentcore.SecurityAuditEvent, 0, capacity),
	}
}

func (l *AuditLog) RecordSecurityAuditEvent(event agentcore.SecurityAuditEvent) {
	if l == nil {
		return
	}
	event = agentcore.SanitizeSecurityAuditEvent(event)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.nextSeq++
	event.Seq = l.nextSeq
	if strings.TrimSpace(event.ID) == "" {
		event.ID = newAuditID(event.Seq)
	}

	if len(l.events) == l.capacity {
		copy(l.events, l.events[1:])
		l.events[len(l.events)-1] = event
		return
	}
	l.events = append(l.events, event)
}

func (l *AuditLog) Events(filter AuditEventFilter) []agentcore.SecurityAuditEvent {
	if l == nil {
		return nil
	}

	runID := strings.TrimSpace(filter.RunID)
	limit := filter.Limit
	if limit < 0 {
		limit = 0
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	matched := make([]agentcore.SecurityAuditEvent, 0, len(l.events))
	for _, event := range l.events {
		if runID != "" && event.RunID != runID {
			continue
		}
		matched = append(matched, event)
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	out := make([]agentcore.SecurityAuditEvent, len(matched))
	copy(out, matched)
	return out
}

var auditIDCounter uint64

func newAuditID(seq uint64) string {
	localSeq := atomic.AddUint64(&auditIDCounter, 1)
	return fmt.Sprintf("audit_%d_%d", seq, localSeq)
}
