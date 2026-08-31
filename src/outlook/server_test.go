package outlook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"awaken/src/mcpclient"
)

func TestRuntimePublishesOutlookToolsAndTurnsGraph401IntoNeedsAuth(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer graph.Close()

	tokens := &fakeTokens{token: "expired"}
	runtime := NewRuntime(tokens)
	runtime.SetHTTPClientForTest(graph.Client(), graph.URL)
	inProcess := mcpclient.NewInProcRegistry()
	if err := inProcess.Register(ServerName, runtime.Factory()); err != nil {
		t.Fatal(err)
	}
	manager := mcpclient.NewManager(mcpclient.WithInProcRegistry(inProcess))
	runtime.SetNeedsAuthCallback(manager.MarkServerNeedsAuth)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager.Start(ctx, []mcpclient.ScopedMcpServerConfig{{
		Name: ServerName, Scope: mcpclient.ScopeUser,
		Config: mcpclient.McpServerConfig{InProc: &mcpclient.InProcTransportConfig{ServerName: ServerName}},
	}})
	defer manager.Shutdown(context.Background())

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot, err := manager.SnapshotServer(ServerName); err == nil && snapshot.State == mcpclient.StateConnected && snapshot.ToolCount == 8 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	snapshot, err := manager.SnapshotServer(ServerName)
	if err != nil || snapshot.State != mcpclient.StateConnected || snapshot.ToolCount != 8 {
		t.Fatalf("snapshot = %+v err = %v", snapshot, err)
	}
	_, err = manager.CallTool(context.Background(), "mcp__awaken-outlook__get_message", map[string]any{"messageId": "m1"})
	var needsAuth *mcpclient.NeedsAuthError
	if !errors.As(err, &needsAuth) || needsAuth.ServerName != ServerName {
		t.Fatalf("expected typed needs-auth error, got %T %v", err, err)
	}
	snapshot, err = manager.SnapshotServer(ServerName)
	if err != nil || snapshot.State != mcpclient.StateNeedsAuth {
		t.Fatalf("snapshot after 401 = %+v err = %v", snapshot, err)
	}
}
