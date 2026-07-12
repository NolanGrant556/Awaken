// Package trash provides a unified interface for moving files to the
// operating system's recycle bin / trash instead of permanently deleting
// them. It is consumed by tools like apply_patch so that deletions remain
// recoverable.
package trash

import "errors"

// ErrCrossVolumeTrashUnsupported means the adapter could not provide an
// atomic move into the platform Trash. Callers must fail closed; copying then
// deleting the source would violate recoverable-delete semantics.
var ErrCrossVolumeTrashUnsupported = errors.New("cross-volume Trash move is unsupported")

// TrashAdapter is the interface for moving files to the OS recycle bin or
// trash directory. Implementations must be safe for concurrent use.
type TrashAdapter interface {
	TrashPath(path string) (TrashResult, error)
}

// TrashResult describes the outcome of a successful trash operation.
type TrashResult struct {
	// OriginalPath is the absolute path of the file before it was trashed.
	OriginalPath string
	// TrashedPath is the absolute path where the file ended up, if known.
	TrashedPath string
	// Method identifies the mechanism used, e.g. "rename", "nsworkspace",
	// "shfileoperation".
	Method string
}
