package sidequestion

import (
	"errors"
	"testing"
)

func TestBuildForkPlanRejectsEmptyQuestion(t *testing.T) {
	_, err := BuildForkPlan("  \n\t", SideQuestionContext{})
	if !errors.Is(err, ErrEmptyQuestion) {
		t.Fatalf("expected ErrEmptyQuestion, got %v", err)
	}
}

func TestBuildForkPlanStripsInProgressAssistant(t *testing.T) {
	plan, err := BuildForkPlan("what changed?", SideQuestionContext{
		Messages: []Message{
			{Role: RoleUser, Content: "main task"},
			{Role: RoleAssistant, Content: "partial"},
		},
		RebuiltCacheSafeParams: CacheSafeParams{SystemPrompt: "rebuilt"},
	})
	if err != nil {
		t.Fatalf("BuildForkPlan returned error: %v", err)
	}

	if !plan.StrippedInProgressAssistant {
		t.Fatal("expected in-progress assistant to be stripped")
	}
	if plan.ForkMessageCount != 1 {
		t.Fatalf("expected one fork message, got %d", plan.ForkMessageCount)
	}
	if got := plan.CacheSafeParams.ForkContextMessages[0].Content; got != "main task" {
		t.Fatalf("unexpected fork message content %q", got)
	}
}

func TestBuildForkPlanUsesMessagesAfterCompactBoundary(t *testing.T) {
	done := stopReason("end_turn")
	plan, err := BuildForkPlan("quick check", SideQuestionContext{
		Messages: []Message{
			{Role: RoleUser, Content: "before compact"},
			{Role: RoleAssistant, Content: "compact summary", StopReason: done, CompactBoundary: true},
			{Role: RoleUser, Content: "after compact"},
			{Role: RoleAssistant, Content: "answer", StopReason: done},
		},
		RebuiltCacheSafeParams: CacheSafeParams{SystemPrompt: "rebuilt"},
	})
	if err != nil {
		t.Fatalf("BuildForkPlan returned error: %v", err)
	}

	if plan.CompactBoundaryIndex != 1 {
		t.Fatalf("expected compact boundary index 1, got %d", plan.CompactBoundaryIndex)
	}
	got := plan.CacheSafeParams.ForkContextMessages
	if len(got) != 2 {
		t.Fatalf("expected two messages after compact boundary, got %+v", got)
	}
	if got[0].Content != "after compact" || got[1].Content != "answer" {
		t.Fatalf("unexpected fork messages: %+v", got)
	}
}

func TestBuildForkPlanPrefersSavedCacheSafeParams(t *testing.T) {
	plan, err := BuildForkPlan("side question", SideQuestionContext{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
		LastCacheSafeParams: &CacheSafeParams{
			SystemPrompt:   "saved system",
			UserContext:    "saved user",
			SystemContext:  "saved runtime",
			ToolUseContext: "old tools",
		},
		RebuiltCacheSafeParams: CacheSafeParams{
			SystemPrompt:  "rebuilt system",
			UserContext:   "rebuilt user",
			SystemContext: "rebuilt runtime",
		},
		CurrentToolUseContext: "current tools",
	})
	if err != nil {
		t.Fatalf("BuildForkPlan returned error: %v", err)
	}

	if plan.CacheSource != CacheSourceLastSafe {
		t.Fatalf("expected saved cache source, got %s", plan.CacheSource)
	}
	if !plan.CacheHitExpected {
		t.Fatal("expected cache hit marker when saved params are reused")
	}
	if plan.CacheSafeParams.SystemPrompt != "saved system" {
		t.Fatalf("expected saved system prompt, got %q", plan.CacheSafeParams.SystemPrompt)
	}
	if plan.CacheSafeParams.ToolUseContext != "current tools" {
		t.Fatalf("expected current tool context override, got %q", plan.CacheSafeParams.ToolUseContext)
	}
}

func TestBuildForkPlanFallsBackToRebuiltPromptContext(t *testing.T) {
	plan, err := BuildForkPlan("side question", SideQuestionContext{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
		RebuiltCacheSafeParams: CacheSafeParams{
			SystemPrompt:  "rebuilt system",
			UserContext:   "rebuilt user",
			SystemContext: "rebuilt runtime",
		},
	})
	if err != nil {
		t.Fatalf("BuildForkPlan returned error: %v", err)
	}

	if plan.CacheSource != CacheSourceRebuilt {
		t.Fatalf("expected rebuilt cache source, got %s", plan.CacheSource)
	}
	if plan.CacheHitExpected {
		t.Fatal("did not expect cache hit marker for rebuilt params")
	}
	if plan.CacheSafeParams.SystemPrompt != "rebuilt system" ||
		plan.CacheSafeParams.UserContext != "rebuilt user" ||
		plan.CacheSafeParams.SystemContext != "rebuilt runtime" {
		t.Fatalf("unexpected fallback params: %+v", plan.CacheSafeParams)
	}
}

