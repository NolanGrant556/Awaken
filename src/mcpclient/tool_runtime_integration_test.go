package mcpclient_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/mcpclient"
	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type unrestrictedRuntimeInput struct {
	Command    string           `json:"command,omitempty"`
	ID         string           `json:"id,omitempty"`
	Recipients []map[string]any `json:"recipients,omitempty"`
}

type unrestrictedRuntimeOutput struct {
	OK bool `json:"ok"`
}

func TestToolRuntimeLetsConnectedMCPToolsCallProviderDirectly(t *testing.T) {
	const serverName = "runtime-unrestricted-mcp"
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
		{Name: "delete_record"},
	}
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		for _, definition := range definitions {
			mcp.AddTool(server, definition, func(_ context.Context, _ *mcp.CallToolRequest, _ unrestrictedRuntimeInput) (*mcp.CallToolResult, unrestrictedRuntimeOutput, error) {
				calls.Add(1)
				return nil, unrestrictedRuntimeOutput{OK: true}, nil
			})
		}
		return server
	}); err != nil {
		t.Fatalf("register in-process MCP server: %v", err)
	}
	mgr := mcpclient.NewManager(mcpclient.WithInProcRegistry(registry))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []mcpclient.ScopedMcpServerConfig{{
		Name: serverName, Scope: mcpclient.ScopeProject,
		Config: mcpclient.McpServerConfig{InProc: &mcpclient.InProcTransportConfig{ServerName: serverName}},
	}})
	defer mgr.Shutdown(context.Background())
	defer mcpclient.UnregisterMcpToolsForTest(serverName)
	if err := mcpclient.WaitForServerStateForTest(mgr, serverName, mcpclient.StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for MCP server: %v", err)
	}
	officeToolName := mcpclient.McpToolName(serverName, "officecli")
	request := agentcore.NewRequestBuilder(agentcore.AgentConfig{}).Build(convcontext.New(), nil)
	officeExposed := false
	for _, definition := range request.Tools {
		if definition.Function.Name == officeToolName {
			officeExposed = true
			if _, ok := definition.Function.Parameters.Properties["command"]; !ok {
				t.Fatalf("OfficeCLI command schema was not exposed to the model: %+v", definition.Function.Parameters)
			}
			break
		}
	}
	if !officeExposed {
		t.Fatalf("connected OfficeCLI-style MCP tool was not exposed to the model")
	}

	runtime := agentcore.NewToolRuntime(nil)
	callsToMake := []convcontext.ToolCall{
		{ID: "office", Function: convcontext.FunctionCall{Name: officeToolName, Arguments: `{"command":"word create report.docx"}`}},
		{ID: "message", Function: convcontext.FunctionCall{Name: mcpclient.McpToolName(serverName, "send_message"), Arguments: `{"recipients":[{"id":"group-1","kind":"group"},{"id":"group-2","kind":"group"}]}`}},
		{ID: "delete", Function: convcontext.FunctionCall{Name: mcpclient.McpToolName(serverName, "delete_record"), Arguments: `{"id":"42"}`}},
	}
	for _, call := range callsToMake {
		result := runtime.Execute(ctx, call)
		if result.Outcome.Status != tool.OutcomeSucceeded || result.Outcome.TransportStatus != tool.TransportCompleted {
			t.Fatalf("%s was blocked before MCP transport: %+v", call.Function.Name, result)
		}
	}
	if calls.Load() != int32(len(callsToMake)) {
		t.Fatalf("expected all MCP calls to reach provider, calls=%d", calls.Load())
	}
}
