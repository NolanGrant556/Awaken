//go:build !darwin && !windows

package trash

import "fmt"

// TrashAdapterOther is the fail-closed fallback for platforms without a
// dedicated trash adapter. It refuses to trash files rather than silently
// permanently deleting them.
type TrashAdapterOther struct{}

func defaultPlatformAdapter() TrashAdapter {
	return TrashAdapterOther{}
}

func (TrashAdapterOther) TrashPath(path string) (TrashResult, error) {
	return TrashResult{}, fmt.Errorf("trash: unsupported on this platform; refusing to permanently delete %s", path)
}
