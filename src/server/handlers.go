package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"awaken/src/agentcore"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
)

type runHandlers struct {
	runService *awruntime.RunService
}

type resolveApprovalRequest struct {
	Decision string `json:"decision"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeErrorJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Code: code, Message: message})
}

func (h runHandlers) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req awruntime.StartRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	run, err := h.runService.StartRun(req)
	if err != nil {
		if errors.Is(err, awruntime.ErrInvalidInput) {
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to start run")
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

func (h runHandlers) handleResolveApproval(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	approvalID := r.PathValue("approval_id")

	var req resolveApprovalRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var decision agentcore.ApprovalDecision
	switch req.Decision {
	case string(agentcore.ApprovalDecisionApproved):
		decision = agentcore.ApprovalDecisionApproved
	case string(agentcore.ApprovalDecisionRejected):
		decision = agentcore.ApprovalDecisionRejected
	default:
		http.Error(w, "decision must be approved or rejected", http.StatusBadRequest)
		return
	}

	if err := h.runService.ResolveApproval(runID, approvalID, decision); err != nil {
		switch {
		case errors.Is(err, awruntime.ErrRunNotFound), errors.Is(err, awruntime.ErrApprovalNotFound):
			http.NotFound(w, r)
		default:
			http.Error(w, "failed to resolve approval", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

func handleRuntimeInfo(w http.ResponseWriter, r *http.Request, provider RuntimeInfoProvider) {
	if provider == nil {
		http.Error(w, "runtime info unavailable", http.StatusServiceUnavailable)
		return
	}

	data, err := json.Marshal(provider.GetRuntimeInfo())
	if err != nil {
		http.Error(w, "failed to serialize runtime info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
