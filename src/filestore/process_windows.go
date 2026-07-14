//go:build windows

package filestore

import "os"

func processAlive(pid int) bool {
	// os.FindProcess opens a real handle on Windows; failure means the
	// process cannot be addressed and its lock may be reclaimed.
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	process.Release()
	return true
}
