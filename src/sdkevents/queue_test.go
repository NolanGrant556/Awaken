package sdkevents

import (
	"fmt"
	"regexp"
	"testing"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestQueueUsesDefaultCapacityAndDropsOldest(t *testing.T) {
	q := NewQueue(Options{Enabled: true, NonInteractive: true})

	for i := 0; i < DefaultCapacity+1; i++ {
		q.Enqueue(NewTaskStartedEvent(TaskStarted{
			TaskID:      fmt.Sprintf("task-%d", i),
			Description: "started",
		}))
	}

	drained := q.Drain("session-1")
	if len(drained) != DefaultCapacity {
		t.Fatalf("drained %d events, want %d", len(drained), DefaultCapacity)
	}
	if drained[0].TaskID != "task-1" {
		t.Fatalf("first drained task = %q, want oldest retained task-1", drained[0].TaskID)
	}
	if drained[len(drained)-1].TaskID != fmt.Sprintf("task-%d", DefaultCapacity) {
		t.Fatalf("last drained task = %q, want newest task", drained[len(drained)-1].TaskID)
	}
}

func TestQueueDropsOldestWithConfiguredCapacity(t *testing.T) {
	q := NewQueue(Options{Enabled: true, NonInteractive: true, Capacity: 2})

	q.Enqueue(NewTaskStartedEvent(TaskStarted{TaskID: "first", Description: "one"}))
	q.Enqueue(NewTaskStartedEvent(TaskStarted{TaskID: "second", Description: "two"}))
	q.Enqueue(NewTaskStartedEvent(TaskStarted{TaskID: "third", Description: "three"}))

	drained := q.Drain("session-1")
	if len(drained) != 2 {
		t.Fatalf("drained %d events, want 2", len(drained))
	}
	if drained[0].TaskID != "second" || drained[1].TaskID != "third" {
		t.Fatalf("drained task order = [%q, %q], want [second, third]", drained[0].TaskID, drained[1].TaskID)
	}
}

func TestQueueOnlyEnqueuesWhenEnabledAndNonInteractive(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		nonInteractive bool
		wantEvents     int
	}{
		{name: "disabled", enabled: false, nonInteractive: true, wantEvents: 0},
		{name: "interactive", enabled: true, nonInteractive: false, wantEvents: 0},
		{name: "enabled non interactive", enabled: true, nonInteractive: true, wantEvents: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue(Options{
				Enabled:        tt.enabled,
				NonInteractive: tt.nonInteractive,
				Capacity:       2,
			})
			q.Enqueue(NewTaskStartedEvent(TaskStarted{TaskID: "task-1", Description: "started"}))

			drained := q.Drain("session-1")
			if len(drained) != tt.wantEvents {
				t.Fatalf("drained %d events, want %d", len(drained), tt.wantEvents)
			}
		})
	}
}

func TestDrainAddsMetadataAndClearsQueue(t *testing.T) {
	q := NewQueue(Options{Enabled: true, NonInteractive: true, Capacity: 3})
	q.Enqueue(NewTaskProgressEvent(TaskProgress{
		TaskID:      "task-1",
		Description: "working",
		Usage: Usage{
			TotalTokens: 42,
			ToolUses:    2,
			DurationMS:  1500,
		},
	}))
	q.Enqueue(NewSessionStateChangedEvent(SessionStateChanged{State: SessionStateIdle}))

	drained := q.Drain("session-1")
	if len(drained) != 2 {
		t.Fatalf("drained %d events, want 2", len(drained))
	}

	seenUUIDs := map[string]bool{}
	for _, event := range drained {
		if !uuidPattern.MatchString(event.UUID) {
			t.Fatalf("uuid %q is not an RFC 4122 version 4 UUID", event.UUID)
		}
		if seenUUIDs[event.UUID] {
			t.Fatalf("uuid %q was reused in one drain", event.UUID)
		}
		seenUUIDs[event.UUID] = true
		if event.SessionID != "session-1" {
			t.Fatalf("session_id = %q, want session-1", event.SessionID)
		}
	}

	if again := q.Drain("session-1"); len(again) != 0 {
		t.Fatalf("second drain returned %d events, want empty queue", len(again))
	}
}

