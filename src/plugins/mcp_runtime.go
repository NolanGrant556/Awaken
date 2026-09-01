package plugins

import (
	"context"
	"errors"
	"fmt"

	"awaken/src/mcpclient"
)

// ReconcileMCPServers projects the desired plugin-scope MCP configuration
// without disturbing already-matching sessions. Plugin mutations often affect
// only Skills, setup, or parent visibility; those mutations must not restart
// unrelated MCP servers.
func ReconcileMCPServers(ctx context.Context, manager *mcpclient.Manager, desired []mcpclient.ScopedMcpServerConfig) error {
	if manager == nil {
		return nil
	}
	desiredNames := make(map[string]mcpclient.ScopedMcpServerConfig, len(desired))
	for _, scoped := range desired {
		desiredNames[scoped.Name] = scoped
	}

	var reconcileErrors []error
	for _, snapshot := range manager.Snapshot() {
		if !IsPluginServerIdentity(snapshot.Name) {
			continue
		}
		if _, ok := desiredNames[snapshot.Name]; ok {
			continue
		}
		if err := manager.Remove(snapshot.Name); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("remove plugin MCP server %s: %w", snapshot.Name, err))
		}
	}
	for _, scoped := range desired {
		if _, err := manager.ApplyConfig(ctx, scoped); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("apply plugin MCP server %s: %w", scoped.Name, err))
		}
	}
	return errors.Join(reconcileErrors...)
}
