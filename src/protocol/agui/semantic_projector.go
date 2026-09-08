package agui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"awaken/src/agentcore"
	"awaken/src/instructions"
	"awaken/src/tool"
)

// ProjectSemanticRecords converts durable SemanticRecord values into AG-UI events.
// Live SSE and transcript replay must share this projector.
func ProjectSemanticRecords(records []agentcore.SemanticRecord) []Event {
	out := make([]Event, 0, len(records)*2)
	sentEstimates := make(map[string]int)
	for _, record := range records {
		if record.Kind == agentcore.SemanticKindModelRequest {
			requestID := stringFromAny(record.Payload["request_id"])
			estimate := intFromAny(record.Payload["estimated_input_tokens"])
			if requestID != "" && estimate > 0 {
				sentEstimates[requestID] = estimate
			}
		}
		frames := ProjectSemanticRecord(record)
		for index := range frames {
			usage := frames[index].ModelUsage
			if usage == nil || usage.SentEstimatedInputTokens != nil {
				continue
			}
			if estimate := sentEstimates[usage.RequestID]; estimate > 0 {
				usage.SentEstimatedInputTokens = &estimate
			}
		}
		out = append(out, frames...)
	}
	return out
}

// ProjectSemanticRecord projects one SemanticRecord into zero or more AG-UI frames.
func ProjectSemanticRecord(record agentcore.SemanticRecord) []Event {
	frames := projectSemanticRecord(record)
	for i := range frames {
		frames[i].Seq = FrameSeq(record.Seq, i)
		frames[i].SemanticSeq = frames[i].Seq
		frames[i].RecordID = record.RecordID
	}
	return frames
}

