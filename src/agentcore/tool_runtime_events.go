package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"strings"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tools/ask_user"
)

func (rt *ToolRuntime) decideToolPermission(
	ctx stdcontext.Context,
	selectedTool tool.Tool,
	definition tool.ToolDefinition,
	input string,
) tool.PermissionDecision {
	if checker, ok := selectedTool.(tool.PermissionCheckingTool); ok {
		decision := checker.CheckPermission(ctx, input, rt.permissionContext)
		if decision.Behavior == "" {
			return tool.DenyPermission("tool returned an empty permission decision")
		}
		return decision
	}

	// Fall back to annotations-based decision
	return tool.DecideWithServerCheck(ctx, definition, rt.serverConnected)
}

func (rt *ToolRuntime) emitArtifacts(
	result ToolExecutionResult,
	selectedTool tool.Tool,
	input string,
	output string,
) []tool.ArtifactDescriptor {
	producingTool, ok := selectedTool.(tool.ArtifactProducingTool)
	if !ok {
		return nil
	}

	var artifacts []tool.ArtifactDescriptor
	for _, artifact := range producingTool.Artifacts(input, output, rt.permissionContext) {
		eventType := ArtifactUpdatedEventType
		if artifact.Action == tool.ArtifactActionCreated || artifact.Action == tool.ArtifactActionPublished {
			eventType = ArtifactCreatedEventType
		}
		contractVersion := artifact.ContractVersion
		if contractVersion <= 0 {
			contractVersion = tool.ArtifactContractVersion
		}
		producer := firstNonEmpty(strings.TrimSpace(artifact.Producer), result.Name)
		role := tool.ResolveArtifactRole(artifact.Role, producer, artifact.Kind, artifact.Path)
		artifact.ContractVersion = contractVersion
		artifact.Producer = producer
		artifact.Role = role
		artifact.ID = firstNonEmpty(strings.TrimSpace(artifact.ID), newAwakenID("artifact"))
		artifact.Title = firstNonEmpty(strings.TrimSpace(artifact.Title), fallbackArtifactTitle(artifact.Path, result.Name))
		artifact.Description = strings.TrimSpace(artifact.Description)
		artifact.Kind = firstNonEmpty(strings.TrimSpace(artifact.Kind), "file")
		artifact.Path = strings.TrimSpace(artifact.Path)
		artifact.Status = string(normalizeArtifactStatus(artifact.Status))
		artifacts = append(artifacts, artifact)
		rt.sink.Emit(AwakenEvent{
			Type:                    eventType,
			Timestamp:               timeNowUTC(),
			ToolCallID:              result.ToolCallID,
			ToolName:                result.Name,
			ArtifactContractVersion: contractVersion,
			ArtifactRole:            role,
			ArtifactProducer:        producer,
			ArtifactID:              artifact.ID,
			ArtifactTitle:           artifact.Title,
			ArtifactDescription:     artifact.Description,
			ArtifactKind:            artifact.Kind,
			ArtifactPath:            artifact.Path,
			ArtifactContent:         artifact.Content,
			ArtifactStatus:          ArtifactStatus(artifact.Status),
		})
	}
	return artifacts
}

func (rt *ToolRuntime) emitCommandOutput(result ToolExecutionResult, output tool.CommandOutput) {
	if output.Delta == "" {
		return
	}
	rt.sink.Emit(AwakenEvent{
		Type:          CommandOutputEventType,
		Timestamp:     timeNowUTC(),
		ToolCallID:    result.ToolCallID,
		ToolName:      result.Name,
		CommandStream: output.Stream,
		Delta:         output.Delta,
		Truncated:     output.Truncated,
	})
}

func (rt *ToolRuntime) emitToolProgress(result ToolExecutionResult, stage, message string, percentage int) {
	event := AwakenEvent{
		Type:                   ToolProgressEventType,
		Timestamp:              timeNowUTC(),
		ToolCallID:             result.ToolCallID,
		ToolName:               result.Name,
		ToolProgressStage:      stage,
		ToolProgressMessage:    message,
		ToolProgressPercentage: percentage,
		Progress:               message,
	}
	applyToolOutcomeFields(&event, result)
	rt.sink.Emit(event)
}

