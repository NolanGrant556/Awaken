package filehistory

import "time"

const DefaultMaxSnapshots = 100

// FileHistoryBackup describes the backup metadata for one tracked file.
// BackupFileName is nil when the file did not exist at that version.
type FileHistoryBackup struct {
	BackupFileName *string
	Version        int
	BackupTime     time.Time
}

// FileHistorySnapshot records the known backups for tracked files at a message.
type FileHistorySnapshot struct {
	MessageID          string
	TrackedFileBackups map[string]FileHistoryBackup
	Timestamp          time.Time
}

// FileHistoryState is a pure in-memory state holder for file history metadata.
type FileHistoryState struct {
	snapshots        []FileHistorySnapshot
	trackedFiles     map[string]struct{}
	snapshotSequence int
	maxSnapshots     int
}

// Option customizes a FileHistoryState.
type Option func(*FileHistoryState)

// WithMaxSnapshots configures the maximum number of retained snapshots.
func WithMaxSnapshots(max int) Option {
	return func(state *FileHistoryState) {
		if max > 0 {
			state.maxSnapshots = max
		}
	}
}

// New creates an empty FileHistoryState.
func New(opts ...Option) *FileHistoryState {
	state := &FileHistoryState{
		snapshots:    []FileHistorySnapshot{},
		trackedFiles: map[string]struct{}{},
		maxSnapshots: DefaultMaxSnapshots,
	}
	for _, opt := range opts {
		opt(state)
	}
	return state
}

// SnapshotSequence returns the monotonically increasing snapshot counter.
func (state *FileHistoryState) SnapshotSequence() int {
	if state == nil {
		return 0
	}
	return state.snapshotSequence
}

// Snapshots returns a copy of the retained snapshots.
func (state *FileHistoryState) Snapshots() []FileHistorySnapshot {
	if state == nil {
		return []FileHistorySnapshot{}
	}
	return cloneSnapshots(state.snapshots)
}

// TrackedFiles returns a copy of the tracked file set.
func (state *FileHistoryState) TrackedFiles() map[string]struct{} {
	if state == nil {
		return map[string]struct{}{}
	}
	return cloneFileSet(state.trackedFiles)
}

// TrackEdit adds filePath to the tracked files and writes backup into the
// latest snapshot. It returns false when there is no snapshot or the latest
// snapshot already has a backup for filePath.
func (state *FileHistoryState) TrackEdit(filePath string, backup FileHistoryBackup) bool {
	if state == nil || len(state.snapshots) == 0 {
		return false
	}

	latestIndex := len(state.snapshots) - 1
	latest := cloneSnapshot(state.snapshots[latestIndex])
	if _, exists := latest.TrackedFileBackups[filePath]; exists {
		return false
	}

	if state.trackedFiles == nil {
		state.trackedFiles = map[string]struct{}{}
	}
	state.trackedFiles[filePath] = struct{}{}
	latest.TrackedFileBackups[filePath] = cloneBackup(backup)
	state.snapshots[latestIndex] = latest
	return true
}

// MakeSnapshot creates a new snapshot for messageID. For each currently
// tracked file, it inherits the latest snapshot backup and then applies the
// changedBackups supplied by the caller. Backups for untracked files are ignored.
func (state *FileHistoryState) MakeSnapshot(messageID string, changedBackups map[string]FileHistoryBackup) FileHistorySnapshot {
	return state.makeSnapshotAt(messageID, changedBackups, time.Now().UTC())
}

