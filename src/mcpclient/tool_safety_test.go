package mcpclient

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func providerMetadata(value MCPToolProviderMetadata) mcp.Meta {
	return mcp.Meta{AwakenProviderMetaKey: value}
}

func TestMCPCallsDoNotRequireAwakenProviderMetadata(t *testing.T) {
	tests := []struct {
		name string
		tool *mcp.Tool
	}{
		{
			name: "metadata-less mutation",
			tool: &mcp.Tool{Name: "update_record"},
		},
		{
			name: "contradictory annotations",
			tool: &mcp.Tool{
				Name: "sync_record",
				Annotations: &mcp.ToolAnnotations{
					ReadOnlyHint: true, DestructiveHint: boolPtr(true), OpenWorldHint: boolPtr(true),
				},
			},
		},
		{
			name: "invalid optional metadata",
			tool: &mcp.Tool{
				Name: "archive_record",
				Meta: mcp.Meta{AwakenProviderMetaKey: map[string]any{"provider_id": 42}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := NewMcpToolAdapter(nil, "third-party", test.tool)
			decision := adapter.CheckPermission(context.Background(), `{}`, tool.PermissionContext{})
			if decision.Behavior != tool.PermissionAllow {
				t.Fatalf("connected MCP semantics should not require Awaken metadata: %+v", decision)
			}
			if test.name == "invalid optional metadata" && len(adapter.ProviderManifest().Warnings) == 0 {
				t.Fatal("invalid optional metadata should remain diagnostic without becoming a call gate")
			}
		})
	}
}

type unrestrictedInput struct {
	Command string `json:"command,omitempty"`
	ID      string `json:"id,omitempty"`
}

type unrestrictedOutput struct {
	OK bool `json:"ok"`
}

func TestMCPGenericMessageAndDeleteToolsReachTransportWithoutHostGates(t *testing.T) {
	const serverName = "unrestricted-mcp-tools"
	var calls atomic.Int32
	definitions := []*mcp.Tool{
		{
			Name: "officecli",
			InputSchema: map[string]any{
				"type": "object", "required": []string{"command"},
				"properties": map[string]any{"command": map[string]any{"type": "string"}},
			},
		},
		{Name: "send_message"},
		{Name: "broadcast_message"},
		{Name: "delete_record"},
	}
	registry := NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		for _, definition := range definitions {
			mcp.AddTool(server, definition, func(_ context.Context, _ *mcp.CallToolRequest, _ unrestrictedInput) (*mcp.CallToolResult, unrestrictedOutput, error) {
				calls.Add(1)
				return nil, unrestrictedOutput{OK: true}, nil
			})
		}
		return server
	}); err != nil {
		t.Fatalf("register in-process server: %v", err)
	}

	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{{
		Name: serverName, Scope: ScopeProject,
		Config: McpServerConfig{InProc: &InProcTransportConfig{ServerName: serverName}},
	}})
	defer mgr.Shutdown(context.Background())
	defer UnregisterMcpToolsForTest(serverName)
	if err := WaitForServerStateForTest(mgr, serverName, StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for server: %v", err)
	}

	inputs := []string{`{"command":"word create report.docx"}`, `{}`, `{}`, `{"id":"42"}`}
	for index, definition := range definitions {
		adapter := NewMcpToolAdapter(mgr, serverName, definition)
		if decision := adapter.CheckPermission(ctx, inputs[index], tool.PermissionContext{}); decision.Behavior != tool.PermissionAllow {
			t.Fatalf("%s was unexpectedly gated: %+v", definition.Name, decision)
		}
		outcome, err := adapter.ExecuteOutcome(ctx, inputs[index], nil)
		if err != nil || outcome.Status != tool.OutcomeSucceeded || outcome.TransportStatus != tool.TransportCompleted {
			t.Fatalf("%s did not reach MCP transport: outcome=%+v err=%v", definition.Name, outcome, err)
		}
	}
	if calls.Load() != int32(len(definitions)) {
		t.Fatalf("expected every connected MCP tool to execute once, calls=%d", calls.Load())
	}
}

func TestMCPProviderManifestIncludesOptionalMetadataAndReadiness(t *testing.T) {
	mgr := NewManager()
	mgr.servers["mail"] = &managedServer{
		name:  "mail",
		state: StateNeedsAuth,
		config: McpServerConfig{Provider: &ServerProviderManifest{
			ProviderID: "connector:mail", DisplayName: "Mail", Source: tool.SourceConnector,
		}},
		errorReason: "login required",
	}
	adapter := NewMcpToolAdapter(mgr, "mail", &mcp.Tool{
		Name: "send_message",
		Meta: providerMetadata(MCPToolProviderMetadata{
			ProviderID: "connector:mail:send", Capabilities: []tool.Capability{tool.CapabilityMessageSend},
		}),
	})
	manifest := adapter.ProviderManifest()
	if manifest.ProviderID != "connector:mail:send" || manifest.Source != tool.SourceConnector {
		t.Fatalf("unexpected provider identity: %+v", manifest)
	}
	if !tool.HasCapability(manifest.Capabilities, tool.CapabilityMessageSend) {
		t.Fatalf("message capability missing: %+v", manifest.Capabilities)
	}
	if len(manifest.Dependencies) == 0 || manifest.Dependencies[len(manifest.Dependencies)-1].Kind != tool.DependencyMCPConnector {
		t.Fatalf("MCP connection dependency missing: %+v", manifest.Dependencies)
	}
	readiness := adapter.ProviderReadiness(tool.ExecutionEnvironmentSnapshot{})
	if readiness.Readiness != tool.ProviderNeedsAuth || readiness.Reason != "login required" {
		t.Fatalf("needs-auth readiness lost: %+v", readiness)
	}
}
