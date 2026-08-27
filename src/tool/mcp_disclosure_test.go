package tool

import (
	"context"
	"strings"
	"testing"
)

func TestResolveMCPDisclosurePolicy(t *testing.T) {
	cases := []struct {
		raw     string
		mode    MCPDisclosureMode
		percent int
	}{
		{"", MCPDisclosureAuto, DefaultMCPDeferralThresholdPercent},
		{"auto", MCPDisclosureAuto, DefaultMCPDeferralThresholdPercent},
		{"AUTO:25", MCPDisclosureAuto, 25},
		{"auto:0", MCPDisclosureAlways, 0},
		{"auto:100", MCPDisclosureOff, 0},
		{"off", MCPDisclosureOff, 0},
		{"false", MCPDisclosureOff, 0},
		{"0", MCPDisclosureOff, 0},
		{"always", MCPDisclosureAlways, 0},
		{"true", MCPDisclosureAlways, 0},
		{"1", MCPDisclosureAlways, 0},
		{"auto:abc", MCPDisclosureAuto, DefaultMCPDeferralThresholdPercent},
		{"garbage", MCPDisclosureAuto, DefaultMCPDeferralThresholdPercent},
	}
	for _, testCase := range cases {
		policy := ResolveMCPDisclosurePolicy(testCase.raw)
		if policy.Mode != testCase.mode || policy.ThresholdPercent != testCase.percent {
			t.Fatalf("ResolveMCPDisclosurePolicy(%q) = %+v, want mode=%s percent=%d",
				testCase.raw, policy, testCase.mode, testCase.percent)
		}
	}
}

type mcpDisclosureFakeTool struct {
	name        string
	server      string
	rawName     string
	description string
	readiness   ProviderReadiness
	properties  map[string]ToolParam
}

func (t *mcpDisclosureFakeTool) Name() string                { return t.name }
func (t *mcpDisclosureFakeTool) Description() string         { return t.description }
func (t *mcpDisclosureFakeTool) Parameters() []ToolParameter { return nil }
func (t *mcpDisclosureFakeTool) Execute(context.Context, string) (string, error) {
	return "ok", nil
}
func (t *mcpDisclosureFakeTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: SourceMCP, Description: t.description,
		ProviderID: ProviderID("mcp:" + t.server),
		InputSchema: ToolParametersSchema{
			Type: "object", Properties: t.properties,
		},
		Annotations: ToolAnnotations{ReadOnly: true, ServerName: t.server, ToolName: t.rawName},
	}
}
func (t *mcpDisclosureFakeTool) ProviderReadiness(ExecutionEnvironmentSnapshot) ProviderReadinessReport {
	readiness := t.readiness
	if readiness == "" {
		readiness = ProviderReady
	}
	return ProviderReadinessReport{Readiness: readiness}
}

func registerMCPDisclosureFixture(t *testing.T) *RunToolAvailabilitySnapshot {
	t.Helper()
	fakes := []*mcpDisclosureFakeTool{
		{
			name: "mcp__github__create_issue", server: "github", rawName: "create_issue",
			description: "Create a new GitHub issue in a repository.",
			properties:  map[string]ToolParam{"title": {Type: "string"}, "body": {Type: "string"}},
		},
		{
			name: "mcp__github__list_issues", server: "github", rawName: "list_issues",
			description: "List issues for a repository with filters.",
			properties:  map[string]ToolParam{"state": {Type: "string"}},
		},
		{
			name: "mcp__plugin__pack-tools__notes__write_note", server: "plugin__pack-tools__notes", rawName: "write_note",
			description: "Persist a note into the plugin note store.",
			readiness:   ProviderNeedsAuth,
			properties:  map[string]ToolParam{"content": {Type: "string"}},
		},
	}
	for _, fake := range fakes {
		fake := fake
		if err := RegisterToolForce(fake); err != nil {
			t.Fatalf("register %s: %v", fake.name, err)
		}
		t.Cleanup(func() { UnregisterTool(fake.name) })
	}
	snapshot := NewRunToolAvailabilitySnapshot(ExecutionEnvironmentSnapshot{SnapshotID: "env_mcp_disclosure"})
	return &snapshot
}

