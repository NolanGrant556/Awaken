package tool

import "testing"

func TestDefaultExposureHidesApplyPatch(t *testing.T) {
	tools := []Tool{
		namedTool("read_file"),
		namedTool("text_edit"),
		namedTool("apply_patch"),
		namedTool("text_file_create"),
	}
	filtered := FilterToolsForExposure(tools, DefaultToolExposurePolicy())
	names := map[string]bool{}
	for _, tool := range filtered {
		names[tool.Name()] = true
	}
	if !names["text_edit"] || !names["read_file"] || !names["text_file_create"] {
		t.Fatalf("expected ordinary file tools visible, got %v", names)
	}
	if names["apply_patch"] {
		t.Fatal("ordinary providers must hide apply_patch by default")
	}
}

func TestCompatAndGrammarExposeApplyPatch(t *testing.T) {
	tools := []Tool{namedTool("apply_patch"), namedTool("text_edit")}
	for _, policy := range []ToolExposurePolicy{
		CompatToolExposurePolicy(),
		GrammarCapableToolExposurePolicy(),
		{Mode: ExposureAll},
	} {
		filtered := FilterToolsForExposure(tools, policy)
		if len(filtered) != 2 {
			t.Fatalf("policy %q should expose both tools, got %d", policy.Mode, len(filtered))
		}
	}
}

func TestExtraVisibleOverridesHide(t *testing.T) {
	tools := []Tool{namedTool("apply_patch")}
	filtered := FilterToolsForExposure(tools, ToolExposurePolicy{
		Mode:         ExposureDefault,
		ExtraVisible: []string{"apply_patch"},
	})
	if len(filtered) != 1 {
		t.Fatalf("expected ExtraVisible to keep apply_patch, got %d", len(filtered))
	}
}
