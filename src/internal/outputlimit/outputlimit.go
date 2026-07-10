package outputlimit

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const DefaultToolResultMaxChars = 20000

type Result struct {
	Content       string
	Truncated     bool
	OriginalChars int
}

// Prefix returns at most maxChars Unicode code points from content. The
// boolean reports whether any characters were omitted. Converting through
// []rune also guarantees callers never persist or display a partial UTF-8
// sequence when a string arrived with malformed bytes.
func Prefix(content string, maxChars int) (string, bool) {
	runes := []rune(content)
	if maxChars <= 0 {
		return "", len(runes) > 0
	}
	if len(runes) <= maxChars {
		return string(runes), false
	}
	return string(runes[:maxChars]), true
}

func Limit(content string, maxChars int) Result {
	originalChars := utf8.RuneCountInString(content)
	if maxChars <= 0 || originalChars <= maxChars {
		return Result{Content: content, OriginalChars: originalChars}
	}

	runes := []rune(content)
	notice := fmt.Sprintf("\n\n[... truncated: omitted %d chars ...]\n\n", originalChars-maxChars)
	noticeRunes := []rune(notice)
	if maxChars <= len(noticeRunes)+2 {
		return Result{
			Content:       strings.TrimSpace(notice),
			Truncated:     true,
			OriginalChars: originalChars,
		}
	}

	remaining := maxChars - len(noticeRunes)
	headChars := remaining / 2
	tailChars := remaining - headChars
	if headChars < 1 {
		headChars = 1
	}
	if tailChars < 1 {
		tailChars = 1
	}

	return Result{
		Content:       string(runes[:headChars]) + notice + string(runes[len(runes)-tailChars:]),
		Truncated:     true,
		OriginalChars: originalChars,
	}
}

type CappedBuffer struct {
	maxChars  int
	seenChars int
	full      []rune
	head      []rune
	tail      []rune
	headLimit int
	tailLimit int
}

func NewCappedBuffer(maxChars int) *CappedBuffer {
	headLimit := maxChars / 2
	tailLimit := maxChars - headLimit
	if headLimit < 1 {
		headLimit = 1
	}
	if tailLimit < 1 {
		tailLimit = 1
	}
	return &CappedBuffer{
		maxChars:  maxChars,
		headLimit: headLimit,
		tailLimit: tailLimit,
	}
}

func (b *CappedBuffer) Write(p []byte) (int, error) {
	if b.maxChars <= 0 {
		return len(p), nil
	}
	for _, r := range string(p) {
		b.seenChars++
		if b.seenChars <= b.maxChars {
			b.full = append(b.full, r)
			continue
		}

		if b.head == nil {
			b.head = append([]rune(nil), b.full[:b.headLimit]...)
			b.tail = append([]rune(nil), b.full[len(b.full)-b.tailLimit:]...)
			b.full = nil
		}
		b.tail = append(b.tail, r)
		if len(b.tail) > b.tailLimit {
			b.tail = b.tail[len(b.tail)-b.tailLimit:]
		}
	}
	return len(p), nil
}

func (b *CappedBuffer) Result() Result {
	if b.seenChars <= b.maxChars {
		return Result{Content: string(b.full), OriginalChars: b.seenChars}
	}

	notice := fmt.Sprintf("\n\n[... truncated: omitted %d chars ...]\n\n", b.seenChars-len(b.head)-len(b.tail))
	return Result{
		Content:       string(b.head) + notice + string(b.tail),
		Truncated:     true,
		OriginalChars: b.seenChars,
	}
}
