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
	maxEvents   int
	mu          sync.Mutex
	events      []Event
	nextSeq     uint64
	subscribers map[int]chan Event
	nextID      int
	closed      bool
	done        chan struct{}
}

func NewActivityBus() *EventBus { bus := NewEventBus(); bus.maxEvents = 4096; return bus }

func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
	close(b.done)
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[int]chan Event),
		done:        make(chan struct{}),
	}
}

func (b *EventBus) Append(event Event, terminal bool) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0
	}

	b.nextSeq++
	event.Seq = b.nextSeq

	b.events = append(b.events, event)
	if b.maxEvents > 0 && len(b.events) > b.maxEvents {
		b.events[0] = Event{}
		b.events = b.events[1:]
	}

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
	return event.Seq
}

// RetainAfterClose keeps late child facts available for transcript replay.
// It never reopens the terminated root stream or wakes its subscribers.
func (b *EventBus) RetainAfterClose(event Event) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		return 0
	}
	b.nextSeq++
	event.Seq = b.nextSeq
	b.events = append(b.events, event)
	return event.Seq
}

func (b *EventBus) Subscribe(afterSeq uint64) Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.subscribeLocked(afterSeq)
}

// SubscribeRetained validates the replay range and subscribes under one lock;
// otherwise a burst between validation and subscription can silently lose facts.
func (b *EventBus) SubscribeRetained(afterSeq uint64) (Subscription, uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	last := streamCursorForEventSeq(b.nextSeq)
	if len(b.events) > 0 && b.events[0].Seq > 1 && afterSeq < b.events[0].Seq-1 {
		return Subscription{}, last, ErrActivityCursorExpired
	}
	return b.subscribeLocked(afterSeq), last, nil
}

func (b *EventBus) subscribeLocked(afterSeq uint64) Subscription {
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
