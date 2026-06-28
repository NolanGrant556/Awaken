package texttoolparser

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Result struct {
	Content   string
	ToolCalls []ToolCall
	Found     bool
}

type Parser interface {
	Parse(text string) Result
}

type ParserFunc func(text string) Result

func (fn ParserFunc) Parse(text string) Result {
	return fn(text)
}

type Registry struct {
	parsers map[string]Parser
}

func NewRegistry() *Registry {
	registry := &Registry{parsers: map[string]Parser{}}
	registry.Register("hermes", ParserFunc(parseHermes))
	registry.Register("qwen", ParserFunc(parseHermes))
	registry.Register("kimi", ParserFunc(parseKimiK2))
	registry.Register("kimi-k2", ParserFunc(parseKimiK2))
	registry.Register("kimi_k2", ParserFunc(parseKimiK2))
	return registry
}

func (registry *Registry) Register(format string, parser Parser) {
	if registry == nil || parser == nil {
		return
	}
	format = normalizeFormat(format)
	if format == "" {
		return
	}
	registry.parsers[format] = parser
}

func (registry *Registry) Parse(format, text string) Result {
	if registry == nil {
		return notFound(text)
	}
	parser, ok := registry.parsers[normalizeFormat(format)]
	if !ok {
		return notFound(text)
	}
	return parser.Parse(text)
}

var DefaultRegistry = NewRegistry()

func Parse(format, text string) Result {
	return DefaultRegistry.Parse(format, text)
}

func normalizeFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

func notFound(text string) Result {
	return Result{Content: text, Found: false}
}

var hermesPattern = regexp.MustCompile(`(?s)<tool_call>\s*(.*?)\s*</tool_call>|<tool_call>\s*(.*)$`)

type hermesPayload struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func parseHermes(text string) Result {
	if !strings.Contains(text, "<tool_call>") {
		return notFound(text)
	}

	matches := hermesPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return notFound(text)
	}

	fencedRanges := fencedCodeRanges(text)
	toolCalls := make([]ToolCall, 0, len(matches))
	stripRanges := make([]textRange, 0, len(matches))
	for _, match := range matches {
		if overlapsAnyRange(match[0], match[1], fencedRanges) {
			continue
		}
		rawJSON := submatch(text, match, 2)
		if rawJSON == "" {
			rawJSON = submatch(text, match, 4)
		}
		if strings.TrimSpace(rawJSON) == "" {
			continue
		}

		payload, ok := decodeHermesPayload(rawJSON)
		if !ok {
			return notFound(text)
		}
		if strings.TrimSpace(payload.Name) == "" {
			continue
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        "call_texttool_" + strconv.Itoa(len(toolCalls)),
			Name:      payload.Name,
			Arguments: payload.Arguments,
		})
		stripRanges = append(stripRanges, textRange{start: match[0], end: match[1]})
	}

	if len(toolCalls) == 0 {
		return notFound(text)
	}
	return Result{
		Content:   stripTextRanges(text, stripRanges),
		ToolCalls: toolCalls,
		Found:     true,
	}
}

func decodeHermesPayload(rawJSON string) (ToolCall, bool) {
	var payload hermesPayload
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return ToolCall{}, false
	}

	arguments := "{}"
	if len(payload.Arguments) > 0 {
		compact, ok := compactJSONObject(payload.Arguments)
		if !ok {
			return ToolCall{}, false
		}
		arguments = compact
	}

	return ToolCall{
		Name:      strings.TrimSpace(payload.Name),
		Arguments: arguments,
	}, true
}

var kimiPattern = regexp.MustCompile(`(?s)<\|tool_call_begin\|>\s*([^<]+?:\d+)\s*<\|tool_call_argument_begin\|>\s*(.*?)\s*<\|tool_call_end\|>`)

var kimiStartTokens = []string{
	"<|tool_calls_section_begin|>",
	"<|tool_call_section_begin|>",
}

var kimiEndTokens = []string{
	"<|tool_calls_section_end|>",
	"<|tool_call_section_end|>",
}

