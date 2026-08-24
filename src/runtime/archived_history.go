package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

const (
	defaultArchivedHistoryListLimit   = 10
	maxArchivedHistoryListLimit       = 20
	defaultArchivedHistoryReadLimit   = 20
	maxArchivedHistoryReadLimit       = 20
	defaultArchivedHistoryItemChars   = 2000
	maxArchivedHistoryItemChars       = 4000
	maxArchivedHistoryQueryChars      = 200
	maxArchivedHistoryIdentifierChars = 256
	maxArchivedHistoryCursorChars     = 4096
)

type archivedHistoryCursor struct {
	Query      string    `json:"query,omitempty"`
	ProjectID  string    `json:"project_id,omitempty"`
	ArchivedAt time.Time `json:"archived_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	CreatedAt  time.Time `json:"created_at"`
	ThreadID   string    `json:"thread_id"`
}

// ListArchivedHistory exposes only stable thread identities and display
// metadata. The physical FileStore generation and session paths remain inside
// Runtime and are never returned to the model.
func (s *RunService) ListArchivedHistory(_ context.Context, request tool.ArchivedHistoryListRequest) (tool.ArchivedHistoryListResult, error) {
	query := strings.TrimSpace(request.Query)
	projectID := strings.TrimSpace(request.ProjectID)
	if utf8.RuneCountInString(query) > maxArchivedHistoryQueryChars ||
		utf8.RuneCountInString(projectID) > maxArchivedHistoryIdentifierChars ||
		utf8.RuneCountInString(request.Cursor) > maxArchivedHistoryCursorChars {
		return tool.ArchivedHistoryListResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidInput, "query, project_id, or cursor is too long", false)
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultArchivedHistoryListLimit
	}
	if limit < 1 || limit > maxArchivedHistoryListLimit {
		return tool.ArchivedHistoryListResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidInput, "limit must be between 1 and 20", false)
	}

	queryFolded := strings.ToLower(query)
	archived := s.ListThreads(ThreadListViewArchived)
	filtered := make([]ThreadSnapshot, 0, len(archived))
	for _, snapshot := range archived {
		if projectID != "" && snapshot.ProjectID != projectID && snapshot.Workspace.ProjectID != projectID {
			continue
		}
		if queryFolded != "" {
			haystack := strings.ToLower(strings.Join([]string{
				snapshot.Label,
				snapshot.ThreadID,
				snapshot.ProjectID,
				snapshot.Workspace.ProjectID,
				snapshot.Workspace.ProjectName,
			}, "\n"))
			if !strings.Contains(haystack, queryFolded) {
				continue
			}
		}
		filtered = append(filtered, snapshot)
	}

	start := 0
	if strings.TrimSpace(request.Cursor) != "" {
		cursor, err := decodeArchivedHistoryCursor(request.Cursor)
		if err != nil || cursor.Query != query || cursor.ProjectID != projectID {
			return tool.ArchivedHistoryListResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidCursor, "cursor is invalid for this query", false)
		}
		found := false
		for index, snapshot := range filtered {
			if archivedHistoryCursorMatches(cursor, snapshot) {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			return tool.ArchivedHistoryListResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidCursor, "cursor no longer identifies an archived thread", true)
		}
	}

	end := min(start+limit, len(filtered))
	result := tool.ArchivedHistoryListResult{Threads: make([]tool.ArchivedHistoryThread, 0, end-start)}
	for _, snapshot := range filtered[start:end] {
		result.Threads = append(result.Threads, archivedHistoryThread(snapshot))
	}
	if end < len(filtered) && end > start {
		cursor, err := encodeArchivedHistoryCursor(query, projectID, filtered[end-1])
		if err != nil {
			return tool.ArchivedHistoryListResult{}, archivedHistoryError(tool.ArchivedHistoryErrorReadFailed, "failed to encode the next cursor", true)
		}
		result.NextCursor = cursor
	}
	return result, nil
}

// ReadArchivedHistory resolves the model-provided thread_id against the
// archived metadata view before reading the canonical journal. This exact-ID
// gate prevents path traversal and prevents the tool from reading active or
// deleted threads.
func (s *RunService) ReadArchivedHistory(_ context.Context, request tool.ArchivedHistoryReadRequest) (tool.ArchivedHistoryReadResult, error) {
	threadID := strings.TrimSpace(request.ThreadID)
	if threadID == "" || utf8.RuneCountInString(threadID) > maxArchivedHistoryIdentifierChars {
		return tool.ArchivedHistoryReadResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidInput, "thread_id is required and must be at most 256 characters", false)
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultArchivedHistoryReadLimit
	}
	if limit < 1 || limit > maxArchivedHistoryReadLimit {
		return tool.ArchivedHistoryReadResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidInput, "limit must be between 1 and 20", false)
	}
	maxChars := request.MaxCharsPerItem
	if maxChars == 0 {
		maxChars = defaultArchivedHistoryItemChars
	}
	if maxChars < 100 || maxChars > maxArchivedHistoryItemChars {
		return tool.ArchivedHistoryReadResult{}, archivedHistoryError(tool.ArchivedHistoryErrorInvalidInput, "max_chars_per_item must be between 100 and 4000", false)
	}

	var archivedSnapshot *ThreadSnapshot
	for _, snapshot := range s.ListThreads(ThreadListViewArchived) {
		if snapshot.ThreadID == threadID {
			copy := snapshot
			archivedSnapshot = &copy
			break
		}
	}
	if archivedSnapshot == nil {
		return tool.ArchivedHistoryReadResult{}, archivedHistoryError(tool.ArchivedHistoryErrorNotFound, "archived thread was not found", false)
	}

	transcript, err := s.GetThreadTranscript(threadID)
	if err != nil {
		if errors.Is(err, ErrThreadNotFound) {
			return tool.ArchivedHistoryReadResult{}, archivedHistoryError(tool.ArchivedHistoryErrorNotFound, "archived thread was not found", false)
		}
		return tool.ArchivedHistoryReadResult{}, archivedHistoryError(tool.ArchivedHistoryErrorReadFailed, "the archived journal could not be read", true)
	}

	records := make([]agentcore.SemanticRecord, 0)
	for _, run := range transcript.Runs {
		records = append(records, run.SemanticRecords...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Seq != records[j].Seq {
			return records[i].Seq < records[j].Seq
		}
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
	items := make([]tool.ArchivedHistoryItem, 0, len(records))
	for _, record := range records {
		if request.BeforeSeq > 0 && record.Seq >= request.BeforeSeq {
			continue
		}
		item, ok := archivedHistoryItem(record, request.IncludeToolActivity, maxChars)
		if ok {
			items = append(items, item)
		}
	}
	start := max(0, len(items)-limit)
	page := append([]tool.ArchivedHistoryItem(nil), items[start:]...)
	result := tool.ArchivedHistoryReadResult{
		Thread: archivedHistoryThread(*archivedSnapshot),
		Items:  page,
	}
	if start > 0 && len(page) > 0 {
		result.NextBeforeSeq = page[0].Seq
	}
	return result, nil
}

func archivedHistoryItem(record agentcore.SemanticRecord, includeToolActivity bool, maxChars int) (tool.ArchivedHistoryItem, bool) {
	item := tool.ArchivedHistoryItem{Seq: record.Seq, Kind: string(record.Kind), Timestamp: record.Timestamp}
	switch record.Kind {
	case agentcore.SemanticKindUserMessage:
		item.Role = "user"
		item.Content = stringPayload(record.Payload, "content")
	case agentcore.SemanticKindAssistantMessage:
		item.Role = "assistant"
		item.Content = stringPayload(record.Payload, "content")
	case agentcore.SemanticKindToolCall:
		if !includeToolActivity {
			return tool.ArchivedHistoryItem{}, false
		}
		item.Role = "assistant"
		item.ToolName = stringPayload(record.Payload, "tool_name")
		item.ToolCallID = stringPayload(record.Payload, "tool_call_id")
		// Arguments are deliberately omitted. The archive tool exposes the fact
		// that a tool ran without replaying credentials or large command bodies.
	case agentcore.SemanticKindToolResult:
		if !includeToolActivity {
			return tool.ArchivedHistoryItem{}, false
		}
		item.Role = "tool"
		item.ToolName = stringPayload(record.Payload, "tool_name")
		item.ToolCallID = stringPayload(record.Payload, "tool_call_id")
		item.Content = stringPayload(record.Payload, "content")
		item.IsError = boolPayload(record.Payload, "is_error")
	default:
		return tool.ArchivedHistoryItem{}, false
	}
	item.Content, item.ContentTruncated = truncateArchivedHistoryContent(item.Content, maxChars)
	return item, true
}

func truncateArchivedHistoryContent(content string, maxChars int) (string, bool) {
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content, false
	}
	return string(runes[:maxChars]), true
}

func archivedHistoryThread(snapshot ThreadSnapshot) tool.ArchivedHistoryThread {
	projectID := strings.TrimSpace(snapshot.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(snapshot.Workspace.ProjectID)
	}
	return tool.ArchivedHistoryThread{
		ThreadID: snapshot.ThreadID, Title: snapshot.Label, ProjectID: projectID,
		ProjectName: strings.TrimSpace(snapshot.Workspace.ProjectName), CreatedAt: snapshot.CreatedAt,
		UpdatedAt: snapshot.UpdatedAt, ArchivedAt: snapshot.ArchivedAt,
	}
}

func encodeArchivedHistoryCursor(query, projectID string, snapshot ThreadSnapshot) (string, error) {
	data, err := json.Marshal(archivedHistoryCursor{
		Query: query, ProjectID: projectID, ArchivedAt: snapshot.ArchivedAt,
		UpdatedAt: snapshot.UpdatedAt, CreatedAt: snapshot.CreatedAt, ThreadID: snapshot.ThreadID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeArchivedHistoryCursor(raw string) (archivedHistoryCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return archivedHistoryCursor{}, err
	}
	var cursor archivedHistoryCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return archivedHistoryCursor{}, err
	}
	if strings.TrimSpace(cursor.ThreadID) == "" {
		return archivedHistoryCursor{}, errors.New("missing thread_id")
	}
	return cursor, nil
}

func archivedHistoryCursorMatches(cursor archivedHistoryCursor, snapshot ThreadSnapshot) bool {
	return cursor.ThreadID == snapshot.ThreadID && cursor.ArchivedAt.Equal(snapshot.ArchivedAt) &&
		cursor.UpdatedAt.Equal(snapshot.UpdatedAt) && cursor.CreatedAt.Equal(snapshot.CreatedAt)
}

func archivedHistoryError(code, message string, retryable bool) error {
	return &tool.ArchivedHistoryError{Code: code, Message: message, Retryable: retryable}
}
