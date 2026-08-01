package glob

import (
	"context"
	"fmt"
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

func TestGlobPaginatesStableResultsWithoutShellFallback(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 135; index++ {
		name := fmt.Sprintf("file-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	first, err := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"*.txt","limit":50}`, nil)
	if err != nil || first.IsError() {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	cursor, _ := first.Details["next_cursor"].(string)
	if cursor == "" || first.Details["total"] != 135 || first.Details["returned"] != 50 {
		t.Fatalf("first page metadata=%+v", first.Details)
	}
	second, err := (&Tool{}).ExecuteOutcome(ctx, fmt.Sprintf(`{"pattern":"*.txt","limit":50,"cursor":%q}`, cursor), nil)
	if err != nil || second.IsError() {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if strings.Contains(second.Content, "file-049.txt") || !strings.Contains(second.Content, "file-050.txt") {
		t.Fatalf("second page is not the stable continuation: %q", second.Content)
	}
	if second.Details["next_cursor"] == "" {
		t.Fatalf("second page lost continuation: %+v", second.Details)
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

func TestGlobStillFailsWhenRipgrepRejectsThePattern(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	outcome, err := (&Tool{}).ExecuteOutcome(ctx, `{"pattern":"["}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "glob_failed" {
		t.Fatalf("outcome=%+v", outcome)
	}
}
