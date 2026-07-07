package agui

type EventType string

const (
	EventRunStarted                  EventType = "RUN_STARTED"
	EventRunFinished                 EventType = "RUN_FINISHED"
	EventRunError                    EventType = "RUN_ERROR"
	EventRunStatusChanged            EventType = "RUN_STATUS_CHANGED"
	EventAwakenTurnStarted           EventType = "AWAKEN_TURN_STARTED"
	EventAwakenTurnFinished          EventType = "AWAKEN_TURN_FINISHED"
	EventTextMessageStart            EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent          EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd              EventType = "TEXT_MESSAGE_END"
	EventReasoningStart              EventType = "REASONING_START"
	EventReasoningMessageStart       EventType = "REASONING_MESSAGE_START"
	EventReasoningMessageContent     EventType = "REASONING_MESSAGE_CONTENT"
	EventReasoningMessageEnd         EventType = "REASONING_MESSAGE_END"
	EventReasoningEnd                EventType = "REASONING_END"
	EventToolCallStart               EventType = "TOOL_CALL_START"
	EventToolCallArgsDelta           EventType = "TOOL_CALL_ARGS_DELTA"
	EventToolCallArgs                EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd                 EventType = "TOOL_CALL_END"
	EventToolProgress                EventType = "TOOL_PROGRESS"
	EventToolCallResult              EventType = "TOOL_CALL_RESULT"
	EventAwakenToolPermissionDecided EventType = "AWAKEN_TOOL_PERMISSION_DECIDED"
	EventAwakenCommandOutput         EventType = "AWAKEN_COMMAND_OUTPUT"
	EventAwakenPatchUpdated          EventType = "AWAKEN_PATCH_UPDATED"
	EventAwakenTurnDiff              EventType = "AWAKEN_TURN_DIFF"
	EventAwakenContextEstimated      EventType = "AWAKEN_CONTEXT_ESTIMATED"
	EventAwakenContextWarning        EventType = "AWAKEN_CONTEXT_WARNING"
	EventAwakenContextBlocked        EventType = "AWAKEN_CONTEXT_BLOCKED"
	EventAwakenCompactStarted        EventType = "AWAKEN_COMPACT_STARTED"
	EventAwakenCompactFinished       EventType = "AWAKEN_COMPACT_FINISHED"
	EventAwakenCompactFailed         EventType = "AWAKEN_COMPACT_FAILED"
	EventAwakenToolPairingRepaired   EventType = "AWAKEN_TOOL_PAIRING_REPAIRED"
	EventAwakenAuthRequired          EventType = "AWAKEN_AUTH_REQUIRED"
	EventAwakenRuntimePrepare        EventType = "AWAKEN_RUNTIME_PREPARE"
	EventAwakenRuntimeReady          EventType = "AWAKEN_RUNTIME_READY"
	EventAwakenRuntimeMissing        EventType = "AWAKEN_RUNTIME_MISSING"
	EventAwakenMassReviewRequested   EventType = "AWAKEN_MASS_MESSAGE_REVIEW_REQUESTED"
	EventAwakenMassReviewResolved    EventType = "AWAKEN_MASS_MESSAGE_REVIEW_RESOLVED"
	EventAwakenArtifactCreated       EventType = "AWAKEN_ARTIFACT_CREATED"
	EventAwakenArtifactUpdated       EventType = "AWAKEN_ARTIFACT_UPDATED"
	EventAwakenUnknown               EventType = "AWAKEN_UNKNOWN_EVENT"
)

type RunOutcome struct {
	Type string `json:"type"`
}

