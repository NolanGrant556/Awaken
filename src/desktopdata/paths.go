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

// SessionsRoot is the single subtree of the Awaken business-data root that the
// agent is allowed to mutate. Everything else under the root (config,
// credentials, store generations, user memory) stays protected, so this path is
// the only carve-out the task-scope protected-root check honours.
func SessionsRoot(root string) string {
	return filepath.Join(filepath.Clean(root), "sessions")
}

// SessionWorkspacePath is the working directory a run falls back to when its
// project has no folder attached. It gives the run a Bash cwd and a base for
// relative paths without asking the user to pick a folder first.
//
// The directory is keyed by project rather than by thread because the workspace
// root is what identifies a thread in the run service; a per-thread directory
// would mint a new identity on every conversation. Threads in the same
// folder-less project therefore share one place to keep what they produce.
func SessionWorkspacePath(root, projectID string) string {
	return filepath.Join(SessionsRoot(root), projectID)
}

// SessionOutputPath is where a folder-less run's produced files land.
func SessionOutputPath(root, projectID string) string {
	return filepath.Join(SessionWorkspacePath(root, projectID), "output")
}
