package plugins

import "os"

// Windows FileInfo.Sys does not expose syscall.Stat_t. Package extraction
// still rejects symlinks and special files; hardlink creation is not part of
// the copy path used by plugin installation on Windows.
func hasMultipleHardlinks(os.FileInfo) bool {
	return false
}
