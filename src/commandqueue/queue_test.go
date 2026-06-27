package commandqueue

import (
	"reflect"
	"testing"
)

func TestPriorityFIFO(t *testing.T) {
	queue := NewQueue()
	queue.Enqueue(Command{ID: "later-1", Priority: PriorityLater})
	queue.Enqueue(Command{ID: "next-1"})
	queue.Enqueue(Command{ID: "now-1", Priority: PriorityNow})
	queue.Enqueue(Command{ID: "now-2", Priority: PriorityNow})
	queue.Enqueue(Command{ID: "next-2", Priority: PriorityNext})

	got := drainIDs(t, queue)
	want := []string{"now-1", "now-2", "next-1", "next-2", "later-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dequeue order = %v, want %v", got, want)
	}
}

func TestFilterDequeue(t *testing.T) {
	queue := NewQueue()
	queue.Enqueue(Command{ID: "global-now", Priority: PriorityNow})
	queue.Enqueue(Command{ID: "agent-later", Priority: PriorityLater, AgentID: "agent-1"})
	queue.Enqueue(Command{ID: "agent-now", Priority: PriorityNow, AgentID: "agent-1"})

	filter := func(command Command) bool {
		return command.AgentID == "agent-1"
	}

	peeked, ok := queue.Peek(filter)
	if !ok || peeked.ID != "agent-now" {
		t.Fatalf("peek = %+v, %v; want agent-now, true", peeked, ok)
	}

	dequeued, ok := queue.Dequeue(filter)
	if !ok || dequeued.ID != "agent-now" {
		t.Fatalf("dequeue = %+v, %v; want agent-now, true", dequeued, ok)
	}

	remaining := queue.Snapshot()
	if len(remaining) != 2 || remaining[0].ID != "global-now" || remaining[1].ID != "agent-later" {
		t.Fatalf("unexpected remaining queue: %+v", remaining)
	}
}

func TestDequeueAllMatchingUsesPriorityOrder(t *testing.T) {
	queue := NewQueue()
	queue.Enqueue(Command{ID: "skip", Priority: PriorityNow, AgentID: "other"})
	queue.Enqueue(Command{ID: "match-next", Priority: PriorityNext, AgentID: "agent-1"})
	queue.Enqueue(Command{ID: "match-now", Priority: PriorityNow, AgentID: "agent-1"})
	queue.Enqueue(Command{ID: "match-later", Priority: PriorityLater, AgentID: "agent-1"})

	matched := queue.DequeueAllMatching(func(command Command) bool {
		return command.AgentID == "agent-1"
	})

	if got, want := commandIDs(matched), []string{"match-now", "match-next", "match-later"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matched order = %v, want %v", got, want)
	}
	if got, want := commandIDs(queue.Snapshot()), []string{"skip"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining = %v, want %v", got, want)
	}
}

func TestNotificationDefaultsToLater(t *testing.T) {
	queue := NewQueue()
	queue.EnqueueNotification(Command{ID: "notification", Value: "background tick"})

	snapshot := queue.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot length = %d, want 1", len(snapshot))
	}
	if snapshot[0].Priority != PriorityLater {
		t.Fatalf("priority = %q, want %q", snapshot[0].Priority, PriorityLater)
	}
	if snapshot[0].Mode != ModeTaskNotification {
		t.Fatalf("mode = %q, want %q", snapshot[0].Mode, ModeTaskNotification)
	}
}

func TestSnapshotReturnsCopy(t *testing.T) {
	queue := NewQueue()
	queue.Enqueue(Command{ID: "original", Value: "keep"})

	snapshot := queue.Snapshot()
	snapshot[0].Value = "changed"
	snapshot = append(snapshot, Command{ID: "extra"})

	next := queue.Snapshot()
	if len(next) != 1 {
		t.Fatalf("queue length = %d, want 1", len(next))
	}
	if next[0].Value != "keep" {
		t.Fatalf("queue value = %q, want keep", next[0].Value)
	}
}

