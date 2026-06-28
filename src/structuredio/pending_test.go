package structuredio

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestPendingRequestsResolveResponse(t *testing.T) {
	manager := NewPendingRequests(WithRequestIDGenerator(sequenceIDs("req-1")))

	pending, err := manager.SendRequest(map[string]any{
		"subtype":     "can_use_tool",
		"tool_use_id": "tool-1",
	})
	if err != nil {
		t.Fatalf("SendRequest returned error: %v", err)
	}
	if pending.RequestID != "req-1" {
		t.Fatalf("unexpected request id: %q", pending.RequestID)
	}
	if len(manager.Pending()) != 1 {
		t.Fatalf("expected one pending request")
	}

	response := controlResponse("req-1", "success", map[string]any{"toolUseID": "tool-1", "behavior": "allow"}, "")
	resolution := manager.ResolveResponse(response)
	if resolution.Status != ResponseResolved {
		t.Fatalf("unexpected resolution status: %s", resolution.Status)
	}
	if len(manager.Pending()) != 0 {
		t.Fatalf("expected pending request to be removed")
	}

	result := <-pending.Done()
	if result.Err != nil {
		t.Fatalf("pending result has error: %v", result.Err)
	}
	if result.Response.Response.RequestID != "req-1" {
		t.Fatalf("unexpected response delivered to pending request: %+v", result.Response)
	}
}

func TestPendingRequestsRejectAllOnClose(t *testing.T) {
	closeErr := errors.New("closed")
	manager := NewPendingRequests(WithRequestIDGenerator(sequenceIDs("req-1", "req-2")))

	first, err := manager.SendRequest(map[string]any{"subtype": "hook_callback"})
	if err != nil {
		t.Fatalf("SendRequest first returned error: %v", err)
	}
	second, err := manager.SendRequest(map[string]any{"subtype": "elicitation"})
	if err != nil {
		t.Fatalf("SendRequest second returned error: %v", err)
	}

	rejected := manager.RejectAllOnClose(closeErr)
	if len(rejected) != 2 {
		t.Fatalf("expected two rejected requests, got %d", len(rejected))
	}
	if len(manager.Pending()) != 0 {
		t.Fatalf("expected pending list to be empty")
	}
	for _, pending := range []*PendingRequest{first, second} {
		result := <-pending.Done()
		if !errors.Is(result.Err, closeErr) {
			t.Fatalf("expected close error, got %v", result.Err)
		}
	}
	if _, err := manager.SendRequest(map[string]any{"subtype": "after_close"}); !errors.Is(err, ErrPendingRequestsClosed) {
		t.Fatalf("expected ErrPendingRequestsClosed, got %v", err)
	}
}

func TestPendingRequestsDuplicateResponseIgnored(t *testing.T) {
	manager := NewPendingRequests(WithRequestIDGenerator(sequenceIDs("req-1")))

	_, err := manager.SendRequest(map[string]any{
		"subtype":     "can_use_tool",
		"tool_use_id": "tool-1",
	})
	if err != nil {
		t.Fatalf("SendRequest returned error: %v", err)
	}

	response := controlResponse("req-1", "success", map[string]any{"toolUseID": "tool-1"}, "")
	if resolution := manager.ResolveResponse(response); resolution.Status != ResponseResolved {
		t.Fatalf("initial response was not resolved: %s", resolution.Status)
	}
	if resolution := manager.ResolveResponse(response); resolution.Status != ResponseResolvedDuplicate {
		t.Fatalf("duplicate same request was not recognized: %s", resolution.Status)
	}

	lateDuplicate := controlResponse("different-req", "success", map[string]any{"toolUseID": "tool-1"}, "")
	if resolution := manager.ResolveResponse(lateDuplicate); resolution.Status != ResponseResolvedDuplicate {
		t.Fatalf("late duplicate was not recognized: %s", resolution.Status)
	}

	unexpected := controlResponse("missing-req", "success", map[string]any{"toolUseID": "other-tool"}, "")
	if resolution := manager.ResolveResponse(unexpected); resolution.Status != ResponseUnexpected {
		t.Fatalf("unexpected response was not classified correctly: %s", resolution.Status)
	}
}

func TestPendingRequestsResolvedIDSetIsBounded(t *testing.T) {
	manager := NewPendingRequests(
		WithRequestIDGenerator(sequenceIDs("req-1", "req-2")),
		WithResolvedIDLimit(2),
	)

	if _, err := manager.SendRequest(map[string]any{"tool_use_id": "tool-1"}); err != nil {
		t.Fatalf("SendRequest first returned error: %v", err)
	}
	if resolution := manager.ResolveResponse(controlResponse("req-1", "success", map[string]any{"toolUseID": "tool-1"}, "")); resolution.Status != ResponseResolved {
		t.Fatalf("first response was not resolved: %s", resolution.Status)
	}

	if _, err := manager.SendRequest(map[string]any{"tool_use_id": "tool-2"}); err != nil {
		t.Fatalf("SendRequest second returned error: %v", err)
	}
	if resolution := manager.ResolveResponse(controlResponse("req-2", "success", map[string]any{"toolUseID": "tool-2"}, "")); resolution.Status != ResponseResolved {
		t.Fatalf("second response was not resolved: %s", resolution.Status)
	}

	evicted := controlResponse("req-1", "success", map[string]any{"toolUseID": "tool-1"}, "")
	if resolution := manager.ResolveResponse(evicted); resolution.Status != ResponseUnexpected {
		t.Fatalf("old resolved id should have been evicted, got: %s", resolution.Status)
	}

	current := controlResponse("req-2", "success", map[string]any{"toolUseID": "tool-2"}, "")
	if resolution := manager.ResolveResponse(current); resolution.Status != ResponseResolvedDuplicate {
		t.Fatalf("recent resolved id should still be tracked, got: %s", resolution.Status)
	}
}

func sequenceIDs(ids ...string) func() string {
	index := 0
	return func() string {
		if index >= len(ids) {
			return ""
		}
		id := ids[index]
		index++
		return id
	}
}

func controlResponse(requestID, subtype string, payload map[string]any, message string) ControlResponse {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return ControlResponse{
		Type: EnvelopeControlResponse,
		Response: ControlResponsePayload{
			RequestID: requestID,
			Subtype:   subtype,
			Response:  rawPayload,
			Error:     message,
		},
	}
}
