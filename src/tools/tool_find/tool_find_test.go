package toolfind

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"awaken/src/tool"
)

func fixtureSearchSpace() tool.DeferredToolSearchSpace {
	titleSchema := tool.ToolParametersSchema{
		Type:       "object",
		Properties: map[string]tool.ToolParam{"title": {Type: "string"}},
	}
	return tool.DeferredToolSearchSpace{
		Builtin: []tool.BuiltinCatalogTool{
			{
				Name:        "configure_hook",
				Summary:     "View or modify Awaken Hook configurations from natural language requests.",
				Description: "View or modify Awaken Hook configurations from natural language requests.",
				InputSchema: tool.ToolParametersSchema{
					Type:       "object",
					Properties: map[string]tool.ToolParam{"action": {Type: "string"}},
				},
			},
		},
		Servers: []tool.MCPCatalogServer{
			{
				Server: "issues",
				Tools: []tool.MCPCatalogTool{
					{Name: "mcp__issues__close_issue", RawName: "close_issue", Server: "issues", Description: "Close one tracked issue.", InputSchema: titleSchema},
					{Name: "mcp__issues__create_issue", RawName: "create_issue", Server: "issues", Description: "Create a tracked issue.", InputSchema: titleSchema},
				},
			},
			{
				Server:    "mailer",
				Readiness: tool.ProviderNeedsAuth,
				Tools: []tool.MCPCatalogTool{
					{Name: "mcp__mailer__send_mail", RawName: "send_mail", Server: "mailer", Description: "Send an email message.", Readiness: tool.ProviderNeedsAuth, InputSchema: titleSchema},
				},
			},
		},
	}
}

func fixtureContext(observer tool.DeferredToolActivationObserver) context.Context {
	ctx := tool.ContextWithDeferredToolSearchSpace(context.Background(), fixtureSearchSpace())
	if observer != nil {
		ctx = tool.ContextWithDeferredToolActivationObserver(ctx, observer)
	}
	return ctx
}

func TestExecuteSearchLoadsMatchesAndReportsPendingServers(t *testing.T) {
	var observed []string
	ctx := fixtureContext(func(_ context.Context, names []string) error {
		observed = append(observed, names...)
		return nil
	})

	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"query":"create issue"}`, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if outcome.IsError() {
		t.Fatalf("execute outcome failed: %+v", outcome)
	}
	raw := outcome.Content
	var output struct {
		Matches []struct {
			Name   string `json:"name"`
			Origin string `json:"origin"`
			Server string `json:"server"`
		} `json:"matches"`
		Loaded             []string `json:"loaded"`
		TotalDeferredTools int      `json:"total_deferred_tools"`
		PendingServers     []string `json:"pending_servers"`
		Note               string   `json:"note"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("result must be JSON: %v\n%s", err, raw)
	}
	if len(output.Matches) == 0 || output.Matches[0].Name != "mcp__issues__create_issue" || output.Matches[0].Origin != "mcp" {
		t.Fatalf("top match wrong: %+v", output.Matches)
	}
	if strings.Contains(raw, "input_schema") || strings.Contains(raw, "Create a tracked issue") {
		t.Fatalf("compact result must not duplicate deferred schemas or descriptions: %s", raw)
	}
	if payload := outcome.ModelPayload(); strings.Contains(payload, "input_schema") || strings.Contains(payload, "Create a tracked issue") {
		t.Fatalf("model payload must not duplicate deferred schemas or descriptions: %s", payload)
	}
	if len(output.Loaded) == 0 || output.Loaded[0] != "mcp__issues__create_issue" {
		t.Fatalf("loaded names missing: %+v", output.Loaded)
	}
	if len(observed) == 0 || observed[0] != "mcp__issues__create_issue" {
		t.Fatalf("observer must receive loaded names: %+v", observed)
	}
	if output.TotalDeferredTools != 4 {
		t.Fatalf("total should count builtin and MCP deferred tools: %d", output.TotalDeferredTools)
	}
	foundPending := false
	for _, pending := range output.PendingServers {
		if strings.Contains(pending, "mailer") && strings.Contains(pending, "needs_auth") {
			foundPending = true
		}
	}
	if !foundPending {
		t.Fatalf("needs-auth server must be reported pending: %+v", output.PendingServers)
	}
}

func TestExecuteLoadsDeferredBuiltinTool(t *testing.T) {
	var observed []string
	ctx := fixtureContext(func(_ context.Context, names []string) error {
		observed = append(observed, names...)
		return nil
	})

	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"query":"configure_hook"}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("execute: %v %+v", err, outcome)
	}
	var output struct {
		Matches []struct {
			Name   string `json:"name"`
			Origin string `json:"origin"`
			Server string `json:"server"`
		} `json:"matches"`
		Loaded []string `json:"loaded"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &output); err != nil {
		t.Fatalf("result must be JSON: %v\n%s", err, outcome.Content)
	}
	if len(output.Matches) != 1 || output.Matches[0].Name != "configure_hook" || output.Matches[0].Origin != "builtin" {
		t.Fatalf("builtin match wrong: %+v", output.Matches)
	}
	if output.Matches[0].Server != "" {
		t.Fatalf("builtin match must not carry a server identity: %+v", output.Matches)
	}
	if len(observed) != 1 || observed[0] != "configure_hook" {
		t.Fatalf("observer must receive the builtin name: %+v", observed)
	}
}

func TestExecuteValidatesInputAndCatalogPresence(t *testing.T) {
	if _, err := (&Tool{}).Execute(context.Background(), `{"query":"anything"}`); err == nil ||
		!strings.Contains(err.Error(), "tool_find_catalog_unavailable") {
		t.Fatalf("missing catalog must fail closed: %v", err)
	}
	ctx := fixtureContext(nil)
	if _, err := (&Tool{}).Execute(ctx, `{"query":"  "}`); err == nil ||
		!strings.Contains(err.Error(), "tool_find_query_required") {
		t.Fatalf("empty query must be rejected: %v", err)
	}
	if _, err := (&Tool{}).Execute(ctx, `not json`); err == nil {
		t.Fatalf("non-JSON input must be rejected")
	}
}

func TestExecutePropagatesObserverPersistFailure(t *testing.T) {
	ctx := fixtureContext(func(context.Context, []string) error {
		return context.DeadlineExceeded
	})
	if _, err := (&Tool{}).Execute(ctx, `{"query":"select:mcp__issues__close_issue"}`); err == nil ||
		!strings.Contains(err.Error(), "tool_find_activation_persist_failed") {
		t.Fatalf("observer failure must fail the call: %v", err)
	}
}
