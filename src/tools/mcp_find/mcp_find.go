// Package mcpfind implements the builtin auxiliary tool that searches the
// deferred MCP tool catalog pinned in the current run snapshot and loads the
// matched full definitions on demand (progressive disclosure).
package mcpfind

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/tool"
)

// Tool searches deferred MCP tools by keyword or exact selection. It never
// executes MCP calls itself: results carry compact matched names, and the
// runner-owned activation observer adds the full definitions to the next
// model request's tool list.
type Tool struct{}

func (t *Tool) Name() string { return tool.MCPFindToolName }

func (t *Tool) Description() string {
	return "Search connected MCP tools whose full definitions are deferred (not loaded into context) and load the matched definitions on demand."
}

func (t *Tool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{
			Name:        "query",
			Type:        "string",
			Description: "Keyword query (e.g. \"github create issue\"), \"select:<tool1>,<tool2>\" for exact selection, or an exact tool name. Prefix a keyword with + to require it.",
			Required:    true,
		},
		{
			Name:        "limit",
			Type:        "integer",
			Description: "Maximum number of matched tools to return and load (default 5, max 20).",
			Required:    false,
		},
	}
}

func (t *Tool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        tool.MCPFindToolName,
		DisplayName: "MCP Find",
		Description: t.Description(),
		Purpose:     "Discover deferred MCP tools by keyword and load their full definitions into the model tool list without keeping the whole MCP catalog in context.",
		WhenToUse: []string{
			"An MCP server listed in this tool's catalog likely provides the capability the task needs",
			"You know the target MCP tool name and need its input schema before calling it",
		},
		WhenNotToUse: []string{
			"The needed tool definition is already loaded in your tool list — call it directly",
			"The task is served by builtin tools (files, shell, web search) without MCP",
		},
		InputSchema: tool.ToolParametersSchema{
			Type: "object",
			Properties: map[string]tool.ToolParam{
				"query": {Type: "string", Description: "Keyword query, select:<names>, or an exact tool name."},
				"limit": {Type: "integer", Description: "Maximum matches to return and load (default 5, max 20)."},
			},
			Required:             []string{"query"},
			AdditionalProperties: tool.BoolPtr(false),
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"query": {Type: "string", Description: "The executed query."},
			"matches": {Type: "array", Description: "Matched tools by full name, server and readiness; full schemas are injected into the next model request.", Items: &tool.ToolParam{
				Type: "object", Properties: map[string]tool.ToolParam{
					"name":      {Type: "string", Description: "Full registered MCP tool name."},
					"server":    {Type: "string", Description: "Owning MCP server identity."},
					"readiness": {Type: "string", Description: "Non-ready state when applicable."},
				}, Required: []string{"name", "server"}, AdditionalProperties: tool.BoolPtr(false),
			}},
			"loaded":          {Type: "array", Description: "Tool names loaded into the model tool list for subsequent turns.", Items: &tool.ToolParam{Type: "string"}},
			"total_mcp_tools": {Type: "integer", Description: "Total MCP tools in the current run snapshot."},
			"pending_servers": {Type: "array", Description: "Servers that are not ready (needs auth or unavailable).", Items: &tool.ToolParam{Type: "string"}},
			"note":            {Type: "string", Description: "Follow-up guidance for calling the loaded tools."},
		}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{{
			Kind:          tool.SideEffectNone,
			Description:   "Reads the pinned run tool snapshot in memory; records which deferred definitions were loaded. No workspace, network or MCP server call.",
			Reversibility: tool.ReversibilityNotApplicable,
		}},
		Permissions: tool.ToolPermissionSpec{
			CapabilityClass: tool.CapabilityLocalTransform,
			WorkspaceScope:  tool.WorkspaceScopeNone,
			FileBridgeScope: tool.FileBridgeNone,
			NetworkScope:    tool.NetworkScopeNone,
		},
		Examples: []tool.ToolExample{
			{
				Scenario: "Find GitHub issue tools",
				Input:    map[string]any{"query": "github create issue"},
				Outcome:  "Returns matched mcp__github__* names and loads their definitions for direct calls.",
			},
			{
				Scenario: "Load two exact tools",
				Input:    map[string]any{"query": "select:mcp__github__create_issue,mcp__github__list_issues"},
				Outcome:  "Returns both names and loads their definitions.",
			},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "mcp_find_query_required", Cause: "query was empty.", Retryable: false, Recovery: []string{"Provide keywords or select:<tool names>."}},
			{Code: "mcp_find_snapshot_unavailable", Cause: "No pinned run tool snapshot is bound to this execution.", Retryable: false, Recovery: []string{"Retry within a normal Awaken run."}},
			{Code: "mcp_find_activation_persist_failed", Cause: "The durable activation record could not be appended.", Retryable: true, Recovery: []string{"Retry the search once persistence recovers."}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: true, Destructive: false, OpenWorld: false,
			CapabilityClass: tool.CapabilityLocalTransform, TrustedOrigin: tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeNone, NetworkScope: tool.NetworkScopeNone,
		},
	})
}

