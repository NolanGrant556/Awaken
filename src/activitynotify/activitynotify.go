package activitynotify

import (
	"fmt"
	"sync"
	"time"
)

type EventType string

const (
	EventAPICall      EventType = "api_call"
	EventToolStart    EventType = "tool_start"
	EventToolOutput   EventType = "tool_output"
	EventToolFinish   EventType = "tool_finish"
	EventUserInput    EventType = "user_input"
	EventHeartbeat    EventType = "heartbeat"
	EventBackgrounded EventType = "backgrounded"
	EventForegrounded EventType = "foregrounded"
	EventCompleted    EventType = "completed"
	EventFailed       EventType = "failed"
	EventCancelled    EventType = "cancelled"
)

type Event struct {
	Type         EventType
	At           time.Time
	TaskID       string
	SessionID    string
	ToolName     string
	Message      string
	APICallCount int
}

type TimeoutMode string

const (
	TimeoutDisabled   TimeoutMode = "disabled"
	TimeoutInactivity TimeoutMode = "inactivity"
)

type TimeoutPolicy struct {
	Mode            TimeoutMode
	InactivityLimit time.Duration
}

func DisabledTimeoutPolicy() TimeoutPolicy {
	return TimeoutPolicy{Mode: TimeoutDisabled}
}

func InactivityTimeoutPolicy(limit time.Duration) TimeoutPolicy {
	return TimeoutPolicy{Mode: TimeoutInactivity, InactivityLimit: limit}
}

func (p TimeoutPolicy) Enabled() bool {
	p = p.normalized()
	return p.Mode == TimeoutInactivity && p.InactivityLimit > 0
}

func (p TimeoutPolicy) normalized() TimeoutPolicy {
	if p.Mode == "" && p.InactivityLimit > 0 {
		p.Mode = TimeoutInactivity
	}
	if p.Mode == TimeoutInactivity && p.InactivityLimit <= 0 {
		p.Mode = TimeoutDisabled
	}
	return p
}

type TaskStatus string

const (
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type TrackerConfig struct {
	TimeoutPolicy TimeoutPolicy
	MaxAPICalls   int
}

type Tracker struct {
	mu    sync.RWMutex
	state trackerState
}

type trackerState struct {
	lastActivityAt          time.Time
	lastActivityType        EventType
	lastActivityDescription string
	currentTool             string
	apiCallCount            int
	maxAPICalls             int
	backgrounded            bool
	status                  TaskStatus
	timeoutPolicy           TimeoutPolicy
}

func NewTracker(policy TimeoutPolicy) *Tracker {
	return NewTrackerWithConfig(TrackerConfig{TimeoutPolicy: policy})
}

func NewTrackerWithConfig(config TrackerConfig) *Tracker {
	return &Tracker{
		state: trackerState{
			maxAPICalls:   config.MaxAPICalls,
			status:        TaskStatusRunning,
			timeoutPolicy: config.TimeoutPolicy.normalized(),
		},
	}
}

func (t *Tracker) SetTimeoutPolicy(policy TimeoutPolicy) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.timeoutPolicy = policy.normalized()
}

func (t *Tracker) Record(event Event) ActivitySummary {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	} else {
		event.At = event.At.UTC()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch event.Type {
	case EventAPICall:
		if event.APICallCount > 0 {
			t.state.apiCallCount = event.APICallCount
		} else {
			t.state.apiCallCount++
		}
		t.touchLocked(event, firstNonEmpty(event.Message, fmt.Sprintf("api call #%d", t.state.apiCallCount)))
	case EventToolStart:
		t.state.currentTool = event.ToolName
		t.touchLocked(event, firstNonEmpty(event.Message, describeToolEvent("tool started", event.ToolName)))
	case EventToolOutput:
		if event.ToolName != "" {
			t.state.currentTool = event.ToolName
		}
		t.touchLocked(event, firstNonEmpty(event.Message, describeToolEvent("tool output", event.ToolName)))
	case EventToolFinish:
		if event.ToolName == "" || event.ToolName == t.state.currentTool {
			t.state.currentTool = ""
		}
		t.touchLocked(event, firstNonEmpty(event.Message, describeToolEvent("tool finished", event.ToolName)))
	case EventUserInput:
		t.statusIfRunningLocked()
		t.touchLocked(event, firstNonEmpty(event.Message, "user input"))
	case EventHeartbeat:
		t.touchLocked(event, firstNonEmpty(event.Message, "heartbeat"))
	case EventBackgrounded:
		t.state.backgrounded = true
	case EventForegrounded:
		t.state.backgrounded = false
	case EventCompleted:
		t.state.status = TaskStatusCompleted
		t.state.currentTool = ""
		t.touchLocked(event, firstNonEmpty(event.Message, "completed"))
	case EventFailed:
		t.state.status = TaskStatusFailed
		t.state.currentTool = ""
		t.touchLocked(event, firstNonEmpty(event.Message, "failed"))
	case EventCancelled:
		t.state.status = TaskStatusCancelled
		t.state.currentTool = ""
		t.touchLocked(event, firstNonEmpty(event.Message, "cancelled"))
	}

	return t.summaryLocked(event.At)
}

