package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
)

type runHandlers struct {
	runService *awruntime.RunService
}

type createRunRequest struct {
	Input string `json:"input"`
}

func (h runHandlers) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	run, err := h.runService.StartRun(req.Input)
	if err != nil {
		if errors.Is(err, awruntime.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to start run", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

func (h runHandlers) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	sub, err := h.runService.Subscribe(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to subscribe run events", http.StatusInternalServerError)
		return
	}
	if sub.Close != nil {
		defer sub.Close()
	}

	stream, ok := NewSSEStream(w)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	for _, event := range sub.History {
		if err := writeRuntimeEvent(stream, event); err != nil {
			return
		}
	}

	if sub.Events == nil {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-sub.Events:
			if !ok {
				return
			}
			if err := writeRuntimeEvent(stream, event); err != nil {
				return
			}
		}
	}
}

func (h runHandlers) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	run, err := h.runService.CancelRun(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to cancel run", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeRuntimeEvent(stream *SSEStream, event awruntime.Event) error {
	for _, mapped := range mapRuntimeEvent(event) {
		if err := stream.WriteEvent(mapped); err != nil {
			return err
		}
	}
	return nil
}

func mapRuntimeEvent(event awruntime.Event) []agui.Event {
	switch event.Kind {
	case awruntime.EventKindAgent:
		return agui.MapEvent(event.AgentEvent)
	case awruntime.EventKindRunCancelled:
		return []agui.Event{{
			Type:      agui.EventRunError,
			Timestamp: event.Timestamp.UnixMilli(),
			ThreadID:  event.ThreadID,
			RunID:     event.RunID,
			Message:   event.Message,
			Code:      "RUN_CANCELLED",
		}}
	case awruntime.EventKindRunError:
		return []agui.Event{{
			Type:      agui.EventRunError,
			Timestamp: event.Timestamp.UnixMilli(),
			ThreadID:  event.ThreadID,
			RunID:     event.RunID,
			Message:   event.Message,
			Code:      event.Code,
		}}
	default:
		return nil
	}
}