func TestRemoveAndClear(t *testing.T) {
	queue := NewQueue()
	queue.Enqueue(Command{ID: "prompt", Mode: ModePrompt})
	queue.Enqueue(Command{ID: "bash", Mode: ModeBash})
	queue.EnqueueNotification(Command{ID: "notification"})

	removed := queue.RemoveByFilter(func(command Command) bool {
		return command.Mode == ModeBash
	})
	if got, want := commandIDs(removed), []string{"bash"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed = %v, want %v", got, want)
	}
	if queue.Len() != 2 {
		t.Fatalf("queue length after remove = %d, want 2", queue.Len())
	}

	queue.Clear()
	if queue.Len() != 0 {
		t.Fatalf("queue length after clear = %d, want 0", queue.Len())
	}
}

func TestEditableVisibleHelpers(t *testing.T) {
	tests := []struct {
		name     string
		command  Command
		editable bool
		visible  bool
	}{
		{
			name:     "prompt",
			command:  Command{Mode: ModePrompt},
			editable: true,
			visible:  true,
		},
		{
			name:     "bash",
			command:  Command{Mode: ModeBash},
			editable: true,
			visible:  true,
		},
		{
			name:     "task notification",
			command:  Command{Mode: ModeTaskNotification},
			editable: false,
			visible:  false,
		},
		{
			name:     "meta",
			command:  Command{Mode: ModePrompt, IsMeta: true},
			editable: false,
			visible:  false,
		},
		{
			name:     "channel mode",
			command:  Command{Mode: ModeChannel},
			editable: false,
			visible:  true,
		},
		{
			name:     "channel origin",
			command:  Command{Mode: ModePrompt, OriginKind: "channel"},
			editable: false,
			visible:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsEditable(test.command); got != test.editable {
				t.Fatalf("IsEditable = %v, want %v", got, test.editable)
			}
			if got := IsVisible(test.command); got != test.visible {
				t.Fatalf("IsVisible = %v, want %v", got, test.visible)
			}
		})
	}
}

func TestSlashCommandSkip(t *testing.T) {
	tests := []struct {
		name    string
		command Command
		want    bool
	}{
		{
			name:    "slash command",
			command: Command{Value: "/help"},
			want:    true,
		},
		{
			name:    "trimmed slash command",
			command: Command{Value: "  /help  "},
			want:    true,
		},
		{
			name:    "skip slash commands",
			command: Command{Value: "/help", SkipSlashCommands: true},
			want:    false,
		},
		{
			name:    "plain text",
			command: Command{Value: "please run /help"},
			want:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsSlashCommand(test.command); got != test.want {
				t.Fatalf("IsSlashCommand = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPopAllEditableRetainsTaskNotification(t *testing.T) {
	queue := NewQueue()
	queue.Enqueue(Command{ID: "prompt-1", Value: "first", Mode: ModePrompt})
	queue.EnqueueNotification(Command{ID: "notification", Value: "background"})
	queue.Enqueue(Command{ID: "bash-1", Value: "second", Mode: ModeBash})

	text, cursorOffset, ok := queue.PopAllEditable("current", 3)
	if !ok {
		t.Fatalf("expected editable commands to be popped")
	}
	if want := "first\nsecond\ncurrent"; text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	if want := len("first\nsecond") + 1 + 3; cursorOffset != want {
		t.Fatalf("cursor offset = %d, want %d", cursorOffset, want)
	}

	remaining := queue.Snapshot()
	if got, want := commandIDs(remaining), []string{"notification"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remaining = %v, want %v", got, want)
	}
	if remaining[0].Mode != ModeTaskNotification {
		t.Fatalf("remaining mode = %q, want %q", remaining[0].Mode, ModeTaskNotification)
	}

	operations := queue.Operations()
	if got, want := operationTypes(operations[len(operations)-2:]), []string{"popAll", "popAll"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("last operation types = %v, want %v", got, want)
	}
}

func drainIDs(t *testing.T, queue *Queue) []string {
	t.Helper()

	ids := make([]string, 0)
	for {
		command, ok := queue.Dequeue(nil)
		if !ok {
			return ids
		}
		ids = append(ids, command.ID)
	}
}

func commandIDs(commands []Command) []string {
	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		ids = append(ids, command.ID)
	}
	return ids
}

func operationTypes(operations []Operation) []string {
	types := make([]string, 0, len(operations))
	for _, operation := range operations {
		types = append(types, operation.Type)
	}
	return types
}
