package mcpclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var _ tool.InputSchemaProvider = (*McpToolAdapter)(nil)
var _ tool.OutcomeTool = (*McpToolAdapter)(nil)
var _ tool.PermissionCheckingTool = (*McpToolAdapter)(nil)
var _ tool.ProviderManifestProvider = (*McpToolAdapter)(nil)
var _ tool.ProviderReadinessProvider = (*McpToolAdapter)(nil)

func TestMcpToolAdapterDoesNotTrustOfficialLookingServerName(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "github", &mcp.Tool{
		Name: "search",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(true),
		},
	})
	if adapter.trustedOrigin != tool.TrustedOriginThirdPartyMCP {
		t.Fatalf("user-named github MCP was elevated to %q", adapter.trustedOrigin)
	}
	if adapter.fileBridgeScope != tool.FileBridgeNone {
		t.Fatalf("spoofed official name received file bridge scope %q", adapter.fileBridgeScope)
	}

	mgr := NewManager()
	mgr.servers["host-connector"] = &managedServer{
		name: "host-connector", state: StateConnected, sourceConnectorID: "connector/github",
	}
	hostAdapter := NewMcpToolAdapter(mgr, "host-connector", &mcp.Tool{
		Name: "search",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true),
		},
	})
	if hostAdapter.trustedOrigin != tool.TrustedOriginOfficialConnector {
		t.Fatalf("host connector provenance was not retained: %q", hostAdapter.trustedOrigin)
	}
}

func TestMcpToolAdapterCanonicalizesDottedPluginServerNames(t *testing.T) {
	serverName := "plugin__awaken.acceptance.20260827__local-smoke"
	adapter := NewMcpToolAdapter(nil, serverName, &mcp.Tool{Name: "echo"})
	if got, want := adapter.Name(), "mcp__plugin__awaken_acceptance_20260827__local-smoke__echo"; got != want {
		t.Fatalf("adapter name = %q, want %q", got, want)
	}
	if strings.Contains(adapter.Name(), ".") {
		t.Fatalf("canonical MCP tool name must not contain dots: %q", adapter.Name())
	}
	if err := tool.RegisterToolForce(adapter); err != nil {
		t.Fatalf("register canonical MCP tool: %v", err)
	}
	t.Cleanup(func() { tool.UnregisterTool(adapter.Name()) })
	if _, err := tool.GetTool(adapter.Name()); err != nil {
		t.Fatalf("registry lookup failed for canonical MCP tool name: %v", err)
	}
	definition := adapter.Definition()
	if definition.Annotations.ServerName != serverName || definition.Annotations.CanonicalServerName != "plugin__awaken_acceptance_20260827__local-smoke" || definition.Annotations.ToolName != "echo" {
		t.Fatalf("raw MCP identity was not preserved in annotations: %+v", definition.Annotations)
	}
}

func TestMcpToolNameEncodesUnsafeComponentsReversibly(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "dot", raw: "read.item"},
		{name: "unicode", raw: "读取"},
		{name: "reserved sentinel", raw: "x0_echo"},
		{name: "space", raw: "read item"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded := encodeMCPNameComponent(test.raw)
			if !strings.HasPrefix(encoded, mcpEncodedSentinel) {
				t.Fatalf("encoded component %q does not carry sentinel", encoded)
			}
			if strings.ContainsAny(encoded, ". ") {
				t.Fatalf("encoded component is not model-safe: %q", encoded)
			}
			decoded, ok := decodeMCPNameComponent(encoded)
			if !ok || decoded != test.raw {
				t.Fatalf("decode(%q) = %q, %v; want %q, true", encoded, decoded, ok, test.raw)
			}
		})
	}

	if got := encodeMCPNameComponent("read_item"); got != "read_item" {
		t.Fatalf("safe component changed: %q", got)
	}
	if encodeMCPNameComponent("x0_echo") == encodeMCPNameComponent("echo") {
		t.Fatal("reserved-sentinel raw name collided with ordinary raw name")
	}
	if got := modelFacingMCPServerName("plugin__acme.tools__server.v1"); !strings.HasPrefix(got, "plugin__acme_tools__x0_") {
		t.Fatalf("plugin/server component mapping is wrong: %q", got)
	}
	if dotted, invalidUnderscore := modelFacingMCPServerName("plugin__acme.tools__server"), modelFacingMCPServerName("plugin__acme_tools__server"); dotted == invalidUnderscore {
		t.Fatalf("valid dotted plugin name collided with out-of-spec underscore name: %q", dotted)
	}

	serverName := "plugin__acme.tools__server.v1"
	for _, rawToolName := range []string{"read.item", "x0_echo"} {
		t.Run("register "+rawToolName, func(t *testing.T) {
			adapter := NewMcpToolAdapter(nil, serverName, &mcp.Tool{Name: rawToolName})
			if err := tool.RegisterToolForce(adapter); err != nil {
				t.Fatalf("register encoded raw MCP tool %q as %q: %v", rawToolName, adapter.Name(), err)
			}
			t.Cleanup(func() { tool.UnregisterTool(adapter.Name()) })
			if definition := adapter.Definition(); definition.Annotations.ServerName != serverName || definition.Annotations.ToolName != rawToolName {
				t.Fatalf("raw identity for %q was not preserved: %+v", rawToolName, definition.Annotations)
			}
		})
	}
}

