package delegationevents

import "time"

type EventType string

const (
	Unknown EventType = "unknown"

	TaskSpawned   EventType = "delegate.task_spawned"
	TaskProgress  EventType = "delegate.task_progress"
	TaskCompleted EventType = "delegate.task_completed"
	TaskFailed    EventType = "delegate.task_failed"
	TaskThinking  EventType = "delegate.task_thinking"
	ToolStarted   EventType = "delegate.tool_started"
	ToolCompleted EventType = "delegate.tool_completed"
)

var legacyEvents = map[string]EventType{
	"_thinking":           TaskThinking,
	"reasoning.available": TaskThinking,
	"tool.started":        ToolStarted,
	"tool.completed":      ToolCompleted,
	"subagent_progress":   TaskProgress,
}

var canonicalEvents = map[string]EventType{
	string(TaskSpawned):   TaskSpawned,
	string(TaskProgress):  TaskProgress,
	string(TaskCompleted): TaskCompleted,
	string(TaskFailed):    TaskFailed,
	string(TaskThinking):  TaskThinking,
	string(ToolStarted):   ToolStarted,
	string(ToolCompleted): ToolCompleted,
}

type ProgressEvent struct {
	Type     EventType
	ToolName string
	Preview  string
	Args     any
	Duration time.Duration
	IsError  bool
}

func Normalize(input string) (EventType, bool) {
	if eventType, ok := legacyEvents[input]; ok {
		return eventType, true
	}
	if eventType, ok := canonicalEvents[input]; ok {
		return eventType, true
	}
	return Unknown, false
}

func NewProgressEvent(eventType string, toolName string, preview string, args any) (ProgressEvent, bool) {
	normalized, ok := Normalize(eventType)
	if !ok {
		return ProgressEvent{Type: Unknown}, false
	}

	event := ProgressEvent{
		Type:     normalized,
		ToolName: toolName,
		Preview:  preview,
		Args:     args,
	}
	if normalized == TaskProgress {
		if event.Preview == "" {
			event.Preview = event.ToolName
		}
		event.ToolName = ""
	}
	return event, true
}

func (eventType EventType) RelayName() string {
	switch eventType {
	case TaskSpawned:
		return "subagent.start"
	case TaskProgress:
		return "subagent_progress"
	case TaskCompleted:
		return "subagent.complete"
	case TaskFailed:
		return "subagent.failed"
	case TaskThinking:
		return "subagent.thinking"
	case ToolStarted:
		return "subagent.tool"
	case ToolCompleted:
		return "subagent.tool.completed"
	default:
		return string(Unknown)
	}
}

func (eventType EventType) NoopDisplay() bool {
	return eventType == ToolCompleted
}

func (event ProgressEvent) RelayName() string {
	return event.Type.RelayName()
}

func (event ProgressEvent) NoopDisplay() bool {
	return event.Type.NoopDisplay()
}