func (t *Tracker) Summary(now time.Time) ActivitySummary {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.summaryLocked(now)
}

func (t *Tracker) EvaluateTimeout(now time.Time) TimeoutDecision {
	return EvaluateTimeout(t.Summary(now))
}

func (t *Tracker) touchLocked(event Event, description string) {
	t.state.lastActivityAt = event.At
	t.state.lastActivityType = event.Type
	t.state.lastActivityDescription = description
}

func (t *Tracker) statusIfRunningLocked() {
	if t.state.status == "" {
		t.state.status = TaskStatusRunning
	}
}

func (t *Tracker) summaryLocked(now time.Time) ActivitySummary {
	idleFor := time.Duration(0)
	if !t.state.lastActivityAt.IsZero() {
		idleFor = now.Sub(t.state.lastActivityAt)
		if idleFor < 0 {
			idleFor = 0
		}
	}

	return ActivitySummary{
		LastActivityAt:          t.state.lastActivityAt,
		LastActivityType:        t.state.lastActivityType,
		LastActivityDescription: t.state.lastActivityDescription,
		SecondsSinceActivity:    idleFor.Seconds(),
		IdleFor:                 idleFor,
		CurrentTool:             t.state.currentTool,
		APICallCount:            t.state.apiCallCount,
		MaxAPICalls:             t.state.maxAPICalls,
		Backgrounded:            t.state.backgrounded,
		Status:                  t.state.status,
		Terminal:                t.state.status.IsTerminal(),
		TimeoutPolicy:           t.state.timeoutPolicy.normalized(),
	}
}

type ActivitySummary struct {
	LastActivityAt          time.Time
	LastActivityType        EventType
	LastActivityDescription string
	SecondsSinceActivity    float64
	IdleFor                 time.Duration
	CurrentTool             string
	APICallCount            int
	MaxAPICalls             int
	Backgrounded            bool
	Status                  TaskStatus
	Terminal                bool
	TimeoutPolicy           TimeoutPolicy
}

type TimeoutDecision struct {
	TimedOut  bool
	Reason    string
	IdleFor   time.Duration
	Limit     time.Duration
	Remaining time.Duration
	Summary   ActivitySummary
}

func EvaluateTimeout(summary ActivitySummary) TimeoutDecision {
	policy := summary.TimeoutPolicy.normalized()
	decision := TimeoutDecision{
		IdleFor: summary.IdleFor,
		Limit:   policy.InactivityLimit,
		Summary: summary,
	}

	if summary.Terminal {
		decision.Reason = "task is terminal"
		return decision
	}
	if !policy.Enabled() {
		decision.Reason = "timeout disabled"
		return decision
	}
	if summary.LastActivityAt.IsZero() {
		decision.Reason = "no activity recorded"
		return decision
	}
	if summary.IdleFor >= policy.InactivityLimit {
		decision.TimedOut = true
		decision.Reason = fmt.Sprintf("idle for %s; inactivity limit is %s", summary.IdleFor, policy.InactivityLimit)
		return decision
	}

	decision.Remaining = policy.InactivityLimit - summary.IdleFor
	decision.Reason = "activity is fresh"
	return decision
}

func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusCompleted || s == TaskStatusFailed || s == TaskStatusCancelled
}

func (e EventType) IsTerminal() bool {
	return e == EventCompleted || e == EventFailed || e == EventCancelled
}

func (e EventType) IsProgress() bool {
	switch e {
	case EventAPICall, EventToolStart, EventToolOutput, EventToolFinish, EventHeartbeat, EventCompleted, EventFailed, EventCancelled:
		return true
	default:
		return false
	}
}

func describeToolEvent(prefix, toolName string) string {
	if toolName == "" {
		return prefix
	}
	return prefix + ": " + toolName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