func TestCollectMCPCatalogGroupsServersIncludingPluginIdentity(t *testing.T) {
	snapshot := registerMCPDisclosureFixture(t)
	servers := CollectMCPCatalog(snapshot)

	byServer := map[string]MCPCatalogServer{}
	for _, server := range servers {
		byServer[server.Server] = server
	}
	github, ok := byServer["github"]
	if !ok || len(github.Tools) != 2 {
		t.Fatalf("github group missing or wrong size: %+v", servers)
	}
	if github.Tools[0].Name != "mcp__github__create_issue" || github.Tools[0].RawName != "create_issue" {
		t.Fatalf("github tools not sorted or raw names wrong: %+v", github.Tools)
	}
	if github.Readiness != ProviderReady {
		t.Fatalf("github readiness should stay ready: %+v", github)
	}
	plugin, ok := byServer["plugin__pack-tools__notes"]
	if !ok || len(plugin.Tools) != 1 {
		t.Fatalf("plugin server identity lost: %+v", servers)
	}
	if plugin.Readiness != ProviderNeedsAuth {
		t.Fatalf("plugin server readiness should surface needs_auth: %+v", plugin)
	}
	if plugin.Tools[0].InputSchema.Properties["content"].Type != "string" {
		t.Fatalf("input schema not preserved: %+v", plugin.Tools[0].InputSchema)
	}
}

func TestMCPServerFromToolNameFallsBackWithoutAnnotations(t *testing.T) {
	if got := mcpServerFromToolName("mcp__github__create_issue"); got != "github" {
		t.Fatalf("plain server = %q", got)
	}
	if got := mcpServerFromToolName("mcp__plugin__pack__notes__write_note"); got != "plugin__pack__notes" {
		t.Fatalf("plugin server = %q", got)
	}
}

func TestRenderMCPCatalogDegradesWithinBudget(t *testing.T) {
	snapshot := registerMCPDisclosureFixture(t)
	servers := CollectMCPCatalog(snapshot)
	keep := servers[:0]
	for _, server := range servers {
		if server.Server == "github" || server.Server == "plugin__pack-tools__notes" {
			keep = append(keep, server)
		}
	}
	servers = keep

	full := RenderMCPCatalog(servers, DefaultMCPCatalogBudgetRunes)
	if !strings.Contains(full, "create_issue: Create a new GitHub issue") {
		t.Fatalf("full catalog should include per-tool descriptions: %q", full)
	}
	if !strings.Contains(full, "needs_auth") {
		t.Fatalf("non-ready server state must be visible: %q", full)
	}

	nameOnly := RenderMCPCatalog(servers, 150)
	if strings.Contains(nameOnly, "Create a new GitHub issue") {
		t.Fatalf("tight budget should drop descriptions: %q", nameOnly)
	}
	if !strings.Contains(nameOnly, "create_issue") && !strings.Contains(nameOnly, "(2 tools)") {
		t.Fatalf("tight budget should keep names or counts: %q", nameOnly)
	}

	truncated := RenderMCPCatalog(servers, 60)
	if !strings.Contains(truncated, "catalog truncated") {
		t.Fatalf("overflow must carry an explicit truncation marker: %q", truncated)
	}
	if len([]rune(truncated)) > 120 {
		t.Fatalf("truncated catalog exceeds bound: %d runes", len([]rune(truncated)))
	}
}

func TestSearchMCPToolsKeywordSelectAndRequiredTerms(t *testing.T) {
	snapshot := registerMCPDisclosureFixture(t)
	servers := CollectMCPCatalog(snapshot)

	matches := SearchMCPTools(servers, "github issue create", 5)
	if len(matches) == 0 || matches[0].Tool.Name != "mcp__github__create_issue" {
		t.Fatalf("keyword search top hit wrong: %+v", matches)
	}

	matches = SearchMCPTools(servers, "+notes write", 5)
	if len(matches) != 1 || matches[0].Tool.Name != "mcp__plugin__pack-tools__notes__write_note" {
		t.Fatalf("+required term should filter to plugin tool: %+v", matches)
	}

	matches = SearchMCPTools(servers, "select:mcp__github__list_issues, write_note", 5)
	if len(matches) != 2 || matches[0].Tool.Name != "mcp__github__list_issues" || matches[1].Tool.Name != "mcp__plugin__pack-tools__notes__write_note" {
		t.Fatalf("select multi-resolution wrong: %+v", matches)
	}

	matches = SearchMCPTools(servers, "create_issue", 5)
	if len(matches) != 1 || matches[0].Tool.Name != "mcp__github__create_issue" {
		t.Fatalf("bare raw name should resolve exactly: %+v", matches)
	}

	if matches = SearchMCPTools(servers, "zzz_nothing_matches", 5); len(matches) != 0 {
		t.Fatalf("expected no matches: %+v", matches)
	}

	matches = SearchMCPTools(servers, "issue", 1)
	if len(matches) != 1 {
		t.Fatalf("limit must cap matches: %+v", matches)
	}
}