func TestBuildProviderRouteOrdersPreferredAndSkipsUnavailable(t *testing.T) {
	route := BuildProviderRoute([]ProviderCandidate{
		{Name: "openrouter", Provider: "openrouter", Model: "or-fast", Available: false, SupportsText: true},
		{Name: "nous", Provider: "nous", Model: "nous-fast", Available: true, SupportsText: true},
		{Name: "codex", Provider: "openai-codex", Model: "codex-fast", Available: true, SupportsText: true},
	}, "openrouter", true)

	current, ok := route.Current()
	if !ok {
		t.Fatal("expected a provider candidate")
	}
	if current.Provider != "nous" {
		t.Fatalf("expected unavailable preferred provider to be skipped, got %+v", current)
	}
	if len(route.Candidates) != 2 {
		t.Fatalf("expected two usable candidates, got %+v", route.Candidates)
	}
}

func TestBuildProviderRouteFiltersVisionCandidates(t *testing.T) {
	route := BuildProviderRouteForTask([]ProviderCandidate{
		{Name: "main", Provider: "main", Model: "text-only", Available: true, SupportsText: true},
		{Name: "openrouter", Provider: "openrouter", Model: "vision", Available: true, SupportsVision: true},
		{Name: "nous", Provider: "nous", Model: "vision", Available: true, SupportsVision: true},
	}, "main", true, TaskTypeVision)

	current, ok := route.Current()
	if !ok {
		t.Fatal("expected a vision provider candidate")
	}
	if current.Provider != "openrouter" {
		t.Fatalf("expected first vision-capable fallback, got %+v", current)
	}
	if len(route.Candidates) != 2 {
		t.Fatalf("expected only vision-capable candidates, got %+v", route.Candidates)
	}
}

func TestProviderRouteFallsBackOnCreditAndRateLimitErrors(t *testing.T) {
	route := BuildProviderRoute([]ProviderCandidate{
		{Name: "openrouter", Provider: "openrouter", Model: "or-fast", Available: true, SupportsText: true},
		{Name: "nous", Provider: "nous", Model: "nous-fast", Available: true, SupportsText: true},
		{Name: "codex", Provider: "openai-codex", Model: "codex-fast", Available: true, SupportsText: true},
	}, "", true)

	next, ok := route.AdvanceOnError(&ProviderError{StatusCode: 402, Message: "insufficient credits"})
	if !ok {
		t.Fatal("expected credit error to advance to fallback provider")
	}
	if next.Provider != "nous" {
		t.Fatalf("expected nous fallback, got %+v", next)
	}

	next, ok = route.AdvanceOnError(ProviderError{StatusCode: 429, Message: "rate limit exceeded"})
	if !ok {
		t.Fatal("expected rate-limit error to advance to fallback provider")
	}
	if next.Provider != "openai-codex" {
		t.Fatalf("expected codex fallback, got %+v", next)
	}
	if len(route.FailureRecord) != 2 {
		t.Fatalf("expected two recorded failures, got %+v", route.FailureRecord)
	}
	if route.FailureRecord[0].Class != ProviderErrorCreditExhausted ||
		route.FailureRecord[1].Class != ProviderErrorRateLimited {
		t.Fatalf("unexpected failure classes: %+v", route.FailureRecord)
	}
}

func TestRecordResultKeepsAbortSafeStatus(t *testing.T) {
	plan := ForkPlan{
		Question: "will this abort?",
		UsageIncrement: UsageCounters{
			SideQuestionCountIncrement: 1,
		},
	}

	result := RecordResult(plan, RunOutcome{
		Response: "late response",
		Aborted:  true,
		Usage: UsageCounters{
			PromptTokens: 8,
			TotalTokens:  8,
		},
	})

	if result.Status != RunStatusAborted {
		t.Fatalf("expected aborted status, got %s", result.Status)
	}
	if result.Response != "" || result.Error != "" {
		t.Fatalf("abort result should not surface response or error: %+v", result)
	}
	if result.Usage.SideQuestionCountIncrement != 1 || result.Usage.TotalTokens != 8 {
		t.Fatalf("unexpected usage counters: %+v", result.Usage)
	}
}

func stopReason(reason string) *string {
	return &reason
}
