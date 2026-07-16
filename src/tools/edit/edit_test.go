package edit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool"
	readtool "awaken/src/tools/read"
)

func editContext(root string) context.Context {
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	return tool.ContextWithToolSessionState(ctx, tool.NewToolSessionState(root))
}

func primeRead(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	outcome, err := (&readtool.Tool{}).ExecuteOutcome(ctx, `{"file_path":"`+path+`"}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("Read=%+v err=%v", outcome, err)
	}
}

func TestEditRequiresReadAndRejectsExternalChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	ctx := editContext(root)
	notRead, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"sample.txt","old_string":"alpha","new_string":"beta"}`, nil)
	if notRead.ErrorCode != "file_not_read" {
		t.Fatalf("notRead=%+v", notRead)
	}
	primeRead(t, ctx, "sample.txt")
	if err := os.WriteFile(path, []byte("external\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	stale, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"sample.txt","old_string":"alpha","new_string":"beta"}`, nil)
	if stale.ErrorCode != "file_changed_since_read" {
		t.Fatalf("stale=%+v", stale)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "external\n" {
		t.Fatalf("stale Edit changed file: %q", data)
	}
}

func TestEditUniquenessReplaceAllAndCRLFBOM(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	data := append([]byte{0xef, 0xbb, 0xbf}, []byte("x\r\nkeep\r\nx\r\n")...)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	ctx := editContext(root)
	primeRead(t, ctx, "sample.txt")
	nonUnique, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"sample.txt","old_string":"x","new_string":"y"}`, nil)
	if nonUnique.ErrorCode != "edit_match_not_unique" {
		t.Fatalf("nonUnique=%+v", nonUnique)
	}
	success, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"sample.txt","old_string":"x","new_string":"y","replace_all":true}`, nil)
	if success.IsError() || success.PatchSummary == "" || len(success.Changes) != 1 {
		t.Fatalf("success=%+v", success)
	}
	after, _ := os.ReadFile(path)
	want := append([]byte{0xef, 0xbb, 0xbf}, []byte("y\r\nkeep\r\ny\r\n")...)
	if string(after) != string(want) {
		t.Fatalf("after=%v want=%v", after, want)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%v", info.Mode())
	}
}
