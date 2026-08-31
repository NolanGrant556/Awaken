package agentcore

import (
	"fmt"
	"sync/atomic"
	"time"

	"awaken/src/tool"
)

type AwakenEventType string

const (
	RunStartedEventType              AwakenEventType = "RunStarted"
	RunFinishedEventType             AwakenEventType = "RunFinished"
	RunErrorEventType                AwakenEventType = "RunError"
	TurnStartedEventType             AwakenEventType = "TurnStarted"
	TurnFinishedEventType            AwakenEventType = "TurnFinished"
	TextMessageStartedEventType      AwakenEventType = "TextMessageStarted"
	TextDeltaEventType               AwakenEventType = "TextDelta"
	TextMessageFinishedEventType     AwakenEventType = "TextMessageFinished"
	ReasoningStartedEventType        AwakenEventType = "ReasoningStarted"
	ReasoningDeltaEventType          AwakenEventType = "ReasoningDelta"
	ReasoningFinishedEventType       AwakenEventType = "ReasoningFinished"
	ToolCallStartedEventType         AwakenEventType = "ToolCallStarted"
	ToolCallArgsDeltaEventType       AwakenEventType = "ToolCallArgsDelta"
	ToolCallArgsEventType            AwakenEventType = "ToolCallArgs"
	ToolCallFinishedEventType        AwakenEventType = "ToolCallFinished"
	ToolProgressEventType            AwakenEventType = "ToolProgress"
	ToolPermissionDecidedEventType   AwakenEventType = "ToolPermissionDecided"
	ToolResultEventType              AwakenEventType = "ToolResult"
	CommandOutputEventType           AwakenEventType = "CommandOutput"
	PatchUpdatedEventType            AwakenEventType = "PatchUpdated"
	TurnDiffEventType                AwakenEventType = "TurnDiff"
	ContextEstimatedEventType        AwakenEventType = "ContextEstimated"
	ContextWarningEventType          AwakenEventType = "ContextWarning"
	ContextBlockedEventType          AwakenEventType = "ContextBlocked"
	CompactStartedEventType          AwakenEventType = "CompactStarted"
	CompactFinishedEventType         AwakenEventType = "CompactFinished"
	CompactFailedEventType           AwakenEventType = "CompactFailed"
	ToolPairingRepairedEventType     AwakenEventType = "ToolPairingRepaired"
	AuthRequiredEventType            AwakenEventType = "AuthRequired"
	RuntimePrepareEventType          AwakenEventType = "RuntimePrepare"
	RuntimePrepareStartedEventType   AwakenEventType = "RuntimePrepareStarted"
	RuntimePrepareProgressEventType  AwakenEventType = "RuntimePrepareProgress"
	RuntimeReadyEventType            AwakenEventType = "RuntimeReady"
	RuntimeRepairFailedEventType     AwakenEventType = "RuntimeRepairFailed"
	RuntimeMissingEventType          AwakenEventType = "RuntimeMissing"
	UserQuestionRequestedType        AwakenEventType = "UserQuestionRequested"
	UserQuestionResolvedType         AwakenEventType = "UserQuestionResolved"
	ExternalInteractionRequestedType AwakenEventType = "ExternalInteractionRequested"
	ExternalInteractionResolvedType  AwakenEventType = "ExternalInteractionResolved"
	ArtifactCreatedEventType         AwakenEventType = "ArtifactCreated"
	ArtifactUpdatedEventType         AwakenEventType = "ArtifactUpdated"
	SkillActivatedEventType          AwakenEventType = "SkillActivated"
	ResourceReceiptEventType         AwakenEventType = "ResourceReceipt"
	ToolRecoveryRequiredEventType    AwakenEventType = "ToolRecoveryRequired"
	ToolRetryBlockedEventType        AwakenEventType = "ToolRetryBlocked"
	MemoryRecalledEventType          AwakenEventType = "MemoryRecalled"
	MemoryMutatedEventType           AwakenEventType = "MemoryMutated"
	ContextSnapshotEventType         AwakenEventType = "ContextSnapshot"
	ModelUsageEventType              AwakenEventType = "ModelUsage"
	ACPSessionStartedEventType       AwakenEventType = "ACPSessionStarted"
	ACPSessionClosedEventType        AwakenEventType = "ACPSessionClosed"
	ACPSessionInterruptedEventType   AwakenEventType = "ACPSessionInterrupted"
	ACPTurnStartedEventType          AwakenEventType = "ACPTurnStarted"
	ACPTurnFinishedEventType         AwakenEventType = "ACPTurnFinished"
	ACPToolSnapshotEventType         AwakenEventType = "ACPToolSnapshot"
	ACPPermissionRequestedEventType  AwakenEventType = "ACPPermissionRequested"
	ACPPermissionResolvedEventType   AwakenEventType = "ACPPermissionResolved"
	ProviderReconnectEventType       AwakenEventType = "ProviderReconnect"
)

