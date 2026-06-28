package transportbatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var (
	ErrClosed          = errors.New("transportbatch: uploader closed")
	ErrBatchTooLarge   = errors.New("transportbatch: enqueue batch exceeds max queue size")
	ErrInvalidItemSize = errors.New("transportbatch: item byte size must be non-negative")
)

type RetryableError struct {
	Err        error
	RetryAfter time.Duration
}

func (e RetryableError) Error() string {
	if e.Err == nil {
		return "retryable error"
	}
	return e.Err.Error()
}

func (e RetryableError) Unwrap() error {
	return e.Err
}

func (e RetryableError) RetryAfterDelay() time.Duration {
	return e.RetryAfter
}

type retryAfterError interface {
	RetryAfterDelay() time.Duration
}

type ItemDrop[T any] struct {
	Item T
	Err  error
}

type Config[T any] struct {
	MaxBatchSize  int
	MaxBatchBytes int
	MaxQueueSize  int
	Send          func(context.Context, []T) error

	BaseDelay time.Duration
	MaxDelay  time.Duration
	Jitter    time.Duration

	MaxConsecutiveFailures int
	OnBatchDropped         func([]T, int)

	ItemBytes     func(T) (int, error)
	OnItemDropped func(ItemDrop[T])

	Sleep      func(context.Context, time.Duration) error
	JitterFunc func(time.Duration) time.Duration
}

type Uploader[T any] struct {
	cfg Config[T]

	ctx    context.Context
	cancel context.CancelFunc

	mu                  sync.Mutex
	pending             []T
	pendingAtClose      int
	draining            bool
	closed              bool
	droppedBatches      int
	backpressureWaiters []chan struct{}
	flushWaiters        []chan error
}

func NewUploader[T any](cfg Config[T]) (*Uploader[T], error) {
	if cfg.MaxBatchSize <= 0 {
		return nil, fmt.Errorf("transportbatch: MaxBatchSize must be greater than zero")
	}
	if cfg.MaxBatchBytes < 0 {
		return nil, fmt.Errorf("transportbatch: MaxBatchBytes must not be negative")
	}
	if cfg.MaxQueueSize <= 0 {
		return nil, fmt.Errorf("transportbatch: MaxQueueSize must be greater than zero")
	}
	if cfg.Send == nil {
		return nil, fmt.Errorf("transportbatch: Send must not be nil")
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = cfg.BaseDelay
	}
	if cfg.MaxDelay < cfg.BaseDelay {
		return nil, fmt.Errorf("transportbatch: MaxDelay must be greater than or equal to BaseDelay")
	}
	if cfg.Jitter < 0 {
		return nil, fmt.Errorf("transportbatch: Jitter must not be negative")
	}
	if cfg.MaxConsecutiveFailures < 0 {
		return nil, fmt.Errorf("transportbatch: MaxConsecutiveFailures must not be negative")
	}
	if cfg.ItemBytes == nil {
		cfg.ItemBytes = jsonItemBytes[T]
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleep
	}
	if cfg.JitterFunc == nil {
		cfg.JitterFunc = randomJitter
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Uploader[T]{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (u *Uploader[T]) Enqueue(ctx context.Context, items ...T) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) > u.cfg.MaxQueueSize {
		return ErrBatchTooLarge
	}
	if ctx == nil {
		ctx = context.Background()
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	for len(u.pending)+len(items) > u.cfg.MaxQueueSize && !u.closed {
		waiter := make(chan struct{})
		u.backpressureWaiters = append(u.backpressureWaiters, waiter)
		u.mu.Unlock()
		select {
		case <-waiter:
		case <-ctx.Done():
			u.removeBackpressureWaiter(waiter)
			u.mu.Lock()
			return ctx.Err()
		}
		u.mu.Lock()
	}
	if u.closed {
		return ErrClosed
	}

	u.pending = append(u.pending, items...)
	u.startDrainLocked()
	return nil
}

func (u *Uploader[T]) Flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return ErrClosed
	}
	if len(u.pending) == 0 && !u.draining {
		u.mu.Unlock()
		return nil
	}

	waiter := make(chan error, 1)
	u.flushWaiters = append(u.flushWaiters, waiter)
	u.startDrainLocked()
	u.mu.Unlock()

	select {
	case err := <-waiter:
		return err
	case <-ctx.Done():
		u.removeFlushWaiter(waiter)
		return ctx.Err()
	}
}

func (u *Uploader[T]) Close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return
	}
	u.closed = true
	u.pendingAtClose = len(u.pending)
	u.pending = nil
	u.cancel()
	u.releaseBackpressureLocked()
	u.resolveFlushLocked(ErrClosed)
}

func (u *Uploader[T]) PendingCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return u.pendingAtClose
	}
	return len(u.pending)
}

func (u *Uploader[T]) DroppedBatchCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.droppedBatches
}

func (u *Uploader[T]) startDrainLocked() {
	if u.draining || u.closed || len(u.pending) == 0 {
		return
	}
	u.draining = true
	go u.drain()
}

