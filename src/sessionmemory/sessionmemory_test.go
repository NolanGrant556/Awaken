package sessionmemory

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStoreScopeIsolation(t *testing.T) {
	store := NewStore()

	mustAdd(t, store, MemoryEntry{
		Scope:     MemoryScopeSession,
		SessionID: "session-1",
		Key:       "handoff",
		Content:   "session one memory",
	})
	mustAdd(t, store, MemoryEntry{
		Scope:     MemoryScopeSession,
		SessionID: "session-2",
		Key:       "handoff",
		Content:   "session two memory",
	})
	mustAdd(t, store, MemoryEntry{
		Scope:   MemoryScopeAgent,
		AgentID: "planner",
		Key:     "handoff",
		Content: "planner memory",
	})
	mustAdd(t, store, MemoryEntry{
		Scope:   MemoryScopeAgent,
		AgentID: "coder",
		Key:     "handoff",
		Content: "coder memory",
	})

	sessionEntries, err := store.SearchByScope(MemoryScopeSession, "", MemoryFilter{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("SearchByScope returned error: %v", err)
	}
	if len(sessionEntries) != 1 || sessionEntries[0].Content != "session one memory" {
		t.Fatalf("expected only session-1 memory, got %+v", sessionEntries)
	}

	agentEntries, err := store.SearchByScope(MemoryScopeAgent, "", MemoryFilter{AgentID: "planner"})
	if err != nil {
		t.Fatalf("SearchByScope returned error: %v", err)
	}
	if len(agentEntries) != 1 || agentEntries[0].Content != "planner memory" {
		t.Fatalf("expected only planner memory, got %+v", agentEntries)
	}

	missing, err := store.SearchByScope(MemoryScopeSession, "", MemoryFilter{SessionID: "missing"})
	if err != nil {
		t.Fatalf("SearchByScope returned error: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected missing session to return no memories, got %+v", missing)
	}
}

func TestPromptBuilderTruncatesAndOmitsSensitiveFields(t *testing.T) {
	snapshot := MemorySnapshot{
		Entries: []MemoryEntry{
			{
				Scope:           MemoryScopeProject,
				Key:             "style",
				Content:         strings.Repeat("follow existing Go package patterns ", 20),
				Source:          "project defaults",
				Metadata:        map[string]string{"api_key": "SECRET_METADATA"},
				SensitiveFields: map[string]string{"token": "SECRET_FIELD"},
			},
			{
				Scope:     MemoryScopeUser,
				Key:       "private",
				Content:   "SECRET_CONTENT",
				Source:    "hidden",
				Sensitive: true,
			},
		},
	}

	injection := (PromptBuilder{MaxChars: 220}).Build(snapshot)
	if !injection.Injected {
		t.Fatalf("expected prompt injection")
	}
	if !injection.Truncated {
		t.Fatalf("expected prompt injection to be truncated")
	}
	if runeLen(injection.Section) > 220 {
		t.Fatalf("expected section to respect max chars, got %d", runeLen(injection.Section))
	}
	if !strings.Contains(injection.Section, "source: project / project defaults") {
		t.Fatalf("expected source label, got %q", injection.Section)
	}
	for _, secret := range []string{"SECRET_METADATA", "SECRET_FIELD", "SECRET_CONTENT"} {
		if strings.Contains(injection.Section, secret) {
			t.Fatalf("expected prompt to omit sensitive value %q: %q", secret, injection.Section)
		}
	}
}

func TestPromptBuilderOmitsEmptyMemory(t *testing.T) {
	injection := (PromptBuilder{}).Build(MemorySnapshot{})
	if injection.Injected || injection.Section != "" || injection.EntryCount != 0 {
		t.Fatalf("expected empty snapshot to produce no injection, got %+v", injection)
	}
}

func TestPlanExtractionThresholds(t *testing.T) {
	thresholds := ExtractionThresholds{
		MinimumTokensToInit:        1000,
		MinimumTokensBetweenUpdate: 400,
		ToolCallsBetweenUpdates:    2,
	}
	candidate := ExtractionCandidate{
		Scope:     MemoryScopeSession,
		SessionID: "session-1",
		Key:       "summary",
	}

	belowInit := PlanExtraction(TranscriptStats{
		MessageCount: 4,
		TokenCount:   999,
	}, thresholds, candidate)
	if belowInit.ShouldExtract {
		t.Fatalf("expected no extraction below initialization threshold: %+v", belowInit)
	}
	if belowInit.NextState.MemoryInitialized {
		t.Fatalf("expected memory to remain uninitialized")
	}

	belowTokenGrowth := PlanExtraction(TranscriptStats{
		MessageCount:                 6,
		TokenCount:                   1200,
		TokensAtLastExtraction:       900,
		ToolCallsSinceLastExtraction: 4,
		LastAssistantHasToolCalls:    false,
		MemoryInitialized:            true,
	}, thresholds, candidate)
	if belowTokenGrowth.ShouldExtract {
		t.Fatalf("expected token threshold to remain mandatory: %+v", belowTokenGrowth)
	}

	waitingForBreak := PlanExtraction(TranscriptStats{
		MessageCount:                 6,
		TokenCount:                   1400,
		TokensAtLastExtraction:       900,
		ToolCallsSinceLastExtraction: 1,
		LastAssistantHasToolCalls:    true,
		MemoryInitialized:            true,
	}, thresholds, candidate)
	if waitingForBreak.ShouldExtract {
		t.Fatalf("expected no extraction while waiting for tool threshold or natural break: %+v", waitingForBreak)
	}

	naturalBreak := PlanExtraction(TranscriptStats{
		MessageCount:                 7,
		TokenCount:                   1400,
		TokensAtLastExtraction:       900,
		ToolCallsSinceLastExtraction: 1,
		LastAssistantHasToolCalls:    false,
		LastMessageID:                "msg-7",
		MemoryInitialized:            true,
	}, thresholds, candidate)
	if !naturalBreak.ShouldExtract {
		t.Fatalf("expected extraction at natural break: %+v", naturalBreak)
	}
	if naturalBreak.NextState.TokensAtLastExtraction != 1400 {
		t.Fatalf("expected next state token count to update, got %+v", naturalBreak.NextState)
	}
	if naturalBreak.NextState.LastExtractedMessageID != "msg-7" {
		t.Fatalf("expected last extracted message id to update, got %+v", naturalBreak.NextState)
	}
}

func TestSnapshotOrdering(t *testing.T) {
	store := NewStore()
	base := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)

	mustAdd(t, store, MemoryEntry{Scope: MemoryScopeAgent, AgentID: "coder", Key: "agent", Content: "agent", CreatedAt: base.Add(1 * time.Minute)})
	mustAdd(t, store, MemoryEntry{Scope: MemoryScopeSession, SessionID: "session-1", Key: "session", Content: "session", CreatedAt: base.Add(1 * time.Minute)})
	mustAdd(t, store, MemoryEntry{Scope: MemoryScopeProject, Key: "project-b", Content: "project b", CreatedAt: base.Add(2 * time.Minute)})
	mustAdd(t, store, MemoryEntry{Scope: MemoryScopeUser, Key: "user", Content: "user", CreatedAt: base.Add(5 * time.Minute)})
	mustAdd(t, store, MemoryEntry{Scope: MemoryScopeLocal, Key: "local", Content: "local", CreatedAt: base.Add(1 * time.Minute)})
	mustAdd(t, store, MemoryEntry{Scope: MemoryScopeProject, Key: "project-a", Content: "project a", CreatedAt: base.Add(1 * time.Minute)})

	snapshot, err := store.Snapshot(MemoryFilter{})
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}

	got := make([]string, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		got = append(got, entry.Key)
	}
	want := []string{"user", "project-a", "project-b", "local", "session", "agent"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected snapshot order: got %v want %v", got, want)
	}
}

