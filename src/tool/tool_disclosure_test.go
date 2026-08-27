package tool

import (
	"context"
	"strings"
	"testing"
)

func TestResolveDisclosurePolicy(t *testing.T) {
	cases := []struct {
		raw     string
		mode    DisclosureMode
		percent int
	}{
		{"", DisclosureAlways, 0},
		{"auto", DisclosureAuto, DefaultDeferralThresholdPercent},
		{"AUTO:25", DisclosureAuto, 25},
		{"auto:0", DisclosureAlways, 0},
		{"auto:100", DisclosureOff, 0},
		{"off", DisclosureOff, 0},
		{"false", DisclosureOff, 0},
		{"0", DisclosureOff, 0},
		{"always", DisclosureAlways, 0},
		{"true", DisclosureAlways, 0},
		{"1", DisclosureAlways, 0},
		{"auto:abc", DisclosureAlways, 0},
		{"garbage", DisclosureAlways, 0},
	}
	for _, testCase := range cases {
		policy := ResolveDisclosurePolicy(testCase.raw)
		if policy.Mode != testCase.mode || policy.ThresholdPercent != testCase.percent {
			t.Fatalf("ResolveDisclosurePolicy(%q) = %+v, want mode=%s percent=%d",
				testCase.raw, policy, testCase.mode, testCase.percent)
		}
	}
}

func TestDeferrableBuiltinToolNamesExcludeCoreLoopTools(t *testing.T) {
	names := DeferrableBuiltinToolNames()
	if len(names) == 0 {
		t.Fatal("deferrable builtin group must not be empty")
	}
	forbidden := []string{
		"Bash", "Read", "Edit", "Write", "Glob", "Grep", "WebSearch",
		"subagent", "update_plan", "ask_user", "artifact_publish", "memory",
		"trash_file", "skill", "skill_resource_read", "skill_resource_list",
		ToolFindToolName, "computer_observe", "computer_act", "computer_verify",
		"image_generate", "video_generate",
	}
	for _, name := range forbidden {
		if IsDeferrableBuiltinToolName(name) {
			t.Fatalf("core-loop tool %q must never be deferrable", name)
		}
	}
	for _, required := range []string{"configure_hook", "archived_history", "plugin", "calculator"} {
		if !IsDeferrableBuiltinToolName(required) {
			t.Fatalf("expected %q in the deferrable builtin group", required)
		}
	}
}

type disclosureFakeTool struct {
	name        string
	source      ToolSource
	server      string
	rawName     string
	description string
	readiness   ProviderReadiness
	properties  map[string]ToolParam
}

func (t *disclosureFakeTool) Name() string                { return t.name }
func (t *disclosureFakeTool) Description() string         { return t.description }
func (t *disclosureFakeTool) Parameters() []ToolParameter { return nil }
func (t *disclosureFakeTool) Execute(context.Context, string) (string, error) {
	return "ok", nil
}
func (t *disclosureFakeTool) Definition() ToolDefinition {
	source := t.source
	if source == "" {
		source = SourceMCP
	}
	definition := ToolDefinition{
		Name: t.name, DisplayName: t.name, Source: source, Description: t.description,
		InputSchema: ToolParametersSchema{
			Type: "object", Properties: t.properties,
		},
		Annotations: ToolAnnotations{ReadOnly: true},
	}
	if source == SourceMCP {
		definition.ProviderID = ProviderID("mcp:" + t.server)
		definition.Annotations.ServerName = t.server
		definition.Annotations.ToolName = t.rawName
	}
	return definition
}
func (t *disclosureFakeTool) ProviderReadiness(ExecutionEnvironmentSnapshot) ProviderReadinessReport {
	readiness := t.readiness
	if readiness == "" {
		readiness = ProviderReady
	}
	return ProviderReadinessReport{Readiness: readiness}
}

