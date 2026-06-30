package agui

type EventType string

const (
	EventRunStarted              EventType = "RUN_STARTED"
	EventRunFinished             EventType = "RUN_FINISHED"
	EventRunError                EventType = "RUN_ERROR"
	EventRunStatusChanged        EventType = "RUN_STATUS_CHANGED"
	EventTextMessageStart        EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent      EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd          EventType = "TEXT_MESSAGE_END"
	EventReasoningStart          EventType = "REASONING_START"
	EventReasoningMessageStart   EventType = "REASONING_MESSAGE_START"
	EventReasoningMessageContent EventType = "REASONING_MESSAGE_CONTENT"
	EventReasoningMessageEnd     EventType = "REASONING_MESSAGE_END"
	EventReasoningEnd            EventType = "REASONING_END"
	EventToolCallStart           EventType = "TOOL_CALL_START"
	EventToolCallArgsDelta       EventType = "TOOL_CALL_ARGS_DELTA"
	EventToolCallArgs            EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd             EventType = "TOOL_CALL_END"
	EventToolProgress            EventType = "TOOL_PROGRESS"
	EventToolCallResult          EventType = "TOOL_CALL_RESULT"
	EventAwakenCommandOutput     EventType = "AWAKEN_COMMAND_OUTPUT"
	EventAwakenPatchUpdated      EventType = "AWAKEN_PATCH_UPDATED"
	EventAwakenTurnDiff          EventType = "AWAKEN_TURN_DIFF"
	EventAwakenApprovalRequested EventType = "AWAKEN_APPROVAL_REQUESTED"
	EventAwakenApprovalResolved  EventType = "AWAKEN_APPROVAL_RESOLVED"
	EventAwakenArtifactCreated   EventType = "AWAKEN_ARTIFACT_CREATED"
	EventAwakenArtifactUpdated   EventType = "AWAKEN_ARTIFACT_UPDATED"
)

type RunOutcome struct {
	Type string `json:"type"`
}

type Event struct {
	Type                  EventType   `json:"type"`
	Seq                   uint64      `json:"seq,omitempty"`
	Timestamp             int64       `json:"timestamp,omitempty"`
	ThreadID              string      `json:"threadId,omitempty"`
	RunID                 string      `json:"runId,omitempty"`
	Status                string      `json:"status,omitempty"`
	MessageID             string      `json:"messageId,omitempty"`
	ParentRunID           string      `json:"parentRunId,omitempty"`
	ParentMessageID       string      `json:"parentMessageId,omitempty"`
	Role                  string      `json:"role,omitempty"`
	Name                  string      `json:"name,omitempty"`
	Delta                 string      `json:"delta,omitempty"`
	ToolCallID            string      `json:"toolCallId,omitempty"`
	ToolCallName          string      `json:"toolCallName,omitempty"`
	Content               string      `json:"content,omitempty"`
	Result                string      `json:"result,omitempty"`
	Progress              string      `json:"progress,omitempty"`
	CommandStream         string      `json:"commandStream,omitempty"`
	Patch                 string      `json:"patch,omitempty"`
	PatchSummary          string      `json:"patchSummary,omitempty"`
	Diff                  string      `json:"diff,omitempty"`
	Truncated             bool        `json:"truncated,omitempty"`
	Message               string      `json:"message,omitempty"`
	Code                  string      `json:"code,omitempty"`
	ApprovalID            string      `json:"approvalId,omitempty"`
	ApprovalTitle         string      `json:"approvalTitle,omitempty"`
	ApprovalDescription   string      `json:"approvalDescription,omitempty"`
	ApprovalRiskTone      string      `json:"approvalRiskTone,omitempty"`
	ApprovalDecision      string      `json:"approvalDecision,omitempty"`
	ApprovalAffectedPaths []string    `json:"approvalAffectedPaths,omitempty"`
	ApprovalPatchSummary  string      `json:"approvalPatchSummary,omitempty"`
	ArtifactID            string      `json:"artifactId,omitempty"`
	ArtifactTitle         string      `json:"artifactTitle,omitempty"`
	ArtifactDescription   string      `json:"artifactDescription,omitempty"`
	ArtifactKind          string      `json:"artifactKind,omitempty"`
	ArtifactPath          string      `json:"artifactPath,omitempty"`
	ArtifactContent       string      `json:"artifactContent,omitempty"`
	ArtifactStatus        string      `json:"artifactStatus,omitempty"`
	Outcome               *RunOutcome `json:"outcome,omitempty"`
}