func TestMcpToolAdapterPreservesOutputSchemaAsDetailsContract(t *testing.T) {
	additionalProperties := false
	adapter := NewMcpToolAdapter(nil, "schema-server", &mcp.Tool{
		Name: "update_record",
		OutputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": additionalProperties,
			"required":             []string{"record_id", "updated"},
			"properties": map[string]any{
				"record_id": map[string]any{"type": "string", "description": "updated record"},
				"updated":   map[string]any{"type": "boolean", "description": "whether an update occurred"},
			},
		},
	})

	if len(adapter.outputSchemaJSON) == 0 {
		t.Fatal("expected constructor to retain MCP output schema JSON")
	}
	def := tool.ResolveDefinition(adapter)
	if def.OutputSchema.Type != "object" || def.OutputSchema.AdditionalProperties == nil || *def.OutputSchema.AdditionalProperties {
		t.Fatalf("unexpected output schema root: %+v", def.OutputSchema)
	}
	if def.OutputSchema.Properties["record_id"].Type != "string" || def.OutputSchema.Properties["updated"].Type != "boolean" {
		t.Fatalf("expected MCP output properties in details contract: %+v", def.OutputSchema)
	}
	if resolution := tool.ResolveDefinitionReport(adapter); resolution.Quality == tool.DefinitionQualityStrict {
		t.Fatalf("external MCP definition must remain compatibility graded: %+v", resolution)
	}
}

func TestMcpToolAdapterTreatsMissingOutputSchemaAsUntypedDetails(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "schema-server", &mcp.Tool{Name: "legacy_structured"})
	def := tool.ResolveDefinition(adapter)
	if def.OutputSchema.Type != "object" || def.OutputSchema.AdditionalProperties == nil || !*def.OutputSchema.AdditionalProperties {
		t.Fatalf("missing MCP output schema must accept untyped structured details: %+v", def.OutputSchema)
	}
	outcome := adapter.callResultOutcome(&mcp.CallToolResult{
		StructuredContent: map[string]any{"ok": true, "count": float64(1)},
	})
	outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: adapter.Name(),
		OutputSchema:  def.OutputSchema,
	})
	for _, warning := range outcome.Warnings {
		if warning.Code == "tool_output_schema_mismatch" {
			t.Fatalf("untyped MCP structured output must not produce a false schema warning: %+v", outcome)
		}
	}
}

