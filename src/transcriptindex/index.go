package transcriptindex

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

const firstPromptLimit = 200

var skipFirstPromptPattern = regexp.MustCompile(`^\s*<[a-z][A-Za-z0-9_-]*(?:\s|>|/)`)

type Entry struct {
	Type             string  `json:"type"`
	SessionID        string  `json:"sessionId,omitempty"`
	UUID             string  `json:"uuid,omitempty"`
	ParentUUID       string  `json:"parentUuid,omitempty"`
	Message          Message `json:"message,omitempty"`
	IsMeta           bool    `json:"isMeta,omitempty"`
	IsCompactSummary bool    `json:"isCompactSummary,omitempty"`
	Title            string  `json:"title,omitempty"`
	CustomTitle      string  `json:"customTitle,omitempty"`
	AITitle          string  `json:"aiTitle,omitempty"`
	SummaryText      string  `json:"summary,omitempty"`
}

type Message struct {
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Text      string          `json:"text,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	UUID      string          `json:"uuid,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type Summary struct {
	SessionID    string `json:"sessionId,omitempty"`
	Title        string `json:"title,omitempty"`
	FirstPrompt  string `json:"firstPrompt,omitempty"`
	Text         string `json:"text,omitempty"`
	MessageCount int    `json:"messageCount"`
}

type SessionItem struct {
	Summary
	Messages []Message `json:"messages,omitempty"`
}

type Result struct {
	Item         SessionItem `json:"item"`
	Score        int         `json:"score"`
	Match        string      `json:"match"`
	MessageIndex int         `json:"messageIndex,omitempty"`
}

