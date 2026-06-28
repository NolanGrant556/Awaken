package magicdocs

import (
	"errors"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestDetectMagicDocHeaderWithItalicInstructions(t *testing.T) {
	content := "intro\n  #   magic   doc :   Runtime Map  \n\n _Keep it architectural._\nbody"

	detection, ok := DetectMagicDocHeader(content)
	if !ok {
		t.Fatal("expected magic doc header")
	}
	if detection.Title != "Runtime Map" {
		t.Fatalf("expected title Runtime Map, got %q", detection.Title)
	}
	if detection.HeaderLine != "  #   magic   doc :   Runtime Map  " {
		t.Fatalf("expected exact header line, got %q", detection.HeaderLine)
	}
	if !detection.HasInstructions || detection.Instructions != "Keep it architectural." {
		t.Fatalf("unexpected instructions: %+v", detection)
	}
	if detection.ItalicLine != " _Keep it architectural._" {
		t.Fatalf("expected exact italic line, got %q", detection.ItalicLine)
	}
}

func TestDetectMagicDocHeaderWithoutItalicInstructions(t *testing.T) {
	content := "# MAGIC DOC: Plain\nNot italic\n"

	detection, ok := DetectMagicDocHeader(content)
	if !ok {
		t.Fatal("expected magic doc header")
	}
	if detection.Title != "Plain" {
		t.Fatalf("expected title Plain, got %q", detection.Title)
	}
	if detection.HasInstructions || detection.Instructions != "" || detection.ItalicLine != "" {
		t.Fatalf("expected no italic instructions, got %+v", detection)
	}
}

func TestTrackerRegisterDedupe(t *testing.T) {
	tracker := NewTracker()

	if !tracker.Register("docs/runtime.md") {
		t.Fatal("expected first register to add")
	}
	if tracker.Register("docs/runtime.md") {
		t.Fatal("expected duplicate register to be ignored")
	}
	if tracker.Len() != 1 {
		t.Fatalf("expected one tracked doc, got %d", tracker.Len())
	}
	paths := tracker.Paths()
	if len(paths) != 1 || paths[0] != "docs/runtime.md" {
		t.Fatalf("unexpected paths: %+v", paths)
	}
}

func TestTrackerPlanSkipsNonMainQueryToolCallsAndNoDocs(t *testing.T) {
	t.Run("non-main query", func(t *testing.T) {
		tracker := NewTracker()
		tracker.Register("docs/runtime.md")

		plan, err := tracker.PlanUpdates(UpdateContext{
			QuerySource: "magic_docs",
			Latest: map[string]ReadResult{
				"docs/runtime.md": SuccessfulRead("# MAGIC DOC: Runtime\n"),
			},
		})
		if err != nil {
			t.Fatalf("PlanUpdates returned error: %v", err)
		}
		if plan.SkipReason != PlanSkipNonMainQuery || len(plan.Updates) != 0 {
			t.Fatalf("expected non-main skip, got %+v", plan)
		}
	})

	t.Run("last assistant has tool calls", func(t *testing.T) {
		tracker := NewTracker()
		tracker.Register("docs/runtime.md")

		plan, err := tracker.PlanUpdates(UpdateContext{
			QuerySource: QuerySourceReplMainThread,
			Messages: []convcontext.Message{
				{Role: convcontext.RoleAssistant, ToolCalls: []convcontext.ToolCall{
					{ID: "call_1", Function: convcontext.FunctionCall{Name: "read_file"}},
				}},
				{Role: convcontext.RoleTool, ToolCallID: "call_1", Name: "read_file", Content: "ok"},
			},
			Latest: map[string]ReadResult{
				"docs/runtime.md": SuccessfulRead("# MAGIC DOC: Runtime\n"),
			},
		})
		if err != nil {
			t.Fatalf("PlanUpdates returned error: %v", err)
		}
		if plan.SkipReason != PlanSkipLastAssistantToolCalls || len(plan.Updates) != 0 {
			t.Fatalf("expected tool-call skip, got %+v", plan)
		}
	})

	t.Run("no tracked docs", func(t *testing.T) {
		plan, err := NewTracker().PlanUpdates(UpdateContext{
			QuerySource: QuerySourceReplMainThread,
		})
		if err != nil {
			t.Fatalf("PlanUpdates returned error: %v", err)
		}
		if plan.SkipReason != PlanSkipNoTrackedDocs || len(plan.Updates) != 0 {
			t.Fatalf("expected no-docs skip, got %+v", plan)
		}
	})
}

func TestTrackerPlanRemovesDeletedInaccessibleAndNoHeaderDocs(t *testing.T) {
	tracker := NewTracker()
	tracker.Register("docs/deleted.md")
	tracker.Register("docs/inaccessible.md")
	tracker.Register("docs/plain.md")

	plan, err := tracker.PlanUpdates(UpdateContext{
		QuerySource: QuerySourceReplMainThread,
		Latest: map[string]ReadResult{
			"docs/deleted.md":      DeletedRead(errors.New("missing")),
			"docs/inaccessible.md": InaccessibleRead(errors.New("permission denied")),
			"docs/plain.md":        SuccessfulRead("# Regular Doc\n"),
		},
	})
	if err != nil {
		t.Fatalf("PlanUpdates returned error: %v", err)
	}
	if len(plan.Updates) != 0 {
		t.Fatalf("expected no updates, got %+v", plan.Updates)
	}
	if tracker.Len() != 0 {
		t.Fatalf("expected all docs removed, got paths %+v", tracker.Paths())
	}
}

func TestTrackerPlanBuildsSequentialUpdatesInRegistrationOrder(t *testing.T) {
	tracker := NewTracker()
	tracker.Register("docs/first.md")
	tracker.Register("docs/second.md")

	plan, err := tracker.PlanUpdates(UpdateContext{
		QuerySource: QuerySourceReplMainThread,
		Latest: map[string]ReadResult{
			"docs/first.md":  SuccessfulRead("# MAGIC DOC: First\n"),
			"docs/second.md": SuccessfulRead("# MAGIC DOC: Second\n"),
		},
	})
	if err != nil {
		t.Fatalf("PlanUpdates returned error: %v", err)
	}
	if !plan.Sequential {
		t.Fatal("expected sequential update plan")
	}
	if len(plan.Updates) != 2 {
		t.Fatalf("expected two updates, got %+v", plan.Updates)
	}
	if plan.Updates[0].Path != "docs/first.md" || plan.Updates[1].Path != "docs/second.md" {
		t.Fatalf("expected registration order, got %+v", plan.Updates)
	}
	if plan.Updates[0].Detection.Title != "First" || plan.Updates[1].Detection.Title != "Second" {
		t.Fatalf("unexpected detections: %+v", plan.Updates)
	}
}

func TestBuildUpdatePromptPreservesRulesAndCustomInstructions(t *testing.T) {
	content := "#   MAGIC DOC : Core Runtime\n*Focus on architecture.*\n\nCurrent notes"
	detection, ok := DetectMagicDocHeader(content)
	if !ok {
		t.Fatal("expected magic doc header")
	}

	prompt := BuildUpdatePrompt(PromptOptions{
		DocContents: content,
		DocPath:     "docs/core-runtime.md",
		Detection:   detection,
	})

	for _, want := range []string{
		"#   MAGIC DOC : Core Runtime",
		"*Focus on architecture.*",
		"DOCUMENT-SPECIFIC UPDATE INSTRUCTIONS",
		"Focus on architecture.",
		"Your ONLY task is to use the Edit tool",
		"Use the Edit tool with file_path: docs/core-runtime.md",
		"NOT a changelog or history",
		"OVERVIEWS, ARCHITECTURE, and ENTRY POINTS",
		"Do NOT duplicate information that's already obvious from reading the source code",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q\n\n%s", want, prompt)
		}
	}
}

