package textfilecreate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestTextFileCreateWritesMarkdown(t *testing.T) {
	root := t.TempDir()
	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))

	out, err := (&TextFileCreateTool{}).Execute(ctx, `{"path":"reports/ERP差异报告.md","content":"# 标题\n正文\n"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "ERP差异报告.md") {
		t.Fatalf("unexpected output: %q", out)
	}
	data, err := os.ReadFile(filepath.Join(root, "reports", "ERP差异报告.md"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "# 标题\n正文\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestTextFileCreateRejectsSensitivePath(t *testing.T) {
	root := t.TempDir()
	decision := (&TextFileCreateTool{}).CheckPermission(
		context.Background(),
		`{"path":".env","content":"SECRET=1"}`,
		tool.NewPermissionContext(root, []string{root}),
	)
	if decision.Behavior != tool.PermissionDeny {
		t.Fatalf("expected deny for sensitive path, got %+v", decision)
	}
}