func projectSemanticRecord(record agentcore.SemanticRecord) []Event {
	base := Event{
		Timestamp: toUnixMillis(record.Timestamp),
		ThreadID:  record.ThreadID,
		RunID:     record.RunID,
		TurnID:    record.TurnID,
		Seq:       record.Seq,
	}
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	if stringFromAny(payload["hook_invocation_id"]) != "" {
		switch record.Kind {
		case agentcore.SemanticKindHookInvocationStarted, agentcore.SemanticKindHookInvocationCompleted, agentcore.SemanticKindModelUsage:
		default:
			return nil
		}
	}
	if taskID := stringFromAny(payload["subagent_task_id"]); taskID != "" {
		switch record.Kind {
		case agentcore.SemanticKindReasoningSummary,
			agentcore.SemanticKindAssistantMessage,
			agentcore.SemanticKindToolCall,
			agentcore.SemanticKindToolResult,
			agentcore.SemanticKindHookInvocationStarted,
			agentcore.SemanticKindHookInvocationCompleted,
			agentcore.SemanticKindTurnBoundary:
			childPayload := make(map[string]any, len(payload))
			for key, value := range payload {
				childPayload[key] = value
			}
			delete(childPayload, "subagent_task_id")
			delete(childPayload, "agent_id")
			delete(childPayload, "parent_agent_id")
			childRecord := record
			childRecord.Payload = childPayload
			frames := projectSemanticRecord(childRecord)
			for i := range frames {
				frames[i].SubagentTaskID = taskID
				frames[i].SubagentAgentID = stringFromAny(payload["agent_id"])
				frames[i].SubagentParentAgentID = stringFromAny(payload["parent_agent_id"])
			}
			return frames
		default:
			// Child lifecycle and host-owned task state are projected from the
			// manager records below, never as root run lifecycle frames.
			return nil
		}
	}

	switch record.Kind {
	case agentcore.SemanticKindRunStarted:
		base.Type = EventRunStarted
		return []Event{base}
	case agentcore.SemanticKindContextSnapshot:
		var snapshot agentcore.ContextUsageSnapshot
		if decodeSemanticPayload(payload, &snapshot) != nil {
			return nil
		}
		base.Type = EventAwakenContextSnapshot
		base.ContextUsageSnapshot = mapContextUsageSnapshot(&snapshot)
		return []Event{base}
	case agentcore.SemanticKindInstructionSnapshot:
		var snapshot instructions.Summary
		if decodeSemanticPayload(payload, &snapshot) != nil {
			return nil
		}
		base.Type = EventAwakenInstructionSnapshot
		base.InstructionSnapshot = mapInstructionSnapshot(snapshot)
		return []Event{base}
	case agentcore.SemanticKindModelRequest:
		// Request composition is durable for audit/recovery but the separate
		// context_snapshot record owns the renderer projection.
		return nil
	case agentcore.SemanticKindHookSnapshot:
		// Hook configuration receipts are host audit/configuration facts exposed
		// through /v1/hooks and the run DTO, not the narrative wire.
		return nil
	case agentcore.SemanticKindHookInvocationStarted:
		base.Type = EventAwakenHookStarted
		applyHookPayload(&base, payload)
		return []Event{base}
	case agentcore.SemanticKindHookInvocationCompleted:
		base.Type = EventAwakenHookCompleted
		applyHookPayload(&base, payload)
		return []Event{base}
	case agentcore.SemanticKindContextEpoch, agentcore.SemanticKindProviderOpaqueState:
		// These records are local durability authorities. Their payloads must not
		// enter AG-UI, renderer state, Memory or telemetry.
		return nil
	case agentcore.SemanticKindModelUsage:
		var usage agentcore.ModelUsageRecord
		if decodeSemanticPayload(payload, &usage) != nil {
			return nil
		}
		base.Type = EventAwakenModelUsage
		base.ModelUsage = mapModelUsageRecord(&usage)
		return []Event{base}
	case agentcore.SemanticKindSubagentSpawned,
		agentcore.SemanticKindSubagentStatusChanged,
		agentcore.SemanticKindSubagentTerminal:
		changed := base
		changed.Type = EventAwakenSubagentChanged
		applySubagentPayload(&changed, payload)
		return []Event{changed}
	case agentcore.SemanticKindSubagentProgress:
		activity := base
		activity.Type = EventAwakenSubagentActivity
		applySubagentPayload(&activity, payload)
		return []Event{activity}
	case agentcore.SemanticKindSubagentMessageQueued:
		message := base
		message.Type = EventAwakenSubagentMessageQueued
		applySubagentPayload(&message, payload)
		return []Event{message}
	case agentcore.SemanticKindMemoryRecalled:
		memoryEvent := base
		memoryEvent.Type = EventAwakenMemoryRecalled
		raw, err := json.Marshal(payload["memories"])
		if err == nil {
			var receipts []agentcore.MemoryRecallReceipt
			if json.Unmarshal(raw, &receipts) == nil {
				for _, receipt := range receipts {
					memoryEvent.Memories = append(memoryEvent.Memories, MemoryReceipt{MemoryID: receipt.MemoryID, Scope: receipt.Scope, Type: receipt.Type, Revision: receipt.Revision, Bytes: receipt.Bytes})
				}
			}
		}
		return []Event{memoryEvent}
	case agentcore.SemanticKindMemoryMutated:
		memoryEvent := base
		raw, err := json.Marshal(payload["memory"])
		if err == nil {
			var receipt agentcore.MemoryRecallReceipt
			if json.Unmarshal(raw, &receipt) == nil {
				memoryEvent.Memories = mapMemoryReceipts([]agentcore.MemoryRecallReceipt{receipt})
			}
		}
		memoryEvent.Type = memoryMutationEventType(memoryEvent.Memories)
		return []Event{memoryEvent}
	case agentcore.SemanticKindUserMessage:
		// User prompts are restored via run metadata; no AG-UI frame required.
		return nil
	case agentcore.SemanticKindReasoningSummary:
		messageID := stringFromAny(payload["message_id"])
		if messageID == "" {
			messageID = record.RecordID
		}
		content := stringFromAny(payload["content"])
		if strings.TrimSpace(content) == "" {
			return nil
		}
		start := base
		start.Type = EventReasoningStart
		start.MessageID = messageID
		messageStart := base
		messageStart.Type = EventReasoningMessageStart
		messageStart.MessageID = messageID
		messageStart.Role = "reasoning"
		delta := base
		delta.Type = EventReasoningMessageContent
		delta.MessageID = messageID
		delta.Role = "reasoning"
		delta.Delta = content
		messageEnd := base
		messageEnd.Type = EventReasoningMessageEnd
		messageEnd.MessageID = messageID
		messageEnd.Role = "reasoning"
		reasoningEnd := base
		reasoningEnd.Type = EventReasoningEnd
		reasoningEnd.MessageID = messageID
		return []Event{start, messageStart, delta, messageEnd, reasoningEnd}
	case agentcore.SemanticKindAssistantMessage:
		messageID := stringFromAny(payload["message_id"])
		if messageID == "" {
			messageID = record.RecordID
		}
		content := stringFromAny(payload["content"])
		if strings.TrimSpace(content) == "" {
			return nil
		}
		start := base
		start.Type = EventTextMessageStart
		start.MessageID = messageID
		start.Role = "assistant"
		delta := base
		delta.Type = EventTextMessageContent
		delta.MessageID = messageID
		delta.Role = "assistant"
		delta.Delta = content
		end := base
		end.Type = EventTextMessageEnd
		end.MessageID = messageID
		end.Role = "assistant"
		return []Event{start, delta, end}
	case agentcore.SemanticKindToolCall:
		toolCallID := stringFromAny(payload["tool_call_id"])
		toolName := stringFromAny(payload["tool_name"])
		args := stringFromAny(payload["arguments"])
		parent := record.ParentID
		start := base
		start.Type = EventToolCallStart
		start.ToolCallID = toolCallID
		start.ToolCallName = toolName
		start.ParentMessageID = parent
		frames := []Event{start}
		if args != "" {
			argsEvent := base
			argsEvent.Type = EventToolCallArgs
			argsEvent.ToolCallID = toolCallID
			argsEvent.ToolCallName = toolName
			argsEvent.Delta = args
			frames = append(frames, argsEvent)
		}
		end := base
		end.Type = EventToolCallEnd
		end.ToolCallID = toolCallID
		end.ToolCallName = toolName
		frames = append(frames, end)
		return frames
	case agentcore.SemanticKindToolResult:
		result := base
		result.Type = EventToolCallResult
		result.ToolCallID = stringFromAny(payload["tool_call_id"])
		result.ToolCallName = stringFromAny(payload["tool_name"])
		result.Content = stringFromAny(payload["content"])
		result.Role = "tool"
		result.TrashedPaths = stringSliceFromAny(payload["trashed_paths"])
		result.AffectedPaths = stringSliceFromAny(payload["affected_paths"])
		result.PatchSummary = stringFromAny(payload["patch_summary"])
		applyOutcomePayload(&result, payload)
		applyExecutionMetadata(&result, payload)
		return []Event{result}
	case agentcore.SemanticKindArtifact:
		artifact := base
		if boolFromAny(payload["created"]) {
			artifact.Type = EventAwakenArtifactCreated
		} else {
			artifact.Type = EventAwakenArtifactUpdated
		}
		artifact.ToolCallID = stringFromAny(payload["tool_call_id"])
		artifact.ToolCallName = stringFromAny(payload["tool_name"])
		artifact.ArtifactContractVersion = tool.ArtifactContractVersionOrLegacy(intFromAny(payload["contract_version"]))
		artifact.ArtifactProducer = firstNonEmpty(stringFromAny(payload["producer"]), artifact.ToolCallName)
		artifact.ArtifactID = stringFromAny(payload["artifact_id"])
		artifact.ArtifactGroupID = stringFromAny(payload["group_id"])
		artifact.ArtifactGroupIndex = intFromAny(payload["group_index"])
		artifact.ArtifactTitle = stringFromAny(payload["title"])
		artifact.ArtifactDescription = stringFromAny(payload["description"])
		artifact.ArtifactKind = stringFromAny(payload["kind"])
		artifact.ArtifactPath = stringFromAny(payload["path"])
		artifact.ArtifactContent = stringFromAny(payload["content"])
		artifact.ArtifactStatus = stringFromAny(payload["status"])
		artifact.ArtifactRole = string(tool.ResolveArtifactRole(
			tool.ArtifactRole(stringFromAny(payload["role"])),
			artifact.ArtifactProducer,
			artifact.ArtifactKind,
			artifact.ArtifactPath,
		))
		return []Event{artifact}
	case agentcore.SemanticKindTurnDiff:
		diff := base
		diff.Type = EventAwakenTurnDiff
		diff.Diff = stringFromAny(payload["diff"])
		diff.Truncated = boolFromAny(payload["truncated"])
		return []Event{diff}
	case agentcore.SemanticKindTurnBoundary:
		boundary := base
		boundary.Type = EventAwakenTurnFinished
		boundary.TerminalReason = stringFromAny(payload["terminal_reason"])
		boundary.TransitionReason = stringFromAny(payload["transition_reason"])
		return []Event{boundary}
	case agentcore.SemanticKindRunTerminal:
		status := stringFromAny(payload["status"])
		switch status {
		case "finished":
			finished := base
			finished.Type = EventRunFinished
			finished.Result = stringFromAny(payload["result"])
			finished.TerminalReason = stringFromAny(payload["terminal_reason"])
			finished.Outcome = &RunOutcome{Type: "success"}
			return []Event{finished}
		case "cancelled":
			cancelled := base
			cancelled.Type = EventRunError
			cancelled.Message = firstNonEmpty(stringFromAny(payload["error"]), "run cancelled")
			cancelled.Code = firstNonEmpty(stringFromAny(payload["code"]), "RUN_CANCELLED")
			cancelled.TerminalReason = stringFromAny(payload["terminal_reason"])
			return []Event{cancelled}
		case "interrupted":
			interrupted := base
			interrupted.Type = EventRunError
			interrupted.Message = firstNonEmpty(stringFromAny(payload["error"]), "run interrupted")
			interrupted.Code = firstNonEmpty(stringFromAny(payload["code"]), "PROCESS_RESTARTED")
			interrupted.TerminalReason = firstNonEmpty(stringFromAny(payload["terminal_reason"]), stringFromAny(payload["reason"]))
			return []Event{interrupted}
		default:
			errored := base
			errored.Type = EventRunError
			errored.Message = firstNonEmpty(stringFromAny(payload["error"]), "run failed")
			errored.Code = stringFromAny(payload["code"])
			errored.TerminalReason = stringFromAny(payload["terminal_reason"])
			return []Event{errored}
		}
	case agentcore.SemanticKindPermissionDecision:
		decision := base
		decision.Type = EventAwakenToolPermissionDecided
		decision.ToolCallID = stringFromAny(payload["tool_call_id"])
		decision.ToolCallName = stringFromAny(payload["tool_name"])
		decision.ToolDecisionAction = stringFromAny(payload["action"])
		decision.ToolDecisionSource = stringFromAny(payload["source"])
		decision.ToolDecisionReason = stringFromAny(payload["reason"])
		decision.StaticPermissionBehavior = stringFromAny(payload["behavior"])
		decision.StaticPermissionReason = stringFromAny(payload["static_reason"])
		decision.AffectedPaths = stringSliceFromAny(payload["affected_paths"])
		decision.PatchSummary = stringFromAny(payload["patch_summary"])
		decision.ProfileName = stringFromAny(payload["profile_name"])
		decision.TransformedFrom = stringFromAny(payload["transformed_from"])
		decision.TransformedTo = stringFromAny(payload["transformed_to"])
		decision.TrashedPaths = stringSliceFromAny(payload["trashed_paths"])
		decision.GuardianViolation = stringFromAny(payload["guardian_violation"])
		return []Event{decision}
	case agentcore.SemanticKindPersistenceWarning:
		warning := base
		warning.Type = EventAwakenUnknown
		warning.RawType = "persistence_warning"
		warning.Message = stringFromAny(payload["error"])
		return []Event{warning}
	case agentcore.SemanticKindRuntimeNotice:
		notice := base
		eventType := agentcore.AwakenEventType(stringFromAny(payload["event_type"]))
		switch eventType {
		case agentcore.RuntimePrepareEventType, agentcore.RuntimePrepareStartedEventType:
			notice.Type = EventAwakenRuntimePrepare
		case agentcore.RuntimePrepareProgressEventType:
			notice.Type = EventAwakenRuntimePrepareProgress
		case agentcore.RuntimeReadyEventType:
			notice.Type = EventAwakenRuntimeReady
		case agentcore.RuntimeRepairFailedEventType:
			notice.Type = EventAwakenRuntimeRepairFailed
		case agentcore.RuntimeMissingEventType:
			notice.Type = EventAwakenRuntimeMissing
		default:
			notice.Type = EventAwakenUnknown
			notice.RawType = string(eventType)
		}
		notice.Message = stringFromAny(payload["content"])
		notice.Progress = stringFromAny(payload["progress"])
		notice.ToolCallID = stringFromAny(payload["tool_call_id"])
		notice.ToolCallName = stringFromAny(payload["tool_name"])
		notice.RuntimeName = stringFromAny(payload["runtime_name"])
		notice.RuntimeVersion = stringFromAny(payload["runtime_version"])
		return []Event{notice}
	case agentcore.SemanticKindScheduledNotice:
		// Host-owned fixed reminder (contract [固定提醒](docs/12_定时任务/定时执行与提醒/实现方案.md#固定提醒)/[HTTP、IPC 与事件](docs/12_定时任务/定时执行与提醒/实现方案.md#httpipc-与事件)): one replayable frame
		// carrying only task/execution identity, the task name and the fixed
		// content. Missing payload fields degrade to empty strings; the frame
		// is still emitted so live and replay stay in lockstep.
		notice := base
		notice.Type = EventAwakenScheduledNotice
		notice.TaskID = stringFromAny(payload["task_id"])
		notice.ExecutionID = stringFromAny(payload["execution_id"])
		notice.TaskName = stringFromAny(payload["task_name"])
		notice.NoticeContent = stringFromAny(payload["content"])
		return []Event{notice}
	case agentcore.SemanticKindPlanUpdated:
		// Durable run-plan full snapshot (contract [事件合同](docs/02_主智能体闭环/运行计划/实现方案.md#事件合同)): one replayable frame
		// carrying the sanitized snapshot and derived counters. Missing payload
		// fields degrade to zero values; the frame is still emitted so live and
		// replay stay in lockstep. Version ordering/dedup is the Desktop
		// reducer's job; the projector only projects faithfully.
		plan := base
		plan.Type = EventAwakenPlanUpdated
		plan.PlanID = stringFromAny(payload["plan_id"])
		plan.PlanVersion = uint64(int64FromAny(payload["version"]))
		plan.PlanExplanation = stringFromAny(payload["explanation"])
		plan.PlanCompletedCount = intFromAny(payload["completed_count"])
		plan.PlanTotalCount = intFromAny(payload["total_count"])
		plan.PlanCurrentStep = intFromAny(payload["current_step"])
		plan.PlanAllCompleted = boolFromAny(payload["all_completed"])
		plan.ToolCallID = stringFromAny(payload["tool_call_id"])
		raw, err := json.Marshal(payload["items"])
		if err == nil {
			var items []struct {
				Step       string `json:"step"`
				ActiveForm string `json:"active_form"`
				Status     string `json:"status"`
			}
			if json.Unmarshal(raw, &items) == nil {
				for _, item := range items {
					plan.PlanItems = append(plan.PlanItems, RunPlanItemReceipt{Step: item.Step, ActiveForm: item.ActiveForm, Status: item.Status})
				}
			}
		}
		return []Event{plan}
	case agentcore.SemanticKindResearchAccepted,
		agentcore.SemanticKindResearchPlan,
		agentcore.SemanticKindResearchClaimBatch,
		agentcore.SemanticKindResearchVerification,
		agentcore.SemanticKindResearchReport,
		agentcore.SemanticKindResearchTerminal:
		phase := base
		phase.Type = EventAwakenResearchPhase
		phase.ResearchPhase = firstNonEmpty(stringFromAny(payload["phase"]), researchPhaseFromKind(record.Kind))
		phase.ResearchRevision = intFromAny(payload["revision"])
		phase.ResearchQuestionCount = intFromAny(payload["question_count"])
		phase.ResearchClaimCount = intFromAny(payload["claim_count"])
		phase.ResearchVerifiedCount = intFromAny(payload["verified_count"])
		phase.ResearchPartialCount = intFromAny(payload["partial_count"])
		phase.ResearchRejectedCount = intFromAny(payload["rejected_count"])
		return []Event{phase}
	case agentcore.SemanticKindRecoveryDecision:
		recovery := base
		if boolFromAny(payload["blocked"]) {
			recovery.Type = EventAwakenToolRetryBlocked
		} else {
			recovery.Type = EventAwakenToolRecoveryRequired
		}
		recovery.ToolCallID = stringFromAny(payload["tool_call_id"])
		recovery.ToolCallName = stringFromAny(payload["tool_name"])
		recovery.Code = stringFromAny(payload["error_code"])
		recovery.ErrorCode = stringFromAny(payload["error_code"])
		recovery.Message = stringFromAny(payload["message"])
		if recovery.Message == "" {
			if boolFromAny(payload["blocked"]) {
				recovery.Message = "tool retry blocked"
			} else {
				recovery.Message = "tool recovery required"
			}
		}
		recovery.RecoveryOptions = stringSliceFromAny(payload["recovery_options"])
		recovery.ToolStatus = stringFromAny(payload["tool_status"])
		recovery.TransportStatus = stringFromAny(payload["transport_status"])
		recovery.SemanticStatus = stringFromAny(payload["semantic_status"])
		recovery.ProcessStatus = stringFromAny(payload["process_status"])
		recovery.ErrorCategory = stringFromAny(payload["error_category"])
		recovery.Retryable = boolFromAny(payload["retryable"])
		applyExecutionMetadata(&recovery, payload)
		return []Event{recovery}
	case agentcore.SemanticKindSkillActivated:
		invoked := skillEventFromPayload(base, payload)
		invoked.Type = EventAwakenSkillInvoked
		frames := []Event{invoked}
		if invoked.SkillStatus == "" || invoked.SkillStatus == "ok" {
			executionContext := invoked
			executionContext.Type = EventAwakenSkillExecutionContext
			frames = append(frames, executionContext)
		} else {
			failed := invoked
			failed.Type = EventAwakenSkillFailedToLoad
			frames = append(frames, failed)
		}
		return frames
	case agentcore.SemanticKindResourceReceipt:
		loaded := skillEventFromPayload(base, payload)
		loaded.Type = EventAwakenSkillContentLoaded
		loaded.SkillResource = stringFromAny(payload["resource"])
		loaded.SkillResourceSource = stringFromAny(payload["source"])
		loaded.SkillResourceSHA256 = stringFromAny(payload["sha256"])
		loaded.SkillResourceSize = int64FromAny(payload["size"])
		loaded.SkillResourceMode = stringFromAny(payload["mode"])
		return []Event{loaded}
	case agentcore.SemanticKindComputerReviewRequested,
		agentcore.SemanticKindComputerReviewResolved,
		agentcore.SemanticKindComputerSessionOpened,
		agentcore.SemanticKindComputerStateChanged,
		agentcore.SemanticKindComputerVerificationCompleted,
		agentcore.SemanticKindComputerControlTransferStarted,
		agentcore.SemanticKindComputerControlTransferCompleted,
		agentcore.SemanticKindComputerSessionClosed,
		agentcore.SemanticKindComputerActionStarted:
		return projectComputerEvent(base, record.Kind, payload)
	case agentcore.SemanticKindThread,
		agentcore.SemanticKindThreadRewound,
		agentcore.SemanticKindFileChange,
		// Progressive-disclosure activation is host-internal bookkeeping:
		// the tool_find tool call/result already carries the user-visible
		// story, and the loaded set only shapes the next request projection.
		// The legacy MCP-only kind stays suppressed for replayed journals.
		agentcore.SemanticKindDeferredToolsActivated,
		agentcore.SemanticKindMCPToolsActivated:
		return nil
	default:
		unknown := base
		unknown.Type = EventAwakenUnknown
		unknown.RawType = string(record.Kind)
		return []Event{unknown}
	}
}

