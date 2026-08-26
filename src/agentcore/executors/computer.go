package executors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"awaken/src/computer"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

// ComputerExecutor routes computer_observe, computer_act and computer_verify
// through the run-bound Computer runtime. When no runtime is bound it fails
// closed with a clear needs-runtime outcome.
type ComputerExecutor struct {
	runtime *computer.Runtime
}

// NewComputerExecutor returns an executor wired to the given runtime. A nil
// runtime is allowed and produces needs-runtime outcomes.
func NewComputerExecutor(runtime *computer.Runtime) *ComputerExecutor {
	return &ComputerExecutor{runtime: runtime}
}

func (e *ComputerExecutor) Execute(
	ctx context.Context,
	intent profile.ExecutionIntent,
	execProfile profile.ExecutionProfile,
	t tool.Tool,
	input string,
	observer tool.ToolObserver,
) (ToolExecutionOutput, error) {
	if intent.Lane != profile.LaneComputer {
		return deniedOutput("computer_lane_mismatch", fmt.Sprintf("tool %q was routed to computer lane but declared lane %q", intent.ToolName, intent.Lane))
	}

	switch strings.ToLower(intent.ToolName) {
	case "computer_observe", "computer_act", "computer_verify":
		if e.runtime == nil {
			return needsRuntimeOutput("Computer runtime is not bound to this run"), nil
		}
	default:
		out, _ := deniedOutput("computer_unknown_tool", fmt.Sprintf("unknown computer tool %q", intent.ToolName))
		return out, nil
	}

	switch strings.ToLower(intent.ToolName) {
	case "computer_observe":
		return e.executeObserve(ctx, input)
	case "computer_act":
		return e.executeAct(ctx, input)
	case "computer_verify":
		return e.executeVerify(ctx, input)
	default:
		out, _ := deniedOutput("computer_unknown_tool", fmt.Sprintf("unknown computer tool %q", intent.ToolName))
		return out, nil
	}
}

type observeInput struct {
	Scope     string   `json:"scope"`
	TargetRef string   `json:"target_ref,omitempty"`
	Include   []string `json:"include"`
	Reason    string   `json:"reason,omitempty"`
}

func parseWindowTargetRef(value string) (computer.TargetRef, error) {
	value = strings.TrimSpace(value)
	const pidPrefix = "pid:"
	const windowSeparator = "/window:"
	if !strings.HasPrefix(value, pidPrefix) {
		return computer.TargetRef{}, fmt.Errorf("target_ref must use pid:<pid>/window:<window_id>")
	}
	parts := strings.SplitN(strings.TrimPrefix(value, pidPrefix), windowSeparator, 2)
	if len(parts) != 2 {
		return computer.TargetRef{}, fmt.Errorf("target_ref must use pid:<pid>/window:<window_id>")
	}
	pid, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil || pid <= 0 {
		return computer.TargetRef{}, fmt.Errorf("target_ref pid must be a positive integer")
	}
	windowID, err := strconv.ParseUint(parts[1], 10, 53)
	if err != nil || windowID == 0 {
		return computer.TargetRef{}, fmt.Errorf("target_ref window_id must be a positive safe integer")
	}
	return computer.TargetRef{
		PID:             int32(pid),
		WindowID:        strconv.FormatUint(windowID, 10),
		Generation:      1,
		CoordinateSpace: string(computer.CaptureWindow),
	}, nil
}

