package runtime

import "sync"

type Subscription struct {
	History []Event
	Events  <-chan Event
	Close   func()
}

type EventBus struct {
	mu          sync.Mutex
	events      []Event
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

func (b *EventBus) Subscribe() Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	history := append([]Event(nil), b.events...)
	if b.closed {
		return Subscription{History: history}
	}

	id := b.nextID
	b.nextID++
	ch := make(chan Event, 32)
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
