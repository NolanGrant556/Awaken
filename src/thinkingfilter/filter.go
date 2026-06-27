package thinkingfilter

import (
	"regexp"
	"strings"
	"unicode"
)

var reasoningTags = []string{
	"REASONING_SCRATCHPAD",
	"think",
	"thinking",
	"reasoning",
	"thought",
}

var toolBlockTags = []string{
	"tool_call",
	"tool_calls",
	"tool_result",
	"function_call",
	"function_calls",
}

var nameAttrPattern = regexp.MustCompile(`(?i)\bname\s*=`)

// StripReasoningTags removes hidden reasoning and tool-call XML blocks from a
// complete text value.
func StripReasoningTags(text string) string {
	var filter Filter
	return filter.Push(text) + filter.Flush()
}

// Filter incrementally removes hidden reasoning and tool-call XML blocks from
// streamed text deltas. The zero value is ready to use.
type Filter struct {
	inBlock      bool
	closeTags    []string
	pending      string
	lineBoundary bool
	initialized  bool
	literalTags  map[string]int
}

// Push filters one streamed delta and returns only the visible text.
func (f *Filter) Push(delta string) string {
	if delta == "" {
		return ""
	}
	f.ensureInitialized()

	buf := f.pending + delta
	f.pending = ""

	var out strings.Builder
	for len(buf) > 0 {
		if f.inBlock {
			idx, length := findClose(buf, f.closeTags)
			if idx >= 0 {
				f.inBlock = false
				f.closeTags = nil
				buf = buf[idx+length:]
				continue
			}

			held := partialCloseSuffixLen(buf, f.closeTags)
			if held > 0 {
				f.pending = buf[len(buf)-held:]
			}
			return out.String()
		}

		action, ok := f.nextOutsideAction(buf)
		if !ok {
			held := partialTagSuffixLen(buf)
			visible := buf
			if held > 0 {
				visible = buf[:len(buf)-held]
				f.pending = buf[len(buf)-held:]
			}
			out.WriteString(visible)
			f.updateLineBoundary(visible)
			return out.String()
		}

		if action.index > 0 {
			visible := buf[:action.index]
			out.WriteString(visible)
			f.updateLineBoundary(visible)
		}

		if action.open {
			f.inBlock = true
			f.closeTags = action.closeTags
		}
		buf = buf[action.index+action.length:]
	}

	return out.String()
}

// Flush returns any visible text still held while waiting for a possible split
// tag. If the stream ends inside a hidden block, that unfinished block is
// discarded.
func (f *Filter) Flush() string {
	f.ensureInitialized()
	if f.inBlock {
		f.inBlock = false
		f.closeTags = nil
		f.pending = ""
		return ""
	}

	visible := f.pending
	f.pending = ""
	f.updateLineBoundary(visible)
	return visible
}

type outsideAction struct {
	index     int
	length    int
	open      bool
	closeTags []string
}

func (f *Filter) ensureInitialized() {
	if f.initialized {
		return
	}
	f.lineBoundary = true
	f.initialized = true
	f.literalTags = make(map[string]int)
}

func (f *Filter) nextOutsideAction(buf string) (outsideAction, bool) {
	for idx := 0; idx < len(buf); idx++ {
		if buf[idx] != '<' {
			continue
		}

		if length, tag, ok := matchCloseAt(buf, idx, orphanCloseTags()); ok {
			if f.literalTags[tag] > 0 {
				f.literalTags[tag]--
				continue
			}
			return outsideAction{index: idx, length: length}, true
		}

		if length, closeTags, gated, tag, ok := matchOpenAt(buf, idx); ok {
			if gated && !f.isBlockBoundary(buf[:idx]) {
				f.literalTags[tag]++
				continue
			}
			return outsideAction{
				index:     idx,
				length:    length,
				open:      true,
				closeTags: closeTags,
			}, true
		}
	}

	return outsideAction{}, false
}

func matchOpenAt(buf string, idx int) (int, []string, bool, string, bool) {
	for _, tag := range reasoningTags {
		literal := "<" + tag + ">"
		if hasFoldPrefix(buf[idx:], literal) {
			return len(literal), reasoningTags, true, tag, true
		}
	}

	for _, tag := range toolBlockTags {
		if length, ok := matchVariableOpenAt(buf, idx, tag); ok {
			return length, []string{tag}, false, tag, true
		}
	}

	if length, ok := matchFunctionOpenAt(buf, idx); ok {
		return length, []string{"function"}, true, "function", true
	}

	return 0, nil, false, "", false
}

