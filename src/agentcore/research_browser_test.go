package agentcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"awaken/src/browserhost"
	convcontext "awaken/src/context"
	"awaken/src/researchruntime"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	_ "awaken/src/tools/browser"
)

func TestResearchBrowserRestrictionSurvivesDescendantsAndStaleToolCalls(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	t.Cleanup(func() { _ = browserhost.Set(nil) })
	if err := browserhost.Set(&browserhost.Registration{BaseURL: server.URL, Token: "test"}); err != nil {
		t.Fatal(err)
	}
	root := rootAgentConfig(profile.WorkspaceWrite())
	root.ResearchIntent = &researchruntime.Intent{Query: "research", SourceHint: researchruntime.SourceWeb}
	root.ToolExposure = tool.ToolExposurePolicy{Mode: tool.ExposureAll, ExtraVisible: []string{"browser_navigate"}}
	child, err := CreateSubagent(root, SpawnConfig{AgentID: "child", ParentAgentID: "root"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := CreateSubagent(child, SpawnConfig{AgentID: "grandchild", ParentAgentID: "child"})
	if err != nil {
		t.Fatal(err)
	}
	if child.ResearchIntent != nil || grandchild.ResearchIntent != nil || !child.BackgroundResearch || !grandchild.BackgroundResearch {
		t.Fatal("research restriction must inherit without creating another workflow")
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(tool.ExecutionEnvironmentSnapshot{SnapshotID: "research-browser"})
	for _, cfg := range []AgentConfig{root, child, grandchild} {
		for _, pinned := range []bool{false, true} {
			if pinned {
				cfg.ToolAvailabilitySnapshot = &snapshot
			}
			for _, schema := range NewRequestBuilder(cfg).getTools() {
				if strings.HasPrefix(schema.Function.Name, "browser_") {
					t.Fatalf("foreground tool leaked: %s", schema.Function.Name)
				}
			}
		}
		runtime := NewToolRuntimeWithConfig(nil, toolRuntimeConfigFromAgentConfig(cfg))
		for _, name := range []string{"browser_navigate", "browser_snapshot", "browser_click", "browser_type", "browser_wait", "browser_extract", "browser_release"} {
			result := runtime.Execute(context.Background(), convcontext.ToolCall{ID: "call", Type: "function", Function: convcontext.FunctionCall{Name: name, Arguments: `{"url":"https://example.com"}`}})
			if !result.IsError || result.ErrorCode != "research_background_only" {
				t.Fatalf("stale call not blocked: %+v", result)
			}
		}
	}
	if calls.Load() != 0 {
		t.Fatal("research reached the visible Desktop browser host")
	}
	// A subsequent ordinary run must retain normal browser functionality.
	root.ResearchIntent = nil
	found := false
	for _, schema := range NewRequestBuilder(root).getTools() {
		if schema.Function.Name == "browser_navigate" {
			found = true
		}
	}
	if !found {
		t.Fatal("ordinary browser tools were removed")
	}
}
