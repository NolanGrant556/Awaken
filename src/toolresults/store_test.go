package toolresults

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStorePersistsAndReadsFullContent(t *testing.T) {
	workspace := t.TempDir()
	storeDir := filepath.Join(t.TempDir(), "tool-results")
	store := NewStore(Config{Dir: storeDir, WorkspaceRoot: workspace})
	full := strings.Repeat("alpha\n", 20) + "COMPLETE_TAIL"

	handle, err := store.Persist(full, 24)
	if err != nil {
		t.Fatalf("persist tool result: %v", err)
	}

	if handle.ID == "" || strings.Contains(handle.ID, "/") || strings.Contains(handle.ID, "..") {
		t.Fatalf("expected safe handle id, got %q", handle.ID)
	}
	if handle.Path == "" || !filepath.IsAbs(handle.Path) {
		t.Fatalf("expected absolute path, got %q", handle.Path)
	}
	if isWithinDir(handle.Path, workspace) {
		t.Fatalf("expected handle path outside workspace, got %q in %q", handle.Path, workspace)
	}
	if handle.RelativePath != handle.ID+".txt" {
		t.Fatalf("unexpected relative path: %q", handle.RelativePath)
	}
	if handle.OriginalBytes != len([]byte(full)) || handle.OriginalChars != utf8.RuneCountInString(full) {
		t.Fatalf("unexpected original sizes: %+v", handle)
	}
	if !handle.Truncated || strings.Contains(handle.Preview, "COMPLETE_TAIL") {
		t.Fatalf("expected truncated preview without tail, got %+v", handle)
	}

	got, err := store.Read(handle.ID)
	if err != nil {
		t.Fatalf("read persisted tool result: %v", err)
	}
	if got != full {
		t.Fatalf("expected full persisted content, got %q", got)
	}
}

func TestStoreRejectsWorkspaceDirectory(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(Config{
		Dir:           filepath.Join(workspace, ".cache", "tool-results"),
		WorkspaceRoot: workspace,
	})

	_, err := store.Persist("too large for inline context", 10)
	if !errors.Is(err, ErrStorageInsideWorkspace) {
		t.Fatalf("expected workspace storage rejection, got %v", err)
	}
}

func TestStoreReadRejectsTraversalHandle(t *testing.T) {
	store := NewStore(Config{Dir: t.TempDir()})

	_, err := store.Read("../secret")
	if !errors.Is(err, ErrInvalidHandleID) {
		t.Fatalf("expected invalid handle id, got %v", err)
	}
}

func TestPreviewIsRuneSafe(t *testing.T) {
	preview, truncated := Preview("你好世界", 3)
	if preview != "你好世" || !truncated {
		t.Fatalf("unexpected preview %q truncated=%v", preview, truncated)
	}
}

func TestFormatModelContentIncludesHandleMetadata(t *testing.T) {
	handle := Handle{
		ID:            "tr_test",
		Path:          "/tmp/awaken/tool-results/tr_test.txt",
		RelativePath:  "tr_test.txt",
		OriginalBytes: 120,
		OriginalChars: 100,
		Preview:       "first lines",
		Truncated:     true,
	}

	content := FormatModelContent(handle)
	for _, want := range []string{
		"<persisted-tool-result>",
		"handle_id: tr_test",
		"relative_path: tr_test.txt",
		"original_bytes: 120",
		"original_chars: 100",
		"truncated: true",
		"first lines",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected formatted content to contain %q, got %q", want, content)
		}
	}
	if strings.Contains(content, "/tmp/awaken/tool-results") {
		t.Fatalf("formatted model content should not expose absolute storage path: %q", content)
	}
}
