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
	EventAwakenApprovalRequested     EventType = "AWAKEN_APPROVAL_REQUESTED"
	EventAwakenApprovalResolved      EventType = "AWAKEN_APPROVAL_RESOLVED"
	EventAwakenArtifactCreated       EventType = "AWAKEN_ARTIFACT_CREATED"
	EventAwakenArtifactUpdated       EventType = "AWAKEN_ARTIFACT_UPDATED"
	EventAwakenUnknown               EventType = "AWAKEN_UNKNOWN_EVENT"
)

type RunOutcome struct {
	Type string `json:"type"`
}

type Event struct {
	Type                        EventType   `json:"type"`
	Seq                         uint64      `json:"seq,omitempty"`
	Timestamp                   int64       `json:"timestamp,omitempty"`
	ThreadID                    string      `json:"threadId,omitempty"`
	RunID                       string      `json:"runId,omitempty"`
	TurnID                      string      `json:"turnId,omitempty"`
	QueryChainID                string      `json:"queryChainId,omitempty"`
	Depth                       int         `json:"depth,omitempty"`
	TurnCount                   int         `json:"turnCount,omitempty"`
	Status                      string      `json:"status,omitempty"`
	MessageID                   string      `json:"messageId,omitempty"`
	ParentRunID                 string      `json:"parentRunId,omitempty"`
	ParentMessageID             string      `json:"parentMessageId,omitempty"`
	Role                        string      `json:"role,omitempty"`
	Name                        string      `json:"name,omitempty"`
	Delta                       string      `json:"delta,omitempty"`
	ToolCallID                  string      `json:"toolCallId,omitempty"`
	ToolCallName                string      `json:"toolCallName,omitempty"`
	Content                     string      `json:"content,omitempty"`
	Result                      string      `json:"result,omitempty"`
	Progress                    string      `json:"progress,omitempty"`
	CommandStream               string      `json:"commandStream,omitempty"`
	Patch                       string      `json:"patch,omitempty"`
	PatchSummary                string      `json:"patchSummary,omitempty"`
	Diff                        string      `json:"diff,omitempty"`
	Truncated                   bool        `json:"truncated,omitempty"`
	Message                     string      `json:"message,omitempty"`
	Code                        string      `json:"code,omitempty"`
	TerminalReason              string      `json:"terminalReason,omitempty"`
	TransitionReason            string      `json:"transitionReason,omitempty"`
	ContextModelWindow          int         `json:"contextModelWindow,omitempty"`
	ContextOutputReserve        int         `json:"contextOutputReserve,omitempty"`
	ContextEffectiveWindow      int         `json:"contextEffectiveWindow,omitempty"`
	ContextEstimatedTokens      int         `json:"contextEstimatedTokens,omitempty"`
	ContextWarningThreshold     int         `json:"contextWarningThreshold,omitempty"`
	ContextAutoCompactThreshold int         `json:"contextAutoCompactThreshold,omitempty"`
	ContextCompactTarget        int         `json:"contextCompactTarget,omitempty"`
	ContextEstimatedDeltaTokens int         `json:"contextEstimatedDeltaTokens,omitempty"`
	StaticPermissionBehavior    string      `json:"staticPermissionBehavior,omitempty"`
	StaticPermissionReason      string      `json:"staticPermissionReason,omitempty"`
	ToolDecisionAction          string      `json:"toolDecisionAction,omitempty"`
	ToolDecisionSource          string      `json:"toolDecisionSource,omitempty"`
	ToolDecisionReason          string      `json:"toolDecisionReason,omitempty"`
	ApprovalID                  string      `json:"approvalId,omitempty"`
	ApprovalTitle               string      `json:"approvalTitle,omitempty"`
	ApprovalDescription         string      `json:"approvalDescription,omitempty"`
	ApprovalRiskTone            string      `json:"approvalRiskTone,omitempty"`
	ApprovalDecision            string      `json:"approvalDecision,omitempty"`
	ApprovalAffectedPaths       []string    `json:"approvalAffectedPaths,omitempty"`
	ApprovalPatchSummary        string      `json:"approvalPatchSummary,omitempty"`
	ArtifactID                  string      `json:"artifactId,omitempty"`
	ArtifactTitle               string      `json:"artifactTitle,omitempty"`
	ArtifactDescription         string      `json:"artifactDescription,omitempty"`
	ArtifactKind                string      `json:"artifactKind,omitempty"`
	ArtifactPath                string      `json:"artifactPath,omitempty"`
	ArtifactContent             string      `json:"artifactContent,omitempty"`
	ArtifactStatus              string      `json:"artifactStatus,omitempty"`
	Outcome                     *RunOutcome `json:"outcome,omitempty"`
	RawType                     string      `json:"rawType,omitempty"`
	RawEvent                    any         `json:"rawEvent,omitempty"`
}
