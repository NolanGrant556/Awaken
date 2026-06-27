package permissionstats

import "sort"

// Stats holds the permission prompt attribution counters for a session.
type Stats struct {
	PromptCount                       int
	PromptCountAtLastCommit           int
	PermissionPromptCount             int
	PermissionPromptCountAtLastCommit int
	EscapeCount                       int
	EscapeCountAtLastCommit           int
}

// Delta holds counts accumulated since the most recent commit baseline.
type Delta struct {
	PromptCount           int
	PermissionPromptCount int
	EscapeCount           int
}

// SinceLastCommit returns the counts accumulated after the last commit baseline.
func (s Stats) SinceLastCommit() Delta {
	return Delta{
		PromptCount:           s.PromptCount - s.PromptCountAtLastCommit,
		PermissionPromptCount: s.PermissionPromptCount - s.PermissionPromptCountAtLastCommit,
		EscapeCount:           s.EscapeCount - s.EscapeCountAtLastCommit,
	}
}

// Delta returns the counts accumulated after the last commit baseline.
func (s Stats) Delta() Delta {
	return s.SinceLastCommit()
}

// Snapshot is a full-state dump that can restore a Tracker later.
type Snapshot struct {
	Stats
	PermissionPromptToolUseIDs []string
}

// Tracker maintains permission prompt attribution counters.
type Tracker struct {
	stats                      Stats
	permissionPromptToolUseIDs map[string]struct{}
}

// NewTracker returns an empty permission stats tracker.
func NewTracker() *Tracker {
	return &Tracker{
		permissionPromptToolUseIDs: make(map[string]struct{}),
	}
}

// RecordPrompt increments the prompt counter and returns the updated stats.
func (t *Tracker) RecordPrompt() Stats {
	t.stats.PromptCount++
	return t.stats
}

// RecordPermissionPrompt increments the permission prompt counter once per toolUseID.
func (t *Tracker) RecordPermissionPrompt(toolUseID string) (Stats, bool) {
	t.ensurePermissionPromptToolUseIDs()
	if _, ok := t.permissionPromptToolUseIDs[toolUseID]; ok {
		return t.stats, false
	}

	t.permissionPromptToolUseIDs[toolUseID] = struct{}{}
	t.stats.PermissionPromptCount++
	return t.stats, true
}

// RecordEscape increments the escape counter and returns the updated stats.
func (t *Tracker) RecordEscape() Stats {
	t.stats.EscapeCount++
	return t.stats
}

// MarkCommitBaseline records the current counts as the last commit baseline.
func (t *Tracker) MarkCommitBaseline() Stats {
	t.stats.PromptCountAtLastCommit = t.stats.PromptCount
	t.stats.PermissionPromptCountAtLastCommit = t.stats.PermissionPromptCount
	t.stats.EscapeCountAtLastCommit = t.stats.EscapeCount
	return t.stats
}

// Stats returns a copy of the current counters.
func (t *Tracker) Stats() Stats {
	return t.stats
}

// SinceLastCommit returns the counts accumulated after the last commit baseline.
func (t *Tracker) SinceLastCommit() Delta {
	return t.stats.SinceLastCommit()
}

// Delta returns the counts accumulated after the last commit baseline.
func (t *Tracker) Delta() Delta {
	return t.SinceLastCommit()
}

// Snapshot returns a full-state snapshot with copied internal collections.
func (t *Tracker) Snapshot() Snapshot {
	toolUseIDs := make([]string, 0, len(t.permissionPromptToolUseIDs))
	for toolUseID := range t.permissionPromptToolUseIDs {
		toolUseIDs = append(toolUseIDs, toolUseID)
	}
	sort.Strings(toolUseIDs)

	return Snapshot{
		Stats:                      t.stats,
		PermissionPromptToolUseIDs: toolUseIDs,
	}
}

// RestoreFromSnapshots restores a tracker from the last full-state snapshot.
func RestoreFromSnapshots(snapshots []Snapshot) *Tracker {
	tracker := NewTracker()
	if len(snapshots) == 0 {
		return tracker
	}

	lastSnapshot := snapshots[len(snapshots)-1]
	tracker.stats = lastSnapshot.Stats
	tracker.permissionPromptToolUseIDs = make(map[string]struct{}, len(lastSnapshot.PermissionPromptToolUseIDs))
	for _, toolUseID := range lastSnapshot.PermissionPromptToolUseIDs {
		tracker.permissionPromptToolUseIDs[toolUseID] = struct{}{}
	}
	return tracker
}

func (t *Tracker) ensurePermissionPromptToolUseIDs() {
	if t.permissionPromptToolUseIDs == nil {
		t.permissionPromptToolUseIDs = make(map[string]struct{})
	}
}
