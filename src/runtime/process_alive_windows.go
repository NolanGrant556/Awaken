//go:build windows

package runtime

import (
	"golang.org/x/sys/windows"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// A terminated Windows process object can remain open while another process
	// still holds a handle. OpenProcess succeeding therefore does not prove that
	// the process is running; check its exit code as well.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	const stillActive = 259
	return exitCode == stillActive
}
