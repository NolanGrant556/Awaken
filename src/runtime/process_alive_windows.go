//go:build windows

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
	// Signal 0 on Windows does not reliably check existence; use OpenProcess.
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(proc.Pid))
	if err != nil {
		return false
	}
	_ = syscall.CloseHandle(handle)
	return true
}
