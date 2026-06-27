package remotesdk

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConvertAssistantMessagePreservesIDsTimestampAndToolUse(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC)
	msg := SDKMessage{
		Type:      SDKMessageAssistant,
		UUID:      "assistant-uuid",
		SessionID: "session-1",
		Message: SDKConversationMessage{
			Role: "assistant",
			Content: BlockContent(
				ContentBlock{Type: "text", Text: "I can help."},
				ContentBlock{
					Type:  "tool_use",
					ID:    "toolu-1",
					Name:  "file_read",
					Input: map[string]any{"path": "README.md"},
				},
			),
		},
	}

	result := ConvertMessage(msg, ConvertOptions{FallbackTimestamp: fallback})

	if result.Kind != ConversionEvent {
		t.Fatalf("expected event conversion, got %+v", result)
	}
	event := result.Event
	if event.Type != EventAssistantMessage {
		t.Fatalf("expected assistant event, got %s", event.Type)
	}
	if event.UUID != "assistant-uuid" || event.SessionID != "session-1" {
		t.Fatalf("IDs were not preserved: %+v", event)
	}
	if !event.Timestamp.Equal(fallback) {
		t.Fatalf("expected fallback timestamp %s, got %s", fallback, event.Timestamp)
	}
	if event.Content != "I can help." {
		t.Fatalf("unexpected content %q", event.Content)
	}
	if len(event.ToolUses) != 1 {
		t.Fatalf("expected one tool use, got %+v", event.ToolUses)
	}
	if event.ToolUses[0].ID != "toolu-1" || event.ToolUses[0].Name != "file_read" {
		t.Fatalf("tool use was not preserved: %+v", event.ToolUses[0])
	}
}

func TestConvertUserMessagesHonorsEchoAndToolResultOptions(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 9, 1, 0, 0, time.UTC)
	userText := SDKMessage{
		Type:      SDKMessageUser,
		UUID:      "user-1",
		SessionID: "session-1",
		Timestamp: "2026-06-28T09:00:00.123456Z",
		Message: SDKConversationMessage{
			Role:    "user",
			Content: TextContent("hello"),
		},
	}

	defaultResult := ConvertMessage(userText, ConvertOptions{FallbackTimestamp: fallback})
	if defaultResult.Kind != ConversionIgnored || defaultResult.Reason != "user_message_echo_ignored" {
		t.Fatalf("expected user echo ignored, got %+v", defaultResult)
	}

	historical := ConvertMessage(userText, ConvertOptions{
		ConvertUserTextMessages: true,
		FallbackTimestamp:       fallback,
	})
	if historical.Kind != ConversionEvent {
		t.Fatalf("expected historical user message event, got %+v", historical)
	}
	if historical.Event.Content != "hello" {
		t.Fatalf("unexpected historical content %q", historical.Event.Content)
	}
	if historical.Event.Timestamp.IsZero() || historical.Event.Timestamp.Equal(fallback) {
		t.Fatalf("expected source timestamp to win, got %s", historical.Event.Timestamp)
	}

	parent := "not-reliable"
	toolResult := SDKMessage{
		Type:            SDKMessageUser,
		UUID:            "tool-result-1",
		ParentToolUseID: &parent,
		Message: SDKConversationMessage{
			Role: "user",
			Content: BlockContent(ContentBlock{
				Type:      "tool_result",
				ToolUseID: "toolu-42",
				Content:   json.RawMessage(`"read ok"`),
			}),
		},
	}

	if !IsToolResultUserMessage(toolResult) {
		t.Fatalf("expected content-shape tool_result detection")
	}

	disabled := ConvertMessage(toolResult, ConvertOptions{FallbackTimestamp: fallback})
	if disabled.Kind != ConversionIgnored || disabled.Reason != "tool_result_conversion_disabled" {
		t.Fatalf("expected tool result ignored without option, got %+v", disabled)
	}

	converted := ConvertMessage(toolResult, ConvertOptions{
		ConvertToolResults: true,
		FallbackTimestamp:  fallback,
	})
	if converted.Kind != ConversionEvent {
		t.Fatalf("expected converted tool result, got %+v", converted)
	}
	if got := converted.Event.ParentToolUseID; got != parent {
		t.Fatalf("expected parent tool id to be preserved as metadata, got %q", got)
	}
	if len(converted.Event.ToolResults) != 1 {
		t.Fatalf("expected one tool result, got %+v", converted.Event.ToolResults)
	}
	if converted.Event.ToolResults[0].ToolUseID != "toolu-42" {
		t.Fatalf("tool result id was not preserved: %+v", converted.Event.ToolResults[0])
	}
}

