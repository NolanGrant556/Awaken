package agentcore

import (
	"fmt"
	"sync/atomic"
	"time"
)

type AwakenEventType string

const (
	RunStartedEventType          AwakenEventType = "RunStarted"
	RunFinishedEventType         AwakenEventType = "RunFinished"
	RunErrorEventType            AwakenEventType = "RunError"
	TextMessageStartedEventType  AwakenEventType = "TextMessageStarted"
	TextDeltaEventType           AwakenEventType = "TextDelta"
	TextMessageFinishedEventType AwakenEventType = "TextMessageFinished"
	ReasoningStartedEventType    AwakenEventType = "ReasoningStarted"
	ReasoningDeltaEventType      AwakenEventType = "ReasoningDelta"
	ReasoningFinishedEventType   AwakenEventType = "ReasoningFinished"
	ToolCallStartedEventType     AwakenEventType = "ToolCallStarted"
	ToolCallArgsDeltaEventType   AwakenEventType = "ToolCallArgsDelta"
	ToolCallArgsEventType        AwakenEventType = "ToolCallArgs"
	ToolCallFinishedEventType    AwakenEventType = "ToolCallFinished"
	ToolProgressEventType        AwakenEventType = "ToolProgress"
	ToolResultEventType          AwakenEventType = "ToolResult"
	CommandOutputEventType       AwakenEventType = "CommandOutput"
	PatchUpdatedEventType        AwakenEventType = "PatchUpdated"
	TurnDiffEventType            AwakenEventType = "TurnDiff"
	ApprovalRequestedEventType   AwakenEventType = "ApprovalRequested"
	ApprovalResolvedEventType    AwakenEventType = "ApprovalResolved"
	ArtifactCreatedEventType     AwakenEventType = "ArtifactCreated"
	ArtifactUpdatedEventType     AwakenEventType = "ArtifactUpdated"
)

type ApprovalRiskTone string

const (
	ApprovalRiskSafe    ApprovalRiskTone = "safe"
	ApprovalRiskWarning ApprovalRiskTone = "warning"
	ApprovalRiskDanger  ApprovalRiskTone = "danger"
)

type ApprovalDecision string

const (
	ApprovalDecisionApproved ApprovalDecision = "approved"
	ApprovalDecisionRejected ApprovalDecision = "rejected"
)

type ArtifactStatus string

const (
	ArtifactStatusReady   ArtifactStatus = "ready"
	ArtifactStatusPending ArtifactStatus = "pending"
	ArtifactStatusError   ArtifactStatus = "error"
)

type AwakenEvent struct {
	Type                  AwakenEventType
	Timestamp             time.Time
	ThreadID              string
	RunID                 string
	MessageID             string
	ParentMessageID       string
	Role                  string
	Delta                 string
	Content               string
	ToolCallID            string
	ToolName              string
	Arguments             string
	Progress              string
	CommandStream         string
	Patch                 string
	PatchSummary          string
	Diff                  string
	Truncated             bool
	Error                 string
	Code                  string
	ApprovalID            string
	ApprovalTitle         string
	ApprovalDescription   string
	ApprovalRiskTone      ApprovalRiskTone
	ApprovalDecision      ApprovalDecision
	ApprovalAffectedPaths []string
	ApprovalPatchSummary  string
	ArtifactID            string
	ArtifactTitle         string
	ArtifactDescription   string
	ArtifactKind          string
	ArtifactPath          string
	ArtifactContent       string
	ArtifactStatus        ArtifactStatus
}

var awakenIDCounter uint64

func newAwakenID(prefix string) string {
	seq := atomic.AddUint64(&awakenIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
