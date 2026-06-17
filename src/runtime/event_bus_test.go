package runtime

import (
	"testing"
	"time"
)

func TestEventBusDropsSlowSubscriberWithoutBlockingAppend(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe()

	for i := 0; i < cap(sub.Events); i++ {
		bus.Append(Event{Message: "queued"}, false)
	}

	done := make(chan struct{})
	go func() {
		bus.Append(Event{Message: "drop slow subscriber"}, false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Append blocked on a full subscriber channel")
	}

	bus.mu.Lock()
	subscriberCount := len(bus.subscribers)
	bus.mu.Unlock()
	if subscriberCount != 0 {
		t.Fatalf("expected slow subscriber to be removed, got %d subscribers", subscriberCount)
	}

	sub.Close()
}
