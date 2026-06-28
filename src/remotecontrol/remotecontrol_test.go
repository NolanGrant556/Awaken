package remotecontrol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHandleRequestInitializeMinimalSuccess(t *testing.T) {
	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-init",
		Request:   ControlRequestPayload{Subtype: RequestSubtypeInitialize},
	}, Options{PID: 42})

	if response.Type != MessageTypeControlResponse {
		t.Fatalf("expected control_response, got %q", response.Type)
	}
	if response.Response.Subtype != ResponseSubtypeSuccess {
		t.Fatalf("expected success, got %+v", response.Response)
	}
	if response.Response.RequestID != "req-init" {
		t.Fatalf("unexpected request id %q", response.Response.RequestID)
	}
	capabilities, ok := response.Response.Response.(InitializeCapabilities)
	if !ok {
		t.Fatalf("expected InitializeCapabilities, got %T", response.Response.Response)
	}
	if capabilities.OutputStyle != "normal" || capabilities.PID != 42 {
		t.Fatalf("unexpected capabilities: %+v", capabilities)
	}
	if len(capabilities.Commands) != 0 || len(capabilities.Models) != 0 {
		t.Fatalf("expected empty commands/models, got %+v", capabilities)
	}
	if len(capabilities.AvailableOutputStyles) != 1 || capabilities.AvailableOutputStyles[0] != "normal" {
		t.Fatalf("unexpected output styles: %+v", capabilities.AvailableOutputStyles)
	}
	if capabilities.Account == nil {
		t.Fatalf("expected empty account object, got nil")
	}
}

func TestHandleRequestSetModelCallback(t *testing.T) {
	model := "claude-sonnet-4"
	var got *string

	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-model",
		Request: ControlRequestPayload{
			Subtype: RequestSubtypeSetModel,
			Model:   &model,
		},
	}, Options{
		OnSetModel: func(model *string) {
			got = model
		},
	})

	if response.Response.Subtype != ResponseSubtypeSuccess {
		t.Fatalf("expected success, got %+v", response.Response)
	}
	if got == nil || *got != model {
		t.Fatalf("expected model callback %q, got %v", model, got)
	}
}

func TestHandleRequestInterruptCallback(t *testing.T) {
	called := false

	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-interrupt",
		Request:   ControlRequestPayload{Subtype: RequestSubtypeInterrupt},
	}, Options{
		OnInterrupt: func() {
			called = true
		},
	})

	if response.Response.Subtype != ResponseSubtypeSuccess {
		t.Fatalf("expected success, got %+v", response.Response)
	}
	if !called {
		t.Fatalf("expected interrupt callback")
	}
}

func TestHandleRequestSetPermissionModeError(t *testing.T) {
	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-permission",
		Request: ControlRequestPayload{
			Subtype: RequestSubtypeSetPermissionMode,
			Mode:    "bypassPermissions",
		},
	}, Options{
		OnSetPermissionMode: func(mode string) PermissionModeResult {
			if mode != "bypassPermissions" {
				t.Fatalf("unexpected mode %q", mode)
			}
			return PermissionModeResult{OK: false, Error: "mode rejected"}
		},
	})

	if response.Response.Subtype != ResponseSubtypeError {
		t.Fatalf("expected error, got %+v", response.Response)
	}
	if response.Response.Error != "mode rejected" {
		t.Fatalf("unexpected error %q", response.Response.Error)
	}
}

func TestHandleRequestSetPermissionModeNoHandler(t *testing.T) {
	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-permission",
		Request: ControlRequestPayload{
			Subtype: RequestSubtypeSetPermissionMode,
			Mode:    "plan",
		},
	}, Options{})

	if response.Response.Subtype != ResponseSubtypeError {
		t.Fatalf("expected error, got %+v", response.Response)
	}
	if !strings.Contains(response.Response.Error, "callback not registered") {
		t.Fatalf("unexpected error %q", response.Response.Error)
	}
}

func TestHandleRequestOutboundOnlyRejectsMutableRequest(t *testing.T) {
	called := false
	model := "claude-opus-4"

	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-model",
		Request: ControlRequestPayload{
			Subtype: RequestSubtypeSetModel,
			Model:   &model,
		},
	}, Options{
		OutboundOnly: true,
		OnSetModel: func(model *string) {
			called = true
		},
	})

	if response.Response.Subtype != ResponseSubtypeError {
		t.Fatalf("expected error, got %+v", response.Response)
	}
	if response.Response.Error != outboundOnlyError {
		t.Fatalf("unexpected error %q", response.Response.Error)
	}
	if called {
		t.Fatalf("outbound-only request should not call handler")
	}
}

func TestHandleRequestUnknownSubtypeError(t *testing.T) {
	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-unknown",
		Request:   ControlRequestPayload{Subtype: "reload_plugins"},
	}, Options{})

	if response.Response.Subtype != ResponseSubtypeError {
		t.Fatalf("expected error, got %+v", response.Response)
	}
	if !strings.Contains(response.Response.Error, "reload_plugins") {
		t.Fatalf("expected subtype in error, got %q", response.Response.Error)
	}
}

