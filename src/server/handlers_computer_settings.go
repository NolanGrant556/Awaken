package server

import (
	"errors"
	"net/http"

	"awaken/src/computer"
	"awaken/src/filestore"
)

type computerSettingsHandlers struct {
	store     *computer.SettingsStore
	onUpdated func(computer.Settings)
}

func (h computerSettingsHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	document, err := h.store.Get()
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "COMPUTER_SETTINGS_READ_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, document)
}

type updateComputerSettingsRequest struct {
	ExpectedRevision uint64 `json:"expected_revision"`
	computer.SettingsPatch
}

func (h computerSettingsHandlers) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var request updateComputerSettingsRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPUTER_SETTINGS_INVALID", err.Error())
		return
	}

	document, err := h.store.Update(request.ExpectedRevision, request.SettingsPatch)
	if err != nil {
		if errors.Is(err, filestore.ErrRevisionConflict) {
			writeErrorJSON(w, http.StatusConflict, "COMPUTER_SETTINGS_REVISION_CONFLICT", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "COMPUTER_SETTINGS_UPDATE_FAILED", err.Error())
		return
	}
	if h.onUpdated != nil {
		h.onUpdated(document.Value)
	}

	writeJSON(w, http.StatusOK, document)
}