func mapInstructionSnapshot(snapshot instructions.Summary) *InstructionSnapshotSummary {
	result := &InstructionSnapshotSummary{
		SnapshotID: snapshot.SnapshotID, CapturedAt: snapshot.CapturedAt.UTC().Format(time.RFC3339Nano), Partial: snapshot.Partial,
		Sources:     make([]InstructionSourceSummary, 0, len(snapshot.Sources)),
		Diagnostics: make([]InstructionDiagnostic, 0, len(snapshot.Diagnostics)),
	}
	for _, source := range snapshot.Sources {
		result.Sources = append(result.Sources, InstructionSourceSummary{
			Scope: string(source.Scope), DisplayPath: source.DisplayPath, ScopeRoot: source.ScopeRoot,
			Digest: source.Digest, OriginalBytes: source.OriginalBytes, AppliedBytes: source.AppliedBytes,
			Truncated: source.Truncated, Status: string(source.Status),
		})
	}
	for _, diagnostic := range snapshot.Diagnostics {
		result.Diagnostics = append(result.Diagnostics, InstructionDiagnostic{
			Code: diagnostic.Code, Message: diagnostic.Message, Scope: string(diagnostic.Scope), DisplayPath: diagnostic.DisplayPath,
		})
	}
	return result
}

func applyHookPayload(event *Event, payload map[string]any) {
	event.HookInvocationID = stringFromAny(payload["invocation_id"])
	event.HookID = stringFromAny(payload["hook_id"])
	event.HookEvent = stringFromAny(payload["event"])
	event.HookSourceKind = stringFromAny(payload["source_kind"])
	event.Status = stringFromAny(payload["status"])
	event.HookAction = stringFromAny(payload["action"])
	event.HookReasonCode = stringFromAny(payload["reason_code"])
	event.HookReason = stringFromAny(payload["reason"])
	event.HookOutputPreview = stringFromAny(payload["output_preview"])
	event.DurationMs = int64(intFromAny(payload["duration_ms"]))
}