const (
	ProviderReconnectWaiting   = "waiting_for_network"
	ProviderReconnectRetrying  = "reconnecting"
	ProviderReconnectRecovered = "recovered"
)

type ProviderReconnectStatus struct {
	Phase           string `json:"phase"`
	Attempt         int    `json:"attempt,omitempty"`
	MaxAttempts     int    `json:"maxAttempts,omitempty"`
	Detail          string `json:"detail,omitempty"`
	ResetInProgress bool   `json:"resetInProgress,omitempty"`
}

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
	AgentID                          string
	ParentAgentID                    string
	SubagentTaskID                   string
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
	ToolPreparationKind              string
	ToolPreparationPath              string
	ToolPreparationAddedLines        int
	ToolPreparationDeletedLines      int
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
	ContextUsageSnapshot             *ContextUsageSnapshot
	ModelUsage                       *ModelUsageRecord
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
	CompactMandatoryFloor            int
	CompactLocalProjected            int
	CompactConservativeProjected     int
	CompactContributionTokens        map[string]int
	CompactNativeAttempted           *bool
	CompactNativeUsed                *bool
	CompactNativeReason              string
	MemoryReceipts                   []MemoryRecallReceipt
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
	// transformed tool name, when a generic policy transform applies. Empty for allow/deny.
	TransformedTo string
	// TrashedPaths lists the absolute paths moved to the OS recycle bin
	// during this call. Surfaced on ToolPermissionDecided (for the audit
	// trail) and on ToolResult (for the user-facing trash notification).
	TrashedPaths []string
	// GuardianViolation is populated when a guardian constraint was
	// violated. Empty for non-guardian denials.
	GuardianViolation      string
	ToolProgressStage      string
	ToolProgressMessage    string
	ToolProgressPercentage int
	// ToolOutcome fields (Phase 1). Canonical status lives in ToolStatus;
	// Desktop maps it to ToolDisplayState and must not infer from content.
	ToolStatus                 string
	TransportStatus            string
	SemanticStatus             string
	ProcessStatus              string
	ProcessExitCode            *int
	ProcessTimedOut            bool
	ErrorCode                  string
	ErrorCategory              string
	Retryable                  bool
	RecoveryOptions            []string
	DurationMs                 int64
	Diagnostic                 map[string]any
	ToolDefinition             tool.ToolDefinition
	RiskHints                  *tool.RiskHints
	AffectedPaths              []string
	AuthProvider               string
	AuthURL                    string
	RuntimeName                string
	RuntimeVersion             string
	QuestionID                 string
	QuestionAnswer             string
	QuestionResolution         string
	InteractionID              string
	InteractionKind            string
	InteractionResolution      string
	InteractionTitle           string
	InteractionSafeMessage     string
	InteractionVerificationURL string
	InteractionQRImageURL      string
	InteractionProvider        string
	InteractionRecoveries      []string
	InteractionFormSchema      map[string]any
	InteractionRequiresReplay  bool
	ArtifactContractVersion    int
	ArtifactRole               tool.ArtifactRole
	ArtifactProducer           string
	ArtifactID                 string
	ArtifactGroupID            string
	ArtifactGroupIndex         int
	ArtifactTitle              string
	ArtifactDescription        string
	ArtifactKind               string
	ArtifactPath               string
	ArtifactContent            string
	ArtifactStatus             ArtifactStatus
	SkillID                    string
	SkillVersion               string
	ResourcePath               string
	ResourceSHA256             string
	ResourceSize               int64
	ResourceMode               string
	ResourceSource             string
	ProviderReconnect          *ProviderReconnectStatus
}

var awakenIDCounter uint64

func newAwakenID(prefix string) string {
	seq := atomic.AddUint64(&awakenIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
