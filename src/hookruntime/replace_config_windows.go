//go:build windows

package hookruntime

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func replaceConfigFile(source, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return fmt.Errorf("convert source path: %w", err)
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("convert target path: %w", err)
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// MoveFileEx with MOVEFILE_WRITE_THROUGH already requests durable completion.
func syncConfigDirectory(string) error { return nil }
