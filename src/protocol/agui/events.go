package agui

type EventType string

const (
	EventRunStarted                         EventType = "RUN_STARTED"
	EventRunFinished                        EventType = "RUN_FINISHED"
	EventRunError                           EventType = "RUN_ERROR"
	EventRunStatusChanged                   EventType = "RUN_STATUS_CHANGED"
	EventAwakenTurnStarted                  EventType = "AWAKEN_TURN_STARTED"
	EventAwakenTurnFinished                 EventType = "AWAKEN_TURN_FINISHED"
	EventTextMessageStart                   EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent                 EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd                     EventType = "TEXT_MESSAGE_END"
	EventReasoningStart                     EventType = "REASONING_START"
	EventReasoningMessageStart              EventType = "REASONING_MESSAGE_START"
	EventReasoningMessageContent            EventType = "REASONING_MESSAGE_CONTENT"
	EventReasoningMessageEnd                EventType = "REASONING_MESSAGE_END"
	EventReasoningEnd                       EventType = "REASONING_END"
	EventToolCallStart                      EventType = "TOOL_CALL_START"
	EventToolCallArgsDelta                  EventType = "TOOL_CALL_ARGS_DELTA"
	EventToolCallArgs                       EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd                        EventType = "TOOL_CALL_END"
	EventToolProgress                       EventType = "TOOL_PROGRESS"
	EventToolCallResult                     EventType = "TOOL_CALL_RESULT"
	EventAwakenToolPermissionDecided        EventType = "AWAKEN_TOOL_PERMISSION_DECIDED"
	EventAwakenCommandOutput                EventType = "AWAKEN_COMMAND_OUTPUT"
	EventAwakenPatchUpdated                 EventType = "AWAKEN_PATCH_UPDATED"
	EventAwakenTurnDiff                     EventType = "AWAKEN_TURN_DIFF"
	EventAwakenContextEstimated             EventType = "AWAKEN_CONTEXT_ESTIMATED"
	EventAwakenContextWarning               EventType = "AWAKEN_CONTEXT_WARNING"
	EventAwakenContextBlocked               EventType = "AWAKEN_CONTEXT_BLOCKED"
	EventAwakenCompactStarted               EventType = "AWAKEN_COMPACT_STARTED"
	EventAwakenCompactFinished              EventType = "AWAKEN_COMPACT_FINISHED"
	EventAwakenCompactFailed                EventType = "AWAKEN_COMPACT_FAILED"
	EventAwakenToolPairingRepaired          EventType = "AWAKEN_TOOL_PAIRING_REPAIRED"
	EventAwakenAuthRequired                 EventType = "AWAKEN_AUTH_REQUIRED"
	EventAwakenRuntimePrepare               EventType = "AWAKEN_RUNTIME_PREPARE"
	EventAwakenRuntimePrepareProgress       EventType = "AWAKEN_RUNTIME_PREPARE_PROGRESS"
	EventAwakenRuntimeReady                 EventType = "AWAKEN_RUNTIME_READY"
	EventAwakenRuntimeRepairFailed          EventType = "AWAKEN_RUNTIME_REPAIR_FAILED"
	EventAwakenRuntimeMissing               EventType = "AWAKEN_RUNTIME_MISSING"
	EventAwakenSkillDiscovered              EventType = "AWAKEN_SKILL_DISCOVERED"
	EventAwakenSkillInvoked                 EventType = "AWAKEN_SKILL_INVOKED"
	EventAwakenSkillContentLoaded           EventType = "AWAKEN_SKILL_CONTENT_LOADED"
	EventAwakenSkillExecutionContext        EventType = "AWAKEN_SKILL_EXECUTION_CONTEXT"
	EventAwakenSkillFailedToLoad            EventType = "AWAKEN_SKILL_FAILED_TO_LOAD"
	EventAwakenHookStarted                  EventType = "AWAKEN_HOOK_STARTED"
	EventAwakenHookCompleted                EventType = "AWAKEN_HOOK_COMPLETED"
	EventAwakenUserQuestionRequested        EventType = "AWAKEN_USER_QUESTION_REQUESTED"
	EventAwakenUserQuestionResolved         EventType = "AWAKEN_USER_QUESTION_RESOLVED"
	EventAwakenExternalInteractionRequested EventType = "AWAKEN_EXTERNAL_INTERACTION_REQUESTED"
	EventAwakenExternalInteractionResolved  EventType = "AWAKEN_EXTERNAL_INTERACTION_RESOLVED"
	EventAwakenArtifactCreated              EventType = "AWAKEN_ARTIFACT_CREATED"
	EventAwakenArtifactUpdated              EventType = "AWAKEN_ARTIFACT_UPDATED"
	EventAwakenToolRecoveryRequired         EventType = "AWAKEN_TOOL_RECOVERY_REQUIRED"
	EventAwakenToolRetryBlocked             EventType = "AWAKEN_TOOL_RETRY_BLOCKED"
	EventAwakenMemoryRecalled               EventType = "AWAKEN_MEMORY_RECALLED"
	EventAwakenMemoryCreated                EventType = "AWAKEN_MEMORY_CREATED"
	EventAwakenMemoryUpdated                EventType = "AWAKEN_MEMORY_UPDATED"
	EventAwakenMemorySuperseded             EventType = "AWAKEN_MEMORY_SUPERSEDED"
	EventAwakenMemoryDeleted                EventType = "AWAKEN_MEMORY_DELETED"
	EventAwakenMemoryRejected               EventType = "AWAKEN_MEMORY_REJECTED"
	EventAwakenContextSnapshot              EventType = "AWAKEN_CONTEXT_SNAPSHOT"
	EventAwakenInstructionSnapshot          EventType = "AWAKEN_INSTRUCTION_SNAPSHOT"
	EventAwakenModelUsage                   EventType = "AWAKEN_MODEL_USAGE"
	EventAwakenSubagentChanged              EventType = "AWAKEN_SUBAGENT_CHANGED"
	EventAwakenSubagentActivity             EventType = "AWAKEN_SUBAGENT_ACTIVITY"
	EventAwakenSubagentMessageQueued        EventType = "AWAKEN_SUBAGENT_MESSAGE_QUEUED"
	EventAwakenACPSessionChanged            EventType = "AWAKEN_ACP_SESSION_CHANGED"
	EventAwakenACPTurnChanged               EventType = "AWAKEN_ACP_TURN_CHANGED"
	EventAwakenACPToolChanged               EventType = "AWAKEN_ACP_TOOL_CHANGED"
	EventAwakenACPPermissionRequested       EventType = "AWAKEN_ACP_PERMISSION_REQUESTED"
	EventAwakenACPPermissionResolved        EventType = "AWAKEN_ACP_PERMISSION_RESOLVED"
	// EventAwakenScheduledNotice is the host-owned fixed-reminder fact of a
	// scheduled task (contract docs/12_定时任务/定时任务设计契约.md §9.4/§10).
	// It is a replayable in-thread fact, never a user or assistant message.
	EventAwakenScheduledNotice EventType = "AWAKEN_SCHEDULED_NOTICE"
	// EventAwakenPlanUpdated is the replayable full-snapshot fact of one root
	// run's plan (contract: 运行计划与任务进度设计契约 §8.1). There are no
	// started/item/finished delta events; the whole sanitized snapshot travels
	// on every frame so reconnect, dedup and replay never depend on lost frames.
	EventAwakenPlanUpdated             EventType = "AWAKEN_PLAN_UPDATED"
	EventAwakenResearchPhase           EventType = "AWAKEN_RESEARCH_PHASE"
	EventAwakenComputerReviewRequested EventType = "AWAKEN_COMPUTER_REVIEW_REQUESTED"
	EventAwakenComputerReviewResolved  EventType = "AWAKEN_COMPUTER_REVIEW_RESOLVED"
	EventAwakenComputerSessionChanged  EventType = "AWAKEN_COMPUTER_SESSION_CHANGED"
	EventAwakenComputerVerification    EventType = "AWAKEN_COMPUTER_VERIFICATION_COMPLETED"
	EventAwakenComputerHandoffStarted  EventType = "AWAKEN_COMPUTER_HANDOFF_STARTED"
	EventAwakenComputerHandoffEnded    EventType = "AWAKEN_COMPUTER_HANDOFF_ENDED"
	// EventAwakenProviderReconnect is a live-only status frame while the
	// runner waits for the provider network or retries a disconnected
	// stream. It is never written to SemanticJournal and must not appear
	// in durable replay. Contract: docs/09_AGUI与A2UI协议/事件与字段契约.md.
	EventAwakenProviderReconnect EventType = "AWAKEN_PROVIDER_RECONNECT"
	EventAwakenUnknown           EventType = "AWAKEN_UNKNOWN_EVENT"
)

