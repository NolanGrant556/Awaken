package mcpmanagement

import (
	"errors"
	"testing"
)

func TestRegistryCRUD(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("expected registry, got %v", err)
	}

	added, err := registry.Add(testStdioServer("server-1", "Alpha"))
	if err != nil {
		t.Fatalf("expected add to succeed, got %v", err)
	}
	if added.ID != "server-1" || added.Name != "Alpha" {
		t.Fatalf("unexpected added server: %+v", added)
	}

	got, err := registry.Get("server-1")
	if err != nil {
		t.Fatalf("expected get to succeed, got %v", err)
	}
	if got.Transport.Command != "npx" {
		t.Fatalf("unexpected command %q", got.Transport.Command)
	}

	if listed := registry.List(); len(listed) != 1 || listed[0].ID != "server-1" {
		t.Fatalf("expected stable list with server-1, got %+v", listed)
	}

	disabled, err := registry.Enable("server-1", false)
	if err != nil {
		t.Fatalf("expected enable update to succeed, got %v", err)
	}
	if disabled.Enabled {
		t.Fatal("expected server to be disabled")
	}

	got.Description = "updated"
	updated, err := registry.Update(got)
	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if updated.Description != "updated" {
		t.Fatalf("expected updated description, got %q", updated.Description)
	}

	if err := registry.Delete("server-1"); err != nil {
		t.Fatalf("expected delete to succeed, got %v", err)
	}
	if _, err := registry.Get("server-1"); !errors.Is(err, ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
}

func TestRegistryRejectsBuiltinDelete(t *testing.T) {
	server := testStdioServer("builtin-1", "Builtin")
	server.Builtin = true
	registry, err := NewRegistry(server)
	if err != nil {
		t.Fatalf("expected registry, got %v", err)
	}

	if err := registry.Delete("builtin-1"); !errors.Is(err, ErrBuiltinDelete) {
		t.Fatalf("expected ErrBuiltinDelete, got %v", err)
	}
	got, err := registry.Get("builtin-1")
	if err != nil {
		t.Fatalf("expected builtin server to remain, got %v", err)
	}
	if !got.Builtin {
		t.Fatal("expected server to remain builtin")
	}
}

func TestNormalizeServerValidatesTransport(t *testing.T) {
	cases := []struct {
		name      string
		server    MCPServer
		wantError error
	}{
		{
			name:      "missing id",
			server:    MCPServer{Name: "Missing ID", Transport: TransportConfig{Type: TransportStdio, Command: "npx"}},
			wantError: ErrInvalidServer,
		},
		{
			name:      "missing name",
			server:    MCPServer{ID: "missing-name", Transport: TransportConfig{Type: TransportStdio, Command: "npx"}},
			wantError: ErrInvalidServer,
		},
		{
			name:      "missing transport",
			server:    MCPServer{ID: "missing-transport", Name: "Missing Transport"},
			wantError: ErrInvalidTransport,
		},
		{
			name:      "stdio missing command",
			server:    MCPServer{ID: "bad-stdio", Name: "Bad Stdio", Transport: TransportConfig{Type: TransportStdio}},
			wantError: ErrInvalidTransport,
		},
		{
			name:      "http invalid url",
			server:    MCPServer{ID: "bad-http", Name: "Bad HTTP", Transport: TransportConfig{Type: TransportHTTP, URL: "localhost:3000"}},
			wantError: ErrInvalidTransport,
		},
		{
			name:      "http rejects command fields",
			server:    MCPServer{ID: "bad-http-fields", Name: "Bad HTTP Fields", Transport: TransportConfig{Type: TransportHTTP, URL: "https://example.com/mcp", Command: "npx"}},
			wantError: ErrInvalidTransport,
		},
		{
			name:      "stdio rejects url fields",
			server:    MCPServer{ID: "bad-stdio-fields", Name: "Bad Stdio Fields", Transport: TransportConfig{Type: TransportStdio, Command: "npx", URL: "https://example.com/mcp"}},
			wantError: ErrInvalidTransport,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeServer(tc.server); !errors.Is(err, tc.wantError) {
				t.Fatalf("expected %v, got %v", tc.wantError, err)
			}
		})
	}

	valid := MCPServer{
		ID:   "streamable",
		Name: "Streamable",
		Transport: TransportConfig{
			Type: Transport("streamable-http"),
			URL:  "https://example.com/mcp",
		},
	}
	normalized, err := NormalizeServer(valid)
	if err != nil {
		t.Fatalf("expected streamable http to normalize, got %v", err)
	}
	if normalized.Transport.Type != TransportStreamableHTTP {
		t.Fatalf("expected normalized streamable_http, got %q", normalized.Transport.Type)
	}
}

