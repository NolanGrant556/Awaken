package taskoutput

import (
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	DefaultMaxMemoryBytes = 8 * 1024 * 1024
	DefaultRecentLines    = 100

	maxRecentLineBytes = 4096
	stderrLinePrefix   = "[stderr] "
)

type Snapshot struct {
	Stdout      string
	Stderr      string
	RecentLines []string
	TotalLines  int
	TotalBytes  int64
	Incomplete  bool
	Spilled     bool
}

type Buffer struct {
	MaxMemoryBytes int

	mu             sync.Mutex
	stdout         string
	stderr         string
	recent         []recentLine
	stdoutLine     lineState
	stderrLine     lineState
	nextLineSeq    int64
	completedLines int
	totalBytes     int64
	incomplete     bool
	spilled        bool
}

func NewBuffer(maxMemoryBytes int) *Buffer {
	return &Buffer{MaxMemoryBytes: maxMemoryBytes}
}

func (b *Buffer) WriteStdout(data string) {
	b.write(streamStdout, data)
}

func (b *Buffer) WriteStderr(data string) {
	b.write(streamStderr, data)
}

func (b *Buffer) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	recent := make([]string, len(b.recent))
	for i, line := range b.recent {
		if line.stream == streamStderr {
			recent[i] = stderrLinePrefix + line.text
			continue
		}
		recent[i] = line.text
	}

	totalLines := b.completedLines
	if b.stdoutLine.open {
		totalLines++
	}
	if b.stderrLine.open {
		totalLines++
	}

	return Snapshot{
		Stdout:      b.stdout,
		Stderr:      b.stderr,
		RecentLines: recent,
		TotalLines:  totalLines,
		TotalBytes:  b.totalBytes,
		Incomplete:  b.incomplete || b.spilled,
		Spilled:     b.spilled,
	}
}

type Result struct {
	Code             int
	Interrupted      bool
	BackgroundTaskID string
	OutputFilePath   string
	OutputFileSize   int64
	PreSpawnError    string
}

func (r Result) Success() bool {
	return r.Code == 0 &&
		!r.Interrupted &&
		r.BackgroundTaskID == "" &&
		r.PreSpawnError == ""
}

func TailLines(text string, n int) []string {
	if n <= 0 || text == "" {
		return []string{}
	}

	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return append([]string(nil), lines...)
}

type outputStream int

const (
	streamStdout outputStream = iota
	streamStderr
)

type recentLine struct {
	stream outputStream
	seq    int64
	text   string
}

type lineState struct {
	open    bool
	text    string
	seq     int64
	visible bool
}

func (b *Buffer) write(stream outputStream, data string) {
	if data == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalBytes += int64(len(data))
	b.recordLines(stream, data)
	b.appendStoredOutput(stream, data)
}

func (b *Buffer) recordLines(stream outputStream, data string) {
	state := b.lineState(stream)
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '\n' {
			continue
		}
		b.appendLinePart(state, stream, data[start:i])
		b.completeLine(state)
		start = i + 1
	}
	if start < len(data) {
		b.appendLinePart(state, stream, data[start:])
	}
}

func (b *Buffer) appendLinePart(state *lineState, stream outputStream, part string) {
	b.ensureOpenLine(state, stream)
	state.text = appendCappedLine(state.text, part)
	if state.visible {
		b.updateRecentLine(state.seq, state.text)
	}
}

func (b *Buffer) ensureOpenLine(state *lineState, stream outputStream) {
	if state.open && state.visible {
		return
	}
	if !state.open {
		state.open = true
		state.text = ""
	}

	state.seq = b.nextLineSeq
	b.nextLineSeq++
	state.visible = true
	b.recent = append(b.recent, recentLine{
		stream: stream,
		seq:    state.seq,
		text:   state.text,
	})
	b.trimRecentLines()
}

func (b *Buffer) completeLine(state *lineState) {
	if strings.HasSuffix(state.text, "\r") {
		state.text = strings.TrimSuffix(state.text, "\r")
		if state.visible {
			b.updateRecentLine(state.seq, state.text)
		}
	}
	b.completedLines++
	state.open = false
	state.text = ""
	state.seq = 0
	state.visible = false
}

func (b *Buffer) appendStoredOutput(stream outputStream, data string) {
	if b.spilled {
		return
	}

	remaining := b.maxMemoryBytes() - len(b.stdout) - len(b.stderr)
	if remaining <= 0 {
		b.markSpilled()
		return
	}

	if len(data) > remaining {
		b.appendStoredOutputPart(stream, data[:remaining])
		b.markSpilled()
		return
	}

	b.appendStoredOutputPart(stream, data)
}

func (b *Buffer) appendStoredOutputPart(stream outputStream, data string) {
	if data == "" {
		return
	}
	if stream == streamStderr {
		b.stderr += data
		return
	}
	b.stdout += data
}

func (b *Buffer) markSpilled() {
	b.spilled = true
	b.incomplete = true
}

func (b *Buffer) maxMemoryBytes() int {
	if b.MaxMemoryBytes <= 0 {
		return DefaultMaxMemoryBytes
	}
	return b.MaxMemoryBytes
}

func (b *Buffer) lineState(stream outputStream) *lineState {
	if stream == streamStderr {
		return &b.stderrLine
	}
	return &b.stdoutLine
}

func (b *Buffer) updateRecentLine(seq int64, text string) {
	for i := len(b.recent) - 1; i >= 0; i-- {
		if b.recent[i].seq == seq {
			b.recent[i].text = text
			return
		}
	}
}

func (b *Buffer) trimRecentLines() {
	if len(b.recent) <= DefaultRecentLines {
		return
	}

	removed := b.recent[:len(b.recent)-DefaultRecentLines]
	for _, line := range removed {
		b.markLineInvisible(line.seq)
	}
	kept := append([]recentLine(nil), b.recent[len(b.recent)-DefaultRecentLines:]...)
	b.recent = kept
}

func (b *Buffer) markLineInvisible(seq int64) {
	if b.stdoutLine.visible && b.stdoutLine.seq == seq {
		b.stdoutLine.visible = false
	}
	if b.stderrLine.visible && b.stderrLine.seq == seq {
		b.stderrLine.visible = false
	}
}

func appendCappedLine(current, part string) string {
	if part == "" {
		return current
	}
	if len(current)+len(part) <= maxRecentLineBytes {
		return current + part
	}
	if len(part) >= maxRecentLineBytes {
		return tailBytes(part, maxRecentLineBytes)
	}

	keep := maxRecentLineBytes - len(part)
	return tailBytes(current, keep) + part
}

func tailBytes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}

	start := len(text) - limit
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}
