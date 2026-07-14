package desktopdata

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	awakenHomeEnv = "AWAKEN_HOME"
)

// AwakenHomeRoot resolves the canonical business-data root shared by Core
// FileStore and MCP credential metadata. An empty result means the caller must
// choose an isolated fallback and must not implicitly write the real home.
func AwakenHomeRoot() string {
	if root := strings.TrimSpace(os.Getenv(awakenHomeEnv)); root != "" {
		return filepath.Clean(root)
	}
	return ""
}

// MCPConfigPath returns the only user-scoped MCP server configuration path
// under an already-resolved Awaken business-data root.
func MCPConfigPath(root string) string {
	return filepath.Join(filepath.Clean(root), "config", "mcp-servers.json")
}
