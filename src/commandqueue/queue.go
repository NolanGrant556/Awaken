package commandqueue

import "strings"

type Priority string

const (
	PriorityNow   Priority = "now"
	PriorityNext  Priority = "next"
	PriorityLater Priority = "later"
)

type Mode string

const (
	ModePrompt           Mode = "prompt"
	ModeBash             Mode = "bash"
	ModeTaskNotification Mode = "task-notification"
	ModeChannel          Mode = "channel"
)

type Command struct {
	ID                string
	Value             string
	Mode              Mode
	Priority          Priority
	IsMeta            bool
	SkipSlashCommands bool
	OriginKind        string
	AgentID           string
}

type Operation struct {
	Type      string
	CommandID string
	Content   string
}

type Filter func(Command) bool

type Queue struct {
	commands   []Command
	operations []Operation
}

func NewQueue() *Queue {
	return &Queue{}
}

func (q *Queue) Enqueue(command Command) {
	command = normalizeCommand(command, PriorityNext, ModePrompt)
	q.commands = append(q.commands, command)
	q.log("enqueue", command)
}

func (q *Queue) EnqueueNotification(command Command) {
	command = normalizeCommand(command, PriorityLater, ModeTaskNotification)
	q.commands = append(q.commands, command)
	q.log("enqueue", command)
}

func (q *Queue) Peek(filter Filter) (Command, bool) {
	idx := q.bestIndex(filter)
	if idx < 0 {
		return Command{}, false
	}
	return q.commands[idx], true
}

func (q *Queue) Dequeue(filter Filter) (Command, bool) {
	idx := q.bestIndex(filter)
	if idx < 0 {
		return Command{}, false
	}

	command := q.commands[idx]
	q.commands = append(q.commands[:idx], q.commands[idx+1:]...)
	q.log("dequeue", command)
	return command, true
}

func (q *Queue) DequeueAll() []Command {
	if len(q.commands) == 0 {
		return nil
	}

	commands := orderedCommands(q.commands)
	q.commands = nil
	for _, command := range commands {
		q.log("dequeue", command)
	}
	return commands
}

func (q *Queue) DequeueAllMatching(filter Filter) []Command {
	if filter == nil || len(q.commands) == 0 {
		return nil
	}

	matched := make([]Command, 0)
	remaining := q.commands[:0]
	for _, command := range q.commands {
		if filter(command) {
			matched = append(matched, command)
			continue
		}
		remaining = append(remaining, command)
	}
	if len(matched) == 0 {
		q.commands = remaining
		return nil
	}

	q.commands = remaining
	matched = orderedCommands(matched)
	for _, command := range matched {
		q.log("dequeue", command)
	}
	return matched
}

func (q *Queue) RemoveByFilter(filter Filter) []Command {
	if filter == nil || len(q.commands) == 0 {
		return nil
	}

	removed := make([]Command, 0)
	remaining := q.commands[:0]
	for _, command := range q.commands {
		if filter(command) {
			removed = append(removed, command)
			continue
		}
		remaining = append(remaining, command)
	}
	if len(removed) == 0 {
		q.commands = remaining
		return nil
	}

	q.commands = remaining
	for _, command := range removed {
		q.log("remove", command)
	}
	return removed
}

func (q *Queue) Clear() {
	if len(q.commands) == 0 {
		return
	}
	q.commands = nil
	q.operations = append(q.operations, Operation{Type: "clear"})
}

func (q *Queue) Snapshot() []Command {
	return append([]Command(nil), q.commands...)
}

func (q *Queue) Operations() []Operation {
	return append([]Operation(nil), q.operations...)
}

func (q *Queue) Len() int {
	return len(q.commands)
}

func (q *Queue) PopAllEditable(currentInput string, cursorOffset int) (string, int, bool) {
	if len(q.commands) == 0 {
		return currentInput, cursorOffset, false
	}

	editable := make([]Command, 0)
	remaining := q.commands[:0]
	for _, command := range q.commands {
		if IsEditable(command) {
			editable = append(editable, command)
			continue
		}
		remaining = append(remaining, command)
	}
	if len(editable) == 0 {
		q.commands = remaining
		return currentInput, cursorOffset, false
	}

	parts := make([]string, 0, len(editable)+1)
	for _, command := range editable {
		if command.Value != "" {
			parts = append(parts, command.Value)
		}
		q.log("popAll", command)
	}

	prefixLen := len(strings.Join(parts, "\n"))
	if currentInput != "" {
		parts = append(parts, currentInput)
	}

	text := strings.Join(parts, "\n")
	newCursorOffset := cursorOffset
	if prefixLen > 0 {
		newCursorOffset = prefixLen
		if currentInput != "" {
			newCursorOffset += 1 + cursorOffset
		}
	}

	q.commands = remaining
	return text, newCursorOffset, true
}

func IsEditable(command Command) bool {
	if command.IsMeta || isChannel(command) {
		return false
	}
	return command.Mode != ModeTaskNotification
}

func IsVisible(command Command) bool {
	return isChannel(command) || IsEditable(command)
}

func IsSlashCommand(command Command) bool {
	return !command.SkipSlashCommands && strings.HasPrefix(strings.TrimSpace(command.Value), "/")
}

func (q *Queue) bestIndex(filter Filter) int {
	bestIdx := -1
	bestPriority := priorityRank(PriorityLater) + 1
	for idx, command := range q.commands {
		if filter != nil && !filter(command) {
			continue
		}

		priority := priorityRank(command.Priority)
		if priority < bestPriority {
			bestIdx = idx
			bestPriority = priority
		}
	}
	return bestIdx
}

func (q *Queue) log(operationType string, command Command) {
	q.operations = append(q.operations, Operation{
		Type:      operationType,
		CommandID: command.ID,
		Content:   command.Value,
	})
}

func normalizeCommand(command Command, defaultPriority Priority, defaultMode Mode) Command {
	if command.Priority == "" {
		command.Priority = defaultPriority
	}
	if command.Mode == "" {
		command.Mode = defaultMode
	}
	return command
}

func orderedCommands(commands []Command) []Command {
	ordered := make([]Command, 0, len(commands))
	for _, priority := range []Priority{PriorityNow, PriorityNext, PriorityLater} {
		for _, command := range commands {
			if priorityRank(command.Priority) == priorityRank(priority) {
				ordered = append(ordered, command)
			}
		}
	}
	return ordered
}

func priorityRank(priority Priority) int {
	switch priority {
	case PriorityNow:
		return 0
	case PriorityLater:
		return 2
	default:
		return 1
	}
}

func isChannel(command Command) bool {
	return command.Mode == ModeChannel || command.OriginKind == "channel"
}
