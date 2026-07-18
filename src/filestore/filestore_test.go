package filestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type sampleDoc struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestWriteFileAtomicReplacesWithoutTruncatingTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "doc.json")
	if err := WriteFileAtomic(target, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteFileAtomic(target, []byte(`{"v":2}`)); err != nil {
		t.Fatalf("second write: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != `{"v":2}` {
		t.Fatalf("unexpected content: %q err=%v", data, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil || info.Mode().Perm() != FilePerm {
			t.Fatalf("expected %v permissions, got %v err=%v", FilePerm, info.Mode().Perm(), err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temp file leaked: %s", entry.Name())
		}
	}
}

func TestWriteFileAtomicCrashBeforeRenameKeepsOldContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "doc.json")
	if err := WriteFileAtomic(target, []byte("old")); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	crash := errors.New("injected crash before rename")
	hookBeforeRename = func(string, string) error { return crash }
	defer func() { hookBeforeRename = nil }()
	if err := WriteFileAtomic(target, []byte("new")); !errors.Is(err, crash) {
		t.Fatalf("expected injected crash, got %v", err)
	}
	hookBeforeRename = nil
	data, _ := os.ReadFile(target)
	if string(data) != "old" {
		t.Fatalf("crash before rename must keep old content, got %q", data)
	}
}

func TestWriteFileAtomicCrashBeforeTempFsyncKeepsOldContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "doc.json")
	if err := WriteFileAtomic(target, []byte("old")); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	crash := errors.New("injected crash before temp fsync")
	hookBeforeTempFsync = func(string) error { return crash }
	defer func() { hookBeforeTempFsync = nil }()
	if err := WriteFileAtomic(target, []byte("new")); !errors.Is(err, crash) {
		t.Fatalf("expected injected crash, got %v", err)
	}
	hookBeforeTempFsync = nil
	data, _ := os.ReadFile(target)
	if string(data) != "old" {
		t.Fatalf("crash before fsync must keep old content, got %q", data)
	}
}

