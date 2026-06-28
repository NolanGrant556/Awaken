package cleanupregistry

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRegisterUnregister(t *testing.T) {
	registry := NewWithMode(RunModeSequential)
	var calls []string

	unregisterFirst := registry.Register("same", func(context.Context) error {
		calls = append(calls, "first")
		return nil
	})
	unregisterSecond := registry.Register("same", func(context.Context) error {
		calls = append(calls, "second")
		return nil
	})

	result := registry.Run(context.Background())
	if result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("expected two successes, got %+v", result)
	}
	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatalf("expected duplicate names to preserve registration order, got %v", calls)
	}

	unregisterFirst()
	unregisterFirst()
	calls = nil
	result = registry.Run(context.Background())
	if result.Succeeded != 1 || len(result.Handlers) != 1 {
		t.Fatalf("expected one remaining handler, got %+v", result)
	}
	if !reflect.DeepEqual(calls, []string{"second"}) {
		t.Fatalf("expected only second handler after unregister, got %v", calls)
	}

	unregisterSecond()
	unregisterSecond()
	result = registry.Run(context.Background())
	if len(result.Handlers) != 0 || result.Succeeded != 0 || result.Failed != 0 {
		t.Fatalf("expected empty result after unregistering all handlers, got %+v", result)
	}
}

func TestFailureDoesNotBlockOtherHandlers(t *testing.T) {
	registry := NewWithMode(RunModeSequential)
	errBoom := errors.New("boom")
	var calls []string

	registry.Register("bad", func(context.Context) error {
		calls = append(calls, "bad")
		return errBoom
	})
	registry.Register("good", func(context.Context) error {
		calls = append(calls, "good")
		return nil
	})

	result := registry.Run(context.Background())
	if result.Failed != 1 || result.Succeeded != 1 {
		t.Fatalf("expected one failure and one success, got %+v", result)
	}
	if !reflect.DeepEqual(calls, []string{"bad", "good"}) {
		t.Fatalf("expected later handler to run after failure, got %v", calls)
	}
	errs := result.Errors()
	if len(errs) != 1 || !errors.Is(errs[0], errBoom) {
		t.Fatalf("expected collected error %v, got %v", errBoom, errs)
	}
}

func TestPanicRecovery(t *testing.T) {
	registry := NewWithMode(RunModeSequential)
	var calls []string

	registry.Register("panic", func(context.Context) error {
		calls = append(calls, "panic")
		panic("explode")
	})
	registry.Register("after", func(context.Context) error {
		calls = append(calls, "after")
		return nil
	})

	result := registry.Run(context.Background())
	if result.Failed != 1 || result.Succeeded != 1 {
		t.Fatalf("expected one recovered panic and one success, got %+v", result)
	}
	if !reflect.DeepEqual(calls, []string{"panic", "after"}) {
		t.Fatalf("expected handler after panic to run, got %v", calls)
	}
	if len(result.Errors()) != 1 || !strings.Contains(result.Errors()[0].Error(), "panicked") {
		t.Fatalf("expected panic error, got %v", result.Errors())
	}
}

func TestSnapshotIsolation(t *testing.T) {
	registry := NewWithMode(RunModeSequential)
	var calls []string
	var unregisterSecond func()

	registry.Register("first", func(context.Context) error {
		calls = append(calls, "first")
		unregisterSecond()
		registry.Register("third", func(context.Context) error {
			calls = append(calls, "third")
			return nil
		})
		return nil
	})
	unregisterSecond = registry.Register("second", func(context.Context) error {
		calls = append(calls, "second")
		return nil
	})

	firstResult := registry.Run(context.Background())
	if firstResult.Succeeded != 2 || firstResult.Failed != 0 {
		t.Fatalf("expected current snapshot to run two handlers, got %+v", firstResult)
	}
	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatalf("expected in-run mutations not to affect current snapshot, got %v", calls)
	}

	calls = nil
	secondResult := registry.Run(context.Background())
	if secondResult.Succeeded != 2 || secondResult.Failed != 0 {
		t.Fatalf("expected later snapshot to include first and third, got %+v", secondResult)
	}
	if !reflect.DeepEqual(calls, []string{"first", "third"}) {
		t.Fatalf("expected later snapshot to reflect mutations, got %v", calls)
	}
}

func TestContextCancellationAndTimeout(t *testing.T) {
	registry := NewWithMode(RunModeSequential)

	registry.Register("waits-for-context", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	result := registry.Run(ctx)
	if result.Failed != 1 || result.Succeeded != 0 {
		t.Fatalf("expected context timeout failure, got %+v", result)
	}
	if len(result.Errors()) != 1 || !errors.Is(result.Errors()[0], context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", result.Errors())
	}
	if result.Handlers[0].Duration <= 0 {
		t.Fatalf("expected duration to be recorded, got %s", result.Handlers[0].Duration)
	}
}

func TestSequentialOrder(t *testing.T) {
	registry := NewWithMode(RunModeSequential)
	var calls []string

	registry.Register("one", func(context.Context) error {
		calls = append(calls, "one")
		return nil
	})
	registry.Register("two", func(context.Context) error {
		calls = append(calls, "two")
		return nil
	})
	registry.Register("three", func(context.Context) error {
		calls = append(calls, "three")
		return nil
	})

	result := registry.Run(context.Background())
	if result.Succeeded != 3 || result.Failed != 0 {
		t.Fatalf("expected all handlers to succeed, got %+v", result)
	}
	if !reflect.DeepEqual(calls, []string{"one", "two", "three"}) {
		t.Fatalf("expected sequential registration order, got %v", calls)
	}
}

func TestParallelReturnsDeterministicResultOrder(t *testing.T) {
	registry := New()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	registry.Register("slow-first", func(context.Context) error {
		wg.Done()
		<-release
		return nil
	})
	registry.Register("fast-second", func(context.Context) error {
		wg.Wait()
		return nil
	})

	done := make(chan Result, 1)
	go func() {
		done <- registry.Run(context.Background())
	}()

	wg.Wait()
	close(release)
	result := <-done
	if result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("expected both handlers to succeed, got %+v", result)
	}

	names := []string{result.Handlers[0].Name, result.Handlers[1].Name}
	if !reflect.DeepEqual(names, []string{"slow-first", "fast-second"}) {
		t.Fatalf("expected deterministic registration-order results, got %v", names)
	}
}
