package remotesdk

import (
	"testing"
	"time"
)

func TestPermissionRequestRecordValidationAndEvent(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	record, err := NewPermissionRequestRecord("req-1", SDKControlPermissionRequest{
		Subtype:        "can_use_tool",
		ToolName:       "apply_patch",
		ToolUseID:      "toolu-1",
		Input:          map[string]any{"patch": "*** Begin Patch"},
		DecisionReason: "Writes files",
		BlockedPath:    "src/main.go",
		DisplayName:    "Apply Patch",
		PermissionSuggestions: []map[string]any{
			{"behavior": "allow", "rule": "apply_patch"},
		},
	}, ConvertOptions{FallbackTimestamp: fallback})
	if err != nil {
		t.Fatalf("unexpected request validation error: %v", err)
	}
	if record.Title != "Approve Apply Patch" {
		t.Fatalf("unexpected title %q", record.Title)
	}
	if record.Description != "Writes files\n\nBlocked path: src/main.go" {
		t.Fatalf("unexpected description %q", record.Description)
	}
	if !record.Timestamp.Equal(fallback) {
		t.Fatalf("expected fallback timestamp, got %s", record.Timestamp)
	}

	event := record.Event()
	if event.Type != EventPermissionRequest || event.Severity != SeverityWarning {
		t.Fatalf("unexpected permission event: %+v", event)
	}
	if event.ToolUseID != "toolu-1" || event.ToolName != "apply_patch" {
		t.Fatalf("tool metadata not preserved on event: %+v", event)
	}
}

func TestPermissionRequestRecordRejectsInvalidRecords(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 10, 1, 0, 0, time.UTC)

	cases := []struct {
		name      string
		requestID string
		request   SDKControlPermissionRequest
	}{
		{
			name:      "missing request id",
			requestID: "",
			request: SDKControlPermissionRequest{
				ToolName:  "shell",
				ToolUseID: "toolu-1",
			},
		},
		{
			name:      "missing tool use id",
			requestID: "req-1",
			request: SDKControlPermissionRequest{
				ToolName: "shell",
			},
		},
		{
			name:      "missing tool name",
			requestID: "req-1",
			request: SDKControlPermissionRequest{
				ToolUseID: "toolu-1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPermissionRequestRecord(tc.requestID, tc.request, ConvertOptions{FallbackTimestamp: fallback})
			if err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}

	_, err := PermissionRequestRecordFromControl(SDKControlRequest{
		Type:      "control_request",
		RequestID: "req-1",
		Request: SDKControlPermissionRequest{
			Subtype:   "set_model",
			ToolName:  "shell",
			ToolUseID: "toolu-1",
		},
	}, ConvertOptions{FallbackTimestamp: fallback})
	if err == nil {
		t.Fatalf("expected unsupported control subtype error")
	}
}

func TestPermissionResponseRecordValidationAndSDKResponse(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 10, 2, 0, 0, time.UTC)
	request := PermissionRequestRecord{
		RequestID: "req-1",
		ToolUseID: "toolu-1",
		ToolName:  "shell",
		Timestamp: fallback,
	}

	allow, err := NewPermissionResponseRecord(request, PermissionAllow, map[string]any{"command": "ls"}, "", ConvertOptions{FallbackTimestamp: fallback})
	if err != nil {
		t.Fatalf("unexpected allow response error: %v", err)
	}
	response := allow.SDKControlResponse()
	if response.Type != "control_response" || response.Response.Subtype != "success" {
		t.Fatalf("unexpected control response wrapper: %+v", response)
	}
	if response.Response.RequestID != "req-1" {
		t.Fatalf("request id was not preserved: %+v", response)
	}
	if response.Response.Response.Behavior != PermissionAllow {
		t.Fatalf("unexpected response behavior: %+v", response.Response.Response)
	}
	if response.Response.Response.UpdatedInput["command"] != "ls" {
		t.Fatalf("updated input was not preserved: %+v", response.Response.Response.UpdatedInput)
	}

	deny, err := NewPermissionResponseRecord(request, PermissionDeny, nil, "not allowed", ConvertOptions{FallbackTimestamp: fallback})
	if err != nil {
		t.Fatalf("unexpected deny response error: %v", err)
	}
	denyResponse := deny.SDKControlResponse()
	if denyResponse.Response.Response.Behavior != PermissionDeny || denyResponse.Response.Response.Message != "not allowed" {
		t.Fatalf("unexpected deny response payload: %+v", denyResponse.Response.Response)
	}
	if deny.Event().Severity != SeverityWarning {
		t.Fatalf("expected deny event warning severity")
	}
}

func TestPermissionResponseRecordRejectsInvalidRecords(t *testing.T) {
	fallback := time.Date(2026, 6, 28, 10, 3, 0, 0, time.UTC)
	request := PermissionRequestRecord{
		RequestID: "req-1",
		ToolUseID: "toolu-1",
		ToolName:  "shell",
		Timestamp: fallback,
	}

	if _, err := NewPermissionResponseRecord(PermissionRequestRecord{}, PermissionAllow, nil, "", ConvertOptions{FallbackTimestamp: fallback}); err == nil {
		t.Fatalf("expected missing request id error")
	}
	if _, err := NewPermissionResponseRecord(request, PermissionDecision("maybe"), nil, "", ConvertOptions{FallbackTimestamp: fallback}); err == nil {
		t.Fatalf("expected invalid decision error")
	}
	if _, err := NewPermissionResponseRecord(request, PermissionDeny, nil, "", ConvertOptions{FallbackTimestamp: fallback}); err == nil {
		t.Fatalf("expected missing denial message error")
	}

	unsupported := UnsupportedControlResponse("req-2", "set_model")
	if unsupported.Type != "control_response" || unsupported.Response.Subtype != "error" || unsupported.Response.RequestID != "req-2" {
		t.Fatalf("unexpected unsupported response: %+v", unsupported)
	}
}