func TestDrainReturnsCopyThatCannotPolluteQueue(t *testing.T) {
	q := NewQueue(Options{Enabled: true, NonInteractive: true, Capacity: 2})
	q.Enqueue(NewTaskStartedEvent(TaskStarted{TaskID: "original", Description: "started"}))

	drained := q.Drain("session-1")
	if len(drained) != 1 {
		t.Fatalf("drained %d events, want 1", len(drained))
	}

	q.Enqueue(NewTaskStartedEvent(TaskStarted{TaskID: "next", Description: "started"}))
	drained[0].TaskID = "polluted"

	next := q.Drain("session-1")
	if len(next) != 1 {
		t.Fatalf("drained %d events after re-enqueue, want 1", len(next))
	}
	if next[0].TaskID != "next" {
		t.Fatalf("next task id = %q, want next", next[0].TaskID)
	}
}

func TestEventsAreCopiedOnEnqueueAndDrain(t *testing.T) {
	q := NewQueue(Options{Enabled: true, NonInteractive: true, Capacity: 2})
	usage := &Usage{TotalTokens: 10, ToolUses: 1, DurationMS: 100}
	event := NewTaskNotificationEvent(TaskNotification{
		TaskID:     "task-1",
		Status:     TaskStatusCompleted,
		OutputFile: "result.md",
		Summary:    "done",
		Usage:      usage,
	})

	q.Enqueue(event)
	event.Usage.TotalTokens = 99
	usage.ToolUses = 99

	drained := q.Drain("session-1")
	if got := drained[0].Usage.TotalTokens; got != 10 {
		t.Fatalf("drained usage total_tokens = %d, want copied value 10", got)
	}
	if got := drained[0].Usage.ToolUses; got != 1 {
		t.Fatalf("drained usage tool_uses = %d, want copied value 1", got)
	}

	drained[0].Usage.TotalTokens = 123
	q.Enqueue(NewTaskNotificationEvent(TaskNotification{
		TaskID: "task-2",
		Status: TaskStatusFailed,
		Usage:  &Usage{TotalTokens: 5},
	}))

	next := q.Drain("session-1")
	if got := next[0].Usage.TotalTokens; got != 5 {
		t.Fatalf("next usage total_tokens = %d, want 5", got)
	}
}

func TestEventConstructorsBuildStructuredSystemEvents(t *testing.T) {
	started := NewTaskStartedEvent(TaskStarted{
		TaskID:       "task-1",
		ToolUseID:    "tool-1",
		Description:  "scan files",
		TaskType:     "analysis",
		WorkflowName: "investigate",
		Prompt:       "look around",
	})
	if started.Type != EventTypeSystem || started.Subtype != EventSubtypeTaskStarted {
		t.Fatalf("started type/subtype = %q/%q", started.Type, started.Subtype)
	}
	if started.TaskID != "task-1" || started.ToolUseID != "tool-1" || started.Prompt != "look around" {
		t.Fatalf("started event fields were not preserved: %+v", started)
	}

	progress := NewTaskProgressEvent(TaskProgress{
		TaskID:       "task-1",
		ToolUseID:    "tool-1",
		Description:  "halfway",
		LastToolName: "grep",
		Summary:      "found matches",
		Usage:        Usage{TotalTokens: 20, ToolUses: 3, DurationMS: 500},
		WorkflowProgress: []WorkflowProgress{{
			Type:       "phase",
			Index:      0,
			PhaseIndex: 1,
			Status:     "running",
			Title:      "Search",
		}},
	})
	if progress.Type != EventTypeSystem || progress.Subtype != EventSubtypeTaskProgress {
		t.Fatalf("progress type/subtype = %q/%q", progress.Type, progress.Subtype)
	}
	if progress.Usage == nil || progress.Usage.TotalTokens != 20 || progress.WorkflowProgress[0].PhaseIndex != 1 {
		t.Fatalf("progress event fields were not preserved: %+v", progress)
	}

	notification := NewTaskNotificationEvent(TaskNotification{
		TaskID:     "task-1",
		ToolUseID:  "tool-1",
		Status:     TaskStatusStopped,
		OutputFile: "output.md",
		Summary:    "stopped by user",
	})
	if notification.Type != EventTypeSystem || notification.Subtype != EventSubtypeTaskNotification {
		t.Fatalf("notification type/subtype = %q/%q", notification.Type, notification.Subtype)
	}
	if notification.Status != TaskStatusStopped || notification.OutputFile != "output.md" {
		t.Fatalf("notification event fields were not preserved: %+v", notification)
	}

	stateChanged := NewSessionStateChangedEvent(SessionStateChanged{State: SessionStateRequiresAction})
	if stateChanged.Type != EventTypeSystem || stateChanged.Subtype != EventSubtypeSessionStateChanged {
		t.Fatalf("state type/subtype = %q/%q", stateChanged.Type, stateChanged.Subtype)
	}
	if stateChanged.State != SessionStateRequiresAction {
		t.Fatalf("state event field = %q, want requires_action", stateChanged.State)
	}
}