func parseKimiK2(text string) Result {
	if findEarliestToken(text, kimiStartTokens) < 0 {
		return notFound(text)
	}

	matches := kimiPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return notFound(text)
	}

	fencedRanges := fencedCodeRanges(text)
	toolCalls := make([]ToolCall, 0, len(matches))
	for _, match := range matches {
		if overlapsAnyRange(match[0], match[1], fencedRanges) {
			continue
		}
		functionID := strings.TrimSpace(submatch(text, match, 2))
		name := kimiFunctionName(functionID)
		if name == "" {
			continue
		}

		arguments, ok := compactJSONObjectString(submatch(text, match, 4))
		if !ok {
			return notFound(text)
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        functionID,
			Name:      name,
			Arguments: arguments,
		})
	}

	if len(toolCalls) == 0 {
		return notFound(text)
	}
	return Result{
		Content:   stripKimiSections(text),
		ToolCalls: toolCalls,
		Found:     true,
	}
}

func kimiFunctionName(functionID string) string {
	beforeColon, _, ok := strings.Cut(functionID, ":")
	if !ok {
		return ""
	}
	name := beforeColon
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	return strings.TrimSpace(name)
}

func compactJSONString(raw string) (string, bool) {
	return compactJSON(json.RawMessage(strings.TrimSpace(raw)))
}

func compactJSONObjectString(raw string) (string, bool) {
	return compactJSONObject(json.RawMessage(strings.TrimSpace(raw)))
}

func compactJSON(raw json.RawMessage) (string, bool) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", false
	}
	return buf.String(), true
}

func compactJSONObject(raw json.RawMessage) (string, bool) {
	compact, ok := compactJSON(raw)
	if !ok {
		return "", false
	}
	if !isJSONObject(compact) {
		return "", false
	}
	return compact, true
}

func isJSONObject(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

type textRange struct {
	start int
	end   int
}

func submatch(text string, match []int, startIndex int) string {
	if startIndex+1 >= len(match) || match[startIndex] < 0 || match[startIndex+1] < 0 {
		return ""
	}
	return text[match[startIndex]:match[startIndex+1]]
}

func stripTextRanges(text string, ranges []textRange) string {
	if len(ranges) == 0 {
		return strings.TrimSpace(text)
	}

	var builder strings.Builder
	cursor := 0
	for _, r := range ranges {
		if r.start > cursor {
			builder.WriteString(text[cursor:r.start])
		}
		if r.end > cursor {
			cursor = r.end
		}
	}
	if cursor < len(text) {
		builder.WriteString(text[cursor:])
	}
	return strings.TrimSpace(builder.String())
}

func stripKimiSections(text string) string {
	ranges := make([]textRange, 0)
	cursor := 0
	for {
		start := findEarliestTokenFrom(text, kimiStartTokens, cursor)
		if start < 0 {
			break
		}

		end := findEarliestTokenFrom(text, kimiEndTokens, start)
		if end < 0 {
			ranges = append(ranges, textRange{start: start, end: len(text)})
			break
		}

		endToken := tokenAt(text, kimiEndTokens, end)
		rangeEnd := end + len(endToken)
		ranges = append(ranges, textRange{start: start, end: rangeEnd})
		cursor = rangeEnd
	}
	return stripTextRanges(text, ranges)
}

func fencedCodeRanges(text string) []textRange {
	ranges := make([]textRange, 0)
	cursor := 0
	for {
		start := strings.Index(text[cursor:], "```")
		if start < 0 {
			break
		}
		start += cursor
		end := strings.Index(text[start+3:], "```")
		if end < 0 {
			ranges = append(ranges, textRange{start: start, end: len(text)})
			break
		}
		end += start + 6
		ranges = append(ranges, textRange{start: start, end: end})
		cursor = end
	}
	return ranges
}

func overlapsAnyRange(start, end int, ranges []textRange) bool {
	for _, r := range ranges {
		if start < r.end && end > r.start {
			return true
		}
	}
	return false
}

func findEarliestToken(text string, tokens []string) int {
	return findEarliestTokenFrom(text, tokens, 0)
}

func findEarliestTokenFrom(text string, tokens []string, offset int) int {
	if offset < 0 || offset > len(text) {
		return -1
	}
	earliest := -1
	for _, token := range tokens {
		idx := strings.Index(text[offset:], token)
		if idx < 0 {
			continue
		}
		idx += offset
		if earliest < 0 || idx < earliest {
			earliest = idx
		}
	}
	return earliest
}

func tokenAt(text string, tokens []string, offset int) string {
	for _, token := range tokens {
		if strings.HasPrefix(text[offset:], token) {
			return token
		}
	}
	return ""
}
