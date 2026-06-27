package permissionstats

import (
	"reflect"
	"testing"
)

func TestTrackerRecordsStatsAndDelta(t *testing.T) {
	tracker := NewTracker()

	tracker.RecordPrompt()
	tracker.RecordPrompt()
	if _, counted := tracker.RecordPermissionPrompt("tool-1"); !counted {
		t.Fatalf("expected first permission prompt to be counted")
	}
	tracker.RecordEscape()

	wantStats := Stats{
		PromptCount:           2,
		PermissionPromptCount: 1,
		EscapeCount:           1,
	}
	if got := tracker.Stats(); got != wantStats {
		t.Fatalf("unexpected stats: got %+v want %+v", got, wantStats)
	}

	wantDelta := Delta{
		PromptCount:           2,
		PermissionPromptCount: 1,
		EscapeCount:           1,
	}
	if got := tracker.SinceLastCommit(); got != wantDelta {
		t.Fatalf("unexpected delta: got %+v want %+v", got, wantDelta)
	}
	if got := tracker.Delta(); got != wantDelta {
		t.Fatalf("unexpected Delta alias result: got %+v want %+v", got, wantDelta)
	}
}

func TestMarkCommitBaselineUsesCurrentCounts(t *testing.T) {
	tracker := NewTracker()
	tracker.RecordPrompt()
	tracker.RecordPermissionPrompt("tool-1")
	tracker.RecordEscape()

	tracker.MarkCommitBaseline()
	if got := tracker.SinceLastCommit(); got != (Delta{}) {
		t.Fatalf("expected empty delta after baseline, got %+v", got)
	}

	tracker.RecordPrompt()
	tracker.RecordPermissionPrompt("tool-2")
	tracker.RecordEscape()

	wantStats := Stats{
		PromptCount:                       2,
		PromptCountAtLastCommit:           1,
		PermissionPromptCount:             2,
		PermissionPromptCountAtLastCommit: 1,
		EscapeCount:                       2,
		EscapeCountAtLastCommit:           1,
	}
	if got := tracker.Stats(); got != wantStats {
		t.Fatalf("unexpected stats after new activity: got %+v want %+v", got, wantStats)
	}

	wantDelta := Delta{
		PromptCount:           1,
		PermissionPromptCount: 1,
		EscapeCount:           1,
	}
	if got := tracker.SinceLastCommit(); got != wantDelta {
		t.Fatalf("unexpected delta after new activity: got %+v want %+v", got, wantDelta)
	}
}

func TestRecordPermissionPromptDedupesByToolUseID(t *testing.T) {
	var tracker Tracker

	if _, counted := tracker.RecordPermissionPrompt("tool-1"); !counted {
		t.Fatalf("expected first tool-1 prompt to be counted")
	}
	if _, counted := tracker.RecordPermissionPrompt("tool-1"); counted {
		t.Fatalf("expected duplicate tool-1 prompt to be ignored")
	}
	if _, counted := tracker.RecordPermissionPrompt("tool-2"); !counted {
		t.Fatalf("expected new tool-2 prompt to be counted")
	}

	if got := tracker.Stats().PermissionPromptCount; got != 2 {
		t.Fatalf("unexpected permission prompt count: got %d want 2", got)
	}
}

func TestSnapshotReturnsCopiedInternalCollections(t *testing.T) {
	tracker := NewTracker()
	tracker.RecordPermissionPrompt("tool-1")
	tracker.RecordPermissionPrompt("tool-2")

	snapshot := tracker.Snapshot()
	if !reflect.DeepEqual(snapshot.PermissionPromptToolUseIDs, []string{"tool-1", "tool-2"}) {
		t.Fatalf("unexpected tool use IDs: %+v", snapshot.PermissionPromptToolUseIDs)
	}

	snapshot.PermissionPromptToolUseIDs[0] = "changed"
	if _, counted := tracker.RecordPermissionPrompt("tool-1"); counted {
		t.Fatalf("mutating snapshot slice should not affect tracker dedupe state")
	}

	nextSnapshot := tracker.Snapshot()
	if !reflect.DeepEqual(nextSnapshot.PermissionPromptToolUseIDs, []string{"tool-1", "tool-2"}) {
		t.Fatalf("unexpected tool use IDs after external mutation: %+v", nextSnapshot.PermissionPromptToolUseIDs)
	}
}

func TestRestoreFromSnapshotsUsesOnlyLastSnapshot(t *testing.T) {
	snapshots := []Snapshot{
		{
			Stats: Stats{
				PromptCount:           10,
				PermissionPromptCount: 10,
				EscapeCount:           10,
			},
			PermissionPromptToolUseIDs: []string{"old-tool"},
		},
		{
			Stats: Stats{
				PromptCount:                       3,
				PromptCountAtLastCommit:           1,
				PermissionPromptCount:             2,
				PermissionPromptCountAtLastCommit: 1,
				EscapeCount:                       4,
				EscapeCountAtLastCommit:           2,
			},
			PermissionPromptToolUseIDs: []string{"new-tool"},
		},
	}

	tracker := RestoreFromSnapshots(snapshots)

	wantStats := snapshots[1].Stats
	if got := tracker.Stats(); got != wantStats {
		t.Fatalf("unexpected restored stats: got %+v want %+v", got, wantStats)
	}

	wantDelta := Delta{
		PromptCount:           2,
		PermissionPromptCount: 1,
		EscapeCount:           2,
	}
	if got := tracker.SinceLastCommit(); got != wantDelta {
		t.Fatalf("unexpected restored delta: got %+v want %+v", got, wantDelta)
	}

	if _, counted := tracker.RecordPermissionPrompt("new-tool"); counted {
		t.Fatalf("expected toolUseID from last snapshot to remain deduped")
	}
	if _, counted := tracker.RecordPermissionPrompt("old-tool"); !counted {
		t.Fatalf("expected toolUseID from older snapshot to be ignored during restore")
	}
	if got := tracker.Stats().PermissionPromptCount; got != 3 {
		t.Fatalf("unexpected count after recording old-tool: got %d want 3", got)
	}
}

func TestRestoreFromSnapshotsCopiesLastSnapshotCollections(t *testing.T) {
	snapshots := []Snapshot{
		{
			Stats:                      Stats{PermissionPromptCount: 1},
			PermissionPromptToolUseIDs: []string{"tool-1"},
		},
	}

	tracker := RestoreFromSnapshots(snapshots)
	snapshots[0].PermissionPromptToolUseIDs[0] = "changed"

	if _, counted := tracker.RecordPermissionPrompt("tool-1"); counted {
		t.Fatalf("mutating source snapshot should not affect restored dedupe state")
	}
	if _, counted := tracker.RecordPermissionPrompt("changed"); !counted {
		t.Fatalf("expected mutated source ID to be unrelated to restored tracker")
	}
}

func TestRestoreFromSnapshotsWithoutSnapshotsReturnsEmptyTracker(t *testing.T) {
	tracker := RestoreFromSnapshots(nil)

	if got := tracker.Stats(); got != (Stats{}) {
		t.Fatalf("unexpected stats for empty restore: %+v", got)
	}
	if got := tracker.SinceLastCommit(); got != (Delta{}) {
		t.Fatalf("unexpected delta for empty restore: %+v", got)
	}
}