func projectComputerEvent(base Event, kind agentcore.SemanticKind, payload map[string]any) []Event {
	event := base
	switch kind {
	case agentcore.SemanticKindComputerReviewRequested:
		event.Type = EventAwakenComputerReviewRequested
	case agentcore.SemanticKindComputerReviewResolved:
		event.Type = EventAwakenComputerReviewResolved
	case agentcore.SemanticKindComputerSessionOpened:
		event.Type = EventAwakenComputerSessionChanged
		event.ComputerSessionState = "ready"
	case agentcore.SemanticKindComputerStateChanged:
		event.Type = EventAwakenComputerSessionChanged
		event.ComputerSessionState = stringFromAny(payload["to_state"])
	case agentcore.SemanticKindComputerVerificationCompleted:
		event.Type = EventAwakenComputerVerification
		event.ComputerVerificationStatus = stringFromAny(payload["status"])
	case agentcore.SemanticKindComputerControlTransferStarted:
		event.Type = EventAwakenComputerHandoffStarted
		event.ComputerSessionState = "taken_over"
	case agentcore.SemanticKindComputerControlTransferCompleted:
		event.Type = EventAwakenComputerHandoffEnded
		event.ComputerSessionState = "ready"
	case agentcore.SemanticKindComputerSessionClosed:
		event.Type = EventAwakenComputerSessionChanged
		event.ComputerSessionState = "closed"
	case agentcore.SemanticKindComputerActionStarted:
		event.Type = EventAwakenComputerPointerHint
	}
	event.ComputerReviewID = stringFromAny(payload["review_id"])
	event.ComputerActionID = stringFromAny(payload["action_id"])
	event.ComputerActionDigest = stringFromAny(payload["action_digest"])
	event.ComputerTargetSummary = stringFromAny(payload["target_summary"])
	event.ComputerEffectSummary = stringFromAny(payload["effect_summary"])
	event.ComputerRisk = stringFromAny(payload["risk"])
	event.ComputerSessionID = stringFromAny(payload["session_id"])
	event.ComputerObservationID = stringFromAny(payload["observation_id"])
	event.ComputerDecision = stringFromAny(payload["decision"])
	event.ComputerActor = stringFromAny(payload["actor"])
	event.ComputerEvidenceRefs = stringSliceFromAny(payload["evidence_refs"])
	event.ComputerPointerKind = firstNonEmpty(stringFromAny(payload["pointer_kind"]), stringFromAny(payload["action"]))
	event.ComputerPointerSpace = stringFromAny(payload["pointer_space"])
	event.ComputerPointerX = optionalIntPointer(payload, "pointer_x")
	event.ComputerPointerY = optionalIntPointer(payload, "pointer_y")
	event.ComputerPointerToX = optionalIntPointer(payload, "pointer_to_x")
	event.ComputerPointerToY = optionalIntPointer(payload, "pointer_to_y")
	switch kind {
	case agentcore.SemanticKindComputerControlTransferStarted:
		event.ComputerHandoff = true
	case agentcore.SemanticKindComputerControlTransferCompleted:
		event.ComputerHandoff = false
	}
	return []Event{event}
}

