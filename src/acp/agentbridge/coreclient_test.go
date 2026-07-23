package agentbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	awruntime "awaken/src/runtime"
)

func TestCoreClientVerifyIdentity(t *testing.T) {
	id, _ := awruntime.GenerateRuntimeIdentity(t.TempDir(), "headless", "http://127.0.0.1:18080")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/control/runtime/identity" {
			http.NotFound(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+id.ControlToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runtime_identity": id.RuntimeIdentity,
			"instance_id":      id.InstanceID,
		})
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, id.ControlToken)
	if err := client.VerifyIdentity(context.Background(), id); err != nil {
		t.Fatalf("verify identity: %v", err)
	}
}

func TestCoreClientVerifyIdentityMismatch(t *testing.T) {
	id, _ := awruntime.GenerateRuntimeIdentity(t.TempDir(), "headless", "http://127.0.0.1:18080")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runtime_identity": "other",
			"instance_id":      id.InstanceID,
		})
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, id.ControlToken)
	if err := client.VerifyIdentity(context.Background(), id); err == nil {
		t.Fatal("expected identity mismatch error")
	}
}

func TestCoreClientLeaseLifecycle(t *testing.T) {
	leaseID := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/control/leases":
			w.WriteHeader(http.StatusCreated)
			leaseID = "lease_test_123"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"lease_id":   leaseID,
				"expires_at": time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano),
			})
		case "POST /v1/control/leases/lease_test_123/heartbeat":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"expires_at": time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano),
			})
		case "DELETE /v1/control/leases/lease_test_123":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "released"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, "token")
	gotID, _, err := client.CreateLease(context.Background(), "bridge-1")
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	if gotID != leaseID {
		t.Fatalf("lease id mismatch: got %q, want %q", gotID, leaseID)
	}
	if _, err := client.HeartbeatLease(context.Background(), leaseID); err != nil {
		t.Fatalf("heartbeat lease: %v", err)
	}
	if err := client.ReleaseLease(context.Background(), leaseID); err != nil {
		t.Fatalf("release lease: %v", err)
	}
}

func TestCoreClientCreateACPSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /v1/control/acp/sessions" {
			http.NotFound(w, r)
			return
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"acp_session_id": req["acp_session_id"],
			"project_id":     "proj_1",
			"thread_id":      "thread_1",
			"canonical_cwd":  req["cwd"],
			"visibility":     req["visibility"],
		})
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, "token")
	mapping, err := client.CreateACPSession(context.Background(), "acp_1", "/tmp/ws", "visible", "test", "1.0")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if mapping.ThreadID != "thread_1" {
		t.Fatalf("unexpected thread id: %q", mapping.ThreadID)
	}
}

func TestCoreClientResolveExternalInteraction(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, "token")
	if err := client.resolveExternalInteraction(context.Background(), "run_1", "ix_1", "completed"); err != nil {
		t.Fatalf("resolve external interaction: %v", err)
	}
	if gotPath != "POST /v1/runs/run_1/external-interactions/ix_1/resolve" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotBody["resolution"] != "completed" {
		t.Fatalf("unexpected resolution: %q", gotBody["resolution"])
	}
}

func TestCoreClientStreamRunEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "GET /v1/runs/run_1/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"type":"TEXT_DELTA","seq":1,"content":"hello"}`)
		flusher.Flush()
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"type":"RUN_FINISHED","seq":2}`)
		flusher.Flush()
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, "token")
	frames, errs := client.StreamRunEvents(context.Background(), "run_1", 0)
	var got []string
	for frame := range frames {
		got = append(got, string(frame.Type))
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected stream to close")
	}
	if len(got) != 2 || strings.Join(got, ",") != "TEXT_DELTA,RUN_FINISHED" {
		t.Fatalf("unexpected frames: %v", got)
	}
}

func TestCoreClientCreateRunForSessionUsesCoreOwnedScope(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method+" "+r.URL.Path != "POST /v1/control/acp/sessions/acp_1/runs" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"run_id":    "run_1",
			"thread_id": "thread_1",
			"status":    "running",
		})
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, "token")
	if _, err := client.CreateRunForSession(context.Background(), "acp_1", "hello", "profile_1"); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if body["input"] != "hello" {
		t.Fatalf("input mismatch: %v", body["input"])
	}
	if body["model_profile_id"] != "profile_1" {
		t.Fatalf("model profile mismatch: %v", body["model_profile_id"])
	}
	if _, exists := body["thread_id"]; exists {
		t.Fatal("bridge must not submit Core-owned thread_id")
	}
	if _, exists := body["workspace_root"]; exists {
		t.Fatal("bridge must not submit Core-owned workspace_root")
	}
}

func TestCoreClientCreateRunForSessionIncludesBoundedErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"MISSING_MODEL_PROFILE","message":"model profile is required"}`))
	}))
	defer server.Close()

	client := NewCoreClient(server.URL, "token")
	_, err := client.CreateRunForSession(context.Background(), "acp_1", "hello", "")
	if err == nil {
		t.Fatal("expected create run error")
	}
	if !strings.Contains(err.Error(), "MISSING_MODEL_PROFILE") {
		t.Fatalf("expected response body in error, got %v", err)
	}
}
