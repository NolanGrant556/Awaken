package mcpclient

import (
	"strings"
	"testing"

	"awaken/src/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mixedReadTool(meta any) *mcp.Tool {
	return &mcp.Tool{
		Name:        "records",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"action": map[string]any{"type": "string"}}},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: tool.BoolPtr(true)},
		Meta:        mcp.Meta{readOperationsMetadataKey: meta},
	}
}

func TestMCPReadOperationsExactProviderDeclaration(t *testing.T) {
	a := NewMcpToolAdapter(nil, "mixed", mixedReadTool(map[string]any{"parameter": "action", "values": []string{"query"}}))
	if a.IsReadOnly() || a.Definition().Annotations != NewMcpToolAdapter(nil, "mixed", mixedReadTool(nil)).Definition().Annotations {
		t.Fatal("per-call hint weakened the whole tool definition")
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		read bool
	}{
		{"query", map[string]any{"action": "query"}, true},
		{"execute", map[string]any{"action": "execute"}, false},
		{"resume", map[string]any{"action": "resume"}, false},
		{"prepare persists local plan", map[string]any{"action": "prepare"}, false},
		{"unknown", map[string]any{"action": "inspect"}, false},
		{"missing", nil, false},
		{"null", map[string]any{"action": nil}, false},
		{"number", map[string]any{"action": 1}, false},
		{"array", map[string]any{"action": []string{"query", "execute"}}, false},
		{"nested", map[string]any{"input": map[string]any{"action": "query"}}, false},
		{"case", map[string]any{"action": "QUERY"}, false},
		{"whitespace", map[string]any{"action": " query "}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &mcp.CallToolResult{StructuredContent: map[string]any{"readOnly": true, "side_effect_state": "none"}}
			outcome := a.callResultOutcomeForArguments(result, tc.args).Normalize(tool.OutcomeNormalizationContext{OperationType: a.Name(), DeclaredSideEffects: tool.ResolveDefinition(a).SideEffects})
			if (outcome.Operation.SideEffectState == tool.OutcomeSideEffectNone) != tc.read {
				t.Fatalf("incorrect per-call side effect: %+v", outcome)
			}
		})
	}
	// Host publication is a write even when the provider operation reads data.
	a.artifactOutputEnabled = true
	if a.declaredReadOperation(map[string]any{"action": "query"}) {
		t.Fatal("artifact bridge downgraded")
	}
}

func TestMCPReadOperationsMalformedAndResultOnlyMetadataDoNotDowngrade(t *testing.T) {
	for _, meta := range []any{
		nil, "query", []string{"query"},
		map[string]any{"parameter": "action", "values": []string{}},
		map[string]any{"parameter": "missing", "values": []string{"query"}},
		map[string]any{"parameter": "action", "values": []string{"query", "query"}},
		map[string]any{"parameter": "action", "values": []string{" query "}},
		map[string]any{"parameter": "action", "values": []any{true}},
		map[string]any{"parameter": "action", "values": []string{"query"}, "default": "query"},
		map[string]any{"parameter": "action", "values": []string{strings.Repeat("x", 2100)}},
	} {
		a := NewMcpToolAdapter(nil, "mixed", mixedReadTool(meta))
		result := &mcp.CallToolResult{Meta: mcp.Meta{readOperationsMetadataKey: map[string]any{"parameter": "action", "values": []string{"query"}}}}
		outcome := a.callResultOutcomeForArguments(result, map[string]any{"action": "query"}).Normalize(tool.OutcomeNormalizationContext{OperationType: a.Name(), DeclaredSideEffects: tool.ResolveDefinition(a).SideEffects})
		if outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown || len(outcome.Warnings) == 0 {
			t.Fatalf("malformed metadata downgraded: %#v: %+v", meta, outcome)
		}
	}
	definition := mixedReadTool(nil)
	definition.Meta = nil
	a := NewMcpToolAdapter(nil, "mixed", definition)
	if a.declaredReadOperation(map[string]any{"action": "query"}) {
		t.Fatal("query action inferred without provider declaration")
	}
}

func TestMCPReadOperationsErrorMetadataKeepsExistingRetryBudget(t *testing.T) {
	a := NewMcpToolAdapter(nil, "mixed", mixedReadTool(map[string]any{"parameter": "action", "values": []string{"query"}}))
	result := &mcp.CallToolResult{IsError: true, Meta: mcp.Meta{toolErrorMetadataKey: readErrorMeta("provider", false, true)}}
	read := a.callResultOutcomeForArguments(result, map[string]any{"action": "query"})
	if read.ErrorCode != "mcp_provider_read_timeout" || read.Retryable || read.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("read error: %+v", read)
	}
	write := a.callResultOutcomeForArguments(result, map[string]any{"action": "execute"})
	if write.ErrorCode != errorCodeMCPReportedError || write.Retryable || write.Operation != nil {
		t.Fatalf("write error downgraded: %+v", write)
	}
	nilResult := a.callResultOutcomeForArguments(nil, map[string]any{"action": "query"})
	if nilResult.Operation != nil {
		t.Fatal("empty transport result promoted to a confirmed read")
	}
}