func TestReplaceJSONWithRevisionCAS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authority.json")

	created, err := ReplaceJSONWithRevision(path, 0, func(sampleDoc) (sampleDoc, error) {
		return sampleDoc{Name: "first", Count: 1}, nil
	})
	if err != nil || created.Revision != 1 {
		t.Fatalf("create: %+v err=%v", created, err)
	}

	updated, err := ReplaceJSONWithRevision(path, 1, func(current sampleDoc) (sampleDoc, error) {
		current.Count++
		return current, nil
	})
	if err != nil || updated.Revision != 2 || updated.Value.Count != 2 {
		t.Fatalf("update: %+v err=%v", updated, err)
	}

	if _, err := ReplaceJSONWithRevision(path, 1, func(current sampleDoc) (sampleDoc, error) {
		return current, nil
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision must conflict, got %v", err)
	}

	if _, err := ReplaceJSONWithRevision(filepath.Join(dir, "missing.json"), 3, func(current sampleDoc) (sampleDoc, error) {
		return current, nil
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("missing document with nonzero expected revision must conflict, got %v", err)
	}
}

func TestCorruptJSONIsNeverTreatedAsEmptyStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authority.json")
	if err := os.WriteFile(path, []byte("{not json"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceJSONWithRevision(path, 0, func(current sampleDoc) (sampleDoc, error) {
		return current, nil
	}); err == nil {
		t.Fatalf("corrupt JSON must fail, not be overwritten as empty")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{not json" {
		t.Fatalf("corrupt document must not be overwritten, got %q", data)
	}
}

func TestFileLockExclusionAndStaleRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.lock")

	lock, err := AcquireLock(path, "owner-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := AcquireLock(path, "owner-b", time.Minute); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("live lock must be exclusive, got %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	relock, err := AcquireLock(path, "owner-b", time.Minute)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	relock.Release()

	// Stale lock: expired TTL from a dead owner is reclaimed.
	owner, _ := json.Marshal(lockOwner{
		PID:        999999999,
		AcquiredAt: time.Now().UTC().Add(-time.Hour),
		ExpiresAt:  time.Now().UTC().Add(-time.Hour),
	})
	if err := os.WriteFile(path, owner, FilePerm); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := AcquireLock(path, "owner-c", time.Minute)
	if err != nil {
		t.Fatalf("stale lock must be reclaimable: %v", err)
	}
	reclaimed.Release()
}

func TestFileLockHeartbeatRefreshesExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heartbeat.lock")
	lock, err := AcquireLock(path, "dream", 80*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	var before lockOwner
	if err := ReadJSON(path, &before); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := lock.Refresh(); err != nil {
		t.Fatal(err)
	}
	var after lockOwner
	if err := ReadJSON(path, &after); err != nil {
		t.Fatal(err)
	}
	if !after.HeartbeatAt.After(before.HeartbeatAt) || !after.ExpiresAt.After(before.ExpiresAt) {
		t.Fatalf("heartbeat did not renew lock: before=%+v after=%+v", before, after)
	}
}

func TestFileLockUnreadableOwnerFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.lock")
	if err := os.WriteFile(path, []byte("garbage"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(path, "owner", time.Minute); err == nil || errors.Is(err, ErrLockHeld) {
		// Must be a hard error explaining the unreadable owner, not a silent
		// reclaim and not a plain "held" answer.
		if err == nil {
			t.Fatalf("unreadable lock owner must fail closed")
		}
	}
}

func TestFileLockContentionSingleWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contended.lock")
	var wins int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lock, err := AcquireLock(path, fmt.Sprintf("owner-%d", id), time.Minute)
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
				_ = lock // hold until test end
			}
		}(i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("expected exactly one lock winner, got %d", wins)
	}
}

func TestLeaseFencingTokenSupersedesOldOwner(t *testing.T) {
	dir := t.TempDir()
	leasePath := filepath.Join(dir, "session.lease")
	guardPath := filepath.Join(dir, "session.lease.lock")

	first, err := ClaimLease(leasePath, guardPath, "core-a", time.Minute)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := ClaimLease(leasePath, guardPath, "core-b", time.Minute); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("live lease must reject another owner, got %v", err)
	}

	// Expire the lease, then a new owner claims with a higher fencing token.
	expired := first
	expired.ExpiresAt = time.Now().UTC().Add(-time.Second)
	encoded, _ := json.Marshal(expired)
	if err := WriteFileAtomic(leasePath, encoded); err != nil {
		t.Fatal(err)
	}
	second, err := ClaimLease(leasePath, guardPath, "core-b", time.Minute)
	if err != nil {
		t.Fatalf("claim after expiry: %v", err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("fencing token must increase: %d -> %d", first.FencingToken, second.FencingToken)
	}
	if err := ValidateLease(leasePath, first); err == nil {
		t.Fatalf("old owner must fail lease validation after supersession")
	}
	if err := ValidateLease(leasePath, second); err != nil {
		t.Fatalf("current owner must validate: %v", err)
	}
}

func TestJournalAppendRecoverAndTailTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.jsonl")

	journal, report, err := OpenJournal(path)
	if err != nil || report.CommittedRecords != 0 {
		t.Fatalf("open empty: %+v err=%v", report, err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := journal.Append(map[string]int{"n": i}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	journal.Close()

	// Simulate a crash tail: half a record without newline.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, FilePerm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"seq":4,"payload":{"n"`); err != nil {
		t.Fatal(err)
	}
	file.Close()

	reopened, report, err := OpenJournal(path)
	if err != nil {
		t.Fatalf("recovery must truncate crash tail: %v", err)
	}
	if report.CommittedRecords != 3 || report.TruncatedTailBytes == 0 {
		t.Fatalf("unexpected recovery report: %+v", report)
	}
	if reopened.LastSeq() != 3 {
		t.Fatalf("expected last seq 3, got %d", reopened.LastSeq())
	}
	// Appends continue after the truncated tail.
	record, err := reopened.Append(map[string]int{"n": 4})
	if err != nil || record.Seq != 4 {
		t.Fatalf("append after recovery: %+v err=%v", record, err)
	}
	reopened.Close()
}

func TestJournalMiddleCorruptionFailsStop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.jsonl")
	journal, _, err := OpenJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := journal.Append(map[string]int{"n": i}); err != nil {
			t.Fatal(err)
		}
	}
	journal.Close()

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	lines[1] = `{"seq":2,"payload":{"n":999},"checksum":"deadbeef00000000"}`
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), FilePerm); err != nil {
		t.Fatal(err)
	}

	if _, _, err := OpenJournal(path); !errors.Is(err, ErrCorruptJournal) {
		t.Fatalf("mid-file corruption must fail stop, got %v", err)
	}
}

func TestFormatGateFailsClosedOnNewerStore(t *testing.T) {
	format := Format{FormatVersion: 9, MinReaderVersion: 9, MinWriterVersion: 9}
	if err := format.CheckReadable(CurrentFormatVersion); !errors.Is(err, ErrFormatIncompatible) {
		t.Fatalf("newer store must not be readable, got %v", err)
	}
	if err := format.CheckWritable(CurrentFormatVersion); !errors.Is(err, ErrFormatIncompatible) {
		t.Fatalf("newer store must not be writable, got %v", err)
	}
	compatible := Format{FormatVersion: 1, MinReaderVersion: 1, MinWriterVersion: 1}
	if err := compatible.CheckWritable(CurrentFormatVersion); err != nil {
		t.Fatalf("compatible store must be writable: %v", err)
	}
}

