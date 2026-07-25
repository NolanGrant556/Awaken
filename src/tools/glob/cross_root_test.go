package glob

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
