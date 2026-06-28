package mcp

import (
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestStaticRegistrySnapshotAndToolDefinitions(t *testing.T) {
	originalHeaders := map[string]string{
		"Authorization": "Bearer secret-token",
		"x-api-key":     "super-secret-key",
	}
	registry, err := NewStaticRegistry([]Server{
		{
			Config: ServerConfig{
				ID:        "docs server",
				Name:      "Docs Server",
				Transport: TransportHTTP,
				Enabled:   true,
				URL:       "https://example.invalid/mcp",
				Headers:   originalHeaders,
			},
			State: ServerRuntime{
				Mode:   RuntimeModeStatic,
				Status: ServerStatusReady,
			},
			Tools: []ToolDescriptor{
				{
					Name:          "lookup page",
					Description:   "Look up documentation",
					ExposeToModel: true,
					InputSchema: tool.ToolParametersSchema{
						Type: "object",
						Properties: map[string]tool.ToolParam{
							"query": {Type: "string", Description: "Search query"},
						},
						Required: []string{"query"},
					},
				},
				{
					Name:        "hidden tool",
					Description: "not exposed yet",
					InputSchema: tool.ToolParametersSchema{
						Type:       "object",
						Properties: map[string]tool.ToolParam{},
					},
				},
			},
		},
		{
			Config: ServerConfig{
				ID:        "filesystem",
				Name:      "Filesystem",
				Transport: TransportStdio,
				Enabled:   false,
				Command:   "npx",
				Args:      []string{"server-filesystem"},
			},
			State: ServerRuntime{
				Mode:    RuntimeModeTestDouble,
				Status:  ServerStatusDisabled,
				Message: "disabled in current profile",
			},
			Tools: []ToolDescriptor{{
				Name:          "read_file",
				Description:   "Reads a file",
				ExposeToModel: true,
				InputSchema: tool.ToolParametersSchema{
					Type: "object",
					Properties: map[string]tool.ToolParam{
						"path": {Type: "string", Description: "File path"},
					},
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("failed to build registry: %v", err)
	}

	snapshot := registry.Snapshot()
	if snapshot.Summary.ServerCount != 2 {
		t.Fatalf("expected 2 servers, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.ReadyServerCount != 1 {
		t.Fatalf("expected 1 ready server, got %+v", snapshot.Summary)
	}
	if snapshot.Summary.ToolCount != 3 || snapshot.Summary.ExposedToolCount != 1 {
		t.Fatalf("unexpected summary counts: %+v", snapshot.Summary)
	}
	if got := snapshot.Servers[0].Config.Headers["Authorization"]; got != RedactedHeaderValue {
		t.Fatalf("expected Authorization header to be redacted, got %q", got)
	}
	if got := snapshot.Servers[0].Config.Headers["x-api-key"]; got != RedactedHeaderValue {
		t.Fatalf("expected x-api-key header to be redacted, got %q", got)
	}
	if originalHeaders["Authorization"] != "Bearer secret-token" || originalHeaders["x-api-key"] != "super-secret-key" {
		t.Fatalf("snapshot redaction must not mutate original headers: %+v", originalHeaders)
	}

	definitions := registry.ToolDefinitions()
	if len(definitions) != 1 {
		t.Fatalf("expected 1 exposed definition, got %+v", definitions)
	}
	if definitions[0].Function.Name != "mcp__docs_server__lookup_page" {
		t.Fatalf("unexpected adapted tool name %q", definitions[0].Function.Name)
	}
	if !strings.Contains(definitions[0].Function.Description, "Docs Server") {
		t.Fatalf("expected server context in description, got %q", definitions[0].Function.Description)
	}
}

func TestRedactedSnapshotRedactsStandaloneSnapshots(t *testing.T) {
	raw := Snapshot{
		Servers: []Server{{
			Config: ServerConfig{
				ID:        "raw",
				Name:      "Raw",
				Transport: TransportHTTP,
				Enabled:   true,
				URL:       "https://example.invalid",
				Headers: map[string]string{
					"Authorization": "Bearer raw-secret",
				},
			},
			State: ServerRuntime{
				Mode:   RuntimeModeStatic,
				Status: ServerStatusReady,
			},
		}},
		Summary: SnapshotSummary{ServerCount: 1},
	}

	redacted := RedactedSnapshot(raw)
	if redacted.Servers[0].Config.Headers["Authorization"] != RedactedHeaderValue {
		t.Fatalf("expected redacted Authorization header, got %+v", redacted.Servers[0].Config.Headers)
	}
	if raw.Servers[0].Config.Headers["Authorization"] != "Bearer raw-secret" {
		t.Fatalf("redaction must not mutate source snapshot: %+v", raw.Servers[0].Config.Headers)
	}
}

func TestNewStaticRegistryRejectsInvalidConfigs(t *testing.T) {
	_, err := NewStaticRegistry([]Server{{
		Config: ServerConfig{
			ID:        "broken",
			Name:      "Broken",
			Transport: TransportSSE,
			Enabled:   true,
		},
		State: ServerRuntime{
			Mode:   RuntimeModeStatic,
			Status: ServerStatusReady,
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "requires url") {
		t.Fatalf("expected missing url error, got %v", err)
	}
}

func TestNewStaticRegistryRejectsDuplicateToolNames(t *testing.T) {
	_, err := NewStaticRegistry([]Server{{
		Config: ServerConfig{
			ID:        "dup",
			Name:      "Dup",
			Transport: TransportHTTP,
			Enabled:   true,
			URL:       "https://example.invalid",
		},
		State: ServerRuntime{
			Mode:   RuntimeModeStatic,
			Status: ServerStatusReady,
		},
		Tools: []ToolDescriptor{
			{Name: "same", InputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{}}},
			{Name: "same", InputSchema: tool.ToolParametersSchema{Type: "object", Properties: map[string]tool.ToolParam{}}},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "duplicate tool") {
		t.Fatalf("expected duplicate tool error, got %v", err)
	}
}