func applySubagentPayload(dst *Event, payload map[string]any) {
	if dst == nil {
		return
	}
	dst.SubagentTaskID = firstNonEmpty(stringFromAny(payload["task_id"]), stringFromAny(payload["subagent_task_id"]))
	dst.SubagentAgentID = firstNonEmpty(stringFromAny(payload["agent_id"]), stringFromAny(payload["subagent_agent_id"]))
	dst.SubagentParentAgentID = stringFromAny(payload["parent_agent_id"])
	dst.SubagentParentToolCallID = stringFromAny(payload["parent_tool_call_id"])
	dst.SubagentAgentType = stringFromAny(payload["agent_type"])
	dst.SubagentDescription = stringFromAny(payload["description"])
	dst.SubagentStatus = stringFromAny(payload["status"])
	dst.SubagentWorkspaceMode = stringFromAny(payload["workspace_mode"])
	dst.SubagentModelProfileID = stringFromAny(payload["model_profile_id"])
	dst.SubagentActivity = stringFromAny(payload["current_activity"])
	dst.SubagentToolUseCount = intFromAny(payload["tool_use_count"])
	dst.SubagentEstimatedTokens = intFromAny(payload["estimated_tokens"])
	dst.SubagentInputTokens = intFromAny(payload["input_tokens"])
	dst.SubagentOutputTokens = intFromAny(payload["output_tokens"])
	dst.SubagentTotalTokens = intFromAny(payload["total_tokens"])
	dst.SubagentResultPreview = stringFromAny(payload["result_preview"])
	dst.SubagentErrorCode = stringFromAny(payload["error_code"])
	dst.SubagentErrorMessage = stringFromAny(payload["error_message"])
	dst.SubagentSideEffectState = stringFromAny(payload["side_effect_state"])
	dst.SubagentCreatedAt = stringFromAny(payload["created_at"])
	dst.SubagentStartedAt = stringFromAny(payload["started_at"])
	dst.SubagentFinishedAt = stringFromAny(payload["finished_at"])
}