func TestConvertStreamStatusResultAndCompactMessages(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 9, 2, 0, 0, time.UTC)

	init := ConvertMessage(SDKMessage{
		Type:      SDKMessageSystem,
		Subtype:   string(SDKSystemInit),
		UUID:      "init-1",
		SessionID: "session-1",
		Model:     "claude-sonnet-4",
		CWD:       "/workspace",
		Tools:     []string{"file_read", "shell"},
	}, ConvertOptions{FallbackTimestamp: fallback})
	if init.Kind != ConversionEvent || init.Event.Type != EventSystemInit {
		t.Fatalf("unexpected init conversion: %+v", init)
	}
	if init.Event.Model != "claude-sonnet-4" || len(init.Event.Tools) != 2 {
		t.Fatalf("init metadata was not preserved: %+v", init.Event)
	}

	stream := ConvertMessage(SDKMessage{
		Type: SDKMessageStreamEvent,
		Event: SDKStreamEvent{
			Type:  "content_block_delta",
			Delta: SDKStreamDelta{Type: "text_delta", Text: "hi"},
		},
	}, ConvertOptions{FallbackTimestamp: fallback})
	if stream.Kind != ConversionEvent || stream.Event.Type != EventStreamEvent || stream.Event.Delta != "hi" {
		t.Fatalf("unexpected stream conversion: %+v", stream)
	}

	statusValue := "compacting"
	status := ConvertMessage(SDKMessage{
		Type:    SDKMessageSystem,
		Subtype: string(SDKSystemStatus),
		Status:  &statusValue,
	}, ConvertOptions{FallbackTimestamp: fallback})
	if status.Kind != ConversionEvent || status.Event.Type != EventStatus {
		t.Fatalf("unexpected status conversion: %+v", status)
	}
	if status.Event.Severity != SeverityInfo || status.Event.Content != "Compacting conversation" {
		t.Fatalf("unexpected compacting status event: %+v", status.Event)
	}

	errorResult := ConvertMessage(SDKMessage{
		Type:    SDKMessageResult,
		Subtype: "error_during_execution",
		IsError: true,
		Errors:  []string{"provider failed"},
	}, ConvertOptions{FallbackTimestamp: fallback})
	if errorResult.Kind != ConversionEvent || errorResult.Event.Severity != SeverityError {
		t.Fatalf("expected error result event, got %+v", errorResult)
	}
	if errorResult.Event.Content != "provider failed" {
		t.Fatalf("unexpected error content %q", errorResult.Event.Content)
	}

	toolProgress := ConvertMessage(SDKMessage{
		Type:           SDKMessageToolProgress,
		UUID:           "progress-1",
		ToolUseID:      "toolu-progress",
		ToolName:       "shell",
		ElapsedSeconds: 3,
		TaskID:         "task-1",
	}, ConvertOptions{FallbackTimestamp: fallback})
	if toolProgress.Kind != ConversionEvent || toolProgress.Event.Type != EventToolProgress {
		t.Fatalf("unexpected tool progress conversion: %+v", toolProgress)
	}
	if toolProgress.Event.ToolUseID != "toolu-progress" || toolProgress.Event.ToolName != "shell" || toolProgress.Event.TaskID != "task-1" {
		t.Fatalf("tool progress metadata was not preserved: %+v", toolProgress.Event)
	}

	compact := ConvertMessage(SDKMessage{
		Type:    SDKMessageSystem,
		Subtype: string(SDKSystemCompactBoundary),
		CompactMetadata: CompactMetadata{
			Trigger:   "auto",
			PreTokens: 1234,
			PreservedSegment: &PreservedSegment{
				HeadUUID:   "head",
				AnchorUUID: "anchor",
				TailUUID:   "tail",
			},
		},
	}, ConvertOptions{FallbackTimestamp: fallback})
	if compact.Kind != ConversionEvent || compact.Event.Type != EventCompactBoundary {
		t.Fatalf("unexpected compact conversion: %+v", compact)
	}
	if compact.Event.CompactMetadata == nil || compact.Event.CompactMetadata.PreservedSegment.AnchorUUID != "anchor" {
		t.Fatalf("compact metadata was not preserved: %+v", compact.Event.CompactMetadata)
	}
}