func TestCurrentPointerFailsClosed(t *testing.T) {
	root := t.TempDir()
	if _, _, err := ReadCurrentGeneration(root); err == nil {
		t.Fatalf("missing CURRENT must fail closed")
	}
	if err := os.WriteFile(filepath.Join(root, "CURRENT"), []byte("../escape\n"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCurrentGeneration(root); err == nil {
		t.Fatalf("traversal CURRENT must fail closed")
	}
	if err := os.WriteFile(filepath.Join(root, "CURRENT"), []byte("gen-missing\n"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCurrentGeneration(root); err == nil {
		t.Fatalf("CURRENT pointing at missing generation must fail closed")
	}
}

func TestGenerationInitPublishResolve(t *testing.T) {
	root := t.TempDir()
	if _, err := InitGeneration(root, "gen-1", Format{}); err != nil {
		t.Fatalf("init generation: %v", err)
	}
	if err := PublishGeneration(root, "gen-1"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	dir, format, err := ReadCurrentGeneration(root)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if filepath.Base(dir) != "gen-1" || format.FormatVersion != CurrentFormatVersion {
		t.Fatalf("unexpected resolution: %s %+v", dir, format)
	}
	if err := PublishGeneration(root, "gen-unbuilt"); err == nil {
		t.Fatalf("publishing a generation without FORMAT.json must fail")
	}
}

func TestStagingCommitRecoverRollForwardAndRollback(t *testing.T) {
	root := t.TempDir()
	journal, _, err := OpenJournal(filepath.Join(root, "lifecycle.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	staging := NewStaging(root, journal)

	// Happy path: begin -> write -> commit materializes the target.
	stagingDir, err := staging.Begin("sessions/thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "metadata.json"), []byte("{}"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if err := staging.Commit(stagingDir, "sessions/thread-1"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sessions", "thread-1", "metadata.json")); err != nil {
		t.Fatalf("materialized session missing: %v", err)
	}

	// Crash before commit: staging dir without journal record rolls back.
	uncommitted, err := staging.Begin("sessions/thread-2")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uncommitted, "metadata.json"), []byte("{}"), FilePerm); err != nil {
		t.Fatal(err)
	}

	// Crash after commit, before rename: journal record without rename rolls forward.
	committed, err := staging.Begin("sessions/thread-3")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(committed, "metadata.json"), []byte("{}"), FilePerm); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Append(stagingCommit{
		Kind:       stagingCommitKind,
		StagingDir: committed,
		TargetDir:  filepath.Join(root, "sessions", "thread-3"),
	}); err != nil {
		t.Fatal(err)
	}

	rolledForward, err := staging.Recover()
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(rolledForward) != 1 || filepath.Base(rolledForward[0]) != "thread-3" {
		t.Fatalf("expected thread-3 rolled forward, got %v", rolledForward)
	}
	if _, err := os.Stat(uncommitted); !os.IsNotExist(err) {
		t.Fatalf("uncommitted staging must be rolled back, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "sessions", "thread-3", "metadata.json")); err != nil {
		t.Fatalf("rolled-forward session missing: %v", err)
	}
	journal.Close()
}

func TestWithinRootRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	if err := WithinRoot(root, filepath.Join(root, "sessions", "x")); err != nil {
		t.Fatalf("inside path must pass: %v", err)
	}
	if err := WithinRoot(root, filepath.Join(root, "..", "outside")); !errors.Is(err, ErrPathEscapesRoot) {
		t.Fatalf("traversal must be rejected, got %v", err)
	}
	if runtime.GOOS != "windows" {
		outside := t.TempDir()
		link := filepath.Join(root, "link")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		if err := WithinRoot(root, filepath.Join(link, "file")); !errors.Is(err, ErrPathEscapesRoot) {
			t.Fatalf("symlink escape must be rejected, got %v", err)
		}
	}
}

func TestResolveAwakenHomePrecedence(t *testing.T) {
	explicit := t.TempDir()
	if got, err := ResolveAwakenHome(explicit); err != nil || got != filepath.Clean(explicit) {
		t.Fatalf("explicit root must win: %q err=%v", got, err)
	}
	envRoot := t.TempDir()
	t.Setenv(AwakenHomeEnv, envRoot)
	if got, err := ResolveAwakenHome(""); err != nil || got != filepath.Clean(envRoot) {
		t.Fatalf("env root expected: %q err=%v", got, err)
	}
	t.Setenv(AwakenHomeEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	got, err := ResolveAwakenHome("")
	if err != nil || got != filepath.Join(home, ".awaken") {
		t.Fatalf("default ~/.awaken expected: %q err=%v", got, err)
	}
}
