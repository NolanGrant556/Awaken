package runtime

import (
	"testing"
	"time"
)

func TestEventBusDropsSlowSubscriberWithoutBlockingAppend(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe(0)

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

// TestEventBusAssignsStrictlyIncreasingSeq verifies that Append assigns a
// strictly increasing Seq starting at 1 to each event, and that the seq is
// reflected both in history replay and live delivery.
func TestEventBusAssignsStrictlyIncreasingSeq(t *testing.T) {
	bus := NewEventBus()
	sub := bus.Subscribe(0)
	if sub.Close != nil {
		defer sub.Close()
	}

	for i := 0; i < 3; i++ {
		bus.Append(Event{Message: "event"}, false)
	}

	if len(sub.History) != 0 {
		t.Fatalf("expected empty history for subscriber created before any events, got %d", len(sub.History))
	}

	var live []Event
	for i := 0; i < 3; i++ {
		select {
		case event := <-sub.Events:
			live = append(live, event)
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("timed out waiting for live event %d", i)
		}
	}

	for i, event := range live {
		expected := uint64(i + 1)
		if event.Seq != expected {
			t.Fatalf("event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
	}

	// A fresh subscriber with afterSeq=0 should see all three events with the
	// same strictly increasing seq values in history.
	replay := bus.Subscribe(0)
	if len(replay.History) != 3 {
		t.Fatalf("expected 3 replayed events, got %d", len(replay.History))
	}
	for i, event := range replay.History {
		expected := uint64(i + 1)
		if event.Seq != expected {
			t.Fatalf("replay event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
	}
}

// TestEventBusSubscribeAfterSeqFiltersHistory verifies that Subscribe(afterSeq)
// only returns history events whose Seq is strictly greater than afterSeq, and
// that the live channel still receives all subsequent events.
func TestEventBusSubscribeAfterSeqFiltersHistory(t *testing.T) {
	bus := NewEventBus()

	for i := 0; i < 3; i++ {
		bus.Append(Event{Message: "history"}, false)
	}

	// afterSeq=1 should exclude the first event (seq=1) and include seq=2,3.
	sub := bus.Subscribe(1)
	if sub.Close != nil {
		defer sub.Close()
	}

	if len(sub.History) != 2 {
		t.Fatalf("expected 2 replayed events after seq=1, got %d", len(sub.History))
	}
	for i, event := range sub.History {
		expected := uint64(i + 2)
		if event.Seq != expected {
			t.Fatalf("replay event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
	}

	// Live events appended after subscription should still arrive with seq > 1.
	bus.Append(Event{Message: "live"}, false)

	select {
	case event := <-sub.Events:
		if event.Seq != 4 {
			t.Fatalf("expected live event seq 4, got %d", event.Seq)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for live event")
	}
}

// TestEventBusSubscribeAfterSeqEqualToLastReturnsNoHistory verifies that
// subscribing with afterSeq equal to the last seen seq returns an empty
// history but still receives new live events.
func TestEventBusSubscribeAfterSeqEqualToLastReturnsNoHistory(t *testing.T) {
	bus := NewEventBus()

	for i := 0; i < 3; i++ {
		bus.Append(Event{Message: "history"}, false)
	}

	sub := bus.Subscribe(3)
	if sub.Close != nil {
		defer sub.Close()
	}

	if len(sub.History) != 0 {
		t.Fatalf("expected 0 replayed events after seq=3, got %d", len(sub.History))
	}

	bus.Append(Event{Message: "live"}, false)

	select {
	case event := <-sub.Events:
		if event.Seq != 4 {
			t.Fatalf("expected live event seq 4, got %d", event.Seq)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for live event")
	}
}

// TestEventBusSubscribeAfterSeqBeyondLastReturnsNoHistory verifies that
// subscribing with afterSeq greater than the last seen seq returns an empty
// history.
func TestEventBusSubscribeAfterSeqBeyondLastReturnsNoHistory(t *testing.T) {
	bus := NewEventBus()

	for i := 0; i < 3; i++ {
		bus.Append(Event{Message: "history"}, false)
	}

	sub := bus.Subscribe(100)
	if sub.Close != nil {
		defer sub.Close()
	}

	if len(sub.History) != 0 {
		t.Fatalf("expected 0 replayed events after seq=100, got %d", len(sub.History))
	}
}
