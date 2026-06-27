package gatewaymeta

import (
	"testing"
	"time"
)

func TestNewEntryInitializesMetadata(t *testing.T) {
	createdAt := time.Date(2026, 6, 28, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	entry := NewEntry("agent:main:telegram:dm:chat-1", "session-1", SessionSource{
		Platform: "telegram",
		ChatID:   "chat-1",
		ChatName: "Nolan",
		UserID:   "user-1",
		RoutingHints: map[string]string{
			"reply_to": "message-1",
		},
	}, createdAt)

	snapshot := entry.ToSnapshot()
	wantTime := createdAt.UTC()
	if snapshot.SessionKey != "agent:main:telegram:dm:chat-1" {
		t.Fatalf("expected session key to be preserved, got %q", snapshot.SessionKey)
	}
	if snapshot.SessionID != "session-1" {
		t.Fatalf("expected session id to be preserved, got %q", snapshot.SessionID)
	}
	if !snapshot.CreatedAt.Equal(wantTime) {
		t.Fatalf("expected created at %s, got %s", wantTime, snapshot.CreatedAt)
	}
	if !snapshot.UpdatedAt.Equal(wantTime) {
		t.Fatalf("expected updated at %s, got %s", wantTime, snapshot.UpdatedAt)
	}
	if snapshot.DisplayName != "Nolan" {
		t.Fatalf("expected display name from chat name, got %q", snapshot.DisplayName)
	}
	if snapshot.Platform != "telegram" {
		t.Fatalf("expected platform telegram, got %q", snapshot.Platform)
	}
	if snapshot.ChatType != ChatTypeDM {
		t.Fatalf("expected default chat type dm, got %q", snapshot.ChatType)
	}
	if snapshot.Cost.Status != CostStatusUnknown {
		t.Fatalf("expected unknown cost status, got %q", snapshot.Cost.Status)
	}
	if snapshot.Suspended || snapshot.Resume.Pending || snapshot.AutoReset.WasAutoReset {
		t.Fatalf("expected fresh entry to have no lifecycle flags, got %+v", snapshot)
	}
}

func TestEntryTouchAndUpdateActivity(t *testing.T) {
	createdAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	touchedAt := createdAt.Add(5 * time.Minute)
	activeAt := createdAt.Add(10 * time.Minute)
	entry := NewEntry("key", "session", SessionSource{}, createdAt)

	entry.Touch(touchedAt)
	snapshot := entry.ToSnapshot()
	if !snapshot.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created at to stay %s, got %s", createdAt, snapshot.CreatedAt)
	}
	if !snapshot.UpdatedAt.Equal(touchedAt) {
		t.Fatalf("expected touched updated at %s, got %s", touchedAt, snapshot.UpdatedAt)
	}
	if snapshot.LastPromptTokens != 0 {
		t.Fatalf("expected no prompt token update from Touch, got %d", snapshot.LastPromptTokens)
	}

	entry.UpdateActivity(activeAt, 4096)
	snapshot = entry.ToSnapshot()
	if !snapshot.UpdatedAt.Equal(activeAt) {
		t.Fatalf("expected activity updated at %s, got %s", activeAt, snapshot.UpdatedAt)
	}
	if snapshot.LastPromptTokens != 4096 {
		t.Fatalf("expected last prompt tokens 4096, got %d", snapshot.LastPromptTokens)
	}
}

func TestEntryAddUsageAccumulatesTokensAndCost(t *testing.T) {
	createdAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	firstAt := createdAt.Add(1 * time.Minute)
	secondAt := createdAt.Add(2 * time.Minute)
	entry := NewEntry("key", "session", SessionSource{}, createdAt)

	entry.AddUsage(TokenUsage{
		InputTokens:      10,
		OutputTokens:     20,
		CacheReadTokens:  3,
		CacheWriteTokens: 2,
	}, CostState{EstimatedUSD: 0.25, Status: CostStatusEstimated}, firstAt)
	entry.AddUsage(TokenUsage{
		InputTokens:  7,
		OutputTokens: 9,
		TotalTokens:  30,
	}, CostState{EstimatedUSD: 0.10}, secondAt)

	snapshot := entry.ToSnapshot()
	if snapshot.Usage.InputTokens != 17 {
		t.Fatalf("expected input tokens 17, got %d", snapshot.Usage.InputTokens)
	}
	if snapshot.Usage.OutputTokens != 29 {
		t.Fatalf("expected output tokens 29, got %d", snapshot.Usage.OutputTokens)
	}
	if snapshot.Usage.CacheReadTokens != 3 {
		t.Fatalf("expected cache read tokens 3, got %d", snapshot.Usage.CacheReadTokens)
	}
	if snapshot.Usage.CacheWriteTokens != 2 {
		t.Fatalf("expected cache write tokens 2, got %d", snapshot.Usage.CacheWriteTokens)
	}
	if snapshot.Usage.TotalTokens != 65 {
		t.Fatalf("expected total tokens 65, got %d", snapshot.Usage.TotalTokens)
	}
	if snapshot.Cost.EstimatedUSD != 0.35 {
		t.Fatalf("expected estimated cost 0.35, got %.2f", snapshot.Cost.EstimatedUSD)
	}
	if snapshot.Cost.Status != CostStatusEstimated {
		t.Fatalf("expected estimated cost status to persist, got %q", snapshot.Cost.Status)
	}
	if !snapshot.UpdatedAt.Equal(secondAt) {
		t.Fatalf("expected updated at %s, got %s", secondAt, snapshot.UpdatedAt)
	}
}

