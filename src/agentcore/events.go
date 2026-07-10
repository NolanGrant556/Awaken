package agentcore

import (
	"fmt"
	"sync/atomic"
	"time"

	"awaken/src/tool"
)

type AwakenEventType string

const (
	RunStartedEventType            AwakenEventType = "RunStarted"
	RunFinishedEventType           AwakenEventType = "RunFinished"
	RunErrorEventType              AwakenEventType = "RunError"
	TurnStartedEventType           AwakenEventType = "TurnStarted"
	TurnFinishedEventType          AwakenEventType = "TurnFinished"
	TextMessageStartedEventType    AwakenEventType = "TextMessageStarted"
	TextDeltaEventType             AwakenEventType = "TextDelta"
	TextMessageFinishedEventType   AwakenEventType = "TextMessageFinished"
	ReasoningStartedEventType      AwakenEventType = "ReasoningStarted"
	ReasoningDeltaEventType        AwakenEventType = "ReasoningDelta"
	ReasoningFinishedEventType     AwakenEventType = "ReasoningFinished"
	ToolCallStartedEventType       AwakenEventType = "ToolCallStarted"
	ToolCallArgsDeltaEventType     AwakenEventType = "ToolCallArgsDelta"
	ToolCallArgsEventType          AwakenEventType = "ToolCallArgs"
	ToolCallFinishedEventType      AwakenEventType = "ToolCallFinished"
	ToolProgressEventType          AwakenEventType = "ToolProgress"
	ToolPermissionDecidedEventType AwakenEventType = "ToolPermissionDecided"
	ToolResultEventType            AwakenEventType = "ToolResult"
	CommandOutputEventType         AwakenEventType = "CommandOutput"
	PatchUpdatedEventType          AwakenEventType = "PatchUpdated"
	TurnDiffEventType              AwakenEventType = "TurnDiff"
	ContextEstimatedEventType      AwakenEventType = "ContextEstimated"
	ContextWarningEventType        AwakenEventType = "ContextWarning"
	ContextBlockedEventType        AwakenEventType = "ContextBlocked"
	CompactStartedEventType        AwakenEventType = "CompactStarted"
	CompactFinishedEventType       AwakenEventType = "CompactFinished"
	CompactFailedEventType         AwakenEventType = "CompactFailed"
	ToolPairingRepairedEventType   AwakenEventType = "ToolPairingRepaired"
	AuthRequiredEventType          AwakenEventType = "AuthRequired"
	RuntimePrepareEventType        AwakenEventType = "RuntimePrepare"
	RuntimeReadyEventType          AwakenEventType = "RuntimeReady"
	RuntimeMissingEventType        AwakenEventType = "RuntimeMissing"
	MassMessageReviewRequestedType AwakenEventType = "MassMessageReviewRequested"
	MassMessageReviewResolvedType  AwakenEventType = "MassMessageReviewResolved"
	UserQuestionRequestedType      AwakenEventType = "UserQuestionRequested"
	UserQuestionResolvedType       AwakenEventType = "UserQuestionResolved"
	ArtifactCreatedEventType       AwakenEventType = "ArtifactCreated"
	ArtifactUpdatedEventType       AwakenEventType = "ArtifactUpdated"
)

type ReviewDecision string

const (
	ReviewDecisionApproved ReviewDecision = "approved"
	ReviewDecisionRejected ReviewDecision = "rejected"
)

type ArtifactStatus string

const (
	ArtifactStatusReady   ArtifactStatus = "ready"
	ArtifactStatusPending ArtifactStatus = "pending"
	ArtifactStatusError   ArtifactStatus = "error"
)