func (e *ComputerExecutor) executeObserve(ctx context.Context, input string) (ToolExecutionOutput, error) {
	var in observeInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return validationOutput("invalid observe input"), nil
	}
	// The CUA SDK's pointer/keyboard tools and get_desktop_state operate in a
	// desktop session. Default there so an omitted scope produces an actually
	// usable first observation instead of a strict window session that cannot
	// capture pixels or inject input.
	scope := computer.CaptureDesktop
	if in.Scope != "" {
		scope = computer.CaptureScope(in.Scope)
	}
	target := computer.TargetRef{Generation: 1, CoordinateSpace: string(scope)}
	if scope == computer.CaptureWindow && strings.TrimSpace(in.TargetRef) != "" {
		parsed, err := parseWindowTargetRef(in.TargetRef)
		if err != nil {
			return validationOutput("invalid window target_ref: " + err.Error()), nil
		}
		target = parsed
	}
	current, active := e.runtime.Session()
	if active {
		if in.TargetRef == "" && current.CaptureScope == scope {
			target = current.Target
			target.CoordinateSpace = string(scope)
		}
	}
	targetChanged := active && scope == computer.CaptureWindow &&
		(current.Target.PID != target.PID || current.Target.WindowID != target.WindowID)
	if active && (current.CaptureScope != scope || targetChanged) {
		closeReason := "capture_scope_changed"
		if targetChanged {
			closeReason = "capture_target_changed"
		}
		if err := e.runtime.Close(ctx, closeReason); err != nil {
			return computerErrorOutput(err, "computer_session_scope_change_failed"), nil
		}
	}
	if _, active := e.runtime.Session(); !active {
		if _, err := e.runtime.OpenSession(ctx, scope, target); err != nil {
			return computerErrorOutput(err, "computer_session_open_failed"), nil
		}
	}
	opts := computer.ObserveOptions{
		Scope:                scope,
		IncludeScreenshot:    contains(in.Include, "screenshot"),
		IncludeAccessibility: contains(in.Include, "accessibility"),
		Reason:               in.Reason,
	}
	obs, err := e.runtime.Observe(ctx, opts)
	if err != nil && isEndedHostSessionError(err) {
		// The sidecar session has its own lifetime. Reopen it transparently at
		// the observation boundary, where retrying is read-only and cannot
		// duplicate input. Never retry a failed action automatically.
		if closeErr := e.runtime.Close(ctx, "host_session_ended"); closeErr != nil {
			return computerErrorOutput(closeErr, "computer_session_recovery_failed"), nil
		}
		if _, openErr := e.runtime.OpenSession(ctx, scope, target); openErr != nil {
			return computerErrorOutput(openErr, "computer_session_recovery_failed"), nil
		}
		obs, err = e.runtime.Observe(ctx, opts)
	}
	if err != nil {
		return computerErrorOutput(err, "observe_failed"), nil
	}
	payloadValue := map[string]any{
		"observation_id":    string(obs.ObservationID),
		"session_id":        string(obs.SessionID),
		"scope":             string(obs.Scope),
		"target_generation": obs.Target.Generation,
		"captured_at":       obs.Freshness.CapturedAt.Format(time.RFC3339Nano),
	}
	if space := obs.CoordinateSpace; space != nil {
		payloadValue["coordinate_unit"] = space.Unit
		payloadValue["screen_width"] = space.Width
		payloadValue["screen_height"] = space.Height
		payloadValue["source_screenshot_width"] = space.SourceScreenshotWidth
		payloadValue["source_screenshot_height"] = space.SourceScreenshotHeight
		payloadValue["backing_scale_x"] = space.ScaleX
		payloadValue["backing_scale_y"] = space.ScaleY
	}
	if obs.Scope == computer.CaptureDesktop {
		// Desktop observation is also the exact-window discovery response. Keep
		// the field present even when there are zero candidates so the model can
		// distinguish an empty discovery from an old/invalid response shape.
		payloadValue["available_windows"] = modelVisibleWindows(obs.AvailableWindows)
	}
	if obs.Target.PID > 0 && obs.Target.WindowID != "" {
		payloadValue["target_ref"] = fmt.Sprintf("pid:%d/window:%s", obs.Target.PID, obs.Target.WindowID)
		payloadValue["target_pid"] = obs.Target.PID
		payloadValue["target_window_id"] = obs.Target.WindowID
	}
	var accessibilityWarning *tool.OutcomeNotice
	if obs.AccessibilityTree != nil {
		if data, readErr := e.runtime.ReadMedia(ctx, *obs.AccessibilityTree); readErr == nil {
			var accessibility any
			if parseErr := json.Unmarshal(data, &accessibility); parseErr == nil {
				// Accessibility element_token values are observation-bound action
				// locators. They must be model-visible; otherwise a background
				// window click can only fall back to best-effort synthetic pixels.
				payloadValue["accessibility"] = accessibility
			} else {
				accessibilityWarning = &tool.OutcomeNotice{
					Code:    "observe_accessibility_invalid",
					Message: fmt.Sprintf("Accessibility was captured but is not valid JSON: %v", parseErr),
				}
			}
		} else if contains(in.Include, "accessibility") {
			accessibilityWarning = &tool.OutcomeNotice{
				Code:    "observe_accessibility_unavailable",
				Message: fmt.Sprintf("Accessibility was requested but could not be read: %v", readErr),
			}
		}
	}
	payload, _ := json.Marshal(payloadValue)
	outcome := tool.SucceededOutcome(string(payload))
	outcome.Details = payloadValue
	if accessibilityWarning != nil {
		outcome.Warnings = append(outcome.Warnings, *accessibilityWarning)
	}
	if obs.Screenshot != nil {
		// Read through the runtime so session ownership and access audit are
		// enforced. Tests without a media store may use a direct local path.
		if data, err := e.runtime.ReadMedia(ctx, *obs.Screenshot); err == nil {
			outcome.ModelContent = append(outcome.ModelContent, tool.ModelContentPart{
				Type:       "image",
				MimeType:   obs.Screenshot.MIMEType,
				ByteSize:   obs.Screenshot.ByteSize,
				StorageKey: obs.Screenshot.StorageKey,
				DataBase64: base64.StdEncoding.EncodeToString(data),
			})
		} else if contains(in.Include, "screenshot") {
			outcome.Warnings = append(outcome.Warnings, tool.OutcomeNotice{
				Code:    "observe_screenshot_unavailable",
				Message: fmt.Sprintf("Screenshot was requested but could not be read: %v", err),
			})
		}
	}
	return ToolExecutionOutput{Outcome: outcome, Content: string(payload)}, nil
}

