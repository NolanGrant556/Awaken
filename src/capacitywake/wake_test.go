package capacitywake

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWaitReturnsWhenParentCanceled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	wake := New(parent)

	done := make(chan error, 1)
	go func() {
		done <- wake.Wait(context.Background(), time.Hour)
	}()

	cancel()

	if err := receiveErr(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWaitReturnsWhenWoken(t *testing.T) {
	wake := New(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- wake.Wait(context.Background(), time.Hour)
	}()

	time.Sleep(10 * time.Millisecond)
	wake.Wake()

	if err := receiveErr(t, done); err != nil {
		t.Fatalf("expected nil error after wake, got %v", err)
	}
}

func TestWakeRearmsFutureWaits(t *testing.T) {
	wake := New(context.Background())
	first := make(chan error, 1)

	go func() {
		first <- wake.Wait(context.Background(), time.Hour)
	}()

	time.Sleep(10 * time.Millisecond)
	wake.Wake()
	if err := receiveErr(t, first); err != nil {
		t.Fatalf("expected first wait to wake with nil error, got %v", err)
	}

	second := make(chan error, 1)
	go func() {
		second <- wake.Wait(context.Background(), time.Hour)
	}()

	select {
	case err := <-second:
		t.Fatalf("expected re-armed wait to block, got %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	wake.Wake()
	if err := receiveErr(t, second); err != nil {
		t.Fatalf("expected second wait to wake with nil error, got %v", err)
	}
}

func TestWakeReleasesMultipleWaiters(t *testing.T) {
	wake := New(context.Background())
	const waiterCount = 5
	done := make(chan error, waiterCount)

	for i := 0; i < waiterCount; i++ {
		go func() {
			done <- wake.Wait(context.Background(), time.Hour)
		}()
	}

	time.Sleep(10 * time.Millisecond)
	wake.Wake()

	for i := 0; i < waiterCount; i++ {
		if err := receiveErr(t, done); err != nil {
			t.Fatalf("waiter %d: expected nil error after wake, got %v", i, err)
		}
	}
}

func TestWaitTimesOutNormally(t *testing.T) {
	wake := New(context.Background())

	start := time.Now()
	if err := wake.Wait(context.Background(), 15*time.Millisecond); err != nil {
		t.Fatalf("expected nil error on timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("expected wait to last until timeout, returned after %s", elapsed)
	}
}

func TestConcurrentWakeIsSafeAndRearms(t *testing.T) {
	wake := New(context.Background())
	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				wake.Wake()
			}
		}()
	}
	wg.Wait()

	start := time.Now()
	if err := wake.Wait(context.Background(), 15*time.Millisecond); err != nil {
		t.Fatalf("expected nil error after concurrent wakes, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Fatalf("expected final wait to use a fresh channel, returned after %s", elapsed)
	}
}

func receiveErr(t *testing.T, ch <-chan error) error {
	t.Helper()

	select {
	case err := <-ch:
		return err
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for Wait to return")
		return nil
	}
}
