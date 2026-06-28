package transportbatch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestBatchesByMaxBatchSize(t *testing.T) {
	var batches [][]int
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize: 2,
		MaxQueueSize: 10,
		Send: func(ctx context.Context, batch []int) error {
			batches = append(batches, append([]int(nil), batch...))
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), 1, 2, 3, 4, 5); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	want := [][]int{{1, 2}, {3, 4}, {5}}
	if !reflect.DeepEqual(batches, want) {
		t.Fatalf("expected batches %v, got %v", want, batches)
	}
}

func TestBatchesByMaxBatchBytes(t *testing.T) {
	var batches [][]string
	uploader := newTestUploader(t, Config[string]{
		MaxBatchSize:  10,
		MaxBatchBytes: 5,
		MaxQueueSize:  10,
		ItemBytes: func(item string) (int, error) {
			return len(item), nil
		},
		Send: func(ctx context.Context, batch []string) error {
			batches = append(batches, append([]string(nil), batch...))
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), "aa", "bb", "ccc", "d"); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	want := [][]string{{"aa", "bb"}, {"ccc", "d"}}
	if !reflect.DeepEqual(batches, want) {
		t.Fatalf("expected batches %v, got %v", want, batches)
	}
}

func TestOversizedFirstItemIsSentAlone(t *testing.T) {
	var batches [][]string
	uploader := newTestUploader(t, Config[string]{
		MaxBatchSize:  10,
		MaxBatchBytes: 3,
		MaxQueueSize:  10,
		ItemBytes: func(item string) (int, error) {
			return len(item), nil
		},
		Send: func(ctx context.Context, batch []string) error {
			batches = append(batches, append([]string(nil), batch...))
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), "large", "x"); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	want := [][]string{{"large"}, {"x"}}
	if !reflect.DeepEqual(batches, want) {
		t.Fatalf("expected batches %v, got %v", want, batches)
	}
}

func TestUnserializableItemIsDropped(t *testing.T) {
	var batches [][]any
	var drops []ItemDrop[any]
	uploader := newTestUploader(t, Config[any]{
		MaxBatchSize:  10,
		MaxBatchBytes: 100,
		MaxQueueSize:  10,
		OnItemDropped: func(drop ItemDrop[any]) {
			drops = append(drops, drop)
		},
		Send: func(ctx context.Context, batch []any) error {
			batches = append(batches, append([]any(nil), batch...))
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), func() {}, "ok"); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	if len(drops) != 1 || drops[0].Err == nil {
		t.Fatalf("expected one unserializable item drop, got %+v", drops)
	}
	want := [][]any{{"ok"}}
	if !reflect.DeepEqual(batches, want) {
		t.Fatalf("expected batches %v, got %v", want, batches)
	}
}

func TestSendIsSerial(t *testing.T) {
	started := make(chan []int, 2)
	release := make(chan struct{})
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize: 1,
		MaxQueueSize: 10,
		Send: func(ctx context.Context, batch []int) error {
			started <- append([]int(nil), batch...)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	if err := uploader.Enqueue(context.Background(), 1, 2); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if got := waitForBatch(t, started); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("expected first batch [1], got %v", got)
	}
	select {
	case got := <-started:
		t.Fatalf("send was not serial; second batch started before first finished: %v", got)
	case <-time.After(20 * time.Millisecond):
	}

	release <- struct{}{}
	if got := waitForBatch(t, started); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("expected second batch [2], got %v", got)
	}
	release <- struct{}{}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
}

func TestFailureRetryPreservesOrder(t *testing.T) {
	var attempts [][]int
	failOnce := true
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize: 2,
		MaxQueueSize: 10,
		Sleep:        immediateSleep,
		Send: func(ctx context.Context, batch []int) error {
			attempts = append(attempts, append([]int(nil), batch...))
			if failOnce {
				failOnce = false
				return errors.New("temporary")
			}
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), 1, 2, 3); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	want := [][]int{{1, 2}, {1, 2}, {3}}
	if !reflect.DeepEqual(attempts, want) {
		t.Fatalf("expected attempts %v, got %v", want, attempts)
	}
}

func TestRetryAfterOverridesBackoffDelay(t *testing.T) {
	sleeper := &recordingSleeper{}
	failOnce := true
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize: 1,
		MaxQueueSize: 10,
		BaseDelay:    10 * time.Millisecond,
		MaxDelay:     time.Second,
		Sleep:        sleeper.Sleep,
		Send: func(ctx context.Context, batch []int) error {
			if failOnce {
				failOnce = false
				return fmt.Errorf("wrapped: %w", &RetryableError{
					Err:        errors.New("rate limited"),
					RetryAfter: 250 * time.Millisecond,
				})
			}
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), 1); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	want := []time.Duration{250 * time.Millisecond}
	if got := sleeper.Delays(); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected retry delays %v, got %v", want, got)
	}
}