func TestEntryMarkAutoReset(t *testing.T) {
	createdAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	resetAt := createdAt.Add(3 * time.Minute)
	entry := NewEntry("key", "session", SessionSource{}, createdAt)

	entry.MarkAutoReset("idle", true, resetAt)

	snapshot := entry.ToSnapshot()
	if !snapshot.AutoReset.WasAutoReset {
		t.Fatalf("expected auto reset flag")
	}
	if snapshot.AutoReset.Reason != "idle" {
		t.Fatalf("expected idle reset reason, got %q", snapshot.AutoReset.Reason)
	}
	if !snapshot.AutoReset.ResetHadActivity {
		t.Fatalf("expected reset to record prior activity")
	}
	if snapshot.AutoReset.MarkedAt == nil || !snapshot.AutoReset.MarkedAt.Equal(resetAt) {
		t.Fatalf("expected reset marked at %s, got %v", resetAt, snapshot.AutoReset.MarkedAt)
	}
	if !snapshot.UpdatedAt.Equal(resetAt) {
		t.Fatalf("expected updated at %s, got %s", resetAt, snapshot.UpdatedAt)
	}
}

func TestEntrySuspendAndResumePendingTransitions(t *testing.T) {
	createdAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	resumeAt := createdAt.Add(1 * time.Minute)
	clearAt := createdAt.Add(2 * time.Minute)
	secondResumeAt := createdAt.Add(3 * time.Minute)
	suspendAt := createdAt.Add(4 * time.Minute)
	entry := NewEntry("key", "session", SessionSource{}, createdAt)

	if !entry.MarkResumePending("restart_timeout", resumeAt) {
		t.Fatalf("expected resume pending to be marked")
	}
	snapshot := entry.ToSnapshot()
	if snapshot.Suspended {
		t.Fatalf("did not expect resume pending to suspend the entry")
	}
	if !snapshot.Resume.Pending {
		t.Fatalf("expected resume pending")
	}
	if snapshot.Resume.Reason != "restart_timeout" {
		t.Fatalf("expected restart timeout reason, got %q", snapshot.Resume.Reason)
	}
	if snapshot.Resume.MarkedAt == nil || !snapshot.Resume.MarkedAt.Equal(resumeAt) {
		t.Fatalf("expected resume marked at %s, got %v", resumeAt, snapshot.Resume.MarkedAt)
	}

	if !entry.ClearResumePending(clearAt) {
		t.Fatalf("expected resume pending to be cleared")
	}
	snapshot = entry.ToSnapshot()
	if snapshot.Resume.Pending || snapshot.Resume.Reason != "" || snapshot.Resume.MarkedAt != nil {
		t.Fatalf("expected resume state to be cleared, got %+v", snapshot.Resume)
	}
	if !snapshot.UpdatedAt.Equal(clearAt) {
		t.Fatalf("expected updated at %s, got %s", clearAt, snapshot.UpdatedAt)
	}

	if !entry.MarkResumePending("restart_timeout", secondResumeAt) {
		t.Fatalf("expected second resume pending mark")
	}
	entry.Suspend(suspendAt)
	snapshot = entry.ToSnapshot()
	if !snapshot.Suspended {
		t.Fatalf("expected suspended entry")
	}
	if snapshot.Resume.Pending {
		t.Fatalf("expected suspend to clear resume pending")
	}
	if !snapshot.UpdatedAt.Equal(suspendAt) {
		t.Fatalf("expected updated at %s, got %s", suspendAt, snapshot.UpdatedAt)
	}
	if entry.MarkResumePending("restart_timeout", suspendAt.Add(time.Minute)) {
		t.Fatalf("expected suspended entry to reject resume pending")
	}
	snapshot = entry.ToSnapshot()
	if snapshot.Resume.Pending {
		t.Fatalf("expected suspended entry to remain non-resumable")
	}
}

func TestSnapshotAndCloneDefensiveCopy(t *testing.T) {
	createdAt := time.Date(2026, 6, 28, 1, 0, 0, 0, time.UTC)
	entry := NewEntry("key", "session", SessionSource{
		Platform: "slack",
		ChatID:   "chat-1",
		RoutingHints: map[string]string{
			"workspace": "workspace-1",
		},
	}, createdAt)

	snapshot := entry.ToSnapshot()
	snapshot.Source.RoutingHints["workspace"] = "mutated"
	snapshot.Source.RoutingHints["new"] = "value"
	if snapshot.AutoReset.MarkedAt != nil {
		t.Fatalf("did not expect auto reset marked at in fresh snapshot")
	}

	fresh := entry.ToSnapshot()
	if fresh.Source.RoutingHints["workspace"] != "workspace-1" {
		t.Fatalf("expected original routing hint to survive snapshot mutation, got %q", fresh.Source.RoutingHints["workspace"])
	}
	if _, ok := fresh.Source.RoutingHints["new"]; ok {
		t.Fatalf("did not expect new routing hint to leak back to entry")
	}

	clone := entry.Clone()
	clone.Suspend(createdAt.Add(time.Minute))
	cloneSnapshot := clone.ToSnapshot()
	originalSnapshot := entry.ToSnapshot()
	if !cloneSnapshot.Suspended {
		t.Fatalf("expected clone to be independently mutable")
	}
	if originalSnapshot.Suspended {
		t.Fatalf("did not expect clone mutation to suspend original")
	}
	cloneSnapshot.Source.RoutingHints["workspace"] = "clone-mutated"
	if entry.ToSnapshot().Source.RoutingHints["workspace"] != "workspace-1" {
		t.Fatalf("expected clone snapshot mutation not to affect original")
	}
}
