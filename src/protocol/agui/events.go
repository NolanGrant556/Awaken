package agui

type EventType string

const (
	EventRunStarted             EventType = "RUN_STARTED"
	EventRunFinished            EventType = "RUN_FINISHED"
	EventRunError               EventType = "RUN_ERROR"
	EventTextMessageStart       EventType = "TEXT_MESSAGE_START"
	EventTextMessageContent     EventType = "TEXT_MESSAGE_CONTENT"
	EventTextMessageEnd         EventType = "TEXT_MESSAGE_END"
	EventReasoningStart         EventType = "REASONING_START"
	EventReasoningMessageStart  EventType = "REASONING_MESSAGE_START"
	EventReasoningMessageContent EventType = "REASONING_MESSAGE_CONTENT"
	EventReasoningMessageEnd    EventType = "REASONING_MESSAGE_END"
	EventReasoningEnd           EventType = "REASONING_END"
	EventToolCallStart          EventType = "TOOL_CALL_START"
	EventToolCallArgs           EventType = "TOOL_CALL_ARGS"
	EventToolCallEnd            EventType = "TOOL_CALL_END"
	EventToolCallResult         EventType = "TOOL_CALL_RESULT"
)

type RunOutcome struct {
	Type string `json:"type"`
}

type Event struct {
	Type            EventType    `json:"type"`
	Timestamp       int64        `json:"timestamp,omitempty"`
	ThreadID        string       `json:"threadId,omitempty"`
	RunID           string       `json:"runId,omitempty"`
	MessageID       string       `json:"messageId,omitempty"`
	ParentRunID     string       `json:"parentRunId,omitempty"`
	ParentMessageID string       `json:"parentMessageId,omitempty"`
	Role            string       `json:"role,omitempty"`
	Name            string       `json:"name,omitempty"`
	Delta           string       `json:"delta,omitempty"`
	ToolCallID      string       `json:"toolCallId,omitempty"`
	ToolCallName    string       `json:"toolCallName,omitempty"`
	Content         string       `json:"content,omitempty"`
	Result          string       `json:"result,omitempty"`
	Message         string       `json:"message,omitempty"`
	Code            string       `json:"code,omitempty"`
	Outcome         *RunOutcome  `json:"outcome,omitempty"`
}
