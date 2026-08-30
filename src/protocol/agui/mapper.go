package agui

import (
	"encoding/json"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

func MapEvent(event agentcore.AwakenEvent) []Event {
	base := Event{
		Timestamp:                        toUnixMillis(event.Timestamp),
		ThreadID:                         event.ThreadID,
		RunID:                            event.RunID,
		TurnID:                           event.TurnID,
		SubagentTaskID:                   event.SubagentTaskID,
		SubagentAgentID:                  event.AgentID,
		SubagentParentAgentID:            event.ParentAgentID,
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
		ContextUsageSnapshot:             mapContextUsageSnapshot(event.ContextUsageSnapshot),
		ModelUsage:                       mapModelUsageRecord(event.ModelUsage),
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
	case agentcore.MemoryRecalledEventType:
		base.Type = EventAwakenMemoryRecalled
		base.Memories = make([]MemoryReceipt, 0, len(event.MemoryReceipts))
		for _, receipt := range event.MemoryReceipts {
			base.Memories = append(base.Memories, MemoryReceipt{MemoryID: receipt.MemoryID, Scope: receipt.Scope, Type: receipt.Type, Revision: receipt.Revision, Bytes: receipt.Bytes})
		}
		return []Event{base}
	case agentcore.MemoryMutatedEventType:
		base.Memories = mapMemoryReceipts(event.MemoryReceipts)
		base.Type = memoryMutationEventType(base.Memories)
		return []Event{base}
	case agentcore.ContextSnapshotEventType:
		base.Type = EventAwakenContextSnapshot
		return []Event{base}
	case agentcore.ModelUsageEventType:
		base.Type = EventAwakenModelUsage
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
		base.ToolPreparationKind = event.ToolPreparationKind
		base.ToolPreparationPath = event.ToolPreparationPath
		base.ToolPreparationAddedLines = event.ToolPreparationAddedLines
		base.ToolPreparationDeletedLines = event.ToolPreparationDeletedLines
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
		if base.Progress == "" {
			base.Progress = event.ToolProgressMessage
		}
		base.Message = firstNonEmpty(event.ToolProgressMessage, event.Progress)
		base.ToolProgressStage = event.ToolProgressStage
		base.ToolProgressMessage = event.ToolProgressMessage
		base.ToolProgressPercentage = event.ToolProgressPercentage
		copyToolOutcomeFields(&base, event)
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
		base.AffectedPaths = event.AffectedPaths
		base.PatchSummary = event.PatchSummary
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
		base.ToolProgressStage = event.ToolProgressStage
		base.ToolProgressMessage = event.ToolProgressMessage
		base.ToolProgressPercentage = event.ToolProgressPercentage
		copyToolOutcomeFields(&base, event)
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
		base.AffectedPaths = event.AffectedPaths
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
	case agentcore.AuthRequiredEventType:
		base.Type = EventAwakenAuthRequired
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = event.Content
		base.AuthProvider = event.AuthProvider
		base.AuthURL = event.AuthURL
		return []Event{base}
	case agentcore.RuntimePrepareEventType, agentcore.RuntimePrepareStartedEventType:
		base.Type = EventAwakenRuntimePrepare
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = event.Content
		base.RuntimeName = event.RuntimeName
		base.RuntimeVersion = event.RuntimeVersion
		return []Event{base}
	case agentcore.RuntimePrepareProgressEventType:
		base.Type = EventAwakenRuntimePrepareProgress
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = event.Content
		base.Progress = event.Progress
		base.RuntimeName = event.RuntimeName
		base.RuntimeVersion = event.RuntimeVersion
		return []Event{base}
	case agentcore.RuntimeReadyEventType:
		base.Type = EventAwakenRuntimeReady
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = event.Content
		base.RuntimeName = event.RuntimeName
		base.RuntimeVersion = event.RuntimeVersion
		return []Event{base}
	case agentcore.RuntimeRepairFailedEventType:
		base.Type = EventAwakenRuntimeRepairFailed
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = firstNonEmpty(event.Content, event.Error)
		base.RuntimeName = event.RuntimeName
		base.RuntimeVersion = event.RuntimeVersion
		return []Event{base}
	case agentcore.RuntimeMissingEventType:
		base.Type = EventAwakenRuntimeMissing
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = event.Content
		base.RuntimeName = event.RuntimeName
		base.RuntimeVersion = event.RuntimeVersion
		return []Event{base}
	case agentcore.AwakenEventType("SkillDiscovered"):
		base.Type = EventAwakenSkillDiscovered
		copySkillFields(&base, event)
		return []Event{base}
	case agentcore.AwakenEventType("SkillInvoked"), agentcore.SkillActivatedEventType:
		base.Type = EventAwakenSkillInvoked
		copySkillFields(&base, event)
		return []Event{base}
	case agentcore.AwakenEventType("SkillContentLoaded"), agentcore.ResourceReceiptEventType:
		base.Type = EventAwakenSkillContentLoaded
		copySkillFields(&base, event)
		return []Event{base}
	case agentcore.AwakenEventType("SkillExecutionContext"):
		base.Type = EventAwakenSkillExecutionContext
		copySkillFields(&base, event)
		return []Event{base}
	case agentcore.AwakenEventType("SkillFailedToLoad"):
		base.Type = EventAwakenSkillFailedToLoad
		copySkillFields(&base, event)
		return []Event{base}
	case agentcore.UserQuestionRequestedType:
		base.Type = EventAwakenUserQuestionRequested
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.QuestionID = event.QuestionID
		base.Content = event.Content
		return []Event{base}
	case agentcore.UserQuestionResolvedType:
		base.Type = EventAwakenUserQuestionResolved
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.QuestionID = event.QuestionID
		base.QuestionAnswer = event.QuestionAnswer
		base.QuestionResolution = event.QuestionResolution
		return []Event{base}
	case agentcore.ExternalInteractionRequestedType:
		base.Type = EventAwakenExternalInteractionRequested
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.InteractionID = event.InteractionID
		base.InteractionKind = event.InteractionKind
		base.InteractionTitle = event.InteractionTitle
		base.InteractionSafeMessage = event.InteractionSafeMessage
		base.InteractionVerificationURL = event.InteractionVerificationURL
		base.InteractionQRImageURL = event.InteractionQRImageURL
		base.InteractionProvider = event.InteractionProvider
		base.InteractionRecoveries = append([]string(nil), event.InteractionRecoveries...)
		base.InteractionFormSchema = event.InteractionFormSchema
		base.AuthProvider = event.AuthProvider
		base.AuthURL = event.AuthURL
		return []Event{base}
	case agentcore.ExternalInteractionResolvedType:
		base.Type = EventAwakenExternalInteractionResolved
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.InteractionID = event.InteractionID
		base.InteractionKind = event.InteractionKind
		base.InteractionResolution = event.InteractionResolution
		base.InteractionProvider = event.InteractionProvider
		return []Event{base}
	case agentcore.ArtifactCreatedEventType:
		base.Type = EventAwakenArtifactCreated
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ArtifactContractVersion = tool.ArtifactContractVersionOrLegacy(event.ArtifactContractVersion)
		base.ArtifactProducer = firstNonEmpty(event.ArtifactProducer, event.ToolName)
		base.ArtifactRole = string(tool.ResolveArtifactRole(event.ArtifactRole, base.ArtifactProducer, event.ArtifactKind, event.ArtifactPath))
		base.ArtifactID = event.ArtifactID
		base.ArtifactGroupID = event.ArtifactGroupID
		base.ArtifactGroupIndex = event.ArtifactGroupIndex
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
		base.ArtifactContractVersion = tool.ArtifactContractVersionOrLegacy(event.ArtifactContractVersion)
		base.ArtifactProducer = firstNonEmpty(event.ArtifactProducer, event.ToolName)
		base.ArtifactRole = string(tool.ResolveArtifactRole(event.ArtifactRole, base.ArtifactProducer, event.ArtifactKind, event.ArtifactPath))
		base.ArtifactID = event.ArtifactID
		base.ArtifactGroupID = event.ArtifactGroupID
		base.ArtifactGroupIndex = event.ArtifactGroupIndex
		base.ArtifactTitle = event.ArtifactTitle
		base.ArtifactDescription = event.ArtifactDescription
		base.ArtifactKind = event.ArtifactKind
		base.ArtifactPath = event.ArtifactPath
		base.ArtifactContent = event.ArtifactContent
		base.ArtifactStatus = string(event.ArtifactStatus)
		return []Event{base}
	case agentcore.ToolRecoveryRequiredEventType:
		base.Type = EventAwakenToolRecoveryRequired
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = firstNonEmpty(event.Content, event.Error)
		base.Code = firstNonEmpty(event.Code, event.ErrorCode)
		copyToolOutcomeFields(&base, event)
		return []Event{base}
	case agentcore.ToolRetryBlockedEventType:
		base.Type = EventAwakenToolRetryBlocked
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Message = firstNonEmpty(event.Content, event.Error)
		base.Code = firstNonEmpty(event.Code, event.ErrorCode)
		copyToolOutcomeFields(&base, event)
		return []Event{base}
	case agentcore.ACPSessionStartedEventType:
		base.Type = EventAwakenACPSessionChanged
		base.Status = "started"
		base.ToolCallID = event.ToolCallID
		base.Message = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPSessionClosedEventType:
		base.Type = EventAwakenACPSessionChanged
		base.Status = "closed"
		base.ToolCallID = event.ToolCallID
		base.Message = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPSessionInterruptedEventType:
		base.Type = EventAwakenACPSessionChanged
		base.Status = "interrupted"
		base.ToolCallID = event.ToolCallID
		base.Message = firstNonEmpty(event.Error, event.Content)
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPTurnStartedEventType:
		base.Type = EventAwakenACPTurnChanged
		base.Status = "started"
		base.ToolCallID = event.ToolCallID
		base.Message = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPTurnFinishedEventType:
		base.Type = EventAwakenACPTurnChanged
		base.Status = "finished"
		base.ToolCallID = event.ToolCallID
		base.Content = event.Content
		base.Message = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPToolSnapshotEventType:
		base.Type = EventAwakenACPToolChanged
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.Content = event.Content
		base.Message = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPPermissionRequestedEventType:
		base.Type = EventAwakenACPPermissionRequested
		base.ToolCallID = event.ToolCallID
		base.InteractionID = event.InteractionID
		base.Message = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ACPPermissionResolvedEventType:
		base.Type = EventAwakenACPPermissionResolved
		base.ToolCallID = event.ToolCallID
		base.InteractionID = event.InteractionID
		base.Message = event.Content
		base.Status = event.Content
		copyACPFields(&base, event)
		return []Event{base}
	case agentcore.ProviderReconnectEventType:
		base.Type = EventAwakenProviderReconnect
		if event.ProviderReconnect != nil {
			base.ProviderReconnect = &ProviderReconnectPayload{
				Phase:           event.ProviderReconnect.Phase,
				Attempt:         event.ProviderReconnect.Attempt,
				MaxAttempts:     event.ProviderReconnect.MaxAttempts,
				Detail:          event.ProviderReconnect.Detail,
				ResetInProgress: event.ProviderReconnect.ResetInProgress,
			}
		}
		return []Event{base}
	default:
		base.Type = EventAwakenUnknown
		base.RawType = string(event.Type)
		base.Message = event.Content
		base.RawEvent = rawAwakenEvent(event)
		return []Event{base}
	}
}

func mapContextUsageSnapshot(snapshot *agentcore.ContextUsageSnapshot) *ContextUsageSnapshot {
	if snapshot == nil {
		return nil
	}
	mapped := &ContextUsageSnapshot{
		SchemaVersion: snapshot.SchemaVersion, SnapshotID: snapshot.SnapshotID, EstimatorID: snapshot.EstimatorID,
		Basis: snapshot.Basis, RequestID: snapshot.RequestID, ModelID: snapshot.ModelID,
		CapabilityModelID: snapshot.CapabilityModelID, EndpointSelector: snapshot.EndpointSelector,
		ModelSelectorKind: snapshot.ModelSelectorKind,
		ProtocolFamily:    snapshot.ProtocolFamily, ContextEpochID: snapshot.ContextEpochID,
		ContextEpochBoundaryReason: snapshot.ContextEpochBoundaryReason, RequestShapeDigest: snapshot.RequestShapeDigest,
		ModelWindowTokens: snapshot.ModelWindowTokens, OutputReserveTokens: snapshot.OutputReserveTokens,
		EffectiveInputWindowTokens: snapshot.EffectiveInputWindowTokens, EstimatedInputTokens: snapshot.EstimatedInputTokens,
		WarningThresholdTokens: snapshot.WarningThresholdTokens, AutoCompactThresholdTokens: snapshot.AutoCompactThresholdTokens,
		CompactTargetTokens: snapshot.CompactTargetTokens, GeneratedAt: snapshot.GeneratedAt.UTC().Format(time.RFC3339Nano),
	}
	for _, contribution := range snapshot.Contributions {
		mapped.Contributions = append(mapped.Contributions, ContextUsageContribution{
			Kind: string(contribution.Kind), EstimatedTokens: contribution.EstimatedTokens, SourceCount: contribution.SourceCount,
		})
	}
	return mapped
}

func mapModelUsageRecord(usage *agentcore.ModelUsageRecord) *ModelUsageRecord {
	if usage == nil {
		return nil
	}
	return &ModelUsageRecord{
		SchemaVersion: usage.SchemaVersion, RequestID: usage.RequestID, Purpose: usage.Purpose,
		ProviderID: usage.ProviderID, ProfileID: usage.ProfileID, CredentialRevision: usage.CredentialRevision,
		ModelID: usage.ModelID, CapabilityModelID: usage.CapabilityModelID, EndpointSelector: usage.EndpointSelector,
		ModelSelectorKind: usage.ModelSelectorKind, ProtocolFamily: usage.ProtocolFamily, EndpointContractID: usage.EndpointContractID,
		ReasoningPolicy: usage.ReasoningPolicy,
		CacheMode:       usage.CacheMode, CacheKeyApplied: usage.CacheKeyApplied,
		ContextEpochID: usage.ContextEpochID, PrefixDigest: usage.PrefixDigest, InvalidationReason: usage.InvalidationReason,
		RetryIndex: usage.RetryIndex, AuxiliaryFallback: usage.AuxiliaryFallback,
		SentEstimatedInputTokens: cloneIntPointer(usage.SentEstimatedInputTokens),
		InputTokens:              cloneIntPointer(usage.InputTokens), CachedInputTokens: cloneIntPointer(usage.CachedInputTokens),
		UncachedInputTokens: cloneIntPointer(usage.UncachedInputTokens), CacheWriteTokens: cloneIntPointer(usage.CacheWriteTokens),
		OutputTokens: cloneIntPointer(usage.OutputTokens), ReasoningTokens: cloneIntPointer(usage.ReasoningTokens),
		TotalTokens:   cloneIntPointer(usage.TotalTokens),
		RawUsage:      mapRawModelUsage(usage.RawUsage),
		UsageComplete: usage.UsageComplete, CalculatedCostUSD: cloneFloatPointer(usage.CalculatedCostUSD),
		PricingSnapshot: mapPricingSnapshot(usage.PricingSnapshot), Currency: usage.Currency,
		ProviderRequestID: usage.ProviderRequestID, RateLimitObservation: mapRateLimitObservation(usage.RateLimitObservation), LatencyMS: usage.LatencyMS,
		Status: string(usage.Status), ErrorCode: usage.ErrorCode, CompletedAt: usage.CompletedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapRateLimitObservation(value *agentcore.RateLimitObservation) *RateLimitObservation {
	if value == nil {
		return nil
	}
	return &RateLimitObservation{
		RequestLimit: value.RequestLimit, RequestRemaining: value.RequestRemaining, RequestReset: value.RequestReset,
		TokenLimit: value.TokenLimit, TokenRemaining: value.TokenRemaining, TokenReset: value.TokenReset,
		ObservedAt: value.ObservedAt.UTC().Format(time.RFC3339Nano),
	}
}

func mapPricingSnapshot(value *agentcore.PricingSnapshot) *PricingSnapshot {
	if value == nil {
		return nil
	}
	return &PricingSnapshot{
		SnapshotID: value.SnapshotID, ProviderID: value.ProviderID, ModelID: value.ModelID,
		Currency: value.Currency, UnitTokens: value.UnitTokens,
		CachedInputPerUnitUSD: value.CachedInputPerUnitUSD, UncachedInputPerUnitUSD: value.UncachedInputPerUnitUSD,
		OutputPerUnitUSD: value.OutputPerUnitUSD, EffectiveDate: value.EffectiveDate, SourceURL: value.SourceURL,
	}
}

func cloneFloatPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func mapRawModelUsage(usage agentcore.RawModelUsage) RawModelUsage {
	return RawModelUsage{
		NormalizerID: usage.NormalizerID, PromptTokens: cloneIntPointer(usage.PromptTokens),
		InputTokens: cloneIntPointer(usage.InputTokens), CompletionTokens: cloneIntPointer(usage.CompletionTokens),
		OutputTokens: cloneIntPointer(usage.OutputTokens), ReasoningTokens: cloneIntPointer(usage.ReasoningTokens),
		TotalTokens:  cloneIntPointer(usage.TotalTokens),
		CachedTokens: cloneIntPointer(usage.CachedTokens), PromptCacheHitTokens: cloneIntPointer(usage.PromptCacheHitTokens),
		PromptCacheMissTokens: cloneIntPointer(usage.PromptCacheMissTokens), CacheReadInputTokens: cloneIntPointer(usage.CacheReadInputTokens),
		CacheCreationInputTokens: cloneIntPointer(usage.CacheCreationInputTokens),
	}
}

func mapMemoryReceipts(receipts []agentcore.MemoryRecallReceipt) []MemoryReceipt {
	result := make([]MemoryReceipt, 0, len(receipts))
	for _, receipt := range receipts {
		result = append(result, MemoryReceipt{MemoryID: receipt.MemoryID, Scope: receipt.Scope, Type: receipt.Type, Revision: receipt.Revision, Bytes: receipt.Bytes, Action: receipt.Action, Status: receipt.Status, ErrorCode: receipt.ErrorCode})
	}
	return result
}

func memoryMutationEventType(receipts []MemoryReceipt) EventType {
	if len(receipts) == 0 || receipts[0].Status == "rejected" {
		return EventAwakenMemoryRejected
	}
	switch receipts[0].Action {
	case "create":
		return EventAwakenMemoryCreated
	case "update":
		return EventAwakenMemoryUpdated
	case "supersede":
		return EventAwakenMemorySuperseded
	case "delete":
		return EventAwakenMemoryDeleted
	default:
		return EventAwakenMemoryRejected
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

func copyToolOutcomeFields(dst *Event, event agentcore.AwakenEvent) {
	dst.ToolStatus = event.ToolStatus
	dst.TransportStatus = event.TransportStatus
	dst.SemanticStatus = event.SemanticStatus
	dst.ProcessStatus = event.ProcessStatus
	dst.ProcessExitCode = cloneIntPointer(event.ProcessExitCode)
	dst.ProcessTimedOut = event.ProcessTimedOut
	dst.ErrorCode = event.ErrorCode
	dst.ErrorCategory = event.ErrorCategory
	dst.Retryable = event.Retryable
	dst.RecoveryOptions = append([]string(nil), event.RecoveryOptions...)
	dst.DurationMs = event.DurationMs
	if len(event.Diagnostic) > 0 {
		dst.Diagnostic = make(map[string]any, len(event.Diagnostic))
		for key, value := range event.Diagnostic {
			dst.Diagnostic[key] = value
		}
		applyExecutionMetadata(dst, event.Diagnostic)
	}
}

func copyACPFields(dst *Event, event agentcore.AwakenEvent) {
	if len(event.Diagnostic) > 0 {
		dst.Diagnostic = make(map[string]any, len(event.Diagnostic))
		for key, value := range event.Diagnostic {
			dst.Diagnostic[key] = value
		}
	}
	// Promote stable ACP identity fields from the diagnostic blob to typed wire
	// fields so the renderer does not need to parse raw diagnostic maps.
	if v, ok := stringDiagnostic(event.Diagnostic, "runner_id"); ok {
		dst.ACPRunnerID = v
	}
	if v, ok := stringDiagnostic(event.Diagnostic, "acp_session_id"); ok {
		dst.ACPSessionID = v
	}
	if v, ok := stringDiagnostic(event.Diagnostic, "turn_id"); ok {
		dst.ACPTurnID = v
	}
	if v, ok := stringDiagnostic(event.Diagnostic, "acp_tool_call_id"); ok {
		dst.ACPToolCallID = v
	}
	dst.ACPPermissionOptions = acpPermissionOptionsFromDiagnostic(event.Diagnostic)
}

func stringDiagnostic(diagnostic map[string]any, key string) (string, bool) {
	if diagnostic == nil {
		return "", false
	}
	raw, ok := diagnostic[key]
	if !ok {
		return "", false
	}
	v, ok := raw.(string)
	return v, ok
}

func acpPermissionOptionsFromDiagnostic(diagnostic map[string]any) []ACPPermissionOption {
	if diagnostic == nil {
		return nil
	}
	raw, ok := diagnostic["options"]
	if !ok {
		return nil
	}

	var maps []map[string]any
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
	case []map[string]any:
		maps = v
	default:
		return nil
	}

	out := make([]ACPPermissionOption, 0, len(maps))
	for _, m := range maps {
		opt := ACPPermissionOption{}
		if v, ok := m["option_id"].(string); ok {
			opt.OptionID = v
		}
		if v, ok := m["name"].(string); ok {
			opt.Name = v
		}
		if v, ok := m["kind"].(string); ok {
			opt.Kind = v
		}
		if opt.OptionID != "" {
			out = append(out, opt)
		}
	}
	return out
}

func copySkillFields(dst *Event, event agentcore.AwakenEvent) {
	dst.SkillID = event.SkillID
	dst.SkillVersion = event.SkillVersion
	dst.SkillResource = event.ResourcePath
	dst.SkillResourceSHA256 = event.ResourceSHA256
	dst.SkillResourceSize = event.ResourceSize
	dst.SkillResourceMode = event.ResourceMode
	dst.SkillResourceSource = event.ResourceSource
	if len(event.Diagnostic) > 0 {
		dst.SkillSource = firstStringValue(event.Diagnostic, "skill_source", "skillSource", "source")
		dst.SkillPath = firstStringValue(event.Diagnostic, "skill_path", "skillPath", "skill_file_path", "skillFilePath", "path")
		dst.SkillStatus = firstStringValue(event.Diagnostic, "skill_status", "skillStatus", "compatibility_status", "compatibilityStatus", "status")
		dst.SkillEffectiveTools = stringSliceFromAny(firstPresentValue(event.Diagnostic, "effective_tools", "effectiveTools"))
		dst.SkillDiagnostics = diagnosticStringsFromAny(firstPresentValue(event.Diagnostic, "diagnostics", "compatibility_errors", "compatibilityErrors"))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
