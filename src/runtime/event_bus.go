package runtime

import (
	"sync"

	"awaken/src/protocol/agui"
)

const subscriberBufferSize = 2048

type Subscription struct {
	History []Event
	Events  <-chan Event
	// Done closes only when the run publishes its terminal event. Events may
	// close earlier when a slow subscriber is disconnected, or be nil on replay.
	Done  <-chan struct{}
	Close func()
}

type EventBus struct {
	mu          sync.Mutex
	events      []Event
	nextSeq     uint64
	subscribers map[int]chan Event
	nextID      int
	closed      bool
	done        chan struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[int]chan Event),
		done:        make(chan struct{}),
	}
}

func (b *EventBus) Append(event Event, terminal bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}

	b.nextSeq++
	event.Seq = b.nextSeq

	b.events = append(b.events, event)

	if terminal {
		b.closed = true
	}

	for id, ch := range b.subscribers {
		if terminal {
			select {
			case ch <- event:
			default:
			}
			close(ch)
			delete(b.subscribers, id)
			continue
		}

		select {
		case ch <- event:
		default:
			close(ch)
			delete(b.subscribers, id)
		}
	}

	if terminal {
		b.subscribers = nil
		close(b.done)
	}
}

func (b *EventBus) Subscribe(afterSeq uint64) Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	history := make([]Event, 0, len(b.events))
	for _, event := range b.events {
		if event.Seq > afterSeq {
			history = append(history, event)
		}
	}
	if b.closed {
		return Subscription{History: history, Done: b.done}
	}

	id := b.nextID
	b.nextID++
	ch := make(chan Event, subscriberBufferSize)
	b.subscribers[id] = ch

	return Subscription{
		History: history,
		Events:  ch,
		Done:    b.done,
		Close: func() {
			b.mu.Lock()
			delete(b.subscribers, id)
			b.mu.Unlock()
		},
	}
}

// StreamCursor returns a watermark after every frame slot belonging to the
// latest appended runtime event. The next event always starts above it.
func (b *EventBus) StreamCursor() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return streamCursorForEventSeq(b.nextSeq)
}

// Snapshot returns an atomic copy of all retained stream envelopes plus the
// cursor immediately after them. Active transcript assembly uses this before
// reading the journal, so records published later are left for SSE resume.
func (b *EventBus) Snapshot() ([]Event, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	events := append([]Event(nil), b.events...)
	return events, streamCursorForEventSeq(b.nextSeq)
}

func streamCursorForEventSeq(seq uint64) uint64 {
	if seq == 0 {
		return 0
	}
	return agui.FrameSeq(seq, int(agui.FrameSeqStride-1))
}
