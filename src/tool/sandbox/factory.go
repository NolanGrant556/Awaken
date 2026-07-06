package sandbox

import "sync"

var (
	adapterMu       sync.RWMutex
	adapterOverride SandboxAdapter
)

// DefaultAdapter returns the platform-appropriate SandboxAdapter. On macOS
// and Windows this is currently a degraded placeholder; on other platforms
// it reports no sandbox support. If an override has been installed via
// SetAdapter, that override is returned instead.
//
// This mirrors the trash package's factory pattern so tests can inject
// mocks via SetAdapter.
func DefaultAdapter() SandboxAdapter {
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
func SetAdapter(a SandboxAdapter) {
	adapterMu.Lock()
	defer adapterMu.Unlock()
	adapterOverride = a
}

// GetAdapter returns the currently active adapter, honoring any override
// set via SetAdapter. It is equivalent to DefaultAdapter.
func GetAdapter() SandboxAdapter {
	return DefaultAdapter()
}
