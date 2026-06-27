package shellhooks

import (
	"strings"
	"sync"
	"time"
)

type ApprovalState string

const (
	ApprovalUnknown  ApprovalState = "unknown"
	ApprovalApproved ApprovalState = "approved"
	ApprovalDenied   ApprovalState = "denied"
)

type HookKey struct {
	Event   Event  `json:"event"`
	Matcher string `json:"matcher,omitempty"`
	Command string `json:"command"`
}

type ApprovalRecord struct {
	Key        HookKey       `json:"key"`
	State      ApprovalState `json:"state"`
	Reason     string        `json:"reason,omitempty"`
	ApprovedAt time.Time     `json:"approved_at,omitempty"`
}

type ApprovalStore interface {
	State(spec Spec) ApprovalState
}

type MemoryApprovalStore struct {
	mu      sync.RWMutex
	records map[HookKey]ApprovalRecord
}

func NewMemoryApprovalStore(records ...ApprovalRecord) *MemoryApprovalStore {
	store := &MemoryApprovalStore{records: make(map[HookKey]ApprovalRecord)}
	for _, record := range records {
		if record.State == "" {
			record.State = ApprovalUnknown
		}
		store.records[record.Key] = record
	}
	return store
}

func (s *MemoryApprovalStore) Set(spec Spec, state ApprovalState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[HookKey]ApprovalRecord)
	}
	key := KeyForSpec(spec)
	record := ApprovalRecord{Key: key, State: state}
	if state == ApprovalApproved {
		record.ApprovedAt = time.Now().UTC()
	}
	s.records[key] = record
}

func (s *MemoryApprovalStore) State(spec Spec) ApprovalState {
	if s == nil {
		return ApprovalUnknown
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[KeyForSpec(spec)]
	if !ok || record.State == "" {
		return ApprovalUnknown
	}
	return record.State
}

func KeyForSpec(spec Spec) HookKey {
	return HookKey{
		Event:   spec.Event,
		Matcher: strings.TrimSpace(spec.Matcher),
		Command: commandIdentity(spec),
	}
}

func commandIdentity(spec Spec) string {
	if len(spec.Argv) > 0 {
		return "argv:" + strings.Join(spec.Argv, "\x00")
	}
	return "command:" + strings.TrimSpace(spec.Command)
}
