// Package filestore provides the durable-file primitives used by the
// current FileStore authority documented in
// docs/03_运行时与本地服务/数据存储与重放/实现方案.md:
// atomic JSON replacement with parent-directory fsync, fsynced append-only
// journals with committed-offset tail recovery, OS-backed cross-process
// locks, revision compare-and-swap, format-version gating and strict 0700/0600
// permissions. Platforms that cannot satisfy the contract fail closed; the
// package never degrades to plain overwrite writes or lock-free mutation.
package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DirPerm is the required mode for authority-holding directories.
	DirPerm os.FileMode = 0o700
	// FilePerm is the required mode for authority JSON/JSONL/ciphertext files.
	FilePerm os.FileMode = 0o600
)

var (
	ErrRevisionConflict   = errors.New("filestore: revision conflict")
	ErrLockHeld           = errors.New("filestore: lock is held by another owner")
	ErrFormatIncompatible = errors.New("filestore: store format is not compatible with this build")
	ErrCorruptJournal     = errors.New("filestore: journal is corrupt before its tail")
	ErrPathEscapesRoot    = errors.New("filestore: path escapes the store root")
)

// AwakenHomeEnv is the explicit override consumed only by tests and explicit
// development fixtures. The production Desktop Main resolves the user home
// itself and injects the result; it must not trust arbitrary inherited env.
const AwakenHomeEnv = "AWAKEN_HOME"

// ResolveAwakenHome returns the business data root. explicit (if non-empty)
// wins, then the AWAKEN_HOME environment variable, then ~/.awaken.
func ResolveAwakenHome(explicit string) (string, error) {
	if root := strings.TrimSpace(explicit); root != "" {
		return filepath.Clean(root), nil
	}
	if root := strings.TrimSpace(os.Getenv(AwakenHomeEnv)); root != "" {
		return filepath.Clean(root), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("filestore: resolve user home: %w", err)
	}
	return filepath.Join(home, ".awaken"), nil
}

// EnsureDir creates dir (and parents) with the contract directory mode.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, DirPerm)
}

// WithinRoot verifies that path, after resolving symlinks on its existing
// ancestors, stays inside root. It fails closed on traversal or symlink swap.
func WithinRoot(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("filestore: resolve root: %w", err)
	}
	target := filepath.Clean(path)
	// Resolve the deepest existing ancestor so a symlinked intermediate
	// directory cannot smuggle the target outside the root.
	probe := target
	for {
		if resolved, err := filepath.EvalSymlinks(probe); err == nil {
			rel, relErr := filepath.Rel(resolvedRoot, resolved)
			if relErr != nil {
				return ErrPathEscapesRoot
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return ErrPathEscapesRoot
			}
			return nil
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return ErrPathEscapesRoot
		}
		probe = parent
	}
}
