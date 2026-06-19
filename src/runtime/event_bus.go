package runtime

import "sync"

const subscriberBufferSize = 2048

type Subscription struct {
	History []Event
	Events  <-chan Event
	Close   func()
}

type EventBus struct {
	mu          sync.Mutex
	events      []Event
	nextSeq     uint64
	subscribers map[int]chan Event
	nextID      int
	closed      bool
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[int]chan Event),
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
		return Subscription{History: history}
	}

	id := b.nextID
	b.nextID++
	ch := make(chan Event, subscriberBufferSize)
	b.subscribers[id] = ch

	return Subscription{
		History: history,
		Events:  ch,
		Close: func() {
			b.mu.Lock()
			delete(b.subscribers, id)
			b.mu.Unlock()
		},
	}
}
