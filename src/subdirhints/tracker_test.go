package subdirhints

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestTrackerExtractsPathFilePathAndWorkdir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/AGENTS.md", "src hint")
	writeFile(t, root, "pkg/CLAUDE.md", "pkg hint")
	writeFile(t, root, "ops/agents.md", "ops hint")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("read_file", map[string]any{"path": "src/main.go"})
	assertContains(t, result.Text, "[Subdirectory context discovered: src/AGENTS.md]\nsrc hint")

	tracker.Reset()
	result = tracker.CheckToolCall("edit_file", map[string]any{"file_path": "pkg/server.go"})
	assertContains(t, result.Text, "[Subdirectory context discovered: pkg/CLAUDE.md]\npkg hint")

	tracker.Reset()
	result = tracker.CheckToolCall("shell", map[string]any{"workdir": "ops"})
	assertContains(t, result.Text, "[Subdirectory context discovered: ops/agents.md]\nops hint")
}

func TestTrackerExtractsShellCommandToken(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "scripts/AGENTS.md", "script hint")
	writeFile(t, root, "scripts/run.sh", "#!/bin/sh\n")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("shell", map[string]any{
		"command": "sh scripts/run.sh",
	})

	assertContains(t, result.Text, "[Subdirectory context discovered: scripts/AGENTS.md]\nscript hint")
}

func TestTrackerExtractsQuotedShellPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "space dir/AGENTS.md", "quoted hint")
	writeFile(t, root, "space dir/file.go", "package main\n")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("shell", map[string]any{
		"command": `cat "space dir/file.go"`,
	})

	assertContains(t, result.Text, "[Subdirectory context discovered: space dir/AGENTS.md]\nquoted hint")
}

func TestTrackerWalksAncestors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/AGENTS.md", "docs hint")
	writeFile(t, root, "docs/feature/CLAUDE.md", "feature hint")
	writeFile(t, root, "docs/feature/deep/example.md", "example")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("read_file", map[string]any{"path": "docs/feature/deep/example.md"})

	assertContains(t, result.Text, "[Subdirectory context discovered: docs/AGENTS.md]\ndocs hint")
	assertContains(t, result.Text, "[Subdirectory context discovered: docs/feature/CLAUDE.md]\nfeature hint")
	if strings.Index(result.Text, "docs/AGENTS.md") > strings.Index(result.Text, "docs/feature/CLAUDE.md") {
		t.Fatalf("expected ancestor hint before child hint, got:\n%s", result.Text)
	}
}

func TestTrackerUsesFirstMatchingHintFilePerDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/AGENTS.md", "agents wins")
	writeFile(t, root, "pkg/CLAUDE.md", "claude loses")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("read_file", map[string]any{"path": "pkg/file.go"})

	assertContains(t, result.Text, "agents wins")
	assertNotContains(t, result.Text, "claude loses")
	if len(result.Hints) != 1 || result.Hints[0].Filename != "AGENTS.md" {
		t.Fatalf("expected AGENTS.md first match, got %+v", result.Hints)
	}
}

func TestTrackerTruncatesContentAndReturnsWarning(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/AGENTS.md", "123456789")

	tracker := newTestTracker(t, root, Options{MaxHintChars: 5})
	result := tracker.CheckToolCall("read_file", map[string]any{"path": "pkg/file.go"})

	assertContains(t, result.Text, "[Subdirectory context discovered: pkg/AGENTS.md]\n12345")
	assertNotContains(t, result.Text, "6789")
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "truncated to 5 chars") {
		t.Fatalf("expected truncation warning, got %#v", result.Warnings)
	}
	if len(result.Hints) != 1 || !result.Hints[0].Truncated {
		t.Fatalf("expected truncated hint metadata, got %+v", result.Hints)
	}
}

func TestTrackerIgnoresOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "AGENTS.md", "outside hint")
	writeFile(t, outside, "secret.txt", "secret")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("read_file", map[string]any{"path": filepath.Join(outside, "secret.txt")})

	if !result.Empty() {
		t.Fatalf("expected outside path to be ignored, got %+v", result)
	}
	assertLoadedDirs(t, tracker.LoadedDirs(), root)
}