type actInput struct {
	ObservationID   string         `json:"observation_id"`
	Action          string         `json:"action"`
	Target          actTargetInput `json:"target"`
	Parameters      map[string]any `json:"parameters,omitempty"`
	ExpectedContext map[string]any `json:"expected_context,omitempty"`
	Intent          string         `json:"intent"`
	ExpectedEffect  string         `json:"expected_effect"`
	IdempotencyKey  string         `json:"idempotency_key"`
}

type actTargetInput struct {
	computer.ElementRef
	X         *int     `json:"x,omitempty"`
	Y         *int     `json:"y,omitempty"`
	Key       string   `json:"key,omitempty"`
	Text      string   `json:"text,omitempty"`
	Keys      []string `json:"keys,omitempty"`
	Modifiers []string `json:"modifiers,omitempty"`
}

func (in actInput) normalizedTargetAndParameters() (computer.ElementRef, map[string]any) {
	target := in.Target.ElementRef
	parameters := computer.CloneActionParameters(in.Parameters)
	if target.PixelFallback == nil && in.Target.X != nil && in.Target.Y != nil {
		target.PixelFallback = &computer.Point{X: *in.Target.X, Y: *in.Target.Y}
	} else if target.PixelFallback == nil {
		// Preserve incomplete legacy coordinate input so action-specific
		// validation can report the missing half instead of silently dropping it.
		if in.Target.X != nil {
			parameters["x"] = *in.Target.X
		}
		if in.Target.Y != nil {
			parameters["y"] = *in.Target.Y
		}
	}
	if in.Target.Key != "" {
		if _, ok := parameters["key"]; !ok {
			parameters["key"] = in.Target.Key
		}
	}
	if in.Target.Text != "" {
		if _, ok := parameters["text"]; !ok {
			parameters["text"] = in.Target.Text
		}
	}
	if len(in.Target.Keys) > 0 {
		if _, ok := parameters["keys"]; !ok {
			parameters["keys"] = append([]string(nil), in.Target.Keys...)
		}
	}
	if len(in.Target.Modifiers) > 0 {
		if _, ok := parameters["modifiers"]; !ok {
			parameters["modifiers"] = append([]string(nil), in.Target.Modifiers...)
		}
	}
	return target, parameters
}