func TestMcpToolAdapterPreservesNullableOutputTypesWithoutFalseMismatch(t *testing.T) {
	for _, test := range []struct {
		name       string
		schema     map[string]any
		structured map[string]any
		rootField  string
	}{
		{
			name: "ordinary nullable array",
			schema: map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"messages", "hasMore"},
				"properties": map[string]any{
					"messages": map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "object", "additionalProperties": true}},
					"hasMore":  map[string]any{"type": "boolean"},
				},
			},
			structured: map[string]any{"messages": []any{}, "hasMore": false},
			rootField:  "messages",
		},
		{
			name: "nullable array with canonical field conflict",
			schema: map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"matches", "truncated"},
				"properties": map[string]any{
					"matches":   map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "string"}},
					"truncated": map[string]any{"type": "boolean"},
				},
			},
			structured: map[string]any{"matches": []any{"sentitems"}, "truncated": false},
			rootField:  "mcp_payload",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := NewMcpToolAdapter(nil, "schema-server", &mcp.Tool{Name: "nullable", OutputSchema: test.schema})
			definition := adapter.Definition()
			if _, ok := definition.OutputSchema.Properties[test.rootField]; !ok {
				t.Fatalf("normalized schema lost %q: %+v warnings=%v", test.rootField, definition.OutputSchema, adapter.OutputSchemaWarnings())
			}
			outcome := adapter.callResultOutcome(&mcp.CallToolResult{StructuredContent: test.structured})
			normalized := outcome.Normalize(tool.OutcomeNormalizationContext{
				OperationType: adapter.Name(), OutputSchema: definition.OutputSchema,
			})
			for _, warning := range normalized.Warnings {
				if warning.Code == "tool_output_schema_mismatch" {
					t.Fatalf("nullable output produced false schema warning: %+v", normalized)
				}
			}
		})
	}
}

func TestMcpToolAdapterWrapsCanonicalOutputSchemaConflicts(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "schema-server", &mcp.Tool{
		Name: "status_probe",
		OutputSchema: map[string]any{
			"type":     "object",
			"required": []string{"status", "record_id"},
			"properties": map[string]any{
				"status":    map[string]any{"type": "string"},
				"record_id": map[string]any{"type": "string"},
			},
		},
	})

	def := adapter.Definition()
	payload, ok := def.OutputSchema.Properties["mcp_payload"]
	if !ok || payload.Properties["status"].Type != "string" || payload.Properties["record_id"].Type != "string" {
		t.Fatalf("expected complete schema nested under mcp_payload: %+v", def.OutputSchema)
	}
	if len(adapter.OutputSchemaWarnings()) == 0 {
		t.Fatal("expected compatibility adaptation warning")
	}
	if err := tool.RegisterToolForce(adapter); err != nil {
		t.Fatalf("canonical field conflict must not break MCP compatibility registration: %v", err)
	}
	defer tool.UnregisterTool(adapter.Name())
	registered, err := tool.GetRegisteredTool(adapter.Name())
	if err != nil {
		t.Fatalf("read registered MCP tool: %v", err)
	}
	if registered.Quality == tool.DefinitionQualityStrict {
		t.Fatalf("conflict-adapted MCP schema must not be strict: %+v", registered)
	}

	outcome := adapter.callResultOutcome(&mcp.CallToolResult{
		StructuredContent: map[string]any{"status": "ok", "record_id": "42"},
	})
	wrapped, ok := outcome.Details["mcp_payload"].(map[string]any)
	if !ok || wrapped["status"] != "ok" || wrapped["record_id"] != "42" {
		t.Fatalf("expected result shape to match wrapped schema: %+v", outcome.Details)
	}
}

func TestMcpToolAdapterPreservesRecursiveInputSchema(t *testing.T) {
	additionalProperties := false
	mcpSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": additionalProperties,
		"required":             []string{"filter"},
		"properties": map[string]any{
			"filter": map[string]any{
				"type":        "object",
				"description": "structured query",
				"required":    []string{"terms"},
				"properties": map[string]any{
					"terms": map[string]any{
						"type":     "array",
						"minItems": 1,
						"items": map[string]any{
							"type": "string",
							"enum": []any{"open", "closed"},
						},
					},
				},
			},
		},
	}

	adapter := NewMcpToolAdapter(nil, "schema-server", &mcp.Tool{
		Name:        "search",
		Description: "search records",
		InputSchema: mcpSchema,
	})

	if len(adapter.inputSchemaJSON) == 0 {
		t.Fatal("expected constructor to retain MCP input schema JSON")
	}
	resolved := tool.ResolveInputSchema(adapter)
	if resolved.Type != "object" || resolved.AdditionalProperties == nil || *resolved.AdditionalProperties {
		t.Fatalf("unexpected root schema: %+v", resolved)
	}
	filter, ok := resolved.Properties["filter"]
	if !ok || filter.Type != "object" {
		t.Fatalf("expected recursive filter object, got %+v", filter)
	}
	terms, ok := filter.Properties["terms"]
	if !ok || terms.Type != "array" || terms.Items == nil {
		t.Fatalf("expected recursive terms array, got %+v", terms)
	}
	if terms.MinItems == nil || *terms.MinItems != 1 {
		t.Fatalf("expected minItems=1, got %+v", terms.MinItems)
	}
	if terms.Items.Type != "string" || len(terms.Items.Enum) != 2 {
		t.Fatalf("expected string enum item schema, got %+v", terms.Items)
	}
}