type AwakenEvent struct {
	Type                             AwakenEventType
	Timestamp                        time.Time
	ThreadID                         string
	RunID                            string
	TurnID                           string
	QueryChainID                     string
	Depth                            int
	TurnCount                        int
	MessageID                        string
	ParentMessageID                  string
	Role                             string
	Delta                            string
	Content                          string
	ToolCallID                       string
	ToolName                         string
	Arguments                        string
	Progress                         string
	CommandStream                    string
	Patch                            string
	PatchSummary                     string
	Diff                             string
	Truncated                        bool
	Error                            string
	Code                             string
	TerminalReason                   TerminalReason
	TransitionReason                 TransitionReason
	ContextModelWindow               int
	ContextOutputReserve             int
	ContextEffectiveWindow           int
	ContextEstimatedTokens           int
	ContextActualTokens              int
	ContextWarningThreshold          int
	ContextAutoCompactThreshold      int
	ContextCompactTarget             int
	ContextEstimatedDeltaTokens      int
	CompactMode                      CompactMode
	CompactBoundaryID                string
	CompactPreEstimate               *int
	CompactPostEstimate              *int
	CompactSummaryModel              string
	CompactFallbackUsed              *bool
	CompactToolResultsPruned         *bool
	CompactToolResultsPrunedCount    int
	CompactPrunedSafeGroupCount      int
	CompactPrunedMessageCount        int
	CompactSummaryMaxTokens          int
	CompactSummaryRetryCount         int
	CompactSummaryRawChars           int
	CompactSummaryParsedSections     int
	CompactSummaryMeaningfulSections int
	CompactSummaryQualityIssue       string
	CompactSummarySourceHadUser      *bool
	CompactSummaryLatestUserPinned   *bool
	CompactFallbackMarker            string
	CompactFailureReason             string
	StaticPermissionBehavior         string
	StaticPermissionReason           string
	ToolDecisionAction               string
	ToolDecisionSource               string
	ToolDecisionReason               string
	// ProfileName is the ExecutionProfile name applied to this call (Task 16
	// unified observability). Surfaced on ToolPermissionDecided events so the
	// UI can show "Profile: workspace_write" alongside the decision.
	ProfileName string
	// TransformedFrom is the original tool name when the decision was a
	// transform (e.g. "shell"). Empty for allow/deny.
	TransformedFrom string
	// TransformedTo is the rewritten tool name when the decision was a
	// transform (e.g. "apply_patch"). Empty for allow/deny.
	TransformedTo string
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. Surfaced on ToolPermissionDecided (for the audit
	// trail) and on ToolResult (for the user-facing trash notification).
	TrashedPaths []string
	// GuardianViolation is populated when a guardian constraint was
	// violated. Empty for non-guardian denials.
	GuardianViolation          string
	ToolProgressStage          string
	ToolProgressMessage        string
	ToolProgressPercentage     int
	// ToolOutcome fields (Phase 1). Canonical status lives in ToolStatus;
	// Desktop maps it to ToolDisplayState and must not infer from content.
	ToolStatus       string
	TransportStatus  string
	SemanticStatus   string
	ProcessStatus    string
	ProcessExitCode  *int
	ProcessTimedOut  bool
	ErrorCode        string
	ErrorCategory    string
	Retryable        bool
	RecoveryOptions  []string
	DurationMs       int64
	ToolDefinition   tool.ToolDefinition
	RiskHints        *tool.RiskHints
	AffectedPaths    []string
	AuthProvider               string
	AuthURL                    string
	RuntimeName                string
	RuntimeVersion             string
	ReviewID                   string
	ReviewDecision             ReviewDecision
	MassMessageChannel         string
	MassMessageRecipientScope  string
	MassMessageRecipientCount  int
	MassMessageBodyPreview     string
	MassMessageTemplatePreview string
	MassMessageAttachments     []string
	MassMessageSendingAccount  string
	MassMessageSourceTool      string
	QuestionID                 string
	QuestionAnswer             string
	ArtifactID                 string
	ArtifactTitle              string
	ArtifactDescription        string
	ArtifactKind               string
	ArtifactPath               string
	ArtifactContent            string
	ArtifactStatus             ArtifactStatus
}

var awakenIDCounter uint64

func newAwakenID(prefix string) string {
	seq := atomic.AddUint64(&awakenIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
