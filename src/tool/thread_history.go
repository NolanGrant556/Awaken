package tool

import "context"

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