func (rt *ToolRuntime) emitHostActivity(result ToolExecutionResult, selectedTool tool.Tool, input string) {
	activity := tool.DefaultActivity(result.Name)
	if describing, ok := selectedTool.(tool.ActivityDescribingTool); ok {
		described := describing.DescribeActivity(input)
		if strings.TrimSpace(described.Title) != "" || strings.TrimSpace(described.Stage) != "" {
			activity = described
		}
	}
	stage := firstNonEmpty(strings.TrimSpace(activity.Stage), "started")
	message := firstNonEmpty(strings.TrimSpace(activity.Title), strings.TrimSpace(activity.Detail), "正在执行 "+result.Name)
	if detail := strings.TrimSpace(activity.Detail); detail != "" && detail != message {
		message = message + ": " + detail
	}
	rt.emitToolProgress(result, stage, message, 0)
}

func auditOutcomeLabel(result ToolExecutionResult) string {
	if result.Outcome.Status != "" {
		return string(result.Outcome.Status)
	}
	if result.IsError {
		return "failed"
	}
	if result.Status != "" {
		return string(result.Status)
	}
	return "succeeded"
}

func (rt *ToolRuntime) emitAuthRequired(result ToolExecutionResult, def tool.ToolDefinition, decision ToolDecision) {
	rt.sink.Emit(AwakenEvent{
		Type:               AuthRequiredEventType,
		Timestamp:          timeNowUTC(),
		ToolCallID:         result.ToolCallID,
		ToolName:           result.Name,
		Content:            decision.Reason,
		ToolDecisionAction: string(decision.Action),
		ToolDecisionSource: string(decision.Source),
		ToolDecisionReason: strings.TrimSpace(decision.Reason),
		ToolDefinition:     def,
		AuthProvider:       firstNonEmpty(def.Annotations.ServerName, def.Annotations.ToolName, string(def.Source)),
	})
}

func (rt *ToolRuntime) emitRuntimeMissing(result ToolExecutionResult, decision ToolDecision) {
	rt.sink.Emit(AwakenEvent{
		Type:               RuntimeMissingEventType,
		Timestamp:          timeNowUTC(),
		ToolCallID:         result.ToolCallID,
		ToolName:           result.Name,
		Content:            decision.Reason,
		ToolDecisionAction: string(decision.Action),
		ToolDecisionSource: string(decision.Source),
		ToolDecisionReason: strings.TrimSpace(decision.Reason),
		RuntimeName:        "host-environment",
	})
}

func (rt *ToolRuntime) emitRuntimePrepareStarted(toolCallID, toolName, message string) {
	rt.sink.Emit(AwakenEvent{
		Type:        RuntimePrepareStartedEventType,
		Timestamp:   timeNowUTC(),
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Content:     message,
		RuntimeName: "host-environment",
	})
}

func (rt *ToolRuntime) emitRuntimePrepareProgress(toolCallID, toolName, message, progress string) {
	rt.sink.Emit(AwakenEvent{
		Type:        RuntimePrepareProgressEventType,
		Timestamp:   timeNowUTC(),
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Content:     message,
		Progress:    progress,
		RuntimeName: "host-environment",
	})
}

func (rt *ToolRuntime) emitRuntimeReady(toolCallID, toolName, version, message string) {
	rt.sink.Emit(AwakenEvent{
		Type:           RuntimeReadyEventType,
		Timestamp:      timeNowUTC(),
		ToolCallID:     toolCallID,
		ToolName:       toolName,
		Content:        message,
		RuntimeName:    "host-environment",
		RuntimeVersion: version,
	})
}

func (rt *ToolRuntime) emitRuntimeRepairFailed(toolCallID, toolName, message string) {
	rt.sink.Emit(AwakenEvent{
		Type:        RuntimeRepairFailedEventType,
		Timestamp:   timeNowUTC(),
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		Content:     message,
		Error:       message,
		RuntimeName: "host-environment",
	})
}

