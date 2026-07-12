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

func messageProviderTool(name string) *mcp.Tool {
	return &mcp.Tool{
		Meta: providerMetadata(MCPToolProviderMetadata{
			ProviderID:   "connector:mail:send",
			Capabilities: []tool.Capability{tool.CapabilityMessageSend},
			MessageEgress: &MCPMessageEgressContract{
				Channel:             "email",
				SendingAccount:      "sender@example.com",
				RecipientsField:     "recipients",
				RecipientIDField:    "address",
				RecipientKindField:  "kind",
				RecipientScopeField: "recipient_scope",
				MessageCountField:   "message_count",
				ExpandsField:        "expands",
				BodyField:           "body",
				AttachmentsField:    "attachments",
			},
		}),
		Name: name,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recipients":      map[string]any{"type": "array"},
				"recipient_scope": map[string]any{"type": "string"},
				"message_count":   map[string]any{"type": "integer"},
				"expands":         map[string]any{"type": "boolean"},
				"body":            map[string]any{"type": "string"},
				"attachments":     map[string]any{"type": "array"},
			},
		},
	}
}

func TestMCPOrdinaryCallsDefaultToAutomaticAllow(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "third-party", &mcp.Tool{
		Name: "update_record",
		Meta: providerMetadata(MCPToolProviderMetadata{
			ProviderID: "mcp:third-party:update_record",
		}),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(true),
		},
	})
	decision := adapter.CheckPermission(context.Background(), `{}`, tool.PermissionContext{})
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("ordinary MCP update should auto-execute, got %+v", decision)
	}
}

func TestMCPReadOnlyCallWithoutProviderMetadataRemainsAutomatic(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "third-party", &mcp.Tool{
		Name: "search_records",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(true),
		},
	})
	decision := adapter.CheckPermission(context.Background(), `{}`, tool.PermissionContext{})
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("read-only MCP call should remain automatic without Awaken provider metadata, got %+v", decision)
	}
}

func TestMCPInvalidProviderMetadataDoesNotSatisfyMutationContract(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "third-party", &mcp.Tool{
		Name: "update_record",
		Meta: mcp.Meta{AwakenProviderMetaKey: map[string]any{
			"provider_id": 42,
		}},
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
	})
	decision := adapter.CheckPermission(context.Background(), `{}`, tool.PermissionContext{})
	if decision.Behavior != tool.PermissionUnsupported || decision.ErrorCode != ErrorCodeUnverifiedProviderMetadata {
		t.Fatalf("invalid provider metadata must not authorize a mutation, got %+v", decision)
	}
	if warnings := adapter.ProviderManifest().Warnings; len(warnings) == 0 {
		t.Fatal("invalid provider metadata should remain visible in provider diagnostics")
	}
}

type disguisedMutationInput struct {
	ID string `json:"id"`
}

type disguisedMutationOutput struct {
	Accepted bool `json:"accepted"`
}

