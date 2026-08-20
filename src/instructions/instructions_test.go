package instructions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoaderCombinesGlobalThenPrimaryProject(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeTestFile(t, filepath.Join(home, "AGENTS.md"), "global rule")
	writeTestFile(t, filepath.Join(project, ".awaken", "AGENTS.md"), "project rule")
	loader := NewLoader(home)
	loader.Now = func() time.Time { return time.Unix(10, 0) }

	snapshot := loader.Load(project, false)
	if got := len(snapshot.Summary.Sources); got != 2 {
		t.Fatalf("sources=%d, want 2", got)
	}
	if strings.Index(snapshot.ModelPrefix(), "global rule") >= strings.Index(snapshot.ModelPrefix(), "project rule") {
		t.Fatalf("expected global before project: %s", snapshot.ModelPrefix())
	}
	if snapshot.Summary.Partial {
		t.Fatal("complete sources must not be partial")
	}
}

func TestLoaderUsesOnlyPrimaryProjectFolder(t *testing.T) {
	home := t.TempDir()
	primary := t.TempDir()
	secondary := t.TempDir()
	writeTestFile(t, filepath.Join(primary, ".awaken", "AGENTS.md"), "primary")
	writeTestFile(t, filepath.Join(secondary, ".awaken", "AGENTS.md"), "secondary")
	snapshot := NewLoader(home).Load(primary, false)
	if strings.Contains(snapshot.ModelPrefix(), "secondary") || !strings.Contains(snapshot.ModelPrefix(), "primary") {
		t.Fatalf("wrong project authority: %s", snapshot.ModelPrefix())
	}
}

func TestLoaderFreezesCurrentRunAndRefreshesNextRun(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "AGENTS.md")
	writeTestFile(t, path, "first rule")
	loader := NewLoader(home)
	first := loader.Load("", true)
	writeTestFile(t, path, "second rule")
	second := loader.Load("", true)
	if !strings.Contains(first.ModelPrefix(), "first rule") || strings.Contains(first.ModelPrefix(), "second rule") {
		t.Fatalf("first snapshot was not frozen: %q", first.ModelPrefix())
	}
	if !strings.Contains(second.ModelPrefix(), "second rule") || first.Digest == second.Digest {
		t.Fatalf("next snapshot did not refresh: first=%s second=%s", first.Digest, second.Digest)
	}
}

func TestLoaderTreatsMissingSourcesAsNormal(t *testing.T) {
	snapshot := NewLoader(t.TempDir()).Load(t.TempDir(), false)
	if snapshot.Summary.Partial || len(snapshot.Summary.Sources) != 0 || len(snapshot.Summary.Diagnostics) != 0 || snapshot.ModelPrefix() != "" {
		t.Fatalf("missing sources should be a complete empty snapshot: %+v", snapshot.Summary)
	}
}

func TestLoaderTruncatesAndReplacesInvalidUTF8(t *testing.T) {
	home := t.TempDir()
	data := append([]byte{0xff}, []byte(strings.Repeat("a", SourceMaxBytes+100))...)
	if err := os.WriteFile(filepath.Join(home, "AGENTS.md"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := NewLoader(home).Load("", true)
	if !snapshot.Summary.Partial || len(snapshot.Summary.Sources) != 1 || !snapshot.Summary.Sources[0].Truncated {
		t.Fatalf("expected partial truncated snapshot: %#v", snapshot.Summary)
	}
	if !strings.Contains(snapshot.ModelPrefix(), "�") || !strings.Contains(snapshot.ModelPrefix(), "truncated at 16 KiB") {
		t.Fatalf("missing replacement/truncation marker")
	}
	if snapshot.Summary.Sources[0].AppliedBytes > SourceMaxBytes {
		t.Fatalf("applied bytes exceed limit: %d", snapshot.Summary.Sources[0].AppliedBytes)
	}
}

func TestLoaderSkipsSymlinkOutsideScope(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeTestFile(t, outside, "escape")
	if err := os.Symlink(outside, filepath.Join(home, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	snapshot := NewLoader(home).Load("", true)
	if !snapshot.Summary.Partial || strings.Contains(snapshot.ModelPrefix(), "escape") {
		t.Fatalf("outside symlink should be skipped: %#v", snapshot)
	}
	if snapshot.Summary.Diagnostics[0].Code != "SOURCE_OUTSIDE_SCOPE" {
		t.Fatalf("diagnostic=%s", snapshot.Summary.Diagnostics[0].Code)
	}
}

func TestLoaderContinuesWithOtherSourceWhenOneIsInvalid(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "AGENTS.md"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(project, ".awaken", "AGENTS.md"), "project remains available")
	snapshot := NewLoader(home).Load(project, false)
	if !snapshot.Summary.Partial || !strings.Contains(snapshot.ModelPrefix(), "project remains available") {
		t.Fatalf("valid project source should survive global failure: %+v", snapshot.Summary)
	}
	if len(snapshot.Summary.Diagnostics) != 1 || snapshot.Summary.Diagnostics[0].Scope != ScopeGlobal {
		t.Fatalf("unexpected diagnostics: %+v", snapshot.Summary.Diagnostics)
	}
}

func TestGlobalDocumentStoreCASAndDelete(t *testing.T) {
	store := NewGlobalDocumentStore(t.TempDir())
	missing, err := store.Get()
	if err != nil || missing.Exists || missing.Revision != "" {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
	created, err := store.Put("", "hello")
	if err != nil || !created.Exists || created.Content != "hello" || created.Revision == "" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if _, err := store.Put("stale", "lost"); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	deleted, err := store.Delete(created.Revision)
	if err != nil || deleted.Exists {
		t.Fatalf("deleted=%#v err=%v", deleted, err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
