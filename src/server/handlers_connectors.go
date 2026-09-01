package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"awaken/src/connectorauth"
	"awaken/src/connectors"
)

// connectorHandlers wires the /v1/connectors/* HTTP surface to a
// connectors.Registry.
type connectorHandlers struct {
	registry *connectors.Registry
	auth     connectorAuthService
}

type connectorAuthService interface {
	Start(context.Context, string) (connectors.ConnectorAuthStartResult, error)
	Session(string) (connectors.ConnectorAuthSession, error)
	Cancel(string) (connectors.ConnectorAuthSession, error)
	MarkAdminApprovalPending(string) (connectors.ConnectorAuthSession, error)
	Disconnect(context.Context, string) error
}

// connectorInfo is the wire shape for entries returned by GET /v1/connectors.
type connectorInfo struct {
	ID                      string                              `json:"id"`
	Kind                    connectors.ConnectorKind            `json:"kind"`
	DisplayName             string                              `json:"displayName"`
	Description             string                              `json:"description,omitempty"`
	Category                connectors.Category                 `json:"category"`
	Provider                string                              `json:"provider,omitempty"`
	State                   connectors.ConnectorState           `json:"state"`
	SourceOrigin            connectors.SourceOrigin             `json:"sourceOrigin"`
	CatalogPlacement        connectors.CatalogPlacement         `json:"catalogPlacement"`
	AuthType                connectors.AuthType                 `json:"authType"`
	LastError               string                              `json:"lastError,omitempty"`
	ServerNames             []string                            `json:"serverNames,omitempty"`
	ShadowedBy              string                              `json:"shadowedBy,omitempty"`
	CLIReadiness            *connectors.CLIConnectorReadiness   `json:"cliReadiness,omitempty"`
	LegacyMCPReviewRequired bool                                `json:"legacyMcpReviewRequired,omitempty"`
	Account                 *connectors.ConnectorAccountProfile `json:"account,omitempty"`
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
	ConnectorID      string     `json:"connectorId"`
	AuthorizationURL string     `json:"authorizationUrl,omitempty"`
	State            string     `json:"connectorState"`
	SessionID        string     `json:"sessionId,omitempty"`
	AuthStatus       string     `json:"authStatus,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	Message          string     `json:"message,omitempty"`
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
		if s.CatalogPlacement == connectors.CatalogPluginOnly {
			continue
		}
		out = append(out, connectorInfo{
			ID:                      s.ID,
			Kind:                    s.Kind,
			DisplayName:             s.DisplayName,
			Description:             s.Description,
			Category:                s.Category,
			Provider:                s.Provider,
			State:                   s.State,
			SourceOrigin:            s.SourceOrigin,
			CatalogPlacement:        s.CatalogPlacement,
			AuthType:                s.AuthType,
			LastError:               s.LastError,
			ServerNames:             s.ServerNames,
			ShadowedBy:              s.ShadowedBy,
			CLIReadiness:            s.CLIReadiness,
			LegacyMCPReviewRequired: s.LegacyMCPReviewRequired,
			Account:                 s.Account,
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

	var req connectRequest
	_ = decodeJSON(r, &req)
	if def.OAuth != nil {
		if h.auth == nil {
			writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_AUTH_UNAVAILABLE", "connector account service is unavailable")
			return
		}
		if req.RedirectURL != "" || req.ClientID != "" || req.ClientSecret != "" || len(req.Scopes) > 0 {
			writeErrorJSON(w, http.StatusUnprocessableEntity, "CONNECTOR_AUTH_OVERRIDE_REJECTED", "host connector authorization cannot be overridden by the client")
			return
		}
		result, startErr := h.auth.Start(r.Context(), connectorID)
		if startErr != nil {
			switch {
			case errors.Is(startErr, connectorauth.ErrClientIDRequired):
				writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_OAUTH_CLIENT_ID_REQUIRED", "Awaken 的 Microsoft 登录配置尚未发布，当前不能发起账号授权。")
			case errors.Is(startErr, connectorauth.ErrUnsupported):
				writeErrorJSON(w, http.StatusUnprocessableEntity, "CONNECTOR_AUTH_UNSUPPORTED", startErr.Error())
			default:
				writeErrorJSON(w, http.StatusBadRequest, "CONNECTOR_AUTH_FAILED", "connector authorization could not start")
			}
			return
		}
		writeJSON(w, http.StatusOK, connectResponse{
			ConnectorID: result.ConnectorID, AuthorizationURL: result.AuthorizationURL,
			State: result.Status, SessionID: result.SessionID, AuthStatus: result.Status,
			ExpiresAt: result.ExpiresAt, Message: "请在已打开的 Microsoft 页面完成 Outlook 授权",
		})
		return
	}

	if err := h.registry.Connect(r.Context(), connectorID); err != nil {
		if errors.Is(err, connectors.ErrConnectorActionUnsupported) {
			writeErrorJSON(w, http.StatusUnprocessableEntity, "CONNECTOR_ACTION_UNSUPPORTED", err.Error())
			return
		}
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

	def, defErr := h.registry.Definition(connectorID)
	if defErr != nil {
		writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_NOT_FOUND", defErr.Error())
		return
	}
	var disconnectErr error
	if def.OAuth != nil {
		if h.auth == nil {
			writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_AUTH_UNAVAILABLE", "connector account service is unavailable")
			return
		}
		disconnectErr = h.auth.Disconnect(r.Context(), connectorID)
	} else {
		disconnectErr = h.registry.Disconnect(r.Context(), connectorID)
	}
	if disconnectErr != nil {
		if errors.Is(disconnectErr, connectors.ErrConnectorActionUnsupported) {
			writeErrorJSON(w, http.StatusUnprocessableEntity, "CONNECTOR_ACTION_UNSUPPORTED", disconnectErr.Error())
			return
		}
		if errors.Is(disconnectErr, connectors.ErrConnectorNotConnected) {
			writeErrorJSON(w, http.StatusConflict, "NOT_CONNECTED", disconnectErr.Error())
			return
		}
		if errors.Is(disconnectErr, connectors.ErrConnectorNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_NOT_FOUND", disconnectErr.Error())
			return
		}
		if def.OAuth != nil {
			writeErrorJSON(w, http.StatusInternalServerError, "CONNECTOR_DISCONNECT_FAILED", "无法清除本机连接器账号，请重试。")
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", disconnectErr.Error())
		return
	}

	resp := disconnectResponse{
		ConnectorID: connectorID,
		State:       "disconnected",
		Message:     "connector " + connectorID + " disconnected",
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h connectorHandlers) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_AUTH_UNAVAILABLE", "connector account service is unavailable")
		return
	}
	view, err := h.auth.Session(r.PathValue("sessionId"))
	if err != nil {
		if errors.Is(err, connectorauth.ErrSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_AUTH_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "connector authorization session could not be read")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h connectorHandlers) handleCancelAuthSession(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_AUTH_UNAVAILABLE", "connector account service is unavailable")
		return
	}
	view, err := h.auth.Cancel(r.PathValue("sessionId"))
	if err != nil {
		if errors.Is(err, connectorauth.ErrSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_AUTH_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "connector authorization session could not be cancelled")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h connectorHandlers) handleAdminApprovalPending(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_AUTH_UNAVAILABLE", "connector account service is unavailable")
		return
	}
	view, err := h.auth.MarkAdminApprovalPending(r.PathValue("sessionId"))
	if err != nil {
		if errors.Is(err, connectorauth.ErrSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_AUTH_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "CONNECTOR_APPROVAL_STATE_FAILED", "administrator approval state could not be saved")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h connectorHandlers) handleCLIReadiness(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "CONNECTOR_UNAVAILABLE", "connector registry not configured")
		return
	}
	connectorID := r.PathValue("id")
	var snapshot connectors.CLIConnectorReadiness
	if err := decodeJSON(r, &snapshot); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid CLI readiness payload")
		return
	}
	if err := h.registry.UpdateCLIReadiness(r.Header.Get("X-Awaken-Control-Token"), connectorID, snapshot); err != nil {
		switch {
		case errors.Is(err, connectors.ErrCLIReadinessUnauthorized):
			writeErrorJSON(w, http.StatusUnauthorized, "CLI_READINESS_UNAUTHORIZED", "CLI readiness update is unauthorized")
		case errors.Is(err, connectors.ErrConnectorNotFound):
			writeErrorJSON(w, http.StatusNotFound, "CONNECTOR_NOT_FOUND", err.Error())
		default:
			writeErrorJSON(w, http.StatusUnprocessableEntity, "INVALID_CLI_READINESS", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// Ensure json import stays referenced.
var _ = json.Marshal
