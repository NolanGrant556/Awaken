package compactengine

import (
	"strings"
	"testing"
)

func TestPlanSkipsWhenUnderBudget(t *testing.T) {
	messages := []Message{
		msg("s0", RoleSystem, 5),
		msg("u1", RoleUser, 10),
	}
	engine := New(Config{TokenBudget: 100, SummaryTokenEstimate: 10, LastTurnsToKeep: 1}, failSummarizer{t: t})

	plan := engine.Plan(messages, CompactionReasonManual)
	if plan.ShouldCompact {
		t.Fatalf("expected no compaction plan")
	}
	if plan.SkipReason != SkipReasonUnderBudget {
		t.Fatalf("expected under-budget skip, got %q", plan.SkipReason)
	}

	result, err := engine.Compact(messages, CompactionReasonManual)
	if err != nil {
		t.Fatalf("compact under budget: %v", err)
	}
	if len(result.Messages) != len(messages) {
		t.Fatalf("expected unchanged messages, got %d", len(result.Messages))
	}
	if result.Metrics.WasCompacted {
		t.Fatalf("under-budget result should not report compaction")
	}
}

func TestPlanChoosesMiddleRegion(t *testing.T) {
	messages := []Message{
		msg("s0", RoleSystem, 5),
		msg("u1", RoleUser, 5),
		assistant("a2", 5, "call_first"),
		tool("t3", 5, "call_first"),
		msg("u4", RoleUser, 10),
		msg("a5", RoleAssistant, 90),
		msg("u6", RoleUser, 10),
		msg("a7", RoleAssistant, 5),
		msg("u8", RoleUser, 5),
	}
	plan := Plan(messages, Config{TokenBudget: 90, SummaryTokenEstimate: 10, LastTurnsToKeep: 2}, CompactionReasonAuto)

	if !plan.ShouldCompact {
		t.Fatalf("expected compaction, skip=%q", plan.SkipReason)
	}
	if plan.StartIndex != 5 || plan.EndIndex != 6 {
		t.Fatalf("expected centered middle region [5,6), got [%d,%d)", plan.StartIndex, plan.EndIndex)
	}
	if got := strings.Join(plan.SelectedSourceIDs, ","); got != "a5" {
		t.Fatalf("expected selected source a5, got %q", got)
	}
	if plan.Reason != CompactionReasonAuto {
		t.Fatalf("expected auto reason, got %q", plan.Reason)
	}
}

func TestPlanProtectsFirstTurnsLastTurnsAndToolPairing(t *testing.T) {
	messages := []Message{
		msg("s0", RoleSystem, 5),
		msg("u1", RoleUser, 5),
		assistant("a2", 5, "call_first"),
		tool("t3", 5, "call_first"),
		msg("u4", RoleUser, 10),
		assistant("a5", 10, "call_middle"),
		tool("t6", 50, "call_middle"),
		msg("u7", RoleUser, 10),
		msg("a8", RoleAssistant, 5),
		msg("u9", RoleUser, 5),
	}
	plan := Plan(messages, Config{TokenBudget: 70, SummaryTokenEstimate: 10, LastTurnsToKeep: 2}, CompactionReasonManual)

	if !plan.ShouldCompact {
		t.Fatalf("expected compaction, skip=%q", plan.SkipReason)
	}
	assertInts(t, plan.ProtectedIndices, []int{0, 1, 2, 3, 8, 9})
	if plan.StartIndex != 5 || plan.EndIndex != 7 {
		t.Fatalf("expected tool call/result pair [5,7), got [%d,%d)", plan.StartIndex, plan.EndIndex)
	}
	if got := strings.Join(plan.SelectedSourceIDs, ","); got != "a5,t6" {
		t.Fatalf("expected selected tool pair sources, got %q", got)
	}
}