func TestRouteIngressControlResponse(t *testing.T) {
	called := false
	result := RouteIngress([]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"req-1"}}`), UUIDSets{}, IngressHandlers{
		OnControlResponse: func(response ControlResponse) {
			called = true
			if response.Response.RequestID != "req-1" {
				t.Fatalf("unexpected response: %+v", response)
			}
		},
	})

	if result.Kind != IngressControlResponse {
		t.Fatalf("expected control_response route, got %+v", result)
	}
	if !called {
		t.Fatalf("expected control response handler")
	}
}

func TestRouteIngressUserMessageEchoDedup(t *testing.T) {
	posted := NewBoundedUUIDSet(8)
	inbound := NewBoundedUUIDSet(8)
	posted.Add("echo-uuid")

	var got []SDKMessage
	handlers := IngressHandlers{
		OnUserMessage: func(message SDKMessage) {
			got = append(got, message)
		},
	}
	sets := UUIDSets{
		RecentPosted:  posted,
		RecentInbound: inbound,
	}

	echo := RouteIngress([]byte(`{"type":"user","uuid":"echo-uuid","message":{"content":"same"}}`), sets, handlers)
	if echo.Kind != IngressIgnored || echo.IgnoredReason != "posted_echo" {
		t.Fatalf("expected posted echo ignore, got %+v", echo)
	}
	if len(got) != 0 {
		t.Fatalf("echo should not reach user handler")
	}

	first := RouteIngress([]byte(`{"type":"user","uuid":"inbound-uuid","message":{"content":"hello"}}`), sets, handlers)
	if first.Kind != IngressUserMessage {
		t.Fatalf("expected user route, got %+v", first)
	}
	if len(got) != 1 || got[0].UUID != "inbound-uuid" {
		t.Fatalf("expected one inbound message, got %+v", got)
	}
	if !inbound.Has("inbound-uuid") {
		t.Fatalf("expected inbound uuid to be recorded")
	}

	duplicate := RouteIngress([]byte(`{"type":"user","uuid":"inbound-uuid","message":{"content":"hello again"}}`), sets, handlers)
	if duplicate.Kind != IngressIgnored || duplicate.IgnoredReason != "inbound_duplicate" {
		t.Fatalf("expected inbound duplicate ignore, got %+v", duplicate)
	}
	if len(got) != 1 {
		t.Fatalf("duplicate should not reach user handler")
	}
}

func TestBoundedUUIDSetEviction(t *testing.T) {
	set := NewBoundedUUIDSet(2)
	set.Add("a")
	set.Add("b")
	set.Add("a")
	set.Add("c")

	if set.Has("a") {
		t.Fatalf("expected oldest uuid to be evicted")
	}
	if !set.Has("b") || !set.Has("c") {
		t.Fatalf("expected b and c to remain")
	}

	set.Add("d")
	if set.Has("b") {
		t.Fatalf("expected b to be evicted next")
	}
	if !set.Has("c") || !set.Has("d") {
		t.Fatalf("expected c and d to remain")
	}
}

func TestRouteIngressControlRequestReturnsResponse(t *testing.T) {
	called := false
	result := RouteIngress([]byte(`{"type":"control_request","request_id":"req-init","request":{"subtype":"initialize"}}`), UUIDSets{}, IngressHandlers{
		RequestOptions: Options{PID: 99},
		OnControlRequest: func(request ControlRequest, response ControlResponse) {
			called = true
			if request.RequestID != "req-init" {
				t.Fatalf("unexpected request: %+v", request)
			}
			if response.Response.Subtype != ResponseSubtypeSuccess {
				t.Fatalf("unexpected response: %+v", response)
			}
		},
	})

	if result.Kind != IngressControlRequest {
		t.Fatalf("expected control_request route, got %+v", result)
	}
	if result.ResponseToSend == nil || result.ResponseToSend.Response.Subtype != ResponseSubtypeSuccess {
		t.Fatalf("expected response to send, got %+v", result.ResponseToSend)
	}
	if !called {
		t.Fatalf("expected control request handler")
	}
}

func TestInitializeResponseJSONUsesObjectsAndArrays(t *testing.T) {
	response := HandleRequest(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "req-init",
		Request:   ControlRequestPayload{Subtype: RequestSubtypeInitialize},
	}, Options{PID: 7})

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	body := parsed["response"].(map[string]any)
	payload := body["response"].(map[string]any)
	if _, ok := payload["account"].(map[string]any); !ok {
		t.Fatalf("expected account object, got %T", payload["account"])
	}
	if _, ok := payload["commands"].([]any); !ok {
		t.Fatalf("expected commands array, got %T", payload["commands"])
	}
}
