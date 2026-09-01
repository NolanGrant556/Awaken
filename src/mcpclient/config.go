package mcpclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"awaken/src/desktopdata"
	"awaken/src/filestore"
)

// McpServerConfig is the wire shape of a single server entry in project
// awaken.mcp.json or user ~/.awaken/config/mcp-servers.json. Exactly one
// transport variant field must be set.
type McpServerConfig struct {
	Stdio  *StdioTransportConfig  `json:"stdio,omitempty"`
	HTTP   *HTTPTransportConfig   `json:"http,omitempty"`
	SSE    *HTTPTransportConfig   `json:"sse,omitempty"`
	WS     *HTTPTransportConfig   `json:"ws,omitempty"`
	InProc *InProcTransportConfig `json:"inProcess,omitempty"`

	Enabled            *bool  `json:"enabled,omitempty"`
	DisplayName        string `json:"displayName,omitempty"`
	HeadersHelper      string `json:"headersHelper,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
	// OAuthResource is the protected resource URI declared by an MCP source.
	// It affects OAuth discovery and credential identity, not the transport endpoint.
	OAuthResource string `json:"oauthResource,omitempty"`
	// OAuthClientID is an optional public pre-registered client identifier.
	OAuthClientID string `json:"oauthClientId,omitempty"`
	// OAuthClientMetadataURL is an optional public HTTPS Client ID Metadata
	// Document URL used only when the authorization server advertises CIMD.
	OAuthClientMetadataURL string   `json:"oauthClientMetadataUrl,omitempty"`
	OAuthScopes            []string `json:"oauthScopes,omitempty"`
	// DisabledTools is a host-owned per-server projection used by Plugin
	// component overrides. Disabled tools remain visible to management APIs but
	// are not registered into the next run's central Tool Catalog snapshot.
	DisabledTools []string `json:"disabledTools,omitempty"`
	// Provider is stable connector/provider identity. Host-mediated Artifact
	// publication remains per-tool MCP _meta and must never be inherited from
	// this server-level declaration.
	Provider *ServerProviderManifest `json:"provider,omitempty"`
	// SecretBindings are Core-owned opaque references resolved immediately
	// before a transport or child process starts. They are never persisted or
	// returned by management DTOs.
	SecretBindings []RuntimeSecretBinding `json:"-"`
}

type RuntimeSecretBinding struct {
	Target string
	Name   string
	Ref    string
}

// ScopedMcpServerConfig pairs a server config with the scope it came from.
type ScopedMcpServerConfig struct {
	Name  string
	Scope Scope
	// SourceRevision identifies runtime-affecting source material that is not
	// represented in Config. Plugin projections use the package digest so a
	// same-path binary replacement rebuilds the server, while unrelated catalog
	// mutations with the same package digest preserve the live session.
	SourceRevision string
	Config         McpServerConfig
}

// StdioTransportConfig describes a child-process MCP server.
type StdioTransportConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	// PluginRoot and PluginData are host-owned reserved environment values
	// injected after configured env. They are never persisted.
	PluginRoot string `json:"-"`
	PluginData string `json:"-"`
}

// HTTPTransportConfig covers streamable HTTP, SSE, and WebSocket variants.
type HTTPTransportConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// InProcTransportConfig wires a local Go-implemented MCP server by name.
type InProcTransportConfig struct {
	ServerName string `json:"serverName"`
}

// ConfigFile is the on-disk JSON shape for both project and user configs.
type ConfigFile struct {
	Servers map[string]McpServerConfig `json:"servers"`
}

// LoadProjectConfig reads ./awaken.mcp.json from the given workspace root.
// A missing file returns an empty result with no error.
func LoadProjectConfig(workspaceRoot string) (ConfigFile, error) {
	path := filepath.Join(workspaceRoot, "awaken.mcp.json")
	return loadConfigFile(path)
}

// SaveProjectConfig writes ./awaken.mcp.json in the given workspace root.
func SaveProjectConfig(workspaceRoot string, cfg ConfigFile) error {
	if strings.TrimSpace(workspaceRoot) == "" {
		return fmt.Errorf("workspace root is required")
	}
	path := filepath.Join(workspaceRoot, "awaken.mcp.json")
	return withConfigFileLock(path, func() error {
		return saveConfigFile(path, cfg, "project")
	})
}

// LoadUserConfig reads ~/.awaken/config/mcp-servers.json. A missing file
// returns an empty result with no error. There is deliberately no legacy
// import or fallback path.
func LoadUserConfig() (ConfigFile, error) {
	return loadConfigFile(UserConfigPath())
}

// SaveUserConfig writes cfg to ~/.awaken/config/mcp-servers.json, creating the
// parent directory if necessary.
func SaveUserConfig(cfg ConfigFile) error {
	path := UserConfigPath()
	if err := ensurePrivateConfigDir(filepath.Dir(path)); err != nil {
		return err
	}
	return withConfigFileLock(path, func() error {
		return saveConfigFile(path, cfg, "user")
	})
}

// MutateUserConfig serializes a read-modify-write transaction across Awaken
// processes. The latest complete file is read only after the cross-process
// lock is held, then atomically replaced with the mutated document.
func MutateUserConfig(mutate func(*ConfigFile) (bool, error)) error {
	if mutate == nil {
		return fmt.Errorf("mutate user mcp config: callback is required")
	}
	path := UserConfigPath()
	if err := ensurePrivateConfigDir(filepath.Dir(path)); err != nil {
		return err
	}
	return withConfigFileLock(path, func() error {
		cfg, err := loadConfigFile(path)
		if err != nil {
			return err
		}
		if cfg.Servers == nil {
			cfg.Servers = map[string]McpServerConfig{}
		}
		changed, err := mutate(&cfg)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return saveConfigFile(path, cfg, "user")
	})
}

// MutateProjectConfig provides the same locked, atomic read-modify-write
// semantics for <workspace>/awaken.mcp.json.
func MutateProjectConfig(workspaceRoot string, mutate func(*ConfigFile) (bool, error)) error {
	if strings.TrimSpace(workspaceRoot) == "" {
		return fmt.Errorf("workspace root is required")
	}
	if mutate == nil {
		return fmt.Errorf("mutate project mcp config: callback is required")
	}
	path := filepath.Join(workspaceRoot, "awaken.mcp.json")
	return withConfigFileLock(path, func() error {
		cfg, err := loadConfigFile(path)
		if err != nil {
			return err
		}
		if cfg.Servers == nil {
			cfg.Servers = map[string]McpServerConfig{}
		}
		changed, err := mutate(&cfg)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return saveConfigFile(path, cfg, "project")
	})
}

func UserConfigPath() string {
	root := desktopdata.AwakenHomeRoot()
	if root == "" {
		// Standalone/test callers without an injected data root must not touch
		// the real home. Keep their config process-local and temporary.
		root = filepath.Join(os.TempDir(), fmt.Sprintf("awaken-mcp-config-%d", os.Getpid()))
	}
	return desktopdata.MCPConfigPath(root)
}

func ensurePrivateConfigDir(dir string) error {
	if err := os.MkdirAll(dir, filestore.DirPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, filestore.DirPerm); err != nil {
		return fmt.Errorf("chmod %s: %w", dir, err)
	}
	return nil
}

func withConfigFileLock(path string, action func() error) error {
	deadline := time.Now().Add(5 * time.Second)
	lockPath := path + ".lock"
	owner := fmt.Sprintf("awaken-mcp-config-%d", os.Getpid())
	for {
		lock, err := filestore.AcquireLock(lockPath, owner, 10*time.Second)
		if err == nil {
			actionErr := action()
			releaseErr := lock.Release()
			if actionErr != nil {
				return actionErr
			}
			if releaseErr != nil {
				return releaseErr
			}
			return nil
		}
		if !errors.Is(err, filestore.ErrLockHeld) || time.Now().After(deadline) {
			return fmt.Errorf("lock %s: %w", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func saveConfigFile(path string, cfg ConfigFile, scope string) error {
	if cfg.Servers == nil {
		cfg.Servers = map[string]McpServerConfig{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize %s mcp config: %w", scope, err)
	}
	if err := filestore.WriteFileAtomic(path, append(data, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func loadConfigFile(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ConfigFile{}, nil
		}
		return ConfigFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ConfigFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// MergeScoped combines plugin, user, project, and dynamic-scoped server lists.
// Later scopes override earlier ones on name collision: plugin is lowest,
// then user, then project, then dynamic. Invalid entries are dropped with a
// warning logged via the provided logger (or silently if nil). Disabled
// entries are retained so the manager can restore their disabled state after
// a process restart without attempting a connection.
func MergeScoped(
	plugin []ScopedMcpServerConfig,
	user []ScopedMcpServerConfig,
	project []ScopedMcpServerConfig,
	dynamic []ScopedMcpServerConfig,
	logger Logger,
) []ScopedMcpServerConfig {
	merged := make(map[string]ScopedMcpServerConfig, len(plugin)+len(user)+len(project)+len(dynamic))
	add := func(scoped []ScopedMcpServerConfig) {
		for _, entry := range scoped {
			if !entry.Config.Valid() {
				if logger != nil {
					logger.Warnf("mcp server %q has invalid config, skipping", entry.Name)
				}
				continue
			}
			if existing, ok := merged[entry.Name]; ok {
				if logger != nil {
					logger.Debugf("mcp server %q overridden by %s scope (was %s)", entry.Name, entry.Scope, existing.Scope)
				}
			}
			if entry.Scope == ScopePlugin {
				// Agent Plugins configuration has already received the only two
				// portable substitutions permitted by the specification. Keeping
				// it literal here prevents native-config ${VAR} expansion from
				// mutating plugin commands, URLs, headers, args, env, or cwd.
				merged[entry.Name] = entry
			} else {
				merged[entry.Name] = expandEnvInScoped(entry)
			}
		}
	}
	add(plugin)
	add(user)
	add(project)
	add(dynamic)

	result := make([]ScopedMcpServerConfig, 0, len(merged))
	for _, entry := range merged {
		if entry.Config.Enabled != nil && !*entry.Config.Enabled {
			if logger != nil {
				logger.Debugf("mcp server %q retained in disabled state from config", entry.Name)
			}
		}
		result = append(result, entry)
	}
	return result
}

// ToScoped wraps a ConfigFile into a slice of ScopedMcpServerConfig.
func ToScoped(cfg ConfigFile, scope Scope) []ScopedMcpServerConfig {
	out := make([]ScopedMcpServerConfig, 0, len(cfg.Servers))
	for name, serverCfg := range cfg.Servers {
		out = append(out, ScopedMcpServerConfig{
			Name:   name,
			Scope:  scope,
			Config: serverCfg,
		})
	}
	return out
}

// TransportKind returns the transport variant declared by this config, or
// an empty string if the config is invalid (zero or multiple variants set).
func (c McpServerConfig) TransportKind() TransportKind {
	count := 0
	var kind TransportKind
	if c.Stdio != nil {
		count++
		kind = TransportStdio
	}
	if c.HTTP != nil {
		count++
		kind = TransportHTTP
	}
	if c.SSE != nil {
		count++
		kind = TransportSSE
	}
	if c.WS != nil {
		count++
		kind = TransportWS
	}
	if c.InProc != nil {
		count++
		kind = TransportInProc
	}
	if count != 1 {
		return ""
	}
	return kind
}

func (c McpServerConfig) Valid() bool {
	kind := c.TransportKind()
	if kind == "" {
		return false
	}
	switch kind {
	case TransportStdio:
		return c.Stdio != nil && strings.TrimSpace(c.Stdio.Command) != ""
	case TransportHTTP, TransportSSE, TransportWS:
		urlField := ""
		if c.HTTP != nil {
			urlField = c.HTTP.URL
		}
		if c.SSE != nil {
			urlField = c.SSE.URL
		}
		if c.WS != nil {
			urlField = c.WS.URL
		}
		return strings.TrimSpace(urlField) != ""
	case TransportInProc:
		return c.InProc != nil && strings.TrimSpace(c.InProc.ServerName) != ""
	}
	return false
}

func expandEnvInScoped(entry ScopedMcpServerConfig) ScopedMcpServerConfig {
	entry.Config.HeadersHelper = expandEnv(entry.Config.HeadersHelper)
	if entry.Config.Stdio != nil {
		entry.Config.Stdio.Command = expandEnv(entry.Config.Stdio.Command)
		entry.Config.Stdio.Cwd = expandEnv(entry.Config.Stdio.Cwd)
		expandedArgs := make([]string, len(entry.Config.Stdio.Args))
		for i, arg := range entry.Config.Stdio.Args {
			expandedArgs[i] = expandEnv(arg)
		}
		entry.Config.Stdio.Args = expandedArgs
		expandedEnv := make(map[string]string, len(entry.Config.Stdio.Env))
		for k, v := range entry.Config.Stdio.Env {
			expandedEnv[k] = expandEnv(v)
		}
		entry.Config.Stdio.Env = expandedEnv
	}
	for _, field := range []**HTTPTransportConfig{&entry.Config.HTTP, &entry.Config.SSE, &entry.Config.WS} {
		if *field == nil {
			continue
		}
		(*field).URL = expandEnv((*field).URL)
		if (*field).Headers != nil {
			expandedHeaders := make(map[string]string, len((*field).Headers))
			for k, v := range (*field).Headers {
				expandedHeaders[k] = expandEnv(v)
			}
			(*field).Headers = expandedHeaders
		}
	}
	if entry.Config.InProc != nil {
		entry.Config.InProc.ServerName = expandEnv(entry.Config.InProc.ServerName)
	}
	return entry
}

// expandEnv replaces ${VAR} patterns with the corresponding environment
// variable value. Unknown variables expand to empty string. On Windows,
// %VAR%-style references are not expanded to avoid ambiguity with paths.
func expandEnv(value string) string {
	if !strings.Contains(value, "${") {
		return value
	}
	var sb strings.Builder
	i := 0
	for i < len(value) {
		if i+1 < len(value) && value[i] == '$' && value[i+1] == '{' {
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				sb.WriteString(value[i:])
				break
			}
			name := value[i+2 : i+2+end]
			sb.WriteString(os.Getenv(name))
			i = i + 2 + end + 1
			continue
		}
		sb.WriteByte(value[i])
		i++
	}
	// Avoid unused-variable warning on non-windows builds.
	_ = runtime.GOOS
	return sb.String()
}
