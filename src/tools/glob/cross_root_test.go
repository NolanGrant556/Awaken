package glob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
	"awaken/src/tools/internal/searchroots"
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

func writeFile(t *testing.T, root, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGlobWithoutPathCoversEverySecondaryRoot(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	writeFile(t, primary, "primary.go")
	writeFile(t, secondary, "secondary.go")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"*.go"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if !strings.Contains(outcome.Content, filepath.ToSlash(filepath.Join(primary, "primary.go"))) {
		t.Fatalf("missing primary match: %q", outcome.Content)
	}
	if !strings.Contains(outcome.Content, filepath.ToSlash(filepath.Join(secondary, "secondary.go"))) {
		t.Fatalf("missing secondary match: %q", outcome.Content)
	}
	if outcome.Details["path_style"] != "absolute" {
		t.Fatalf("path_style=%v", outcome.Details["path_style"])
	}
	if total, ok := outcome.Details["total"].(int); !ok || total != 2 {
		t.Fatalf("total=%v", outcome.Details["total"])
	}
}

func TestGlobTreatsAnEmptyProjectRootAsNoMatches(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	writeFile(t, primary, "primary.md")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"**/*.md"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	expected := filepath.ToSlash(filepath.Join(primary, "primary.md"))
	if outcome.Content != expected {
		t.Fatalf("content=%q want=%q", outcome.Content, expected)
	}
	if total, ok := outcome.Details["total"].(int); !ok || total != 1 {
		t.Fatalf("total=%v", outcome.Details["total"])
	}
}

func TestGlobReturnsSuccessWhenEveryProjectRootIsEmpty(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"**/*.md"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Content != "No files matched." {
		t.Fatalf("content=%q", outcome.Content)
	}
	if total, ok := outcome.Details["total"].(int); !ok || total != 0 {
		t.Fatalf("total=%v", outcome.Details["total"])
	}
}

func TestGlobWithExplicitPathStaysOnThatRootAndKeepsRelativePaths(t *testing.T) {
	primary := t.TempDir()
	secondary := t.TempDir()
	writeFile(t, primary, "primary.go")
	writeFile(t, secondary, "secondary.go")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"*.go","path":"`+secondary+`"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Content != "secondary.go" {
		t.Fatalf("content=%q", outcome.Content)
	}
	if outcome.Details["path_style"] != "root_relative" {
		t.Fatalf("path_style=%v", outcome.Details["path_style"])
	}
}

func TestGlobDoesNotReportAFileTwiceWhenARootIsNestedInAnother(t *testing.T) {
	primary := t.TempDir()
	nested := filepath.Join(primary, "packages", "inner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nested, "inner.go")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, nested),
		`{"pattern":"**/*.go"}`,
		nil,
	)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if total, ok := outcome.Details["total"].(int); !ok || total != 1 {
		t.Fatalf("nested root duplicated results: total=%v content=%q", outcome.Details["total"], outcome.Content)
	}
	if outcome.Details["path_style"] != "root_relative" {
		t.Fatalf("path_style=%v", outcome.Details["path_style"])
	}
}

func TestGlobProjectModeRejectsTheSiblingRootsCommonParent(t *testing.T) {
	parent := t.TempDir()
	primary := filepath.Join(parent, "project-a")
	secondary := filepath.Join(parent, "project-b")
	outside := filepath.Join(parent, "not-in-project")
	for _, dir := range []string{primary, secondary, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, outside, "outside.go")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary, secondary),
		`{"pattern":"*.go","path":"`+parent+`"}`,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ErrorCode != searchroots.ErrorCodeProjectScopePathMismatch {
		t.Fatalf("outcome=%+v", outcome)
	}
}

func TestGlobExplicitPathModeKeepsOpenFilesystemAccess(t *testing.T) {
	primary := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "outside.go")

	outcome, err := (&Tool{}).ExecuteOutcome(
		multiRootContext(t, primary),
		`{"pattern":"*.go","path":"`+outside+`","scope_mode":"explicit_path"}`,
		nil,
	)
	if err != nil || outcome.IsError() || outcome.Content != "outside.go" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Details["scope_relation"] != "outside_project" || outcome.Details["scope_source"] != "user_explicit" {
		t.Fatalf("scope metadata=%+v", outcome.Details)
	}
}
