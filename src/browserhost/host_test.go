package browserhost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetRejectsNonLoopbackAndClears(t *testing.T) {
	t.Cleanup(func() { _ = Set(nil) })
	if err := Set(&Registration{BaseURL: "https://example.com", Token: "secret"}); err == nil {
		t.Fatal("expected non-loopback host to fail")
	}
	if Ready() {
		t.Fatal("host should stay unregistered")
	}
	if err := Set(&Registration{BaseURL: "http://127.0.0.1:9", Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	if !Ready() {
		t.Fatal("expected ready after loopback registration")
	}
	if err := Set(nil); err != nil {
		t.Fatal(err)
	}
	if Ready() {
		t.Fatal("expected cleared host")
	}
}

func TestCallFailClosedWithoutHost(t *testing.T) {
	t.Cleanup(func() { _ = Set(nil) })
	_ = Set(nil)
	result, err := Call(context.Background(), ActionRequest{Action: "snapshot", ThreadID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "browser_host_unavailable" {
		t.Fatalf("got %+v", result)
	}
}

func TestCallPostsAuthorizedAction(t *testing.T) {
	t.Cleanup(func() { _ = Set(nil) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var request ActionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode: %v", err)
		}
		if request.Action != "snapshot" || request.ThreadID != "thread" {
			t.Errorf("unexpected request %+v", request)
		}
		_ = json.NewEncoder(w).Encode(Result{OK: true, Snapshot: "[1] a Home"})
	}))
	t.Cleanup(server.Close)
	if err := Set(&Registration{BaseURL: server.URL, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	result, err := Call(context.Background(), ActionRequest{Action: "snapshot", ThreadID: "thread"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Snapshot != "[1] a Home" {
		t.Fatalf("got %+v", result)
	}
}

func TestCallRejectsAnotherInvocationReceipt(t *testing.T) {
	t.Cleanup(func() { _ = Set(nil) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Result{OK: true, RequestID: "call-other", URL: "https://wrong.example/"})
	}))
	defer server.Close()
	if err := Set(&Registration{BaseURL: server.URL, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	result, err := Call(context.Background(), ActionRequest{Action: "navigate", ThreadID: "thread", RequestID: "call-owned"})
	if err != nil || result.OK || result.Error != "browser_receipt_mismatch" || result.URL != "" {
		t.Fatalf("unexpected receipt: %+v, %v", result, err)
	}
}

func TestReleaseOwnerCarriesAssignmentAndEndReason(t *testing.T) {
	t.Cleanup(func() { _ = Set(nil) })
	var requests []ActionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ActionRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		requests = append(requests, request)
		_ = json.NewEncoder(w).Encode(Result{OK: true})
	}))
	defer server.Close()
	if err := Set(&Registration{BaseURL: server.URL, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	ReleaseOwner("thread", "run", "agent", "task", false)
	ReleaseOwner("thread", "run", "agent", "task", true)
	if len(requests) != 2 || requests[0].Action != "release" || requests[1].Action != "end_owner" || requests[1].AgentID != "agent" || requests[1].TaskID != "task" {
		t.Fatalf("unexpected cleanup: %+v", requests)
	}
}
