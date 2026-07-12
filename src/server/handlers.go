package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"awaken/src/agentcore"
	"awaken/src/protocol/agui"
	awruntime "awaken/src/runtime"
	"awaken/src/skills"
)

type runHandlers struct {
	runService             *awruntime.RunService
	skillRegistry          *skills.SkillRegistry
	syncSkillsForWorkspace func(string)
}

// Keep the local name for the SSE cursor conversion below while sharing the
// actual transport contract with durable semantic projection.
const frameSeqStride uint64 = agui.FrameSeqStride

// frameSeq derives a unique AG-UI frame seq from a runtime event seq and the
// frame index within that event. Because runtime seq is strictly increasing,
// runtimeSeq*stride is strictly increasing, and the sub-index fills the gap,
// guaranteeing monotonicity in stream order.
func frameSeq(runtimeSeq uint64, frameIndex int) uint64 {
	return agui.FrameSeq(runtimeSeq, frameIndex)
}

type renameThreadRequest struct {
	Label string `json:"label"`
}

type resolveMassMessageReviewRequest struct {
	Decision                string `json:"decision"`
	EnvelopeDigest          string `json:"envelopeDigest"`
	BodyDigest              string `json:"bodyDigest"`
	RecipientIdentityDigest string `json:"recipientIdentityDigest"`
}