func TestCompactReportsTokenSavingsMetricsAndBoundary(t *testing.T) {
	messages := []Message{
		msg("s0", RoleSystem, 5),
		msg("u1", RoleUser, 5),
		assistant("a2", 5, "call_first"),
		tool("t3", 5, "call_first"),
		msg("u4", RoleUser, 10),
		msg("a5", RoleAssistant, 90),
		msg("u6", RoleUser, 10),
		msg("a7", RoleAssistant, 5),
		msg("u8", RoleUser, 5),
	}
	engine := New(Config{TokenBudget: 90, SummaryTokenEstimate: 10, LastTurnsToKeep: 2}, nil)

	result, err := engine.Compact(messages, CompactionReasonAuto)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if !result.Metrics.WasCompacted {
		t.Fatalf("expected metrics to report compaction")
	}
	if result.Metrics.TokensSaved <= 0 {
		t.Fatalf("expected positive token savings, got %d", result.Metrics.TokensSaved)
	}
	if result.Boundary.Reason != CompactionReasonAuto {
		t.Fatalf("expected auto boundary, got %q", result.Boundary.Reason)
	}
	if result.Boundary.StartIndex != 5 || result.Boundary.EndIndex != 6 {
		t.Fatalf("unexpected boundary range [%d,%d)", result.Boundary.StartIndex, result.Boundary.EndIndex)
	}
	if result.StillOverLimit {
		t.Fatalf("expected compacted result to fit target")
	}
	if len(result.Messages) != len(messages)+1 {
		t.Fatalf("single message region should be replaced by boundary+summary, got %d messages", len(result.Messages))
	}
}

func TestSummaryMaterialReplacesMediaWithMarkers(t *testing.T) {
	summarizer := &captureSummarizer{summary: "media summarized"}
	messages := []Message{
		msg("s0", RoleSystem, 5),
		msg("u1", RoleUser, 5),
		assistant("a2", 5, "call_first"),
		tool("t3", 5, "call_first"),
		{
			ID:            "u4",
			SourceID:      "u4",
			Role:          RoleUser,
			Content:       "Please inspect the attached screenshot.",
			TokenEstimate: 80,
			Media: []MediaMarker{{
				ID:          "img-1",
				Kind:        MediaImage,
				Description: "screenshot",
				Payload:     "raw-image-bytes-that-must-not-reach-the-summarizer",
			}},
		},
		msg("a5", RoleAssistant, 5),
		msg("u6", RoleUser, 5),
	}
	engine := New(Config{TokenBudget: 70, SummaryTokenEstimate: 10, LastTurnsToKeep: 2}, summarizer)

	result, err := engine.Compact(messages, CompactionReasonManual)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if !strings.Contains(summarizer.input.Material, "[image:img-1 screenshot]") {
		t.Fatalf("expected image marker in summary material:\n%s", summarizer.input.Material)
	}
	if strings.Contains(summarizer.input.Material, "raw-image-bytes") {
		t.Fatalf("media payload leaked into summary material:\n%s", summarizer.input.Material)
	}
	if !strings.HasPrefix(result.SummaryMessage.Content, FallbackSummaryPrefix) {
		t.Fatalf("expected normalized summary prefix, got %q", result.SummaryMessage.Content)
	}
}

func TestCircuitBreakerStopsAutoCompactionAfterConsecutiveFailures(t *testing.T) {
	config := compactTestAutoConfig()
	breaker := CircuitBreakerState{}
	breaker = breaker.RecordFailure()
	breaker = breaker.RecordFailure()
	breaker = breaker.RecordFailure()

	if !breaker.IsOpen(config.MaxConsecutiveFailures) {
		t.Fatalf("expected circuit breaker open")
	}
	decision := ShouldAutoCompact(850, config, breaker)
	if decision.ShouldCompact {
		t.Fatalf("open circuit breaker should block auto compaction")
	}
	if decision.Reason != AutoCompactDecisionCircuitBreakerOpen {
		t.Fatalf("expected circuit breaker reason, got %q", decision.Reason)
	}

	breaker = breaker.RecordSuccess()
	if breaker.IsOpen(config.MaxConsecutiveFailures) {
		t.Fatalf("success should reset circuit breaker")
	}
}

