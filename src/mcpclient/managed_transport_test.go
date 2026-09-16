package mcpclient

import (
	"strings"
	"testing"
)

func TestMissingManagerDoesNotRunUnmanagedPackageCommand(t *testing.T) {
	_, _, err := buildTransport(McpServerConfig{Stdio: &StdioTransportConfig{Command: "npx", Args: []string{"-y", "@playwright/mcp"}}}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "environment_manager_unavailable") {
		t.Fatalf("unexpected transport result: %v", err)
	}
}
