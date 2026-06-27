package transcriptjsonl

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

const maxPromptChars = 200

var (
	utf8BOM           = []byte{0xef, 0xbb, 0xbf}
	uuidPattern       = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	skipFirstPromptRE = regexp.MustCompile(`^(?:\s*<[a-z][\w-]*[\s>]|\[Request interrupted by user[^\]]*\])`)
)

// Entry is a lightweight JSONL transcript entry.
//
// Common top-level fields are promoted for callers that need a stable handle,
// while Fields and RawLine keep the parser from binding to one transcript
// schema too early.
type Entry struct {
	Type        string
	UUID        string
	ParentUUID  string
	SessionID   string
	Timestamp   string
	IsSidechain bool
	Message     map[string]any
	Fields      map[string]any
	RawLine     string
}

// SkippedLine summarizes a line that was ignored by ParseLines.
type SkippedLine struct {
	Line    int
	Offset  int
	Summary string
	Preview string
}

// ParseResult is the output of parsing a JSONL transcript chunk.
type ParseResult struct {
	Entries []Entry
	Skipped int
	Errors  []SkippedLine
}

// ParseString parses JSONL transcript data from a string.
func ParseString(data string) ParseResult {
	return ParseLines([]byte(data))
}

// ParseLines parses JSONL transcript data from bytes.
//
// Empty lines are ignored. Malformed lines and non-object JSON values are
// skipped and reported in the result summaries.
func ParseLines(data []byte) ParseResult {
	var result ParseResult
	start := 0
	lineNo := 1

	if bytes.HasPrefix(data, utf8BOM) {
		start = len(utf8BOM)
	}

	for start < len(data) {
		lineStart := start
		end := len(data)
		if i := bytes.IndexByte(data[start:], '\n'); i >= 0 {
			end = start + i
		}

		line := bytes.TrimSpace(data[start:end])
		if lineNo == 1 {
			line = bytes.TrimPrefix(line, utf8BOM)
		}

		if len(line) > 0 {
			entry, err := parseEntryLine(line)
			if err != nil {
				result.addSkipped(lineNo, lineStart, line, err.Error())
			} else {
				result.Entries = append(result.Entries, entry)
			}
		}

		if end == len(data) {
			break
		}
		start = end + 1
		lineNo++
	}

	return result
}

// ValidateUUID reports whether s has the canonical UUID shape used by Claude
// transcripts.
func ValidateUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// ExtractStringField extracts the first JSON string field named key from raw
// text without fully parsing the line. It can succeed on a truncated or
// otherwise malformed line as long as the target string value is complete.
func ExtractStringField(rawLine, key string) (string, bool) {
	searchFrom := 0
	for searchFrom < len(rawLine) {
		value, next, ok := findStringField(rawLine, key, searchFrom)
		if next <= searchFrom {
			searchFrom++
			continue
		}
		if ok {
			return value, true
		}
		searchFrom = next
	}
	return "", false
}

// ExtractLastStringField extracts the last JSON string field named key from a
// byte chunk without fully parsing it.
func ExtractLastStringField(data []byte, key string) (string, bool) {
	return ExtractLastStringFieldString(string(data), key)
}

// ExtractLastStringFieldString extracts the last JSON string field named key
// from a string chunk without fully parsing it.
func ExtractLastStringFieldString(data, key string) (string, bool) {
	var last string
	found := false
	searchFrom := 0
	for searchFrom < len(data) {
		value, next, ok := findStringField(data, key, searchFrom)
		if next <= searchFrom {
			searchFrom++
			continue
		}
		searchFrom = next
		if ok {
			last = value
			found = true
		}
	}
	return last, found
}

// ExtractFirstPromptFromHead returns the first meaningful user prompt found in
// a JSONL head chunk.
func ExtractFirstPromptFromHead(data []byte) string {
	return ExtractFirstPromptFromHeadString(string(data))
}

// ExtractFirstPromptFromHeadString returns the first meaningful user prompt
// found in a JSONL head chunk.
func ExtractFirstPromptFromHeadString(head string) string {
	start := 0
	lineNo := 1
	for start < len(head) {
		end := len(head)
		if i := strings.IndexByte(head[start:], '\n'); i >= 0 {
			end = start + i
		}

		line := strings.TrimSpace(head[start:end])
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line != "" {
			if prompt := firstPromptFromLine(line); prompt != "" {
				return prompt
			}
		}

		if end == len(head) {
			break
		}
		start = end + 1
		lineNo++
	}
	return ""
}

func parseEntryLine(line []byte) (Entry, error) {
	var fields map[string]any
	if err := json.Unmarshal(line, &fields); err != nil {
		return Entry{}, err
	}
	if fields == nil {
		return Entry{}, errNotObject{}
	}
	return entryFromFields(fields, string(line)), nil
}

type errNotObject struct{}

func (errNotObject) Error() string {
	return "JSON value is not an object"
}

