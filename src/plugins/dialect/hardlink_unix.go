//go:build !windows

package dialect

import (
	"os"
	"syscall"
)

func hasMultipleHardlinks(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink > 1
}
