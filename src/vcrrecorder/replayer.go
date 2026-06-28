package vcrrecorder

import (
	"context"
	"time"
)

type ReplayOptions struct {
	PreserveIntervals bool
	Sleep             func(context.Context, time.Duration) error
}

type Replayer struct {
	events            []Event
	preserveIntervals bool
	sleep             func(context.Context, time.Duration) error
}

func NewReplayer(recording Recording, opts ReplayOptions) *Replayer {
	events := append([]Event(nil), recording.Events...)
	sleep := opts.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	return &Replayer{
		events:            events,
		preserveIntervals: opts.PreserveIntervals,
		sleep:             sleep,
	}
}

func (r *Replayer) Events() []Event {
	if r == nil {
		return nil
	}
	return append([]Event(nil), r.events...)
}

func (r *Replayer) Replay(ctx context.Context, emit func(Event) error) error {
	if r == nil || emit == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for i, event := range r.events {
		if i > 0 && r.preserveIntervals {
			delay := event.Timestamp.Sub(r.events[i-1].Timestamp)
			if delay > 0 {
				if err := r.sleep(ctx, delay); err != nil {
					return err
				}
			}
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
