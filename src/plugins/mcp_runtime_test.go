package plugins

import (
	"context"
	"sync"
	"testing"

	"awaken/src/mcpclient"
)

func TestReconcileMCPServersPreservesMatchingSessionsAndRemovesOnlyPluginOwnedServers(t *testing.T) {
	disabled := false
	var changesMu sync.Mutex
	var changes []string
	manager := mcpclient.NewManager(mcpclient.WithToolsChangedCallback(func(name string) {
		changesMu.Lock()
		changes = append(changes, name)
		changesMu.Unlock()
	}))
	pluginConfig := mcpclient.ScopedMcpServerConfig{
		Name:  "plugin__fixture__local",
		Scope: mcpclient.ScopePlugin,
		Config: mcpclient.McpServerConfig{
			HTTP:    &mcpclient.HTTPTransportConfig{URL: "https://fixture.invalid/mcp"},
			Enabled: &disabled,
		},
	}
	userConfig := mcpclient.ScopedMcpServerConfig{
		Name:  "user-server",
		Scope: mcpclient.ScopeUser,
		Config: mcpclient.McpServerConfig{
			HTTP:    &mcpclient.HTTPTransportConfig{URL: "https://user.invalid/mcp"},
			Enabled: &disabled,
		},
	}
	manager.Start(context.Background(), []mcpclient.ScopedMcpServerConfig{pluginConfig, userConfig})
	t.Cleanup(func() { manager.Shutdown(context.Background()) })

	if err := ReconcileMCPServers(context.Background(), manager, []mcpclient.ScopedMcpServerConfig{pluginConfig}); err != nil {
		t.Fatal(err)
	}
	changesMu.Lock()
	if len(changes) != 0 {
		t.Fatalf("matching plugin config was restarted: %v", changes)
	}
	changesMu.Unlock()

	if err := ReconcileMCPServers(context.Background(), manager, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SnapshotServer(pluginConfig.Name); err == nil {
		t.Fatal("removed plugin server remained in manager")
	}
	if _, err := manager.SnapshotServer(userConfig.Name); err != nil {
		t.Fatalf("non-plugin server was removed: %v", err)
	}
	changesMu.Lock()
	defer changesMu.Unlock()
	if len(changes) != 1 || changes[0] != pluginConfig.Name {
		t.Fatalf("remove notifications=%v", changes)
	}
}