func TestMCPDisguisedMutationWithoutProviderMetadataFailsClosedBeforeTransport(t *testing.T) {
	const serverName = "disguised-mutation-safety"
	var calls atomic.Int32
	ordinaryAnnotations := &mcp.ToolAnnotations{
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(true),
	}
	undeclared := &mcp.Tool{
		Name:        "sync_record",
		Annotations: ordinaryAnnotations,
	}
	declared := &mcp.Tool{
		Name:        "sync_record_declared",
		Annotations: ordinaryAnnotations,
		Meta: providerMetadata(MCPToolProviderMetadata{
			ProviderID: "mcp:disguised-mutation-safety:sync_record_declared",
		}),
	}

	registry := NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		handler := func(_ context.Context, _ *mcp.CallToolRequest, _ disguisedMutationInput) (*mcp.CallToolResult, disguisedMutationOutput, error) {
			calls.Add(1)
			return nil, disguisedMutationOutput{Accepted: true}, nil
		}
		mcp.AddTool(server, undeclared, handler)
		mcp.AddTool(server, declared, handler)
		return server
	}); err != nil {
		t.Fatalf("register in-process server: %v", err)
	}

	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{{
		Name:   serverName,
		Scope:  ScopeProject,
		Config: McpServerConfig{InProc: &InProcTransportConfig{ServerName: serverName}},
	}})
	defer mgr.Shutdown(context.Background())
	defer UnregisterMcpToolsForTest(serverName)
	if err := WaitForServerStateForTest(mgr, serverName, StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for server: %v", err)
	}

	hostile := NewMcpToolAdapter(mgr, serverName, undeclared)
	permission := hostile.CheckPermission(ctx, `{"id":"42"}`, tool.PermissionContext{})
	if permission.Behavior != tool.PermissionUnsupported || permission.ErrorCode != ErrorCodeUnverifiedProviderMetadata {
		t.Fatalf("metadata-less disguised mutation must be typed unsupported, got %+v", permission)
	}
	if calls.Load() != 0 {
		t.Fatalf("permission preflight reached hostile MCP transport (%d calls)", calls.Load())
	}
	outcome, err := hostile.ExecuteOutcome(ctx, `{"id":"42"}`, nil)
	if err != nil {
		t.Fatalf("metadata failure should be a structured outcome: %v", err)
	}
	if outcome.Status != tool.OutcomeUnsupported || outcome.ErrorCode != ErrorCodeUnverifiedProviderMetadata || outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("unexpected metadata-less mutation outcome: %+v", outcome)
	}
	if calls.Load() != 0 {
		t.Fatalf("metadata-less disguised mutation reached MCP transport (%d calls)", calls.Load())
	}

	ordinary := NewMcpToolAdapter(mgr, serverName, declared)
	if permission = ordinary.CheckPermission(ctx, `{"id":"42"}`, tool.PermissionContext{}); permission.Behavior != tool.PermissionAllow {
		t.Fatalf("valid explicitly declared ordinary mutation should auto-execute, got %+v", permission)
	}
	outcome, err = ordinary.ExecuteOutcome(ctx, `{"id":"42"}`, nil)
	if err != nil || outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("declared ordinary mutation should reach transport: outcome=%+v err=%v", outcome, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected only the explicitly declared mutation to reach transport, got %d calls", calls.Load())
	}
}

func TestMCPMessageEgressIsInputAware(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "mail", messageProviderTool("send_message"))

	t.Run("one observable individual is automatic", func(t *testing.T) {
		decision := adapter.CheckPermission(context.Background(), `{
			"recipients":[{"address":"person@example.com","kind":"individual"}],
			"body":"hello"
		}`, tool.PermissionContext{})
		if decision.Behavior != tool.PermissionAllow {
			t.Fatalf("single-recipient send should auto-execute, got %+v", decision)
		}
	})

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name: "multiple recipients",
			input: `{"recipients":[
				{"address":"a@example.com","kind":"individual"},
				{"address":"b@example.com","kind":"individual"}
			],"body":"hello"}`,
			want: 2,
		},
		{
			name:  "group recipient",
			input: `{"recipients":[{"address":"group-42","kind":"group"}],"body":"hello"}`,
			want:  1,
		},
		{
			name:  "provider expands recipient set",
			input: `{"recipients":[{"address":"person@example.com","kind":"individual"}],"expands":true,"body":"hello"}`,
			want:  1,
		},
		{
			name:  "multiple messages",
			input: `{"recipients":[{"address":"person@example.com","kind":"individual"}],"message_count":3,"body":"hello"}`,
			want:  1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := adapter.CheckPermission(context.Background(), test.input, tool.PermissionContext{})
			if decision.Behavior != tool.PermissionNeedsMassMessageReview {
				t.Fatalf("mass send should require its dedicated review, got %+v", decision)
			}
			metadata := decision.Metadata.MassMessage
			if metadata == nil || metadata.RecipientCount != test.want || metadata.Channel != "email" || metadata.SendingAccount != "sender@example.com" {
				t.Fatalf("review metadata lost provider envelope facts: %+v", metadata)
			}
			if metadata.EnvelopeDigest == "" || metadata.BodyDigest == "" || metadata.RecipientIdentityDigest == "" {
				t.Fatalf("review metadata lost canonical approval binding: %+v", metadata)
			}
		})
	}
}

func TestMCPMessageEgressFailsClosedWhenRecipientsAreUnknown(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "mail", messageProviderTool("send_message"))
	decision := adapter.CheckPermission(context.Background(), `{
		"recipients":[{"address":"person@example.com"}],
		"body":"hello"
	}`, tool.PermissionContext{})
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("unknown recipient kind must fail closed, got %+v", decision)
	}
	outcome, err := adapter.ExecuteOutcome(context.Background(), `{
		"recipients":[{"address":"person@example.com"}],
		"body":"hello"
	}`, nil)
	if err != nil {
		t.Fatalf("policy denial should be a structured outcome, got %v", err)
	}
	if outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != ErrorCodeUnobservableMessageRecipients || outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("unexpected unknown-recipient outcome: %+v", outcome)
	}
}

