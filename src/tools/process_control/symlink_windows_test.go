//go:build windows

package processcontrol

import "os"

func createSymlink(oldname string, newname string) error {
	return os.Symlink(oldname, newname)
}
