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
)

// McpServerConfig is the wire shape of a single server entry in
// awaken.mcp.json or ~/.awaken/mcp.json. Exactly one transport variant
// field must be set.
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
}

// ScopedMcpServerConfig pairs a server config with the scope it came from.
type ScopedMcpServerConfig struct {
	Name   string
	Scope  Scope
	Config McpServerConfig
}

// StdioTransportConfig describes a child-process MCP server.
type StdioTransportConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
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
	if cfg.Servers == nil {
		cfg.Servers = map[string]McpServerConfig{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize project mcp config: %w", err)
	}
	path := filepath.Join(workspaceRoot, "awaken.mcp.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LoadUserConfig reads ~/.awaken/mcp.json. A missing file returns an empty
// result with no error.
func LoadUserConfig() (ConfigFile, error) {
	path := UserConfigPath()
	if err := migrateLegacyUserConfig(path); err != nil {
		return ConfigFile{}, err
	}
	return loadConfigFile(path)
}

// SaveUserConfig writes cfg to ~/.awaken/mcp.json, creating the parent
// directory if necessary.
func SaveUserConfig(cfg ConfigFile) error {
	if cfg.Servers == nil {
		cfg.Servers = map[string]McpServerConfig{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize user mcp config: %w", err)
	}
	path := UserConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func UserConfigPath() string {
	return desktopdata.Detect().MCPConfigPath()
}

func LegacyUserConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".awaken", "mcp.json")
	}
	return filepath.Join(home, ".awaken", "mcp.json")
}

func migrateLegacyUserConfig(targetPath string) error {
	if strings.TrimSpace(targetPath) == "" {
		return fmt.Errorf("desktop mcp config path is required")
	}
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", targetPath, err)
	}
	legacyPath := LegacyUserConfigPath()
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read legacy mcp config %s: %w", legacyPath, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(targetPath), err)
	}
	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		return fmt.Errorf("write migrated mcp config %s: %w", targetPath, err)
	}
	paths := desktopdata.Detect()
	if err := paths.Ensure(); err == nil {
		marker := map[string]string{
			"migrated_at": time.Now().UTC().Format(time.RFC3339Nano),
			"source":      legacyPath,
			"target":      targetPath,
		}
		if markerData, marshalErr := json.MarshalIndent(marker, "", "  "); marshalErr == nil {
			_ = os.WriteFile(paths.MigrationMarkerPath("mcp-servers-legacy-migrated"), append(markerData, '\n'), 0o600)
		}
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

// MergeScoped combines user, project, and dynamic-scoped server lists.
// Project scope overrides user scope on name collision; dynamic scope is
// applied last and overrides both. Invalid entries are dropped with a
// warning logged via the provided logger (or silently if nil).
func MergeScoped(
	user []ScopedMcpServerConfig,
	project []ScopedMcpServerConfig,
	dynamic []ScopedMcpServerConfig,
	logger Logger,
) []ScopedMcpServerConfig {
	merged := make(map[string]ScopedMcpServerConfig, len(user)+len(project)+len(dynamic))
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
			merged[entry.Name] = expandEnvInScoped(entry)
		}
	}
	add(user)
	add(project)
	add(dynamic)

	result := make([]ScopedMcpServerConfig, 0, len(merged))
	for _, entry := range merged {
		enabled := true
		if entry.Config.Enabled != nil {
			enabled = *entry.Config.Enabled
		}
		if !enabled {
			if logger != nil {
				logger.Debugf("mcp server %q disabled by config", entry.Name)
			}
			continue
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