func (e *ComputerExecutor) executeAct(ctx context.Context, input string) (ToolExecutionOutput, error) {
	var in actInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return validationOutput("invalid act input"), nil
	}
	if in.ObservationID == "" {
		return validationOutput("observation_id is required"), nil
	}
	target, parameters := in.normalizedTargetAndParameters()
	actionKind := computer.ActionKind(in.Action)
	if actionKind == computer.ActionWait {
		if _, ok := parameters["ms"]; !ok {
			if duration, exists := parameters["duration_ms"]; exists {
				parameters["ms"] = duration
			}
		}
	}
	if actionKind == computer.ActionHotkey {
		if keys, ok := parameters["keys"].(string); ok {
			normalized := splitHotkey(keys)
			if len(normalized) > 0 {
				parameters["keys"] = normalized
			}
		}
	}
	if actionKind == computer.ActionKey {
		if key, ok := parameters["key"].(string); ok {
			keys := normalizedKeyChord(key)
			if len(keys) > 1 {
				actionKind = computer.ActionHotkey
				delete(parameters, "key")
				parameters["keys"] = keys
			}
		}
	}
	spec := computer.ActionSpec{
		Kind:            actionKind,
		Target:          target,
		Parameters:      parameters,
		ExpectedContext: in.ExpectedContext,
		Intent:          in.Intent,
		ExpectedEffect:  in.ExpectedEffect,
		IdempotencyKey:  in.IdempotencyKey,
	}
	if err := computer.ValidateActionSpec(spec); err != nil {
		return validationOutput("invalid computer_act input: " + err.Error()), nil
	}
	obs, err := e.runtime.GetObservation(computer.ObservationID(in.ObservationID))
	if err != nil {
		return computerErrorOutput(err, "computer_stale_observation"), nil
	}
	prepared, err := e.runtime.Prepare(ctx, obs, spec)
	if err != nil {
		return computerErrorOutput(err, "prepare_failed"), nil
	}

	switch prepared.Risk {
	case computer.RiskForbidden:
		return forbiddenActionOutput(prepared), nil
	case computer.RiskReview:
		return e.requestReview(ctx, prepared)
	case computer.RiskHandoff:
		return e.requestHandoff(ctx, prepared)
	}

	res, err := e.runtime.Perform(ctx, prepared, nil)
	if err != nil {
		return computerErrorOutput(err, "perform_failed"), nil
	}
	return e.actionResultOutput(ctx, prepared, res, "")
}

func splitHotkey(value string) []string {
	parts := strings.Split(value, "+")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			keys = append(keys, trimmed)
		}
	}
	return keys
}

func normalizedKeyChord(value string) []string {
	switch strings.TrimSpace(value) {
	case "*":
		return []string{"shift", "8"}
	case "+":
		return []string{"shift", "="}
	default:
		return splitHotkey(value)
	}
}

func isEndedHostSessionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "session") &&
		(strings.Contains(message, "has ended") ||
			strings.Contains(message, "already ended") ||
			strings.Contains(message, "session ended"))
}

