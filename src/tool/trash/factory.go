package trash

import "sync"

var (
	adapterMu       sync.RWMutex
	adapterOverride TrashAdapter
)

// DefaultAdapter returns the platform-appropriate TrashAdapter. On macOS
// this moves files to ~/.Trash; on Windows to %USERPROFILE%\.trash; on
// other platforms it returns a fail-closed adapter that refuses to delete.
// If an override has been installed via SetAdapter, that override is
// returned instead.
func DefaultAdapter() TrashAdapter {
	adapterMu.RLock()
	override := adapterOverride
	adapterMu.RUnlock()
	if override != nil {
		return override
	}
	return defaultPlatformAdapter()
}

// SetAdapter installs a process-wide override returned by DefaultAdapter.
// Passing nil clears the override so the platform default is used again.
// This is primarily intended for tests.
func SetAdapter(a TrashAdapter) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapterOverride = a
}

// GetAdapter returns the currently active adapter, honoring any override
// set via SetAdapter. It is equivalent to DefaultAdapter.
func GetAdapter() TrashAdapter {
	return DefaultAdapter()
}
