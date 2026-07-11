//go:build !windows

package officerender

import "os"

func replaceFileAtomically(source, destination string) error {
	return os.Rename(source, destination)
}