type rawEntry struct {
	Type             string          `json:"type"`
	SessionID        string          `json:"sessionId"`
	UUID             string          `json:"uuid"`
	ParentUUID       string          `json:"parentUuid"`
	Message          json.RawMessage `json:"message"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Title            string          `json:"title"`
	CustomTitle      string          `json:"customTitle"`
	AITitle          string          `json:"aiTitle"`
	SummaryText      string          `json:"summary"`
}

type sessionBuilder struct {
	item  SessionItem
	lines []string
}

type searchCandidate struct {
	result Result
	order  int
}

func ParseJSONLLite(text string) []SessionItem {
	return BuildLiteIndex(strings.Split(text, "\n"))
}

func BuildLiteIndex(lines []string) []SessionItem {
	builders := map[string]*sessionBuilder{}
	var order []string

	ensure := func(sessionID string) *sessionBuilder {
		if builder, ok := builders[sessionID]; ok {
			return builder
		}
		builder := &sessionBuilder{item: SessionItem{
			Summary: Summary{SessionID: sessionID},
		}}
		builders[sessionID] = builder
		order = append(order, sessionID)
		return builder
	}

	for _, line := range lines {
		entry, raw, ok := parseEntry(line)
		if !ok {
			continue
		}

		if title, ok := entryTitle(entry); ok {
			builder := ensure(entry.SessionID)
			builder.item.Title = title
			continue
		}
		if entry.Type == "summary" && strings.TrimSpace(entry.SummaryText) != "" {
			builder := ensure(entry.SessionID)
			builder.item.Text = strings.TrimSpace(entry.SummaryText)
			continue
		}
		if !isTranscriptType(entry.Type) || shouldSkipEntry(entry, raw) {
			continue
		}

		texts, _ := entryTexts(entry, raw)
		message := Message{
			Type:      entry.Type,
			Role:      entry.Message.Role,
			Text:      normalizeMessageText(texts),
			UUID:      entry.UUID,
			SessionID: entry.SessionID,
		}
		if message.Role == "" {
			message.Role = entry.Type
		}

		builder := ensure(entry.SessionID)
		builder.item.Messages = append(builder.item.Messages, message)
		builder.lines = append(builder.lines, strings.TrimSpace(line))
	}

	items := make([]SessionItem, 0, len(order))
	for _, sessionID := range order {
		builder := builders[sessionID]
		builder.item.MessageCount = len(builder.item.Messages)
		if builder.item.FirstPrompt == "" {
			builder.item.FirstPrompt = ExtractFirstPrompt(builder.lines)
		}
		if builder.item.Title == "" && len(builder.item.Messages) == 0 && builder.item.Text == "" {
			continue
		}
		items = append(items, builder.item)
	}
	return items
}

func ExtractFirstPrompt(lines []string) string {
	commandFallback := ""

	for _, line := range lines {
		entry, raw, ok := parseEntry(line)
		if !ok || entry.Type != "user" || shouldSkipEntry(entry, raw) {
			continue
		}

		texts, _ := entryTexts(entry, raw)
		for _, text := range texts {
			result := normalizePromptText(text)
			if result == "" {
				continue
			}

			if commandName := strings.TrimSpace(extractTag(result, "command-name")); commandName != "" {
				if commandFallback == "" {
					commandFallback = commandName
				}
				continue
			}

			if bashInput := strings.TrimSpace(extractTag(result, "bash-input")); bashInput != "" {
				return truncatePrompt("! " + bashInput)
			}

			if skipFirstPromptPattern.MatchString(result) || strings.HasPrefix(result, "[Request interrupted by user") {
				continue
			}
			return truncatePrompt(result)
		}
	}

	return truncatePrompt(commandFallback)
}

func Search(items []SessionItem, query string, limit int) []Result {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	candidates := make([]searchCandidate, 0, len(items))
	for i, item := range items {
		result, ok := matchItem(item, query)
		if !ok {
			continue
		}
		candidates = append(candidates, searchCandidate{result: result, order: i})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].result.Score != candidates[j].result.Score {
			return candidates[i].result.Score > candidates[j].result.Score
		}
		return candidates[i].order < candidates[j].order
	})

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]Result, 0, len(candidates))
	for _, candidate := range candidates {
		results = append(results, candidate.result)
	}
	return results
}

func matchItem(item SessionItem, query string) (Result, bool) {
	result := Result{Item: item, MessageIndex: -1}
	if containsFolded(item.SessionID, query) {
		result.Score = 400
		result.Match = "session"
		return result, true
	}
	if containsFolded(item.Title, query) {
		result.Score = 300
		result.Match = "title"
		return result, true
	}
	if containsFolded(item.FirstPrompt, query) {
		result.Score = 200
		result.Match = "first_prompt"
		return result, true
	}
	for i, message := range item.Messages {
		if containsFolded(message.Text, query) {
			result.Score = 100
			result.Match = "message"
			result.MessageIndex = i
			return result, true
		}
	}
	return Result{}, false
}

func containsFolded(text, query string) bool {
	return strings.Contains(strings.ToLower(text), query)
}

func parseEntry(line string) (Entry, map[string]json.RawMessage, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Entry{}, nil, false
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return Entry{}, nil, false
	}

	var parsed rawEntry
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		return Entry{}, nil, false
	}
	if parsed.SessionID == "" {
		parsed.SessionID = stringField(raw, "session_id")
	}

	entry := Entry{
		Type:             parsed.Type,
		SessionID:        parsed.SessionID,
		UUID:             parsed.UUID,
		ParentUUID:       parsed.ParentUUID,
		IsMeta:           parsed.IsMeta,
		IsCompactSummary: parsed.IsCompactSummary,
		Title:            parsed.Title,
		CustomTitle:      parsed.CustomTitle,
		AITitle:          parsed.AITitle,
		SummaryText:      parsed.SummaryText,
	}

	if len(parsed.Message) > 0 && string(parsed.Message) != "null" {
		if err := json.Unmarshal(parsed.Message, &entry.Message); err != nil {
			if text := rawString(parsed.Message); text != "" {
				entry.Message.Text = text
			}
		}
	}
	if entry.Message.Type == "" {
		entry.Message.Type = entry.Type
	}
	if entry.Message.Role == "" {
		entry.Message.Role = entry.Type
	}
	if entry.Message.UUID == "" {
		entry.Message.UUID = entry.UUID
	}
	if entry.Message.SessionID == "" {
		entry.Message.SessionID = entry.SessionID
	}
	return entry, raw, true
}

func isTranscriptType(entryType string) bool {
	switch entryType {
	case "user", "assistant", "attachment", "system":
		return true
	default:
		return false
	}
}

func shouldSkipEntry(entry Entry, raw map[string]json.RawMessage) bool {
	if entry.IsMeta || entry.IsCompactSummary {
		return true
	}
	_, toolResultOnly := entryTexts(entry, raw)
	return toolResultOnly
}

func entryTitle(entry Entry) (string, bool) {
	var candidates []string
	switch entry.Type {
	case "custom-title":
		candidates = []string{entry.CustomTitle, entry.Title}
	case "ai-title":
		candidates = []string{entry.AITitle, entry.Title, entry.CustomTitle}
	case "title":
		candidates = []string{entry.Title, entry.CustomTitle, entry.AITitle}
	default:
		return "", false
	}

	for _, title := range candidates {
		title = strings.TrimSpace(title)
		if title != "" {
			return title, true
		}
	}
	return "", false
}

func entryTexts(entry Entry, raw map[string]json.RawMessage) ([]string, bool) {
	var texts []string
	toolResultOnly := false

	if entry.Message.Text != "" {
		texts = append(texts, entry.Message.Text)
	}
	if len(entry.Message.Content) > 0 {
		contentTexts, onlyToolResult := contentTexts(entry.Message.Content)
		texts = append(texts, contentTexts...)
		toolResultOnly = onlyToolResult
	}
	if len(texts) == 0 {
		if rawText := stringField(raw, "text"); rawText != "" {
			texts = append(texts, rawText)
		}
		if content, ok := raw["content"]; ok {
			contentTexts, onlyToolResult := contentTexts(content)
			texts = append(texts, contentTexts...)
			toolResultOnly = onlyToolResult
		}
		if attachment, ok := raw["attachment"]; ok {
			texts = append(texts, objectTextFields(attachment)...)
		}
	}

	clean := texts[:0]
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text != "" {
			clean = append(clean, text)
		}
	}
	return clean, toolResultOnly && len(clean) == 0
}

func contentTexts(raw json.RawMessage) ([]string, bool) {
	rawText := strings.TrimSpace(string(raw))
	if rawText == "" || rawText == "null" {
		return nil, false
	}

	if text := rawString(raw); text != "" {
		return []string{text}, false
	}

	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var texts []string
		hasToolResult := false
		for _, block := range blocks {
			blockType := stringField(block, "type")
			if blockType == "tool_result" {
				hasToolResult = true
				continue
			}
			if blockType == "text" || blockType == "" {
				if text := stringField(block, "text"); text != "" {
					texts = append(texts, text)
				}
			}
		}
		return texts, hasToolResult && len(texts) == 0
	}

	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err == nil {
		blockType := stringField(block, "type")
		if blockType == "tool_result" {
			return nil, true
		}
		if blockType == "text" || blockType == "" {
			if text := stringField(block, "text"); text != "" {
				return []string{text}, false
			}
		}
	}

	return nil, false
}

func objectTextFields(raw json.RawMessage) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	var texts []string
	for _, field := range []string{"text", "content", "name"} {
		if text := stringField(obj, field); text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

func stringField(raw map[string]json.RawMessage, field string) string {
	value, ok := raw[field]
	if !ok {
		return ""
	}
	return rawString(value)
}

func rawString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func normalizeMessageText(texts []string) string {
	normalized := make([]string, 0, len(texts))
	for _, text := range texts {
		text = normalizePromptText(text)
		if text != "" {
			normalized = append(normalized, text)
		}
	}
	return strings.Join(normalized, " ")
}

func normalizePromptText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	return strings.TrimSpace(text)
}

func truncatePrompt(text string) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= firstPromptLimit {
		return text
	}
	return strings.TrimSpace(string(runes[:firstPromptLimit])) + "..."
}

func extractTag(text, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(text, open)
	if start < 0 {
		return ""
	}
	start += len(open)
	end := strings.Index(text[start:], close)
	if end < 0 {
		return ""
	}
	return text[start : start+end]
}