func TestNormalizeRedactsSecretsAndTestPlanStaysRedacted(t *testing.T) {
	stdio := testStdioServer("env-server", "Env Server")
	stdio.Transport.Env = map[string]string{
		"API_TOKEN": "secret-token",
		"PATH":      "/usr/bin",
	}
	registry, err := NewRegistry(stdio)
	if err != nil {
		t.Fatalf("expected registry, got %v", err)
	}

	got, err := registry.Get("env-server")
	if err != nil {
		t.Fatalf("expected get to succeed, got %v", err)
	}
	if got.Transport.Env["API_TOKEN"] != RedactedValue {
		t.Fatalf("expected API_TOKEN to be redacted, got %q", got.Transport.Env["API_TOKEN"])
	}
	if got.Transport.Env["PATH"] != "/usr/bin" {
		t.Fatalf("expected PATH to remain visible, got %q", got.Transport.Env["PATH"])
	}

	plan, err := registry.TestPlan("env-server")
	if err != nil {
		t.Fatalf("expected test plan, got %v", err)
	}
	if plan.Env["API_TOKEN"] != RedactedValue || plan.Command != "npx" {
		t.Fatalf("unexpected stdio test plan: %+v", plan)
	}

	httpServer := MCPServer{
		ID:      "http-server",
		Name:    "HTTP Server",
		Enabled: true,
		Transport: TransportConfig{
			Type: TransportHTTP,
			URL:  "https://example.com/mcp",
			Headers: map[string]string{
				"Authorization": "Bearer token",
				"Accept":        "application/json",
			},
		},
	}
	added, err := registry.Add(httpServer)
	if err != nil {
		t.Fatalf("expected http add to succeed, got %v", err)
	}
	if added.Transport.Headers["Authorization"] != RedactedValue {
		t.Fatalf("expected Authorization header to be redacted, got %q", added.Transport.Headers["Authorization"])
	}
	if added.Transport.Headers["Accept"] != "application/json" {
		t.Fatalf("expected Accept header to remain visible, got %q", added.Transport.Headers["Accept"])
	}
}

