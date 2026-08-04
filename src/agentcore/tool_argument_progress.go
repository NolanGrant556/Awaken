package agentcore

import (
	"path"
	"strconv"
	"strings"
	"unicode"

	"awaken/src/llm"
)

const (
	toolPreparationWrite = "write"
	toolPreparationEdit  = "edit"
	maxJSONKeyBytes      = 64
	maxJSONPathBytes     = 4096
	maxDisplayPathRunes  = 120
)

type toolArgumentProgress struct {
	Kind         string
	Path         string
	AddedLines   int
	DeletedLines int
}

type toolArgumentProgressTracker struct {
	calls map[string]*partialToolArgumentState
}

type partialToolArgumentState struct {
	kind    string
	scanner partialJSONObjectScanner
}

func (t *toolArgumentProgressTracker) consume(delta llm.ToolCallDelta) (toolArgumentProgress, bool) {
	key := streamedToolCallKey(delta)
	if t.calls == nil {
		t.calls = make(map[string]*partialToolArgumentState)
	}
	state := t.calls[key]
	if state == nil {
		state = &partialToolArgumentState{}
		t.calls[key] = state
	}
	if kind := preparationKindForTool(delta.Name); kind != "" {
		state.kind = kind
	}
	state.scanner.consume(delta.ArgumentsDelta)
	if state.kind == "" {
		return toolArgumentProgress{}, false
	}

	progress := toolArgumentProgress{
		Kind: state.kind,
		Path: safePreparationBasename(state.scanner.path),
	}
	switch state.kind {
	case toolPreparationWrite:
		progress.AddedLines = state.scanner.content.lines()
	case toolPreparationEdit:
		progress.AddedLines = state.scanner.newString.lines()
		progress.DeletedLines = state.scanner.oldString.lines()
	}
	return progress, true
}

func (t *toolArgumentProgressTracker) reset() {
	t.calls = nil
}

func streamedToolCallKey(delta llm.ToolCallDelta) string {
	if strings.TrimSpace(delta.ID) != "" {
		return delta.ID
	}
	return "tool_index_" + strconv.Itoa(delta.Index)
}

func preparationKindForTool(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write":
		return toolPreparationWrite
	case "edit":
		return toolPreparationEdit
	default:
		return ""
	}
}

func safePreparationBasename(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	value = strings.TrimRight(strings.ReplaceAll(value, "\\", "/"), "/")
	if value == "" {
		return ""
	}
	base := path.Base(value)
	if base == "." || base == ".." || base == "/" {
		return ""
	}
	runes := []rune(base)
	if len(runes) > maxDisplayPathRunes {
		base = string(runes[:maxDisplayPathRunes-1]) + "…"
	}
	return base
}

type partialJSONStringLines struct {
	hasContent bool
	newlines   int
}

func (s partialJSONStringLines) lines() int {
	if !s.hasContent {
		return 0
	}
	return s.newlines + 1
}

type partialJSONStringRole uint8

const (
	partialJSONStringIgnored partialJSONStringRole = iota
	partialJSONStringKey
	partialJSONStringPath
	partialJSONStringContent
	partialJSONStringOld
	partialJSONStringNew
)

// partialJSONObjectScanner consumes an incomplete, streamed JSON object without
// retaining file contents. It recognizes only top-level string fields needed
// for Write/Edit preparation UI. All other values are skipped.
type partialJSONObjectScanner struct {
	depth          int
	inString       bool
	escaped        bool
	unicodeDigits  int
	unicodeValue   rune
	role           partialJSONStringRole
	expectingKey   bool
	expectingValue bool
	afterKey       bool
	currentKey     string
	raw            strings.Builder
	rawOverflow    bool
	path           string
	content        partialJSONStringLines
	oldString      partialJSONStringLines
	newString      partialJSONStringLines
}

func (s *partialJSONObjectScanner) consume(fragment string) {
	for i := 0; i < len(fragment); i++ {
		ch := fragment[i]
		if s.inString {
			s.consumeStringByte(ch)
			continue
		}

		switch ch {
		case '{', '[':
			s.depth++
			if s.depth == 1 {
				s.expectingKey = true
			}
		case '}', ']':
			if s.depth > 0 {
				s.depth--
			}
		case '"':
			s.startString()
		case ':':
			if s.depth == 1 && s.afterKey {
				s.afterKey = false
				s.expectingValue = true
			}
		case ',':
			if s.depth == 1 {
				s.expectingKey = true
				s.expectingValue = false
				s.afterKey = false
				s.currentKey = ""
			}
		}
	}
}

