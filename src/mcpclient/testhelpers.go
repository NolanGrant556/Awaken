package mcpclient

import (
	"errors"
	"time"

	"awaken/src/tool"
)

// UnregisterMcpToolsForTest removes every MCP-sourced tool registered by the
// named server from the global tool registry. Exported for use by
// integration tests in other packages (e.g. src/server smoke tests) that
// need a clean slate before driving the manager through a fresh connect
// cycle. Safe to call when no tools are registered.
func UnregisterMcpToolsForTest(serverName string) {
	tool.UnregisterMCPToolsByServer(serverName)
}

// WaitForServerStateForTest polls SnapshotServer until the named server
// reaches the target state or the timeout elapses. Exported for use by
// integration tests that need to wait for the manager's asynchronous
// connect loop to settle.
func WaitForServerStateForTest(mgr *Manager, name string, want ServerState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := mgr.SnapshotServer(name)
		if err == nil && snap.State == want {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("timeout waiting for state " + string(want))
}
