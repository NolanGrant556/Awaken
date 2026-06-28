package transcriptindex

import (
	"strings"
	"testing"
)

func TestBuildLiteIndexTranscriptMessageTypes(t *testing.T) {
	lines := []string{
		`{"type":"user","sessionId":"s1","uuid":"u1","message":{"content":"hello"}}`,
		`{"type":"assistant","sessionId":"s1","uuid":"a1","message":{"content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"attachment","sessionId":"s1","uuid":"att1","message":{"content":"note.txt"}}`,
		`{"type":"system","sessionId":"s1","uuid":"sys1","message":{"content":"system note"}}`,
		`{"type":"progress","sessionId":"s1","uuid":"p1","message":{"content":"running"}}`,
	}

	items := BuildLiteIndex(lines)
	if len(items) != 1 {
		t.Fatalf("expected 1 session item, got %d", len(items))
	}
	got := messageTypes(items[0].Messages)
	want := []string{"user", "assistant", "attachment", "system"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected message types: got %v want %v", got, want)
	}
	if items[0].MessageCount != 4 {
		t.Fatalf("expected message count 4, got %d", items[0].MessageCount)
	}
}

func TestBuildLiteIndexSkipsEphemeralRows(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "progress",
			line: `{"type":"progress","sessionId":"s1","message":{"content":"running"}}`,
		},
		{
			name: "tool result only",
			line: `{"type":"user","sessionId":"s1","message":{"content":[{"type":"tool_result","content":"done"}]}}`,
		},
		{
			name: "meta",
			line: `{"type":"user","sessionId":"s1","isMeta":true,"message":{"content":"meta"}}`,
		},
		{
			name: "compact summary",
			line: `{"type":"user","sessionId":"s1","isCompactSummary":true,"message":{"content":"summary"}}`,
		},
		{
			name: "bad json",
			line: `{not json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := BuildLiteIndex([]string{tt.line, `{"type":"user","sessionId":"s1","message":{"content":"real prompt"}}`})
			if len(items) != 1 {
				t.Fatalf("expected 1 session item, got %d", len(items))
			}
			if len(items[0].Messages) != 1 {
				t.Fatalf("expected only real prompt message, got %+v", items[0].Messages)
			}
			if items[0].FirstPrompt != "real prompt" {
				t.Fatalf("unexpected first prompt %q", items[0].FirstPrompt)
			}
		})
	}
}

func TestExtractFirstPrompt(t *testing.T) {
	longPrompt := strings.Repeat("x", 205)
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "plain text",
			lines: []string{`{"type":"user","message":{"content":" hello\nworld "}}`},
			want:  "hello world",
		},
		{
			name:  "bash input",
			lines: []string{`{"type":"user","message":{"content":"<bash-input>npm test</bash-input>"}}`},
			want:  "! npm test",
		},
		{
			name:  "command name fallback",
			lines: []string{`{"type":"user","message":{"content":"<command-name>/clear</command-name>"}}`},
			want:  "/clear",
		},
		{
			name: "xml auto context skipped",
			lines: []string{
				`{"type":"user","message":{"content":"<ide_context>auto</ide_context>"}}`,
				`{"type":"user","message":{"content":"real request"}}`,
			},
			want: "real request",
		},
		{
			name:  "xml auto context before text block skipped",
			lines: []string{`{"type":"user","message":{"content":[{"type":"text","text":"<ide_opened_file>auto</ide_opened_file>"},{"type":"text","text":"real block"}]}}`},
			want:  "real block",
		},
		{
			name:  "truncate",
			lines: []string{`{"type":"user","message":{"content":"` + longPrompt + `"}}`},
			want:  strings.Repeat("x", 200) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractFirstPrompt(tt.lines); got != tt.want {
				t.Fatalf("ExtractFirstPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseJSONLLiteSkipsBadJSONLine(t *testing.T) {
	text := strings.Join([]string{
		`{"type":"custom-title","sessionId":"s1","customTitle":"Searchable Session"}`,
		`{bad json`,
		`{"type":"user","sessionId":"s1","message":{"content":"first prompt"}}`,
	}, "\n")

	items := ParseJSONLLite(text)
	if len(items) != 1 {
		t.Fatalf("expected 1 session item, got %d", len(items))
	}
	if items[0].Title != "Searchable Session" {
		t.Fatalf("unexpected title %q", items[0].Title)
	}
	if items[0].FirstPrompt != "first prompt" {
		t.Fatalf("unexpected first prompt %q", items[0].FirstPrompt)
	}
}

func TestSearchLimitAndCaseInsensitive(t *testing.T) {
	items := []SessionItem{
		{Summary: Summary{SessionID: "s1"}, Messages: []Message{{Text: "transcript text"}}},
		{Summary: Summary{SessionID: "s2", Title: "Transcript Search"}},
		{Summary: Summary{SessionID: "s3", FirstPrompt: "transcript prompt"}},
	}

	results := Search(items, "TRANSCRIPT", 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Item.SessionID != "s2" || results[0].Match != "title" {
		t.Fatalf("expected title match first, got %+v", results[0])
	}
	if results[1].Item.SessionID != "s3" || results[1].Match != "first_prompt" {
		t.Fatalf("expected first prompt match second, got %+v", results[1])
	}
}

func messageTypes(messages []Message) []string {
	types := make([]string, 0, len(messages))
	for _, message := range messages {
		types = append(types, message.Type)
	}
	return types
}
