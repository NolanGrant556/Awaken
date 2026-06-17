package agui

import (
	"time"

	"awaken/src/agentcore"
)

func MapEvent(event agentcore.AwakenEvent) []Event {
	base := Event{
		Timestamp: toUnixMillis(event.Timestamp),
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		MessageID: event.MessageID,
		Role:      event.Role,
	}

	switch event.Type {
	case agentcore.RunStartedEventType:
		base.Type = EventRunStarted
		return []Event{base}
	case agentcore.RunFinishedEventType:
		base.Type = EventRunFinished
		base.Result = event.Content
		base.Outcome = &RunOutcome{Type: "success"}
		return []Event{base}
	case agentcore.RunErrorEventType:
		base.Type = EventRunError
		base.Message = event.Error
		base.Code = event.Code
		return []Event{base}
	case agentcore.TextMessageStartedEventType:
		base.Type = EventTextMessageStart
		if base.Role == "" {
			base.Role = "assistant"
		}
		return []Event{base}
	case agentcore.TextDeltaEventType:
		base.Type = EventTextMessageContent
		base.Delta = event.Delta
		return []Event{base}
	case agentcore.TextMessageFinishedEventType:
		base.Type = EventTextMessageEnd
		return []Event{base}
	case agentcore.ReasoningStartedEventType:
		start := base
		start.Type = EventReasoningStart
		messageStart := base
		messageStart.Type = EventReasoningMessageStart
		messageStart.Role = "reasoning"
		return []Event{start, messageStart}
	case agentcore.ReasoningDeltaEventType:
		base.Type = EventReasoningMessageContent
		base.Delta = event.Delta
		return []Event{base}
	case agentcore.ReasoningFinishedEventType:
		messageEnd := base
		messageEnd.Type = EventReasoningMessageEnd
		reasoningEnd := base
		reasoningEnd.Type = EventReasoningEnd
		return []Event{messageEnd, reasoningEnd}
	case agentcore.ToolCallStartedEventType:
		base.Type = EventToolCallStart
		base.ToolCallID = event.ToolCallID
		base.ToolCallName = event.ToolName
		base.ParentMessageID = event.ParentMessageID
		return []Event{base}
	case agentcore.ToolCallArgsEventType:
		base.Type = EventToolCallArgs
		base.ToolCallID = event.ToolCallID
		base.Delta = event.Arguments
		return []Event{base}
	case agentcore.ToolCallFinishedEventType:
		base.Type = EventToolCallEnd
		base.ToolCallID = event.ToolCallID
		return []Event{base}
	case agentcore.ToolResultEventType:
		base.Type = EventToolCallResult
		base.ToolCallID = event.ToolCallID
		base.Content = event.Content
		if base.Role == "" {
			base.Role = "tool"
		}
		return []Event{base}
	default:
		return nil
	}
}

func toUnixMillis(ts time.Time) int64 {
	if ts.IsZero() {
		return 0
	}
	return ts.UnixMilli()
}
