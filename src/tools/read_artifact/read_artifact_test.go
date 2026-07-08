package readartifact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool"
)

func TestReadArtifactReadsCSV(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "report.csv")
	if err := os.WriteFile(path, []byte("name,amount\nAlice,10\nBob,20\n"), 0o644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	out, err := (&ReadArtifactTool{}).Execute(ctx, `{"path":"report.csv"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "类型: csv") || !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestReadArtifactAllowsAttachedFileOutsideReadableRoots(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	path := filepath.Join(outsideDir, "report.csv")
	if err := os.WriteFile(path, []byte("name,amount\nCarol,30\n"), 0o644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	permCtx := tool.NewTaskScopePermissionContext(
		"scope-test",
		root,
		[]string{root},
		[]string{root},
		[]string{root},
		[]tool.ScopedFile{{Path: path, DisplayName: "report.csv"}},
		nil,
		filepath.Join(t.TempDir(), "tmp"),
	)
	ctx := tool.ContextWithPermissionContext(context.Background(), permCtx)
	out, err := (&ReadArtifactTool{}).Execute(ctx, `{"path":"`+path+`"}`)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "Carol") {
		t.Fatalf("expected attached file content, got %q", out)
	}
}

func TestReadArtifactUnsupportedTypeHasRoutingHint(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "image.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	_, err := (&ReadArtifactTool{}).Execute(ctx, `{"path":"image.png"}`)
	if err == nil || !strings.Contains(err.Error(), "image_inspect") || !strings.Contains(err.Error(), "read_file") {
		t.Fatalf("expected unsupported type routing hint, got %v", err)
	}
}

func TestReadArtifactEmptyPDFHasClearDiagnostic(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty.pdf")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("failed to write empty pdf: %v", err)
	}

	ctx := tool.ContextWithPermissionContext(context.Background(), tool.NewPermissionContext(root, []string{root}))
	_, err := (&ReadArtifactTool{}).Execute(ctx, `{"path":"empty.pdf"}`)
	if err == nil || !strings.Contains(err.Error(), "空 PDF") || !strings.Contains(err.Error(), "0 字节") {
		t.Fatalf("expected empty pdf diagnostic, got %v", err)
	}
}
