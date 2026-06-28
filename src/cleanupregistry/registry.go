package cleanupregistry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var ErrNilHandler = errors.New("cleanupregistry: nil handler")

type RunMode int

const (
	RunModeParallel RunMode = iota
	RunModeSequential
)

type Handler func(context.Context) error

type Registry struct {
	mu       sync.RWMutex
	mode     RunMode
	nextID   uint64
	handlers map[uint64]registeredHandler
}

type registeredHandler struct {
	id   uint64
	name string
	fn   Handler
}

type HandlerResult struct {
	Name      string
	Succeeded bool
	Err       error
	Duration  time.Duration
}

type Result struct {
	Handlers  []HandlerResult
	Succeeded int
	Failed    int
}

func New() *Registry {
	return NewWithMode(RunModeParallel)
}

func NewWithMode(mode RunMode) *Registry {
	return &Registry{
		mode:     mode,
		handlers: make(map[uint64]registeredHandler),
	}
}

func (r *Registry) Register(name string, fn func(context.Context) error) func() {
	if r == nil {
		return func() {}
	}
	if fn == nil {
		fn = func(context.Context) error {
			return ErrNilHandler
		}
	}

	r.mu.Lock()
	if r.handlers == nil {
		r.handlers = make(map[uint64]registeredHandler)
	}
	id := r.nextID
	r.nextID++
	r.handlers[id] = registeredHandler{
		id:   id,
		name: name,
		fn:   Handler(fn),
	}
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.handlers, id)
			r.mu.Unlock()
		})
	}
}

func (r *Registry) Run(ctx context.Context) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return Result{}
	}

	handlers, mode := r.snapshot()
	if len(handlers) == 0 {
		return Result{}
	}

	var results []HandlerResult
	switch mode {
	case RunModeSequential:
		results = runSequential(ctx, handlers)
	default:
		results = runParallel(ctx, handlers)
	}
	return newResult(results)
}

func (r *Registry) snapshot() ([]registeredHandler, RunMode) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handlers := make([]registeredHandler, 0, len(r.handlers))
	for _, handler := range r.handlers {
		handlers = append(handlers, handler)
	}
	sort.Slice(handlers, func(i, j int) bool {
		return handlers[i].id < handlers[j].id
	})
	return handlers, r.mode
}

func runSequential(ctx context.Context, handlers []registeredHandler) []HandlerResult {
	results := make([]HandlerResult, 0, len(handlers))
	for _, handler := range handlers {
		results = append(results, runHandler(ctx, handler))
	}
	return results
}

func runParallel(ctx context.Context, handlers []registeredHandler) []HandlerResult {
	results := make([]HandlerResult, len(handlers))
	var wg sync.WaitGroup
	wg.Add(len(handlers))
	for i, handler := range handlers {
		i := i
		handler := handler
		go func() {
			defer wg.Done()
			results[i] = runHandler(ctx, handler)
		}()
	}
	wg.Wait()
	return results
}

func runHandler(ctx context.Context, handler registeredHandler) (result HandlerResult) {
	started := time.Now()
	result.Name = handler.name
	defer func() {
		result.Duration = time.Since(started)
		if recovered := recover(); recovered != nil {
			result.Err = fmt.Errorf("cleanup handler %q panicked: %v", handler.name, recovered)
		}
		result.Succeeded = result.Err == nil
	}()

	result.Err = handler.fn(ctx)
	return result
}

func newResult(results []HandlerResult) Result {
	result := Result{
		Handlers: make([]HandlerResult, len(results)),
	}
	copy(result.Handlers, results)
	for _, handlerResult := range result.Handlers {
		if handlerResult.Err != nil {
			result.Failed++
			continue
		}
		result.Succeeded++
	}
	return result
}

func (r Result) Errors() []error {
	errs := make([]error, 0, r.Failed)
	for _, handlerResult := range r.Handlers {
		if handlerResult.Err != nil {
			errs = append(errs, handlerResult.Err)
		}
	}
	return errs
}
