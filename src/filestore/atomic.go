package filestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// injectable failure points for crash-injection tests. Each hook runs before
// the named step; returning an error simulates a crash at that point.
var (
	hookBeforeTempFsync  func(path string) error
	hookBeforeRename     func(temp, target string) error
	hookBeforeParentSync func(dir string) error
)

// WriteFileAtomic durably replaces target with data:
// unique same-directory temp (0600) -> write -> fsync(temp) -> rename ->
// fsync(parent directory). The target file is never truncated in place and a
// crash at any step leaves either the old or the new complete content.
func WriteFileAtomic(target string, data []byte) error {
	dir := filepath.Dir(target)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("filestore: create temp for %s: %w", target, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		temp.Close()
		os.Remove(tempName)
	}
	if err := temp.Chmod(FilePerm); err != nil {
		cleanup()
		return fmt.Errorf("filestore: chmod temp: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("filestore: write temp: %w", err)
	}
	if hookBeforeTempFsync != nil {
		if err := hookBeforeTempFsync(target); err != nil {
			cleanup()
			return err
		}
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("filestore: fsync temp: %w", err)
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("filestore: close temp: %w", err)
	}
	if hookBeforeRename != nil {
		if err := hookBeforeRename(tempName, target); err != nil {
			os.Remove(tempName)
			return err
		}
	}
	if err := os.Rename(tempName, target); err != nil {
		os.Remove(tempName)
		return fmt.Errorf("filestore: rename temp into place: %w", err)
	}
	if hookBeforeParentSync != nil {
		if err := hookBeforeParentSync(dir); err != nil {
			return err
		}
	}
	if err := SyncDir(dir); err != nil {
		return err
	}
	return nil
}

// SyncDir fsyncs a directory so a prior rename in it is durable. On Windows
// directory handles cannot be fsynced; rename durability is provided by the
// filesystem (MoveFileEx semantics) and this is a documented platform no-op,
// not a silent downgrade of the general contract.
func SyncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("filestore: open dir for fsync: %w", err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("filestore: fsync dir %s: %w", dir, err)
	}
	return nil
}

// VersionedDocument is the envelope for revision-CAS JSON authorities.
type VersionedDocument[T any] struct {
	Revision uint64 `json:"revision"`
	Value    T      `json:"value"`
}

// ReadJSON loads a JSON document. A missing file returns os.ErrNotExist.
// Corrupt JSON is reported as an error and must never be treated as an empty
// store by callers.
func ReadJSON[T any](path string, out *T) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("filestore: corrupt JSON at %s (never overwrite as empty): %w", path, err)
	}
	return nil
}

// ReplaceJSONWithRevision performs a revision compare-and-swap update of a
// versioned JSON document while the caller holds the appropriate lock:
// re-read latest -> verify expectedRevision -> mutate -> atomic replace.
// expectedRevision 0 requires that the document does not exist yet.
func ReplaceJSONWithRevision[T any](path string, expectedRevision uint64, mutate func(current T) (T, error)) (VersionedDocument[T], error) {
	var doc VersionedDocument[T]
	err := ReadJSON(path, &doc)
	switch {
	case err == nil:
		if doc.Revision != expectedRevision {
			return VersionedDocument[T]{}, fmt.Errorf("%w: have %d, expected %d", ErrRevisionConflict, doc.Revision, expectedRevision)
		}
	case os.IsNotExist(err):
		if expectedRevision != 0 {
			return VersionedDocument[T]{}, fmt.Errorf("%w: document missing, expected revision %d", ErrRevisionConflict, expectedRevision)
		}
	default:
		return VersionedDocument[T]{}, err
	}
	next, err := mutate(doc.Value)
	if err != nil {
		return VersionedDocument[T]{}, err
	}
	updated := VersionedDocument[T]{Revision: expectedRevision + 1, Value: next}
	encoded, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return VersionedDocument[T]{}, fmt.Errorf("filestore: encode %s: %w", path, err)
	}
	if err := WriteFileAtomic(path, append(encoded, '\n')); err != nil {
		return VersionedDocument[T]{}, err
	}
	return updated, nil
}
