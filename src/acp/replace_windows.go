//go:build windows

package acp

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func replaceFile(src, dst string) error {
	from, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return fmt.Errorf("convert src: %w", err)
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return fmt.Errorf("convert dst: %w", err)
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
