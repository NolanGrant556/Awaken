package grep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func multiRootContext(t *testing.T, roots ...string) context.Context {
	t.Helper()
	permissionContext := tool.PermissionContext{
		WorkspaceRoot: roots[0],
		ReadableRoots: roots,
		WritableRoots: roots,
	}.Normalize()
	return tool.ContextWithPermissionContext(context.Background(), permissionContext)
}

func writeFile(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGrepWithoutPathMergesMatchesFromEveryReadableRoot(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	writeFile(t, primary, "primary.go", "package main\n// needle here\n")
	writeFile(t, secondary, "secondary.go", "package docs\n// needle there\n")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"needle"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if !strings.Contains(outcome.Content, filepath.ToSlash(filepath.Join(primary, "primary.go"))+":2:") {
		t.Fatalf("missing primary row: %q", outcome.Content)
	}
	if !strings.Contains(outcome.Content, filepath.ToSlash(filepath.Join(secondary, "secondary.go"))+":2:") {
		t.Fatalf("missing secondary row: %q", outcome.Content)
	}
	if outcome.Details["match_count"] != 2 {
		t.Fatalf("match_count=%v", outcome.Details["match_count"])
	}
	if outcome.Details["path_style"] != "absolute" {
		t.Fatalf("path_style=%v", outcome.Details["path_style"])
	}
}

func TestGrepCountModeKeepsItsNumericSuffixAcrossRoots(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	writeFile(t, primary, "primary.go", "needle\nneedle\n")
	writeFile(t, secondary, "secondary.go", "needle\n")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"needle","output_mode":"count"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if !strings.Contains(outcome.Content, filepath.ToSlash(filepath.Join(primary, "primary.go"))+":2") {
		t.Fatalf("missing primary count: %q", outcome.Content)
	}
	if !strings.Contains(outcome.Content, filepath.ToSlash(filepath.Join(secondary, "secondary.go"))+":1") {
		t.Fatalf("missing secondary count: %q", outcome.Content)
	}
}

func TestGrepWithExplicitPathKeepsSingleRootRelativeRows(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	writeFile(t, primary, "primary.go", "needle\n")
	writeFile(t, secondary, "secondary.go", "needle\n")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"needle","path":"primary.go"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Content != "primary.go:1:needle" {
		t.Fatalf("content=%q", outcome.Content)
	}
	if outcome.Details["path_style"] != "root_relative" {
		t.Fatalf("path_style=%v", outcome.Details["path_style"])
	}
}

// An explicit path is a direct instruction, so it is searched even when it sits
// outside every root. Only the default scope -- the search with no path -- is
// limited to the project folders, which is what keeps an unscoped grep from
// walking the whole disk.
func TestGrepSearchesAnExplicitPathOutsideEveryRoot(t *testing.T) {
	primary := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "secret.go", "needle\n")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary),
		`{"pattern":"needle","path":"`+outside+`"}`,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.IsError() || !strings.Contains(outcome.Content, "secret.go") {
		t.Fatalf("outcome=%+v", outcome)
	}
}

// The default scope stays inside the project even when a match exists nearby on
// disk, so an unscoped search does not silently widen to the filesystem.
func TestGrepDefaultScopeSkipsDirectoriesOutsideTheProject(t *testing.T) {
	primary := t.TempDir()
	outside := t.TempDir()
	writeFile(t, primary, "inside.go", "needle\n")
	writeFile(t, outside, "secret.go", "needle\n")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary),
		`{"pattern":"needle"}`,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.IsError() || strings.Contains(outcome.Content, "secret.go") {
		t.Fatalf("outcome=%+v", outcome)
	}
	if !strings.Contains(outcome.Content, "inside.go") {
		t.Fatalf("expected the in-project match, got %+v", outcome)
	}
}
