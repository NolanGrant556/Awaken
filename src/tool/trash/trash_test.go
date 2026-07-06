package trash

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultAdapterReturnsNonNil(t *testing.T) {
	if DefaultAdapter() == nil {
		t.Fatal("DefaultAdapter returned nil")
	}
}

func TestSetAdapterOverride(t *testing.T) {
	t.Cleanup(func() { SetAdapter(nil) })

	mock := &recordingAdapter{trashFn: func(path string) (TrashResult, error) {
		return TrashResult{OriginalPath: path, TrashedPath: path + ".trashed", Method: "mock"}, nil
	}}
	SetAdapter(mock)

	if GetAdapter() != mock {
		t.Fatal("GetAdapter did not return the override")
	}

	result, err := DefaultAdapter().TrashPath("/tmp/whatever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "mock" {
		t.Fatalf("expected mock adapter to be used, got method %q", result.Method)
	}
}

func TestSetAdapterNilRestoresDefault(t *testing.T) {
	t.Cleanup(func() { SetAdapter(nil) })

	mock := &recordingAdapter{trashFn: func(path string) (TrashResult, error) {
		return TrashResult{Method: "mock"}, nil
	}}
	SetAdapter(mock)
	SetAdapter(nil)

	if GetAdapter() == mock {
		t.Fatal("expected override to be cleared after SetAdapter(nil)")
	}
}

// TestTrashPathNonExistentReturnsError verifies that trashing a path that
// does not exist returns an error on every platform.
func TestTrashPathNonExistentReturnsError(t *testing.T) {
	t.Cleanup(func() { SetAdapter(nil) })

	adapter := DefaultAdapter()
	missing := filepath.Join(t.TempDir(), "does-not-exist.txt")

	_, err := adapter.TrashPath(missing)
	if err == nil {
		t.Fatal("expected error when trashing non-existent path, got nil")
	}
}

// TestTrashPathMovesFileAndPopulatesResult verifies that a real file is
// moved out of its original location and that TrashResult is populated.
// This test only runs on platforms with a working adapter (darwin/windows).
func TestTrashPathMovesFileAndPopulatesResult(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skipf("trash adapter not supported on %s", runtime.GOOS)
	}

	t.Cleanup(func() { SetAdapter(nil) })
	SetAdapter(nil) // clear any override left by prior tests

	adapter := DefaultAdapter()
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "file-to-trash.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	result, err := adapter.TrashPath(src)
	if err != nil {
		t.Fatalf("TrashPath failed: %v", err)
	}
	t.Cleanup(func() {
		if result.TrashedPath != "" {
			_ = os.Remove(result.TrashedPath)
		}
	})

	// (a) original path no longer exists
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("expected original path %s to no longer exist; stat err=%v", src, err)
	}

	// (b) TrashResult is populated
	if result.OriginalPath == "" {
		t.Error("OriginalPath is empty")
	}
	if result.TrashedPath == "" {
		t.Error("TrashedPath is empty")
	}
	if result.Method == "" {
		t.Error("Method is empty")
	}

	// The trashed file should actually exist at TrashedPath.
	if _, err := os.Stat(result.TrashedPath); err != nil {
		t.Errorf("trashed file not found at TrashedPath %s: %v", result.TrashedPath, err)
	}
}

// TestTrashPathHandlesNameCollision verifies that when two files with the
// same base name are trashed, the second one gets a disambiguated name
// rather than overwriting the first.
func TestTrashPathHandlesNameCollision(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skipf("trash adapter not supported on %s", runtime.GOOS)
	}

	t.Cleanup(func() { SetAdapter(nil) })
	SetAdapter(nil)

	adapter := DefaultAdapter()
	tmpDir := t.TempDir()

	// First file
	src1 := filepath.Join(tmpDir, "collide.txt")
	if err := os.WriteFile(src1, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	result1, err := adapter.TrashPath(src1)
	if err != nil {
		t.Fatalf("first TrashPath failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(result1.TrashedPath) })

	// Second file with same base name
	src2 := filepath.Join(tmpDir, "collide.txt")
	if err := os.WriteFile(src2, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	result2, err := adapter.TrashPath(src2)
	if err != nil {
		t.Fatalf("second TrashPath failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(result2.TrashedPath) })

	if result1.TrashedPath == result2.TrashedPath {
		t.Fatalf("expected disambiguated trashed paths, both were %s", result1.TrashedPath)
	}

	// Both trashed files should exist.
	if _, err := os.Stat(result1.TrashedPath); err != nil {
		t.Errorf("first trashed file missing at %s: %v", result1.TrashedPath, err)
	}
	if _, err := os.Stat(result2.TrashedPath); err != nil {
		t.Errorf("second trashed file missing at %s: %v", result2.TrashedPath, err)
	}
}

// recordingAdapter is a test double that delegates to a function.
type recordingAdapter struct {
	trashFn func(path string) (TrashResult, error)
}

func (r *recordingAdapter) TrashPath(path string) (TrashResult, error) {
	return r.trashFn(path)
}