func registerDisclosureFixture(t *testing.T) *RunToolAvailabilitySnapshot {
	t.Helper()
	fakes := []*disclosureFakeTool{
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
		{
			name: "configure_hook", source: SourceBuiltin,
			description: "View or modify Awaken Hook configurations from natural language requests. Never accepts raw shell strings.",
			properties:  map[string]ToolParam{"action": {Type: "string"}},
		},
		{
			name: "calculator", source: SourceBuiltin,
			description: "Evaluate a basic arithmetic expression.",
			properties:  map[string]ToolParam{"expression": {Type: "string"}},
		},
	}
	for _, fake := range fakes {
		fake := fake
		if err := RegisterToolForce(fake); err != nil {
			t.Fatalf("register %s: %v", fake.name, err)
		}
		t.Cleanup(func() { UnregisterTool(fake.name) })
	}
	snapshot := NewRunToolAvailabilitySnapshot(ExecutionEnvironmentSnapshot{SnapshotID: "env_tool_disclosure"})
	return &snapshot
}

func TestCollectMCPCatalogGroupsServersIncludingPluginIdentity(t *testing.T) {
	snapshot := registerDisclosureFixture(t)
	servers := CollectMCPCatalog(snapshot, DefaultToolExposurePolicy())

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

func TestCollectMCPCatalogRespectsExposurePolicy(t *testing.T) {
	snapshot := registerDisclosureFixture(t)
	servers := CollectMCPCatalog(snapshot, ToolExposurePolicy{ExtraHidden: []string{"mcp__github__create_issue"}})
	for _, server := range servers {
		for _, item := range server.Tools {
			if item.Name == "mcp__github__create_issue" {
				t.Fatalf("hidden MCP tool leaked into the catalog: %+v", servers)
			}
		}
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

func TestCollectBuiltinDeferredCatalogFiltersGroupAndExposure(t *testing.T) {
	snapshot := registerDisclosureFixture(t)

	entries := CollectBuiltinDeferredCatalog(snapshot, DefaultToolExposurePolicy())
	byName := map[string]BuiltinCatalogTool{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	hook, ok := byName["configure_hook"]
	if !ok {
		t.Fatalf("configure_hook missing from builtin catalog: %+v", entries)
	}
	if hook.Summary != "View or modify Awaken Hook configurations from natural language requests." {
		t.Fatalf("summary must keep only the first sentence: %q", hook.Summary)
	}
	if _, ok := byName["calculator"]; !ok {
		t.Fatalf("calculator missing from builtin catalog: %+v", entries)
	}
	for _, entry := range entries {
		if !IsDeferrableBuiltinToolName(entry.Name) {
			t.Fatalf("non-deferrable builtin leaked into catalog: %+v", entry)
		}
	}

	hidden := CollectBuiltinDeferredCatalog(snapshot, ToolExposurePolicy{ExtraHidden: []string{"configure_hook"}})
	for _, entry := range hidden {
		if entry.Name == "configure_hook" {
			t.Fatalf("hidden builtin tool leaked into the catalog: %+v", hidden)
		}
	}
}

func TestRenderBuiltinCatalogDegradesWithinBudget(t *testing.T) {
	snapshot := registerDisclosureFixture(t)
	entries := CollectBuiltinDeferredCatalog(snapshot, DefaultToolExposurePolicy())

	full := RenderBuiltinCatalog(entries, DefaultBuiltinCatalogBudgetRunes)
	if !strings.Contains(full, "configure_hook: View or modify Awaken Hook configurations") {
		t.Fatalf("full catalog must include name and summary: %q", full)
	}
	if strings.Contains(full, "Never accepts raw shell strings") {
		t.Fatalf("catalog must not include text beyond the first sentence: %q", full)
	}

	namesOnly := RenderBuiltinCatalog(entries, 40)
	if strings.Contains(namesOnly, ": ") && !strings.Contains(namesOnly, "catalog truncated") {
		t.Fatalf("tight budget should drop summaries or truncate explicitly: %q", namesOnly)
	}

	truncated := RenderBuiltinCatalog(entries, 10)
	if !strings.Contains(truncated, "catalog truncated") {
		t.Fatalf("overflow must carry an explicit truncation marker: %q", truncated)
	}
}

func TestRenderMCPCatalogUsesNamesOnlyAndDegradesWithinBudget(t *testing.T) {
	snapshot := registerDisclosureFixture(t)
	servers := CollectMCPCatalog(snapshot, DefaultToolExposurePolicy())
	keep := servers[:0]
	for _, server := range servers {
		if server.Server == "github" || server.Server == "plugin__pack-tools__notes" {
			keep = append(keep, server)
		}
	}
	servers = keep

	full := RenderMCPCatalog(servers, DefaultMCPCatalogBudgetRunes)
	if !strings.Contains(full, "create_issue") || !strings.Contains(full, "list_issues") {
		t.Fatalf("full catalog should include tool names: %q", full)
	}
	if strings.Contains(full, "Create a new GitHub issue") {
		t.Fatalf("model-facing catalog must not include deferred descriptions: %q", full)
	}
	if !strings.Contains(full, "needs_auth") {
		t.Fatalf("non-ready server state must be visible: %q", full)
	}

	nameOnly := RenderMCPCatalog(servers, 150)
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

func testDeferredSearchSpace(t *testing.T) DeferredToolSearchSpace {
	t.Helper()
	snapshot := registerDisclosureFixture(t)
	return DeferredToolSearchSpace{
		Builtin: CollectBuiltinDeferredCatalog(snapshot, DefaultToolExposurePolicy()),
		Servers: CollectMCPCatalog(snapshot, DefaultToolExposurePolicy()),
	}
}

func TestSearchDeferredToolsKeywordSelectAndRequiredTerms(t *testing.T) {
	space := testDeferredSearchSpace(t)

	matches := SearchDeferredTools(space, "github issue create", 5)
	if len(matches) == 0 || matches[0].Name != "mcp__github__create_issue" || matches[0].Origin != SourceMCP {
		t.Fatalf("keyword search top hit wrong: %+v", matches)
	}

	matches = SearchDeferredTools(space, "+notes write", 5)
	if len(matches) != 1 || matches[0].Name != "mcp__plugin__pack-tools__notes__write_note" {
		t.Fatalf("+required term should filter to plugin tool: %+v", matches)
	}

	matches = SearchDeferredTools(space, "select:mcp__github__list_issues, write_note, configure_hook", 5)
	if len(matches) != 3 || matches[0].Name != "mcp__github__list_issues" ||
		matches[1].Name != "mcp__plugin__pack-tools__notes__write_note" ||
		matches[2].Name != "configure_hook" || matches[2].Origin != SourceBuiltin {
		t.Fatalf("select multi-resolution wrong: %+v", matches)
	}
	matches = SearchDeferredTools(space, "select:mcp__github__list_issues, write_note", 1)
	if len(matches) != 1 || matches[0].Name != "mcp__github__list_issues" {
		t.Fatalf("select must honor limit: %+v", matches)
	}

	matches = SearchDeferredTools(space, "create_issue", 5)
	if len(matches) != 1 || matches[0].Name != "mcp__github__create_issue" {
		t.Fatalf("bare raw name should resolve exactly: %+v", matches)
	}

	matches = SearchDeferredTools(space, "configure_hook", 5)
	if len(matches) != 1 || matches[0].Name != "configure_hook" || matches[0].Origin != SourceBuiltin {
		t.Fatalf("bare builtin name should resolve exactly: %+v", matches)
	}

	matches = SearchDeferredTools(space, "hook configuration", 5)
	if len(matches) == 0 || matches[0].Name != "configure_hook" {
		t.Fatalf("builtin keyword search top hit wrong: %+v", matches)
	}

	if matches = SearchDeferredTools(space, "zzz_nothing_matches", 5); len(matches) != 0 {
		t.Fatalf("expected no matches: %+v", matches)
	}

	matches = SearchDeferredTools(space, "issue", 1)
	if len(matches) != 1 {
		t.Fatalf("limit must cap matches: %+v", matches)
	}
}

func TestDeferredToolSearchSpaceContextRoundTrip(t *testing.T) {
	if _, ok := DeferredToolSearchSpaceFromContext(context.Background()); ok {
		t.Fatal("empty context must not carry a search space")
	}
	space := DeferredToolSearchSpace{Builtin: []BuiltinCatalogTool{{Name: "calculator"}}}
	ctx := ContextWithDeferredToolSearchSpace(context.Background(), space)
	got, ok := DeferredToolSearchSpaceFromContext(ctx)
	if !ok || len(got.Builtin) != 1 || got.Builtin[0].Name != "calculator" {
		t.Fatalf("search space round trip failed: %+v ok=%v", got, ok)
	}
	if got.TotalDeferredTools() != 1 {
		t.Fatalf("total deferred count wrong: %d", got.TotalDeferredTools())
	}
}
