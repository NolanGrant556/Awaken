package server

import (
	"net/http"

	awruntime "awaken/src/runtime"
)

// acpSessionControlHandlers handles bridge-only ACP session lifecycle routes.
// Stubs are in place for Stage 3 implementation.
type acpSessionControlHandlers struct {
	runService *awruntime.RunService
}

func (h *acpSessionControlHandlers) handleCreateACPSession(w http.ResponseWriter, r *http.Request) {
	writeErrorJSON(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ACP session creation is implemented in Stage 3")
}

func (h *acpSessionControlHandlers) handleGetACPSession(w http.ResponseWriter, r *http.Request) {
	writeErrorJSON(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ACP session query is implemented in Stage 3")
}

func (h *acpSessionControlHandlers) handleCloseACPSession(w http.ResponseWriter, r *http.Request) {
	writeErrorJSON(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "ACP session close is implemented in Stage 3")
}