func (s *partialJSONObjectScanner) startString() {
	s.inString = true
	s.escaped = false
	s.unicodeDigits = 0
	s.unicodeValue = 0
	s.raw.Reset()
	s.rawOverflow = false
	s.role = partialJSONStringIgnored

	if s.depth != 1 {
		return
	}
	if s.expectingKey {
		s.role = partialJSONStringKey
		s.expectingKey = false
		return
	}
	if !s.expectingValue {
		return
	}
	s.role = partialJSONStringRoleForValue(s.currentKey)
}

func partialJSONStringRoleForValue(key string) partialJSONStringRole {
	switch key {
	case "file_path", "filePath", "path", "target":
		return partialJSONStringPath
	case "content":
		return partialJSONStringContent
	case "old_string":
		return partialJSONStringOld
	case "new_string":
		return partialJSONStringNew
	default:
		return partialJSONStringIgnored
	}
}

func (s *partialJSONObjectScanner) consumeStringByte(ch byte) {
	if s.role == partialJSONStringContent || s.role == partialJSONStringOld || s.role == partialJSONStringNew {
		s.consumeCountedStringByte(ch)
		return
	}

	if s.escaped {
		s.appendRaw(ch)
		s.escaped = false
		return
	}
	if ch == '\\' {
		s.appendRaw(ch)
		s.escaped = true
		return
	}
	if ch == '"' {
		s.finishString()
		return
	}
	s.appendRaw(ch)
}

func (s *partialJSONObjectScanner) consumeCountedStringByte(ch byte) {
	stats := s.currentLineStats()
	if stats == nil {
		return
	}
	if s.unicodeDigits > 0 {
		if value, ok := hexDigit(ch); ok {
			s.unicodeValue = s.unicodeValue*16 + rune(value)
			s.unicodeDigits--
			if s.unicodeDigits == 0 {
				stats.hasContent = true
				if s.unicodeValue == '\n' {
					stats.newlines++
				}
			}
			return
		}
		// Invalid partial JSON is not interpreted further. The canonical full
		// arguments remain the execution authority.
		s.unicodeDigits = 0
		s.unicodeValue = 0
		stats.hasContent = true
	}
	if s.escaped {
		s.escaped = false
		if ch == 'u' {
			s.unicodeDigits = 4
			s.unicodeValue = 0
			return
		}
		stats.hasContent = true
		if ch == 'n' {
			stats.newlines++
		}
		return
	}
	if ch == '\\' {
		s.escaped = true
		return
	}
	if ch == '"' {
		s.finishString()
		return
	}
	stats.hasContent = true
	if ch == '\n' {
		stats.newlines++
	}
}

func (s *partialJSONObjectScanner) currentLineStats() *partialJSONStringLines {
	switch s.role {
	case partialJSONStringContent:
		return &s.content
	case partialJSONStringOld:
		return &s.oldString
	case partialJSONStringNew:
		return &s.newString
	default:
		return nil
	}
}

func (s *partialJSONObjectScanner) finishString() {
	role := s.role
	s.inString = false
	s.escaped = false
	s.unicodeDigits = 0
	s.unicodeValue = 0
	s.role = partialJSONStringIgnored

	if role == partialJSONStringKey {
		if decoded, ok := s.decodedRaw(); ok {
			s.currentKey = decoded
		} else {
			s.currentKey = ""
		}
		s.afterKey = true
		return
	}
	if role == partialJSONStringPath {
		if decoded, ok := s.decodedRaw(); ok {
			s.path = decoded
		}
	}
	if s.depth == 1 {
		s.expectingValue = false
	}
}

func (s *partialJSONObjectScanner) appendRaw(ch byte) {
	if s.rawOverflow {
		return
	}
	limit := maxJSONPathBytes
	if s.role == partialJSONStringKey {
		limit = maxJSONKeyBytes
	}
	if s.raw.Len()+1 > limit {
		s.rawOverflow = true
		s.raw.Reset()
		return
	}
	s.raw.WriteByte(ch)
}

func (s *partialJSONObjectScanner) decodedRaw() (string, bool) {
	if s.rawOverflow {
		return "", false
	}
	decoded, err := strconv.Unquote(`"` + s.raw.String() + `"`)
	if err != nil {
		return "", false
	}
	return decoded, true
}

func hexDigit(ch byte) (int, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10, true
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10, true
	default:
		return 0, false
	}
}
