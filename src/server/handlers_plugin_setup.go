package server

import (
	"errors"
	"net/http"
	"strings"

	"awaken/src/pluginauth"
)

func (h pluginHandlers) ensurePluginAuth(w http.ResponseWriter) bool {
	if h.auth == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "PLUGIN_AUTH_UNAVAILABLE", "plugin auth broker not configured")
		return false
	}
	return true
}

func (h pluginHandlers) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	items, summary, err := h.store.SetupItems(strings.TrimSpace(r.PathValue("name")))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "summary": summary})
}

func (h pluginHandlers) handleStartSetupSession(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) || !h.ensurePluginAuth(w) {
		return
	}
	var request pluginauth.StartRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	result, err := h.auth.Start(r.Context(), strings.TrimSpace(r.PathValue("name")), strings.TrimSpace(r.PathValue("setupId")), request)
	if err != nil {
		h.writePluginAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h pluginHandlers) handleSetupSession(w http.ResponseWriter, r *http.Request) {
	if !h.ensurePluginAuth(w) {
		return
	}
	view, err := h.auth.Session(strings.TrimSpace(r.PathValue("sessionId")))
	if err != nil || view.PluginName != strings.TrimSpace(r.PathValue("name")) {
		writeErrorJSON(w, http.StatusNotFound, "PLUGIN_AUTH_SESSION_NOT_FOUND", "plugin auth session not found")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h pluginHandlers) handleCancelSetupSession(w http.ResponseWriter, r *http.Request) {
	if !h.ensurePluginAuth(w) {
		return
	}
	view, err := h.auth.Session(strings.TrimSpace(r.PathValue("sessionId")))
	if err != nil || view.PluginName != strings.TrimSpace(r.PathValue("name")) {
		writeErrorJSON(w, http.StatusNotFound, "PLUGIN_AUTH_SESSION_NOT_FOUND", "plugin auth session not found")
		return
	}
	view, err = h.auth.Cancel(view.SessionID)
	if err != nil {
		h.writePluginAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h pluginHandlers) handleDisconnectSetup(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) || !h.ensurePluginAuth(w) {
		return
	}
	var request pluginauth.StartRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if err := h.auth.Disconnect(strings.TrimSpace(r.PathValue("name")), strings.TrimSpace(r.PathValue("setupId")), request.PluginDigest, request.RequirementFingerprint); err != nil {
		h.writePluginAuthError(w, err)
		return
	}
	h.refreshRuntime()
	h.writeSetupItems(w, strings.TrimSpace(r.PathValue("name")))
}

func (h pluginHandlers) handleProbeSetup(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	var request pluginauth.StartRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if _, _, err := h.store.ResolveSetupRequirement(name, strings.TrimSpace(r.PathValue("setupId")), request.PluginDigest, request.RequirementFingerprint); err != nil {
		writeErrorJSON(w, http.StatusConflict, "PLUGIN_SETUP_STALE", err.Error())
		return
	}
	h.refreshRuntime()
	h.writeSetupItems(w, name)
}

func (h pluginHandlers) writeSetupItems(w http.ResponseWriter, name string) {
	items, summary, err := h.store.SetupItems(name)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "summary": summary})
}

func (h pluginHandlers) writePluginAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pluginauth.ErrStale):
		writeErrorJSON(w, http.StatusConflict, "PLUGIN_SETUP_STALE", err.Error())
	case errors.Is(err, pluginauth.ErrUnsupported):
		writeErrorJSON(w, http.StatusConflict, "PLUGIN_SETUP_UNSUPPORTED", err.Error())
	default:
		writeErrorJSON(w, http.StatusBadRequest, "PLUGIN_AUTH_FAILED", err.Error())
	}
}