func decodeSemanticPayload(payload map[string]any, target any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func applyOutcomePayload(dst *Event, payload map[string]any) {
	outcomeRaw, ok := payload["outcome"]
	if !ok || outcomeRaw == nil {
		if boolFromAny(payload["is_error"]) {
			dst.ToolStatus = "failed"
		}
		return
	}
	raw, err := json.Marshal(outcomeRaw)
	if err != nil {
		return
	}
	var outcome struct {
		Status          string         `json:"status"`
		TransportStatus string         `json:"transport_status"`
		SemanticStatus  string         `json:"semantic_status"`
		ProcessStatus   string         `json:"process_status"`
		ExitCode        *int           `json:"exit_code"`
		TimedOut        bool           `json:"timed_out"`
		ErrorCode       string         `json:"error_code"`
		ErrorCategory   string         `json:"error_category"`
		Retryable       bool           `json:"retryable"`
		RecoveryOptions []string       `json:"recovery_options"`
		DurationMs      int64          `json:"duration_ms"`
		Diagnostic      map[string]any `json:"diagnostic"`
	}
	if err := json.Unmarshal(raw, &outcome); err != nil {
		return
	}
	dst.ToolStatus = outcome.Status
	dst.TransportStatus = outcome.TransportStatus
	dst.SemanticStatus = outcome.SemanticStatus
	dst.ProcessStatus = outcome.ProcessStatus
	dst.ProcessExitCode = outcome.ExitCode
	dst.ProcessTimedOut = outcome.TimedOut
	dst.ErrorCode = outcome.ErrorCode
	dst.ErrorCategory = outcome.ErrorCategory
	dst.Retryable = outcome.Retryable
	dst.RecoveryOptions = append([]string(nil), outcome.RecoveryOptions...)
	dst.DurationMs = outcome.DurationMs
	if len(outcome.Diagnostic) > 0 {
		dst.Diagnostic = make(map[string]any, len(outcome.Diagnostic))
		for key, value := range outcome.Diagnostic {
			dst.Diagnostic[key] = value
		}
	}
	var outcomePayload map[string]any
	if err := json.Unmarshal(raw, &outcomePayload); err == nil {
		applyExecutionMetadata(dst, outcomePayload)
	}
	applyExecutionMetadata(dst, outcome.Diagnostic)
}

func skillEventFromPayload(base Event, payload map[string]any) Event {
	event := base
	event.SkillID = stringFromAny(payload["skill_id"])
	event.SkillVersion = stringFromAny(payload["skill_version"])
	event.SkillSource = firstNonEmpty(stringFromAny(payload["skill_source"]), stringFromAny(payload["source"]))
	event.SkillPath = firstNonEmpty(
		stringFromAny(payload["skill_path"]),
		stringFromAny(payload["skill_file_path"]),
		stringFromAny(payload["path"]),
	)
	event.SkillStatus = firstNonEmpty(stringFromAny(payload["skill_status"]), stringFromAny(payload["compatibility_status"]), stringFromAny(payload["status"]))
	event.SkillEffectiveTools = stringSliceFromAny(payload["effective_tools"])
	event.SkillDiagnostics = diagnosticStringsFromAny(firstPresentValue(payload, "diagnostics", "compatibility_errors"))
	return event
}

func diagnosticStringsFromAny(value any) []string {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var values []any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				out = append(out, trimmed)
			}
		case map[string]any:
			code := strings.TrimSpace(stringFromAny(typed["code"]))
			message := strings.TrimSpace(stringFromAny(typed["message"]))
			switch {
			case code != "" && message != "":
				out = append(out, code+": "+message)
			case message != "":
				out = append(out, message)
			case code != "":
				out = append(out, code)
			}
		}
	}
	return out
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return ""
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func boolPointerFromAny(value any) *bool {
	valueBool, ok := value.(bool)
	if !ok {
		return nil
	}
	return &valueBool
}

func optionalIntPointer(payload map[string]any, key string) *int {
	if payload == nil {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		number := intFromAny(value)
		return &number
	default:
		return nil
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return int(value)
		}
	default:
		return 0
	}
	return 0
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		return int64(typed)
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		value, err := typed.Int64()
		if err == nil {
			return value
		}
	}
	return 0
}

func stringSliceFromAny(value any) []string {
	var values []any
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values = typed
	default:
		raw, err := json.Marshal(value)
		if err != nil || json.Unmarshal(raw, &values) != nil {
			return nil
		}
	}
	out := make([]string, 0, len(values))
	for _, item := range values {
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" && s != "<nil>" {
			out = append(out, s)
		}
	}
	return out
}

