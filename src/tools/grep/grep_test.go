package grep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestGrepContentNoMatchAndSummaryModes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("beta\nalpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	content, err := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"alpha","output_mode":"content"}`, nil)
	if err != nil || content.IsError() || !strings.HasPrefix(content.Content, "a.txt:1:alpha") {
		t.Fatalf("content=%+v err=%v", content, err)
	}
	files, _ := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"alpha","output_mode":"files_with_matches"}`, nil)
	if files.Content != "a.txt\nb.txt" {
		t.Fatalf("files=%q", files.Content)
	}
	none, _ := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"missing","output_mode":"content"}`, nil)
	if none.IsError() || none.Content != "No matches found." {
		t.Fatalf("none=%+v", none)
	}
}

func TestGrepMissingPinnedRGNeedsRuntime(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	ctx = tool.ContextWithExecutionEnvironmentSnapshot(ctx, tool.ExecutionEnvironmentSnapshot{SnapshotID: "env-no-rg", TransportPATH: t.TempDir()})
	outcome, _ := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"x"}`, nil)
	if outcome.Status != tool.OutcomeNeedsRuntime || outcome.ErrorCode != tool.ErrorCodeNeedsRuntime {
		t.Fatalf("outcome=%+v", outcome)
	}
}
