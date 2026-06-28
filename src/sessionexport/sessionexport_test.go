package sessionexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeFilename(t *testing.T) {
	got := SanitizeFilename(" Hello, 世界 / Awaken_Export!!! -- test ")
	want := "hello-awaken-export-test"
	if got != want {
		t.Fatalf("SanitizeFilename() = %q, want %q", got, want)
	}
}

func TestExtractFirstPromptLine(t *testing.T) {
	messages := []Message{
		{Role: RoleSystem, Parts: []Part{TextPart("system")}},
		{Role: RoleUser, Parts: []Part{TextPart("  First prompt line\r\nSecond line  ")}},
	}

	if got := ExtractFirstPrompt(messages); got != "First prompt line" {
		t.Fatalf("ExtractFirstPrompt() = %q", got)
	}

	long := []Message{{Role: RoleUser, Parts: []Part{TextPart("1234567890abcdef")}}}
	if got := ExtractFirstPromptLine(long, 10); got != "1234567..." {
		t.Fatalf("ExtractFirstPromptLine() = %q, want ASCII ellipsis", got)
	}
}

func TestDefaultFilename(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 34, 56, 0, time.UTC)
	messages := []Message{{Role: RoleUser, Parts: []Part{TextPart("Hello, Awaken!\nIgnore this")}}}

	if got := DefaultTextFilename(messages, now); got != "2026-06-27-123456-hello-awaken.txt" {
		t.Fatalf("DefaultTextFilename() = %q", got)
	}

	if got := DefaultTextFilename(nil, now); got != "2026-06-27-123456-conversation.txt" {
		t.Fatalf("fallback DefaultTextFilename() = %q", got)
	}
}

func TestRenderPlainText(t *testing.T) {
	ts := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	messages := []Message{
		{Role: RoleUser, Timestamp: &ts, Parts: []Part{TextPart("Hello")}},
		{Role: RoleAssistant, Parts: []Part{TextPart("Hi there")}},
	}

	got := RenderPlainText(messages, PlainTextOptions{IncludeTimestamps: true})
	wantContains := []string{
		"## User (2026-06-27T12:00:00Z)",
		"Hello",
		"## Assistant",
		"Hi there",
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderPlainText() missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderPlainTextToolSummary(t *testing.T) {
	messages := []Message{{
		Role: RoleAssistant,
		Parts: []Part{
			ToolCallPart(ToolEvent{ID: "toolu_1", Name: "Bash", Input: "go test ./src/sessionexport"}),
			ToolResultPart(ToolEvent{ID: "toolu_1", Name: "Bash", Output: "ok\nall tests passed"}),
		},
	}}

	got := RenderPlainText(messages)
	wantContains := []string{
		"[tool call] Bash (toolu_1): go test ./src/sessionexport",
		"[tool result] Bash (toolu_1): ok all tests passed",
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderPlainText() missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderJSONLEscapesJSLineTerminators(t *testing.T) {
	original := "before\u2028middle\u2029after"
	messages := []Message{{Role: RoleUser, Parts: []Part{TextPart(original)}}}

	got, err := RenderJSONL(messages)
	if err != nil {
		t.Fatalf("RenderJSONL() error = %v", err)
	}
	if strings.Contains(got, "\u2028") || strings.Contains(got, "\u2029") {
		t.Fatalf("RenderJSONL() contained raw JS line terminators: %q", got)
	}
	if !strings.Contains(got, `\u2028`) || !strings.Contains(got, `\u2029`) {
		t.Fatalf("RenderJSONL() did not contain escaped JS line terminators: %q", got)
	}

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("RenderJSONL() wrote %d lines, want 1", len(lines))
	}

	var decoded Message
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.Parts[0].Text != original {
		t.Fatalf("decoded text = %q, want %q", decoded.Parts[0].Text, original)
	}
}

func TestWriteFileCoercesExtensionAndUsesPrivateMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "conversation.md")

	finalPath, err := WriteFile(target, "hello", FormatPlainText)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if finalPath != filepath.Join(dir, "conversation.txt") {
		t.Fatalf("WriteFile() path = %q", finalPath)
	}

	info, err := os.Stat(finalPath)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != ExportFileMode {
		t.Fatalf("file mode = %v, want %v", got, ExportFileMode)
	}

	jsonlPath, err := EnsureExtension(filepath.Join(dir, "events"), FormatJSONL)
	if err != nil {
		t.Fatalf("EnsureExtension() error = %v", err)
	}
	if jsonlPath != filepath.Join(dir, "events.jsonl") {
		t.Fatalf("EnsureExtension() = %q", jsonlPath)
	}
}

func TestWriteFileRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteFile(dir, "hello", FormatPlainText); err == nil {
		t.Fatal("WriteFile() error = nil, want directory rejection")
	}
}
