package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/agentcore"
)

var (
	ErrInvalidInput     = errors.New("input is required")
	ErrRunNotFound      = errors.New("run not found")
	ErrApprovalNotFound = errors.New("approval not found")
)

type Executor interface {
	Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error)
}

type ExecutorFunc func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)

func (f ExecutorFunc) Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
	return f(ctx, input, sink)
}

type RunService struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	newExecutor func() Executor
}

func NewRunService(newExecutor func() Executor) *RunService {
	return &RunService{
		sessions:    make(map[string]*Session),
		newExecutor: newExecutor,
	}
}

func (s *RunService) StartRun(input string) (Run, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Run{}, ErrInvalidInput
	}
	if s.newExecutor == nil {
		return Run{}, fmt.Errorf("run executor is not configured")
	}

	runID := newRuntimeID("run")
	threadID := newRuntimeID("thread")
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, cancel)

	s.mu.Lock()
	s.sessions[runID] = session
	s.mu.Unlock()

	executor := s.newExecutor()
	go s.execute(session, executor, trimmed, ctx)

	return session.Snapshot(), nil
}

func (s *RunService) Subscribe(runID string) (Subscription, error) {
	session, err := s.session(runID)
	if err != nil {
		return Subscription{}, err
	}
	return session.Subscribe(), nil
}

func (s *RunService) CancelRun(runID string) (Run, error) {
	session, err := s.session(runID)
	if err != nil {
		return Run{}, err
	}
	return session.RequestCancel(), nil
}

func (s *RunService) ResolveApproval(runID, approvalID string, decision agentcore.ApprovalDecision) error {
	session, err := s.session(runID)
	if err != nil {
		return err
	}
	return session.ResolveApproval(approvalID, decision)
}

func (s *RunService) execute(session *Session, executor Executor, input string, ctx context.Context) {
	if executor == nil {
		session.CompleteError("run executor is not configured", "executor_missing")
		return
	}

	result, err := executor.Run(ctx, input, sessionEventSink{session: session})
	if err == nil {
		if session.CancelRequested() {
			session.CompleteCancelled("run cancelled")
			return
		}
		session.CompleteFinished(result)
		return
	}

	if errors.Is(ctx.Err(), context.Canceled) || session.CancelRequested() {
		session.CompleteCancelled("run cancelled")
		return
	}

	session.CompleteError(err.Error(), "")
}

func (s *RunService) session(runID string) (*Session, error) {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrRunNotFound
	}
	return session, nil
}

var runtimeIDCounter uint64

func newRuntimeID(prefix string) string {
	seq := atomic.AddUint64(&runtimeIDCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), seq)
}

type sessionEventSink struct {
	session *Session
}

func (s sessionEventSink) Emit(event agentcore.AwakenEvent) {
	if s.session == nil || s.session.CancelRequested() {
		return
	}
	s.session.RecordAgentEvent(event)
}

func (s sessionEventSink) AwaitApproval(ctx context.Context, request agentcore.ApprovalRequest) (agentcore.ApprovalDecision, error) {
	if s.session == nil {
		return "", context.Canceled
	}
	return s.session.AwaitApproval(ctx, request)
}