func entryFromFields(fields map[string]any, rawLine string) Entry {
	entry := Entry{
		Fields:  fields,
		RawLine: rawLine,
	}
	entry.Type, _ = stringField(fields, "type")
	entry.UUID, _ = stringField(fields, "uuid")
	entry.ParentUUID, _ = stringField(fields, "parentUuid")
	entry.SessionID, _ = stringField(fields, "sessionId")
	entry.Timestamp, _ = stringField(fields, "timestamp")
	entry.IsSidechain, _ = boolField(fields, "isSidechain")
	if message, ok := fields["message"].(map[string]any); ok {
		entry.Message = message
	}
	return entry
}

func (r *ParseResult) addSkipped(lineNo, offset int, line []byte, summary string) {
	r.Skipped++
	r.Errors = append(r.Errors, SkippedLine{
		Line:    lineNo,
		Offset:  offset,
		Summary: summary,
		Preview: previewLine(line),
	})
}

func previewLine(line []byte) string {
	const limit = 160
	preview := string(line)
	preview = strings.ReplaceAll(preview, "\r", " ")
	preview = strings.ReplaceAll(preview, "\n", " ")
	if len(preview) <= limit {
		return preview
	}
	return preview[:limit]
}

func stringField(fields map[string]any, key string) (string, bool) {
	value, ok := fields[key].(string)
	return value, ok
}

func boolField(fields map[string]any, key string) (bool, bool) {
	value, ok := fields[key].(bool)
	return value, ok
}

func findStringField(text, key string, from int) (string, int, bool) {
	if from < 0 {
		from = 0
	}
	quotedKey := strconv.Quote(key)
	searchFrom := from

	for searchFrom < len(text) {
		relative := strings.Index(text[searchFrom:], quotedKey)
		if relative < 0 {
			return "", len(text), false
		}
		idx := searchFrom + relative
		next := idx + len(quotedKey)
		j := next

		for j < len(text) && isJSONWhitespace(text[j]) {
			j++
		}
		if j >= len(text) || text[j] != ':' {
			searchFrom = next
			continue
		}
		j++
		for j < len(text) && isJSONWhitespace(text[j]) {
			j++
		}
		if j >= len(text) || text[j] != '"' {
			searchFrom = next
			continue
		}

		valueStart := j + 1
		for i := valueStart; i < len(text); i++ {
			switch text[i] {
			case '\\':
				i++
			case '"':
				raw := text[valueStart:i]
				unescaped, err := strconv.Unquote(`"` + raw + `"`)
				if err != nil {
					unescaped = raw
				}
				return unescaped, i + 1, true
			}
		}
		return "", next, false
	}

	return "", len(text), false
}

func isJSONWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func firstPromptFromLine(line string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		return ""
	}
	if typ, _ := stringField(fields, "type"); typ != "user" {
		return ""
	}
	if isMeta, _ := boolField(fields, "isMeta"); isMeta {
		return ""
	}
	if isCompactSummary, _ := boolField(fields, "isCompactSummary"); isCompactSummary {
		return ""
	}

	message, ok := fields["message"].(map[string]any)
	if !ok {
		return ""
	}
	content := message["content"]
	if hasToolResultContent(content) {
		return ""
	}

	for _, text := range contentTexts(content) {
		prompt := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
		if prompt == "" {
			continue
		}
		if _, ok := extractTag(prompt, "command-name"); ok {
			continue
		}
		if bash, ok := extractTag(prompt, "bash-input"); ok {
			return truncatePrompt("! " + strings.TrimSpace(bash))
		}
		if skipFirstPromptRE.MatchString(prompt) {
			continue
		}
		return truncatePrompt(prompt)
	}

	return ""
}

func hasToolResultContent(content any) bool {
	blocks, ok := content.([]any)
	if !ok {
		return false
	}
	for _, block := range blocks {
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := stringField(blockMap, "type"); typ == "tool_result" {
			return true
		}
	}
	return false
}

func contentTexts(content any) []string {
	switch value := content.(type) {
	case string:
		return []string{value}
	case []any:
		texts := make([]string, 0, len(value))
		for _, block := range value {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			if typ, _ := stringField(blockMap, "type"); typ != "text" {
				continue
			}
			if text, ok := stringField(blockMap, "text"); ok {
				texts = append(texts, text)
			}
		}
		return texts
	default:
		return nil
	}
}

func extractTag(text, tag string) (string, bool) {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(text, open)
	if start < 0 {
		return "", false
	}
	valueStart := start + len(open)
	end := strings.Index(text[valueStart:], close)
	if end < 0 {
		return "", false
	}
	return text[valueStart : valueStart+end], true
}

func truncatePrompt(prompt string) string {
	runes := []rune(prompt)
	if len(runes) <= maxPromptChars {
		return prompt
	}
	return strings.TrimSpace(string(runes[:maxPromptChars]))
}
