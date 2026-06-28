package filehistory_test

import (
	"testing"
	"time"

	"awaken/src/filehistory"
)

func TestTrackEditAddsBackupToLatestSnapshotOnce(t *testing.T) {
	state := filehistory.New()
	state.MakeSnapshot("message-1", nil)

	backupFileName := "file-a@v1"
	added := state.TrackEdit("file-a.txt", filehistory.FileHistoryBackup{
		BackupFileName: &backupFileName,
		Version:        1,
		BackupTime:     time.Unix(11, 0).UTC(),
	})
	if !added {
		t.Fatal("TrackEdit returned false for a new file")
	}

	backupFileName = "mutated-by-caller"
	overwriteName := "file-a@v2"
	overwrote := state.TrackEdit("file-a.txt", filehistory.FileHistoryBackup{
		BackupFileName: &overwriteName,
		Version:        2,
		BackupTime:     time.Unix(12, 0).UTC(),
	})
	if overwrote {
		t.Fatal("TrackEdit overwrote an existing backup in the latest snapshot")
	}

	latest, ok := state.LatestSnapshot()
	if !ok {
		t.Fatal("LatestSnapshot returned false")
	}
	backup := latest.TrackedFileBackups["file-a.txt"]
	if backup.Version != 1 {
		t.Fatalf("backup version = %d, want 1", backup.Version)
	}
	if backup.BackupFileName == nil || *backup.BackupFileName != "file-a@v1" {
		t.Fatalf("backup file name = %v, want file-a@v1", backup.BackupFileName)
	}

	trackedFiles := state.TrackedFiles()
	if _, ok := trackedFiles["file-a.txt"]; !ok {
		t.Fatal("file-a.txt was not added to tracked files")
	}
}

func TestMakeSnapshotInheritsAndMergesTrackedBackups(t *testing.T) {
	state := filehistory.New()
	state.MakeSnapshot("message-1", nil)

	fileAName := "file-a@v1"
	state.TrackEdit("file-a.txt", filehistory.FileHistoryBackup{
		BackupFileName: &fileAName,
		Version:        1,
		BackupTime:     time.Unix(21, 0).UTC(),
	})
	state.TrackEdit("file-b.txt", filehistory.FileHistoryBackup{
		BackupFileName: nil,
		Version:        1,
		BackupTime:     time.Unix(22, 0).UTC(),
	})

	fileAChangedName := "file-a@v2"
	untrackedName := "untracked@v1"
	snapshot := state.MakeSnapshot("message-2", map[string]filehistory.FileHistoryBackup{
		"file-a.txt": {
			BackupFileName: &fileAChangedName,
			Version:        2,
			BackupTime:     time.Unix(23, 0).UTC(),
		},
		"untracked.txt": {
			BackupFileName: &untrackedName,
			Version:        1,
			BackupTime:     time.Unix(24, 0).UTC(),
		},
	})
	fileAChangedName = "mutated-by-caller"

	if state.SnapshotSequence() != 2 {
		t.Fatalf("snapshot sequence = %d, want 2", state.SnapshotSequence())
	}
	if snapshot.Timestamp.IsZero() {
		t.Fatal("snapshot timestamp is zero")
	}

	fileABackup := snapshot.TrackedFileBackups["file-a.txt"]
	if fileABackup.Version != 2 {
		t.Fatalf("file-a version = %d, want 2", fileABackup.Version)
	}
	if fileABackup.BackupFileName == nil || *fileABackup.BackupFileName != "file-a@v2" {
		t.Fatalf("file-a backup name = %v, want file-a@v2", fileABackup.BackupFileName)
	}

	fileBBackup := snapshot.TrackedFileBackups["file-b.txt"]
	if fileBBackup.Version != 1 {
		t.Fatalf("file-b version = %d, want inherited version 1", fileBBackup.Version)
	}
	if fileBBackup.BackupFileName != nil {
		t.Fatalf("file-b backup file name = %v, want nil", *fileBBackup.BackupFileName)
	}

	if _, ok := snapshot.TrackedFileBackups["untracked.txt"]; ok {
		t.Fatal("MakeSnapshot included a backup for an untracked file")
	}

	latest, ok := state.LatestSnapshot()
	if !ok {
		t.Fatal("LatestSnapshot returned false")
	}
	latestFileA := latest.TrackedFileBackups["file-a.txt"]
	if latestFileA.BackupFileName == nil || *latestFileA.BackupFileName != "file-a@v2" {
		t.Fatalf("latest file-a backup name = %v, want file-a@v2", latestFileA.BackupFileName)
	}
}

