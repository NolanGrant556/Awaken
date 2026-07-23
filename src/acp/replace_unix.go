//go:build !windows

package acp

import "os"

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
