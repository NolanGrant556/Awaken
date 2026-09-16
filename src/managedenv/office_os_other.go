//go:build !darwin && !windows

package managedenv

import "errors"

func officeHostOSVersion() (string, error) {
	return "unknown", errors.New("Office platform not supported")
}
