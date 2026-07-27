package server

import (
	"errors"
	"net/http"
	"strconv"

	"awaken/src/acp"
	"awaken/src/modelprofile"
	awruntime "awaken/src/runtime"
)

// acpRunControlHandlers handles bridge-only ACP run lifecycle routes protected
// by the control token.
type acpRunControlHandlers struct {
	runService        *awruntime.RunService
	acpSessionManager *awruntime.ACPSessionManager
	acpManager        *acp.Manager
	identity          *awruntime.RuntimeIdentity
	modelProfileStore ACPModelProfileStore
}

type acpRunCreateRequest struct {
	ThreadID       string `json:"thread_id"`
	Input          string `json:"input"`
	WorkspaceRoot  string `json:"workspace_root"`
	ModelProfileID string `json:"model_profile_id"`
}

type resolveACPPermissionRequest struct {
	OptionID  string `json:"option_id"`
	Cancelled bool   `json:"cancelled"`
}

func (h *acpRunControlHandlers) handleCreateACPRun(w http.ResponseWriter, r *http.Request) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}

	sessionID := r.PathValue("session_id")
	mapping, err := h.acpSessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_GET_FAILED", err.Error())
		return
	}

	var req acpRunCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.ThreadID != "" && req.ThreadID != mapping.ThreadID {
		writeErrorJSON(w, http.StatusBadRequest, "ACP_SESSION_SCOPE_MISMATCH", "thread_id does not belong to this ACP session")
		return
	}
	if req.WorkspaceRoot != "" {
		canonicalRoot, canonicalErr := awruntime.CanonicalizeCWD(req.WorkspaceRoot)
		if canonicalErr != nil {
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_WORKSPACE_ROOT", canonicalErr.Error())
			return
		}
		if canonicalRoot != mapping.CanonicalCWD {
			writeErrorJSON(w, http.StatusBadRequest, "ACP_SESSION_SCOPE_MISMATCH", "workspace_root does not belong to this ACP session")
			return
		}
	}
	if h.modelProfileStore == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_MODEL_PROFILE_UNAVAILABLE", "model profiles are unavailable")
		return
	}
	profile, err := h.modelProfileStore.Resolve(req.ModelProfileID)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "ACP_MODEL_PROFILE_UNAVAILABLE", "the selected Awaken model profile is unavailable")
		return
	}
	if err := h.acpSessionManager.BeginRun(sessionID); err != nil {
		if errors.Is(err, awruntime.ErrACPSessionBusy) {
			writeErrorJSON(w, http.StatusConflict, "ACP_SESSION_BUSY", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_RESERVE_FAILED", err.Error())
		return
	}
	committed := false
	defer func() {
		if !committed {
			h.acpSessionManager.AbortRun(sessionID)
		}
	}()

	threadID := req.ThreadID
	if threadID == "" {
		threadID = mapping.ThreadID
	}
	workspaceRoot := req.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = mapping.CanonicalCWD
	}

	startReq := awruntime.StartRunRequest{
		Input: req.Input,
		Workspace: awruntime.WorkspaceConfig{
			WorkspaceRoot: workspaceRoot,
		},
		Client: awruntime.ClientInfo{
			EntryPoint:    "awaken-acp",
			Source:        "acp_bridge",
			SessionID:     sessionID,
			ClientName:    mapping.ClientName,
			ClientVersion: mapping.ClientVersion,
		},
		ModelProfile: runtimeModelProfile(profile),
	}
	if threadID != "" {
		startReq.ThreadID = threadID
	}

	run, err := h.runService.StartRun(startReq)
	if errors.Is(err, awruntime.ErrThreadNotFound) && threadID == mapping.ThreadID {
		// The thread was only reserved during session creation. Start a fresh
		// thread and update the session mapping.
		startReq.ThreadID = ""
		run, err = h.runService.StartRun(startReq)
		if err == nil {
			if updateErr := h.acpSessionManager.UpdateThreadID(sessionID, run.ThreadID); updateErr != nil {
				_, _ = h.runService.CancelRun(run.RunID)
				writeErrorJSON(w, http.StatusInternalServerError, "THREAD_UPDATE_FAILED", updateErr.Error())
				return
			}
		}
	}
	if err != nil {
		switch {
		case errors.Is(err, awruntime.ErrThreadNotFound):
			writeErrorJSON(w, http.StatusNotFound, "THREAD_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrThreadWorkspaceMismatch):
			writeErrorJSON(w, http.StatusBadRequest, "THREAD_WORKSPACE_MISMATCH", err.Error())
		case errors.Is(err, awruntime.ErrThreadBusy):
			writeErrorJSON(w, http.StatusConflict, "THREAD_BUSY", err.Error())
		case errors.Is(err, awruntime.ErrProjectOperationConflict):
			writeErrorJSON(w, http.StatusConflict, "PROJECT_OPERATION_CONFLICT", err.Error())
		case errors.Is(err, awruntime.ErrInvalidInput):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		case errors.Is(err, awruntime.ErrUnsupportedInputAttachment):
			writeErrorJSON(w, http.StatusBadRequest, "UNSUPPORTED_INPUT_ATTACHMENT", err.Error())
		case errors.Is(err, awruntime.ErrMissingModelProfile):
			writeErrorJSON(w, http.StatusBadRequest, "MISSING_MODEL_PROFILE", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "RUN_START_FAILED", "failed to start run")
		}
		return
	}

	if err := h.acpSessionManager.SetCurrentRun(sessionID, run.RunID); err != nil {
		_, _ = h.runService.CancelRun(run.RunID)
		writeErrorJSON(w, http.StatusInternalServerError, "SET_CURRENT_RUN_FAILED", err.Error())
		return
	}
	committed = true

	writeJSON(w, http.StatusCreated, run)
}

func runtimeModelProfile(profile modelprofile.ResolvedProfile) *awruntime.ModelProfileConfig {
	return awruntime.ModelProfileConfigFromResolved(profile)
}

func (h *acpRunControlHandlers) handleClearCurrentRun(w http.ResponseWriter, r *http.Request) {
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}

	sessionID := r.PathValue("session_id")
	runID := r.PathValue("run_id")
	_, err := h.acpSessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_GET_FAILED", err.Error())
		return
	}

	h.acpSessionManager.ClearCurrentRun(sessionID, runID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *acpRunControlHandlers) handleACPRunEvents(w http.ResponseWriter, r *http.Request) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}

	sessionID := r.PathValue("session_id")
	runID := r.PathValue("run_id")
	mapping, err := h.acpSessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_GET_FAILED", err.Error())
		return
	}
	if mapping.CurrentRunID != runID {
		writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", "run is not the current run for this session")
		return
	}

	afterCursorStr := r.URL.Query().Get("after_cursor")
	if afterCursorStr == "" {
		afterCursorStr = r.URL.Query().Get("after_seq")
	}
	afterCursor, err := strconv.ParseUint(afterCursorStr, 10, 64)
	if err != nil {
		afterCursor = 0
	}
	coarseAfter := afterCursor / frameSeqStride
	var eventBusAfter uint64
	if coarseAfter > 0 {
		eventBusAfter = coarseAfter - 1
	}

	sub, err := h.runService.Subscribe(runID, eventBusAfter)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "SUBSCRIBE_FAILED", "failed to subscribe run events")
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
	stream.Flush()

	for _, event := range sub.History {
		for _, frame := range mapRuntimeEvent(event) {
			cursor := frame.StreamCursor
			if cursor == 0 {
				cursor = frame.Seq
			}
			if cursor <= afterCursor {
				continue
			}
			if err := stream.WriteEvent(frame); err != nil {
				return
			}
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

func (h *acpRunControlHandlers) handleCancelACPRun(w http.ResponseWriter, r *http.Request) {
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUN_SERVICE_UNAVAILABLE", "run service not initialized")
		return
	}
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}

	sessionID := r.PathValue("session_id")
	runID := r.PathValue("run_id")
	mapping, err := h.acpSessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_GET_FAILED", err.Error())
		return
	}
	if mapping.CurrentRunID != runID {
		writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", "run is not the current run for this session")
		return
	}

	run, err := h.runService.CancelRun(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel run")
		return
	}
	if h.acpManager != nil && run.ThreadID != "" {
		h.acpManager.CancelAllForThread(r.Context(), run.ThreadID)
	}

	writeJSON(w, http.StatusOK, run)
}

func (h *acpRunControlHandlers) handleResolveACPPermission(w http.ResponseWriter, r *http.Request) {
	if h.acpManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_MANAGER_UNAVAILABLE", "acp manager not initialized")
		return
	}
	if h.acpSessionManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "ACP_SESSION_UNAVAILABLE", "acp session manager not initialized")
		return
	}

	sessionID := r.PathValue("session_id")
	runID := r.PathValue("run_id")
	interactionID := r.PathValue("interaction_id")
	mapping, err := h.acpSessionManager.GetSession(sessionID)
	if err != nil {
		if errors.Is(err, awruntime.ErrACPSessionNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "ACP_SESSION_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "ACP_SESSION_GET_FAILED", err.Error())
		return
	}
	if mapping.CurrentRunID != runID {
		writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", "run is not the current run for this session")
		return
	}

	var req resolveACPPermissionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := h.acpManager.ResolvePermission(runID, interactionID, req.OptionID, req.Cancelled); err != nil {
		writeErrorJSON(w, http.StatusNotFound, "PERMISSION_NOT_FOUND", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"resolved": true})
}