func TestMcpToolAdapterPreservesNullableInputTypes(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "schema-server", &mcp.Tool{
		Name: "create_draft",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"subject", "to"},
			"properties": map[string]any{
				"subject": map[string]any{"type": "string"},
				"to": map[string]any{
					"type":  []any{"null", "array"},
					"items": map[string]any{"type": "string"},
				},
			},
		},
	})

	resolved := tool.ResolveInputSchema(adapter)
	to, ok := resolved.Properties["to"]
	if !ok || len(to.OneOf) != 2 {
		t.Fatalf("nullable input property was lost: %+v", resolved)
	}
	var arrayOption *tool.ToolParam
	for index := range to.OneOf {
		if to.OneOf[index].Type == "array" {
			arrayOption = &to.OneOf[index]
		}
	}
	if arrayOption == nil || arrayOption.Items == nil || arrayOption.Items.Type != "string" {
		t.Fatalf("nullable string array was not preserved: %+v", to)
	}
	parameters := adapter.Parameters()
	var toParameter *tool.ToolParameter
	for index := range parameters {
		if parameters[index].Name == "to" {
			toParameter = &parameters[index]
		}
	}
	if toParameter == nil || toParameter.Type != "array" {
		t.Fatalf("legacy parameter fallback did not retain the array type: %+v", parameters)
	}
}

func TestMcpToolAdapterCallResultOutcomeCarriesProtocolTruth(t *testing.T) {
	adapter := NewMcpToolAdapter(nil, "result-server", &mcp.Tool{
		Name: "probe",
		OutputSchema: map[string]any{
			"type":     "object",
			"required": []string{"record_id", "updated"},
			"properties": map[string]any{
				"record_id": map[string]any{"type": "string"},
				"updated":   map[string]any{"type": "boolean"},
			},
		},
	})

	t.Run("success", func(t *testing.T) {
		outcome := adapter.callResultOutcome(&mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "updated record 42"}},
			StructuredContent: map[string]any{"record_id": "42", "updated": true},
		})
		if outcome.Status != tool.OutcomeSucceeded || outcome.IsError() {
			t.Fatalf("expected canonical success, got %+v", outcome)
		}
		if outcome.TransportStatus != tool.TransportCompleted || outcome.SemanticStatus != tool.SemanticSuccess {
			t.Fatalf("unexpected status dimensions: %+v", outcome)
		}
		if got, ok := outcome.Diagnostic["mcp_is_error"].(bool); !ok || got {
			t.Fatalf("expected diagnostic mcp_is_error=false, got %#v", outcome.Diagnostic["mcp_is_error"])
		}
		if outcome.Details["record_id"] != "42" || outcome.Details["updated"] != true {
			t.Fatalf("expected structured MCP result in details, got %#v", outcome.Details)
		}
		if _, duplicated := outcome.Diagnostic["structured_content"]; duplicated {
			t.Fatalf("structured success payload must not be duplicated in diagnostics: %#v", outcome.Diagnostic)
		}
	})

	t.Run("server reported error", func(t *testing.T) {
		outcome := adapter.callResultOutcome(&mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "record 42 is locked"}},
			IsError: true,
		})
		if outcome.Status != tool.OutcomeFailed || !outcome.IsError() {
			t.Fatalf("expected canonical failure, got %+v", outcome)
		}
		if outcome.TransportStatus != tool.TransportCompleted {
			t.Fatalf("MCP business failure must retain completed transport, got %s", outcome.TransportStatus)
		}
		if outcome.ErrorCode != errorCodeMCPReportedError || outcome.ErrorCategory != tool.ErrorCategoryDomain {
			t.Fatalf("unexpected error classification: %+v", outcome)
		}
		if outcome.Retryable {
			t.Fatal("server-reported error must not request a blind retry")
		}
		if strings.Contains(outcome.Content, "[tool error]") {
			t.Fatalf("semantic status must not depend on text prefix: %q", outcome.Content)
		}
		if got, ok := outcome.Diagnostic["mcp_is_error"].(bool); !ok || !got {
			t.Fatalf("expected diagnostic mcp_is_error=true, got %#v", outcome.Diagnostic["mcp_is_error"])
		}
	})
}

