package statusline

import (
	"context"
	"sync"
	"time"
)

type Renderer interface {
	Render(ctx context.Context, input Input) (string, error)
}

type Runner struct {
	mu           sync.Mutex
	renderer     Renderer
	debounce     time.Duration
	timerFactory timerFactory
	timer        timer
	cancel       context.CancelFunc
	seq          uint64
	lastText     string
	lastError    error
}

type timer interface {
	Stop() bool
}

type timerFactory func(time.Duration, func()) timer

func NewRunner(renderer Renderer, debounce time.Duration) *Runner {
	return &Runner{
		renderer:     renderer,
		debounce:     debounce,
		timerFactory: realTimerFactory,
	}
}

func (r *Runner) Schedule(ctx context.Context, input Input) {
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	seq := r.seq
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	if r.cancel != nil {
		r.cancel()
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	if r.debounce <= 0 {
		go r.render(seq, runCtx, input)
		return
	}

	r.timer = r.timerFactory(r.debounce, func() {
		r.render(seq, runCtx, input)
	})
}

func (r *Runner) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func (r *Runner) LastText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastText
}

func (r *Runner) LastError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastError
}

func (r *Runner) render(seq uint64, ctx context.Context, input Input) {
	r.mu.Lock()
	if seq != r.seq {
		r.mu.Unlock()
		return
	}
	r.timer = nil
	r.mu.Unlock()

	text, err := r.renderer.Render(ctx, input)

	r.mu.Lock()
	defer r.mu.Unlock()
	if seq != r.seq {
		return
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if err != nil {
		r.lastError = err
		return
	}
	r.lastText = text
	r.lastError = nil
}

func realTimerFactory(d time.Duration, fn func()) timer {
	return time.AfterFunc(d, fn)
}
