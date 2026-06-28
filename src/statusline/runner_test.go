package statusline

import (
	"context"
	"errors"
	"testing"
	"time"
)

type manualTimerFactory struct {
	timers []*manualTimer
}

func (f *manualTimerFactory) afterFunc(_ time.Duration, fn func()) timer {
	t := &manualTimer{fn: fn}
	f.timers = append(f.timers, t)
	return t
}

func (f *manualTimerFactory) fire(index int) {
	f.timers[index].fire()
}

type manualTimer struct {
	stopped bool
	fn      func()
}

func (t *manualTimer) Stop() bool {
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

func (t *manualTimer) fire() {
	if t.stopped {
		return
	}
	t.stopped = true
	go t.fn()
}

type recordingRenderer struct {
	calls   chan Input
	results chan renderResult
}

type renderResult struct {
	text string
	err  error
}

func newRecordingRenderer() *recordingRenderer {
	return &recordingRenderer{
		calls:   make(chan Input, 10),
		results: make(chan renderResult, 10),
	}
}

func (r *recordingRenderer) Render(_ context.Context, input Input) (string, error) {
	r.calls <- input
	result := <-r.results
	return result.text, result.err
}

func TestRunnerRendersOnlyLastScheduledInput(t *testing.T) {
	renderer := newRecordingRenderer()
	renderer.results <- renderResult{text: "second text"}
	timers := &manualTimerFactory{}
	runner := NewRunner(renderer, time.Second)
	runner.timerFactory = timers.afterFunc
	defer runner.Close()

	runner.Schedule(context.Background(), Input{SessionID: "first"})
	runner.Schedule(context.Background(), Input{SessionID: "second"})

	if len(timers.timers) != 2 {
		t.Fatalf("expected 2 timers, got %d", len(timers.timers))
	}
	timers.fire(0)
	timers.fire(1)

	call := waitForInput(t, renderer.calls)
	if call.SessionID != "second" {
		t.Fatalf("expected only second input to render, got %q", call.SessionID)
	}
	waitForText(t, runner, "second text")

	select {
	case extra := <-renderer.calls:
		t.Fatalf("unexpected extra render for %q", extra.SessionID)
	default:
	}
}

func TestRunnerCancelsInFlightRender(t *testing.T) {
	renderer := &blockingRenderer{
		started:  make(chan string, 2),
		canceled: make(chan string, 1),
		done:     make(chan struct{}),
	}
	timers := &manualTimerFactory{}
	runner := NewRunner(renderer, time.Second)
	runner.timerFactory = timers.afterFunc
	defer runner.Close()

	runner.Schedule(context.Background(), Input{SessionID: "first"})
	timers.fire(0)
	if got := waitForString(t, renderer.started); got != "first" {
		t.Fatalf("expected first render to start, got %q", got)
	}

	runner.Schedule(context.Background(), Input{SessionID: "second"})
	if got := waitForString(t, renderer.canceled); got != "first" {
		t.Fatalf("expected first render to be canceled, got %q", got)
	}

	close(renderer.done)
	timers.fire(1)
	if got := waitForString(t, renderer.started); got != "second" {
		t.Fatalf("expected second render to start, got %q", got)
	}
	waitForText(t, runner, "rendered second")

	if runner.LastError() != nil {
		t.Fatalf("expected canceled stale render not to set LastError, got %v", runner.LastError())
	}
}

func TestRunnerRenderErrorDoesNotOverwriteLastText(t *testing.T) {
	renderer := newRecordingRenderer()
	renderer.results <- renderResult{text: "first text"}
	renderer.results <- renderResult{err: errors.New("render failed")}
	timers := &manualTimerFactory{}
	runner := NewRunner(renderer, time.Second)
	runner.timerFactory = timers.afterFunc
	defer runner.Close()

	runner.Schedule(context.Background(), Input{SessionID: "first"})
	timers.fire(0)
	waitForInput(t, renderer.calls)
	waitForText(t, runner, "first text")

	runner.Schedule(context.Background(), Input{SessionID: "second"})
	timers.fire(1)
	waitForInput(t, renderer.calls)
	waitForError(t, runner)

	if got := runner.LastText(); got != "first text" {
		t.Fatalf("expected error not to overwrite LastText, got %q", got)
	}
}

type blockingRenderer struct {
	started  chan string
	canceled chan string
	done     chan struct{}
}

func (r *blockingRenderer) Render(ctx context.Context, input Input) (string, error) {
	r.started <- input.SessionID
	select {
	case <-ctx.Done():
		r.canceled <- input.SessionID
		return "", ctx.Err()
	case <-r.done:
		return "rendered " + input.SessionID, nil
	}
}

func waitForInput(t *testing.T, ch <-chan Input) Input {
	t.Helper()
	select {
	case input := <-ch:
		return input
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for render input")
	}
	return Input{}
}

func waitForString(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for string")
	}
	return ""
}

func waitForText(t *testing.T, runner *Runner, expected string) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for LastText %q, got %q", expected, runner.LastText())
		case <-tick.C:
			if runner.LastText() == expected {
				return
			}
		}
	}
}

func waitForError(t *testing.T, runner *Runner) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for LastError")
		case <-tick.C:
			if runner.LastError() != nil {
				return
			}
		}
	}
}