func TestSubstituteVariablesSinglePass(t *testing.T) {
	got := SubstituteVariables(
		"A={{docContents}} B={{docPath}} C={{missing}}",
		map[string]string{
			"docContents": "{{docPath}} and $1 stay literal",
			"docPath":     "docs/live.md",
		},
	)
	want := "A={{docPath}} and $1 stay literal B=docs/live.md C={{missing}}"
	if got != want {
		t.Fatalf("unexpected substitution:\nwant %q\ngot  %q", want, got)
	}
}

func TestToolPolicyAllowDeny(t *testing.T) {
	policy := NewToolPolicy("docs/core.md")

	allowed := policy.Check(EditToolName, map[string]any{"file_path": "docs/core.md"})
	if !allowed.Allowed() {
		t.Fatalf("expected allow, got %+v", allowed)
	}

	for _, tt := range []struct {
		name     string
		toolName string
		input    any
	}{
		{name: "wrong tool", toolName: "read_file", input: map[string]any{"file_path": "docs/core.md"}},
		{name: "wrong path", toolName: EditToolName, input: map[string]any{"file_path": "docs/other.md"}},
		{name: "missing path", toolName: EditToolName, input: map[string]any{"old_string": "x"}},
		{name: "json input wrong path", toolName: EditToolName, input: `{"file_path":"docs/other.md"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decision := policy.Check(tt.toolName, tt.input)
			if decision.Allowed() {
				t.Fatalf("expected deny, got %+v", decision)
			}
			if !strings.Contains(decision.Message, "only Edit is allowed for docs/core.md") {
				t.Fatalf("unexpected deny message: %q", decision.Message)
			}
		})
	}
}