func TestTrackerDoesNotReadHintSymlinkOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, root, "pkg/file.go", "package pkg\n")
	writeFile(t, outside, "AGENTS.md", "outside hint")
	link := filepath.Join(root, "pkg", "AGENTS.md")
	if err := os.Symlink(filepath.Join(outside, "AGENTS.md"), link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("read_file", map[string]any{"path": "pkg/file.go"})

	if result.Text != "" || strings.Contains(result.Text, "outside hint") {
		t.Fatalf("expected outside hint symlink to be ignored, got %+v", result)
	}
}

func TestTrackerSkipsAlreadyLoadedDirectory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/AGENTS.md", "src hint")
	writeFile(t, root, "src/main.go", "package main\n")

	tracker := newTestTracker(t, root, Options{})
	first := tracker.CheckToolCall("read_file", map[string]any{"path": "src/main.go"})
	second := tracker.CheckToolCall("read_file", map[string]any{"path": "src/main.go"})

	assertContains(t, first.Text, "src hint")
	if !second.Empty() {
		t.Fatalf("expected no duplicate hint, got %+v", second)
	}
	assertLoadedDirs(t, tracker.LoadedDirs(), root, filepath.Join(root, "src"))
}

func TestTrackerResetAndSnapshot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/AGENTS.md", "src hint")

	tracker := newTestTracker(t, root, Options{MaxAncestorDirs: 3, MaxHintChars: 12})
	tracker.CheckToolCall("read_file", map[string]any{"path": "src/main.go"})

	snapshot := tracker.Snapshot()
	if snapshot.WorkingDir != resolvedPath(t, root) || snapshot.MaxAncestorDirs != 3 || snapshot.MaxHintChars != 12 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	assertLoadedDirs(t, snapshot.LoadedDirs, root, filepath.Join(root, "src"))

	tracker.Reset()
	assertLoadedDirs(t, tracker.LoadedDirs(), root)
}

func TestTrackerIgnoresHiddenDirsURLsGitRemotesAndFlags(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".hidden/AGENTS.md", "hidden hint")
	writeFile(t, root, ".hidden/file.go", "package hidden\n")
	writeFile(t, root, "src/AGENTS.md", "src hint")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("shell", map[string]any{
		"command": "cat -I https://example.com/file git@github.com:org/repo.git .hidden/file.go --src src/file.go",
	})

	assertContains(t, result.Text, "[Subdirectory context discovered: src/AGENTS.md]\nsrc hint")
	assertNotContains(t, result.Text, "hidden hint")
	assertLoadedDirs(t, tracker.LoadedDirs(), root, filepath.Join(root, "src"))
}

func TestTrackerResolvesCommandPathsAgainstWorkdir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "packages/app/AGENTS.md", "app hint")
	writeFile(t, root, "packages/app/main.go", "package app\n")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCall("shell", map[string]any{
		"workdir": "packages/app",
		"command": "cat main.go",
	})

	assertContains(t, result.Text, "[Subdirectory context discovered: packages/app/AGENTS.md]\napp hint")
}

func TestTrackerCheckToolCallJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/AGENTS.md", "pkg hint")

	tracker := newTestTracker(t, root, Options{})
	result := tracker.CheckToolCallJSON("read_file", `{"path":"pkg/file.go"}`)

	assertContains(t, result.Text, "[Subdirectory context discovered: pkg/AGENTS.md]\npkg hint")
}

func newTestTracker(t *testing.T, root string, options Options) *Tracker {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("failed to resolve root: %v", err)
	}
	tracker, err := NewTracker(resolved, options)
	if err != nil {
		t.Fatalf("NewTracker returned error: %v", err)
	}
	return tracker
}

func writeFile(t *testing.T, root string, relPath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create parent dir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", relPath, err)
	}
}

func assertContains(t *testing.T, value string, want string) {
	t.Helper()
	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}

func assertNotContains(t *testing.T, value string, unwanted string) {
	t.Helper()
	if strings.Contains(value, unwanted) {
		t.Fatalf("expected %q not to contain %q", value, unwanted)
	}
}

func assertLoadedDirs(t *testing.T, got []string, want ...string) {
	t.Helper()
	for i, path := range want {
		want[i] = resolvedPath(t, path)
	}
	sortStrings(got)
	sortStrings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected loaded dirs\ngot:  %#v\nwant: %#v", got, want)
	}
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("failed to resolve %s: %v", path, err)
	}
	return resolved
}

func sortStrings(values []string) {
	sort.Strings(values)
}
