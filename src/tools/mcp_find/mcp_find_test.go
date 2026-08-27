package mcpfind

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"awaken/src/tool"
)

type fakeMCPTool struct {
	name        string
	server      string
	rawName     string
	description string
	readiness   tool.ProviderReadiness
}

func (t *fakeMCPTool) Name() string                     { return t.name }
func (t *fakeMCPTool) Description() string              { return t.description }
func (t *fakeMCPTool) Parameters() []tool.ToolParameter { return nil }
func (t *fakeMCPTool) Execute(context.Context, string) (string, error) {
	return "ok", nil
}
func (t *fakeMCPTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: tool.SourceMCP, Description: t.description,
		ProviderID: tool.ProviderID("mcp:" + t.server),
		InputSchema: tool.ToolParametersSchema{
			Type:       "object",
			Properties: map[string]tool.ToolParam{"title": {Type: "string"}},
		},
		Annotations: tool.ToolAnnotations{ReadOnly: true, ServerName: t.server, ToolName: t.rawName},
	}
}
func (t *fakeMCPTool) ProviderReadiness(tool.ExecutionEnvironmentSnapshot) tool.ProviderReadinessReport {
	readiness := t.readiness
	if readiness == "" {
		readiness = tool.ProviderReady
	}
	return tool.ProviderReadinessReport{Readiness: readiness}
}

func fixtureContext(t *testing.T, observer tool.MCPActivationObserver) context.Context {
	t.Helper()
	fakes := []*fakeMCPTool{
		{name: "mcp__issues__create_issue", server: "issues", rawName: "create_issue", description: "Create a tracked issue."},
		{name: "mcp__issues__close_issue", server: "issues", rawName: "close_issue", description: "Close one tracked issue."},
		{name: "mcp__mailer__send_mail", server: "mailer", rawName: "send_mail", description: "Send an email message.", readiness: tool.ProviderNeedsAuth},
	}
	for _, fake := range fakes {
		fake := fake
		if err := tool.RegisterToolForce(fake); err != nil {
			t.Fatalf("register %s: %v", fake.name, err)
		}
		t.Cleanup(func() { tool.UnregisterTool(fake.name) })
	}
	environment := tool.ExecutionEnvironmentSnapshot{SnapshotID: "env_mcp_find"}
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	ctx := tool.ContextWithRunAvailability(context.Background(), snapshot, environment)
	if observer != nil {
		ctx = tool.ContextWithMCPActivationObserver(ctx, observer)
	}
	return ctx
}

func TestExecuteSearchLoadsMatchesAndReportsPendingServers(t *testing.T) {
	var observed []string
	ctx := fixtureContext(t, func(_ context.Context, names []string) error {
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
			Server string `json:"server"`
		} `json:"matches"`
		Loaded         []string `json:"loaded"`
		TotalMCPTools  int      `json:"total_mcp_tools"`
		PendingServers []string `json:"pending_servers"`
		Note           string   `json:"note"`
	}
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("result must be JSON: %v\n%s", err, raw)
	}
	if len(output.Matches) == 0 || output.Matches[0].Name != "mcp__issues__create_issue" {
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
	if output.TotalMCPTools < 3 {
		t.Fatalf("total should count snapshot MCP tools: %d", output.TotalMCPTools)
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

func TestExecuteValidatesInputAndSnapshotPresence(t *testing.T) {
	if _, err := (&Tool{}).Execute(context.Background(), `{"query":"anything"}`); err == nil ||
		!strings.Contains(err.Error(), "mcp_find_snapshot_unavailable") {
		t.Fatalf("missing snapshot must fail closed: %v", err)
	}
	ctx := fixtureContext(t, nil)
	if _, err := (&Tool{}).Execute(ctx, `{"query":"  "}`); err == nil ||
		!strings.Contains(err.Error(), "mcp_find_query_required") {
		t.Fatalf("empty query must be rejected: %v", err)
	}
	if _, err := (&Tool{}).Execute(ctx, `not json`); err == nil {
		t.Fatalf("non-JSON input must be rejected")
	}
}

func TestExecutePropagatesObserverPersistFailure(t *testing.T) {
	ctx := fixtureContext(t, func(context.Context, []string) error {
		return context.DeadlineExceeded
	})
	if _, err := (&Tool{}).Execute(ctx, `{"query":"select:mcp__issues__close_issue"}`); err == nil ||
		!strings.Contains(err.Error(), "mcp_find_activation_persist_failed") {
		t.Fatalf("observer failure must fail the call: %v", err)
	}
}
