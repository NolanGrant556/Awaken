package filestore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// WithPathLock coordinates cooperating Awaken writers across sessions and
// processes. The lock file is not the target inode: atomic replacement of the
// target must not replace its lock. Lock files remain until OS temp cleanup;
// unlinking them on release would let waiters lock different inodes.
func WithPathLock(ctx context.Context, path string, write func() error) error {
	key, err := pathLockIdentity(path)
	if err != nil {
		return err
	}
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("awaken-file-locks-%d", os.Getuid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return fmt.Errorf("filestore: path lock directory is not private")
	}
	name := filepath.Join(dir, fmt.Sprintf("%x.lock", sha256.Sum256([]byte(key))))
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		locked, err := tryPathLock(file)
		if err != nil {
			return err
		}
		if locked {
			return write()
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func pathLockIdentity(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ancestor := absolute
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			if runtime.GOOS == "windows" {
				resolved = strings.ToLower(resolved)
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(ancestor) == ancestor {
			return "", err
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
}