type RunOutcome struct {
	Type string `json:"type"`
}

// ProviderReconnectPayload is the live-only reconnect status carried on
// AWAKEN_PROVIDER_RECONNECT. maxAttempts=0 means unbounded connection wait.
type ProviderReconnectPayload struct {
	Phase           string `json:"phase"`
	Attempt         int    `json:"attempt,omitempty"`
	MaxAttempts     int    `json:"maxAttempts,omitempty"`
	Detail          string `json:"detail,omitempty"`
	ResetInProgress bool   `json:"resetInProgress,omitempty"`
}

type ACPPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind,omitempty"`
}

type MemoryReceipt struct {
	MemoryID  string `json:"memoryId"`
	Scope     string `json:"scope"`
	Type      string `json:"memoryType"`
	Revision  int    `json:"revision"`
	Bytes     int    `json:"bytes,omitempty"`
	Action    string `json:"action,omitempty"`
	Status    string `json:"status,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

type RunPlanItemReceipt struct {
	Step       string `json:"step"`
	ActiveForm string `json:"activeForm"`
	Status     string `json:"status"`
}

type ContextUsageContribution struct {
	Kind            string `json:"kind"`
	EstimatedTokens int    `json:"estimatedTokens"`
	SourceCount     int    `json:"sourceCount"`
}

type ContextUsageSnapshot struct {
	SchemaVersion              int                        `json:"schemaVersion"`
	SnapshotID                 string                     `json:"snapshotId"`
	EstimatorID                string                     `json:"estimatorId"`
	Basis                      string                     `json:"basis"`
	RequestID                  string                     `json:"requestId,omitempty"`
	ModelID                    string                     `json:"modelId,omitempty"`
	CapabilityModelID          string                     `json:"capabilityModelId,omitempty"`
	EndpointSelector           string                     `json:"endpointSelector,omitempty"`
	ModelSelectorKind          string                     `json:"modelSelectorKind,omitempty"`
	ProtocolFamily             string                     `json:"protocolFamily,omitempty"`
	ContextEpochID             string                     `json:"contextEpochId,omitempty"`
	ContextEpochBoundaryReason string                     `json:"contextEpochBoundaryReason,omitempty"`
	RequestShapeDigest         string                     `json:"requestShapeDigest,omitempty"`
	ModelWindowTokens          int                        `json:"modelWindowTokens"`
	OutputReserveTokens        int                        `json:"outputReserveTokens"`
	EffectiveInputWindowTokens int                        `json:"effectiveInputWindowTokens"`
	EstimatedInputTokens       int                        `json:"estimatedInputTokens"`
	WarningThresholdTokens     int                        `json:"warningThresholdTokens"`
	AutoCompactThresholdTokens int                        `json:"autoCompactThresholdTokens"`
	CompactTargetTokens        int                        `json:"compactTargetTokens"`
	Contributions              []ContextUsageContribution `json:"contributions,omitempty"`
	GeneratedAt                string                     `json:"generatedAt"`
}

type InstructionDiagnostic struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Scope       string `json:"scope"`
	DisplayPath string `json:"displayPath"`
}

type InstructionSourceSummary struct {
	Scope         string `json:"scope"`
	DisplayPath   string `json:"displayPath"`
	ScopeRoot     string `json:"scopeRoot,omitempty"`
	Digest        string `json:"digest,omitempty"`
	OriginalBytes int    `json:"originalBytes"`
	AppliedBytes  int    `json:"appliedBytes"`
	Truncated     bool   `json:"truncated"`
	Status        string `json:"status"`
}

type InstructionSnapshotSummary struct {
	SnapshotID  string                     `json:"snapshotId"`
	CapturedAt  string                     `json:"capturedAt"`
	Partial     bool                       `json:"partial"`
	Sources     []InstructionSourceSummary `json:"sources"`
	Diagnostics []InstructionDiagnostic    `json:"diagnostics"`
}

type ModelUsageRecord struct {
	SchemaVersion            int                   `json:"schemaVersion"`
	RequestID                string                `json:"requestId"`
	Purpose                  string                `json:"purpose"`
	ProviderID               string                `json:"providerId,omitempty"`
	ProfileID                string                `json:"profileId,omitempty"`
	CredentialRevision       uint64                `json:"credentialRevision,omitempty"`
	ModelID                  string                `json:"modelId,omitempty"`
	CapabilityModelID        string                `json:"capabilityModelId,omitempty"`
	EndpointSelector         string                `json:"endpointSelector,omitempty"`
	ModelSelectorKind        string                `json:"modelSelectorKind,omitempty"`
	ProtocolFamily           string                `json:"protocolFamily,omitempty"`
	EndpointContractID       string                `json:"endpointContractId,omitempty"`
	ReasoningPolicy          string                `json:"reasoningPolicy,omitempty"`
	CacheMode                string                `json:"cacheMode,omitempty"`
	CacheKeyApplied          bool                  `json:"cacheKeyApplied"`
	ContextEpochID           string                `json:"contextEpochId,omitempty"`
	PrefixDigest             string                `json:"prefixDigest,omitempty"`
	InvalidationReason       string                `json:"invalidationReason,omitempty"`
	RetryIndex               int                   `json:"retryIndex,omitempty"`
	AuxiliaryFallback        bool                  `json:"auxiliaryFallback,omitempty"`
	SentEstimatedInputTokens *int                  `json:"sentEstimatedInputTokens,omitempty"`
	InputTokens              *int                  `json:"inputTokens,omitempty"`
	CachedInputTokens        *int                  `json:"cachedInputTokens,omitempty"`
	UncachedInputTokens      *int                  `json:"uncachedInputTokens,omitempty"`
	CacheWriteTokens         *int                  `json:"cacheWriteTokens,omitempty"`
	OutputTokens             *int                  `json:"outputTokens,omitempty"`
	ReasoningTokens          *int                  `json:"reasoningTokens,omitempty"`
	TotalTokens              *int                  `json:"totalTokens,omitempty"`
	RawUsage                 RawModelUsage         `json:"rawTypedUsage"`
	UsageComplete            bool                  `json:"usageComplete"`
	CalculatedCostUSD        *float64              `json:"calculatedCostUsd,omitempty"`
	PricingSnapshot          *PricingSnapshot      `json:"pricingSnapshot,omitempty"`
	Currency                 string                `json:"currency,omitempty"`
	ProviderRequestID        string                `json:"providerRequestId,omitempty"`
	RateLimitObservation     *RateLimitObservation `json:"rateLimitObservation,omitempty"`
	LatencyMS                int64                 `json:"latencyMs"`
	Status                   string                `json:"status"`
	ErrorCode                string                `json:"errorCode,omitempty"`
	CompletedAt              string                `json:"completedAt"`
}

type RateLimitObservation struct {
	RequestLimit     string `json:"requestLimit,omitempty"`
	RequestRemaining string `json:"requestRemaining,omitempty"`
	RequestReset     string `json:"requestReset,omitempty"`
	TokenLimit       string `json:"tokenLimit,omitempty"`
	TokenRemaining   string `json:"tokenRemaining,omitempty"`
	TokenReset       string `json:"tokenReset,omitempty"`
	ObservedAt       string `json:"observedAt"`
}

type PricingSnapshot struct {
	SnapshotID              string  `json:"snapshotId"`
	ProviderID              string  `json:"providerId"`
	ModelID                 string  `json:"modelId"`
	Currency                string  `json:"currency"`
	UnitTokens              int     `json:"unitTokens"`
	CachedInputPerUnitUSD   float64 `json:"cachedInputPerUnitUsd"`
	UncachedInputPerUnitUSD float64 `json:"uncachedInputPerUnitUsd"`
	OutputPerUnitUSD        float64 `json:"outputPerUnitUsd"`
	EffectiveDate           string  `json:"effectiveDate"`
	SourceURL               string  `json:"sourceUrl"`
}

type RawModelUsage struct {
	NormalizerID             string `json:"normalizerId"`
	PromptTokens             *int   `json:"promptTokens,omitempty"`
	InputTokens              *int   `json:"inputTokens,omitempty"`
	CompletionTokens         *int   `json:"completionTokens,omitempty"`
	OutputTokens             *int   `json:"outputTokens,omitempty"`
	ReasoningTokens          *int   `json:"reasoningTokens,omitempty"`
	TotalTokens              *int   `json:"totalTokens,omitempty"`
	CachedTokens             *int   `json:"cachedTokens,omitempty"`
	PromptCacheHitTokens     *int   `json:"promptCacheHitTokens,omitempty"`
	PromptCacheMissTokens    *int   `json:"promptCacheMissTokens,omitempty"`
	CacheReadInputTokens     *int   `json:"cacheReadInputTokens,omitempty"`
	CacheCreationInputTokens *int   `json:"cacheCreationInputTokens,omitempty"`
}

type Event struct {
	Type EventType `json:"type"`
	Seq  uint64    `json:"seq,omitempty"`
	// StreamCursor is the live transport resume cursor. It is intentionally
	// independent from Seq, which is the stable semantic frame sequence for
	// durable records.
	StreamCursor                     uint64                      `json:"streamCursor,omitempty"`
	SemanticSeq                      uint64                      `json:"semanticSeq,omitempty"`
	RecordID                         string                      `json:"recordId,omitempty"`
	Timestamp                        int64                       `json:"timestamp,omitempty"`
	ThreadID                         string                      `json:"threadId,omitempty"`
	RunID                            string                      `json:"runId,omitempty"`
	TurnID                           string                      `json:"turnId,omitempty"`
	QueryChainID                     string                      `json:"queryChainId,omitempty"`
	Depth                            int                         `json:"depth,omitempty"`
	TurnCount                        int                         `json:"turnCount,omitempty"`
	Status                           string                      `json:"status,omitempty"`
	MessageID                        string                      `json:"messageId,omitempty"`
	ParentRunID                      string                      `json:"parentRunId,omitempty"`
	ParentMessageID                  string                      `json:"parentMessageId,omitempty"`
	Role                             string                      `json:"role,omitempty"`
	Name                             string                      `json:"name,omitempty"`
	Delta                            string                      `json:"delta,omitempty"`
	ToolCallID                       string                      `json:"toolCallId,omitempty"`
	ToolCallName                     string                      `json:"toolCallName,omitempty"`
	ToolPreparationKind              string                      `json:"toolPreparationKind,omitempty"`
	ToolPreparationPath              string                      `json:"toolPreparationPath,omitempty"`
	ToolPreparationAddedLines        int                         `json:"toolPreparationAddedLines,omitempty"`
	ToolPreparationDeletedLines      int                         `json:"toolPreparationDeletedLines,omitempty"`
	Content                          string                      `json:"content,omitempty"`
	Result                           string                      `json:"result,omitempty"`
	Progress                         string                      `json:"progress,omitempty"`
	CommandStream                    string                      `json:"commandStream,omitempty"`
	Patch                            string                      `json:"patch,omitempty"`
	PatchSummary                     string                      `json:"patchSummary,omitempty"`
	Diff                             string                      `json:"diff,omitempty"`
	Truncated                        bool                        `json:"truncated,omitempty"`
	Message                          string                      `json:"message,omitempty"`
	Code                             string                      `json:"code,omitempty"`
	TerminalReason                   string                      `json:"terminalReason,omitempty"`
	TransitionReason                 string                      `json:"transitionReason,omitempty"`
	ContextModelWindow               int                         `json:"contextModelWindow,omitempty"`
	ContextOutputReserve             int                         `json:"contextOutputReserve,omitempty"`
	ContextEffectiveWindow           int                         `json:"contextEffectiveWindow,omitempty"`
	ContextEstimatedTokens           int                         `json:"contextEstimatedTokens,omitempty"`
	ContextActualTokens              int                         `json:"contextActualTokens,omitempty"`
	ContextWarningThreshold          int                         `json:"contextWarningThreshold,omitempty"`
	ContextAutoCompactThreshold      int                         `json:"contextAutoCompactThreshold,omitempty"`
	ContextCompactTarget             int                         `json:"contextCompactTarget,omitempty"`
	ContextEstimatedDeltaTokens      int                         `json:"contextEstimatedDeltaTokens,omitempty"`
	ContextUsageSnapshot             *ContextUsageSnapshot       `json:"contextUsageSnapshot,omitempty"`
	InstructionSnapshot              *InstructionSnapshotSummary `json:"instructionSnapshot,omitempty"`
	ModelUsage                       *ModelUsageRecord           `json:"modelUsage,omitempty"`
	ContextCompactMode               string                      `json:"contextCompactMode,omitempty"`
	ContextCompactBoundaryID         string                      `json:"contextCompactBoundaryId,omitempty"`
	ContextCompactPreEstimate        *int                        `json:"contextCompactPreEstimateTokens,omitempty"`
	ContextCompactPostEstimate       *int                        `json:"contextCompactPostEstimateTokens,omitempty"`
	CompactMode                      string                      `json:"compactMode,omitempty"`
	CompactBoundaryID                string                      `json:"compactBoundaryId,omitempty"`
	CompactPreEstimate               *int                        `json:"compactPreEstimate,omitempty"`
	CompactPostEstimate              *int                        `json:"compactPostEstimate,omitempty"`
	CompactSummaryModel              string                      `json:"compactSummaryModel,omitempty"`
	CompactFallbackUsed              *bool                       `json:"compactFallbackUsed,omitempty"`
	CompactToolResultsPruned         *bool                       `json:"compactToolResultsPruned,omitempty"`
	CompactToolResultsPrunedCount    int                         `json:"compactToolResultsPrunedCount,omitempty"`
	CompactPrunedSafeGroupCount      int                         `json:"compactPrunedSafeGroupCount,omitempty"`
	CompactPrunedMessageCount        int                         `json:"compactPrunedMessageCount,omitempty"`
	CompactSummaryMaxTokens          int                         `json:"compactSummaryMaxTokens,omitempty"`
	CompactSummaryRetryCount         int                         `json:"compactSummaryRetryCount,omitempty"`
	CompactSummaryRawChars           int                         `json:"compactSummaryRawChars,omitempty"`
	CompactSummaryParsedSections     int                         `json:"compactSummaryParsedSections,omitempty"`
	CompactSummaryMeaningfulSections int                         `json:"compactSummaryMeaningfulSections,omitempty"`
	CompactSummaryQualityIssue       string                      `json:"compactSummaryQualityIssue,omitempty"`
	CompactSummarySourceHadUser      *bool                       `json:"compactSummarySourceHadUser,omitempty"`
	CompactSummaryLatestUserPinned   *bool                       `json:"compactSummaryLatestUserPinned,omitempty"`
	CompactFallbackMarker            string                      `json:"compactFallbackMarker,omitempty"`
	CompactFailureReason             string                      `json:"compactFailureReason,omitempty"`
	CompactMandatoryFloor            int                         `json:"compactMandatoryFloor,omitempty"`
	CompactLocalProjected            int                         `json:"compactLocalProjected,omitempty"`
	CompactConservativeProjected     int                         `json:"compactConservativeProjected,omitempty"`
	CompactContributionTokens        map[string]int              `json:"compactContributionTokens,omitempty"`
	CompactNativeAttempted           *bool                       `json:"compactNativeAttempted,omitempty"`
	CompactNativeUsed                *bool                       `json:"compactNativeUsed,omitempty"`
	CompactNativeReason              string                      `json:"compactNativeReason,omitempty"`
	Memories                         []MemoryReceipt             `json:"memories,omitempty"`
	StaticPermissionBehavior         string                      `json:"staticPermissionBehavior,omitempty"`
	StaticPermissionReason           string                      `json:"staticPermissionReason,omitempty"`
	ToolDecisionAction               string                      `json:"toolDecisionAction,omitempty"`
	ToolDecisionSource               string                      `json:"toolDecisionSource,omitempty"`
	ToolDecisionReason               string                      `json:"toolDecisionReason,omitempty"`
	// ProfileName is the ExecutionProfile name applied to this call (Task 16
	// unified observability). Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	ProfileName string `json:"profileName,omitempty"`
	// TransformedFrom is the original tool name when the decision was a
	// transform. Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	TransformedFrom string `json:"transformedFrom,omitempty"`
	// TransformedTo is the rewritten tool name when the decision was a
	// transform. Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	TransformedTo string `json:"transformedTo,omitempty"`
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. The paths come from the structured tool outcome and
	// are carried on the ToolResult event after execution succeeds.
	TrashedPaths []string `json:"trashedPaths,omitempty"`
	// GuardianViolation is populated when a guardian constraint was
	// violated. Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	GuardianViolation          string           `json:"guardianViolation,omitempty"`
	ToolProgressStage          string           `json:"toolProgressStage,omitempty"`
	ToolProgressMessage        string           `json:"toolProgressMessage,omitempty"`
	ToolProgressPercentage     int              `json:"toolProgressPercentage,omitempty"`
	ToolStatus                 string           `json:"toolStatus,omitempty"`
	TransportStatus            string           `json:"transportStatus,omitempty"`
	SemanticStatus             string           `json:"semanticStatus,omitempty"`
	ProcessStatus              string           `json:"processStatus,omitempty"`
	ProcessExitCode            *int             `json:"processExitCode,omitempty"`
	ProcessTimedOut            bool             `json:"processTimedOut,omitempty"`
	ErrorCode                  string           `json:"errorCode,omitempty"`
	ErrorCategory              string           `json:"errorCategory,omitempty"`
	Retryable                  bool             `json:"retryable,omitempty"`
	RecoveryOptions            []string         `json:"recoveryOptions,omitempty"`
	DurationMs                 int64            `json:"durationMs,omitempty"`
	Diagnostic                 map[string]any   `json:"diagnostic,omitempty"`
	EnvironmentSnapshotID      string           `json:"environmentSnapshotId,omitempty"`
	ToolAvailabilitySnapshotID string           `json:"toolAvailabilitySnapshotId,omitempty"`
	ProviderID                 string           `json:"providerId,omitempty"`
	ProviderSource             string           `json:"providerSource,omitempty"`
	Capability                 string           `json:"capability,omitempty"`
	Capabilities               []string         `json:"capabilities,omitempty"`
	Readiness                  string           `json:"readiness,omitempty"`
	Dependency                 map[string]any   `json:"dependency,omitempty"`
	Dependencies               []map[string]any `json:"dependencies,omitempty"`
	SideEffectState            string           `json:"sideEffectState,omitempty"`
	AffectedPaths              []string         `json:"affectedPaths,omitempty"`
	AuthProvider               string           `json:"authProvider,omitempty"`
	AuthURL                    string           `json:"authUrl,omitempty"`
	RuntimeName                string           `json:"runtimeName,omitempty"`
	RuntimeVersion             string           `json:"runtimeVersion,omitempty"`
	SkillID                    string           `json:"skillId,omitempty"`
	SkillVersion               string           `json:"skillVersion,omitempty"`
	SkillSource                string           `json:"skillSource,omitempty"`
	SkillPath                  string           `json:"skillPath,omitempty"`
	SkillStatus                string           `json:"skillStatus,omitempty"`
	SkillResource              string           `json:"skillResource,omitempty"`
	SkillResourceSource        string           `json:"skillResourceSource,omitempty"`
	SkillResourceSHA256        string           `json:"skillResourceSha256,omitempty"`
	SkillResourceSize          int64            `json:"skillResourceSize,omitempty"`
	SkillResourceMode          string           `json:"skillResourceMode,omitempty"`
	SkillEffectiveTools        []string         `json:"skillEffectiveTools,omitempty"`
	SkillDiagnostics           []string         `json:"skillDiagnostics,omitempty"`
	HookInvocationID           string           `json:"hookInvocationId,omitempty"`
	HookID                     string           `json:"hookId,omitempty"`
	HookEvent                  string           `json:"hookEvent,omitempty"`
	HookSourceKind             string           `json:"hookSourceKind,omitempty"`
	HookAction                 string           `json:"hookAction,omitempty"`
	HookReasonCode             string           `json:"hookReasonCode,omitempty"`
	HookReason                 string           `json:"hookReason,omitempty"`
	HookOutputPreview          string           `json:"hookOutputPreview,omitempty"`
	QuestionID                 string           `json:"questionId,omitempty"`
	QuestionAnswer             string           `json:"questionAnswer,omitempty"`
	QuestionResolution         string           `json:"questionResolution,omitempty"`
	InteractionID              string           `json:"interactionId,omitempty"`
	InteractionKind            string           `json:"interactionKind,omitempty"`
	InteractionResolution      string           `json:"interactionResolution,omitempty"`
	InteractionTitle           string           `json:"interactionTitle,omitempty"`
	InteractionSafeMessage     string           `json:"interactionSafeMessage,omitempty"`
	InteractionVerificationURL string           `json:"interactionVerificationUrl,omitempty"`
	InteractionQRImageURL      string           `json:"interactionQrImageUrl,omitempty"`
	InteractionProvider        string           `json:"interactionProvider,omitempty"`
	InteractionRecoveries      []string         `json:"interactionRecoveries,omitempty"`
	InteractionFormSchema      map[string]any   `json:"interactionFormSchema,omitempty"`
	// Native run-scoped Subagent fields.
	SubagentTaskID           string `json:"subagentTaskId,omitempty"`
	SubagentAgentID          string `json:"subagentAgentId,omitempty"`
	SubagentParentAgentID    string `json:"subagentParentAgentId,omitempty"`
	SubagentParentToolCallID string `json:"subagentParentToolCallId,omitempty"`
	SubagentAgentType        string `json:"subagentAgentType,omitempty"`
	SubagentDescription      string `json:"subagentDescription,omitempty"`
	SubagentStatus           string `json:"subagentStatus,omitempty"`
	SubagentWorkspaceMode    string `json:"subagentWorkspaceMode,omitempty"`
	SubagentModelProfileID   string `json:"subagentModelProfileId,omitempty"`
	SubagentActivity         string `json:"subagentActivity,omitempty"`
	SubagentToolUseCount     int    `json:"subagentToolUseCount,omitempty"`
	SubagentEstimatedTokens  int    `json:"subagentEstimatedTokens,omitempty"`
	SubagentInputTokens      int    `json:"subagentInputTokens,omitempty"`
	SubagentOutputTokens     int    `json:"subagentOutputTokens,omitempty"`
	SubagentTotalTokens      int    `json:"subagentTotalTokens,omitempty"`
	SubagentResultPreview    string `json:"subagentResultPreview,omitempty"`
	SubagentErrorCode        string `json:"subagentErrorCode,omitempty"`
	SubagentErrorMessage     string `json:"subagentErrorMessage,omitempty"`
	SubagentSideEffectState  string `json:"subagentSideEffectState,omitempty"`
	SubagentCreatedAt        string `json:"subagentCreatedAt,omitempty"`
	SubagentStartedAt        string `json:"subagentStartedAt,omitempty"`
	SubagentFinishedAt       string `json:"subagentFinishedAt,omitempty"`
	// ACP fields surfaced on AWAKEN_ACP_* events.
	ACPSessionID            string                `json:"acpSessionId,omitempty"`
	ACPRunnerID             string                `json:"acpRunnerId,omitempty"`
	ACPTurnID               string                `json:"acpTurnId,omitempty"`
	ACPToolCallID           string                `json:"acpToolCallId,omitempty"`
	ACPPermissionOptions    []ACPPermissionOption `json:"acpPermissionOptions,omitempty"`
	ArtifactContractVersion int                   `json:"artifactContractVersion,omitempty"`
	ArtifactRole            string                `json:"artifactRole,omitempty"`
	ArtifactProducer        string                `json:"artifactProducer,omitempty"`
	ArtifactID              string                `json:"artifactId,omitempty"`
	ArtifactGroupID         string                `json:"artifactGroupId,omitempty"`
	ArtifactGroupIndex      int                   `json:"artifactGroupIndex,omitempty"`
	ArtifactTitle           string                `json:"artifactTitle,omitempty"`
	ArtifactDescription     string                `json:"artifactDescription,omitempty"`
	ArtifactKind            string                `json:"artifactKind,omitempty"`
	ArtifactPath            string                `json:"artifactPath,omitempty"`
	ArtifactContent         string                `json:"artifactContent,omitempty"`
	ArtifactStatus          string                `json:"artifactStatus,omitempty"`
	// Scheduled-task fixed-reminder fields, surfaced on AWAKEN_SCHEDULED_NOTICE
	// (contract §9.4). They carry only task/execution identity, the task name
	// and the fixed notice content; JSON keys follow the existing camelCase
	// wire convention consumed by apps/desktop/src/shared/agui.ts.
	TaskID        string `json:"taskId,omitempty"`
	ExecutionID   string `json:"executionId,omitempty"`
	TaskName      string `json:"taskName,omitempty"`
	NoticeContent string `json:"noticeContent,omitempty"`
	// Run-plan full-snapshot fields, surfaced on AWAKEN_PLAN_UPDATED (contract
	// 运行计划与任务进度设计契约 §8.1). toolCallId/threadId/runId/turnId/
	// recordId/semanticSeq reuse the existing Event identity fields above.
	PlanID                string               `json:"planId,omitempty"`
	PlanVersion           uint64               `json:"planVersion,omitempty"`
	PlanExplanation       string               `json:"planExplanation,omitempty"`
	PlanItems             []RunPlanItemReceipt `json:"planItems,omitempty"`
	PlanCompletedCount    int                  `json:"planCompletedCount,omitempty"`
	PlanTotalCount        int                  `json:"planTotalCount,omitempty"`
	PlanCurrentStep       int                  `json:"planCurrentStep,omitempty"`
	PlanAllCompleted      bool                 `json:"planAllCompleted,omitempty"`
	ResearchPhase         string               `json:"researchPhase,omitempty"`
	ResearchRevision      int                  `json:"researchRevision,omitempty"`
	ResearchQuestionCount int                  `json:"researchQuestionCount,omitempty"`
	ResearchClaimCount    int                  `json:"researchClaimCount,omitempty"`
	ResearchVerifiedCount int                  `json:"researchVerifiedCount,omitempty"`
	ResearchPartialCount  int                  `json:"researchPartialCount,omitempty"`
	ResearchRejectedCount int                  `json:"researchRejectedCount,omitempty"`
	Outcome               *RunOutcome          `json:"outcome,omitempty"`
	RawType               string               `json:"rawType,omitempty"`
	RawEvent              any                  `json:"rawEvent,omitempty"`
	// Computer Use review/handoff fields, surfaced on AWAKEN_COMPUTER_* events.
	ComputerReviewID           string   `json:"computerReviewId,omitempty"`
	ComputerActionID           string   `json:"computerActionId,omitempty"`
	ComputerActionDigest       string   `json:"computerActionDigest,omitempty"`
	ComputerTargetSummary      string   `json:"computerTargetSummary,omitempty"`
	ComputerEffectSummary      string   `json:"computerEffectSummary,omitempty"`
	ComputerRisk               string   `json:"computerRisk,omitempty"`
	ComputerSessionID          string   `json:"computerSessionId,omitempty"`
	ComputerSessionState       string   `json:"computerSessionState,omitempty"`
	ComputerVerificationStatus string   `json:"computerVerificationStatus,omitempty"`
	ComputerObservationID      string   `json:"computerObservationId,omitempty"`
	ComputerDecision           string   `json:"computerDecision,omitempty"`
	ComputerActor              string   `json:"computerActor,omitempty"`
	ComputerHandoff            bool     `json:"computerHandoff,omitempty"`
	ComputerEvidenceRefs       []string `json:"computerEvidenceRefs,omitempty"`
	// ProviderReconnect is live-only. Replay and SemanticJournal must not
	// reconstruct this frame from durable records.
	ProviderReconnect *ProviderReconnectPayload `json:"providerReconnect,omitempty"`
}
