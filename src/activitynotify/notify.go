package activitynotify

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Notification struct {
	ID        string
	TaskID    string
	SessionID string
	Event     EventType
	Title     string
	Body      string
	At        time.Time
}

type NotificationDecision string

const (
	NotificationQueued               NotificationDecision = "queued"
	NotificationDuplicate            NotificationDecision = "duplicate"
	NotificationSuppressedForeground NotificationDecision = "suppressed_foreground"
	NotificationIgnored              NotificationDecision = "ignored"
)

type NotificationResult struct {
	Decision     NotificationDecision
	Notification *Notification
}

type NotificationManager struct {
	mu     sync.Mutex
	tasks  map[taskKey]notificationTaskState
	seen   map[notificationKey]struct{}
	queued []Notification
	away   map[string]*awayState
}

type taskKey struct {
	sessionID string
	taskID    string
}

type notificationKey struct {
	sessionID string
	taskID    string
	event     EventType
}

type notificationTaskState struct {
	backgrounded bool
}

type awayState struct {
	away                        bool
	wasAwaySinceLastUserMessage bool
	progressSinceAway           bool
	summarySinceLastUserMessage bool
}

func NewNotificationManager() *NotificationManager {
	return &NotificationManager{
		tasks: make(map[taskKey]notificationTaskState),
		seen:  make(map[notificationKey]struct{}),
		away:  make(map[string]*awayState),
	}
}

func (m *NotificationManager) Record(event Event, summary ActivitySummary) NotificationResult {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	} else {
		event.At = event.At.UTC()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := taskKey{sessionID: event.SessionID, taskID: event.TaskID}
	taskState, hasTaskState := m.tasks[key]
	sessionAway := m.awayStateLocked(event.SessionID)

	switch event.Type {
	case EventBackgrounded:
		taskState.backgrounded = true
		m.tasks[key] = taskState
		sessionAway.away = true
		sessionAway.wasAwaySinceLastUserMessage = true
		return NotificationResult{Decision: NotificationIgnored}
	case EventForegrounded:
		taskState.backgrounded = false
		m.tasks[key] = taskState
		sessionAway.away = false
		return NotificationResult{Decision: NotificationIgnored}
	case EventUserInput:
		sessionAway.away = false
		sessionAway.wasAwaySinceLastUserMessage = false
		sessionAway.progressSinceAway = false
		sessionAway.summarySinceLastUserMessage = false
		return NotificationResult{Decision: NotificationIgnored}
	}

	if event.Type.IsProgress() && sessionAway.away {
		sessionAway.progressSinceAway = true
	}

	if !event.Type.IsTerminal() {
		return NotificationResult{Decision: NotificationIgnored}
	}

	notificationKey := notificationKey{sessionID: event.SessionID, taskID: event.TaskID, event: event.Type}
	if _, ok := m.seen[notificationKey]; ok {
		return NotificationResult{Decision: NotificationDuplicate}
	}
	m.seen[notificationKey] = struct{}{}

	backgrounded := summary.Backgrounded
	if hasTaskState {
		backgrounded = taskState.backgrounded
	}
	if !backgrounded {
		return NotificationResult{Decision: NotificationSuppressedForeground}
	}

	notification := buildNotification(event, summary)
	m.queued = append(m.queued, notification)
	return NotificationResult{Decision: NotificationQueued, Notification: &notification}
}

func (m *NotificationManager) Queue() []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Notification(nil), m.queued...)
}

func (m *NotificationManager) Drain() []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]Notification(nil), m.queued...)
	m.queued = nil
	return out
}

type AwaySummaryInput struct {
	UserWasAway                 bool
	ProgressHappened            bool
	SummarySinceLastUserMessage bool
}

func ShouldTriggerAwaySummary(input AwaySummaryInput) bool {
	return input.UserWasAway && input.ProgressHappened && !input.SummarySinceLastUserMessage
}

func (m *NotificationManager) ShouldTriggerAwaySummary(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.awayStateLocked(sessionID)
	return ShouldTriggerAwaySummary(AwaySummaryInput{
		UserWasAway:                 state.wasAwaySinceLastUserMessage,
		ProgressHappened:            state.progressSinceAway,
		SummarySinceLastUserMessage: state.summarySinceLastUserMessage,
	})
}

func (m *NotificationManager) MarkAwaySummarySent(sessionID string, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.awayStateLocked(sessionID)
	state.summarySinceLastUserMessage = true
}

func (m *NotificationManager) awayStateLocked(sessionID string) *awayState {
	state := m.away[sessionID]
	if state == nil {
		state = &awayState{}
		m.away[sessionID] = state
	}
	return state
}

func buildNotification(event Event, summary ActivitySummary) Notification {
	title := fmt.Sprintf("Task %s %s", displayTaskID(event.TaskID), terminalVerb(event.Type))
	bodyParts := []string{title}
	if event.Message != "" {
		bodyParts = append(bodyParts, event.Message)
	}
	if summary.CurrentTool != "" {
		bodyParts = append(bodyParts, "current tool: "+summary.CurrentTool)
	} else if summary.LastActivityDescription != "" {
		bodyParts = append(bodyParts, "last activity: "+summary.LastActivityDescription)
	}
	if summary.APICallCount > 0 {
		bodyParts = append(bodyParts, fmt.Sprintf("api calls: %d", summary.APICallCount))
	}

	return Notification{
		ID:        fmt.Sprintf("%s:%s:%s", event.SessionID, event.TaskID, event.Type),
		TaskID:    event.TaskID,
		SessionID: event.SessionID,
		Event:     event.Type,
		Title:     RedactSecrets(title),
		Body:      RedactSecrets(strings.Join(bodyParts, " | ")),
		At:        event.At,
	}
}

func displayTaskID(taskID string) string {
	if taskID == "" {
		return "unknown"
	}
	return taskID
}

func terminalVerb(event EventType) string {
	switch event {
	case EventCompleted:
		return "completed"
	case EventFailed:
		return "failed"
	case EventCancelled:
		return "cancelled"
	default:
		return string(event)
	}
}