func matchVariableOpenAt(buf string, idx int, tag string) (int, bool) {
	prefix := "<" + tag
	if !hasFoldPrefix(buf[idx:], prefix) {
		return 0, false
	}

	afterName := idx + len(prefix)
	if afterName >= len(buf) {
		return 0, false
	}
	if buf[afterName] != '>' && !isTagWhitespace(rune(buf[afterName])) {
		return 0, false
	}

	end := strings.IndexByte(buf[afterName:], '>')
	if end < 0 {
		return 0, false
	}
	return afterName + end + 1 - idx, true
}

func matchFunctionOpenAt(buf string, idx int) (int, bool) {
	length, ok := matchVariableOpenAt(buf, idx, "function")
	if !ok {
		return 0, false
	}
	if !nameAttrPattern.MatchString(buf[idx : idx+length]) {
		return 0, false
	}
	return length, true
}

func findClose(buf string, closeTags []string) (int, int) {
	for idx := 0; idx < len(buf); idx++ {
		if buf[idx] != '<' {
			continue
		}
		if length, _, ok := matchCloseAt(buf, idx, closeTags); ok {
			return idx, length
		}
	}
	return -1, 0
}

func matchCloseAt(buf string, idx int, tags []string) (int, string, bool) {
	for _, tag := range tags {
		literal := "</" + tag + ">"
		if hasFoldPrefix(buf[idx:], literal) {
			return len(literal), tag, true
		}
	}
	return 0, "", false
}

func partialTagSuffixLen(buf string) int {
	held := 0
	for idx := strings.LastIndexByte(buf, '<'); idx >= 0; idx = lastIndexByteBefore(buf, '<', idx) {
		suffix := buf[idx:]
		if isPartialOpenSuffix(suffix) || isPartialCloseSuffix(suffix, orphanCloseTags()) {
			if len(suffix) > held {
				held = len(suffix)
			}
		}
	}
	return held
}

func partialCloseSuffixLen(buf string, closeTags []string) int {
	held := 0
	for idx := strings.LastIndexByte(buf, '<'); idx >= 0; idx = lastIndexByteBefore(buf, '<', idx) {
		suffix := buf[idx:]
		if isPartialCloseSuffix(suffix, closeTags) && len(suffix) > held {
			held = len(suffix)
		}
	}
	return held
}

func isPartialOpenSuffix(suffix string) bool {
	for _, tag := range reasoningTags {
		literal := "<" + tag + ">"
		if isFoldPartial(suffix, literal) {
			return true
		}
	}

	for _, tag := range append(append([]string{}, toolBlockTags...), "function") {
		if isPartialVariableOpenSuffix(suffix, tag) {
			return true
		}
	}
	return false
}

func isPartialVariableOpenSuffix(suffix string, tag string) bool {
	prefix := "<" + tag
	if len(suffix) <= len(prefix) {
		return isFoldPartial(suffix, prefix)
	}
	if !hasFoldPrefix(suffix, prefix) {
		return false
	}
	if strings.Contains(suffix, ">") {
		return false
	}
	return isTagWhitespace(rune(suffix[len(prefix)]))
}

func isPartialCloseSuffix(suffix string, tags []string) bool {
	for _, tag := range tags {
		literal := "</" + tag + ">"
		if isFoldPartial(suffix, literal) {
			return true
		}
	}
	return false
}

func isFoldPartial(suffix string, literal string) bool {
	return len(suffix) < len(literal) && strings.EqualFold(suffix, literal[:len(suffix)])
}

func hasFoldPrefix(s string, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func (f *Filter) isBlockBoundary(preceding string) bool {
	lastNewline := strings.LastIndexByte(preceding, '\n')
	if lastNewline >= 0 {
		return strings.TrimSpace(preceding[lastNewline+1:]) == ""
	}
	return f.lineBoundary && strings.TrimSpace(preceding) == ""
}

func (f *Filter) updateLineBoundary(text string) {
	for _, r := range text {
		if r == '\n' {
			f.lineBoundary = true
			clear(f.literalTags)
			continue
		}
		if !unicode.IsSpace(r) {
			f.lineBoundary = false
		}
	}
}

func isTagWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func orphanCloseTags() []string {
	tags := make([]string, 0, len(reasoningTags)+len(toolBlockTags)+1)
	tags = append(tags, reasoningTags...)
	tags = append(tags, toolBlockTags...)
	return tags
}

func lastIndexByteBefore(s string, c byte, before int) int {
	if before <= 0 {
		return -1
	}
	return strings.LastIndexByte(s[:before], c)
}
