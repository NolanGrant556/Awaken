package server

import (
	"errors"
	"net/http"

	"awaken/src/computer"
	awruntime "awaken/src/runtime"
)

// computerControlHandlers serves bridge-only Computer Use control routes
// protected by the control token middleware. Phase 1 registers the surface and
// wires it to the run-bound Computer runtime; real Host-mediated sessions are
// still gated by an available driver.
type computerControlHandlers struct {
	runService *awruntime.RunService
}

type resolveComputerReviewRequest struct {
	Decision string `json:"decision"`
	Actor    string `json:"actor,omitempty"`
	// RunID is required because the current route does not include it. The
	// target contract places run_id in the path; until the route changes, the
	// body carries it.
	RunID string `json:"run_id"`
}

func (h *computerControlHandlers) handleGetComputerSession(w http.ResponseWriter, r *http.Request) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}

	runID := r.PathValue("run_id")
	cr, err := h.runService.ComputerRuntime(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusBadRequest, "COMPUTER_RUNTIME_UNAVAILABLE", err.Error())
		return
	}

	binding := cr.Binding()
	sess, active := cr.Session()
	state := string(sess.State)
	if !active {
		state = "none"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":               runID,
		"binding_id":           string(binding.BindingID),
		"environment_id":       binding.EnvironmentID,
		"resource_key":         binding.ResourceKey,
		"policy_snapshot_id":   string(binding.PolicySnapshotID),
		"allowed_scopes":       binding.AllowedScopes,
		"allowed_action_kinds": binding.AllowedActionKinds,
		"session_active":       active && !computer.IsTerminalState(sess.State),
		"session_state":        state,
		"session_id":           string(sess.SessionID),
		"control_epoch":        binding.ControlEpoch.String(),
		"fence_token":          binding.FenceToken.String(),
	})
}

func (h *computerControlHandlers) handleComputerSessionControl(w http.ResponseWriter, r *http.Request, action string) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}

	runID := r.PathValue("run_id")
	cr, err := h.runService.ComputerRuntime(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusBadRequest, "COMPUTER_RUNTIME_UNAVAILABLE", err.Error())
		return
	}

	var opErr error
	switch action {
	case "pause":
		opErr = cr.Pause(r.Context())
	case "takeover":
		opErr = cr.Takeover(r.Context())
	case "return":
		opErr = cr.Return(r.Context())
	case "terminate":
		opErr = cr.Terminate(r.Context(), "user_stopped")
	default:
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_CONTROL_ACTION", "unknown control action")
		return
	}

	if opErr != nil {
		writeErrorJSON(w, http.StatusConflict, "COMPUTER_CONTROL_FAILED", opErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"run_id": runID,
		"action": action,
		"status": "ok",
	})
}

func (h *computerControlHandlers) handleResolveComputerReview(w http.ResponseWriter, r *http.Request) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}

	var req resolveComputerReviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Decision == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "decision is required")
		return
	}

	runID := r.PathValue("run_id")
	if runID == "" {
		runID = req.RunID
	}
	if runID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "run_id is required")
		return
	}

	cr, err := h.runService.ComputerRuntime(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusBadRequest, "COMPUTER_RUNTIME_UNAVAILABLE", err.Error())
		return
	}

	reviewID := computer.ReviewID(r.PathValue("review_id"))
	decision := computer.ReviewDecision(req.Decision)
	review, err := cr.ResolveReview(r.Context(), reviewID, decision, req.Actor, r.Header.Get("X-Request-ID"))
	if err != nil {
		writeErrorJSON(w, http.StatusConflict, "COMPUTER_REVIEW_RESOLUTION_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"review_id":     string(review.ReviewID),
		"action_id":     string(review.ActionID),
		"action_digest": review.ActionDigest,
		"session_id":    string(review.SessionID),
		"run_id":        runID,
		"decision":      string(review.Decision),
		"actor":         review.Actor,
		"decided_at":    review.DecidedAt,
		"expires_at":    review.ExpiresAt,
	})
}

func (h *computerControlHandlers) handleGetComputerHostStatus(w http.ResponseWriter, r *http.Request) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}

	status, err := h.runService.ComputerHostStatus(r.Context())
	if err != nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "COMPUTER_HOST_PROBE_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}
