package taskoutput

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBufferCapturesStdoutStderrAndRecentLines(t *testing.T) {
	var buffer Buffer
	buffer.WriteStdout("hello\nworld\n")
	buffer.WriteStderr("warn\n")

	snapshot := buffer.Snapshot()
	if snapshot.Stdout != "hello\nworld\n" {
		t.Fatalf("unexpected stdout: %q", snapshot.Stdout)
	}
	if snapshot.Stderr != "warn\n" {
		t.Fatalf("unexpected stderr: %q", snapshot.Stderr)
	}
	wantRecent := []string{"hello", "world", "[stderr] warn"}
	if !reflect.DeepEqual(snapshot.RecentLines, wantRecent) {
		t.Fatalf("recent lines mismatch\nwant: %#v\n got: %#v", wantRecent, snapshot.RecentLines)
	}
	if snapshot.TotalLines != 3 {
		t.Fatalf("TotalLines = %d, want 3", snapshot.TotalLines)
	}
	if snapshot.TotalBytes != int64(len("hello\nworld\nwarn\n")) {
		t.Fatalf("TotalBytes = %d, want %d", snapshot.TotalBytes, len("hello\nworld\nwarn\n"))
	}
	if snapshot.Incomplete || snapshot.Spilled {
		t.Fatalf("small output should be complete: %+v", snapshot)
	}
}

func TestBufferCombinesPartialLines(t *testing.T) {
	var buffer Buffer
	buffer.WriteStdout("hel")
	buffer.WriteStdout("lo\n")

	snapshot := buffer.Snapshot()
	if snapshot.TotalLines != 1 {
		t.Fatalf("TotalLines = %d, want 1", snapshot.TotalLines)
	}
	if want := []string{"hello"}; !reflect.DeepEqual(snapshot.RecentLines, want) {
		t.Fatalf("recent lines mismatch\nwant: %#v\n got: %#v", want, snapshot.RecentLines)
	}
}

func TestBufferCountsPendingLinesAndBytes(t *testing.T) {
	var buffer Buffer
	buffer.WriteStdout("alpha\nbeta")
	buffer.WriteStderr("warning")

	snapshot := buffer.Snapshot()
	wantRecent := []string{"alpha", "beta", "[stderr] warning"}
	if !reflect.DeepEqual(snapshot.RecentLines, wantRecent) {
		t.Fatalf("recent lines mismatch\nwant: %#v\n got: %#v", wantRecent, snapshot.RecentLines)
	}
	if snapshot.TotalLines != 3 {
		t.Fatalf("TotalLines = %d, want 3", snapshot.TotalLines)
	}
	wantBytes := len("alpha\nbeta") + len("warning")
	if snapshot.TotalBytes != int64(wantBytes) {
		t.Fatalf("TotalBytes = %d, want %d", snapshot.TotalBytes, wantBytes)
	}
}

func TestBufferKeepsRecentHundredLinesAndTailFive(t *testing.T) {
	text := numberedLines(105)
	var buffer Buffer
	buffer.WriteStdout(text)

	snapshot := buffer.Snapshot()
	if len(snapshot.RecentLines) != 100 {
		t.Fatalf("recent line count = %d, want 100", len(snapshot.RecentLines))
	}
	if snapshot.RecentLines[0] != "line-006" {
		t.Fatalf("first recent line = %q, want line-006", snapshot.RecentLines[0])
	}
	if snapshot.RecentLines[99] != "line-105" {
		t.Fatalf("last recent line = %q, want line-105", snapshot.RecentLines[99])
	}
	if snapshot.TotalLines != 105 {
		t.Fatalf("TotalLines = %d, want 105", snapshot.TotalLines)
	}

	wantTail := []string{"line-101", "line-102", "line-103", "line-104", "line-105"}
	if got := TailLines(text, 5); !reflect.DeepEqual(got, wantTail) {
		t.Fatalf("TailLines(..., 5) mismatch\nwant: %#v\n got: %#v", wantTail, got)
	}
}

