package streamgate

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestGateQueuesAndEndsInOrder(t *testing.T) {
	gate := NewGate[int]()

	gate.Start()
	if !gate.Active() {
		t.Fatal("expected gate to be active after Start")
	}
	if !gate.Enqueue(1, 2) {
		t.Fatal("expected active gate to queue items")
	}
	if !gate.Enqueue(3) {
		t.Fatal("expected active gate to queue more items")
	}

	items := gate.End()
	if !reflect.DeepEqual(items, []int{1, 2, 3}) {
		t.Fatalf("expected queued items in order, got %#v", items)
	}
	if gate.Active() {
		t.Fatal("expected gate to be inactive after End")
	}
	if count := gate.PendingCount(); count != 0 {
		t.Fatalf("expected no pending items after End, got %d", count)
	}
}

func TestGateInactiveEnqueueSendsDirect(t *testing.T) {
	var gate Gate[string]

	if gate.Enqueue("direct") {
		t.Fatal("expected inactive gate to report direct send")
	}
	if count := gate.PendingCount(); count != 0 {
		t.Fatalf("expected inactive enqueue to leave no pending items, got %d", count)
	}
}

func TestGateDropClearsPending(t *testing.T) {
	var gate Gate[string]

	gate.Start()
	gate.Enqueue("a", "b", "c")

	if dropped := gate.Drop(); dropped != 3 {
		t.Fatalf("expected 3 dropped items, got %d", dropped)
	}
	if gate.Active() {
		t.Fatal("expected gate to be inactive after Drop")
	}
	if count := gate.PendingCount(); count != 0 {
		t.Fatalf("expected no pending items after Drop, got %d", count)
	}
	if drained := gate.End(); len(drained) != 0 {
		t.Fatalf("expected Drop to clear drain items, got %#v", drained)
	}
}

func TestGateDeactivatePreservesPending(t *testing.T) {
	var gate Gate[string]

	gate.Start()
	gate.Enqueue("first", "second")
	gate.Deactivate()

	if gate.Active() {
		t.Fatal("expected gate to be inactive after Deactivate")
	}
	if count := gate.PendingCount(); count != 2 {
		t.Fatalf("expected Deactivate to preserve 2 items, got %d", count)
	}
	if gate.Enqueue("direct") {
		t.Fatal("expected enqueue after Deactivate to send direct")
	}

	items := gate.End()
	if !reflect.DeepEqual(items, []string{"first", "second"}) {
		t.Fatalf("expected preserved pending items, got %#v", items)
	}
}

func TestGateConcurrentEnqueue(t *testing.T) {
	var gate Gate[int]
	gate.Start()

	const itemCount = 1000
	var directCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(itemCount)
	for i := 0; i < itemCount; i++ {
		i := i
		go func() {
			defer wg.Done()
			if !gate.Enqueue(i) {
				directCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if directCount.Load() != 0 {
		t.Fatalf("expected all concurrent enqueues to queue, got %d direct sends", directCount.Load())
	}
	if count := gate.PendingCount(); count != itemCount {
		t.Fatalf("expected %d pending items, got %d", itemCount, count)
	}

	items := gate.End()
	if len(items) != itemCount {
		t.Fatalf("expected %d drained items, got %d", itemCount, len(items))
	}
	seen := make(map[int]bool, itemCount)
	for _, item := range items {
		seen[item] = true
	}
	for i := 0; i < itemCount; i++ {
		if !seen[i] {
			t.Fatalf("missing queued item %d", i)
		}
	}
}
