package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"awaken/src/mcpclient"
	"awaken/src/tool"
)

// mcpHandlers wires the /v1/mcp/* HTTP surface to an mcpclient.Manager.
type mcpHandlers struct {
	manager *mcpclient.Manager
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
	if err := h.manager.Enable(r.Context(), name); err != nil {
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
