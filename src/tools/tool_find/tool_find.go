// Package toolfind implements the builtin auxiliary tool that searches the
// deferred tool catalogs pinned for the current run (deferrable builtin
// auxiliary tools and MCP tools) and loads the matched full definitions on
// demand (progressive disclosure).
package toolfind

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/tool"
)

// Tool searches deferred tools by keyword or exact selection. It never
// executes the target tools itself: results carry compact matched names, and
// the runner-owned activation observer adds the full definitions to the next
// model request's tool list.
type Tool struct{}

func (t *Tool) Name() string { return tool.ToolFindToolName }

func (t *Tool) Description() string {
	return "Search available tools whose full definitions are deferred (not loaded into context) and load the matched definitions on demand."
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
		Name:        tool.ToolFindToolName,
		DisplayName: "Tool Find",
		Description: t.Description(),
		Purpose:     "Discover deferred tools (rarely used builtin auxiliaries and MCP tools) by keyword and load their full definitions into the model tool list without keeping every schema in context.",
		WhenToUse: []string{
			"A tool listed in this tool's deferred catalog likely provides the capability the task needs",
			"You know the target deferred tool name and need its input schema before calling it",
		},
		WhenNotToUse: []string{
			"The needed tool definition is already loaded in your tool list — call it directly",
			"The task is served by the already loaded tools (files, shell, web search) without deferred capabilities",
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
			"matches": {Type: "array", Description: "Matched tools by full name, origin and readiness; full schemas are injected into the next model request.", Items: &tool.ToolParam{
				Type: "object", Properties: map[string]tool.ToolParam{
					"name":      {Type: "string", Description: "Full registered tool name."},
					"origin":    {Type: "string", Description: "Tool origin: builtin or mcp."},
					"server":    {Type: "string", Description: "Owning MCP server identity (MCP tools only)."},
					"readiness": {Type: "string", Description: "Non-ready state when applicable."},
				}, Required: []string{"name", "origin"}, AdditionalProperties: tool.BoolPtr(false),
			}},
			"loaded":               {Type: "array", Description: "Tool names loaded into the model tool list for subsequent turns.", Items: &tool.ToolParam{Type: "string"}},
			"total_deferred_tools": {Type: "integer", Description: "Total deferred tools searchable in the current run."},
			"pending_servers":      {Type: "array", Description: "MCP servers that are not ready (needs auth or unavailable).", Items: &tool.ToolParam{Type: "string"}},
			"note":                 {Type: "string", Description: "Follow-up guidance for calling the loaded tools."},
		}, AdditionalProperties: tool.BoolPtr(false)},
		SideEffects: []tool.ToolSideEffect{{
			Kind:          tool.SideEffectNone,
			Description:   "Reads the run-scoped deferred tool catalogs in memory; records which deferred definitions were loaded. No workspace, network or MCP server call.",
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
				Scenario: "Load the Hook configuration tool",
				Input:    map[string]any{"query": "configure_hook"},
				Outcome:  "Returns the builtin tool name and loads its definition.",
			},
			{
				Scenario: "Load two exact tools",
				Input:    map[string]any{"query": "select:mcp__github__create_issue,mcp__github__list_issues"},
				Outcome:  "Returns both names and loads their definitions.",
			},
		},
		FailureModes: []tool.ToolFailureMode{
			{Code: "tool_find_query_required", Cause: "query was empty.", Retryable: false, Recovery: []string{"Provide keywords or select:<tool names>."}},
			{Code: "tool_find_catalog_unavailable", Cause: "No run-scoped deferred tool catalog is bound to this execution.", Retryable: false, Recovery: []string{"Retry within a normal Awaken run."}},
			{Code: "tool_find_activation_persist_failed", Cause: "The durable activation record could not be appended.", Retryable: true, Recovery: []string{"Retry the search once persistence recovers."}},
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
	Origin    string `json:"origin"`
	Server    string `json:"server,omitempty"`
	Readiness string `json:"readiness,omitempty"`
}

type findOutput struct {
	Query              string         `json:"query"`
	Matches            []matchPayload `json:"matches"`
	Loaded             []string       `json:"loaded,omitempty"`
	TotalDeferredTools int            `json:"total_deferred_tools"`
	PendingServers     []string       `json:"pending_servers,omitempty"`
	Note               string         `json:"note,omitempty"`
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

// ExecuteOutcome keeps malformed input and missing-catalog conditions in the
// semantic result channel so the model can correct the invocation.
func (t *Tool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	var request findInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &request); err != nil {
		outcome := tool.FailedOutcome("tool_find arguments must be a JSON object: "+err.Error(), "tool_find_invalid_arguments", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_json_object_arguments"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if strings.TrimSpace(request.Query) == "" {
		outcome := tool.FailedOutcome("tool_find_query_required: query must not be empty", "tool_find_query_required", tool.ErrorCategoryValidation, false)
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

	space, ok := tool.DeferredToolSearchSpaceFromContext(ctx)
	if !ok {
		outcome := tool.FailedOutcome("tool_find_catalog_unavailable: no run-scoped deferred tool catalog bound to this execution", "tool_find_catalog_unavailable", tool.ErrorCategoryRuntime, false)
		outcome.RecoveryOptions = []string{"retry_within_a_normal_run"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	total := space.TotalDeferredTools()
	pending := make([]string, 0)
	for _, server := range space.Servers {
		if server.Readiness != "" && server.Readiness != tool.ProviderReady {
			pending = append(pending, server.Server+" ("+string(server.Readiness)+")")
		}
	}

	matches := tool.SearchDeferredTools(space, request.Query, limit)
	output := findOutput{
		Query:              request.Query,
		Matches:            make([]matchPayload, 0, len(matches)),
		TotalDeferredTools: total,
	}
	loaded := make([]string, 0, len(matches))
	for _, match := range matches {
		readiness := ""
		if match.Readiness != "" && match.Readiness != tool.ProviderReady {
			readiness = string(match.Readiness)
		}
		output.Matches = append(output.Matches, matchPayload{
			Name:      match.Name,
			Origin:    string(match.Origin),
			Server:    match.Server,
			Readiness: readiness,
		})
		loaded = append(loaded, match.Name)
	}

	switch {
	case len(matches) == 0 && total == 0:
		output.Note = "No deferred tools are available in this run."
	case len(matches) == 0 && len(pending) > 0:
		output.Note = "No matching deferred tools. Some MCP servers are still connecting or need auth — retry later or refine the query."
	case len(matches) == 0:
		output.Note = "No matching deferred tools. Try different keywords, a +required term, or select:<exact tool name>."
	default:
		if observer := tool.DeferredToolActivationObserverFromContext(ctx); observer != nil {
			if err := observer(ctx, loaded); err != nil {
				outcome := tool.FailedOutcome("tool_find_activation_persist_failed: "+err.Error(), "tool_find_activation_persist_failed", tool.ErrorCategoryRuntime, true)
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
		outcome := tool.FailedOutcome("tool_find result encoding failed: "+err.Error(), "tool_find_result_encoding_failed", tool.ErrorCategoryRuntime, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	details := map[string]any{
		"query":                output.Query,
		"matches":              output.Matches,
		"total_deferred_tools": output.TotalDeferredTools,
		"note":                 output.Note,
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