func TestImportCandidatesNormalizePlan(t *testing.T) {
	registry, err := NewRegistry(testStdioServer("existing", "Existing"))
	if err != nil {
		t.Fatalf("expected registry, got %v", err)
	}

	plan, err := registry.ImportCandidates(ImportSourceClaudeCode, []ImportCandidate{
		{
			Server: MCPServer{
				ID:      "imported",
				Name:    " Imported ",
				Enabled: true,
				Transport: TransportConfig{
					Type: TransportHTTP,
					URL:  "https://example.com/mcp",
					Headers: map[string]string{
						"X-API-Key": "secret",
					},
				},
				Tools: []ToolInfo{
					{Name: "search", Description: "first"},
					{Name: " Search ", Description: "duplicate"},
				},
			},
		},
		{Server: testStdioServer("duplicate-name", "existing")},
		{Server: testStdioServer("imported", "Other")},
		{
			Server: MCPServer{
				ID:        "invalid",
				Name:      "Invalid",
				Transport: TransportConfig{Type: TransportHTTP, URL: "not-a-url"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected import plan, got %v", err)
	}

	if plan.Source != ImportSourceClaudeCode || plan.ImportableCount != 1 || plan.SkippedCount != 3 {
		t.Fatalf("unexpected import counts: %+v", plan)
	}
	first := plan.Candidates[0]
	if !first.Importable || first.Server.Name != "Imported" {
		t.Fatalf("expected first candidate normalized and importable, got %+v", first)
	}
	if first.Server.Transport.Headers["X-API-Key"] != RedactedValue {
		t.Fatalf("expected imported header secret redacted, got %q", first.Server.Transport.Headers["X-API-Key"])
	}
	if len(first.Server.Tools) != 1 || first.Server.Tools[0].Description != "first" {
		t.Fatalf("expected duplicate tools to be removed, got %+v", first.Server.Tools)
	}
	if plan.Candidates[1].Importable || plan.Candidates[1].SkipReason != "already exists" {
		t.Fatalf("expected existing server to be skipped, got %+v", plan.Candidates[1])
	}
	if plan.Candidates[2].Importable || plan.Candidates[2].SkipReason != "duplicate candidate" {
		t.Fatalf("expected duplicate candidate to be skipped, got %+v", plan.Candidates[2])
	}
	if plan.Candidates[3].Importable || plan.Candidates[3].SkipReason == "" {
		t.Fatalf("expected invalid candidate to be skipped with reason, got %+v", plan.Candidates[3])
	}
}

func TestRegistryConnectionAndOAuthState(t *testing.T) {
	registry, err := NewRegistry(testStdioServer("stateful", "Stateful"))
	if err != nil {
		t.Fatalf("expected registry, got %v", err)
	}

	updated, err := registry.SetConnectionStatus("stateful", ConnectionStatus{
		State:   ConnectionConnected,
		Message: " connected ",
		Tools: []ToolInfo{
			{Name: "read", Description: "first"},
			{Name: "READ", Description: "duplicate"},
		},
	})
	if err != nil {
		t.Fatalf("expected connection status update, got %v", err)
	}
	if updated.Connection.State != ConnectionConnected || updated.Connection.Message != "connected" {
		t.Fatalf("unexpected connection status: %+v", updated.Connection)
	}
	if len(updated.Connection.Tools) != 1 || updated.Connection.Tools[0].Name != "read" {
		t.Fatalf("expected duplicate connection tools to be removed, got %+v", updated.Connection.Tools)
	}

	updated, err = registry.SetOAuthState("stateful", OAuthState{
		Status:  OAuthRequired,
		AuthURL: " https://auth.example.com/start ",
		Message: " login required ",
	})
	if err != nil {
		t.Fatalf("expected oauth state update, got %v", err)
	}
	if updated.OAuth.Status != OAuthRequired || updated.OAuth.AuthURL != "https://auth.example.com/start" || updated.OAuth.Message != "login required" {
		t.Fatalf("unexpected oauth state: %+v", updated.OAuth)
	}
}

func TestNormalizeDedupesDuplicateTools(t *testing.T) {
	server := testStdioServer("tools", "Tools")
	server.Tools = []ToolInfo{
		{Name: "read", Description: "first"},
		{Name: " read ", Description: "duplicate"},
		{Name: "write", Description: "second"},
	}

	normalized, err := NormalizeServer(server)
	if err != nil {
		t.Fatalf("expected normalize to succeed, got %v", err)
	}
	if len(normalized.Tools) != 2 {
		t.Fatalf("expected 2 unique tools, got %+v", normalized.Tools)
	}
	if normalized.Tools[0].Name != "read" || normalized.Tools[0].Description != "first" {
		t.Fatalf("expected first duplicate to win, got %+v", normalized.Tools[0])
	}
}

func testStdioServer(id string, name string) MCPServer {
	return MCPServer{
		ID:      id,
		Name:    name,
		Enabled: true,
		Transport: TransportConfig{
			Type:    TransportStdio,
			Command: "npx",
			Args:    []string{"-y", "@example/mcp"},
		},
	}
}
