package server

import (
	"encoding/json"
	"errors"
	"net/http"

	awruntime "awaken/src/runtime"
)

// acpSessionControlHandlers handles bridge-only ACP session lifecycle routes.
type acpSessionControlHandlers struct {
	runService        *awruntime.RunService
	acpSessionManager *awruntime.ACPSessionManager
}

type acpSessionCreateRequest struct {
	ACPSessionID  string `json:"acp_session_id"`
	CWD           string `json:"cwd"`
	Visibility    string `json:"visibility"`
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

func (h *acpSessionControlHandlers) handleCreateACPSession(w http.ResponseWriter, r *http.Request) {
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}
	var req acpSessionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.ACPSessionID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "acp_session_id is required")
		return
	}
	if req.CWD == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "cwd is required")
		return
	}
	if req.Visibility == "" {
		req.Visibility = "visible"
	}
	if req.Visibility != "visible" && req.Visibility != "ephemeral" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "visibility must be visible or ephemeral")
		return
	}

	mapping, err := h.acpSessionManager.CreateSession(r.Context(), awruntime.ACPSessionCreateRequest{
		ACPSessionID:  req.ACPSessionID,
		CWD:           req.CWD,
		Visibility:    req.Visibility,
		ClientName:    req.ClientName,
		ClientVersion: req.ClientVersion,
	})
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionExists) {
			writeErrorJSON(w, http.StatusConflict, "ACP_SESSION_EXISTS", err.Error())
			return
		}
		if errors.Is(err, awruntime.ErrInvalidCWD) {
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_CWD", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_CREATE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"acp_session_id": mapping.ACPSessionID,
		"project_id":     mapping.ProjectID,
		"thread_id":      mapping.ThreadID,
		"canonical_cwd":  mapping.CanonicalCWD,
		"visibility":     mapping.Visibility,
	})
}

func (h *acpSessionControlHandlers) handleGetACPSession(w http.ResponseWriter, r *http.Request) {
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}
	sessionID := r.PathValue("session_id")
	mapping, err := h.acpSessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_GET_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"acp_session_id": mapping.ACPSessionID,
		"project_id":     mapping.ProjectID,
		"thread_id":      mapping.ThreadID,
		"current_run_id": mapping.CurrentRunID,
		"canonical_cwd":  mapping.CanonicalCWD,
		"visibility":     mapping.Visibility,
	})
}

func (h *acpSessionControlHandlers) handleCloseACPSession(w http.ResponseWriter, r *http.Request) {
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}
	sessionID := r.PathValue("session_id")
	if err := h.acpSessionManager.CloseSession(r.Context(), sessionID); err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_CLOSE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}