func TestStatusAndResultSeverityMapping(t *testing.T) {
	statusCases := map[string]Severity{
		"compacting":   SeverityInfo,
		"blocked":      SeverityWarning,
		"failed":       SeverityError,
		"new_status":   SeverityInfo,
		"rate_limited": SeverityWarning,
	}
	for status, want := range statusCases {
		if got := SeverityForStatus(status); got != want {
			t.Fatalf("SeverityForStatus(%q) = %s, want %s", status, got, want)
		}
	}

	resultCases := []struct {
		subtype string
		isError bool
		want    Severity
	}{
		{subtype: "success", want: SeverityInfo},
		{subtype: "error_max_turns", isError: true, want: SeverityWarning},
		{subtype: "error_max_budget_usd", isError: true, want: SeverityWarning},
		{subtype: "error_during_execution", isError: true, want: SeverityError},
		{subtype: "error_new", isError: true, want: SeverityError},
	}
	for _, tc := range resultCases {
		if got := SeverityForResult(tc.subtype, tc.isError); got != tc.want {
			t.Fatalf("SeverityForResult(%q, %v) = %s, want %s", tc.subtype, tc.isError, got, tc.want)
		}
	}
}

func TestConvertSuccessResultIsIgnoredButResultTextIsAvailable(t *testing.T) {
	msg := SDKMessage{
		Type:    SDKMessageResult,
		Subtype: "success",
		Result:  "done",
	}

	result := ConvertMessage(msg, ConvertOptions{FallbackTimestamp: time.Date(2026, 6, 28, 9, 4, 0, 0, time.UTC)})
	if result.Kind != ConversionIgnored || result.Reason != "success_result_ignored" {
		t.Fatalf("expected success result to be ignored for display, got %+v", result)
	}

	if !IsSessionEndMessage(msg) || !IsSuccessResult(msg) {
		t.Fatalf("success result should still be detectable as session end")
	}
	if text, ok := ResultText(msg); !ok || text != "done" {
		t.Fatalf("expected result text to remain available, got %q, %v", text, ok)
	}
}

func TestIgnoredAndUnsupportedResultsAreExplicit(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 9, 3, 0, 0, time.UTC)

	auth := ConvertMessage(SDKMessage{
		Type: SDKMessageAuthStatus,
		UUID: "auth-1",
	}, ConvertOptions{FallbackTimestamp: fallback})
	if auth.Kind != ConversionIgnored || auth.Reason == "" {
		t.Fatalf("expected explicit ignored auth status, got %+v", auth)
	}

	unknown := ConvertMessage(SDKMessage{
		Type: "future_message",
		UUID: "future-1",
	}, ConvertOptions{FallbackTimestamp: fallback})
	if unknown.Kind != ConversionUnsupported || unknown.Reason == "" {
		t.Fatalf("expected explicit unsupported message, got %+v", unknown)
	}
}
