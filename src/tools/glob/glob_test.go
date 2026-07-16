package glob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestGlobUsesRealRGAndStableSort(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err != nil {
		t.Skip("filesystem fixture unavailable")
	}
	root := t.TempDir()
	for _, name := range []string{"z.go", "a.go", ".hidden.go", "note.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"*.go"}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if outcome.Content != ".hidden.go\na.go\nz.go" {
		t.Fatalf("sort/content=%q", outcome.Content)
	}
}

func TestGlobMissingPinnedRGNeedsRuntime(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	ctx = tool.ContextWithExecutionEnvironmentSnapshot(ctx, tool.ExecutionEnvironmentSnapshot{SnapshotID: "env-no-rg", TransportPATH: strings.Join([]string{t.TempDir()}, string(os.PathListSeparator))})
	outcome, _ := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"*"}`, nil)
	if outcome.Status != tool.OutcomeNeedsRuntime || outcome.ErrorCode != tool.ErrorCodeNeedsRuntime {
		t.Fatalf("outcome=%+v", outcome)
	}
}
