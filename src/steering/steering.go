package steering

import (
	"fmt"
	"strings"
	"sync"
)

const guidancePrefix = "User guidance: "

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Message struct {
	Role    string
	Content any
}

type Report struct {
	Applied     bool
	TargetIndex int
	Drained     string
	Restored    bool
}

type ApplyReport = Report

type Buffer struct {
	mu      sync.Mutex
	pending string
}

type Manager = Buffer

func NewBuffer() *Buffer {
	return &Buffer{}
}

func NewManager() *Manager {
	return NewBuffer()
}

func (b *Buffer) Add(text string) bool {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return false
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == "" {
		b.pending = cleaned
	} else {
		b.pending += "\n" + cleaned
	}
	return true
}

func (b *Buffer) Drain() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.drainLocked()
}

func (b *Buffer) ClearOnInterrupt() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = ""
}

func (b *Buffer) Leftover() string {
	return b.Drain()
}

func (b *Buffer) ApplyToRecentToolResult(messages []Message, recentToolResults int) ([]Message, Report) {
	report := Report{TargetIndex: -1}
	if recentToolResults <= 0 || len(messages) == 0 {
		return messages, report
	}

	start := len(messages) - recentToolResults
	if start < 0 {
		start = 0
	}
	return b.apply(messages, start)
}

func (b *Buffer) ApplyToLastToolResult(messages []Message) ([]Message, Report) {
	return b.apply(messages, 0)
}

func (b *Buffer) apply(messages []Message, start int) ([]Message, Report) {
	report := Report{TargetIndex: -1}
	steerText := b.Drain()
	if steerText == "" {
		return messages, report
	}
	report.Drained = steerText

	targetIndex := -1
	for i := len(messages) - 1; i >= start; i-- {
		if messages[i].Role == "tool" {
			targetIndex = i
			break
		}
	}
	if targetIndex == -1 {
		b.restore(steerText)
		report.Restored = true
		return messages, report
	}

	next := append([]Message(nil), messages...)
	next[targetIndex].Content = appendGuidance(next[targetIndex].Content, steerText)
	report.Applied = true
	report.TargetIndex = targetIndex
	return next, report
}

func (b *Buffer) restore(text string) {
	if text == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending == "" {
		b.pending = text
	} else {
		b.pending += "\n" + text
	}
}

func (b *Buffer) drainLocked() string {
	text := b.pending
	b.pending = ""
	return text
}

func appendGuidance(content any, steerText string) any {
	marker := "\n\n" + guidancePrefix + steerText
	switch existing := content.(type) {
	case string:
		return existing + marker
	case []ContentBlock:
		blocks := append([]ContentBlock(nil), existing...)
		return append(blocks, ContentBlock{Type: "text", Text: guidancePrefix + steerText})
	case []map[string]any:
		blocks := append([]map[string]any(nil), existing...)
		return append(blocks, map[string]any{"type": "text", "text": guidancePrefix + steerText})
	case []any:
		blocks := append([]any(nil), existing...)
		return append(blocks, ContentBlock{Type: "text", Text: guidancePrefix + steerText})
	case nil:
		return marker
	default:
		return fmt.Sprint(existing) + marker
	}
}
