package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func unicodeThreadLabelFixture() (input string, want string) {
	prefix := strings.Repeat("中", threadLabelMaxChars-1)
	return prefix + "🚀后续内容", prefix + "🚀"
}

func assertUnicodeThreadLabel(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("unexpected thread label\n got: %q\nwant: %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("thread label is not valid UTF-8: %q", got)
	}
	if count := utf8.RuneCountInString(got); count != threadLabelMaxChars {
		t.Fatalf("expected %d Unicode characters, got %d in %q", threadLabelMaxChars, count, got)
	}
}

func TestJournalOnlyThreadLabelRestoreIsUnicodeSafe(t *testing.T) {
	input, want := unicodeThreadLabelFixture()
	path := filepath.Join(t.TempDir(), "thread_unicode.jsonl")
	journal, err := NewExecutionJournal(path, "thread_unicode")
	if err != nil {
		t.Fatalf("NewExecutionJournal: %v", err)
	}
	if err := ensureThreadJournalRecord(context.Background(), journal, "thread_unicode", input); err != nil {
		_ = journal.Close()
		t.Fatalf("ensureThreadJournalRecord: %v", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !utf8.Valid(raw) {
		t.Fatalf("journal contains invalid UTF-8: %q", raw)
	}
	records, err := ReadExecutionJournal(path)
	if err != nil {
		t.Fatalf("ReadExecutionJournal: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one journal record, got %d", len(records))
	}
	restored := restoredTranscriptFromSemantic(records)
	assertUnicodeThreadLabel(t, restored.Label, want)
}

func TestSQLiteThreadLabelRoundTripIsUnicodeSafe(t *testing.T) {
	setRuntimeUserDataDir(t)
	input, want := unicodeThreadLabelFixture()
	now := time.Now().UTC()
	store := newThreadStore()
	snapshot := storeTitleTestSnapshot(t, "thread_unicode_sqlite", "run_unicode_sqlite", input, now)
	if err := store.CreateThread(snapshot, "", input); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	threads, err := store.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one SQLite thread, got %d", len(threads))
	}
	assertUnicodeThreadLabel(t, threads[0].Label, want)

	record, err := store.(*sqliteThreadStore).threadRecord(snapshot.ThreadID)
	if err != nil {
		t.Fatalf("threadRecord: %v", err)
	}
	assertUnicodeThreadLabel(t, record.Snapshot.Label, want)
}

func TestLegacyTranscriptThreadLabelRestoreIsUnicodeSafe(t *testing.T) {
	input, want := unicodeThreadLabelFixture()
	path := filepath.Join(t.TempDir(), "thread_legacy_unicode.jsonl")
	if err := appendTranscriptRecords(path, []TranscriptRecord{{
		RecordType: transcriptRecordTypeThread,
		ThreadID:   "thread_legacy_unicode",
		Content:    input,
		Timestamp:  time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("appendTranscriptRecords: %v", err)
	}

	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		t.Fatalf("loadRestoredTranscriptAtPath: %v", err)
	}
	assertUnicodeThreadLabel(t, restored.Label, want)

	projected, err := (LegacyTranscriptAdapter{}).ReadFile(path)
	if err != nil {
		t.Fatalf("LegacyTranscriptAdapter.ReadFile: %v", err)
	}
	semanticRestore := restoredTranscriptFromSemantic(projected)
	assertUnicodeThreadLabel(t, semanticRestore.Label, want)
}
