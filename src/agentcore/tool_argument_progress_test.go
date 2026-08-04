package agentcore

import (
	"testing"

	"awaken/src/llm"
)

func TestToolArgumentProgressTrackerStreamsWriteLinesWithoutContent(t *testing.T) {
	var tracker toolArgumentProgressTracker
	chunks := []string{
		`{"file_path":"/private/reports/live.md","content":"# Title`,
		`\nfirst line`,
		`\nsecond line"}`,
	}
	wantLines := []int{1, 2, 3}
	for index, chunk := range chunks {
		progress, ok := tracker.consume(llm.ToolCallDelta{
			Index:          0,
			ID:             "write-1",
			Name:           "Write",
			ArgumentsDelta: chunk,
		})
		if !ok {
			t.Fatalf("chunk %d did not produce safe progress", index)
		}
		if progress.Kind != toolPreparationWrite || progress.Path != "live.md" {
			t.Fatalf("chunk %d returned unsafe or incorrect identity: %+v", index, progress)
		}
		if progress.AddedLines != wantLines[index] || progress.DeletedLines != 0 {
			t.Fatalf("chunk %d returned incorrect line counts: %+v", index, progress)
		}
	}
}

func TestToolArgumentProgressTrackerStreamsEditAcrossEscapes(t *testing.T) {
	var tracker toolArgumentProgressTracker
	chunks := []string{
		`{"file_path":"C:\\work\\app.ts","old_string":"one\n`,
		`two","new_string":"one\\n`,
		`two\nthree"}`,
	}
	wantAdded := []int{0, 1, 2}
	wantDeleted := []int{2, 2, 2}
	for index, chunk := range chunks {
		progress, ok := tracker.consume(llm.ToolCallDelta{
			Index:          0,
			ID:             "edit-1",
			Name:           "Edit",
			ArgumentsDelta: chunk,
		})
		if !ok {
			t.Fatalf("chunk %d did not produce safe progress", index)
		}
		if progress.Path != "app.ts" {
			t.Fatalf("chunk %d leaked or lost the path basename: %+v", index, progress)
		}
		if progress.AddedLines != wantAdded[index] || progress.DeletedLines != wantDeleted[index] {
			t.Fatalf("chunk %d returned incorrect line counts: %+v", index, progress)
		}
	}
}

func TestToolArgumentProgressTrackerCountsSplitUnicodeNewline(t *testing.T) {
	var tracker toolArgumentProgressTracker
	first, ok := tracker.consume(llm.ToolCallDelta{
		ID:             "write-unicode",
		Name:           "Write",
		ArgumentsDelta: `{"content":"first\u00`,
	})
	if !ok || first.AddedLines != 1 {
		t.Fatalf("unexpected first unicode progress: %+v ok=%v", first, ok)
	}
	second, ok := tracker.consume(llm.ToolCallDelta{
		ID:             "write-unicode",
		Name:           "Write",
		ArgumentsDelta: `0asecond"}`,
	})
	if !ok || second.AddedLines != 2 {
		t.Fatalf("unexpected completed unicode progress: %+v ok=%v", second, ok)
	}
}

func TestToolArgumentProgressTrackerFailsClosedForOtherTools(t *testing.T) {
	var tracker toolArgumentProgressTracker
	progress, ok := tracker.consume(llm.ToolCallDelta{
		ID:             "shell-1",
		Name:           "Bash",
		ArgumentsDelta: `{"command":"printf private"}`,
	})
	if ok || progress != (toolArgumentProgress{}) {
		t.Fatalf("unknown tool must not produce preparation progress: %+v ok=%v", progress, ok)
	}
}

func TestSafePreparationBasenameDropsControlsAndDirectories(t *testing.T) {
	if got := safePreparationBasename("/Users/private/secret\n/report.md"); got != "report.md" {
		t.Fatalf("unexpected safe basename: %q", got)
	}
}
