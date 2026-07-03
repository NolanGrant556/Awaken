package agentcore

import (
	"strings"
	"testing"
)

func TestNormalizeCompactSummaryProducesFixedSectionOrder(t *testing.T) {
	raw := strings.Join([]string{
		"## Goal",
		"- Finish the refactor.",
		"## Active Task",
		"Continue wiring the compactor.",
		"## Critical Context",
		"- Do not touch renderer files.",
	}, "\n")

	normalized := normalizeCompactSummary(raw)
	lastIndex := -1
	for _, section := range compactSummarySections {
		index := strings.Index(normalized, section)
		if index < 0 {
			t.Fatalf("expected section %q in normalized summary, got %q", section, normalized)
		}
		if index <= lastIndex {
			t.Fatalf("expected section %q after previous section, got %q", section, normalized)
		}
		lastIndex = index
	}
	if !strings.Contains(normalized, "Active Task\n- Continue wiring the compactor.") {
		t.Fatalf("expected non-bulleted content to be normalized into bullet form, got %q", normalized)
	}
	if !strings.Contains(normalized, "Constraints & Preferences\n- None.") {
		t.Fatalf("expected missing sections to be filled with None, got %q", normalized)
	}
}
