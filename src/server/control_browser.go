package server

import (
	"net/http"

	"awaken/src/browserhost"
)

type browserHostRegistrationRequest struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

func handleRegisterBrowserHost(w http.ResponseWriter, r *http.Request) {
	var req browserHostRegistrationRequest
	if err := decodeJSON(r, &req); err != nil && r.ContentLength != 0 {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	var registration *browserhost.Registration
	if req.BaseURL != "" || req.Token != "" {
		registration = &browserhost.Registration{BaseURL: req.BaseURL, Token: req.Token}
	}
	if err := browserhost.Set(registration); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_BROWSER_HOST", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ready": browserhost.Ready()})
}
