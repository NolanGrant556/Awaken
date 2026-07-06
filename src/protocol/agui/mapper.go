package agui

import (
	"encoding/json"
	"time"

	"awaken/src/agentcore"
)

func MapEvent(event agentcore.AwakenEvent) []Event {
	base := Event{
		Timestamp:                        toUnixMillis(event.Timestamp),
		ThreadID:                         event.ThreadID,
		RunID:                            event.RunID,
		TurnID:                           event.TurnID,
		QueryChainID:                     event.QueryChainID,
		Depth:                            event.Depth,
		TurnCount:                        event.TurnCount,
		MessageID:                        event.MessageID,
		Role:                             event.Role,
		TerminalReason:                   string(event.TerminalReason),
		TransitionReason:                 string(event.TransitionReason),
		ContextModelWindow:               event.ContextModelWindow,
		ContextOutputReserve:             event.ContextOutputReserve,
		ContextEffectiveWindow:           event.ContextEffectiveWindow,
		ContextEstimatedTokens:           event.ContextEstimatedTokens,
		ContextActualTokens:              event.ContextActualTokens,
		ContextWarningThreshold:          event.ContextWarningThreshold,
		ContextAutoCompactThreshold:      event.ContextAutoCompactThreshold,
		ContextCompactTarget:             event.ContextCompactTarget,
		ContextEstimatedDeltaTokens:      event.ContextEstimatedDeltaTokens,
		ContextCompactMode:               string(event.CompactMode),
		ContextCompactBoundaryID:         event.CompactBoundaryID,
		ContextCompactPreEstimate:        cloneIntPointer(event.CompactPreEstimate),
		ContextCompactPostEstimate:       cloneIntPointer(event.CompactPostEstimate),
		CompactMode:                      string(event.CompactMode),
		CompactBoundaryID:                event.CompactBoundaryID,
		CompactPreEstimate:               cloneIntPointer(event.CompactPreEstimate),
		CompactPostEstimate:              cloneIntPointer(event.CompactPostEstimate),
		CompactSummaryModel:              event.CompactSummaryModel,
		CompactFallbackUsed:              cloneBoolPointer(event.CompactFallbackUsed),
		CompactToolResultsPruned:         cloneBoolPointer(event.CompactToolResultsPruned),
		CompactToolResultsPrunedCount:    event.CompactToolResultsPrunedCount,
		CompactPrunedSafeGroupCount:      event.CompactPrunedSafeGroupCount,
		CompactPrunedMessageCount:        event.CompactPrunedMessageCount,
		CompactSummaryMaxTokens:          event.CompactSummaryMaxTokens,
		CompactSummaryRetryCount:         event.CompactSummaryRetryCount,
		CompactSummaryRawChars:           event.CompactSummaryRawChars,
		CompactSummaryParsedSections:     event.CompactSummaryParsedSections,
		CompactSummaryMeaningfulSections: event.CompactSummaryMeaningfulSections,
		CompactSummaryQualityIssue:       event.CompactSummaryQualityIssue,
		CompactSummarySourceHadUser:      cloneBoolPointer(event.CompactSummarySourceHadUser),
		CompactSummaryLatestUserPinned:   cloneBoolPointer(event.CompactSummaryLatestUserPinned),
		CompactFallbackMarker:            event.CompactFallbackMarker,
		CompactFailureReason:             event.CompactFailureReason,
	}

	switch event.Type {
	case agentcore.RunStartedEventType:
		base.Type = EventRunStarted
		return []Event{base}
	case agentcore.RunFinishedEventType:
		base.Type = EventRunFinished
		base.Result = event.Content
		base.Outcome = &RunOutcome{Type: "success"}
		return []Event{base}
	case agentcore.RunErrorEventType:
		base.Type = EventRunError
		base.Message = event.Error
		base.Code = event.Code
		return []Event{base}
	case agentcore.TurnStartedEventType:
		base.Type = EventAwakenTurnStarted
		return []Event{base}
	case agentcore.TurnFinishedEventType:
		base.Type = EventAwakenTurnFinished
		return []Event{base}
	case agentcore.TextMessageStartedEventType:
		base.Type = EventTextMessageStart
		if base.Role == "" {
			base.Role = "assistant"
		}
		return []Event{base}
	case agentcore.TextDeltaEventType:
		base.Type = EventTextMessageContent
		base.Delta = event.Delta
		return []Event{base}
	case agentcore.TextMessageFinishedEventType:
		base.Type = EventTextMessageEnd
		return []Event{base}
	case agentcore.ReasoningStartedEventType:
		start := base
		start.Type = EventReasoningStart
		messageStart := base
		messageStart.Type = EventReasoningMessageStart
		messageStart.Role = "reasoning"
		return []Event{start, messageStart}
	case agentcore.ReasoningDeltaEventType:
		base.Type = EventReasoningMessageContent
		base.Delta = event.Delta
		return []Event{base}
	case agentcore.ReasoningFinishedEventType:
		messageEnd := base
		messageEnd.Type = EventReasoningMessageEnd
		reasoningEnd := base
		reasoningEnd.Type = EventReasoningEnd
		return []Event{messageEnd, reasoningEnd}
	case agentcore.ToolCallStartedEventType:
		base.Type = EventToolCallStart
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ParentMessageID = event.ParentMessageID
		return []Event{base}
	case agentcore.ToolCallArgsDeltaEventType:
		base.Type = EventToolCallArgsDelta
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Delta = event.Delta
		return []Event{base}
	case agentcore.ToolCallArgsEventType:
		base.Type = EventToolCallArgs
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Delta = event.Arguments
		return []Event{base}
	case agentcore.ToolCallFinishedEventType:
		base.Type = EventToolCallEnd
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		return []Event{base}
	case agentcore.ToolProgressEventType:
		base.Type = EventToolProgress
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Progress = event.Progress
		base.Message = event.Progress
		return []Event{base}
	case agentcore.ToolPermissionDecidedEventType:
		base.Type = EventAwakenToolPermissionDecided
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = event.Content
		base.StaticPermissionBehavior = event.StaticPermissionBehavior
		base.StaticPermissionReason = event.StaticPermissionReason
		base.ToolDecisionAction = event.ToolDecisionAction
		base.ToolDecisionSource = event.ToolDecisionSource
		base.ToolDecisionReason = event.ToolDecisionReason
		base.ApprovalAffectedPaths = event.ApprovalAffectedPaths
		base.ApprovalPatchSummary = event.ApprovalPatchSummary
		// Task 16 unified observability: carry the profile name, transform
		// origin, trashed paths, and guardian violation reason so the
		// Desktop UI can render them.
		base.ProfileName = event.ProfileName
		base.TransformedFrom = event.TransformedFrom
		base.TransformedTo = event.TransformedTo
		base.TrashedPaths = event.TrashedPaths
		base.GuardianViolation = event.GuardianViolation
		return []Event{base}
	case agentcore.ToolResultEventType:
		base.Type = EventToolCallResult
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Content = event.Content
		// TrashedPaths are parsed from the tool output after execution
		// (see tool_runtime.extractTrashedPaths), so they ride on the
		// ToolResult event rather than the pre-execution permission-decided
		// event. Propagate them here so the Desktop UI can surface the
		// "Deleted file moved to recycle bin" notification.
		base.TrashedPaths = event.TrashedPaths
		if base.Role == "" {
			base.Role = "tool"
		}
		return []Event{base}
	case agentcore.CommandOutputEventType:
		base.Type = EventAwakenCommandOutput
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.CommandStream = event.CommandStream
		base.Delta = event.Delta
		base.Truncated = event.Truncated
		return []Event{base}
	case agentcore.PatchUpdatedEventType:
		base.Type = EventAwakenPatchUpdated
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Patch = event.Patch
		base.PatchSummary = event.PatchSummary
		base.ApprovalAffectedPaths = event.ApprovalAffectedPaths
		base.Truncated = event.Truncated
		return []Event{base}
	case agentcore.TurnDiffEventType:
		base.Type = EventAwakenTurnDiff
		base.Diff = event.Diff
		base.Truncated = event.Truncated
		return []Event{base}
	case agentcore.ContextEstimatedEventType:
		base.Type = EventAwakenContextEstimated
		base.Message = event.Content
		return []Event{base}
	case agentcore.ContextWarningEventType:
		base.Type = EventAwakenContextWarning
		base.Message = event.Content
		return []Event{base}
	case agentcore.ContextBlockedEventType:
		base.Type = EventAwakenContextBlocked
		base.Message = event.Content
		return []Event{base}
	case agentcore.CompactStartedEventType:
		base.Type = EventAwakenCompactStarted
		base.Message = event.Content
		return []Event{base}
	case agentcore.CompactFinishedEventType:
		base.Type = EventAwakenCompactFinished
		base.Message = event.Content
		return []Event{base}
	case agentcore.CompactFailedEventType:
		base.Type = EventAwakenCompactFailed
		base.Message = event.Content
		return []Event{base}
	case agentcore.ToolPairingRepairedEventType:
		base.Type = EventAwakenToolPairingRepaired
		base.Message = event.Content
		return []Event{base}
	case agentcore.ApprovalRequestedEventType:
		base.Type = EventAwakenApprovalRequested
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ApprovalID = event.ApprovalID
		base.ApprovalTitle = event.ApprovalTitle
		base.ApprovalDescription = event.ApprovalDescription
		base.ApprovalRiskTone = string(event.ApprovalRiskTone)
		base.ApprovalAffectedPaths = event.ApprovalAffectedPaths
		base.ApprovalPatchSummary = event.ApprovalPatchSummary
		return []Event{base}
	case agentcore.ApprovalResolvedEventType:
		base.Type = EventAwakenApprovalResolved
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ApprovalID = event.ApprovalID
		base.ApprovalTitle = event.ApprovalTitle
		base.ApprovalDescription = event.ApprovalDescription
		base.ApprovalRiskTone = string(event.ApprovalRiskTone)
		base.ApprovalDecision = string(event.ApprovalDecision)
		base.ApprovalAffectedPaths = event.ApprovalAffectedPaths
		base.ApprovalPatchSummary = event.ApprovalPatchSummary
		return []Event{base}
	case agentcore.ArtifactCreatedEventType:
		base.Type = EventAwakenArtifactCreated
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ArtifactID = event.ArtifactID
		base.ArtifactTitle = event.ArtifactTitle
		base.ArtifactDescription = event.ArtifactDescription
		base.ArtifactKind = event.ArtifactKind
		base.ArtifactPath = event.ArtifactPath
		base.ArtifactContent = event.ArtifactContent
		base.ArtifactStatus = string(event.ArtifactStatus)
		return []Event{base}
	case agentcore.ArtifactUpdatedEventType:
		base.Type = EventAwakenArtifactUpdated
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ArtifactID = event.ArtifactID
		base.ArtifactTitle = event.ArtifactTitle
		base.ArtifactDescription = event.ArtifactDescription
		base.ArtifactKind = event.ArtifactKind
		base.ArtifactPath = event.ArtifactPath
		base.ArtifactContent = event.ArtifactContent
		base.ArtifactStatus = string(event.ArtifactStatus)
		return []Event{base}
	default:
		base.Type = EventAwakenUnknown
		base.RawType = string(event.Type)
		base.Message = event.Content
		base.RawEvent = rawAwakenEvent(event)
		return []Event{base}
	}
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func toUnixMillis(ts time.Time) int64 {
	if ts.IsZero() {
		return 0
	}
	return ts.UnixMilli()
}

func rawAwakenEvent(event agentcore.AwakenEvent) map[string]any {
	data, err := json.Marshal(event)
	if err != nil {
		return map[string]any{"Type": string(event.Type)}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return map[string]any{"Type": string(event.Type)}
	}
	return payload
}
