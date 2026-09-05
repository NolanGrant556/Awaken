package mcpclient

import (
	"bytes"
	"encoding/json"
	"regexp"

	"awaken/src/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Optional Awaken extension, not standard MCP error semantics. Never infer it
// from free text or a provider's arbitrary business payload.
const toolErrorMetadataKey = "io.awaken.tool/error/v1"

var providerErrorCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type mcpErrorMetadata struct {
	Code            string `json:"code"`
	Category        string `json:"category"`
	Stage           string `json:"stage"`
	SideEffectState string `json:"side_effect_state"`
	Retryable       *bool  `json:"retryable"`
	RetryOwner      string `json:"retry_owner"`
	RetryExhausted  *bool  `json:"retry_exhausted"`
}

func (a *McpToolAdapter) applyErrorMetadata(outcome *tool.ExecutionOutcome, result *mcp.CallToolResult) {
	raw, exists := result.Meta[toolErrorMetadataKey]
	if !exists {
		return
	}
	invalid := func() {
		outcome.Warnings = append(outcome.Warnings, tool.OutcomeNotice{Code: "mcp_error_metadata_ignored", Message: "Unsupported or contradictory MCP error metadata; conservative failure semantics retained."})
	}
	data, err := json.Marshal(raw)
	if err != nil || len(data) > 2048 {
		invalid()
		return
	}
	var meta mcpErrorMetadata
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&meta) != nil || !providerErrorCode.MatchString(meta.Code) || meta.Stage != "read" || meta.SideEffectState != "none" || meta.Retryable == nil || meta.RetryExhausted == nil || (meta.RetryOwner != "host" && meta.RetryOwner != "provider") {
		invalid()
		return
	}
	// Read annotations are provider hints, not proof of external state. Only a
	// consistent read-only tool can opt into this narrowly scoped extension.
	if !a.errorReadOnly || !a.readOnly || a.destructive || a.artifactOutputEnabled {
		invalid()
		return
	}
	category := tool.ErrorCategoryDomain
	switch meta.Category {
	case "transport":
		category = tool.ErrorCategoryRuntime
	case "validation":
		category = tool.ErrorCategoryValidation
	case "domain":
	case "cancelled":
		category = tool.ErrorCategoryCancelled
	default:
		invalid()
		return
	}
	if *meta.Retryable && (meta.RetryOwner != "host" || *meta.RetryExhausted || meta.Category != "transport") {
		invalid()
		return
	}
	outcome.ErrorCode = "mcp_provider_" + meta.Code // Cannot impersonate host control codes.
	outcome.ErrorCategory = category
	outcome.Retryable = *meta.Retryable
	outcome.Operation = &tool.OperationDescriptor{Type: a.Name(), SideEffectState: tool.OutcomeSideEffectNone}
	outcome.Diagnostic["provider_error"] = meta
	strategy := tool.RetryDoNotRetry
	outcome.RecoveryOptions = []string{"stop_and_report_provider_error"}
	if outcome.Retryable {
		strategy = tool.RetryAfterBackoff
		outcome.RecoveryOptions = []string{"retry_read_after_backoff_within_host_budget"}
	} else if meta.RetryOwner == "provider" && *meta.RetryExhausted {
		outcome.RecoveryOptions = []string{"stop_and_report_exhausted_provider_read", "do_not_change_irrelevant_arguments_to_retry"}
	}
	outcome.Errors = []tool.OutcomeError{{Type: outcome.ErrorCode, Category: category, Stage: tool.ErrorStageExecution, Cause: outcome.ErrorMessage, RetryStrategy: strategy, PossibleFixes: outcome.RecoveryOptions}}
}