// SemanticRecordsToAwakenEvents is used by runtime transcript packaging that still
// wraps agent events for compatibility with existing ThreadTranscriptRun shapes.
func SemanticRecordsToAwakenEvents(records []agentcore.SemanticRecord) []agentcore.AwakenEvent {
	frames := ProjectSemanticRecords(records)
	out := make([]agentcore.AwakenEvent, 0, len(frames))
	for _, frame := range frames {
		out = append(out, awakenEventFromAGUI(frame))
	}
	return out
}

func awakenEventFromAGUI(frame Event) agentcore.AwakenEvent {
	event := agentcore.AwakenEvent{
		Timestamp:               time.UnixMilli(frame.Timestamp).UTC(),
		ThreadID:                frame.ThreadID,
		RunID:                   frame.RunID,
		TurnID:                  frame.TurnID,
		MessageID:               frame.MessageID,
		ParentMessageID:         frame.ParentMessageID,
		Role:                    frame.Role,
		Delta:                   frame.Delta,
		Content:                 firstNonEmpty(frame.Content, frame.Result, frame.Message),
		ToolCallID:              frame.ToolCallID,
		ToolName:                frame.ToolCallName,
		Arguments:               frame.Delta,
		Diff:                    frame.Diff,
		Truncated:               frame.Truncated,
		Error:                   frame.Message,
		Code:                    frame.Code,
		TerminalReason:          agentcore.TerminalReason(frame.TerminalReason),
		ToolStatus:              frame.ToolStatus,
		TransportStatus:         frame.TransportStatus,
		SemanticStatus:          frame.SemanticStatus,
		ProcessStatus:           frame.ProcessStatus,
		ProcessExitCode:         frame.ProcessExitCode,
		ProcessTimedOut:         frame.ProcessTimedOut,
		ErrorCode:               frame.ErrorCode,
		ErrorCategory:           frame.ErrorCategory,
		Retryable:               frame.Retryable,
		RecoveryOptions:         append([]string(nil), frame.RecoveryOptions...),
		DurationMs:              frame.DurationMs,
		ArtifactContractVersion: frame.ArtifactContractVersion,
		ArtifactRole:            tool.ArtifactRole(frame.ArtifactRole),
		ArtifactProducer:        frame.ArtifactProducer,
		ArtifactID:              frame.ArtifactID,
		ArtifactGroupID:         frame.ArtifactGroupID,
		ArtifactGroupIndex:      frame.ArtifactGroupIndex,
		ArtifactTitle:           frame.ArtifactTitle,
		ArtifactDescription:     frame.ArtifactDescription,
		ArtifactKind:            frame.ArtifactKind,
		ArtifactPath:            frame.ArtifactPath,
		ArtifactContent:         frame.ArtifactContent,
		ArtifactStatus:          agentcore.ArtifactStatus(frame.ArtifactStatus),
	}
	if frame.ContextUsageSnapshot != nil {
		snapshot := contextUsageSnapshotFromWire(frame.ContextUsageSnapshot)
		event.ContextUsageSnapshot = &snapshot
	}
	if frame.ModelUsage != nil {
		usage := modelUsageRecordFromWire(frame.ModelUsage)
		event.ModelUsage = &usage
	}
	switch frame.Type {
	case EventRunStarted:
		event.Type = agentcore.RunStartedEventType
	case EventRunFinished:
		event.Type = agentcore.RunFinishedEventType
		event.Content = frame.Result
	case EventRunError:
		event.Type = agentcore.RunErrorEventType
	case EventTextMessageStart:
		event.Type = agentcore.TextMessageStartedEventType
	case EventTextMessageContent:
		event.Type = agentcore.TextDeltaEventType
	case EventTextMessageEnd:
		event.Type = agentcore.TextMessageFinishedEventType
	case EventToolCallStart:
		event.Type = agentcore.ToolCallStartedEventType
	case EventToolCallArgs:
		event.Type = agentcore.ToolCallArgsEventType
		event.Arguments = frame.Delta
	case EventToolCallEnd:
		event.Type = agentcore.ToolCallFinishedEventType
	case EventToolCallResult:
		event.Type = agentcore.ToolResultEventType
	case EventAwakenArtifactCreated:
		event.Type = agentcore.ArtifactCreatedEventType
	case EventAwakenArtifactUpdated:
		event.Type = agentcore.ArtifactUpdatedEventType
	case EventAwakenTurnDiff:
		event.Type = agentcore.TurnDiffEventType
	case EventAwakenTurnFinished:
		event.Type = agentcore.TurnFinishedEventType
	case EventAwakenToolPermissionDecided:
		event.Type = agentcore.ToolPermissionDecidedEventType
	case EventAwakenContextSnapshot:
		event.Type = agentcore.ContextSnapshotEventType
	case EventAwakenModelUsage:
		event.Type = agentcore.ModelUsageEventType
	default:
		event.Type = agentcore.AwakenEventType(frame.RawType)
	}
	return event
}

func contextUsageSnapshotFromWire(value *ContextUsageSnapshot) agentcore.ContextUsageSnapshot {
	snapshot := agentcore.ContextUsageSnapshot{
		SchemaVersion: value.SchemaVersion, SnapshotID: value.SnapshotID, EstimatorID: value.EstimatorID,
		Basis: value.Basis, RequestID: value.RequestID, ModelID: value.ModelID,
		CapabilityModelID: value.CapabilityModelID, EndpointSelector: value.EndpointSelector,
		ModelSelectorKind: value.ModelSelectorKind, ProtocolFamily: value.ProtocolFamily,
		ContextEpochID: value.ContextEpochID, ContextEpochBoundaryReason: value.ContextEpochBoundaryReason,
		RequestShapeDigest: value.RequestShapeDigest, ModelWindowTokens: value.ModelWindowTokens,
		OutputReserveTokens: value.OutputReserveTokens, EffectiveInputWindowTokens: value.EffectiveInputWindowTokens,
		EstimatedInputTokens: value.EstimatedInputTokens, WarningThresholdTokens: value.WarningThresholdTokens,
		AutoCompactThresholdTokens: value.AutoCompactThresholdTokens, CompactTargetTokens: value.CompactTargetTokens,
	}
	snapshot.GeneratedAt, _ = time.Parse(time.RFC3339Nano, value.GeneratedAt)
	for _, contribution := range value.Contributions {
		snapshot.Contributions = append(snapshot.Contributions, agentcore.ContextUsageContribution{
			Kind: agentcore.ContextContributionKind(contribution.Kind), EstimatedTokens: contribution.EstimatedTokens,
			SourceCount: contribution.SourceCount,
		})
	}
	return snapshot
}