func (e *ComputerExecutor) actionResultOutput(ctx context.Context, prepared computer.PreparedAction, res computer.ActionResult, reviewID computer.ReviewID) (ToolExecutionOutput, error) {
	var afterObservation *computer.Observation
	if shouldCaptureAfterAction(res) {
		if observed, err := e.runtime.Observe(ctx, computer.ObserveOptions{
			IncludeScreenshot:    true,
			IncludeAccessibility: true,
			Reason:               "capture the state immediately after the checked action",
		}); err == nil {
			afterObservation = &observed
			_ = e.runtime.AttachAfterObservation(prepared.ActionID, observed.ObservationID)
		}
	}
	payloadValue := map[string]any{
		"action_id":             string(prepared.ActionID),
		"status":                string(res.Status),
		"source_observation_id": string(prepared.ObservationID),
	}
	if res.ErrorCode != "" {
		payloadValue["error_code"] = res.ErrorCode
	}
	if res.ErrorMessage != "" {
		payloadValue["error_message"] = res.ErrorMessage
	}
	if res.CUAStatus != "" {
		payloadValue["cua_status"] = res.CUAStatus
	}
	if res.CUAEffect != "" {
		payloadValue["cua_effect"] = res.CUAEffect
	}
	if res.CUARoute != "" {
		payloadValue["cua_route"] = res.CUARoute
	}
	if res.CUADeliveryMode != "" {
		payloadValue["cua_delivery_mode"] = res.CUADeliveryMode
	}
	if reviewID != "" {
		payloadValue["review_id"] = string(reviewID)
	}
	var observationWarnings []tool.OutcomeNotice
	if afterObservation != nil {
		payloadValue["observation_id"] = string(afterObservation.ObservationID)
		postActionObservation, warnings := e.modelVisibleObservation(ctx, *afterObservation)
		payloadValue["post_action_observation"] = postActionObservation
		observationWarnings = warnings
	}
	payload, _ := json.Marshal(payloadValue)
	var outcome tool.ExecutionOutcome
	switch res.Status {
	case computer.ActionSucceeded:
		outcome = tool.SucceededOutcome(string(payload))
	case computer.ActionFailed:
		outcome = tool.FailedOutcome(string(payload), "computer_action_failed", tool.ErrorCategoryRuntime, true)
	case computer.ActionUnknownOutcome:
		outcome = tool.FailedOutcome(string(payload), "computer_unknown_outcome", tool.ErrorCategoryRuntime, false)
	case computer.ActionCancelledBeforePerform:
		outcome = tool.FailedOutcome(string(payload), tool.ErrorCodeToolCancelled, tool.ErrorCategoryCancelled, false)
	}
	outcome.Details = map[string]any{
		"action_id":             string(prepared.ActionID),
		"action_status":         string(res.Status),
		"source_observation_id": string(prepared.ObservationID),
		"risk":                  string(prepared.Risk),
	}
	if res.ErrorCode != "" {
		outcome.Details["host_error_code"] = res.ErrorCode
	}
	if res.ErrorCategory != "" {
		outcome.Details["host_error_category"] = res.ErrorCategory
	}
	if res.ErrorMessage != "" {
		outcome.Details["host_error_message"] = res.ErrorMessage
	}
	if res.CUAStatus != "" {
		outcome.Details["cua_status"] = res.CUAStatus
	}
	if res.CUAEffect != "" {
		outcome.Details["cua_effect"] = res.CUAEffect
	}
	if res.CUARoute != "" {
		outcome.Details["cua_route"] = res.CUARoute
	}
	if res.CUADeliveryMode != "" {
		outcome.Details["cua_delivery_mode"] = res.CUADeliveryMode
	}
	if reviewID != "" {
		outcome.Details["review_id"] = string(reviewID)
	}
	if afterObservation != nil {
		outcome.Details["observation_id"] = string(afterObservation.ObservationID)
		outcome.Details["post_action_observation"] = payloadValue["post_action_observation"]
		outcome.Warnings = append(outcome.Warnings, observationWarnings...)
		e.attachObservationMedia(ctx, &outcome, *afterObservation)
	} else if res.Status == computer.ActionSucceeded {
		outcome.Warnings = append(outcome.Warnings, tool.OutcomeNotice{
			Code:    "computer_after_observation_unavailable",
			Message: "The action completed, but its post-action observation could not be captured.",
		})
	}
	return ToolExecutionOutput{Outcome: outcome, Content: string(payload)}, nil
}

func shouldCaptureAfterAction(res computer.ActionResult) bool {
	if res.Status == computer.ActionSucceeded {
		return true
	}
	if res.Status != computer.ActionFailed {
		return false
	}
	// These failures are confirmed before OS input. Capturing another CUA
	// accessibility snapshot would only supersede the model's current element
	// tokens without providing any new UI evidence.
	if res.ErrorCategory == "validation" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(res.ErrorCode)) {
	case "stale_element_token", "invalid_action_parameters":
		return false
	default:
		return true
	}
}

