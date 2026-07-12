//go:build darwin

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TrashAdapterMac moves files to the user's macOS ~/.Trash directory using
// os.Rename. This is a pure-Go fallback that avoids cgo; a future
// implementation could call NSWorkspace's
// recycleUrlsFromURLs:completionHandler: via cgo for full Finder
// integration.
type TrashAdapterMac struct {
	// trashDir overrides the default ~/.Trash destination. When empty,
	// ~/.Trash is used. This is primarily for testing.
	trashDir string
	rename   func(oldPath, newPath string) error
}

func defaultPlatformAdapter() TrashAdapter {
	return TrashAdapterMac{}
}

func (a TrashAdapterMac) TrashPath(path string) (TrashResult, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return TrashResult{}, fmt.Errorf("trash: resolve absolute path for %q: %w", path, err)
	}

	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return TrashResult{}, fmt.Errorf("trash: path does not exist: %s", absPath)
		}
		return TrashResult{}, fmt.Errorf("trash: stat %s: %w", absPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return TrashResult{}, fmt.Errorf("trash: refusing symlink or special filesystem object: %s", absPath)
	}
	trashDir := a.trashDir
	if trashDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return TrashResult{}, fmt.Errorf("trash: resolve user home dir: %w", err)
		}
		trashDir = filepath.Join(home, ".Trash")
	}
	if err := os.MkdirAll(trashDir, 0o700); err != nil {
		return TrashResult{}, fmt.Errorf("trash: ensure trash dir %s: %w", trashDir, err)
	}

	base := filepath.Base(absPath)
	target := filepath.Join(trashDir, base)
	if _, err := os.Stat(target); err == nil {
		// Name collision — append a timestamp to disambiguate.
		stamp := time.Now().Format("20060102-150405.000")
		ext := filepath.Ext(base)
		nameWithoutExt := strings.TrimSuffix(base, ext)
		target = filepath.Join(trashDir, nameWithoutExt+"."+stamp+ext)
	}

	rename := a.rename
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(absPath, target); err != nil {
		if isCrossDeviceError(err) {
			return TrashResult{}, fmt.Errorf("trash: %w: %s to %s", ErrCrossVolumeTrashUnsupported, absPath, target)
		}
		return TrashResult{}, fmt.Errorf("trash: move %s to %s: %w", absPath, target, err)
	}

	return TrashResult{
		OriginalPath: absPath,
		TrashedPath:  target,
		Method:       "rename",
	}, nil
}