func TestBufferMarksSpilledAndStopsGrowingStoredOutput(t *testing.T) {
	buffer := Buffer{MaxMemoryBytes: 10}
	buffer.WriteStdout("12345\n")
	buffer.WriteStderr("abcdef\n")
	buffer.WriteStdout("tail\n")

	snapshot := buffer.Snapshot()
	if !snapshot.Spilled || !snapshot.Incomplete {
		t.Fatalf("expected spilled incomplete snapshot: %+v", snapshot)
	}
	if snapshot.Stdout != "12345\n" {
		t.Fatalf("stdout = %q, want %q", snapshot.Stdout, "12345\n")
	}
	if snapshot.Stderr != "abcd" {
		t.Fatalf("stderr = %q, want %q", snapshot.Stderr, "abcd")
	}
	if snapshot.TotalBytes != int64(len("12345\n")+len("abcdef\n")+len("tail\n")) {
		t.Fatalf("TotalBytes = %d", snapshot.TotalBytes)
	}
	if snapshot.TotalLines != 3 {
		t.Fatalf("TotalLines = %d, want 3", snapshot.TotalLines)
	}
	wantRecent := []string{"12345", "[stderr] abcdef", "tail"}
	if !reflect.DeepEqual(snapshot.RecentLines, wantRecent) {
		t.Fatalf("recent lines mismatch\nwant: %#v\n got: %#v", wantRecent, snapshot.RecentLines)
	}
}

func TestSnapshotCopiesRecentLines(t *testing.T) {
	var buffer Buffer
	buffer.WriteStdout("one\ntwo\n")

	snapshot := buffer.Snapshot()
	snapshot.RecentLines[0] = "mutated"

	next := buffer.Snapshot()
	if next.RecentLines[0] != "one" {
		t.Fatalf("snapshot shared recent lines, got %q", next.RecentLines[0])
	}
}

func TestResultSuccess(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "clean exit",
			result: Result{Code: 0},
			want:   true,
		},
		{
			name:   "nonzero code",
			result: Result{Code: 1},
			want:   false,
		},
		{
			name:   "interrupted",
			result: Result{Code: 0, Interrupted: true},
			want:   false,
		},
		{
			name:   "backgrounded",
			result: Result{Code: 0, BackgroundTaskID: "task-1"},
			want:   false,
		},
		{
			name:   "pre spawn error",
			result: Result{Code: 0, PreSpawnError: "missing cwd"},
			want:   false,
		},
		{
			name:   "large output file still success",
			result: Result{Code: 0, OutputFilePath: "/tmp/out", OutputFileSize: 123},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.Success(); got != tt.want {
				t.Fatalf("Success() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTailLinesEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		text string
		n    int
		want []string
	}{
		{
			name: "empty text",
			text: "",
			n:    5,
			want: []string{},
		},
		{
			name: "zero lines requested",
			text: "a\nb",
			n:    0,
			want: []string{},
		},
		{
			name: "without trailing newline",
			text: "a\nb",
			n:    5,
			want: []string{"a", "b"},
		},
		{
			name: "with trailing newline",
			text: "a\nb\n",
			n:    5,
			want: []string{"a", "b"},
		},
		{
			name: "preserves empty lines",
			text: "a\n\nb\n",
			n:    5,
			want: []string{"a", "", "b"},
		},
		{
			name: "only newline",
			text: "\n",
			n:    5,
			want: []string{""},
		},
		{
			name: "limits to n lines",
			text: "a\nb\nc",
			n:    2,
			want: []string{"b", "c"},
		},
		{
			name: "trims carriage return",
			text: "a\r\nb\r\n",
			n:    5,
			want: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TailLines(tt.text, tt.n); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("TailLines(%q, %d) mismatch\nwant: %#v\n got: %#v", tt.text, tt.n, tt.want, got)
			}
		})
	}
}

func TestTailLinesHandlesLongText(t *testing.T) {
	long := strings.Repeat("x", 10000)
	got := TailLines(long+"\nlast\n", 2)
	if len(got) != 2 {
		t.Fatalf("TailLines returned %d lines, want 2", len(got))
	}
	if got[0] != long {
		t.Fatalf("first long line length = %d, want %d", len(got[0]), len(long))
	}
	if got[1] != "last" {
		t.Fatalf("last line = %q, want last", got[1])
	}
}

func numberedLines(count int) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&builder, "line-%03d\n", i)
	}
	return builder.String()
}
