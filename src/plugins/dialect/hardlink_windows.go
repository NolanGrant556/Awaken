//go:build windows

package dialect

import "os"

func hasMultipleHardlinks(os.FileInfo) bool {
	return false
}
