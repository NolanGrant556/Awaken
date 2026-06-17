package agentcore

import (
	"fmt"
	"sync/atomic"
	"time"
)

type AwakenEventType string

const (
	RunStartedEventType           AwakenEventType = "RunStarted"
	RunFinishedEventType          AwakenEventType = "RunFinished"
	RunErrorEventType             AwakenEventType = "RunError"
	TextMessageStartedEventType   AwakenEventType = "TextMessageStarted"
	TextDeltaEventType            AwakenEventType = "TextDelta"
	TextMessageFinishedEventType  AwakenEventType = "TextMessageFinished"
	ReasoningStartedEventType     AwakenEventType = "ReasoningStarted"
	ReasoningDeltaEventType       AwakenEventType = "ReasoningDelta"
	ReasoningFinishedEventType    AwakenEventType = "ReasoningFinished"
	ToolCallStartedEventType      AwakenEventType = "ToolCallStarted"
	ToolCallArgsEventType         AwakenEventType = "ToolCallArgs"
	ToolCallFinishedEventType     AwakenEventType = "ToolCallFinished"
	ToolResultEventType           AwakenEventType = "ToolResult"
)

type AwakenEvent struct {
	Type            AwakenEventType
	Timestamp       time.Time
	ThreadID        string
	RunID           string
	MessageID       string
	ParentMessageID string
	Role            string
	Delta           string
	Content         string
	ToolCallID      string
	ToolName        string
	Arguments       string
	Error           string
	Code            string
}

var awakenIDCounter uint64

func newAwakenID(prefix string) string {
	seq := atomic.AddUint64(&awakenIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}