type answerUserQuestionRequest struct {
	Answer string `json:"answer"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type threadTranscriptRunResponse struct {
	Run          awruntime.Run `json:"run"`
	Events       []agui.Event  `json:"events"`
	StreamCursor uint64        `json:"stream_cursor,omitempty"`
}

type threadTranscriptResponse struct {
	Thread awruntime.ThreadSnapshot      `json:"thread"`
	Runs   []threadTranscriptRunResponse `json:"runs"`
}

func writeErrorJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: code, Message: message})
}

func (h runHandlers) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req awruntime.StartRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if h.syncSkillsForWorkspace != nil {
		h.syncSkillsForWorkspace(req.Workspace.WorkspaceRoot)
	}
	if err := h.validateComposerTokens(req.ComposerTokens); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_COMPOSER_TOKEN", err.Error())
		return
	}

	run, err := h.runService.StartRun(req)
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
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to start run")
		}
		return
	}

	writeJSON(w, http.StatusCreated, run)
}

func (h runHandlers) handleListThreads(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.runService.ListThreads())
}

func (h runHandlers) handleRenameThread(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("thread_id")
	var req renameThreadRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	thread, err := h.runService.RenameThread(threadID, req.Label)
	if err != nil {
		switch {
		case errors.Is(err, awruntime.ErrThreadNotFound):
			writeErrorJSON(w, http.StatusNotFound, "THREAD_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrThreadLabelInvalid):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_THREAD_LABEL", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to rename thread")
		}
		return
	}

	writeJSON(w, http.StatusOK, thread)
}

func (h runHandlers) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("thread_id")
	if err := h.runService.DeleteThread(threadID); err != nil {
		switch {
		case errors.Is(err, awruntime.ErrThreadNotFound):
			writeErrorJSON(w, http.StatusNotFound, "THREAD_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrThreadBusy):
			writeErrorJSON(w, http.StatusConflict, "THREAD_BUSY", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete thread")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
		var frames []agui.Event
		if run.StreamCursor > 0 {
			// Active transcript retains ephemeral frames (reasoning, open text,
			// progress) in EventBus order while durable envelopes still map through
			// the canonical SemanticRecord projector.
			frames = make([]agui.Event, 0, len(run.Events))
			for _, event := range run.Events {
				frames = append(frames, mapRuntimeEvent(event)...)
			}
		} else if len(run.SemanticRecords) > 0 {
			// Durable replay uses the canonical SemanticRecord projector directly.
			// It assigns a distinct stable semantic seq to START/CONTENT/END and
			// other multi-frame records. Live transport resume uses stream_cursor.
			frames = agui.ProjectSemanticRecords(run.SemanticRecords)
		} else {
			frames = make([]agui.Event, 0, len(run.Events))
			for _, event := range run.Events {
				frames = append(frames, mapRuntimeEvent(event)...)
			}
		}
		response.Runs = append(response.Runs, threadTranscriptRunResponse{
			Run:          run.Run,
			Events:       frames,
			StreamCursor: run.StreamCursor,
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

func (h runHandlers) validateComposerTokens(tokens []awruntime.ComposerToken) error {
	if len(tokens) == 0 {
		return nil
	}
	for _, token := range tokens {
		if !strings.EqualFold(strings.TrimSpace(token.Type), "skill") {
			continue
		}
		if h.skillRegistry == nil {
			return errors.New("skill registry not configured")
		}
		skillID := strings.TrimSpace(firstNonEmpty(token.Payload.SkillID, token.ID))
		if skillID == "" {
			return errors.New("skill token is missing skill id")
		}
		def := h.skillRegistry.Get(skillID)
		if def == nil {
			return errors.New("selected skill not found: " + skillID)
		}
		if def.State != skills.StateLoaded {
			if strings.TrimSpace(def.UnavailableReason) != "" {
				return errors.New("selected skill is not loaded: " + def.UnavailableReason)
			}
			return errors.New("selected skill is not loaded: " + skillID)
		}
		if !def.UserInvocable {
			return errors.New("selected skill is not user-invocable: " + skillID)
		}
		if source := strings.TrimSpace(token.Payload.SourceOrigin); source != "" && source != string(def.Source) {
			return errors.New("selected skill source changed: " + skillID)
		}
		if path := strings.TrimSpace(token.Payload.SkillFilePath); path != "" {
			if strings.TrimSpace(def.SkillFilePath) == "" {
				return errors.New("selected skill source has no local package path: " + skillID)
			}
			if cleanPath(path) != cleanPath(def.SkillFilePath) {
				return errors.New("selected skill path changed: " + skillID)
			}
		}
	}
	return nil
}

func cleanPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Clean(path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h runHandlers) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	afterCursorStr := r.URL.Query().Get("after_cursor")
	if afterCursorStr == "" {
		// Compatibility: pre-stream-cursor Desktop clients sent the transport
		// watermark as after_seq.
		afterCursorStr = r.URL.Query().Get("after_seq")
	}
	afterCursor, err := strconv.ParseUint(afterCursorStr, 10, 64)
	if err != nil {
		afterCursor = 0
	}
	// Convert frame-level afterCursor to a coarse runtime-event-level cursor for
	// EventBus filtering. We over-include by at most one runtime event (the one
	// containing the cursor frame) and filter precisely at the frame level
	// during replay.
	coarseAfter := afterCursor / frameSeqStride
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
	// Commit SSE headers even when there is no replay history, allowing the
	// client to know its subscription is established before new events arrive.
	stream.Flush()

	// Replay history with frame-level filtering: expand each runtime event into
	// AG-UI frames and skip frames whose stream cursor is already acknowledged.
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

func (h runHandlers) handleResolveMassMessageReview(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	reviewID := r.PathValue("review_id")
	var req resolveMassMessageReviewRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	var decision agentcore.ReviewDecision
	switch req.Decision {
	case string(agentcore.ReviewDecisionApproved):
		decision = agentcore.ReviewDecisionApproved
	case string(agentcore.ReviewDecisionRejected):
		decision = agentcore.ReviewDecisionRejected
	default:
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REVIEW_DECISION", "decision must be approved or rejected")
		return
	}
	digests := awruntime.MassMessageReviewDigests{
		EnvelopeDigest:          strings.TrimSpace(req.EnvelopeDigest),
		BodyDigest:              strings.TrimSpace(req.BodyDigest),
		RecipientIdentityDigest: strings.TrimSpace(req.RecipientIdentityDigest),
	}
	if digests.EnvelopeDigest == "" || digests.BodyDigest == "" || digests.RecipientIdentityDigest == "" {
		writeErrorJSON(w, http.StatusBadRequest, "MASS_MESSAGE_REVIEW_DIGEST_REQUIRED", "envelopeDigest, bodyDigest, and recipientIdentityDigest are required")
		return
	}

	if err := h.runService.ResolveMassMessageReview(runID, reviewID, decision, digests); err != nil {
		switch {
		case errors.Is(err, awruntime.ErrRunNotFound), errors.Is(err, awruntime.ErrMassMessageReviewNotFound):
			writeErrorJSON(w, http.StatusNotFound, "MASS_MESSAGE_REVIEW_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrRunTerminal):
			writeErrorJSON(w, http.StatusConflict, "RUN_TERMINAL", err.Error())
		case errors.Is(err, awruntime.ErrMassMessageReviewDigestMismatch):
			writeErrorJSON(w, http.StatusConflict, "MASS_MESSAGE_REVIEW_DIGEST_MISMATCH", err.Error())
		case errors.Is(err, awruntime.ErrMassMessageReviewAlreadyResolved):
			writeErrorJSON(w, http.StatusConflict, "MASS_MESSAGE_REVIEW_ALREADY_RESOLVED", err.Error())
		case errors.Is(err, awruntime.ErrMassMessageReviewDigestMissing):
			writeErrorJSON(w, http.StatusBadRequest, "MASS_MESSAGE_REVIEW_DIGEST_REQUIRED", err.Error())
		case errors.Is(err, awruntime.ErrInvalidReviewDecision):
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_REVIEW_DECISION", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to resolve mass message review")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h runHandlers) handleAnswerUserQuestion(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	questionID := r.PathValue("question_id")
	var req answerUserQuestionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	answer := strings.TrimSpace(req.Answer)
	if answer == "" {
		writeErrorJSON(w, http.StatusBadRequest, "EMPTY_ANSWER", "answer is required")
		return
	}

	if err := h.runService.ResolveUserQuestion(runID, questionID, answer); err != nil {
		switch {
		case errors.Is(err, awruntime.ErrRunNotFound), errors.Is(err, awruntime.ErrUserQuestionNotFound):
			writeErrorJSON(w, http.StatusNotFound, "USER_QUESTION_NOT_FOUND", err.Error())
		case errors.Is(err, awruntime.ErrRunTerminal):
			writeErrorJSON(w, http.StatusConflict, "RUN_TERMINAL", err.Error())
		default:
			writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to answer user question")
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
	case awruntime.EventKindSemantic:
		mapped := agui.ProjectSemanticRecord(event.SemanticRecord)
		for i := range mapped {
			cursor := frameSeq(event.Seq, i)
			mapped[i].StreamCursor = cursor
			// Legacy Desktop treats seq as its SSE resume cursor. Keep that wire
			// contract for active/live frames; semanticSeq remains the stable
			// journal-derived identity used by current clients.
			mapped[i].Seq = cursor
		}
		return mapped
	case awruntime.EventKindAgent:
		mapped := agui.MapEvent(event.AgentEvent)
		for i := range mapped {
			cursor := frameSeq(event.Seq, i)
			mapped[i].Seq = cursor
			mapped[i].StreamCursor = cursor
		}
		return mapped
	case awruntime.EventKindRunCancelled:
		return []agui.Event{{
			Type:           agui.EventRunError,
			Seq:            frameSeq(event.Seq, 0),
			StreamCursor:   frameSeq(event.Seq, 0),
			Timestamp:      event.Timestamp.UnixMilli(),
			ThreadID:       event.ThreadID,
			RunID:          event.RunID,
			Message:        event.Message,
			Code:           "RUN_CANCELLED",
			TerminalReason: string(event.TerminalReason),
		}}
	case awruntime.EventKindRunError:
		return []agui.Event{{
			Type:           agui.EventRunError,
			Seq:            frameSeq(event.Seq, 0),
			StreamCursor:   frameSeq(event.Seq, 0),
			Timestamp:      event.Timestamp.UnixMilli(),
			ThreadID:       event.ThreadID,
			RunID:          event.RunID,
			Message:        event.Message,
			Code:           event.Code,
			TerminalReason: string(event.TerminalReason),
		}}
	case awruntime.EventKindRunStatusChanged:
		return []agui.Event{{
			Type:           agui.EventRunStatusChanged,
			Seq:            frameSeq(event.Seq, 0),
			StreamCursor:   frameSeq(event.Seq, 0),
			Timestamp:      event.Timestamp.UnixMilli(),
			ThreadID:       event.ThreadID,
			RunID:          event.RunID,
			Status:         event.Status,
			Message:        event.Message,
			Content:        event.Content,
			Code:           event.Code,
			TerminalReason: string(event.TerminalReason),
		}}
	default:
		return nil
	}
}

// handleGetRunAudit returns audit records for a run.
// GET /v1/runs/{id}/audit
func (h runHandlers) handleGetRunAudit(w http.ResponseWriter, _ *http.Request) {
	// Audit records are written to the AuditSink configured in ToolRuntime.
	// For now, return an empty array — the full audit pipeline requires
	// per-run InMemorySink wiring in RunService (follow-up).
	writeJSON(w, http.StatusOK, []any{})
}
