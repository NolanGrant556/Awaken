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

// HookConfigPath returns the only user-scoped Hook configuration path.
func HookConfigPath(root string) string {
	return filepath.Join(filepath.Clean(root), "config", "hooks.json")
}

// HookTrustPath returns the project Hook trust authority path.
func HookTrustPath(root string) string {
	return filepath.Join(filepath.Clean(root), "config", "hook-trust.json")
}

// PluginsConfigPath returns the only user-scoped plugin state file under an
// already-resolved Awaken business-data root.
func PluginsConfigPath(root string) string {
	return filepath.Join(filepath.Clean(root), "config", "plugins.json")
}

// PluginsRoot returns the installed plugin packages/data tree.
func PluginsRoot(root string) string {
	return filepath.Join(filepath.Clean(root), "plugins")
}

// SessionsRoot is the single subtree of the Awaken business-data root that the
// agent is allowed to mutate. Everything else under the root (config,
// credentials, store generations, user memory) stays protected, so this path is
// the only carve-out the task-scope protected-root check honours.
func SessionsRoot(root string) string {
	return filepath.Join(filepath.Clean(root), "sessions")
}

// SessionWorkspacePath is the working directory for an Awaken-managed run.
// ownerID is a stable project ID for a folder-less project, or a stable thread
// ID for a conversation that does not belong to a project. Runtime validates
// this identifier before using the returned path.
func SessionWorkspacePath(root, ownerID string) string {
	return filepath.Join(SessionsRoot(root), ownerID)
}

// SessionOutputPath is where an Awaken-managed run's produced files land.
func SessionOutputPath(root, ownerID string) string {
	return filepath.Join(SessionWorkspacePath(root, ownerID), "output")
}
