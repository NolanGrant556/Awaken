package tool

import (
	"context"
	"fmt"
	"time"
)

const (
	ArchivedHistoryErrorInvalidInput  = "archived_history_invalid_input"
	ArchivedHistoryErrorInvalidCursor = "archived_history_invalid_cursor"
	ArchivedHistoryErrorNotFound      = "archived_history_not_found"
	ArchivedHistoryErrorReadFailed    = "archived_history_read_failed"
)

type ArchivedHistoryError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *ArchivedHistoryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ArchivedHistoryListRequest is the host-owned query contract behind the
// archived_history tool. Cursor values are opaque to the model.
type ArchivedHistoryListRequest struct {
	Query     string
	ProjectID string
	Limit     int
	Cursor    string
}

// ArchivedHistoryThread is the bounded, path-free identity of one archived
// thread. Physical FileStore paths stay inside Runtime.
type ArchivedHistoryThread struct {
	ThreadID    string    `json:"thread_id"`
	Title       string    `json:"title"`
	ProjectID   string    `json:"project_id,omitempty"`
	ProjectName string    `json:"project_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ArchivedAt  time.Time `json:"archived_at"`
}

type ArchivedHistoryListResult struct {
	Threads    []ArchivedHistoryThread `json:"threads"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type ArchivedHistoryReadRequest struct {
	ThreadID            string
	BeforeSeq           uint64
	Limit               int
	MaxCharsPerItem     int
	IncludeToolActivity bool
}

// ArchivedHistoryItem is one bounded semantic journal item. Content is
// untrusted historical data and never becomes system or user authority.
type ArchivedHistoryItem struct {
	Seq              uint64    `json:"seq"`
	Kind             string    `json:"kind"`
	Role             string    `json:"role,omitempty"`
	Content          string    `json:"content,omitempty"`
	ContentTruncated bool      `json:"content_truncated,omitempty"`
	ToolName         string    `json:"tool_name,omitempty"`
	ToolCallID       string    `json:"tool_call_id,omitempty"`
	IsError          bool      `json:"is_error,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
}

type ArchivedHistoryReadResult struct {
	Thread        ArchivedHistoryThread `json:"thread"`
	Items         []ArchivedHistoryItem `json:"items"`
	NextBeforeSeq uint64                `json:"next_before_seq,omitempty"`
}

// ArchivedHistoryService is the narrow Runtime contract consumed by the
// model-visible archived_history tool. It deliberately exposes no filesystem
// path and accepts only host-resolved thread identities.
type ArchivedHistoryService interface {
	ListArchivedHistory(context.Context, ArchivedHistoryListRequest) (ArchivedHistoryListResult, error)
	ReadArchivedHistory(context.Context, ArchivedHistoryReadRequest) (ArchivedHistoryReadResult, error)
}

type archivedHistoryServiceContextKey struct{}

func ContextWithArchivedHistoryService(ctx context.Context, service ArchivedHistoryService) context.Context {
	return context.WithValue(ctx, archivedHistoryServiceContextKey{}, service)
}

func ArchivedHistoryServiceFromContext(ctx context.Context) (ArchivedHistoryService, bool) {
	if ctx == nil {
		return nil, false
	}
	service, ok := ctx.Value(archivedHistoryServiceContextKey{}).(ArchivedHistoryService)
	return service, ok && service != nil
}