func modelUsageRecordFromWire(value *ModelUsageRecord) agentcore.ModelUsageRecord {
	usage := agentcore.ModelUsageRecord{
		SchemaVersion: value.SchemaVersion, RequestID: value.RequestID, Purpose: value.Purpose,
		ProviderID: value.ProviderID, ProfileID: value.ProfileID, CredentialRevision: value.CredentialRevision,
		ModelID: value.ModelID, CapabilityModelID: value.CapabilityModelID, EndpointSelector: value.EndpointSelector,
		ModelSelectorKind: value.ModelSelectorKind, ProtocolFamily: value.ProtocolFamily, EndpointContractID: value.EndpointContractID,
		ReasoningPolicy: value.ReasoningPolicy,
		CacheMode:       value.CacheMode, CacheKeyApplied: value.CacheKeyApplied,
		ContextEpochID: value.ContextEpochID, PrefixDigest: value.PrefixDigest, InvalidationReason: value.InvalidationReason,
		RetryIndex: value.RetryIndex, AuxiliaryFallback: value.AuxiliaryFallback,
		SentEstimatedInputTokens: cloneIntPointer(value.SentEstimatedInputTokens),
		InputTokens:              cloneIntPointer(value.InputTokens), CachedInputTokens: cloneIntPointer(value.CachedInputTokens),
		UncachedInputTokens: cloneIntPointer(value.UncachedInputTokens), CacheWriteTokens: cloneIntPointer(value.CacheWriteTokens),
		OutputTokens: cloneIntPointer(value.OutputTokens), ReasoningTokens: cloneIntPointer(value.ReasoningTokens),
		TotalTokens:   cloneIntPointer(value.TotalTokens),
		RawUsage:      rawModelUsageRecordFromWire(value.RawUsage),
		UsageComplete: value.UsageComplete, CalculatedCostUSD: cloneFloatPointer(value.CalculatedCostUSD),
		PricingSnapshot: pricingSnapshotFromWire(value.PricingSnapshot), Currency: value.Currency,
		ProviderRequestID: value.ProviderRequestID, RateLimitObservation: rateLimitObservationFromWire(value.RateLimitObservation), LatencyMS: value.LatencyMS,
		Status: agentcore.ModelUsageStatus(value.Status), ErrorCode: value.ErrorCode,
	}
	usage.CompletedAt, _ = time.Parse(time.RFC3339Nano, value.CompletedAt)
	return usage
}

func rateLimitObservationFromWire(value *RateLimitObservation) *agentcore.RateLimitObservation {
	if value == nil {
		return nil
	}
	result := &agentcore.RateLimitObservation{
		RequestLimit: value.RequestLimit, RequestRemaining: value.RequestRemaining, RequestReset: value.RequestReset,
		TokenLimit: value.TokenLimit, TokenRemaining: value.TokenRemaining, TokenReset: value.TokenReset,
	}
	result.ObservedAt, _ = time.Parse(time.RFC3339Nano, value.ObservedAt)
	return result
}

func pricingSnapshotFromWire(value *PricingSnapshot) *agentcore.PricingSnapshot {
	if value == nil {
		return nil
	}
	return &agentcore.PricingSnapshot{
		SnapshotID: value.SnapshotID, ProviderID: value.ProviderID, ModelID: value.ModelID,
		Currency: value.Currency, UnitTokens: value.UnitTokens,
		CachedInputPerUnitUSD: value.CachedInputPerUnitUSD, UncachedInputPerUnitUSD: value.UncachedInputPerUnitUSD,
		OutputPerUnitUSD: value.OutputPerUnitUSD, EffectiveDate: value.EffectiveDate, SourceURL: value.SourceURL,
	}
}

func rawModelUsageRecordFromWire(value RawModelUsage) agentcore.RawModelUsage {
	return agentcore.RawModelUsage{
		NormalizerID: value.NormalizerID, PromptTokens: cloneIntPointer(value.PromptTokens),
		InputTokens: cloneIntPointer(value.InputTokens), CompletionTokens: cloneIntPointer(value.CompletionTokens),
		OutputTokens: cloneIntPointer(value.OutputTokens), ReasoningTokens: cloneIntPointer(value.ReasoningTokens),
		TotalTokens:  cloneIntPointer(value.TotalTokens),
		CachedTokens: cloneIntPointer(value.CachedTokens), PromptCacheHitTokens: cloneIntPointer(value.PromptCacheHitTokens),
		PromptCacheMissTokens: cloneIntPointer(value.PromptCacheMissTokens), CacheReadInputTokens: cloneIntPointer(value.CacheReadInputTokens),
		CacheCreationInputTokens: cloneIntPointer(value.CacheCreationInputTokens),
	}
}

func researchPhaseFromKind(kind agentcore.SemanticKind) string {
	switch kind {
	case agentcore.SemanticKindResearchAccepted:
		return "accepted"
	case agentcore.SemanticKindResearchPlan:
		return "planning"
	case agentcore.SemanticKindResearchClaimBatch:
		return "researching"
	case agentcore.SemanticKindResearchVerification:
		return "verifying"
	case agentcore.SemanticKindResearchReport:
		return "reporting"
	case agentcore.SemanticKindResearchTerminal:
		return "terminal"
	default:
		return ""
	}
}
