//go:build !windows

package managedenv

import "awaken/src/filestore"

func writeAtomicRecord(path string, data []byte) error { return filestore.WriteFileAtomic(path, data) }
