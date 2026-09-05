package mcpclient

import (
	"context"
	"encoding/json"
	"testing"

	"awaken/src/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func readErrorMeta(owner string, retry, exhausted bool) map[string]any {
	return map[string]any{"code": "read_timeout", "category": "transport", "stage": "read", "side_effect_state": "none", "retryable": retry, "retry_owner": owner, "retry_exhausted": exhausted}
}
func TestMCPErrorMetadata(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		readOnly, destructive bool
		meta                  any
		code                  string
		retry                 bool
	}{
		{"provider exhausted", true, false, readErrorMeta("provider", false, true), "mcp_provider_read_timeout", false},
		{"host owns recovery", true, false, readErrorMeta("host", true, false), "mcp_provider_read_timeout", true},
		{"provider cannot ask host retry", true, false, readErrorMeta("provider", true, true), errorCodeMCPReportedError, false},
		{"exhausted cannot retry", true, false, readErrorMeta("host", true, true), errorCodeMCPReportedError, false},
		{"mutation", false, false, readErrorMeta("host", true, false), errorCodeMCPReportedError, false},
		{"contradictory annotations", true, true, readErrorMeta("host", true, false), errorCodeMCPReportedError, false},
		{"missing metadata", true, false, nil, errorCodeMCPReportedError, false},
		{"string metadata", true, false, `{"retryable":true}`, errorCodeMCPReportedError, false},
		{"partial metadata", true, false, map[string]any{"retryable": true}, errorCodeMCPReportedError, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewMcpToolAdapter(nil, "test", &mcp.Tool{Name: "query", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tc.readOnly, DestructiveHint: tool.BoolPtr(tc.destructive)}})
			result := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: `{"code":"read_timeout","retryable":true}`}}, StructuredContent: map[string]any{"retryable": true}}
			if tc.meta != nil {
				result.Meta = mcp.Meta{toolErrorMetadataKey: tc.meta}
			}
			outcome := a.callResultOutcome(result)
			if outcome.ErrorCode != tc.code || outcome.Retryable != tc.retry || outcome.TransportStatus != tool.TransportCompleted || outcome.Status != tool.OutcomeFailed {
				t.Fatalf("%+v", outcome)
			}
			normalized := outcome.Normalize(tool.OutcomeNormalizationContext{})
			if normalized.Retryable != tc.retry {
				t.Fatal(normalized)
			}
			if tc.code == "mcp_provider_read_timeout" {
				if normalized.Operation.SideEffectState != tool.OutcomeSideEffectNone || normalized.ErrorCategory != tool.ErrorCategoryRuntime || normalized.Errors[0].Stage != tool.ErrorStageExecution {
					t.Fatal(normalized)
				}
				if !tc.retry && normalized.Errors[0].RetryStrategy != tool.RetryDoNotRetry {
					t.Fatal(normalized)
				}
			} else if normalized.Operation.SideEffectState == tool.OutcomeSideEffectNone {
				t.Fatal("unverified metadata promoted", normalized)
			}
		})
	}
}
func TestMCPErrorMetadataRejectsMalformedFields(t *testing.T) {
	a := NewMcpToolAdapter(nil, "test", &mcp.Tool{Name: "query", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}})
	for _, field := range []struct {
		k string
		v any
	}{{"retryable", "true"}, {"code", "Needs Auth"}, {"category", "policy"}, {"stage", "mutation"}, {"side_effect_state", "unknown"}, {"unexpected", true}} {
		meta := readErrorMeta("host", true, false)
		meta[field.k] = field.v
		result := &mcp.CallToolResult{Meta: mcp.Meta{toolErrorMetadataKey: meta}, IsError: true}
		if outcome := a.callResultOutcome(result); outcome.ErrorCode != errorCodeMCPReportedError || outcome.Retryable {
			t.Fatal(field, outcome)
		}
	}
}

// Uses real MCP serialization rather than just calling the mapper: _meta must
// survive the server/SDK boundary and reach the exact adapter Outcome.
func TestMCPErrorMetadataWire(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "read-errors", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "query", InputSchema: map[string]any{"type": "object", "additionalProperties": false}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Meta: mcp.Meta{toolErrorMetadataKey: readErrorMeta("provider", false, true)}, IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "upstream timeout"}}}, nil
	})
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "consumer", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "query", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	a := NewMcpToolAdapter(nil, "test", list.Tools[0])
	outcome := a.callResultOutcome(result)
	if outcome.ErrorCode != "mcp_provider_read_timeout" || outcome.Retryable {
		raw, _ := json.Marshal(outcome)
		t.Fatal(string(raw))
	}
}