func (e *ComputerExecutor) modelVisibleObservation(ctx context.Context, obs computer.Observation) (map[string]any, []tool.OutcomeNotice) {
	payload := map[string]any{
		"observation_id":    string(obs.ObservationID),
		"session_id":        string(obs.SessionID),
		"scope":             string(obs.Scope),
		"target_generation": obs.Target.Generation,
		"captured_at":       obs.Freshness.CapturedAt.Format(time.RFC3339Nano),
	}
	if space := obs.CoordinateSpace; space != nil {
		payload["coordinate_unit"] = space.Unit
		payload["screen_width"] = space.Width
		payload["screen_height"] = space.Height
		payload["source_screenshot_width"] = space.SourceScreenshotWidth
		payload["source_screenshot_height"] = space.SourceScreenshotHeight
		payload["backing_scale_x"] = space.ScaleX
		payload["backing_scale_y"] = space.ScaleY
	}
	if obs.Scope == computer.CaptureDesktop {
		payload["available_windows"] = modelVisibleWindows(obs.AvailableWindows)
	}
	if obs.Target.PID > 0 && obs.Target.WindowID != "" {
		payload["target_ref"] = fmt.Sprintf("pid:%d/window:%s", obs.Target.PID, obs.Target.WindowID)
		payload["target_pid"] = obs.Target.PID
		payload["target_window_id"] = obs.Target.WindowID
	}
	if obs.AccessibilityTree == nil {
		return payload, nil
	}
	data, err := e.runtime.ReadMedia(ctx, *obs.AccessibilityTree)
	if err != nil {
		return payload, []tool.OutcomeNotice{{
			Code:    "computer_after_accessibility_unavailable",
			Message: fmt.Sprintf("Post-action accessibility was captured but could not be read: %v", err),
		}}
	}
	var accessibility any
	if err := json.Unmarshal(data, &accessibility); err != nil {
		return payload, []tool.OutcomeNotice{{
			Code:    "computer_after_accessibility_invalid",
			Message: fmt.Sprintf("Post-action accessibility was captured but is not valid JSON: %v", err),
		}}
	}
	// CUA element tokens are snapshot-bound. Returning the replacement tree in
	// the same action result makes the new observation immediately actionable
	// and prevents callers from reusing superseded source-observation tokens.
	payload["accessibility"] = accessibility
	return payload, nil
}

func modelVisibleWindows(windows []computer.WindowRef) []computer.WindowRef {
	if windows == nil {
		return []computer.WindowRef{}
	}
	return windows
}

func (e *ComputerExecutor) attachObservationMedia(ctx context.Context, outcome *tool.ExecutionOutcome, obs computer.Observation) {
	if outcome == nil || obs.Screenshot == nil {
		return
	}
	data, err := e.runtime.ReadMedia(ctx, *obs.Screenshot)
	if err != nil {
		outcome.Warnings = append(outcome.Warnings, tool.OutcomeNotice{
			Code: "computer_screenshot_unavailable", Message: err.Error(),
		})
		return
	}
	outcome.ModelContent = append(outcome.ModelContent, tool.ModelContentPart{
		Type:       "image",
		MimeType:   obs.Screenshot.MIMEType,
		ByteSize:   obs.Screenshot.ByteSize,
		StorageKey: obs.Screenshot.StorageKey,
		DataBase64: base64.StdEncoding.EncodeToString(data),
	})
}

func (e *ComputerExecutor) requestReview(ctx context.Context, prepared computer.PreparedAction) (ToolExecutionOutput, error) {
	review, err := e.runtime.RequestReview(ctx, computer.RequestReviewInput{
		Action:        prepared,
		TargetSummary: fmt.Sprintf("%s on %s", prepared.Kind, prepared.Target.ElementLabel),
		EffectSummary: prepared.ExpectedEffect,
		EvidenceRefs:  []string{string(prepared.ObservationID)},
	})
	if err != nil {
		return computerErrorOutput(err, "review_request_failed"), nil
	}

	resolved, err := e.runtime.WaitForReview(ctx, review.ReviewID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return cancelledReviewOutput(prepared, review.ReviewID), nil
		}
		return computerErrorOutput(err, "review_wait_failed"), nil
	}

	if resolved.Decision != computer.ReviewApprovedOnce {
		msg := fmt.Sprintf("review %s resolved as %s", review.ReviewID, resolved.Decision)
		outcome := tool.FailedOutcome(msg, "computer_review_denied", tool.ErrorCategoryAuthorization, false)
		outcome.Details = map[string]any{
			"action_id":      string(prepared.ActionID),
			"review_id":      string(review.ReviewID),
			"observation_id": string(prepared.ObservationID),
			"decision":       string(resolved.Decision),
			"actor":          resolved.Actor,
		}
		return ToolExecutionOutput{Outcome: outcome, Content: msg, IsError: true, ErrorCode: "computer_review_denied"}, nil
	}

	res, err := e.runtime.Perform(ctx, prepared, resolved)
	if err != nil {
		return computerErrorOutput(err, "perform_failed"), nil
	}
	return e.actionResultOutput(ctx, prepared, res, review.ReviewID)
}

