package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"awaken/src/agentcore"
	"awaken/src/agentdefs"
	"awaken/src/protocol/agui"
	"awaken/src/redaction"
	awruntime "awaken/src/runtime"
	"awaken/src/slashcommands"
	"awaken/src/workspaceguard"
)

type runHandlers struct {
	runService *awruntime.RunService
}

// frameSeqStride reserves seq slots per runtime event so that a single runtime
// event mapping to multiple AG-UI frames still produces unique, monotonic frame
// seqs. Must be >= max frames produced by mapRuntimeEvent (currently 2).
const frameSeqStride uint64 = 16

// frameSeq derives a unique AG-UI frame seq from a runtime event seq and the
// frame index within that event. Because runtime seq is strictly increasing,
// runtimeSeq*stride is strictly increasing, and the sub-index fills the gap,
// guaranteeing monotonicity in stream order.
func frameSeq(runtimeSeq uint64, frameIndex int) uint64 {
	return runtimeSeq*frameSeqStride + uint64(frameIndex)
}

type resolveApprovalRequest struct {
	Decision string `json:"decision"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type workspaceGuardErrorResponse struct {
	Error    string                  `json:"error"`
	Message  string                  `json:"message"`
	Decision workspaceguard.Decision `json:"decision"`
}

type threadTranscriptRunResponse struct {
	Run    awruntime.Run `json:"run"`
	Events []agui.Event  `json:"events"`
}

type threadTranscriptResponse struct {
	Thread awruntime.ThreadSnapshot      `json:"thread"`
	Runs   []threadTranscriptRunResponse `json:"runs"`
}

type agentDefinitionsLoader func(workspaceRoot string) (agentdefs.Result, error)

type auditEventsResponse struct {
	Events []agentcore.SecurityAuditEvent `json:"events"`
}

func writeErrorJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: code, Message: message})
}

func writeWorkspaceGuardErrorJSON(w http.ResponseWriter, status int, policyErr *workspaceguard.PolicyError) {
	code := policyErr.Decision.ErrorCode
	if code == "" {
		code = "WORKSPACE_GUARD_DENIED"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(workspaceGuardErrorResponse{
		Error:    code,
		Message:  policyErr.Error(),
		Decision: policyErr.Decision,
	})
}

func (h runHandlers) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req awruntime.StartRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	run, err := h.runService.StartRun(req)
	if err != nil {
		var policyErr *workspaceguard.PolicyError
		switch {
		case errors.As(err, &policyErr):
			writeWorkspaceGuardErrorJSON(w, http.StatusConflict, policyErr)
		case errors.Is(err, awruntime.ErrThreadNotFound):
			writeErrorJSON(w, http.StatusNotFound, "THREAD_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrThreadWorkspaceMismatch):
			writeErrorJSON(w, http.StatusBadRequest, "THREAD_WORKSPACE_MISMATCH", err.Error())
		case errors.Is(err, awruntime.ErrThreadBusy):
			writeErrorJSON(w, http.StatusConflict, "THREAD_BUSY", err.Error())
		case errors.Is(err, awruntime.ErrParentRunNotFound):
			writeErrorJSON(w, http.StatusNotFound, "PARENT_RUN_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrParentThreadNotFound):
			writeErrorJSON(w, http.StatusNotFound, "PARENT_THREAD_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrDelegationParentMismatch):
			writeErrorJSON(w, http.StatusBadRequest, "DELEGATION_PARENT_MISMATCH", err.Error())
		case errors.Is(err, awruntime.ErrDelegationParentRequired):
			writeErrorJSON(w, http.StatusBadRequest, "DELEGATION_PARENT_REQUIRED", err.Error())
		case errors.Is(err, awruntime.ErrInvalidSlashCommand):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_SLASH_COMMAND", err.Error())
		case errors.Is(err, awruntime.ErrInvalidInput):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to start run")
		}
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

func (h runHandlers) handleListThreads(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.runService.ListThreads())
}

func (h runHandlers) handleCreateScheduledRun(w http.ResponseWriter, r *http.Request) {
	var req awruntime.CreateScheduledRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	run, err := h.runService.CreateScheduledRun(req)
	if err != nil {
		switch {
		case errors.Is(err, awruntime.ErrInvalidRunAt):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_RUN_AT", err.Error())
		case errors.Is(err, awruntime.ErrInvalidInput):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create scheduled run")
		}
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

func (h runHandlers) handleListScheduledRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.runService.ListScheduledRuns())
}

func (h runHandlers) handleCancelScheduledRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := h.runService.CancelScheduledRun(id)
	if err != nil {
		switch {
		case errors.Is(err, awruntime.ErrScheduledRunNotFound):
			writeErrorJSON(w, http.StatusNotFound, "SCHEDULED_RUN_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrScheduledRunNotCancelable):
			writeErrorJSON(w, http.StatusConflict, "SCHEDULED_RUN_NOT_CANCELABLE", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel scheduled run")
		}
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (h runHandlers) handleGetThreadTranscript(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("thread_id")
	transcript, err := h.runService.GetThreadTranscript(threadID)
	if err != nil {
		if errors.Is(err, awruntime.ErrThreadNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "THREAD_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get thread transcript")
		return
	}

	response := threadTranscriptResponse{
		Thread: transcript.Thread,
		Runs:   make([]threadTranscriptRunResponse, 0, len(transcript.Runs)),
	}
	for _, run := range transcript.Runs {
		frames := make([]agui.Event, 0, len(run.Events))
		for _, event := range run.Events {
			frames = append(frames, mapRuntimeEvent(event)...)
		}
		response.Runs = append(response.Runs, threadTranscriptRunResponse{
			Run:    run.Run,
			Events: frames,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (h runHandlers) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	run, err := h.runService.GetRun(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get run")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h runHandlers) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	afterSeqStr := r.URL.Query().Get("after_seq")
	afterSeq, err := strconv.ParseUint(afterSeqStr, 10, 64)
	if err != nil {
		afterSeq = 0
	}
	// Convert frame-level afterSeq to a coarse runtime-event-level afterSeq for
	// EventBus filtering. We over-include by at most one runtime event (the one
	// containing the afterSeq frame) and filter precisely at the frame level
	// during replay.
	coarseAfter := afterSeq / frameSeqStride
	var eventBusAfter uint64
	if coarseAfter > 0 {
		eventBusAfter = coarseAfter - 1
	}
	sub, err := h.runService.Subscribe(runID, eventBusAfter)
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

	// Replay history with frame-level filtering: expand each runtime event into
	// AG-UI frames and skip frames whose seq <= afterSeq.
	for _, event := range sub.History {
		for _, frame := range mapRuntimeEvent(event) {
			if frame.Seq <= afterSeq {
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

func (h runHandlers) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 0 {
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be a non-negative integer")
			return
		}
		limit = parsed
	}

	writeJSON(w, http.StatusOK, auditEventsResponse{
		Events: h.runService.AuditEvents(awruntime.AuditEventFilter{
			RunID: r.URL.Query().Get("run_id"),
			Limit: limit,
		}),
	})
}

func (h runHandlers) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	run, err := h.runService.CancelRun(runID)
	if err != nil {
		if errors.Is(err, awruntime.ErrRunNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to cancel run")
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (h runHandlers) handleResolveApproval(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	approvalID := r.PathValue("approval_id")

	var req resolveApprovalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	var decision agentcore.ApprovalDecision
	switch req.Decision {
	case string(agentcore.ApprovalDecisionApproved):
		decision = agentcore.ApprovalDecisionApproved
	case string(agentcore.ApprovalDecisionRejected):
		decision = agentcore.ApprovalDecisionRejected
	default:
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_DECISION", "decision must be approved or rejected")
		return
	}

	if err := h.runService.ResolveApproval(runID, approvalID, decision); err != nil {
		switch {
		case errors.Is(err, awruntime.ErrRunNotFound), errors.Is(err, awruntime.ErrApprovalNotFound):
			writeErrorJSON(w, http.StatusNotFound, "APPROVAL_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrRunTerminal):
			writeErrorJSON(w, http.StatusConflict, "RUN_TERMINAL", err.Error())
		case errors.Is(err, awruntime.ErrRunCancelling):
			writeErrorJSON(w, http.StatusConflict, "RUN_CANCELLING", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to resolve approval")
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

	info := provider.GetRuntimeInfo()
	if info.SlashCommands == nil {
		info.SlashCommands = slashcommands.ListCommands()
	}

	data, err := json.Marshal(sanitizeRuntimeInfo(info))
	if err != nil {
		http.Error(w, "failed to serialize runtime info", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func handleCapabilities(w http.ResponseWriter, r *http.Request, provider RuntimeInfoProvider) {
	if provider == nil {
		http.Error(w, "capabilities unavailable", http.StatusServiceUnavailable)
		return
	}

	data, err := json.Marshal(sanitizeCapabilitiesSnapshot(provider.GetRuntimeInfo().Capabilities))
	if err != nil {
		http.Error(w, "failed to serialize capabilities", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func handleAgentDefinitions(w http.ResponseWriter, r *http.Request, load agentDefinitionsLoader) {
	workspaceRoot := strings.TrimSpace(r.URL.Query().Get("workspace_root"))
	if workspaceRoot == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_WORKSPACE", "workspace_root query parameter is required")
		return
	}
	if load == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "AGENT_DEFINITIONS_UNAVAILABLE", "agent definition loader unavailable")
		return
	}

	result, err := load(workspaceRoot)
	if err != nil {
		if errors.Is(err, agentdefs.ErrInvalidWorkspace) {
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load agent definitions")
		return
	}

	writeJSON(w, http.StatusOK, result)
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
		mapped := agui.MapEvent(event.AgentEvent)
		for i := range mapped {
			mapped[i].Seq = frameSeq(event.Seq, i)
		}
		return mapped
	case awruntime.EventKindRunCancelled:
		return []agui.Event{{
			Type:      agui.EventRunError,
			Seq:       frameSeq(event.Seq, 0),
			Timestamp: event.Timestamp.UnixMilli(),
			ThreadID:  event.ThreadID,
			RunID:     event.RunID,
			Message:   redaction.RedactString(event.Message),
			Code:      "RUN_CANCELLED",
		}}
	case awruntime.EventKindRunError:
		return []agui.Event{{
			Type:      agui.EventRunError,
			Seq:       frameSeq(event.Seq, 0),
			Timestamp: event.Timestamp.UnixMilli(),
			ThreadID:  event.ThreadID,
			RunID:     event.RunID,
			Message:   redaction.RedactString(event.Message),
			Code:      event.Code,
		}}
	case awruntime.EventKindRunStatusChanged:
		return []agui.Event{{
			Type:      agui.EventRunStatusChanged,
			Seq:       frameSeq(event.Seq, 0),
			Timestamp: event.Timestamp.UnixMilli(),
			ThreadID:  event.ThreadID,
			RunID:     event.RunID,
			Status:    event.Status,
		}}
	default:
		return nil
	}
}