func (u *Uploader[T]) drain() {
	failures := 0
	for {
		u.mu.Lock()
		if u.closed || len(u.pending) == 0 {
			u.draining = false
			if u.closed {
				u.resolveFlushLocked(ErrClosed)
			} else {
				u.resolveFlushLocked(nil)
			}
			u.mu.Unlock()
			return
		}

		batch, drops := u.takeBatchLocked()
		if len(drops) > 0 {
			u.releaseBackpressureLocked()
		}
		u.mu.Unlock()

		for _, drop := range drops {
			if u.cfg.OnItemDropped != nil {
				u.cfg.OnItemDropped(drop)
			}
		}
		if len(batch) == 0 {
			continue
		}

		err := u.cfg.Send(u.ctx, batch)
		u.mu.Lock()
		closed := u.closed
		u.mu.Unlock()
		if closed {
			continue
		}
		if err == nil {
			failures = 0
			u.mu.Lock()
			u.releaseBackpressureLocked()
			u.mu.Unlock()
			continue
		}

		failures++
		if u.shouldDropBatch(failures) {
			dropped := false
			u.mu.Lock()
			if !u.closed {
				u.droppedBatches++
				u.releaseBackpressureLocked()
				dropped = true
			}
			u.mu.Unlock()
			if dropped && u.cfg.OnBatchDropped != nil {
				u.cfg.OnBatchDropped(batch, failures)
			}
			failures = 0
			continue
		}

		u.mu.Lock()
		if !u.closed {
			u.pending = append(append(make([]T, 0, len(batch)+len(u.pending)), batch...), u.pending...)
		}
		u.mu.Unlock()

		if sleepErr := u.cfg.Sleep(u.ctx, u.retryDelay(failures, err)); sleepErr != nil && errors.Is(sleepErr, context.Canceled) {
			continue
		}
	}
}

func (u *Uploader[T]) takeBatchLocked() ([]T, []ItemDrop[T]) {
	if u.cfg.MaxBatchBytes <= 0 {
		batchSize := min(u.cfg.MaxBatchSize, len(u.pending))
		batch := append([]T(nil), u.pending[:batchSize]...)
		u.pending = u.pending[batchSize:]
		return batch, nil
	}

	bytes := 0
	count := 0
	var drops []ItemDrop[T]
	for count < len(u.pending) && count < u.cfg.MaxBatchSize {
		itemBytes, err := u.cfg.ItemBytes(u.pending[count])
		if err != nil {
			drops = append(drops, ItemDrop[T]{Item: u.pending[count], Err: err})
			u.pending = append(u.pending[:count], u.pending[count+1:]...)
			continue
		}
		if itemBytes < 0 {
			drops = append(drops, ItemDrop[T]{Item: u.pending[count], Err: ErrInvalidItemSize})
			u.pending = append(u.pending[:count], u.pending[count+1:]...)
			continue
		}
		if count > 0 && bytes+itemBytes > u.cfg.MaxBatchBytes {
			break
		}
		bytes += itemBytes
		count++
	}
	if count == 0 {
		return nil, drops
	}

	batch := append([]T(nil), u.pending[:count]...)
	u.pending = u.pending[count:]
	return batch, drops
}

func (u *Uploader[T]) shouldDropBatch(failures int) bool {
	return u.cfg.MaxConsecutiveFailures > 0 && failures >= u.cfg.MaxConsecutiveFailures
}

func (u *Uploader[T]) retryDelay(failures int, err error) time.Duration {
	var retryable retryAfterError
	if errors.As(err, &retryable) && retryable.RetryAfterDelay() > 0 {
		return u.clampedDelay(retryable.RetryAfterDelay()) + u.jitter()
	}

	delay := u.cfg.BaseDelay
	for i := 1; i < failures; i++ {
		if delay >= u.cfg.MaxDelay/2 {
			delay = u.cfg.MaxDelay
			break
		}
		delay *= 2
	}
	return u.clampedDelay(delay) + u.jitter()
}

func (u *Uploader[T]) clampedDelay(delay time.Duration) time.Duration {
	if delay < u.cfg.BaseDelay {
		return u.cfg.BaseDelay
	}
	if delay > u.cfg.MaxDelay {
		return u.cfg.MaxDelay
	}
	return delay
}

func (u *Uploader[T]) jitter() time.Duration {
	jitter := u.cfg.JitterFunc(u.cfg.Jitter)
	if jitter < 0 {
		return 0
	}
	return jitter
}

func (u *Uploader[T]) removeBackpressureWaiter(waiter chan struct{}) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i, candidate := range u.backpressureWaiters {
		if candidate == waiter {
			u.backpressureWaiters = append(u.backpressureWaiters[:i], u.backpressureWaiters[i+1:]...)
			return
		}
	}
}

func (u *Uploader[T]) removeFlushWaiter(waiter chan error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i, candidate := range u.flushWaiters {
		if candidate == waiter {
			u.flushWaiters = append(u.flushWaiters[:i], u.flushWaiters[i+1:]...)
			return
		}
	}
}

func (u *Uploader[T]) releaseBackpressureLocked() {
	waiters := u.backpressureWaiters
	u.backpressureWaiters = nil
	for _, waiter := range waiters {
		close(waiter)
	}
}

func (u *Uploader[T]) resolveFlushLocked(err error) {
	waiters := u.flushWaiters
	u.flushWaiters = nil
	for _, waiter := range waiters {
		waiter <- err
		close(waiter)
	}
}

func jsonItemBytes[T any](item T) (int, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func randomJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max)))
}