type Event struct {
	Type                             EventType `json:"type"`
	Seq                              uint64    `json:"seq,omitempty"`
	Timestamp                        int64     `json:"timestamp,omitempty"`
	ThreadID                         string    `json:"threadId,omitempty"`
	RunID                            string    `json:"runId,omitempty"`
	TurnID                           string    `json:"turnId,omitempty"`
	QueryChainID                     string    `json:"queryChainId,omitempty"`
	Depth                            int       `json:"depth,omitempty"`
	TurnCount                        int       `json:"turnCount,omitempty"`
	Status                           string    `json:"status,omitempty"`
	MessageID                        string    `json:"messageId,omitempty"`
	ParentRunID                      string    `json:"parentRunId,omitempty"`
	ParentMessageID                  string    `json:"parentMessageId,omitempty"`
	Role                             string    `json:"role,omitempty"`
	Name                             string    `json:"name,omitempty"`
	Delta                            string    `json:"delta,omitempty"`
	ToolCallID                       string    `json:"toolCallId,omitempty"`
	ToolCallName                     string    `json:"toolCallName,omitempty"`
	Content                          string    `json:"content,omitempty"`
	Result                           string    `json:"result,omitempty"`
	Progress                         string    `json:"progress,omitempty"`
	CommandStream                    string    `json:"commandStream,omitempty"`
	Patch                            string    `json:"patch,omitempty"`
	PatchSummary                     string    `json:"patchSummary,omitempty"`
	Diff                             string    `json:"diff,omitempty"`
	Truncated                        bool      `json:"truncated,omitempty"`
	Message                          string    `json:"message,omitempty"`
	Code                             string    `json:"code,omitempty"`
	TerminalReason                   string    `json:"terminalReason,omitempty"`
	TransitionReason                 string    `json:"transitionReason,omitempty"`
	ContextModelWindow               int       `json:"contextModelWindow,omitempty"`
	ContextOutputReserve             int       `json:"contextOutputReserve,omitempty"`
	ContextEffectiveWindow           int       `json:"contextEffectiveWindow,omitempty"`
	ContextEstimatedTokens           int       `json:"contextEstimatedTokens,omitempty"`
	ContextActualTokens              int       `json:"contextActualTokens,omitempty"`
	ContextWarningThreshold          int       `json:"contextWarningThreshold,omitempty"`
	ContextAutoCompactThreshold      int       `json:"contextAutoCompactThreshold,omitempty"`
	ContextCompactTarget             int       `json:"contextCompactTarget,omitempty"`
	ContextEstimatedDeltaTokens      int       `json:"contextEstimatedDeltaTokens,omitempty"`
	ContextCompactMode               string    `json:"contextCompactMode,omitempty"`
	ContextCompactBoundaryID         string    `json:"contextCompactBoundaryId,omitempty"`
	ContextCompactPreEstimate        *int      `json:"contextCompactPreEstimateTokens,omitempty"`
	ContextCompactPostEstimate       *int      `json:"contextCompactPostEstimateTokens,omitempty"`
	CompactMode                      string    `json:"compactMode,omitempty"`
	CompactBoundaryID                string    `json:"compactBoundaryId,omitempty"`
	CompactPreEstimate               *int      `json:"compactPreEstimate,omitempty"`
	CompactPostEstimate              *int      `json:"compactPostEstimate,omitempty"`
	CompactSummaryModel              string    `json:"compactSummaryModel,omitempty"`
	CompactFallbackUsed              *bool     `json:"compactFallbackUsed,omitempty"`
	CompactToolResultsPruned         *bool     `json:"compactToolResultsPruned,omitempty"`
	CompactToolResultsPrunedCount    int       `json:"compactToolResultsPrunedCount,omitempty"`
	CompactPrunedSafeGroupCount      int       `json:"compactPrunedSafeGroupCount,omitempty"`
	CompactPrunedMessageCount        int       `json:"compactPrunedMessageCount,omitempty"`
	CompactSummaryMaxTokens          int       `json:"compactSummaryMaxTokens,omitempty"`
	CompactSummaryRetryCount         int       `json:"compactSummaryRetryCount,omitempty"`
	CompactSummaryRawChars           int       `json:"compactSummaryRawChars,omitempty"`
	CompactSummaryParsedSections     int       `json:"compactSummaryParsedSections,omitempty"`
	CompactSummaryMeaningfulSections int       `json:"compactSummaryMeaningfulSections,omitempty"`
	CompactSummaryQualityIssue       string    `json:"compactSummaryQualityIssue,omitempty"`
	CompactSummarySourceHadUser      *bool     `json:"compactSummarySourceHadUser,omitempty"`
	CompactSummaryLatestUserPinned   *bool     `json:"compactSummaryLatestUserPinned,omitempty"`
	CompactFallbackMarker            string    `json:"compactFallbackMarker,omitempty"`
	CompactFailureReason             string    `json:"compactFailureReason,omitempty"`
	StaticPermissionBehavior         string    `json:"staticPermissionBehavior,omitempty"`
	StaticPermissionReason           string    `json:"staticPermissionReason,omitempty"`
	ToolDecisionAction               string    `json:"toolDecisionAction,omitempty"`
	ToolDecisionSource               string    `json:"toolDecisionSource,omitempty"`
	ToolDecisionReason               string    `json:"toolDecisionReason,omitempty"`
	// ProfileName is the ExecutionProfile name applied to this call (Task 16
	// unified observability). Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	ProfileName string `json:"profileName,omitempty"`
	// TransformedFrom is the original tool name when the decision was a
	// transform (e.g. "shell"). Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	TransformedFrom string `json:"transformedFrom,omitempty"`
	// TransformedTo is the rewritten tool name when the decision was a
	// transform (e.g. "apply_patch"). Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	TransformedTo string `json:"transformedTo,omitempty"`
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. The paths are parsed from the apply_patch output
	// after execution succeeds (see tool_runtime.extractTrashedPaths), so
	// they are carried on the ToolResult event (post-execution). The
	// AWAKEN_TOOL_PERMISSION_DECIDED event still declares the field for
	// backwards compatibility but it is always nil there.
	TrashedPaths []string `json:"trashedPaths,omitempty"`
	// GuardianViolation is populated when a guardian constraint was
	// violated. Surfaced on AWAKEN_TOOL_PERMISSION_DECIDED.
	GuardianViolation          string      `json:"guardianViolation,omitempty"`
	AffectedPaths              []string    `json:"affectedPaths,omitempty"`
	AuthProvider               string      `json:"authProvider,omitempty"`
	AuthURL                    string      `json:"authUrl,omitempty"`
	RuntimeName                string      `json:"runtimeName,omitempty"`
	RuntimeVersion             string      `json:"runtimeVersion,omitempty"`
	ReviewID                   string      `json:"reviewId,omitempty"`
	ReviewDecision             string      `json:"reviewDecision,omitempty"`
	MassMessageChannel         string      `json:"massMessageChannel,omitempty"`
	MassMessageRecipientScope  string      `json:"massMessageRecipientScope,omitempty"`
	MassMessageRecipientCount  int         `json:"massMessageRecipientCount,omitempty"`
	MassMessageBodyPreview     string      `json:"massMessageBodyPreview,omitempty"`
	MassMessageTemplatePreview string      `json:"massMessageTemplatePreview,omitempty"`
	MassMessageAttachments     []string    `json:"massMessageAttachments,omitempty"`
	MassMessageSendingAccount  string      `json:"massMessageSendingAccount,omitempty"`
	MassMessageSourceTool      string      `json:"massMessageSourceTool,omitempty"`
	ArtifactID                 string      `json:"artifactId,omitempty"`
	ArtifactTitle              string      `json:"artifactTitle,omitempty"`
	ArtifactDescription        string      `json:"artifactDescription,omitempty"`
	ArtifactKind               string      `json:"artifactKind,omitempty"`
	ArtifactPath               string      `json:"artifactPath,omitempty"`
	ArtifactContent            string      `json:"artifactContent,omitempty"`
	ArtifactStatus             string      `json:"artifactStatus,omitempty"`
	Outcome                    *RunOutcome `json:"outcome,omitempty"`
	RawType                    string      `json:"rawType,omitempty"`
	RawEvent                   any         `json:"rawEvent,omitempty"`
}
