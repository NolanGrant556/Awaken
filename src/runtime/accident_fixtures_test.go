package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/agentcore"
)

func TestV1ToolMessagesWithoutToolEventsFixture(t *testing.T) {
	path := filepath.Join("testdata", "v1_tool_messages_without_tool_events.jsonl")
	records, err := readTranscriptRecords(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}

	var (
		messageToolCalls int
		toolMessages     int
		shellSuccessFlag bool
		eventTypes       []agentcore.AwakenEventType
	)
	for _, record := range records {
		switch record.RecordType {
		case transcriptRecordTypeMessage:
			if len(record.ToolCalls) > 0 {
				messageToolCalls += len(record.ToolCalls)
			}
			if record.Role == "tool" {
				toolMessages++
				if record.ToolName == "shell" && !record.IsError {
					shellSuccessFlag = true
				}
			}
		case transcriptRecordTypeEvent:
			if record.Event == nil {
				t.Fatalf("event record missing event payload: %+v", record)
			}
			eventTypes = append(eventTypes, record.Event.Type)
			if !isRecoverableTranscriptEvent(record.Event.Type) {
				t.Fatalf("fixture event %q must be recoverable under current whitelist", record.Event.Type)
			}
			switch record.Event.Type {
			case agentcore.ToolCallStartedEventType,
				agentcore.ToolCallArgsEventType,
				agentcore.ToolCallFinishedEventType,
				agentcore.ToolResultEventType,
				agentcore.ArtifactCreatedEventType,
				agentcore.ArtifactUpdatedEventType:
				t.Fatalf("fixture must not contain tool/artifact events, found %q", record.Event.Type)
			}
		}
	}

	if messageToolCalls < 3 {
		t.Fatalf("expected at least 3 tool_calls in messages, got %d", messageToolCalls)
	}
	if toolMessages < 3 {
		t.Fatalf("expected at least 3 tool messages, got %d", toolMessages)
	}
	if !shellSuccessFlag {
		t.Fatal("expected shell tool message with is_error=false despite exit_code=1 content")
	}
	if len(eventTypes) == 0 {
		t.Fatal("expected recoverable text/terminal events")
	}

	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		t.Fatalf("loadRestoredTranscriptAtPath: %v", err)
	}
	if len(restored.Messages) < 5 {
		t.Fatalf("expected restored messages with tool facts, got %d", len(restored.Messages))
	}
	hasAssistantToolCall := false
	for _, msg := range restored.Messages {
		if len(msg.ToolCalls) > 0 {
			hasAssistantToolCall = true
			break
		}
	}
	if !hasAssistantToolCall {
		t.Fatal("restored messages must retain assistant tool_calls")
	}
}

func TestV1FixtureFileIsDesensitized(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "v1_tool_messages_without_tool_events.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(raw)
	for _, item := range []string{"/Users/", "Library/Application Support"} {
		if strings.Contains(text, item) {
			t.Fatalf("fixture must remain desensitized; found %q", item)
		}
	}
}
