//go:build darwin

package tool

import (
	"os"
	"syscall"
)

func isSystemOwnedExecutable(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}