func (rt *ToolRuntime) massMessageReviewEvent(result ToolExecutionResult, def tool.ToolDefinition, permission tool.PermissionDecision) AwakenEvent {
	metadata := permission.Metadata.MassMessage
	if metadata == nil {
		metadata = def.Annotations.MassMessage
	}
	msg := tool.MassMessageMetadata{}
	if metadata != nil {
		msg = *metadata
	}
	if strings.TrimSpace(msg.ReviewID) == "" {
		msg.ReviewID = newAwakenID("mass_review")
	}
	if strings.TrimSpace(msg.SourceTool) == "" {
		msg.SourceTool = result.Name
	}
	return AwakenEvent{
		Type:                          MassMessageReviewRequestedType,
		Timestamp:                     timeNowUTC(),
		ToolCallID:                    result.ToolCallID,
		ToolName:                      result.Name,
		ToolDefinition:                def,
		ReviewID:                      msg.ReviewID,
		ReviewEnvelopeDigest:          msg.EnvelopeDigest,
		ReviewBodyDigest:              msg.BodyDigest,
		ReviewRecipientIdentityDigest: msg.RecipientIdentityDigest,
		MassMessageChannel:            msg.Channel,
		MassMessageRecipientScope:     msg.RecipientScope,
		MassMessageRecipientCount:     msg.RecipientCount,
		MassMessageMessageCount:       msg.MessageCount,
		MassMessageExpandsRecipients:  msg.ExpandsRecipients,
		MassMessageBodyPreview:        msg.BodyPreview,
		MassMessageTemplatePreview:    msg.TemplatePreview,
		MassMessageAttachments:        append([]string(nil), msg.Attachments...),
		MassMessageSendingAccount:     msg.SendingAccount,
		MassMessageSourceTool:         msg.SourceTool,
	}
}

func (rt *ToolRuntime) userQuestionEvent(result ToolExecutionResult, def tool.ToolDefinition, questions []askuser.Question) AwakenEvent {
	questionID := newAwakenID("user_question")
	contentBytes, err := json.Marshal(map[string]any{
		"questions": questions,
	})
	if err != nil {
		contentBytes = []byte("{}")
	}
	return AwakenEvent{
		Type:           UserQuestionRequestedType,
		Timestamp:      timeNowUTC(),
		ToolCallID:     result.ToolCallID,
		ToolName:       result.Name,
		ToolDefinition: def,
		QuestionID:     questionID,
		Content:        string(contentBytes),
	}
}

func (rt *ToolRuntime) writeAudit(def tool.ToolDefinition, result ToolExecutionResult, permission tool.PermissionDecision, decision, outcome, errReason string) {
	started := result.StartedAt
	if started.IsZero() {
		started = timeNowUTC()
	}
	finished := result.FinishedAt
	if finished.IsZero() {
		finished = timeNowUTC()
	}
	origin := "system_policy"
	if permission.Behavior == tool.PermissionAllow {
		origin = "auto_allow"
	}
	rt.auditSink.Write(tool.ToolAuditRecord{
		ToolCallID:     result.ToolCallID,
		RunID:          rt.runID,
		ToolName:       result.Name,
		Source:         def.Source,
		Annotations:    def.Annotations,
		Decision:       decision,
		DecisionReason: permission.Reason,
		DecisionOrigin: origin,
		StartedAt:      started,
		CompletedAt:    finished,
		Outcome:        outcome,
		ResultSummary:  result.ResultSummary,
		ErrorReason:    errReason,
		// Task 16 unified observability: populate the structured fields from
		// the result so the audit log carries the full profile / intent /
		// decision / transform / trash / subagent / guardian picture. The
		// legacy Decision/DecisionReason fields above remain for
		// backwards-compatible consumers. Note: the `decision` parameter
		// passed in from call sites is the decision SOURCE string (e.g.
		// "tool_rule"), NOT the action; DecisionAction must be derived from
		// the unified ExecutionDecision on the result so it carries
		// "allow"/"deny"/"transform" as the spec requires.
		ProfileName:       result.ProfileName,
		Intent:            result.Intent,
		DecisionAction:    decisionAction(result),
		DecisionSource:    string(result.DecisionSource),
		TransformedFrom:   result.TransformedFrom,
		TransformedTo:     transformedToName(result),
		TrashedPaths:      append([]string(nil), result.TrashedPaths...),
		SubagentOrigin:    rt.subagentOrigin,
		GuardianViolation: result.GuardianViolation,
	})
}

// decisionAction returns the unified decision action ("allow"/"deny"/
// "transform") from the result's ExecutionDecision. It returns "" when the
// unified decision is absent (e.g. unknown-tool failures that bypass the
// decision layer).
func decisionAction(result ToolExecutionResult) string {
	if result.Decision == nil {
		return ""
	}
	return string(result.Decision.Action)
}

