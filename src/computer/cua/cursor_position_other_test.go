//go:build !windows

package cua

func observedCursorPosition() (int, int, bool) {
	return 0, 0, false
}