func TestTokenThresholdStates(t *testing.T) {
	config := compactTestAutoConfig()
	state := CalculateTokenThresholdState(850, config)

	if state.EffectiveContextWindow != 900 {
		t.Fatalf("expected effective window 900, got %d", state.EffectiveContextWindow)
	}
	if state.AutoCompactThreshold != 800 {
		t.Fatalf("expected auto threshold 800, got %d", state.AutoCompactThreshold)
	}
	if state.WarningThreshold != 750 || state.ErrorThreshold != 780 || state.BlockingLimit != 870 {
		t.Fatalf("unexpected thresholds: %+v", state)
	}
	if !state.IsAboveWarningThreshold || !state.IsAboveErrorThreshold || !state.IsAboveAutoCompactThreshold {
		t.Fatalf("expected warning/error/auto threshold flags: %+v", state)
	}
	if state.IsAtBlockingLimit {
		t.Fatalf("850 should not be at blocking limit 870")
	}
	if state.State != ThresholdStateError {
		t.Fatalf("expected error state, got %q", state.State)
	}

	blocked := CalculateTokenThresholdState(875, config)
	if !blocked.IsAtBlockingLimit || blocked.State != ThresholdStateBlocking {
		t.Fatalf("expected blocking state, got %+v", blocked)
	}
}

func TestFallbackSummarizerUsesContextSummaryPrefix(t *testing.T) {
	summary, err := (FallbackSummarizer{}).Summarize(SummaryInput{
		Plan: CompactionPlan{SelectedSourceIDs: []string{"u4", "a5"}},
		Messages: []Message{
			msg("u4", RoleUser, 10),
			msg("a5", RoleAssistant, 10),
		},
	})
	if err != nil {
		t.Fatalf("fallback summarize: %v", err)
	}
	if !strings.HasPrefix(summary, FallbackSummaryPrefix) {
		t.Fatalf("expected prefix %q, got %q", FallbackSummaryPrefix, summary)
	}
	if strings.Count(summary, FallbackSummaryPrefix) != 1 {
		t.Fatalf("expected prefix exactly once, got %q", summary)
	}
}

type captureSummarizer struct {
	input   SummaryInput
	summary string
}

func (s *captureSummarizer) Summarize(input SummaryInput) (string, error) {
	s.input = input
	return s.summary, nil
}

type failSummarizer struct {
	t *testing.T
}

func (s failSummarizer) Summarize(input SummaryInput) (string, error) {
	s.t.Fatalf("summarizer should not be called for skipped compaction")
	return "", nil
}

func msg(sourceID string, role Role, tokens int) Message {
	return Message{
		ID:            sourceID,
		SourceID:      sourceID,
		Role:          role,
		Content:       sourceID,
		TokenEstimate: tokens,
	}
}

func assistant(sourceID string, tokens int, toolCallID string) Message {
	message := msg(sourceID, RoleAssistant, tokens)
	message.ToolCalls = []ToolCallMarker{{ID: toolCallID, Name: "read"}}
	return message
}

func tool(sourceID string, tokens int, toolCallID string) Message {
	message := msg(sourceID, RoleTool, tokens)
	message.ToolResult = &ToolResultMarker{ID: toolCallID, Name: "read"}
	return message
}

func compactTestAutoConfig() AutoCompactConfig {
	return AutoCompactConfig{
		ContextWindowTokens:       1000,
		MaxOutputTokensForSummary: 100,
		AutoCompactBufferTokens:   100,
		WarningBufferTokens:       50,
		ErrorBufferTokens:         20,
		ManualCompactBufferTokens: 30,
		AutoCompactEnabled:        true,
		MaxConsecutiveFailures:    3,
	}
}

func assertInts(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value mismatch: got %v want %v", got, want)
		}
	}
}