func cancelledReviewOutput(prepared computer.PreparedAction, reviewID computer.ReviewID) ToolExecutionOutput {
	msg := "computer review wait was cancelled"
	outcome := tool.FailedOutcome(msg, "computer_review_cancelled", tool.ErrorCategoryCancelled, false)
	outcome.Details = map[string]any{
		"action_id":      string(prepared.ActionID),
		"review_id":      string(reviewID),
		"observation_id": string(prepared.ObservationID),
	}
	return ToolExecutionOutput{Outcome: outcome, Content: msg, IsError: true, ErrorCode: "computer_review_cancelled"}
}

func (e *ComputerExecutor) requestHandoff(ctx context.Context, prepared computer.PreparedAction) (ToolExecutionOutput, error) {
	if err := e.runtime.Takeover(ctx); err != nil {
		return computerErrorOutput(err, "handoff_failed"), nil
	}
	if err := e.runtime.WaitForReturn(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return cancelledHandoffOutput(prepared), nil
		}
		return computerErrorOutput(err, "handoff_wait_failed"), nil
	}
	obs, err := e.runtime.Observe(ctx, computer.ObserveOptions{
		IncludeScreenshot:    true,
		IncludeAccessibility: true,
		Reason:               "capture the state after the user returned computer control",
	})
	if err != nil {
		return computerErrorOutput(err, "handoff_return_observe_failed"), nil
	}
	payload, _ := json.Marshal(map[string]any{
		"action_id":      string(prepared.ActionID),
		"risk":           string(prepared.Risk),
		"observation_id": string(obs.ObservationID),
		"handoff":        "completed",
	})
	outcome := tool.SucceededOutcome(string(payload))
	outcome.Summary = "User completed the sensitive step and returned computer control"
	outcome.Details = map[string]any{
		"action_id":             string(prepared.ActionID),
		"source_observation_id": string(prepared.ObservationID),
		"observation_id":        string(obs.ObservationID),
		"risk":                  string(prepared.Risk),
		"handoff":               "completed",
	}
	e.attachObservationMedia(ctx, &outcome, obs)
	return ToolExecutionOutput{Outcome: outcome, Content: string(payload)}, nil
}

func cancelledHandoffOutput(prepared computer.PreparedAction) ToolExecutionOutput {
	msg := "computer handoff wait was cancelled"
	outcome := tool.FailedOutcome(msg, "computer_handoff_cancelled", tool.ErrorCategoryCancelled, false)
	outcome.Details = map[string]any{
		"action_id":      string(prepared.ActionID),
		"observation_id": string(prepared.ObservationID),
		"risk":           string(prepared.Risk),
	}
	return ToolExecutionOutput{Outcome: outcome, Content: msg, IsError: true, ErrorCode: "computer_handoff_cancelled"}
}

func forbiddenActionOutput(prepared computer.PreparedAction) ToolExecutionOutput {
	payload, _ := json.Marshal(map[string]any{
		"action_id": string(prepared.ActionID),
		"risk":      string(prepared.Risk),
	})
	msg := fmt.Sprintf("action %s is forbidden by policy", prepared.Kind)
	outcome := tool.FailedOutcome(msg, "computer_action_forbidden", tool.ErrorCategoryPolicy, false)
	outcome.Details = map[string]any{
		"action_id":      string(prepared.ActionID),
		"observation_id": string(prepared.ObservationID),
		"risk":           string(prepared.Risk),
	}
	return ToolExecutionOutput{Outcome: outcome, Content: string(payload), IsError: true, ErrorCode: "computer_action_forbidden"}
}

