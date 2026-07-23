//go:build !windows

package runtime

import (
	"os"
	"syscall"
)

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks process existence without delivering a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}
