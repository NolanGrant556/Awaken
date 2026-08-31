package runtime

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

const (
	defaultThreadHistoryTotalChars = 12_000
	maxThreadHistoryTotalChars     = 24_000
)

var (
	threadHistorySecretPattern      = regexp.MustCompile(`(?i)(bearer\s+|basic\s+|sk-|ghp_|github_pat_|glpat-|xoxb-|xoxp-|ya29\.|AIza|hf_|npm_|pypi-|AKIA)[a-z0-9._~+/=-]+`)
	threadHistoryNamedSecretPattern = regexp.MustCompile(
		`(?i)\b(api[_-]?key|password|passwd|passphrase|secret|client[_-]?secret|aws[_-]?secret[_-]?access[_-]?key|cookie|authorization|access[_-]?token|refresh[_-]?token|session[_-]?token|private[_-]?key|credential)\b["']?\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;}]+)`,
	)
)

func (s *RunService) ReadCurrentThreadHistory(_ context.Context, request tool.ThreadHistoryReadRequest) (tool.ThreadHistoryReadResult, error) {
	threadID := strings.TrimSpace(request.ThreadID)
	query := strings.TrimSpace(request.Query)
	if threadID == "" || utf8.RuneCountInString(threadID) > maxArchivedHistoryIdentifierChars || utf8.RuneCountInString(query) > maxArchivedHistoryQueryChars ||
		(request.BeforeSeq > 0 && request.AfterSeq > 0) {
		return tool.ThreadHistoryReadResult{}, archivedHistoryError(tool.ThreadHistoryErrorInvalidInput, "current thread identity, query, or sequence range is invalid", false)
	}
	limit := request.Limit
	if limit == 0 {
		limit = defaultArchivedHistoryReadLimit
	}
	if limit < 1 || limit > maxArchivedHistoryReadLimit {
		return tool.ThreadHistoryReadResult{}, archivedHistoryError(tool.ThreadHistoryErrorInvalidInput, "limit must be between 1 and 20", false)
	}
	maxChars := request.MaxCharsPerItem
	if maxChars == 0 {
		maxChars = defaultArchivedHistoryItemChars
	}
	if maxChars < 100 || maxChars > maxArchivedHistoryItemChars {
		return tool.ThreadHistoryReadResult{}, archivedHistoryError(tool.ThreadHistoryErrorInvalidInput, "max_chars_per_item must be between 100 and 4000", false)
	}
	maxTotalChars := request.MaxTotalChars
	if maxTotalChars == 0 {
		maxTotalChars = defaultThreadHistoryTotalChars
	}
	if maxTotalChars < 1000 || maxTotalChars > maxThreadHistoryTotalChars {
		return tool.ThreadHistoryReadResult{}, archivedHistoryError(tool.ThreadHistoryErrorInvalidInput, "max_total_chars must be between 1000 and 24000", false)
	}

	transcript, err := s.GetThreadTranscript(threadID)
	if err != nil {
		if errors.Is(err, ErrThreadNotFound) {
			return tool.ThreadHistoryReadResult{}, archivedHistoryError(tool.ThreadHistoryErrorReadFailed, "current thread journal was not found", false)
		}
		return tool.ThreadHistoryReadResult{}, archivedHistoryError(tool.ThreadHistoryErrorReadFailed, "current thread journal could not be read", true)
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
	queryLower := strings.ToLower(query)
	items := make([]tool.ArchivedHistoryItem, 0, len(records))
	for _, record := range records {
		if request.BeforeSeq > 0 && record.Seq >= request.BeforeSeq {
			continue
		}
		if request.AfterSeq > 0 && record.Seq <= request.AfterSeq {
			continue
		}
		item, ok := archivedHistoryItem(record, request.IncludeToolActivity, maxChars)
		if !ok {
			continue
		}
		if queryLower != "" && !strings.Contains(strings.ToLower(strings.Join([]string{item.Kind, item.Role, item.Content, item.ToolName, item.ToolCallID}, "\n")), queryLower) {
			continue
		}
		item.Content = sanitizeCurrentThreadHistoryContent(item.Content)
		items = append(items, item)
	}
	result := tool.ThreadHistoryReadResult{ThreadID: threadID, Items: []tool.ArchivedHistoryItem{}}
	if request.AfterSeq > 0 {
		end := min(len(items), limit)
		page, budgetTruncated := boundThreadHistoryItems(items[:end], maxTotalChars, false)
		result.Items = append(result.Items, page...)
		if (end < len(items) || budgetTruncated) && len(result.Items) > 0 {
			result.NextAfterSeq = result.Items[len(result.Items)-1].Seq
		}
		return result, nil
	}
	start := max(0, len(items)-limit)
	page, budgetTruncated := boundThreadHistoryItems(items[start:], maxTotalChars, true)
	result.Items = append(result.Items, page...)
	if (start > 0 || budgetTruncated) && len(result.Items) > 0 {
		result.NextBeforeSeq = result.Items[0].Seq
	}
	return result, nil
}

func sanitizeCurrentThreadHistoryContent(content string) string {
	content = threadHistorySecretPattern.ReplaceAllStringFunc(content, func(value string) string {
		for index, char := range value {
			if char == ' ' {
				return value[:index+1] + "[REDACTED]"
			}
		}
		for _, marker := range []string{"sk-", "ghp_", "github_pat_", "glpat-", "xoxb-", "xoxp-", "ya29.", "aiza", "hf_", "npm_", "pypi-", "akia"} {
			if index := strings.Index(strings.ToLower(value), marker); index >= 0 {
				return value[:index+len(marker)] + "[REDACTED]"
			}
		}
		return "[REDACTED]"
	})
	content = threadHistoryNamedSecretPattern.ReplaceAllStringFunc(content, func(value string) string {
		if index := strings.IndexAny(value, ":="); index >= 0 {
			return strings.TrimSpace(value[:index]) + value[index:index+1] + "[REDACTED]"
		}
		return "[REDACTED]"
	})
	for _, storeRoot := range []string{strings.TrimSpace(activeSessionsDir()), strings.TrimSpace(resolveRuntimeStoreRoot())} {
		if storeRoot != "" {
			content = strings.ReplaceAll(content, storeRoot, "<AWAKEN_THREAD_STORE>")
		}
	}
	return content
}

func boundThreadHistoryItems(items []tool.ArchivedHistoryItem, maxTotalChars int, preferTail bool) ([]tool.ArchivedHistoryItem, bool) {
	if len(items) == 0 || maxTotalChars <= 0 {
		return []tool.ArchivedHistoryItem{}, len(items) > 0
	}
	consume := func(item tool.ArchivedHistoryItem, remaining int) (tool.ArchivedHistoryItem, int, bool) {
		runes := []rune(item.Content)
		if len(runes) <= remaining {
			return item, remaining - len(runes), false
		}
		if remaining <= 0 {
			return tool.ArchivedHistoryItem{}, remaining, true
		}
		item.Content = string(runes[:remaining])
		item.ContentTruncated = true
		return item, 0, true
	}
	remaining := maxTotalChars
	result := make([]tool.ArchivedHistoryItem, 0, len(items))
	truncated := false
	if !preferTail {
		for _, item := range items {
			bounded, next, clipped := consume(item, remaining)
			if bounded.Seq == 0 {
				truncated = true
				break
			}
			result = append(result, bounded)
			remaining = next
			truncated = truncated || clipped
			if remaining == 0 {
				truncated = truncated || len(result) < len(items)
				break
			}
		}
		return result, truncated
	}
	for index := len(items) - 1; index >= 0; index-- {
		bounded, next, clipped := consume(items[index], remaining)
		if bounded.Seq == 0 {
			truncated = true
			break
		}
		result = append([]tool.ArchivedHistoryItem{bounded}, result...)
		remaining = next
		truncated = truncated || clipped
		if remaining == 0 {
			truncated = truncated || len(result) < len(items)
			break
		}
	}
	return result, truncated
}
