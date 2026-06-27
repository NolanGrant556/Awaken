package commandstate

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type State string

const (
	StateStarted   State = "started"
	StateCompleted State = "completed"
)

var (
	ErrCommandNotStarted       = errors.New("command not started")
	ErrCommandAlreadyStarted   = errors.New("command already started")
	ErrCommandAlreadyCompleted = errors.New("command already completed")
)

type Metadata map[string]string

type Event struct {
	UUID     string
	State    State
	Time     time.Time
	Metadata Metadata
}

type Listener func(Event)

type CommandSnapshot struct {
	UUID        string
	State       State
	StartedAt   time.Time
	CompletedAt time.Time
	Metadata    Metadata
}

type Tracker struct {
	mu       sync.RWMutex
	commands map[string]CommandSnapshot
	listener Listener
}

func NewTracker() *Tracker {
	return &Tracker{
		commands: make(map[string]CommandSnapshot),
	}
}

func (t *Tracker) SetListener(listener Listener) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.listener = listener
}

func (t *Tracker) Start(uuid string, metadata Metadata) error {
	now := time.Now()

	t.mu.Lock()
	if command, exists := t.commands[uuid]; exists {
		t.mu.Unlock()
		if command.State == StateCompleted {
			return fmt.Errorf("%w: %s", ErrCommandAlreadyCompleted, uuid)
		}
		return fmt.Errorf("%w: %s", ErrCommandAlreadyStarted, uuid)
	}

	command := CommandSnapshot{
		UUID:      uuid,
		State:     StateStarted,
		StartedAt: now,
		Metadata:  cloneMetadata(metadata),
	}
	t.commands[uuid] = command
	listener := t.listener
	event := Event{
		UUID:     uuid,
		State:    StateStarted,
		Time:     now,
		Metadata: cloneMetadata(metadata),
	}
	t.mu.Unlock()

	notify(listener, event)
	return nil
}

func (t *Tracker) Complete(uuid string, metadata Metadata) error {
	now := time.Now()

	t.mu.Lock()
	command, exists := t.commands[uuid]
	if !exists {
		t.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrCommandNotStarted, uuid)
	}
	if command.State == StateCompleted {
		t.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrCommandAlreadyCompleted, uuid)
	}

	command.State = StateCompleted
	command.CompletedAt = now
	command.Metadata = mergeMetadata(command.Metadata, metadata)
	t.commands[uuid] = command
	listener := t.listener
	event := Event{
		UUID:     uuid,
		State:    StateCompleted,
		Time:     now,
		Metadata: cloneMetadata(metadata),
	}
	t.mu.Unlock()

	notify(listener, event)
	return nil
}

func (t *Tracker) Snapshot() map[string]CommandSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	snapshot := make(map[string]CommandSnapshot, len(t.commands))
	for uuid, command := range t.commands {
		command.Metadata = cloneMetadata(command.Metadata)
		snapshot[uuid] = command
	}
	return snapshot
}

func notify(listener Listener, event Event) {
	if listener != nil {
		listener(event)
	}
}

func cloneMetadata(metadata Metadata) Metadata {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(Metadata, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func mergeMetadata(base Metadata, overlay Metadata) Metadata {
	if len(base) == 0 {
		return cloneMetadata(overlay)
	}
	merged := cloneMetadata(base)
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}
