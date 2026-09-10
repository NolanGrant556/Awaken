package tool

import "context"

// HistoryRange is a Host-granted ancestor range. A fork copies this value;
// neither tool arguments nor later parent activity can enlarge the grant.
type HistoryRange struct {
	AgentID    string `json:"agent_id"`
	ThroughSeq uint64 `json:"through_seq"`
}

type AgentHistoryScope struct {
	AgentID   string         `json:"agent_id"`
	Ancestors []HistoryRange `json:"ancestors,omitempty"`
}

func (s AgentHistoryScope) Allows(agentID string, seq uint64) bool {
	normalize := func(id string) string {
		if id == "root" {
			return ""
		}
		return id
	}
	if normalize(s.AgentID) == normalize(agentID) {
		return true
	}
	for _, ancestor := range s.Ancestors {
		if normalize(ancestor.AgentID) == normalize(agentID) && seq > 0 && seq <= ancestor.ThroughSeq {
			return true
		}
	}
	return false
}

type agentHistoryScopeKey struct{}

func ContextWithAgentHistoryScope(ctx context.Context, scope AgentHistoryScope) context.Context {
	scope.Ancestors = append([]HistoryRange(nil), scope.Ancestors...)
	return context.WithValue(ctx, agentHistoryScopeKey{}, scope)
}
func AgentHistoryScopeFromContext(ctx context.Context) AgentHistoryScope {
	if scope, ok := ctx.Value(agentHistoryScopeKey{}).(AgentHistoryScope); ok {
		return scope
	}
	identity, _ := ToolExecutionIdentityFromContext(ctx)
	return AgentHistoryScope{AgentID: identity.AgentID}
}

const (
	ThreadHistoryErrorInvalidInput       = "thread_history_invalid_input"
	ThreadHistoryErrorReadFailed         = "thread_history_read_failed"
	ThreadHistoryErrorServiceUnavailable = "thread_history_service_unavailable"
)

type ThreadHistoryReadRequest struct {
	ThreadID            string
	Query               string
	BeforeSeq           uint64
	AfterSeq            uint64
	Limit               int
	MaxCharsPerItem     int
	MaxTotalChars       int
	IncludeToolActivity bool
}

type ThreadHistoryReadResult struct {
	ThreadID      string                `json:"thread_id"`
	Items         []ArchivedHistoryItem `json:"items"`
	NextBeforeSeq uint64                `json:"next_before_seq,omitempty"`
	NextAfterSeq  uint64                `json:"next_after_seq,omitempty"`
}

// ThreadHistoryService is intentionally narrower than ArchivedHistoryService:
// callers may read only the host-injected current thread identity.
type ThreadHistoryService interface {
	ReadCurrentThreadHistory(context.Context, ThreadHistoryReadRequest) (ThreadHistoryReadResult, error)
}

func ThreadHistoryServiceFromContext(ctx context.Context) (ThreadHistoryService, bool) {
	if ctx == nil {
		return nil, false
	}
	service, ok := ctx.Value(archivedHistoryServiceContextKey{}).(ThreadHistoryService)
	return service, ok && service != nil
}