func TestUpdatePolicy(t *testing.T) {
	store := NewStore()
	createdAt := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)

	added := mustAdd(t, store, MemoryEntry{
		Scope:     MemoryScopeProject,
		Key:       "Build",
		Content:   "run unit tests",
		Source:    "initial",
		Tags:      []string{"go"},
		Metadata:  map[string]string{"owner": "runtime"},
		CreatedAt: createdAt,
	})
	if added.Key != "build" {
		t.Fatalf("expected key normalization, got %q", added.Key)
	}

	kept, err := store.Add(MemoryEntry{
		Scope:   MemoryScopeProject,
		Key:     " build ",
		Content: "replace me",
	}, UpdatePolicyKeepExisting)
	if err != nil {
		t.Fatalf("Add duplicate with keep_existing returned error: %v", err)
	}
	if kept.Content != "run unit tests" {
		t.Fatalf("expected keep_existing to preserve content, got %q", kept.Content)
	}

	merged, err := store.Update(MemoryEntry{
		Scope:    MemoryScopeProject,
		Key:      "build",
		Content:  "run package smoke tests",
		Source:   "follow-up",
		Tags:     []string{"smoke"},
		Metadata: map[string]string{"owner": "memory", "risk": "low"},
	}, UpdatePolicyMerge)
	if err != nil {
		t.Fatalf("Update merge returned error: %v", err)
	}
	if !strings.Contains(merged.Content, "run unit tests") || !strings.Contains(merged.Content, "run package smoke tests") {
		t.Fatalf("expected merge to retain both content values, got %q", merged.Content)
	}
	if strings.Join(merged.Tags, ",") != "go,smoke" {
		t.Fatalf("expected merged tags, got %v", merged.Tags)
	}
	if merged.Metadata["owner"] != "memory" || merged.Metadata["risk"] != "low" {
		t.Fatalf("expected metadata merge with incoming override, got %v", merged.Metadata)
	}

	replaced, err := store.Update(MemoryEntry{
		Scope:   MemoryScopeProject,
		Key:     "build",
		Content: "final policy",
	}, UpdatePolicyReplace)
	if err != nil {
		t.Fatalf("Update replace returned error: %v", err)
	}
	if replaced.Content != "final policy" || len(replaced.Tags) != 0 || len(replaced.Metadata) != 0 {
		t.Fatalf("expected replace to overwrite previous content and optional fields, got %+v", replaced)
	}
}

