//go:build windows

package cua

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsPoint struct {
	X int32
	Y int32
}

func observedCursorPosition() (int, int, bool) {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("GetCursorPos")
	var point windowsPoint
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&point)))
	return int(point.X), int(point.Y), ok != 0
}

func observedForegroundWindow() (uintptr, bool) {
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("GetForegroundWindow")
	handle, _, _ := proc.Call()
	return handle, handle != 0
}
