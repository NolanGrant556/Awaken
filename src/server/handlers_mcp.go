package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

// mcpHandlers wires the /v1/mcp/* HTTP surface to an mcpclient.Manager.
type mcpHandlers struct {
	manager       *mcpclient.Manager
	workspaceRoot string
}

// mcpServerInfo is the wire shape for entries returned by GET /v1/mcp/servers.
type mcpServerInfo struct {
	Name              string                      `json:"name"`
	State             mcpclient.ServerState       `json:"state"`
	Transport         mcpclient.TransportKind     `json:"transport"`
	Scope             mcpclient.Scope             `json:"scope"`
	ErrorReason       string                      `json:"errorReason,omitempty"`
	LastConnectedAt   *int64                      `json:"lastConnectedAt,omitempty"`
	Capabilities      *mcpclient.ServerCapability `json:"capabilities,omitempty"`
	SourceConnectorID *string                     `json:"sourceConnectorId,omitempty"`
	ShadowedBy        string                      `json:"shadowedBy,omitempty"`
}

// mcpToolInfo is the wire shape for entries returned by GET /v1/mcp/tools.
// It surfaces both builtin and MCP-sourced tools so the desktop UI can
// render the unified tool pool.
type mcpToolInfo struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"readOnly"`
	// Annotations is populated only for MCP-sourced tools; for builtin
	// tools the ReadOnly field is derived from the tool.ReadOnlyTool
	// interface.
	Annotations *mcpclient.MCPToolAnnotations `json:"annotations,omitempty"`
}

type configureMCPServersRequest struct {
	ConfigText string `json:"configText"`
}

type configureMCPServersResponse struct {
	ServerNames []string `json:"serverNames"`
	ConfigPath  string   `json:"configPath,omitempty"`
	Message     string   `json:"message"`
}

type mcpServerConfigResponse struct {
	Name       string `json:"name"`
	ConfigText string `json:"configText"`
	Scope      string `json:"scope"`
}

type updateMcpServerConfigRequest struct {
	ConfigText string `json:"configText"`
}

type mcpConfigPayload struct {
	Servers    map[string]json.RawMessage `json:"servers"`
	McpServers map[string]json.RawMessage `json:"mcpServers"`
}

type mcpServerShorthand struct {
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	Cwd                string            `json:"cwd,omitempty"`
	URL                string            `json:"url,omitempty"`
	Type               string            `json:"type,omitempty"`
	Transport          string            `json:"transport,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	Enabled            *bool             `json:"enabled,omitempty"`
	DisplayName        string            `json:"displayName,omitempty"`
	HeadersHelper      string            `json:"headersHelper,omitempty"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify,omitempty"`
}

func (h mcpHandlers) handleListServers(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	snapshots := h.manager.Snapshot()
	out := make([]mcpServerInfo, 0, len(snapshots))
	for _, snap := range snapshots {
		info := mcpServerInfo{
			Name:              snap.Name,
			State:             snap.State,
			Transport:         snap.Transport,
			Scope:             snap.Scope,
			ErrorReason:       snap.ErrorReason,
			Capabilities:      snap.Capabilities,
			SourceConnectorID: snap.SourceConnectorID,
			ShadowedBy:        snap.ShadowedBy,
		}
		if snap.LastConnectedAt != nil {
			ts := snap.LastConnectedAt.UnixMilli()
			info.LastConnectedAt = &ts
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h mcpHandlers) handleConfigureServers(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	if strings.TrimSpace(h.workspaceRoot) == "" {
		writeErrorJSON(w, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace root not configured")
		return
	}

	var req configureMCPServersRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	servers, err := parseMCPServersConfig(req.ConfigText)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_MCP_CONFIG", err.Error())
		return
	}

	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "USER_CONFIG_INVALID", err.Error())
		return
	}
	if userCfg.Servers == nil {
		userCfg.Servers = map[string]mcpclient.McpServerConfig{}
	}
	for name, cfg := range servers {
		userCfg.Servers[name] = cfg
	}
	if err := mcpclient.SaveUserConfig(userCfg); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "USER_CONFIG_WRITE_FAILED", err.Error())
		return
	}

	serverNames := make([]string, 0, len(servers))
	for name, cfg := range servers {
		serverNames = append(serverNames, name)
		// Detach from the request context so the async connect loop keeps
		// running after the HTTP response is sent. Without this, request
		// cancellation would mask the real connection failure reason.
		h.manager.Start(context.WithoutCancel(r.Context()), []mcpclient.ScopedMcpServerConfig{{
			Name:   name,
			Scope:  mcpclient.ScopeUser,
			Config: cfg,
		}})
	}
	writeJSON(w, http.StatusOK, configureMCPServersResponse{
		ServerNames: serverNames,
		ConfigPath:  userMcpConfigPath(),
		Message:     fmt.Sprintf("configured %d MCP server(s)", len(serverNames)),
	})
}

