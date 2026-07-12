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

type runtimeDeleteInput struct {
	ID string `json:"id"`
}

type runtimeDeleteOutput struct {
	Deleted bool `json:"deleted"`
}

type runtimeRecipient struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type runtimeMessageInput struct {
	Recipients []runtimeRecipient `json:"recipients"`
	Body       string             `json:"body"`
}

type runtimeMessageOutput struct {
	Sent bool `json:"sent"`
}

type approvingMassReviewSink struct {
	request agentcore.AwakenEvent
}

func (s *approvingMassReviewSink) Emit(agentcore.AwakenEvent) {}

func (s *approvingMassReviewSink) AwaitMassMessageReview(_ context.Context, request agentcore.AwakenEvent) (agentcore.ReviewDecision, error) {
	s.request = request
	return agentcore.ReviewDecisionApproved, nil
}

func TestToolRuntimePreservesUnsupportedRecoverableDeleteCode(t *testing.T) {
	const serverName = "runtime-delete-preflight"
	var calls atomic.Int32
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, &mcp.Tool{
			Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
				Capabilities: []tool.Capability{tool.CapabilityFileDelete},
			}},
			Name: "delete_record",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ runtimeDeleteInput) (*mcp.CallToolResult, runtimeDeleteOutput, error) {
			calls.Add(1)
			return nil, runtimeDeleteOutput{Deleted: true}, nil
		})
		return server
	}); err != nil {
		t.Fatalf("register in-process MCP server: %v", err)
	}
	mgr := mcpclient.NewManager(mcpclient.WithInProcRegistry(registry))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []mcpclient.ScopedMcpServerConfig{{
		Name:  serverName,
		Scope: mcpclient.ScopeProject,
		Config: mcpclient.McpServerConfig{
			InProc: &mcpclient.InProcTransportConfig{ServerName: serverName},
		},
	}})
	defer mgr.Shutdown(context.Background())
	defer mcpclient.UnregisterMcpToolsForTest(serverName)
	if err := mcpclient.WaitForServerStateForTest(mgr, serverName, mcpclient.StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for MCP server: %v", err)
	}
	toolName := mcpclient.McpToolName(serverName, "delete_record")

	result := agentcore.NewToolRuntime(nil).Execute(ctx, convcontext.ToolCall{
		ID: "remote_delete_preflight",
		Function: convcontext.FunctionCall{
			Name:      toolName,
			Arguments: `{"id":"42"}`,
		},
	})
	if result.ErrorCode != mcpclient.ErrorCodeUnsupportedRecoverableDelete ||
		result.Outcome.Status != tool.OutcomeUnsupported ||
		result.Outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("ToolRuntime did not preserve remote-delete preflight code: %+v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("ToolRuntime preflight reached remote MCP delete (%d calls)", calls.Load())
	}
}

func TestToolRuntimeUsesInputAwareMCPMessageEgress(t *testing.T) {
	const serverName = "runtime-message-egress"
	var calls atomic.Int32
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, &mcp.Tool{
			Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
				Capabilities: []tool.Capability{tool.CapabilityMessageSend},
				MessageEgress: &mcpclient.MCPMessageEgressContract{
					Channel:            "email",
					SendingAccount:     "sender@example.com",
					RecipientsField:    "recipients",
					RecipientIDField:   "id",
					RecipientKindField: "kind",
					BodyField:          "body",
				},
			}},
			Name: "send_message",
		}, func(_ context.Context, _ *mcp.CallToolRequest, _ runtimeMessageInput) (*mcp.CallToolResult, runtimeMessageOutput, error) {
			calls.Add(1)
			return nil, runtimeMessageOutput{Sent: true}, nil
		})
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
	toolName := mcpclient.McpToolName(serverName, "send_message")
	runtime := agentcore.NewToolRuntime(nil)

	single := runtime.Execute(ctx, convcontext.ToolCall{ID: "single", Function: convcontext.FunctionCall{
		Name: toolName, Arguments: `{"recipients":[{"id":"one@example.com","kind":"individual"}],"body":"hello"}`,
	}})
	if single.Outcome.Status != tool.OutcomeSucceeded || calls.Load() != 1 {
		t.Fatalf("one observable recipient should auto-send: result=%+v calls=%d", single, calls.Load())
	}

	mass := runtime.Execute(ctx, convcontext.ToolCall{ID: "mass", Function: convcontext.FunctionCall{
		Name: toolName, Arguments: `{"recipients":[{"id":"a@example.com","kind":"individual"},{"id":"b@example.com","kind":"individual"}],"body":"hello"}`,
	}})
	if mass.Outcome.Status != tool.OutcomeReviewRequired || mass.ErrorCode != "needs_mass_message_review" || calls.Load() != 1 {
		t.Fatalf("multiple recipients must stop for review: result=%+v calls=%d", mass, calls.Load())
	}

	approvalSink := &approvingMassReviewSink{}
	approvedRuntime := agentcore.NewToolRuntimeWithConfig(approvalSink, agentcore.ToolRuntimeConfig{RunID: "run_mass_approved"})
	approved := approvedRuntime.Execute(ctx, convcontext.ToolCall{ID: "mass-approved", Function: convcontext.FunctionCall{
		Name: toolName, Arguments: `{"recipients":[{"id":"a@example.com","kind":"individual"},{"id":"b@example.com","kind":"individual"}],"body":"hello"}`,
	}})
	if approved.Outcome.Status != tool.OutcomeSucceeded || calls.Load() != 2 {
		t.Fatalf("durably approved runtime path must call provider exactly once: result=%+v calls=%d", approved, calls.Load())
	}
	if approvalSink.request.ReviewEnvelopeDigest == "" || approvalSink.request.ReviewBodyDigest == "" || approvalSink.request.ReviewRecipientIdentityDigest == "" {
		t.Fatalf("review request lost canonical binding: %+v", approvalSink.request)
	}

	unknown := runtime.Execute(ctx, convcontext.ToolCall{ID: "unknown", Function: convcontext.FunctionCall{
		Name: toolName, Arguments: `{"recipients":[{"id":"one@example.com","kind":"mystery"}],"body":"hello"}`,
	}})
	if unknown.Outcome.Status != tool.OutcomeDenied || calls.Load() != 2 {
		t.Fatalf("unknown recipient semantics must fail closed: result=%+v calls=%d", unknown, calls.Load())
	}
}
