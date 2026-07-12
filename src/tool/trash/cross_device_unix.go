//go:build !windows

package trash

import (
	"errors"
	"syscall"
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
