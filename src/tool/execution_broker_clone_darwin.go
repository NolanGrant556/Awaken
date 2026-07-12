//go:build darwin

package tool

import (
	"errors"

	"golang.org/x/sys/unix"
)

// APFS clonefile gives the staging broker a copy-on-write snapshot without
// reading/copying every byte in large workspaces. Unsupported/cross-volume
// filesystems fall back to the bounded Go copy path.
func cloneWorkspaceRegularFile(source string, destination string) (bool, error) {
	err := unix.Clonefile(source, destination, 0)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
		return false, nil
	}
	return false, err
}