func (t *Tool) IsReadOnly() bool { return true }

type findInput struct {
	Query string `json:"query"`
	// Limit tolerates providers that emit integers as JSON floats (e.g. 5.0).
	Limit float64 `json:"limit"`
}

type matchPayload struct {
	Name      string `json:"name"`
	Server    string `json:"server"`
	Readiness string `json:"readiness,omitempty"`
}

type findOutput struct {
	Query          string         `json:"query"`
	Matches        []matchPayload `json:"matches"`
	Loaded         []string       `json:"loaded,omitempty"`
	TotalMCPTools  int            `json:"total_mcp_tools"`
	PendingServers []string       `json:"pending_servers,omitempty"`
	Note           string         `json:"note,omitempty"`
}

func (t *Tool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome keeps malformed input and missing-snapshot conditions in the
// semantic result channel so the model can correct the invocation.
func (t *Tool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	var request findInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &request); err != nil {
		outcome := tool.FailedOutcome("mcp_find arguments must be a JSON object: "+err.Error(), "mcp_find_invalid_arguments", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_json_object_arguments"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if strings.TrimSpace(request.Query) == "" {
		outcome := tool.FailedOutcome("mcp_find_query_required: query must not be empty", "mcp_find_query_required", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_keywords_or_select_names"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	limit := int(request.Limit)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	snapshot, _, ok := tool.RunAvailabilityFromContext(ctx)
	if !ok {
		outcome := tool.FailedOutcome("mcp_find_snapshot_unavailable: no pinned run tool snapshot bound to this execution", "mcp_find_snapshot_unavailable", tool.ErrorCategoryRuntime, false)
		outcome.RecoveryOptions = []string{"retry_within_a_normal_run"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	servers := tool.CollectMCPCatalog(&snapshot)
	total := 0
	pending := make([]string, 0)
	for _, server := range servers {
		total += len(server.Tools)
		if server.Readiness != "" && server.Readiness != tool.ProviderReady {
			pending = append(pending, server.Server+" ("+string(server.Readiness)+")")
		}
	}

	matches := tool.SearchMCPTools(servers, request.Query, limit)
	output := findOutput{
		Query:         request.Query,
		Matches:       make([]matchPayload, 0, len(matches)),
		TotalMCPTools: total,
	}
	loaded := make([]string, 0, len(matches))
	for _, match := range matches {
		readiness := ""
		if match.Tool.Readiness != "" && match.Tool.Readiness != tool.ProviderReady {
			readiness = string(match.Tool.Readiness)
		}
		output.Matches = append(output.Matches, matchPayload{
			Name:      match.Tool.Name,
			Server:    match.Tool.Server,
			Readiness: readiness,
		})
		loaded = append(loaded, match.Tool.Name)
	}

	switch {
	case len(matches) == 0 && total == 0:
		output.Note = "No MCP tools are connected in this run."
	case len(matches) == 0 && len(pending) > 0:
		output.Note = "No matching MCP tools. Some servers are still connecting or need auth — retry later or refine the query."
	case len(matches) == 0:
		output.Note = "No matching MCP tools. Try different keywords, a +required term, or select:<exact tool name>."
	default:
		if observer := tool.MCPActivationObserverFromContext(ctx); observer != nil {
			if err := observer(ctx, loaded); err != nil {
				outcome := tool.FailedOutcome("mcp_find_activation_persist_failed: "+err.Error(), "mcp_find_activation_persist_failed", tool.ErrorCategoryRuntime, true)
				outcome.RecoveryOptions = []string{"retry_after_persistence_recovers"}
				return outcome.WithTiming(started, time.Now().UTC()), nil
			}
			output.Loaded = loaded
			output.Note = "The matched tool definitions are loaded into the next model request and callable by full name."
		} else {
			output.Note = "Matched tool names found, but this execution has no activation observer to load their definitions."
		}
	}
	if len(pending) > 0 {
		output.PendingServers = pending
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		outcome := tool.FailedOutcome("mcp_find result encoding failed: "+err.Error(), "mcp_find_result_encoding_failed", tool.ErrorCategoryRuntime, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	details := map[string]any{
		"query":           output.Query,
		"matches":         output.Matches,
		"total_mcp_tools": output.TotalMCPTools,
		"note":            output.Note,
	}
	if len(output.Loaded) > 0 {
		details["loaded"] = output.Loaded
	}
	if len(output.PendingServers) > 0 {
		details["pending_servers"] = output.PendingServers
	}
	outcome.Details = details
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func init() {
	tool.MustRegisterToolWithMode(&Tool{}, tool.DefinitionValidationStrictBuiltin)
}
