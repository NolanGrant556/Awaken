package mcpclient_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/mcpclient"
	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Real tools/list and calls through Manager + ToolRuntime + supervisor. The
// read hint must survive MCP serialization and canonical Outcome normalization.
func TestMixedMCPReadOperationsRepeatWhileDuplicateWritesStayBlocked(t *testing.T) {
	const serverName = "runtime-mixed-operations"
	var reads, writes atomic.Int32
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "1"}, nil)
		server.AddTool(&mcp.Tool{
			Name: "records", Annotations: &mcp.ToolAnnotations{DestructiveHint: tool.BoolPtr(true)},
			InputSchema: map[string]any{"type": "object", "required": []string{"action"}, "properties": map[string]any{"action": map[string]any{"type": "string"}, "idempotency_key": map[string]any{"type": "string"}}},
			Meta:        mcp.Meta{"io.awaken.tool/read-operations/v1": map[string]any{"parameter": "action", "values": []string{"query"}}},
		}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if string(req.Params.Arguments) == `{"action":"query"}` {
				reads.Add(1)
			} else {
				writes.Add(1)
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
		})
		return server
	}); err != nil {
		t.Fatal(err)
	}
	mgr := mcpclient.NewManager(mcpclient.WithInProcRegistry(registry))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr.Start(ctx, []mcpclient.ScopedMcpServerConfig{{Name: serverName, Scope: mcpclient.ScopeProject, Config: mcpclient.McpServerConfig{InProc: &mcpclient.InProcTransportConfig{ServerName: serverName}}}})
	defer mgr.Shutdown(context.Background())
	defer mcpclient.UnregisterMcpToolsForTest(serverName)
	if err := mcpclient.WaitForServerStateForTest(mgr, serverName, mcpclient.StateConnected, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	runtime := agentcore.NewToolRuntime(nil)
	supervisor := agentcore.NewFailureSupervisor(t.TempDir(), 16, 32)
	name := mcpclient.McpToolName(serverName, "records")
	for i, args := range []string{`{"action":"query"}`, `{"action":"query"}`, `{"action":"execute","idempotency_key":"one"}`, `{"action":"query"}`, `{"action":"execute","idempotency_key":"two"}`, `{"action":"resume"}`} {
		call := convcontext.ToolCall{ID: fmt.Sprint(i), Function: convcontext.FunctionCall{Name: name, Arguments: args}}
		decision := supervisor.BeforeCall(call)
		if i == 4 {
			if !decision.Block || decision.ErrorCode != agentcore.ErrorCodeRepeatedSuccessfulMutation {
				t.Fatalf("repeat write escaped: %+v", decision)
			}
			continue
		}
		if !decision.Allow {
			t.Fatalf("call %d blocked: %+v", i, decision)
		}
		supervisor.NoteRealExecution(decision.CallSignature)
		result := runtime.Execute(ctx, call)
		if result.Outcome.IsError() {
			t.Fatalf("call %d failed: %+v", i, result.Outcome)
		}
		want := tool.OutcomeSideEffectUnknown
		if i == 0 || i == 1 || i == 3 {
			want = tool.OutcomeSideEffectNone
		}
		if result.Outcome.Operation.SideEffectState != want {
			t.Fatalf("call %d side effects: %+v", i, result.Outcome)
		}
		supervisor.AfterOutcome(call, result.Outcome)
	}
	if reads.Load() != 3 || writes.Load() != 2 {
		t.Fatalf("provider count reads=%d writes=%d", reads.Load(), writes.Load())
	}
}

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