func TestDropAfterMaxConsecutiveFailures(t *testing.T) {
	var attempts [][]int
	var dropped [][]int
	var dropFailures []int
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize:           2,
		MaxQueueSize:           10,
		MaxConsecutiveFailures: 2,
		Sleep:                  immediateSleep,
		OnBatchDropped: func(batch []int, failures int) {
			dropped = append(dropped, append([]int(nil), batch...))
			dropFailures = append(dropFailures, failures)
		},
		Send: func(ctx context.Context, batch []int) error {
			attempts = append(attempts, append([]int(nil), batch...))
			if reflect.DeepEqual(batch, []int{1, 2}) {
				return errors.New("still failing")
			}
			return nil
		},
	})

	if err := uploader.Enqueue(context.Background(), 1, 2, 3); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if err := uploader.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	wantAttempts := [][]int{{1, 2}, {1, 2}, {3}}
	if !reflect.DeepEqual(attempts, wantAttempts) {
		t.Fatalf("expected attempts %v, got %v", wantAttempts, attempts)
	}
	if uploader.DroppedBatchCount() != 1 {
		t.Fatalf("expected one dropped batch, got %d", uploader.DroppedBatchCount())
	}
	if !reflect.DeepEqual(dropped, [][]int{{1, 2}}) || !reflect.DeepEqual(dropFailures, []int{2}) {
		t.Fatalf("unexpected dropped batches %v with failures %v", dropped, dropFailures)
	}
}

func TestFlushWaitsForInFlightSend(t *testing.T) {
	sendStarted := make(chan struct{})
	releaseSend := make(chan struct{})
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize: 1,
		MaxQueueSize: 10,
		Send: func(ctx context.Context, batch []int) error {
			close(sendStarted)
			select {
			case <-releaseSend:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	if err := uploader.Enqueue(context.Background(), 1); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	<-sendStarted

	flushDone := make(chan error, 1)
	go func() {
		flushDone <- uploader.Flush(context.Background())
	}()

	select {
	case err := <-flushDone:
		t.Fatalf("Flush returned before in-flight send completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseSend)
	select {
	case err := <-flushDone:
		if err != nil {
			t.Fatalf("Flush returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for Flush")
	}
}

func TestCloseReleasesFlushAndBackpressureWaiters(t *testing.T) {
	sendStarted := make(chan struct{})
	uploader := newTestUploader(t, Config[int]{
		MaxBatchSize: 1,
		MaxQueueSize: 2,
		Send: func(ctx context.Context, batch []int) error {
			select {
			case <-sendStarted:
			default:
				close(sendStarted)
			}
			<-ctx.Done()
			return ctx.Err()
		},
	})

	if err := uploader.Enqueue(context.Background(), 1, 2); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	<-sendStarted

	enqueueDone := make(chan error, 1)
	go func() {
		enqueueDone <- uploader.Enqueue(context.Background(), 3, 4)
	}()
	select {
	case err := <-enqueueDone:
		t.Fatalf("backpressured Enqueue returned early: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	flushDone := make(chan error, 1)
	go func() {
		flushDone <- uploader.Flush(context.Background())
	}()
	select {
	case err := <-flushDone:
		t.Fatalf("Flush returned early: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	uploader.Close()

	select {
	case err := <-enqueueDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("expected ErrClosed from Enqueue, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for backpressured Enqueue to release")
	}
	select {
	case err := <-flushDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("expected ErrClosed from Flush, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for Flush to release")
	}
}

func newTestUploader[T any](t *testing.T, cfg Config[T]) *Uploader[T] {
	t.Helper()
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = time.Millisecond
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = time.Millisecond
	}
	if cfg.Sleep == nil {
		cfg.Sleep = immediateSleep
	}
	uploader, err := NewUploader(cfg)
	if err != nil {
		t.Fatalf("NewUploader returned error: %v", err)
	}
	t.Cleanup(uploader.Close)
	return uploader
}

func immediateSleep(ctx context.Context, delay time.Duration) error {
	return nil
}

func waitForBatch(t *testing.T, ch <-chan []int) []int {
	t.Helper()
	select {
	case batch := <-ch:
		return batch
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for batch")
		return nil
	}
}

type recordingSleeper struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (s *recordingSleeper) Sleep(ctx context.Context, delay time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delays = append(s.delays, delay)
	return nil
}

func (s *recordingSleeper) Delays() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.delays...)
}
