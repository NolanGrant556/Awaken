package server

import (
	"errors"
	"net/http"
	"strconv"

	awruntime "awaken/src/runtime"
)

func (h runHandlers) handleThreadActivity(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseUint(r.URL.Query().Get("after_cursor"), 10, 64)
	coarse := after / frameSeqStride
	if coarse > 0 {
		coarse--
	}
	sub, last, err := h.runService.SubscribeThreadActivity(r.PathValue("thread_id"), coarse)
	if errors.Is(err, awruntime.ErrActivityCursorExpired) {
		writeErrorJSON(w, http.StatusConflict, "ACTIVITY_CURSOR_EXPIRED", err.Error())
		return
	}
	if err != nil {
		writeErrorJSON(w, http.StatusNotFound, "THREAD_NOT_FOUND", "task activity was not found")
		return
	}
	if sub.Close != nil {
		defer sub.Close()
	}
	if after > last {
		writeErrorJSON(w, http.StatusConflict, "ACTIVITY_CURSOR_EXPIRED", "task activity restarted; reload the canonical transcript")
		return
	}
	stream, ok := NewSSEStream(w)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	stream.Flush()
	send := func(event awruntime.Event) bool {
		// Root streams retain their existing cursor domain. This stream carries
		// all child activity, including executions from an earlier root run.
		if event.Kind == awruntime.EventKindAgent {
			if event.AgentEvent.AgentID == "" || event.AgentEvent.AgentID == "root" {
				return true
			}
		} else if event.Kind == awruntime.EventKindSemantic {
			id, _ := event.SemanticRecord.Payload["agent_id"].(string)
			if id == "" || id == "root" {
				return true
			}
		} else {
			return true
		}
		for _, frame := range mapRuntimeEvent(event) {
			if frame.StreamCursor <= after {
				continue
			}
			if err := stream.WriteEvent(frame); err != nil {
				return false
			}
		}
		return true
	}
	for _, event := range sub.History {
		if !send(event) {
			return
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-sub.Events:
			if !ok || !send(event) {
				return
			}
		}
	}
}

func (h runHandlers) handleStopThreadAgents(w http.ResponseWriter, r *http.Request) {
	if err := h.runService.StopThreadAgents(r.Context(), r.PathValue("thread_id")); err != nil {
		writeErrorJSON(w, http.StatusConflict, "STOP_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopping": true})
}
