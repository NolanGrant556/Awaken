package agentcore

import (
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

func TestCompactSummaryPromptUsesBudgetFirstFinalOnlyContract(t *testing.T) {
	system := compactSummarySystemPrompt()
	if strings.Contains(strings.ToLower(system), "may first write an <analysis>") ||
		!strings.Contains(system, "Start the response with <summary> immediately") {
		t.Fatalf("compact summary system prompt must not spend a tight budget on analysis: %s", system)
	}
	instruction := compactSummaryUserInstruction(
		"compact-budget", ContextEnvelope{},
		[]convcontext.Message{{Role: convcontext.RoleUser, Content: "preserve the canary"}},
		nil, "", 1024,
	)
	if !strings.Contains(instruction, "Hard output budget: at most 1024 tokens") ||
		!strings.Contains(instruction, "Fit continuity-critical facts and </summary> before the limit") {
		t.Fatalf("compact summary request must expose the tight output contract: %s", instruction)
	}
	if !strings.Contains(system, "Prefer these section headings") ||
		strings.Contains(system, "Use exactly these section headings") {
		t.Fatalf("compact summary system prompt must treat headings as guidance: %s", system)
	}
}

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

func TestNormalizeCompactSummaryAcceptsCaseInsensitiveHeadings(t *testing.T) {
	raw := strings.Join([]string{
		"active task",
		"- Continue the MODEL-008 retest.",
		"CRITICAL CONTEXT",
		"- Keep source SHA256 9585489d.",
	}, "\n")

	normalized := normalizeCompactSummary(raw)
	if !strings.Contains(normalized, "Active Task\n- Continue the MODEL-008 retest.") ||
		!strings.Contains(normalized, "Critical Context\n- Keep source SHA256 9585489d.") {
		t.Fatalf("expected canonical section names after case-insensitive parse, got %q", normalized)
	}
}

func TestResolveCompactSummaryTextKeepsUnstructuredNarrative(t *testing.T) {
	prose := "主 Agent 已读到第 72 块，子任务还在跑，不要丢掉 SA-R1-FIRST-CH372。"
	text, structuredEmpty := resolveCompactSummaryText(prose, 1024)
	if structuredEmpty || !strings.Contains(text, "SA-R1-FIRST-CH372") {
		t.Fatalf("expected unstructured prose to stay installable, text=%q empty=%v", text, structuredEmpty)
	}
}

func TestResolveCompactSummaryTextTreatsAllNoneAsStructuredEmpty(t *testing.T) {
	raw := renderCompactSummarySections(map[string][]string{})
	text, structuredEmpty := resolveCompactSummaryText(raw, 1024)
	if !structuredEmpty || text != "" {
		t.Fatalf("expected all-None headings to request fallback, text=%q empty=%v", text, structuredEmpty)
	}
}

func TestBoundCompactSummaryToBudgetEnforcesHostLimitAndCriticalBreadth(t *testing.T) {
	canary := "DS-V2-R4-20260901-靛青量尺-5539"
	sections := map[string][]string{}
	for _, section := range compactSummarySections {
		sections[section] = []string{"- " + canary + " " + strings.Repeat(section+" detail ", 120)}
	}
	bounded := boundCompactSummaryToBudget(renderCompactSummarySections(sections), 1024)
	if got, limit := convcontext.EstimateTokens(bounded), 1024-compactSummaryHostHeaderReserveTokens; got > limit {
		t.Fatalf("bounded summary tokens=%d, limit=%d", got, limit)
	}
	if !strings.Contains(bounded, canary) ||
		!compactSummarySectionHasMeaning(parseCompactSummarySections(bounded)["Active Task"]) ||
		!compactSummarySectionHasMeaning(parseCompactSummarySections(bounded)["Critical Context"]) {
		t.Fatalf("host budget lost critical continuity: %s", bounded)
	}
	for _, section := range compactSummarySections {
		if !strings.Contains(bounded, section+"\n") {
			t.Fatalf("bounded summary lost section %q: %s", section, bounded)
		}
	}
}
