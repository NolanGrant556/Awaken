package tool

import "testing"

func TestDefaultExposureUsesGenericRules(t *testing.T) {
	tools := []Tool{
		namedTool("Read"),
		namedTool("Edit"),
		namedTool("Write"),
	}
	filtered := FilterToolsForExposure(tools, DefaultToolExposurePolicy())
	names := map[string]bool{}
	for _, tool := range filtered {
		names[tool.Name()] = true
	}
	if !names["Edit"] || !names["Read"] || !names["Write"] {
		t.Fatalf("expected ordinary file tools visible, got %v", names)
	}
}

func TestLegacyModesUseSameGenericExposure(t *testing.T) {
	tools := []Tool{namedTool("Read"), namedTool("Edit")}
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
	tools := []Tool{namedTool("Edit")}
	filtered := FilterToolsForExposure(tools, ToolExposurePolicy{
		Mode:         ExposureDefault,
		ExtraVisible: []string{"Edit"},
	})
	if len(filtered) != 1 {
		t.Fatalf("expected ExtraVisible to keep Edit, got %d", len(filtered))
	}
}
