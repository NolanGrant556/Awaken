//go:build !windows

package runtime

import (
	"fmt"
	"os"
	"syscall"
)

func projectDirectoryIdentity(_ string, info os.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("directory identity is unavailable")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
