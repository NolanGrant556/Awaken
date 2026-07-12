//go:build !darwin

package tool

import "os"

func isSystemOwnedExecutable(os.FileInfo) bool { return false }
