package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestACPSessionRunReservationAndSnapshotIsolation(t *testing.T) {
	manager := NewACPSessionManager(nil, nil)
	manager.sessions["session_1"] = &ACPSessionMapping{
		ACPSessionID: "session_1",
		ThreadID:     "thread_1",
	}

	if err := manager.BeginRun("session_1"); err != nil {
		t.Fatalf("begin run: %v", err)
	}
	if err := manager.BeginRun("session_1"); !errors.Is(err, ErrACPSessionBusy) {
		t.Fatalf("expected busy reservation, got %v", err)
	}
	if err := manager.SetCurrentRun("session_1", "run_1"); err != nil {
		t.Fatalf("set current run: %v", err)
	}
	if err := manager.SetCurrentRun("session_1", "run_2"); !errors.Is(err, ErrACPSessionBusy) {
		t.Fatalf("expected active run to reject replacement, got %v", err)
	}

	snapshot, err := manager.GetSession("session_1")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.CurrentRunID = "mutated"
	actual, err := manager.GetSession("session_1")
	if err != nil {
		t.Fatal(err)
	}
	if actual.CurrentRunID != "run_1" {
		t.Fatalf("GetSession exposed mutable state: %q", actual.CurrentRunID)
	}

	manager.ClearCurrentRun("session_1", "run_2")
	actual, _ = manager.GetSession("session_1")
	if actual.CurrentRunID != "run_1" {
		t.Fatal("a stale run id cleared the active run")
	}
	manager.ClearCurrentRun("session_1", "run_1")
	actual, _ = manager.GetSession("session_1")
	if actual.CurrentRunID != "" {
		t.Fatalf("expected current run cleared, got %q", actual.CurrentRunID)
	}
}

func TestCanonicalizeCWD(t *testing.T) {
	root := t.TempDir()
	canonical, err := CanonicalizeCWD(root)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if canonical == "" {
		t.Fatal("empty canonical cwd")
	}

	if _, err := CanonicalizeCWD(""); err == nil {
		t.Fatal("expected error for empty cwd")
	}
	if _, err := CanonicalizeCWD("relative/path"); err == nil {
		t.Fatal("expected error for relative cwd")
	}
	if _, err := CanonicalizeCWD(filepath.Join(root, "does-not-exist")); err == nil {
		t.Fatal("expected error for non-existent cwd")
	}
	filePath := filepath.Join(root, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalizeCWD(filePath); err == nil {
		t.Fatal("expected error for file cwd")
	}
	if _, err := CanonicalizeCWD(root + "\x00"); err == nil {
		t.Fatal("expected error for NUL in cwd")
	}
}

func TestStableCWDHash(t *testing.T) {
	h1 := StableCWDHash("/foo/bar")
	h2 := StableCWDHash("/foo/bar")
	h3 := StableCWDHash("/foo/baz")
	if h1 != h2 {
		t.Fatal("same cwd produced different hashes")
	}
	if h1 == h3 {
		t.Fatal("different cwd produced same hash")
	}
}
