package hookprogress

import (
	"sync"
	"time"
)

const maxPendingEvents = 100

type HookExecutionEventType string

const (
	HookStarted  HookExecutionEventType = "started"
	HookProgress HookExecutionEventType = "progress"
	HookResponse HookExecutionEventType = "response"
)

type HookOutcome string

const (
	HookOutcomeSuccess   HookOutcome = "success"
	HookOutcomeError     HookOutcome = "error"
	HookOutcomeCancelled HookOutcome = "cancelled"
)

type HookExecutionEvent struct {
	Type      HookExecutionEventType
	HookID    string
	HookName  string
	HookEvent string
	Stdout    string
	Stderr    string
	Output    string
	ExitCode  *int
	Outcome   HookOutcome
}

type HookProgressSnapshot struct {
	HookID    string
	HookName  string
	HookEvent string
	Stdout    string
	Stderr    string
	Output    string
}

type HookResponseData struct {
	HookID    string
	HookName  string
	HookEvent string
	Output    string
	Stdout    string
	Stderr    string
	ExitCode  *int
	Outcome   HookOutcome
}

type HookEventHandler func(HookExecutionEvent)

type HookProgressIntervalParams struct {
	HookID      string
	HookName    string
	HookEvent   string
	GetSnapshot func() HookProgressSnapshot
	Interval    time.Duration
}

type progressKey struct {
	hookID    string
	hookName  string
	hookEvent string
}

var (
	mu                   sync.Mutex
	handler              HookEventHandler
	pendingEvents        []HookExecutionEvent
	allHookEventsEnabled bool
	lastProgressOutput   = make(map[progressKey]string)
)

var alwaysEmittedHookEvents = map[string]struct{}{
	"SessionStart": {},
	"Setup":        {},
}

var allowedHookEvents = map[string]struct{}{
	"PreToolUse":         {},
	"PostToolUse":        {},
	"PostToolUseFailure": {},
	"Notification":       {},
	"UserPromptSubmit":   {},
	"SessionStart":       {},
	"SessionEnd":         {},
	"Stop":               {},
	"StopFailure":        {},
	"SubagentStart":      {},
	"SubagentStop":       {},
	"PreCompact":         {},
	"PostCompact":        {},
	"PermissionRequest":  {},
	"PermissionDenied":   {},
	"Setup":              {},
	"TeammateIdle":       {},
	"TaskCreated":        {},
	"TaskCompleted":      {},
	"Elicitation":        {},
	"ElicitationResult":  {},
	"ConfigChange":       {},
	"WorktreeCreate":     {},
	"WorktreeRemove":     {},
	"InstructionsLoaded": {},
	"CwdChanged":         {},
	"FileChanged":        {},
}

func RegisterHookEventHandler(next HookEventHandler) {
	var flush []HookExecutionEvent

	mu.Lock()
	handler = next
	if next != nil && len(pendingEvents) > 0 {
		flush = append(flush, pendingEvents...)
		pendingEvents = pendingEvents[:0]
	}
	mu.Unlock()

	for _, event := range flush {
		next(event)
	}
}

func EmitHookStarted(hookID, hookName, hookEvent string) bool {
	if !ShouldEmit(hookEvent) {
		return false
	}

	emit(HookExecutionEvent{
		Type:      HookStarted,
		HookID:    hookID,
		HookName:  hookName,
		HookEvent: hookEvent,
	})
	return true
}

func EmitHookProgress(snapshot HookProgressSnapshot) bool {
	if !ShouldEmit(snapshot.HookEvent) {
		return false
	}

	key := progressKey{
		hookID:    snapshot.HookID,
		hookName:  snapshot.HookName,
		hookEvent: snapshot.HookEvent,
	}

	mu.Lock()
	if lastOutput, ok := lastProgressOutput[key]; ok && lastOutput == snapshot.Output {
		mu.Unlock()
		return false
	}
	lastProgressOutput[key] = snapshot.Output
	mu.Unlock()

	emit(HookExecutionEvent{
		Type:      HookProgress,
		HookID:    snapshot.HookID,
		HookName:  snapshot.HookName,
		HookEvent: snapshot.HookEvent,
		Stdout:    snapshot.Stdout,
		Stderr:    snapshot.Stderr,
		Output:    snapshot.Output,
	})
	return true
}

func StartHookProgressInterval(params HookProgressIntervalParams) func() {
	if params.GetSnapshot == nil || !ShouldEmit(params.HookEvent) {
		return func() {}
	}

	interval := params.Interval
	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				snapshot := params.GetSnapshot()
				snapshot.HookID = firstNonEmpty(snapshot.HookID, params.HookID)
				snapshot.HookName = firstNonEmpty(snapshot.HookName, params.HookName)
				snapshot.HookEvent = firstNonEmpty(snapshot.HookEvent, params.HookEvent)
				EmitHookProgress(snapshot)
			case <-done:
				return
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
		})
	}
}

func EmitHookResponse(data HookResponseData) bool {
	if !ShouldEmit(data.HookEvent) {
		return false
	}

	key := progressKey{
		hookID:    data.HookID,
		hookName:  data.HookName,
		hookEvent: data.HookEvent,
	}

	mu.Lock()
	delete(lastProgressOutput, key)
	mu.Unlock()

	emit(HookExecutionEvent{
		Type:      HookResponse,
		HookID:    data.HookID,
		HookName:  data.HookName,
		HookEvent: data.HookEvent,
		Output:    data.Output,
		Stdout:    data.Stdout,
		Stderr:    data.Stderr,
		ExitCode:  data.ExitCode,
		Outcome:   data.Outcome,
	})
	return true
}

func SetAllHookEventsEnabled(enabled bool) {
	mu.Lock()
	allHookEventsEnabled = enabled
	mu.Unlock()
}

func ShouldEmit(hookEvent string) bool {
	mu.Lock()
	defer mu.Unlock()

	if _, ok := alwaysEmittedHookEvents[hookEvent]; ok {
		return true
	}
	if !allHookEventsEnabled {
		return false
	}
	_, ok := allowedHookEvents[hookEvent]
	return ok
}

func ClearHookEventState() {
	mu.Lock()
	handler = nil
	pendingEvents = nil
	allHookEventsEnabled = false
	lastProgressOutput = make(map[progressKey]string)
	mu.Unlock()
}

func emit(event HookExecutionEvent) {
	var current HookEventHandler

	mu.Lock()
	current = handler
	if current == nil {
		pendingEvents = append(pendingEvents, event)
		if len(pendingEvents) > maxPendingEvents {
			copy(pendingEvents, pendingEvents[len(pendingEvents)-maxPendingEvents:])
			pendingEvents = pendingEvents[:maxPendingEvents]
		}
	}
	mu.Unlock()

	if current != nil {
		current(event)
	}
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