// transformedToName returns the rewritten tool name when the result is the
// target of a transform decision, or "" otherwise. It inspects the
// ExecutionDecision carried on the result: for transform decisions, the
// decision's UpdatedToolName is the rewritten tool (e.g. "apply_patch").
func transformedToName(result ToolExecutionResult) string {
	if result.Decision == nil {
		return ""
	}
	if result.Decision.Action != profile.DecisionTransform {
		return ""
	}
	return result.Decision.UpdatedToolName
}

// extractTrashedPaths parses the apply_patch tool's summary output and
// returns the trashed-path component of every deletion line. The apply_patch
// tool encodes deletions as either:
//
//	D path (已移到回收站: trashedPath)
//	D path (已移到回收站)
//
// In the first form the trashed-path component is returned verbatim. In the
// second form (no explicit trashed path was surfaced) the line contributes
// nothing — the file was trashed but the OS did not report a destination.
// Lines that do not match the deletion pattern are ignored.
//
// This is a best-effort parser keyed off the Chinese marker the apply_patch
// tool emits today. If the marker changes, this parser must be updated in
// lockstep.
func extractTrashedPaths(output string) []string {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	const trashMarker = "已移到回收站"
	var paths []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "D ") {
			continue
		}
		idx := strings.Index(trimmed, trashMarker)
		if idx < 0 {
			continue
		}
		tail := trimmed[idx+len(trashMarker):]
		// Expect either ": trashedPath)" or ")".
		tail = strings.TrimPrefix(tail, ":")
		tail = strings.TrimSpace(tail)
		tail = strings.TrimSuffix(tail, ")")
		tail = strings.TrimSpace(tail)
		if tail == "" {
			continue
		}
		paths = append(paths, tail)
	}
	return paths
}

func (rt *ToolRuntime) emitPatchPreview(result ToolExecutionResult, selectedTool tool.Tool, input string) {
	previewingTool, ok := selectedTool.(tool.PatchPreviewingTool)
	if !ok {
		return
	}
	preview, ok := previewingTool.PatchPreview(input, rt.permissionContext)
	if !ok {
		return
	}
	limited := outputlimit.Limit(preview.Patch, maxPatchPreviewChars)
	rt.sink.Emit(AwakenEvent{
		Type:          PatchUpdatedEventType,
		Timestamp:     timeNowUTC(),
		ToolCallID:    result.ToolCallID,
		ToolName:      result.Name,
		Patch:         limited.Content,
		PatchSummary:  strings.TrimSpace(preview.Summary),
		AffectedPaths: preview.AffectedPaths,
		Truncated:     preview.Truncated || limited.Truncated,
	})
}

func (rt *ToolRuntime) emitToolPermissionDecided(
	result ToolExecutionResult,
	permission tool.PermissionDecision,
	decision ToolDecision,
) {
	rt.sink.Emit(AwakenEvent{
		Type:                     ToolPermissionDecidedEventType,
		Timestamp:                timeNowUTC(),
		ToolCallID:               result.ToolCallID,
		ToolName:                 result.Name,
		Content:                  decision.Reason,
		StaticPermissionBehavior: string(permission.Behavior),
		StaticPermissionReason:   strings.TrimSpace(permission.Reason),
		ToolDecisionAction:       string(decision.Action),
		ToolDecisionSource:       string(decision.Source),
		ToolDecisionReason:       strings.TrimSpace(decision.Reason),
		AffectedPaths:            result.AffectedPaths,
		PatchSummary:             result.PatchSummary,
		// Task 16 unified observability: surface the profile name and the
		// transform origin (original tool name + rewritten tool name) so the
		// Desktop UI can render "Profile: workspace_write" and
		// "shell → apply_patch" alongside the decision. TrashedPaths and
		// GuardianViolation are populated on the result by the executor /
		// guardian check and are mirrored here for the audit trail.
		ProfileName:       result.ProfileName,
		TransformedFrom:   result.TransformedFrom,
		TransformedTo:     decision.UpdatedToolName,
		TrashedPaths:      append([]string(nil), result.TrashedPaths...),
		GuardianViolation: result.GuardianViolation,
	})
}