func (state *FileHistoryState) makeSnapshotAt(messageID string, changedBackups map[string]FileHistoryBackup, timestamp time.Time) FileHistorySnapshot {
	if state == nil {
		return FileHistorySnapshot{
			MessageID:          messageID,
			TrackedFileBackups: map[string]FileHistoryBackup{},
			Timestamp:          timestamp,
		}
	}
	state.ensureInitialized()

	trackedFileBackups := map[string]FileHistoryBackup{}
	latest, hasLatest := state.latestSnapshot()
	for filePath := range state.trackedFiles {
		if hasLatest {
			if inherited, ok := latest.TrackedFileBackups[filePath]; ok {
				trackedFileBackups[filePath] = inherited
			}
		}
		if changed, ok := changedBackups[filePath]; ok {
			trackedFileBackups[filePath] = cloneBackup(changed)
		}
	}

	snapshot := FileHistorySnapshot{
		MessageID:          messageID,
		TrackedFileBackups: trackedFileBackups,
		Timestamp:          timestamp,
	}
	state.snapshots = append(state.snapshots, cloneSnapshot(snapshot))
	if len(state.snapshots) > state.maxSnapshots {
		state.snapshots = state.snapshots[len(state.snapshots)-state.maxSnapshots:]
	}
	state.snapshotSequence++
	return cloneSnapshot(snapshot)
}

// CanRestore reports whether a snapshot exists for messageID.
func (state *FileHistoryState) CanRestore(messageID string) bool {
	_, ok := state.FindSnapshot(messageID)
	return ok
}

// FindSnapshot returns the most recent retained snapshot matching messageID.
func (state *FileHistoryState) FindSnapshot(messageID string) (FileHistorySnapshot, bool) {
	if state == nil {
		return FileHistorySnapshot{}, false
	}
	for i := len(state.snapshots) - 1; i >= 0; i-- {
		if state.snapshots[i].MessageID == messageID {
			return cloneSnapshot(state.snapshots[i]), true
		}
	}
	return FileHistorySnapshot{}, false
}

// LatestSnapshot returns the most recent retained snapshot.
func (state *FileHistoryState) LatestSnapshot() (FileHistorySnapshot, bool) {
	if state == nil {
		return FileHistorySnapshot{}, false
	}
	latest, ok := state.latestSnapshot()
	if !ok {
		return FileHistorySnapshot{}, false
	}
	return cloneSnapshot(latest), true
}

func (state *FileHistoryState) ensureInitialized() {
	if state.trackedFiles == nil {
		state.trackedFiles = map[string]struct{}{}
	}
	if state.snapshots == nil {
		state.snapshots = []FileHistorySnapshot{}
	}
	if state.maxSnapshots <= 0 {
		state.maxSnapshots = DefaultMaxSnapshots
	}
}

func (state *FileHistoryState) latestSnapshot() (FileHistorySnapshot, bool) {
	if len(state.snapshots) == 0 {
		return FileHistorySnapshot{}, false
	}
	return state.snapshots[len(state.snapshots)-1], true
}

func cloneSnapshots(snapshots []FileHistorySnapshot) []FileHistorySnapshot {
	cloned := make([]FileHistorySnapshot, len(snapshots))
	for i, snapshot := range snapshots {
		cloned[i] = cloneSnapshot(snapshot)
	}
	return cloned
}

func cloneSnapshot(snapshot FileHistorySnapshot) FileHistorySnapshot {
	return FileHistorySnapshot{
		MessageID:          snapshot.MessageID,
		TrackedFileBackups: cloneBackups(snapshot.TrackedFileBackups),
		Timestamp:          snapshot.Timestamp,
	}
}

func cloneBackups(backups map[string]FileHistoryBackup) map[string]FileHistoryBackup {
	if backups == nil {
		return map[string]FileHistoryBackup{}
	}
	cloned := make(map[string]FileHistoryBackup, len(backups))
	for filePath, backup := range backups {
		cloned[filePath] = cloneBackup(backup)
	}
	return cloned
}

func cloneBackup(backup FileHistoryBackup) FileHistoryBackup {
	cloned := backup
	if backup.BackupFileName != nil {
		backupFileName := *backup.BackupFileName
		cloned.BackupFileName = &backupFileName
	}
	return cloned
}

func cloneFileSet(files map[string]struct{}) map[string]struct{} {
	if files == nil {
		return map[string]struct{}{}
	}
	cloned := make(map[string]struct{}, len(files))
	for filePath := range files {
		cloned[filePath] = struct{}{}
	}
	return cloned
}
