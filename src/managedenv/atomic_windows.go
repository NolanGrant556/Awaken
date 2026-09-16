//go:build windows

package managedenv

import (
	"errors"
	"syscall"
	"time"

	"awaken/src/filestore"
)

// Antivirus/indexing handles can briefly deny replacement on Windows. Retry
// only those errors, bounded to 310 ms, without deleting the old destination.
func writeAtomicRecord(path string, data []byte) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		err = filestore.WriteFileAtomic(path, data)
		if err == nil {
			return nil
		}
		var errno syscall.Errno
		if !errors.As(err, &errno) || (errno != 5 && errno != 32 && errno != 33) {
			return err
		}
		if attempt < 5 {
			time.Sleep(time.Duration(10<<attempt) * time.Millisecond)
		}
	}
	return err
}
