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

func TestNormalizeCompactSummaryStripsAnalysisWrapper(t *testing.T) {
	raw := strings.Join([]string{
		"<analysis>",
		"internal audit should not be retained",
		"</analysis>",
		"<summary>",
		"1. Active Task:",
		"- Continue low distortion compact.",
		"2. Goal:",
		"- Preserve canary facts.",
		"13. Critical Context:",
		"- legacy canary survives.",
		"</summary>",
	}, "\n")

	normalized := normalizeCompactSummary(raw)
	if strings.Contains(normalized, "internal audit") ||
		strings.Contains(normalized, "<summary>") ||
		strings.Contains(normalized, "</summary>") {
		t.Fatalf("expected analysis and summary wrappers to be stripped, got %q", normalized)
	}
	if !strings.Contains(normalized, "Active Task\n- Continue low distortion compact.") ||
		!strings.Contains(normalized, "Critical Context\n- legacy canary survives.") {
		t.Fatalf("expected numbered headings inside summary wrapper to normalize, got %q", normalized)
	}
}