func TestSnapshotCapDoesNotResetSequenceAndFindsRetainedSnapshots(t *testing.T) {
	state := filehistory.New(filehistory.WithMaxSnapshots(2))

	state.MakeSnapshot("message-1", nil)
	state.MakeSnapshot("message-2", nil)
	state.MakeSnapshot("message-3", nil)

	snapshots := state.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snapshots))
	}
	if snapshots[0].MessageID != "message-2" || snapshots[1].MessageID != "message-3" {
		t.Fatalf("retained snapshots = %q, %q; want message-2, message-3", snapshots[0].MessageID, snapshots[1].MessageID)
	}
	if state.SnapshotSequence() != 3 {
		t.Fatalf("snapshot sequence = %d, want 3", state.SnapshotSequence())
	}
	if state.CanRestore("message-1") {
		t.Fatal("CanRestore returned true for an evicted snapshot")
	}
	if !state.CanRestore("message-3") {
		t.Fatal("CanRestore returned false for a retained snapshot")
	}

	found, ok := state.FindSnapshot("message-2")
	if !ok {
		t.Fatal("FindSnapshot returned false for message-2")
	}
	if found.MessageID != "message-2" {
		t.Fatalf("found message ID = %q, want message-2", found.MessageID)
	}
}

func TestReturnedCollectionsCannotPolluteState(t *testing.T) {
	state := filehistory.New()
	state.MakeSnapshot("message-1", nil)
	backupFileName := "file-a@v1"
	state.TrackEdit("file-a.txt", filehistory.FileHistoryBackup{
		BackupFileName: &backupFileName,
		Version:        1,
		BackupTime:     time.Unix(41, 0).UTC(),
	})

	trackedFiles := state.TrackedFiles()
	delete(trackedFiles, "file-a.txt")
	trackedFiles["injected.txt"] = struct{}{}

	snapshots := state.Snapshots()
	snapshots[0].TrackedFileBackups["injected.txt"] = filehistory.FileHistoryBackup{
		Version:    99,
		BackupTime: time.Unix(42, 0).UTC(),
	}
	exposedBackup := snapshots[0].TrackedFileBackups["file-a.txt"]
	*exposedBackup.BackupFileName = "mutated-through-returned-pointer"

	currentTrackedFiles := state.TrackedFiles()
	if _, ok := currentTrackedFiles["file-a.txt"]; !ok {
		t.Fatal("mutating returned tracked files removed an internal tracked file")
	}
	if _, ok := currentTrackedFiles["injected.txt"]; ok {
		t.Fatal("mutating returned tracked files injected an internal tracked file")
	}

	latest, ok := state.LatestSnapshot()
	if !ok {
		t.Fatal("LatestSnapshot returned false")
	}
	if _, ok := latest.TrackedFileBackups["injected.txt"]; ok {
		t.Fatal("mutating returned snapshots injected an internal backup")
	}
	currentBackup := latest.TrackedFileBackups["file-a.txt"]
	if currentBackup.BackupFileName == nil || *currentBackup.BackupFileName != "file-a@v1" {
		t.Fatalf("internal backup file name = %v, want file-a@v1", currentBackup.BackupFileName)
	}
}