type safetyMessageRecipient struct {
	Address string `json:"address"`
	Kind    string `json:"kind"`
}

type safetyMessageInput struct {
	Recipients []safetyMessageRecipient `json:"recipients"`
	Body       string                   `json:"body"`
}

type safetyMessageOutput struct {
	Sent bool `json:"sent"`
}

func TestMCPMassMessageDirectCallsRequireOneShotExactApproval(t *testing.T) {
	const serverName = "mass-message-exact-approval"
	var calls atomic.Int32
	registry := NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, messageProviderTool("send_message"), func(_ context.Context, _ *mcp.CallToolRequest, _ safetyMessageInput) (*mcp.CallToolResult, safetyMessageOutput, error) {
			calls.Add(1)
			return nil, safetyMessageOutput{Sent: true}, nil
		})
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

	adapter := NewMcpToolAdapter(mgr, serverName, messageProviderTool("send_message"))
	original := `{"recipients":[{"address":"a@example.com","kind":"individual"},{"address":"b@example.com","kind":"individual"}],"body":"approved body"}`
	mutated := `{"recipients":[{"address":"a@example.com","kind":"individual"},{"address":"b@example.com","kind":"individual"}],"body":"mutated body"}`

	direct, err := adapter.ExecuteOutcome(ctx, original, nil)
	if err != nil || direct.Status != tool.OutcomeReviewRequired || direct.TransportStatus != tool.TransportNotStarted || calls.Load() != 0 {
		t.Fatalf("direct ExecuteOutcome bypassed review: outcome=%+v err=%v calls=%d", direct, err, calls.Load())
	}
	if _, err := adapter.Execute(ctx, original); err == nil || calls.Load() != 0 {
		t.Fatalf("legacy Execute bypassed review: err=%v calls=%d", err, calls.Load())
	}

	identityCtx := tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{
		RunID: "run_exact", ToolCallID: "call_exact", ToolName: adapter.Name(),
	})
	arguments, err := parseArguments(original, adapter.parameters)
	if err != nil {
		t.Fatalf("parse approved arguments: %v", err)
	}
	binding, err := adapter.massMessageApprovalBinding(identityCtx, adapter.evaluateSafety(identityCtx, arguments))
	if err != nil {
		t.Fatalf("build approval binding: %v", err)
	}
	mutatedCtx, revokeMutated, err := tool.IssueMassMessageApprovalGrant(identityCtx, binding)
	if err != nil {
		t.Fatalf("issue mutation grant: %v", err)
	}
	defer revokeMutated()
	mutationOutcome, err := adapter.ExecuteOutcome(mutatedCtx, mutated, nil)
	if err != nil || mutationOutcome.Status != tool.OutcomeReviewRequired || calls.Load() != 0 {
		t.Fatalf("mutated envelope consumed approval: outcome=%+v err=%v calls=%d", mutationOutcome, err, calls.Load())
	}
	retryOriginal, err := adapter.ExecuteOutcome(mutatedCtx, original, nil)
	if err != nil || retryOriginal.Status != tool.OutcomeReviewRequired || calls.Load() != 0 {
		t.Fatalf("mismatch did not burn one-shot grant: outcome=%+v err=%v calls=%d", retryOriginal, err, calls.Load())
	}

	approvedCtx, revokeApproved, err := tool.IssueMassMessageApprovalGrant(identityCtx, binding)
	if err != nil {
		t.Fatalf("issue exact grant: %v", err)
	}
	defer revokeApproved()
	approved, err := adapter.ExecuteOutcome(approvedCtx, original, nil)
	if err != nil || approved.Status != tool.OutcomeSucceeded || calls.Load() != 1 {
		t.Fatalf("exact approval did not call external provider once: outcome=%+v err=%v calls=%d", approved, err, calls.Load())
	}
	replay, err := adapter.ExecuteOutcome(approvedCtx, original, nil)
	if err != nil || replay.Status != tool.OutcomeReviewRequired || calls.Load() != 1 {
		t.Fatalf("approval replay reached external provider: outcome=%+v err=%v calls=%d", replay, err, calls.Load())
	}
}

type deleteSafetyInput struct {
	ID        string `json:"id"`
	Confirmed bool   `json:"confirmed"`
}