type verifyInput struct {
	ActionID       string               `json:"action_id"`
	ExpectedEffect string               `json:"expected_effect"`
	Assertions     []computer.Assertion `json:"assertions"`
}

func (e *ComputerExecutor) executeVerify(ctx context.Context, input string) (ToolExecutionOutput, error) {
	var in verifyInput
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return validationOutput("invalid verify input"), nil
	}
	obs, err := e.runtime.Observe(ctx, computer.ObserveOptions{
		IncludeScreenshot:    true,
		IncludeAccessibility: true,
		Reason:               "independently verify the previous computer action",
	})
	if err != nil {
		return computerErrorOutput(err, "verify_observe_failed"), nil
	}
	for i := range in.Assertions {
		in.Assertions[i].Actual = ""
		in.Assertions[i].Matched = false
	}
	if obs.AccessibilityTree != nil {
		if tree, readErr := e.runtime.ReadMedia(ctx, *obs.AccessibilityTree); readErr == nil {
			evaluateAssertionsFromAccessibility(in.Assertions, string(tree))
		}
	}
	res, err := e.runtime.Verify(ctx, computer.ActionID(in.ActionID), obs, in.Assertions)
	if err != nil {
		return computerErrorOutput(err, "verify_failed"), nil
	}
	payloadValue := map[string]any{
		"action_id":           string(res.ActionID),
		"verification_status": string(res.Status),
		"observation_id":      string(res.ObservationID),
		"assertions":          res.Assertions,
	}
	payload, _ := json.Marshal(payloadValue)
	outcome := tool.SucceededOutcome(string(payload))
	outcome.Details = payloadValue
	e.attachObservationMedia(ctx, &outcome, obs)
	return ToolExecutionOutput{Outcome: outcome, Content: string(payload)}, nil
}

func evaluateAssertionsFromAccessibility(assertions []computer.Assertion, tree string) {
	treeLower := strings.ToLower(tree)
	for i := range assertions {
		target := strings.ToLower(strings.TrimSpace(assertions[i].Target))
		expected := strings.TrimSpace(assertions[i].Expected)
		switch strings.ToLower(strings.TrimSpace(assertions[i].Type)) {
		case "element_present":
			assertions[i].Actual = fmt.Sprintf("%t", target != "" && strings.Contains(treeLower, target))
		case "element_absent":
			assertions[i].Actual = fmt.Sprintf("%t", target != "" && !strings.Contains(treeLower, target))
		case "text", "text_contains":
			assertions[i].Actual = fmt.Sprintf("%t", expected != "" && strings.Contains(treeLower, strings.ToLower(expected)))
			if expected != "true" && expected != "false" {
				assertions[i].Expected = "true"
			}
		}
	}
}

func needsRuntimeOutput(message string) ToolExecutionOutput {
	outcome := tool.ExecutionOutcome{
		Status:          tool.OutcomeNeedsRuntime,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		Content:         message,
		Summary:         "Computer Use runtime is not available",
		ErrorCode:       "computer_needs_runtime",
		ErrorCategory:   tool.ErrorCategoryRuntime,
		ErrorMessage:    message,
		Retryable:       false,
	}
	return ToolExecutionOutput{Outcome: outcome, Content: message, IsError: true, ErrorCode: "computer_needs_runtime"}
}

func validationOutput(message string) ToolExecutionOutput {
	outcome := tool.FailedOutcome(message, tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
	outcome.TransportStatus = tool.TransportNotStarted
	outcome.Operation = &tool.OperationDescriptor{Type: "computer_act", SideEffectState: tool.OutcomeSideEffectNotStarted}
	outcome.RecoveryOptions = []string{"correct_action_parameters", "observe_again_if_the_screen_changed"}
	return ToolExecutionOutput{Outcome: outcome, Content: message, IsError: true, ErrorCode: tool.ErrorCodeValidationError}
}

func computerErrorOutput(err error, code string) ToolExecutionOutput {
	msg := err.Error()
	outcome := tool.FailedOutcome(msg, code, tool.ErrorCategoryRuntime, false)
	return ToolExecutionOutput{Outcome: outcome, Content: msg, IsError: true, ErrorCode: code}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(item, want) {
			return true
		}
	}
	return false
}