func (h mcpHandlers) handleGetServerConfig(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	if strings.TrimSpace(h.workspaceRoot) == "" {
		writeErrorJSON(w, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace root not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "server name is required")
		return
	}

	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "USER_CONFIG_INVALID", err.Error())
		return
	}
	if cfg, ok := userCfg.Servers[name]; ok {
		writeServerConfig(w, name, mcpclient.ScopeUser, cfg)
		return
	}

	projectCfg, err := mcpclient.LoadProjectConfig(h.workspaceRoot)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "PROJECT_CONFIG_INVALID", err.Error())
		return
	}
	if cfg, ok := projectCfg.Servers[name]; ok {
		writeServerConfig(w, name, mcpclient.ScopeProject, cfg)
		return
	}

	writeErrorJSON(w, http.StatusNotFound, "SERVER_NOT_FOUND", fmt.Sprintf("mcp server %q not found", name))
}

func writeServerConfig(w http.ResponseWriter, name string, scope mcpclient.Scope, cfg mcpclient.McpServerConfig) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "SERIALIZE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, mcpServerConfigResponse{
		Name:       name,
		ConfigText: string(data),
		Scope:      string(scope),
	})
}

// userMcpConfigPath returns the absolute path to ~/.awaken/mcp.json.
func userMcpConfigPath() string {
	return mcpclient.UserConfigPath()
}

func (h mcpHandlers) handleUpdateServerConfig(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	if strings.TrimSpace(h.workspaceRoot) == "" {
		writeErrorJSON(w, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace root not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "server name is required")
		return
	}

	var req updateMcpServerConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	cfg, err := parseMCPServerEntry(json.RawMessage(req.ConfigText))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_MCP_CONFIG", err.Error())
		return
	}

	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "USER_CONFIG_INVALID", err.Error())
		return
	}
	if userCfg.Servers == nil {
		userCfg.Servers = map[string]mcpclient.McpServerConfig{}
	}
	userCfg.Servers[name] = cfg
	if err := mcpclient.SaveUserConfig(userCfg); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "USER_CONFIG_WRITE_FAILED", err.Error())
		return
	}

	if err := h.manager.RestartWithConfig(context.WithoutCancel(r.Context()), mcpclient.ScopedMcpServerConfig{
		Name:   name,
		Scope:  mcpclient.ScopeUser,
		Config: cfg,
	}); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "RESTART_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, configureMCPServersResponse{
		ServerNames: []string{name},
		ConfigPath:  userMcpConfigPath(),
		Message:     fmt.Sprintf("updated MCP server %q", name),
	})
}

func (h mcpHandlers) handleServerAuth(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "server name is required")
		return
	}
	var req mcpclient.AuthRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	result, err := h.manager.StartAuth(r.Context(), name, req)
	if err != nil {
		if errors.Is(err, mcpclient.ErrAuthFetcherNotInstalled) {
			writeErrorJSON(w, http.StatusConflict, "AUTH_FETCHER_REQUIRED", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusBadRequest, "AUTH_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h mcpHandlers) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	if strings.TrimSpace(h.workspaceRoot) == "" {
		writeErrorJSON(w, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace root not configured")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "server name is required")
		return
	}

	deleted := false
	userCfg, err := mcpclient.LoadUserConfig()
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "USER_CONFIG_INVALID", err.Error())
		return
	}
	if _, ok := userCfg.Servers[name]; ok {
		delete(userCfg.Servers, name)
		if err := mcpclient.SaveUserConfig(userCfg); err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, "USER_CONFIG_WRITE_FAILED", err.Error())
			return
		}
		deleted = true
	}

	if !deleted {
		projectCfg, err := mcpclient.LoadProjectConfig(h.workspaceRoot)
		if err != nil {
			writeErrorJSON(w, http.StatusBadRequest, "PROJECT_CONFIG_INVALID", err.Error())
			return
		}
		if _, ok := projectCfg.Servers[name]; ok {
			delete(projectCfg.Servers, name)
			if err := mcpclient.SaveProjectConfig(h.workspaceRoot, projectCfg); err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, "PROJECT_CONFIG_WRITE_FAILED", err.Error())
				return
			}
			deleted = true
		}
	}

	_, managerErr := h.manager.SnapshotServer(name)
	managerHas := managerErr == nil
	if !deleted && !managerHas {
		writeErrorJSON(w, http.StatusNotFound, "SERVER_NOT_FOUND", fmt.Sprintf("mcp server %q not found", name))
		return
	}

	_ = h.manager.Remove(name)
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func parseMCPServersConfig(configText string) (map[string]mcpclient.McpServerConfig, error) {
	trimmed := strings.TrimSpace(configText)
	if trimmed == "" {
		return nil, fmt.Errorf("config JSON is required")
	}
	var payload mcpConfigPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	servers := make(map[string]mcpclient.McpServerConfig)
	addEntries := func(entries map[string]json.RawMessage) error {
		for rawName, raw := range entries {
			name := safeMCPServerName(rawName)
			if name == "" {
				return fmt.Errorf("server name %q is invalid", rawName)
			}
			if _, exists := servers[name]; exists {
				return fmt.Errorf("server name %q collides after normalization", rawName)
			}
			cfg, err := parseMCPServerEntry(raw)
			if err != nil {
				return fmt.Errorf("server %q: %w", rawName, err)
			}
			if name != rawName && cfg.DisplayName == "" {
				cfg.DisplayName = rawName
			}
			servers[name] = cfg
		}
		return nil
	}
	if err := addEntries(payload.Servers); err != nil {
		return nil, err
	}
	if err := addEntries(payload.McpServers); err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("expected top-level servers or mcpServers")
	}
	return servers, nil
}