type adapterProbeInput struct {
	Mode string `json:"mode"`
}

type adapterProbeOutput struct {
	Accepted bool `json:"accepted"`
}

func TestMcpToolAdapterExecuteOutcomeAndLegacyWrapper(t *testing.T) {
	const serverName = "adapter-outcome"
	registry := NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, &mcp.Tool{
			Name:        "probe",
			Description: "returns success or a server-reported business failure",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			},
		}, func(_ context.Context, _ *mcp.CallToolRequest, input adapterProbeInput) (*mcp.CallToolResult, adapterProbeOutput, error) {
			if input.Mode == "fail" {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "probe rejected the request"}},
					IsError: true,
				}, adapterProbeOutput{}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "probe accepted the request"}},
			}, adapterProbeOutput{Accepted: true}, nil
		})
		return server
	}); err != nil {
		t.Fatalf("register in-process MCP server: %v", err)
	}

	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mgr.Start(ctx, []ScopedMcpServerConfig{{
		Name:  serverName,
		Scope: ScopeProject,
		Config: McpServerConfig{
			InProc: &InProcTransportConfig{ServerName: serverName},
		},
	}})
	defer mgr.Shutdown(context.Background())
	defer UnregisterMcpToolsForTest(serverName)
	if err := WaitForServerStateForTest(mgr, serverName, StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for in-process MCP server: %v", err)
	}

	adapter := NewMcpToolAdapter(mgr, serverName, &mcp.Tool{
		Name: "probe",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(false),
		},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"mode": map[string]any{"type": "string"}},
		},
		OutputSchema: map[string]any{
			"type":       "object",
			"required":   []string{"accepted"},
			"properties": map[string]any{"accepted": map[string]any{"type": "boolean"}},
		},
	})

	success, err := adapter.ExecuteOutcome(ctx, `{"mode":"success"}`, nil)
	if err != nil {
		t.Fatalf("ExecuteOutcome success returned transport error: %v", err)
	}
	if success.Status != tool.OutcomeSucceeded || success.IsError() {
		t.Fatalf("expected successful outcome, got %+v", success)
	}
	if success.Details["accepted"] != true {
		t.Fatalf("ExecuteOutcome must project MCP StructuredContent into schema-backed details: %+v", success)
	}
	if _, duplicated := success.Diagnostic["structured_content"]; duplicated {
		t.Fatalf("successful structured output must not be duplicated in diagnostics: %+v", success.Diagnostic)
	}

	failure, err := adapter.ExecuteOutcome(ctx, `{"mode":"fail"}`, nil)
	if err != nil {
		t.Fatalf("MCP IsError must not become a protocol error: %v", err)
	}
	if failure.Status != tool.OutcomeFailed || failure.ErrorCode != errorCodeMCPReportedError {
		t.Fatalf("expected typed MCP failure, got %+v", failure)
	}

	legacyContent, legacyErr := adapter.Execute(ctx, `{"mode":"fail"}`)
	if legacyErr == nil {
		t.Fatal("legacy Execute must expose MCP semantic failure as an error")
	}
	var typed tool.ToolError
	if !errors.As(legacyErr, &typed) || typed.Code != errorCodeMCPReportedError {
		t.Fatalf("expected typed legacy ToolError, got %T: %v", legacyErr, legacyErr)
	}
	if !strings.Contains(legacyContent, "rejected") {
		t.Fatalf("expected server failure content, got %q", legacyContent)
	}
}
