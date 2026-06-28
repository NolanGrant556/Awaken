package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoreLoadEventsHandlesLargePersistedEvent(t *testing.T) {
	store := newFileStore(t.TempDir())
	largeMessage := strings.Repeat("x", 70*1024)
	event := Event{
		Kind:      EventKindRunError,
		Timestamp: time.Now().UTC(),
		ThreadID:  "thread-1",
		RunID:     "run-1",
		Message:   largeMessage,
		Code:      "RUN_INTERRUPTED",
	}

	if err := store.appendEvent(event); err != nil {
		t.Fatalf("appendEvent returned error: %v", err)
	}

	events, err := store.loadEvents("run-1")
	if err != nil {
		t.Fatalf("loadEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 restored event, got %d", len(events))
	}
	if events[0].Message != largeMessage {
		t.Fatalf("expected restored message to round-trip, got %d chars", len(events[0].Message))
	}
}

func TestFileStoreLoadEventsIgnoresTruncatedLastLine(t *testing.T) {
	store := newFileStore(t.TempDir())
	event := Event{
		Kind:      EventKindRunError,
		Timestamp: time.Now().UTC(),
		ThreadID:  "thread-1",
		RunID:     "run-1",
		Message:   "first",
		Code:      "RUN_INTERRUPTED",
	}

	if err := store.appendEvent(event); err != nil {
		t.Fatalf("appendEvent returned error: %v", err)
	}
	eventsPath := filepath.Join(store.root, "events", "run-1.jsonl")
	file, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("OpenFile returned error: %v", err)
	}
	if _, err := file.WriteString(`{"kind":"run_error","run_id":"run-1"`); err != nil {
		file.Close()
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	events, err := store.loadEvents("run-1")
	if err != nil {
		t.Fatalf("loadEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 complete event after truncation recovery, got %d", len(events))
	}
	if events[0].Message != "first" {
		t.Fatalf("expected the intact event to survive, got %+v", events[0])
	}
}