func TestValidationAndNormalize(t *testing.T) {
	if err := ValidateEntry(MemoryEntry{Scope: "unknown", Key: "k", Content: "content"}); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("expected invalid scope error, got %v", err)
	}
	if err := ValidateEntry(MemoryEntry{Scope: MemoryScopeUser, Key: "k", Content: "  "}); !errors.Is(err, ErrInvalidMemoryEntry) {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
	if err := ValidateEntry(MemoryEntry{Scope: MemoryScopeSession, Key: "k", Content: "content"}); !errors.Is(err, ErrInvalidMemoryEntry) {
		t.Fatalf("expected session id validation error, got %v", err)
	}
	if err := ValidateEntry(MemoryEntry{Scope: MemoryScopeAgent, Key: "k", Content: "content"}); !errors.Is(err, ErrInvalidMemoryEntry) {
		t.Fatalf("expected agent id validation error, got %v", err)
	}
	if _, err := NormalizeUpdatePolicy("append"); !errors.Is(err, ErrInvalidUpdatePolicy) {
		t.Fatalf("expected invalid update policy error, got %v", err)
	}

	normalized, err := NormalizeEntry(MemoryEntry{
		Scope:     " USER ",
		Key:       " Preferred Style ",
		Content:   "  Keep changes small.  ",
		SessionID: "ignored-session",
		AgentID:   "ignored-agent",
		Tags:      []string{"Go", "go", " tests "},
	})
	if err != nil {
		t.Fatalf("NormalizeEntry returned error: %v", err)
	}
	if normalized.Scope != MemoryScopeUser || normalized.Key != "preferred style" || normalized.Content != "Keep changes small." {
		t.Fatalf("unexpected normalized entry: %+v", normalized)
	}
	if normalized.SessionID != "" || normalized.AgentID != "" {
		t.Fatalf("expected global scopes to drop session/agent ids, got %+v", normalized)
	}
	if strings.Join(normalized.Tags, ",") != "go,tests" {
		t.Fatalf("expected tag normalization, got %v", normalized.Tags)
	}
}

func mustAdd(t *testing.T, store *Store, entry MemoryEntry) MemoryEntry {
	t.Helper()
	added, err := store.Add(entry, UpdatePolicyReplace)
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	return added
}
