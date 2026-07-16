package write

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/tool"
	readtool "awaken/src/tools/read"
)

func writeContext(root string) context.Context {
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	return tool.ContextWithToolSessionState(ctx, tool.NewToolSessionState(root))
}

func TestWriteCreatesUTF8WithoutPublishingArtifact(t *testing.T) {
	root := t.TempDir()
	outcome, err := (&Tool{}).ExecuteOutcome(writeContext(root), `{"file_path":"new.md","content":"# hi\n"}`, nil)
	if err != nil || outcome.IsError() || len(outcome.Artifacts) != 0 || len(outcome.Changes) != 1 {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "new.md"))
	if string(data) != "# hi\n" {
		t.Fatalf("data=%q", data)
	}
}

func TestWriteExistingRequiresReadPreservesCRLFAndRejectsStale(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(path, []byte("old\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := writeContext(root)
	notRead, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"existing.txt","content":"new\n"}`, nil)
	if notRead.ErrorCode != "file_not_read" {
		t.Fatalf("notRead=%+v", notRead)
	}
	readOutcome, _ := (&readtool.Tool{}).ExecuteOutcome(ctx, `{"file_path":"existing.txt"}`, nil)
	if readOutcome.IsError() {
		t.Fatalf("Read=%+v", readOutcome)
	}
	success, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"existing.txt","content":"new\n"}`, nil)
	if success.IsError() {
		t.Fatalf("success=%+v", success)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new\r\n" {
		t.Fatalf("newline not preserved: %q", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v", info.Mode())
	}
	if err := os.WriteFile(path, []byte("external\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale, _ := (&Tool{}).ExecuteOutcome(ctx, `{"file_path":"existing.txt","content":"third\n"}`, nil)
	if stale.ErrorCode != "file_changed_since_read" {
		t.Fatalf("stale=%+v", stale)
	}
}
