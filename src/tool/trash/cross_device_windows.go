//go:build windows

package trash

import (
	"errors"
	"syscall"
)

func isCrossDeviceError(err error) bool {
	const errorNotSameDevice syscall.Errno = 17 // ERROR_NOT_SAME_DEVICE
	return errors.Is(err, errorNotSameDevice)
}
