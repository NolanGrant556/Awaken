package agui

type EventType string

const (
	EventRunStarted                   EventType = "RUN_STARTED"
	EventRunFinished                  EventType = "RUN_FINISHED"
	EventRunError                     EventType = "RUN_ERROR"
	EventRunStatusChanged             EventType = "RUN_STATUS_CHANGED"
	EventAwakenTurnStarted            EventType = "AWAKEN_TURN_STARTED"
	EventAwakenTurnFinished           EventType = "AWAKEN_TURN_FINISHED"
	EventTextMessageStart             EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent           EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd               EventType = "TEXT_MESSAGE_END"
	EventReasoningStart               EventType = "REASONING_START"
	EventReasoningMessageStart        EventType = "REASONING_MESSAGE_START"
	EventReasoningMessageContent      EventType = "REASONING_MESSAGE_CONTENT"
	EventReasoningMessageEnd          EventType = "REASONING_MESSAGE_END"
	EventReasoningEnd                 EventType = "REASONING_END"
	EventToolCallStart                EventType = "TOOL_CALL_START"
	EventToolCallArgsDelta            EventType = "TOOL_CALL_ARGS_DELTA"
	EventToolCallArgs                 EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd                  EventType = "TOOL_CALL_END"
	EventToolProgress                 EventType = "TOOL_PROGRESS"
	EventToolCallResult               EventType = "TOOL_CALL_RESULT"
	EventAwakenToolPermissionDecided  EventType = "AWAKEN_TOOL_PERMISSION_DECIDED"
	EventAwakenCommandOutput          EventType = "AWAKEN_COMMAND_OUTPUT"
	EventAwakenPatchUpdated           EventType = "AWAKEN_PATCH_UPDATED"
	EventAwakenTurnDiff               EventType = "AWAKEN_TURN_DIFF"
	EventAwakenContextEstimated       EventType = "AWAKEN_CONTEXT_ESTIMATED"
	EventAwakenContextWarning         EventType = "AWAKEN_CONTEXT_WARNING"
	EventAwakenContextBlocked         EventType = "AWAKEN_CONTEXT_BLOCKED"
	EventAwakenCompactStarted         EventType = "AWAKEN_COMPACT_STARTED"
	EventAwakenCompactFinished        EventType = "AWAKEN_COMPACT_FINISHED"
	EventAwakenCompactFailed          EventType = "AWAKEN_COMPACT_FAILED"
	EventAwakenToolPairingRepaired    EventType = "AWAKEN_TOOL_PAIRING_REPAIRED"
	EventAwakenAuthRequired           EventType = "AWAKEN_AUTH_REQUIRED"
	EventAwakenRuntimePrepare         EventType = "AWAKEN_RUNTIME_PREPARE"
	EventAwakenRuntimePrepareProgress EventType = "AWAKEN_RUNTIME_PREPARE_PROGRESS"
	EventAwakenRuntimeReady           EventType = "AWAKEN_RUNTIME_READY"
	EventAwakenRuntimeRepairFailed    EventType = "AWAKEN_RUNTIME_REPAIR_FAILED"
	EventAwakenRuntimeMissing         EventType = "AWAKEN_RUNTIME_MISSING"
	EventAwakenSkillDiscovered        EventType = "AWAKEN_SKILL_DISCOVERED"
	EventAwakenSkillInvoked           EventType = "AWAKEN_SKILL_INVOKED"
	EventAwakenSkillContentLoaded     EventType = "AWAKEN_SKILL_CONTENT_LOADED"
	EventAwakenSkillExecutionContext  EventType = "AWAKEN_SKILL_EXECUTION_CONTEXT"
	EventAwakenSkillFailedToLoad      EventType = "AWAKEN_SKILL_FAILED_TO_LOAD"
	EventAwakenUserQuestionRequested         EventType = "AWAKEN_USER_QUESTION_REQUESTED"
	EventAwakenUserQuestionResolved          EventType = "AWAKEN_USER_QUESTION_RESOLVED"
	EventAwakenExternalInteractionRequested  EventType = "AWAKEN_EXTERNAL_INTERACTION_REQUESTED"
	EventAwakenExternalInteractionResolved   EventType = "AWAKEN_EXTERNAL_INTERACTION_RESOLVED"
	EventAwakenArtifactCreated        EventType = "AWAKEN_ARTIFACT_CREATED"
	EventAwakenArtifactUpdated        EventType = "AWAKEN_ARTIFACT_UPDATED"
	EventAwakenToolRecoveryRequired   EventType = "AWAKEN_TOOL_RECOVERY_REQUIRED"
	EventAwakenToolRetryBlocked       EventType = "AWAKEN_TOOL_RETRY_BLOCKED"
	EventAwakenUnknown                EventType = "AWAKEN_UNKNOWN_EVENT"
)

