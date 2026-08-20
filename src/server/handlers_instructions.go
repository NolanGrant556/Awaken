package server

import (
	"errors"
	"net/http"

	"awaken/src/instructions"
	awruntime "awaken/src/runtime"
)

type instructionHandlers struct {
	service *awruntime.RunService
}

type putGlobalInstructionRequest struct {
	ExpectedRevision string `json:"expected_revision"`
	Content          string `json:"content"`
}

type deleteGlobalInstructionRequest struct {
	ExpectedRevision string `json:"expected_revision"`
}

func (h instructionHandlers) handleGetGlobal(w http.ResponseWriter, _ *http.Request) {
	document, err := h.service.GetGlobalInstructions()
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "INSTRUCTIONS_UNAVAILABLE", "global instructions are unavailable")
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (h instructionHandlers) handlePutGlobal(w http.ResponseWriter, r *http.Request) {
	var request putGlobalInstructionRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	document, err := h.service.PutGlobalInstructions(request.ExpectedRevision, request.Content)
	if errors.Is(err, instructions.ErrRevisionConflict) {
		writeErrorJSON(w, http.StatusConflict, "INSTRUCTION_REVISION_CONFLICT", "global AGENTS.md changed; reload before saving")
		return
	}
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "INSTRUCTIONS_WRITE_FAILED", "failed to save global AGENTS.md")
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (h instructionHandlers) handleDeleteGlobal(w http.ResponseWriter, r *http.Request) {
	var request deleteGlobalInstructionRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	document, err := h.service.DeleteGlobalInstructions(request.ExpectedRevision)
	if errors.Is(err, instructions.ErrRevisionConflict) {
		writeErrorJSON(w, http.StatusConflict, "INSTRUCTION_REVISION_CONFLICT", "global AGENTS.md changed; reload before deleting")
		return
	}
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "INSTRUCTIONS_DELETE_FAILED", "failed to delete global AGENTS.md")
		return
	}
	writeJSON(w, http.StatusOK, document)
}