func parseMCPServerEntry(raw json.RawMessage) (mcpclient.McpServerConfig, error) {
	var native mcpclient.McpServerConfig
	if err := json.Unmarshal(raw, &native); err == nil && native.Valid() {
		return native, nil
	}

	var shorthand mcpServerShorthand
	if err := json.Unmarshal(raw, &shorthand); err != nil {
		return mcpclient.McpServerConfig{}, err
	}
	cfg := mcpclient.McpServerConfig{
		Enabled:            shorthand.Enabled,
		DisplayName:        shorthand.DisplayName,
		HeadersHelper:      shorthand.HeadersHelper,
		InsecureSkipVerify: shorthand.InsecureSkipVerify,
	}
	if strings.TrimSpace(shorthand.Command) != "" {
		cfg.Stdio = &mcpclient.StdioTransportConfig{
			Command: strings.TrimSpace(shorthand.Command),
			Args:    shorthand.Args,
			Env:     shorthand.Env,
			Cwd:     shorthand.Cwd,
		}
	} else if strings.TrimSpace(shorthand.URL) != "" {
		transport := strings.ToLower(strings.TrimSpace(shorthand.Transport))
		if transport == "" {
			transport = strings.ToLower(strings.TrimSpace(shorthand.Type))
		}
		if transport == "" {
			transport = string(mcpclient.TransportHTTP)
		}
		httpCfg := &mcpclient.HTTPTransportConfig{
			URL:     strings.TrimSpace(shorthand.URL),
			Headers: shorthand.Headers,
		}
		switch mcpclient.TransportKind(transport) {
		case mcpclient.TransportHTTP:
			cfg.HTTP = httpCfg
		case mcpclient.TransportSSE:
			cfg.SSE = httpCfg
		case mcpclient.TransportWS:
			cfg.WS = httpCfg
		default:
			return mcpclient.McpServerConfig{}, fmt.Errorf("unsupported transport %q", transport)
		}
	}
	if !cfg.Valid() {
		return mcpclient.McpServerConfig{}, fmt.Errorf("invalid config; use native servers.* transport blocks or mcpServers.* command/url shorthand")
	}
	return cfg, nil
}

func safeMCPServerName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		default:
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func (h mcpHandlers) handleServerDisable(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "server name is required")
		return
	}
	if err := h.manager.Disable(name); err != nil {
		if errors.Is(err, mcpclient.ErrServerNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "SERVER_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusConflict, "ILLEGAL_STATE", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h mcpHandlers) handleServerEnable(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MCP_UNAVAILABLE", "mcp manager not configured")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "server name is required")
		return
	}
	// Detach from the request context so the async connect loop keeps
	// running after the HTTP response is sent. Without this, request
	// cancellation would mask the real connection failure reason.
	if err := h.manager.Enable(context.WithoutCancel(r.Context()), name); err != nil {
		if errors.Is(err, mcpclient.ErrServerNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "SERVER_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusConflict, "ILLEGAL_STATE", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h mcpHandlers) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools := tool.ListTools()
	out := make([]mcpToolInfo, 0, len(tools))
	for _, t := range tools {
		info := mcpToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Source:      "builtin",
		}
		if readOnly, ok := t.(tool.ReadOnlyTool); ok && readOnly.IsReadOnly() {
			info.ReadOnly = true
		}
		if adapter, ok := t.(*mcpclient.McpToolAdapter); ok {
			annotations := adapter.Annotations()
			info.Source = "mcp"
			info.ReadOnly = annotations.ReadOnly
			info.Annotations = &annotations
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}

// Ensure the json import stays referenced even if future refactors drop
// the explicit Marshal call below.
var _ = json.Marshal
