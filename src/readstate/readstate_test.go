package readstate

import (
	"reflect"
	"testing"
	"time"
)

func TestCacheNormalizesPaths(t *testing.T) {
	cache := NewCache(10)
	readAt := time.Unix(10, 0)

	cache.Set(`/tmp/project/../project\notes.txt`, Entry{
		Content:   "hello",
		Timestamp: readAt,
		Offset:    1,
		Limit:     20,
	})

	entry, ok := cache.Get("/tmp/project/notes.txt")
	if !ok {
		t.Fatalf("expected normalized path lookup to hit")
	}
	if entry.Content != "hello" {
		t.Fatalf("unexpected content: %q", entry.Content)
	}

	keys := cache.Keys()
	want := []string{"/tmp/project/notes.txt"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("unexpected keys: got %v want %v", keys, want)
	}
}

func TestValidateWriteRejectsUnreadFile(t *testing.T) {
	cache := NewCache(10)

	decision, code := cache.ValidateWrite("/tmp/notes.txt", time.Unix(20, 0))

	if decision != DecisionDeny || code != ErrorUnread {
		t.Fatalf("unexpected decision/code: got %s/%s", decision, code)
	}
}

func TestValidateWriteRejectsPartialView(t *testing.T) {
	cache := NewCache(10)
	cache.Set("/tmp/notes.txt", Entry{
		Content:       "partial",
		Timestamp:     time.Unix(20, 0),
		IsPartialView: true,
	})

	decision, code := cache.ValidateWrite("/tmp/notes.txt", time.Unix(20, 0))

	if decision != DecisionDeny || code != ErrorPartialView {
		t.Fatalf("unexpected decision/code: got %s/%s", decision, code)
	}
}

func TestValidateWriteRejectsStaleRead(t *testing.T) {
	cache := NewCache(10)
	cache.Set("/tmp/notes.txt", Entry{
		Content:   "old",
		Timestamp: time.Unix(20, 0),
	})

	decision, code := cache.ValidateWrite("/tmp/notes.txt", time.Unix(21, 0))

	if decision != DecisionDeny || code != ErrorStale {
		t.Fatalf("unexpected decision/code: got %s/%s", decision, code)
	}
}

func TestValidateWriteAllowsFreshRead(t *testing.T) {
	cache := NewCache(10)
	cache.Set("/tmp/notes.txt", Entry{
		Content:   "fresh",
		Timestamp: time.Unix(20, 0),
	})

	decision, code := cache.ValidateWrite("/tmp/notes.txt", time.Unix(20, 0))

	if decision != DecisionAllow || code != ErrorNone {
		t.Fatalf("unexpected decision/code: got %s/%s", decision, code)
	}
}

func TestValidateWriteAllowsMissingFile(t *testing.T) {
	cache := NewCache(10)

	decision, code := cache.ValidateWrite("/tmp/new.txt", time.Time{})

	if decision != DecisionAllow || code != ErrorNone {
		t.Fatalf("unexpected decision/code: got %s/%s", decision, code)
	}
}

func TestValidateWriteAllowsUNCPaths(t *testing.T) {
	cache := NewCache(10)
	paths := []string{
		`\\server\share\notes.txt`,
		"//server/share/notes.txt",
	}

	for _, filePath := range paths {
		decision, code := cache.ValidateWrite(filePath, time.Unix(20, 0))
		if decision != DecisionAllow || code != ErrorNone {
			t.Fatalf("unexpected decision/code for %q: got %s/%s", filePath, decision, code)
		}
	}
}

func TestMergeNewerTimestampWins(t *testing.T) {
	older := NewCache(10)
	newer := NewCache(10)
	older.Set("/tmp/notes.txt", Entry{
		Content:   "old",
		Timestamp: time.Unix(10, 0),
	})
	newer.Set("/tmp/notes.txt", Entry{
		Content:   "new",
		Timestamp: time.Unix(20, 0),
	})

	merged := Merge(older, newer)
	entry, ok := merged.Get("/tmp/notes.txt")
	if !ok {
		t.Fatalf("expected merged entry")
	}
	if entry.Content != "new" {
		t.Fatalf("expected newer entry to win, got %q", entry.Content)
	}

	merged = Merge(newer, older)
	entry, ok = merged.Get("/tmp/notes.txt")
	if !ok {
		t.Fatalf("expected merged entry")
	}
	if entry.Content != "new" {
		t.Fatalf("expected older second entry not to overwrite newer first entry, got %q", entry.Content)
	}
}

func TestCacheEvictsOldestDeterministically(t *testing.T) {
	cache := NewCache(2)
	cache.Set("/tmp/a.txt", Entry{Content: "a", Timestamp: time.Unix(1, 0)})
	cache.Set("/tmp/b.txt", Entry{Content: "b", Timestamp: time.Unix(2, 0)})
	cache.Set("/tmp/c.txt", Entry{Content: "c", Timestamp: time.Unix(3, 0)})

	if _, ok := cache.Get("/tmp/a.txt"); ok {
		t.Fatalf("expected oldest entry to be evicted")
	}
	keys := cache.Keys()
	want := []string{"/tmp/b.txt", "/tmp/c.txt"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("unexpected keys after eviction: got %v want %v", keys, want)
	}
}

func TestDeleteClearKeysAndEntries(t *testing.T) {
	cache := NewCache(10)
	cache.Set("/tmp/a.txt", Entry{Content: "a", Timestamp: time.Unix(1, 0)})
	cache.Set("/tmp/b.txt", Entry{Content: "b", Timestamp: time.Unix(2, 0)})

	if !cache.Delete("/tmp/../tmp/a.txt") {
		t.Fatalf("expected delete to remove normalized path")
	}
	if _, ok := cache.Get("/tmp/a.txt"); ok {
		t.Fatalf("expected deleted entry to be missing")
	}

	entries := cache.Entries()
	if len(entries) != 1 || entries[0].Path != "/tmp/b.txt" || entries[0].Entry.Content != "b" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	cache.Clear()
	if got := cache.Keys(); len(got) != 0 {
		t.Fatalf("expected clear to remove all keys, got %v", got)
	}
}

func TestCacheReturnsDefensiveCopies(t *testing.T) {
	cache := NewCache(10)
	cache.Set("/tmp/notes.txt", Entry{
		Content:   "original",
		Timestamp: time.Unix(1, 0),
		Offset:    2,
		Limit:     3,
	})

	entry, ok := cache.Get("/tmp/notes.txt")
	if !ok {
		t.Fatalf("expected cached entry")
	}
	entry.Content = "changed"
	entry.Offset = 99

	entries := cache.Entries()
	entries[0].Entry.Content = "changed again"
	entries[0].Path = "/tmp/changed.txt"

	keys := cache.Keys()
	keys[0] = "/tmp/changed.txt"

	got, ok := cache.Get("/tmp/notes.txt")
	if !ok {
		t.Fatalf("expected cached entry after local mutations")
	}
	if got.Content != "original" || got.Offset != 2 || got.Limit != 3 {
		t.Fatalf("cache was mutated through returned value: %#v", got)
	}
	if gotKeys := cache.Keys(); !reflect.DeepEqual(gotKeys, []string{"/tmp/notes.txt"}) {
		t.Fatalf("keys were mutated through returned slice: %v", gotKeys)
	}
}
