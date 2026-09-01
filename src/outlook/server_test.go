package outlook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
		if snapshot, err := manager.SnapshotServer(ServerName); err == nil && snapshot.State == mcpclient.StateConnected && snapshot.ToolCount == 37 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	snapshot, err := manager.SnapshotServer(ServerName)
	if err != nil || snapshot.State != mcpclient.StateConnected || snapshot.ToolCount != 37 {
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

func TestOutlookMCPUsesSimpleStringRecipientArrays(t *testing.T) {
	var paths []string
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.Path)
		var payload map[string]any
		if request.Body != nil {
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode Graph payload: %v", err)
			}
		}
		if strings.HasSuffix(request.URL.Path, "/sendMail") || strings.HasSuffix(request.URL.Path, "/forward") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"draft-1","subject":"QA","isDraft":true}`))
	}))
	defer graph.Close()

	runtime := NewRuntime(&fakeTokens{token: "access-token"})
	runtime.SetHTTPClientForTest(graph.Client(), graph.URL)
	runtime.client.scheduler = NewMailScheduler(runtime.client, t.TempDir())
	inProcess := mcpclient.NewInProcRegistry()
	if err := inProcess.Register(ServerName, runtime.Factory()); err != nil {
		t.Fatal(err)
	}
	manager := mcpclient.NewManager(mcpclient.WithInProcRegistry(inProcess))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager.Start(ctx, []mcpclient.ScopedMcpServerConfig{{
		Name: ServerName, Scope: mcpclient.ScopeUser,
		Config: mcpclient.McpServerConfig{InProc: &mcpclient.InProcTransportConfig{ServerName: ServerName}},
	}})
	defer manager.Shutdown(context.Background())
	waitForOutlookTools(t, manager)
	session, err := manager.Session(ServerName)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed.Tools {
		if item.Name != "list_messages" && item.Name != "find_mail_folder" {
			continue
		}
		raw, err := json.Marshal(item.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || len(properties) == 0 {
			t.Fatalf("%s output schema does not expose root properties: %s", item.Name, raw)
		}
		expected := "messages"
		if item.Name == "find_mail_folder" {
			expected = "matches"
		}
		if _, ok := properties[expected]; !ok {
			t.Fatalf("%s output schema is missing %q: %s", item.Name, expected, raw)
		}
		adapter := mcpclient.NewMcpToolAdapter(manager, ServerName, item)
		adaptedOutput := adapter.Definition().OutputSchema
		if item.Name == "find_mail_folder" {
			adaptedOutput = adaptedOutput.Properties["mcp_payload"]
		}
		if _, ok := adaptedOutput.Properties[expected]; !ok {
			t.Fatalf("%s adapter output schema lost %q: raw=%s adapted=%+v warnings=%v", item.Name, expected, raw, adapter.Definition().OutputSchema, adapter.OutputSchemaWarnings())
		}
	}
	tools, err := manager.ServerTools(ServerName)
	if err != nil {
		t.Fatal(err)
	}
	descriptions := map[string]string{}
	for _, item := range tools {
		descriptions[item.Name] = item.Description
	}
	for _, name := range []string{"send_draft", "send_mail", "reply_message", "forward_message", "schedule_mail", "schedule_draft"} {
		if !strings.Contains(descriptions[name], "确认") {
			t.Errorf("%s description must tell the model to confirm: %q", name, descriptions[name])
		}
	}
	for _, name := range []string{"search_messages", "get_recent_emails", "get_messages_batch", "find_mail_folder", "mark_message_read_state", "set_message_categories"} {
		if descriptions[name] == "" {
			t.Errorf("missing model guidance for %s", name)
		}
	}

	addressList := []string{"nolan@example.com"}
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{name: "create_draft", args: map[string]any{"subject": "QA", "body": "draft", "to": addressList}},
		{name: "send_mail", args: map[string]any{"subject": "QA", "body": "send", "to": addressList}},
		{name: "create_forward_draft", args: map[string]any{"messageId": "source-1", "comment": "draft", "to": addressList}},
		{name: "forward_message", args: map[string]any{"messageId": "source-1", "comment": "send", "to": addressList}},
		{name: "schedule_mail", args: map[string]any{
			"subject": "QA", "body": "scheduled", "to": addressList,
			"sendAt": time.Now().Add(time.Hour).Format(time.RFC3339),
		}},
	} {
		result, err := manager.CallTool(context.Background(), mcpclient.McpToolName(ServerName, test.name), test.args)
		if err != nil || result == nil || result.IsError {
			t.Fatalf("%s result=%+v err=%v", test.name, result, err)
		}
	}
	if len(paths) != 5 {
		t.Fatalf("Graph call count = %d, paths=%v", len(paths), paths)
	}
}

func TestSetMessageCategoriesSupportsExplicitClear(t *testing.T) {
	var patched []string
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPatch || !strings.HasSuffix(request.URL.Path, "/messages/m1") {
			t.Fatalf("unexpected Graph request %s %s", request.Method, request.URL.Path)
		}
		var payload struct {
			Categories []string `json:"categories"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		patched = payload.Categories
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m1","categories":[]}`))
	}))
	defer graph.Close()

	runtime := NewRuntime(&fakeTokens{token: "access-token"})
	runtime.SetHTTPClientForTest(graph.Client(), graph.URL)
	inProcess := mcpclient.NewInProcRegistry()
	if err := inProcess.Register(ServerName, runtime.Factory()); err != nil {
		t.Fatal(err)
	}
	manager := mcpclient.NewManager(mcpclient.WithInProcRegistry(inProcess))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager.Start(ctx, []mcpclient.ScopedMcpServerConfig{{
		Name: ServerName, Scope: mcpclient.ScopeUser,
		Config: mcpclient.McpServerConfig{InProc: &mcpclient.InProcTransportConfig{ServerName: ServerName}},
	}})
	defer manager.Shutdown(context.Background())
	waitForOutlookTools(t, manager)
	result, err := manager.CallTool(context.Background(), mcpclient.McpToolName(ServerName, "set_message_categories"), map[string]any{
		"messageId": "m1",
		"clear":     true,
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("clear result=%+v err=%v", result, err)
	}
	if patched == nil || len(patched) != 0 {
		t.Fatalf("Graph categories payload = %#v, want explicit empty array", patched)
	}
}

func waitForOutlookTools(t *testing.T, manager *mcpclient.Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot, err := manager.SnapshotServer(ServerName); err == nil && snapshot.State == mcpclient.StateConnected && snapshot.ToolCount == 37 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snapshot, err := manager.SnapshotServer(ServerName)
	t.Fatalf("Outlook tools did not become ready: snapshot=%+v err=%v", snapshot, err)
}
