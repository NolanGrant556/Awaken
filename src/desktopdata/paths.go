package desktopdata

import (
	"os"
	"path/filepath"
	"strings"
)

const userDataDirEnv = "AWAKEN_USER_DATA_DIR"

// Paths centralizes every Desktop-global storage location used by the
// runtime and MCP subsystems.
type Paths struct {
	root string
}

func FromRoot(root string) Paths {
	return Paths{root: filepath.Clean(root)}
}

// Detect returns the resolved Desktop-global data root. Desktop launches the
// runtime with AWAKEN_USER_DATA_DIR; non-Desktop callers fall back to the
// platform user config directory.
func Detect() Paths {
	if root := strings.TrimSpace(os.Getenv(userDataDirEnv)); root != "" {
		return Paths{root: filepath.Clean(root)}
	}
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return Paths{root: filepath.Join(dir, "@awaken", "desktop")}
	}
	return Paths{root: filepath.Join(os.TempDir(), "awaken-desktop")}
}

func (p Paths) Root() string {
	return p.root
}

func (p Paths) ConfigDir() string {
	return filepath.Join(p.root, "config")
}

func (p Paths) DataDir() string {
	return filepath.Join(p.root, "data")
}

func (p Paths) ThreadsDir() string {
	return filepath.Join(p.DataDir(), "threads")
}

func (p Paths) ThreadsDBPath() string {
	return filepath.Join(p.DataDir(), "threads.sqlite")
}

func (p Paths) ThreadTranscriptPath(threadID string) string {
	return filepath.Join(p.ThreadsDir(), "thread_"+safeFileName(threadID)+".jsonl")
}

func (p Paths) AuthDir() string {
	return filepath.Join(p.DataDir(), "auth")
}

func (p Paths) ConnectorAccountsDBPath() string {
	return filepath.Join(p.AuthDir(), "connector-accounts.sqlite")
}

func (p Paths) MCPConfigPath() string {
	return filepath.Join(p.ConfigDir(), "mcp-servers.json")
}

func (p Paths) MigrationDir() string {
	return filepath.Join(p.DataDir(), "migrations")
}

func (p Paths) MigrationMarkerPath(name string) string {
	return filepath.Join(p.MigrationDir(), safeFileName(name)+".json")
}

// Ensure creates the standard Desktop-global directory tree.
func (p Paths) Ensure() error {
	for _, dir := range []string{p.ConfigDir(), p.DataDir(), p.ThreadsDir(), p.AuthDir(), p.MigrationDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func safeFileName(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", string(filepath.Separator), "_")
	value = replacer.Replace(value)
	value = strings.ReplaceAll(value, "..", "_")
	if value == "" {
		return "item"
	}
	return value
}