type deleteSafetyOutput struct {
	Recoverable bool   `json:"recoverable"`
	RestoreID   string `json:"restore_id,omitempty"`
}

func TestMCPRemoteDeleteRequiresAndVerifiesRecoverableResult(t *testing.T) {
	const serverName = "delete-safety"
	var calls atomic.Int32
	registry := NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "delete_record"}, func(_ context.Context, _ *mcp.CallToolRequest, input deleteSafetyInput) (*mcp.CallToolResult, deleteSafetyOutput, error) {
			calls.Add(1)
			output := deleteSafetyOutput{Recoverable: input.Confirmed, RestoreID: "trash-42"}
			if input.ID == "unverified" {
				output.RestoreID = ""
			}
			return nil, output, nil
		})
		return server
	}); err != nil {
		t.Fatalf("register in-process server: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{{
		Name:   serverName,
		Scope:  ScopeProject,
		Config: McpServerConfig{InProc: &InProcTransportConfig{ServerName: serverName}},
	}})
	defer mgr.Shutdown(context.Background())
	defer UnregisterMcpToolsForTest(serverName)
	if err := WaitForServerStateForTest(mgr, serverName, StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for server: %v", err)
	}

	base := &mcp.Tool{
		Name: "delete_record",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "string"}, "confirmed": map[string]any{"type": "boolean"}},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recoverable": map[string]any{"type": "boolean"},
				"restore_id":  map[string]any{"type": "string"},
			},
		},
	}
	base.Meta = providerMetadata(MCPToolProviderMetadata{Capabilities: []tool.Capability{tool.CapabilityFileDelete}})
	unsupported := NewMcpToolAdapter(mgr, serverName, base)
	permission := unsupported.CheckPermission(ctx, `{"id":"42"}`, tool.PermissionContext{})
	if permission.Behavior != tool.PermissionUnsupported || permission.ErrorCode != ErrorCodeUnsupportedRecoverableDelete {
		t.Fatalf("remote delete preflight lost its structured unsupported code: %+v", permission)
	}
	if calls.Load() != 0 {
		t.Fatalf("permission preflight reached external MCP server (%d calls)", calls.Load())
	}
	outcome, err := unsupported.ExecuteOutcome(ctx, `{"id":"42"}`, nil)
	if err != nil {
		t.Fatalf("unsupported delete should return structured outcome: %v", err)
	}
	if outcome.Status != tool.OutcomeUnsupported || outcome.ErrorCode != ErrorCodeUnsupportedRecoverableDelete || outcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("delete without recovery contract reached wrong outcome: %+v", outcome)
	}
	if calls.Load() != 0 {
		t.Fatalf("unsupported remote delete reached external MCP server (%d calls)", calls.Load())
	}

	base.Meta = providerMetadata(MCPToolProviderMetadata{
		Capabilities: []tool.Capability{tool.CapabilityFileDelete},
		RecoverableDelete: &MCPRecoverableDeleteContract{
			Recoverable:        true,
			ConfirmationField:  "recoverable",
			RestoreHandleField: "restore_id",
		},
	})
	supported := NewMcpToolAdapter(mgr, serverName, base)
	outcome, err = supported.ExecuteOutcome(ctx, `{"id":"42","confirmed":true}`, nil)
	if err != nil || outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("declared recoverable delete should execute: outcome=%+v err=%v", outcome, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one remote delete, got %d", calls.Load())
	}

	outcome, err = supported.ExecuteOutcome(ctx, `{"id":"unverified","confirmed":false}`, nil)
	if err != nil {
		t.Fatalf("unverified provider result should remain structured: %v", err)
	}
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != ErrorCodeRecoverableDeleteResultUnverified || outcome.TransportStatus != tool.TransportCompleted {
		t.Fatalf("provider contract violation was not surfaced: %+v", outcome)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected result verification after second remote call, got %d", calls.Load())
	}
}

func TestMCPProviderManifestIncludesIdentityCapabilitiesDependencyAndReadiness(t *testing.T) {
	mgr := NewManager()
	mgr.servers["mail"] = &managedServer{
		name:  "mail",
		state: StateNeedsAuth,
		config: McpServerConfig{Provider: &ServerProviderManifest{
			ProviderID:  "connector:mail",
			DisplayName: "Mail",
			Source:      tool.SourceConnector,
		}},
		errorReason: "login required",
	}
	adapter := NewMcpToolAdapter(mgr, "mail", messageProviderTool("send_message"))
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
