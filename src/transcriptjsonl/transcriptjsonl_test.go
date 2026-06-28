package transcriptjsonl

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseLinesSkipsBlankMalformedAndStripsBOM(t *testing.T) {
	data := append([]byte{0xef, 0xbb, 0xbf}, []byte(`

{"type":"user","uuid":"11111111-1111-1111-1111-111111111111","parentUuid":"00000000-0000-0000-0000-000000000000","sessionId":"22222222-2222-2222-2222-222222222222","timestamp":"2026-06-28T00:00:00Z","isSidechain":true,"message":{"role":"user","content":"hello"},"extra":42}
{"type":"assistant","uuid":"33333333-3333-3333-3333-333333333333","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}
{"type":"broken"
null
`)...)

	result := ParseLines(data)
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	if result.Skipped != 2 || len(result.Errors) != 2 {
		t.Fatalf("expected 2 skipped summaries, got skipped=%d errors=%d", result.Skipped, len(result.Errors))
	}

	first := result.Entries[0]
	if first.Type != "user" || first.UUID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected first entry identity: %+v", first)
	}
	if first.ParentUUID != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("unexpected parent UUID: %q", first.ParentUUID)
	}
	if first.SessionID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("unexpected session ID: %q", first.SessionID)
	}
	if first.Timestamp != "2026-06-28T00:00:00Z" || !first.IsSidechain {
		t.Fatalf("unexpected timestamp/sidechain: %+v", first)
	}
	if first.Message["content"] != "hello" {
		t.Fatalf("expected message content to be preserved, got %#v", first.Message["content"])
	}
	if first.Fields["extra"].(float64) != 42 {
		t.Fatalf("expected raw fields to be preserved, got %#v", first.Fields["extra"])
	}
	if result.Errors[0].Line == 0 || result.Errors[0].Summary == "" || result.Errors[0].Preview == "" {
		t.Fatalf("expected useful skipped summary, got %+v", result.Errors[0])
	}
}

func TestParseString(t *testing.T) {
	result := ParseString(`{"type":"tag","sessionId":"22222222-2222-2222-2222-222222222222","tag":"focus"}`)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Type != "tag" || result.Entries[0].SessionID == "" {
		t.Fatalf("unexpected parsed entry: %+v", result.Entries[0])
	}
}

func TestValidateUUID(t *testing.T) {
	valid := []string{
		"11111111-1111-1111-1111-111111111111",
		"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE",
	}
	for _, value := range valid {
		if !ValidateUUID(value) {
			t.Fatalf("expected valid UUID: %s", value)
		}
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"111111111111-1111-1111-111111111111",
		"11111111-1111-1111-1111-11111111111g",
	}
	for _, value := range invalid {
		if ValidateUUID(value) {
			t.Fatalf("expected invalid UUID: %s", value)
		}
	}
}

func TestExtractStringFieldHandlesEscapesAndMalformedTail(t *testing.T) {
	line := `{"customTitle": "hello \"quoted\" and \\ slash","other":`

	value, ok := ExtractStringField(line, "customTitle")
	if !ok {
		t.Fatal("expected field to be extracted")
	}
	if value != `hello "quoted" and \ slash` {
		t.Fatalf("unexpected value: %q", value)
	}

	if value, ok := ExtractStringField(line, "other"); ok || value != "" {
		t.Fatalf("expected incomplete non-string field to be absent, got %q", value)
	}
}

func TestExtractLastStringFieldSkipsIncompleteLaterMatch(t *testing.T) {
	data := []byte(strings.Join([]string{
		`{"type":"custom-title","customTitle":"first"}`,
		`{"type":"custom-title","customTitle":"second \"quoted\""}`,
		`{"type":"custom-title","customTitle":"unfinished`,
	}, "\n"))

	value, ok := ExtractLastStringField(data, "customTitle")
	if !ok {
		t.Fatal("expected last complete string field")
	}
	if value != `second "quoted"` {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestExtractFirstPromptFromHeadSkipsSyntheticEntriesAndFindsTextBlock(t *testing.T) {
	head := strings.Join([]string{
		`{"type":"system","message":{"content":"ignore"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"tool output"}]}}`,
		`{"type":"user","isMeta":true,"message":{"content":"metadata"}}`,
		`{"type":"user","isCompactSummary":true,"message":{"content":"summary"}}`,
		`{"type":"user","message":{"content":"<command-name>/clear</command-name><command-message>clear</command-message>"}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"<ide_context>generated</ide_context>"},{"type":"text","text":"real\nprompt"}]}}`,
	}, "\n")

	prompt := ExtractFirstPromptFromHeadString(head)
	if prompt != "real prompt" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestExtractFirstPromptFromHeadStringContentAndTruncates(t *testing.T) {
	longPrompt := strings.Repeat("a", 205)
	line := `{"type":"user","message":{"content":"` + longPrompt + `"}}`

	prompt := ExtractFirstPromptFromHead([]byte(line))
	if utf8.RuneCountInString(prompt) != 200 {
		t.Fatalf("expected prompt to be truncated to 200 chars, got %d", utf8.RuneCountInString(prompt))
	}
	if prompt != strings.Repeat("a", 200) {
		t.Fatalf("unexpected truncated prompt: %q", prompt)
	}
}

func TestExtractFirstPromptFromHeadFormatsBashInput(t *testing.T) {
	line := `{"type":"user","message":{"content":"<bash-input>ls -la</bash-input>"}}`

	prompt := ExtractFirstPromptFromHeadString("\ufeff" + line)
	if prompt != "! ls -la" {
		t.Fatalf("unexpected bash prompt: %q", prompt)
	}
}
