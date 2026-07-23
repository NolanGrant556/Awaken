package server

import (
	"net/http"
)

type acpModelProfileControlHandlers struct {
	store ACPModelProfileStore
}

func (h *acpModelProfileControlHandlers) handleList(w http.ResponseWriter, _ *http.Request) {
	if h.store == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_MODEL_PROFILE_UNAVAILABLE", "model profiles are unavailable")
		return
	}
	catalog, err := h.store.ListReady()
	if err != nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_MODEL_PROFILE_UNAVAILABLE", "no usable Awaken model profile is configured")
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}