type RunOutcome struct {
	Type string `json:"type"`
}

type Event struct {
	Type EventType `json:"type"`
	Seq  uint64    `json:"seq,omitempty"`
	// StreamCursor is the live transport resume cursor. It is intentionally
	// independent from Seq, which is the stable semantic frame sequence for
	// durable records.
	StreamCursor                     uint64 `json:"streamCursor,omitempty"`
	SemanticSeq                      uint64 `json:"semanticSeq,omitempty"`
	RecordID                         string `json:"recordId,omitempty"`
	Timestamp                        int64  `json:"timestamp,omitempty"`
	ThreadID                         string `json:"threadId,omitempty"`
	RunID                            string `json:"runId,omitempty"`
	TurnID                           string `json:"turnId,omitempty"`
	QueryChainID                     string `json:"queryChainId,omitempty"`
	Depth                            int    `json:"depth,omitempty"`
	TurnCount                        int    `json:"turnCount,omitempty"`
	Status                           string `json:"status,omitempty"`
	MessageID                        string `json:"messageId,omitempty"`
	ParentRunID                      string `json:"parentRunId,omitempty"`
	ParentMessageID                  string `json:"parentMessageId,omitempty"`
	Role                             string `json:"role,omitempty"`
	Name                             string `json:"name,omitempty"`
	Delta                            string `json:"delta,omitempty"`
	ToolCallID                       string `json:"toolCallId,omitempty"`
	ToolCallName                     string `json:"toolCallName,omitempty"`
	Content                          string `json:"content,omitempty"`
	Result                           string `json:"result,omitempty"`
	Progress                         string `json:"progress,omitempty"`
	CommandStream                    string `json:"commandStream,omitempty"`
	Patch                            string `json:"patch,omitempty"`
	PatchSummary                     string `json:"patchSummary,omitempty"`
	Diff                             string `json:"diff,omitempty"`
	Truncated                        bool   `json:"truncated,omitempty"`
	Message                          string `json:"message,omitempty"`
	Code                             string `json:"code,omitempty"`
	TerminalReason                   string `json:"terminalReason,omitempty"`
	TransitionReason                 string `json:"transitionReason,omitempty"`
	ContextModelWindow               int    `json:"contextModelWindow,omitempty"`
	ContextOutputReserve             int    `json:"contextOutputReserve,omitempty"`
	ContextEffectiveWindow           int    `json:"contextEffectiveWindow,omitempty"`
	ContextEstimatedTokens           int    `json:"contextEstimatedTokens,omitempty"`
	ContextActualTokens              int    `json:"contextActualTokens,omitempty"`
	ContextWarningThreshold          int    `json:"contextWarningThreshold,omitempty"`
	ContextAutoCompactThreshold      int    `json:"contextAutoCompactThreshold,omitempty"`
	ContextCompactTarget             int    `json:"contextCompactTarget,omitempty"`
	ContextEstimatedDeltaTokens      int    `json:"contextEstimatedDeltaTokens,omitempty"`
	ContextCompactMode               string `json:"contextCompactMode,omitempty"`
	ContextCompactBoundaryID         string `json:"contextCompactBoundaryId,omitempty"`
	ContextCompactPreEstimate        *int   `json:"contextCompactPreEstimateTokens,omitempty"`
	ContextCompactPostEstimate       *int   `json:"contextCompactPostEstimateTokens,omitempty"`
	CompactMode                      string `json:"compactMode,omitempty"`
	CompactBoundaryID                string `json:"compactBoundaryId,omitempty"`
	CompactPreEstimate               *int   `json:"compactPreEstimate,omitempty"`
	CompactPostEstimate              *int   `json:"compactPostEstimate,omitempty"`
	CompactSummaryModel              string `json:"compactSummaryModel,omitempty"`
	CompactFallbackUsed              *bool  `json:"compactFallbackUsed,omitempty"`
	CompactToolResultsPruned         *bool  `json:"compactToolResultsPruned,omitempty"`
	CompactToolResultsPrunedCount    int    `json:"compactToolResultsPrunedCount,omitempty"`
	CompactPrunedSafeGroupCount      int    `json:"compactPrunedSafeGroupCount,omitempty"`
	CompactPrunedMessageCount        int    `json:"compactPrunedMessageCount,omitempty"`
	CompactSummaryMaxTokens          int    `json:"compactSummaryMaxTokens,omitempty"`
	CompactSummaryRetryCount         int    `json:"compactSummaryRetryCount,omitempty"`
	CompactSummaryRawChars           int    `json:"compactSummaryRawChars,omitempty"`
	CompactSummaryParsedSections     int    `json:"compactSummaryParsedSections,omitempty"`
	CompactSummaryMeaningfulSections int    `json:"compactSummaryMeaningfulSections,omitempty"`
	CompactSummaryQualityIssue       string `json:"compactSummaryQualityIssue,omitempty"`
	CompactSummarySourceHadUser      *bool  `json:"compactSummarySourceHadUser,omitempty"`
	CompactSummaryLatestUserPinned   *bool  `json:"compactSummaryLatestUserPinned,omitempty"`
	CompactFallbackMarker            string `json:"compactFallbackMarker,omitempty"`
	CompactFailureReason             string `json:"compactFailureReason,omitempty"`
	StaticPermissionBehavior         string `json:"staticPermissionBehavior,omitempty"`
	StaticPermissionReason           string `json:"staticPermissionReason,omitempty"`
	ToolDecisionAction               string `json:"toolDecisionAction,omitempty"`
	ToolDecisionSource               string `json:"toolDecisionSource,omitempty"`
	ToolDecisionReason               string `json:"toolDecisionReason,omitempty"`
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
	QuestionID                   string   `json:"questionId,omitempty"`
	QuestionAnswer               string   `json:"questionAnswer,omitempty"`
	InteractionID                string   `json:"interactionId,omitempty"`
	InteractionResolution        string   `json:"interactionResolution,omitempty"`
	InteractionTitle             string   `json:"interactionTitle,omitempty"`
	InteractionSafeMessage       string   `json:"interactionSafeMessage,omitempty"`
	InteractionVerificationURL   string   `json:"interactionVerificationUrl,omitempty"`
	InteractionQRImageURL        string   `json:"interactionQrImageUrl,omitempty"`
	InteractionProvider          string   `json:"interactionProvider,omitempty"`
	InteractionRecoveries        []string `json:"interactionRecoveries,omitempty"`
	ArtifactContractVersion      int      `json:"artifactContractVersion,omitempty"`
	ArtifactRole               string           `json:"artifactRole,omitempty"`
	ArtifactProducer           string           `json:"artifactProducer,omitempty"`
	ArtifactID                 string           `json:"artifactId,omitempty"`
	ArtifactTitle              string           `json:"artifactTitle,omitempty"`
	ArtifactDescription        string           `json:"artifactDescription,omitempty"`
	ArtifactKind               string           `json:"artifactKind,omitempty"`
	ArtifactPath               string           `json:"artifactPath,omitempty"`
	ArtifactContent            string           `json:"artifactContent,omitempty"`
	ArtifactStatus             string           `json:"artifactStatus,omitempty"`
	Outcome                    *RunOutcome      `json:"outcome,omitempty"`
	RawType                    string           `json:"rawType,omitempty"`
	RawEvent                   any              `json:"rawEvent,omitempty"`
}
