package sdkevents

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

const DefaultCapacity = 1000

type EventType string

const (
	EventTypeSystem EventType = "system"
)

type EventSubtype string

const (
	EventSubtypeTaskStarted         EventSubtype = "task_started"
	EventSubtypeTaskProgress        EventSubtype = "task_progress"
	EventSubtypeTaskNotification    EventSubtype = "task_notification"
	EventSubtypeSessionStateChanged EventSubtype = "session_state_changed"
)

type TaskStatus string

const (
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusStopped   TaskStatus = "stopped"
)

type SessionState string

const (
	SessionStateIdle           SessionState = "idle"
	SessionStateRunning        SessionState = "running"
	SessionStateRequiresAction SessionState = "requires_action"
)

type Usage struct {
	TotalTokens int `json:"total_tokens"`
	ToolUses    int `json:"tool_uses"`
	DurationMS  int `json:"duration_ms"`
}

type WorkflowProgress struct {
	Type       string `json:"type"`
	Index      int    `json:"index"`
	PhaseIndex int    `json:"phaseIndex,omitempty"`
	Status     string `json:"status,omitempty"`
	Title      string `json:"title,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type Event struct {
	Type             EventType          `json:"type"`
	Subtype          EventSubtype       `json:"subtype"`
	TaskID           string             `json:"task_id,omitempty"`
	ToolUseID        string             `json:"tool_use_id,omitempty"`
	Description      string             `json:"description,omitempty"`
	TaskType         string             `json:"task_type,omitempty"`
	WorkflowName     string             `json:"workflow_name,omitempty"`
	Prompt           string             `json:"prompt,omitempty"`
	Usage            *Usage             `json:"usage,omitempty"`
	LastToolName     string             `json:"last_tool_name,omitempty"`
	Summary          string             `json:"summary,omitempty"`
	WorkflowProgress []WorkflowProgress `json:"workflow_progress,omitempty"`
	Status           TaskStatus         `json:"status,omitempty"`
	OutputFile       string             `json:"output_file,omitempty"`
	State            SessionState       `json:"state,omitempty"`
}

type TaskStarted struct {
	TaskID       string
	ToolUseID    string
	Description  string
	TaskType     string
	WorkflowName string
	Prompt       string
}

func NewTaskStartedEvent(started TaskStarted) Event {
	return Event{
		Type:         EventTypeSystem,
		Subtype:      EventSubtypeTaskStarted,
		TaskID:       started.TaskID,
		ToolUseID:    started.ToolUseID,
		Description:  started.Description,
		TaskType:     started.TaskType,
		WorkflowName: started.WorkflowName,
		Prompt:       started.Prompt,
	}
}

type TaskProgress struct {
	TaskID           string
	ToolUseID        string
	Description      string
	Usage            Usage
	LastToolName     string
	Summary          string
	WorkflowProgress []WorkflowProgress
}

func NewTaskProgressEvent(progress TaskProgress) Event {
	usage := progress.Usage
	return Event{
		Type:             EventTypeSystem,
		Subtype:          EventSubtypeTaskProgress,
		TaskID:           progress.TaskID,
		ToolUseID:        progress.ToolUseID,
		Description:      progress.Description,
		Usage:            &usage,
		LastToolName:     progress.LastToolName,
		Summary:          progress.Summary,
		WorkflowProgress: append([]WorkflowProgress(nil), progress.WorkflowProgress...),
	}
}

type TaskNotification struct {
	TaskID     string
	ToolUseID  string
	Status     TaskStatus
	OutputFile string
	Summary    string
	Usage      *Usage
}

func NewTaskNotificationEvent(notification TaskNotification) Event {
	var usage *Usage
	if notification.Usage != nil {
		usageCopy := *notification.Usage
		usage = &usageCopy
	}
	return Event{
		Type:       EventTypeSystem,
		Subtype:    EventSubtypeTaskNotification,
		TaskID:     notification.TaskID,
		ToolUseID:  notification.ToolUseID,
		Status:     notification.Status,
		OutputFile: notification.OutputFile,
		Summary:    notification.Summary,
		Usage:      usage,
	}
}

type SessionStateChanged struct {
	State SessionState
}

func NewSessionStateChangedEvent(changed SessionStateChanged) Event {
	return Event{
		Type:    EventTypeSystem,
		Subtype: EventSubtypeSessionStateChanged,
		State:   changed.State,
	}
}

type DrainedEvent struct {
	Event
	UUID      string `json:"uuid"`
	SessionID string `json:"session_id"`
}

type Options struct {
	Enabled        bool
	NonInteractive bool
	Capacity       int
}

type Queue struct {
	mu             sync.Mutex
	enabled        bool
	nonInteractive bool
	capacity       int
	events         []Event
}

func NewQueue(opts Options) *Queue {
	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Queue{
		enabled:        opts.Enabled,
		nonInteractive: opts.NonInteractive,
		capacity:       capacity,
		events:         make([]Event, 0, capacity),
	}
}

func (q *Queue) Enqueue(event Event) {
	if q == nil {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.enabled || !q.nonInteractive {
		return
	}

	event = cloneEvent(event)
	event.Type = EventTypeSystem
	if len(q.events) >= q.capacity {
		copy(q.events, q.events[1:])
		q.events[len(q.events)-1] = Event{}
		q.events = q.events[:len(q.events)-1]
	}
	q.events = append(q.events, event)
}

func (q *Queue) Drain(sessionID string) []DrainedEvent {
	if q == nil {
		return []DrainedEvent{}
	}

	q.mu.Lock()
	if len(q.events) == 0 {
		q.mu.Unlock()
		return []DrainedEvent{}
	}

	events := make([]Event, len(q.events))
	for i, event := range q.events {
		events[i] = cloneEvent(event)
	}
	clear(q.events)
	q.events = q.events[:0]
	q.mu.Unlock()

	drained := make([]DrainedEvent, len(events))
	for i, event := range events {
		drained[i] = DrainedEvent{
			Event:     event,
			UUID:      newUUID(),
			SessionID: sessionID,
		}
	}
	return drained
}

func cloneEvent(event Event) Event {
	cloned := event
	if event.Usage != nil {
		usage := *event.Usage
		cloned.Usage = &usage
	}
	if event.WorkflowProgress != nil {
		cloned.WorkflowProgress = append([]WorkflowProgress(nil), event.WorkflowProgress...)
	}
	return cloned
}

func newUUID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	var uuid [36]byte
	hex.Encode(uuid[0:8], bytes[0:4])
	uuid[8] = '-'
	hex.Encode(uuid[9:13], bytes[4:6])
	uuid[13] = '-'
	hex.Encode(uuid[14:18], bytes[6:8])
	uuid[18] = '-'
	hex.Encode(uuid[19:23], bytes[8:10])
	uuid[23] = '-'
	hex.Encode(uuid[24:36], bytes[10:16])
	return string(uuid[:])
}
