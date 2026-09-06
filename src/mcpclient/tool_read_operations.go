package mcpclient

import (
	"bytes"
	"encoding/json"
	"strings"

	"awaken/src/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Optional tools/list declaration for mixed read/write tools. This is an
// Outcome hint, never a permission grant or evidence of observed remote state.
const readOperationsMetadataKey = "io.awaken.tool/read-operations/v1"

type mcpReadOperations struct {
	Parameter string   `json:"parameter"`
	Values    []string `json:"values"`
}

func (a *McpToolAdapter) loadReadOperations(definition *mcp.Tool) {
	raw, exists := definition.Meta[readOperationsMetadataKey]
	if !exists {
		return
	}
	invalid := func() {
		a.readOperationsWarning = "Unsupported MCP read-operations metadata; static tool semantics retained."
	}
	data, err := json.Marshal(raw)
	if err != nil || len(data) > 2048 {
		invalid()
		return
	}
	var meta mcpReadOperations
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&meta) != nil || strings.TrimSpace(meta.Parameter) != meta.Parameter || meta.Parameter == "" || len(meta.Values) == 0 || len(meta.Values) > 32 {
		invalid()
		return
	}
	// Require an explicit top-level string selector in the registered schema.
	// No dotted paths, coerced values, inferred defaults or name heuristics.
	parameter, found := a.InputSchema().Properties[meta.Parameter]
	if !found || parameter.Type != "string" {
		invalid()
		return
	}
	seen := map[string]bool{}
	for _, value := range meta.Values {
		if value == "" || len(value) > 128 || strings.TrimSpace(value) != value || seen[value] {
			invalid()
			return
		}
		seen[value] = true
	}
	a.readOperations = &meta
}

func (a *McpToolAdapter) declaredReadOperation(arguments map[string]any) bool {
	if a.readOperations == nil || a.artifactOutputEnabled {
		return false
	}
	value, ok := arguments[a.readOperations.Parameter].(string)
	if !ok {
		return false
	}
	for _, candidate := range a.readOperations.Values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (a *McpToolAdapter) callResultOutcomeForArguments(result *mcp.CallToolResult, arguments map[string]any) tool.ExecutionOutcome {
	readOperation := a.declaredReadOperation(arguments)
	readErrorEligible := !a.artifactOutputEnabled && (readOperation || (a.errorReadOnly && a.readOnly && !a.destructive))
	outcome := a.callResultOutcomeWithReadEligibility(result, readErrorEligible)
	if readOperation && result != nil {
		outcome.Operation = &tool.OperationDescriptor{Type: a.Name(), SideEffectState: tool.OutcomeSideEffectNone}
		outcome.Diagnostic["provider_read_operation"] = map[string]any{
			"parameter": a.readOperations.Parameter,
			"value":     arguments[a.readOperations.Parameter],
			"source":    readOperationsMetadataKey,
		}
	}
	if a.readOperationsWarning != "" {
		outcome.Warnings = append(outcome.Warnings, tool.OutcomeNotice{Code: "mcp_read_operations_metadata_ignored", Message: a.readOperationsWarning})
	}
	return outcome
}
