package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"awaken/src/connectors"
)

// connectorHandlers wires the /v1/connectors/* HTTP surface to a
// connectors.Registry.
type connectorHandlers struct {
	registry *connectors.Registry
}

// connectorInfo is the wire shape for entries returned by GET /v1/connectors.
type connectorInfo struct {
	ID           string                    `json:"id"`
	DisplayName  string                    `json:"displayName"`
	Category     connectors.Category       `json:"category"`
	Provider     string                    `json:"provider,omitempty"`
	State        connectors.ConnectorState `json:"state"`
	SourceOrigin connectors.SourceOrigin   `json:"sourceOrigin"`
	AuthType     connectors.AuthType       `json:"authType"`
	LastError    string                    `json:"lastError,omitempty"`
	ServerNames  []string                  `json:"serverNames,omitempty"`
	ShadowedBy   string                    `json:"shadowedBy,omitempty"`
}

// connectRequest is the wire payload for POST /v1/connectors/{id}/connect.
type connectRequest struct {
	RedirectURL  string   `json:"redirectUrl,omitempty"`
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

// connectResponse is the wire payload returned by POST /v1/connectors/{id}/connect.
type connectResponse struct {
	ConnectorID      string `json:"connectorId"`
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	State            string `json:"connectorState"`
	Message          string `json:"message,omitempty"`
}

// disconnectResponse is the wire payload returned by POST /v1/connectors/{id}/disconnect.
type disconnectResponse struct {
	ConnectorID string `json:"connectorId"`
	State       string `json:"connectorState"`
	Message     string `json:"message,omitempty"`
}

func (h connectorHandlers) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_UNAVAILABLE", "connector registry not configured")
		return
	}
	statuses := h.registry.List()
	out := make([]connectorInfo, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, connectorInfo{
			ID:           s.ID,
			DisplayName:  s.DisplayName,
			Category:     s.Category,
			Provider:     s.Provider,
			State:        s.State,
			SourceOrigin: s.SourceOrigin,
			AuthType:     s.AuthType,
			LastError:    s.LastError,
			ServerNames:  s.ServerNames,
			ShadowedBy:   s.ShadowedBy,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h connectorHandlers) handleConnect(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_UNAVAILABLE", "connector registry not configured")
		return
	}
	connectorID := r.PathValue("id")
	if connectorID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "connector id is required")
		return
	}

	def, err := h.registry.Definition(connectorID)
	if err != nil {
		if errors.Is(err, connectors.ErrConnectorNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// For OAuth2 connectors, the desktop UI should first complete the OAuth
	// flow with the MCP manager. Here we just Connect the connector.
	// The connectRequest is parsed for forward-compatibility but the v1
	// Connect flow does not initiate OAuth itself — that is done through the
	// existing /v1/mcp/servers/{name}/auth endpoint.
	var req connectRequest
	_ = decodeJSON(r, &req) // Optional body; ignore parse errors for now.

	if err := h.registry.Connect(r.Context(), connectorID); err != nil {
		if errors.Is(err, connectors.ErrConnectorAlreadyConnected) {
			writeErrorJSON(w, http.StatusConflict, "ALREADY_CONNECTED", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusBadRequest, "CONNECT_FAILED", err.Error())
		return
	}

	status, _ := h.registry.Status(connectorID)

	resp := connectResponse{
		ConnectorID: connectorID,
		State:       string(status.State),
		Message:     "connector " + def.DisplayName + " connected successfully",
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h connectorHandlers) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_UNAVAILABLE", "connector registry not configured")
		return
	}
	connectorID := r.PathValue("id")
	if connectorID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "connector id is required")
		return
	}

	if err := h.registry.Disconnect(r.Context(), connectorID); err != nil {
		if errors.Is(err, connectors.ErrConnectorNotConnected) {
			writeErrorJSON(w, http.StatusConflict, "NOT_CONNECTED", err.Error())
			return
		}
		if errors.Is(err, connectors.ErrConnectorNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resp := disconnectResponse{
		ConnectorID: connectorID,
		State:       "disconnected",
		Message:     "connector " + connectorID + " disconnected",
	}
	writeJSON(w, http.StatusOK, resp)
}

// Ensure json import stays referenced.
var _ = json.Marshal
