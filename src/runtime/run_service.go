package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

var (
	ErrInvalidInput     = errors.New("input is required")
	ErrRunNotFound      = errors.New("run not found")
	ErrApprovalNotFound = errors.New("approval not found")
)

type WorkspaceConfig struct {
	WorkspaceRoot string   `json:"workspace_root"`
	WritableRoots []string `json:"writable_roots"`
}

type ClientInfo struct {
	SessionID  string `json:"session_id"`
	RequestID  string `json:"request_id"`
	EntryPoint string `json:"entrypoint"`
}

type StartRunRequest struct {
	Input     string          `json:"input"`
	Workspace WorkspaceConfig `json:"workspace"`
	Client    ClientInfo      `json:"client"`
}

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
	newExecutor func(workspace WorkspaceConfig) Executor
}

func NewRunService(newExecutor func(workspace WorkspaceConfig) Executor) *RunService {
	return &RunService{
		sessions:    make(map[string]*Session),
		newExecutor: newExecutor,
	}
}

func (s *RunService) StartRun(req StartRunRequest) (Run, error) {
	trimmed := strings.TrimSpace(req.Input)
	if trimmed == "" {
		return Run{}, ErrInvalidInput
	}
	if s.newExecutor == nil {
		return Run{}, fmt.Errorf("run executor is not configured")
	}

	workspace, err := normalizeWorkspace(req.Workspace)
	if err != nil {
		return Run{}, err
	}

	runID := newRuntimeID("run")
	threadID := newRuntimeID("thread")
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, cancel)

	s.mu.Lock()
	s.sessions[runID] = session
	s.mu.Unlock()

	executor := s.newExecutor(workspace)
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

func normalizeWorkspace(ws WorkspaceConfig) (WorkspaceConfig, error) {
	root := strings.TrimSpace(ws.WorkspaceRoot)
	if root == "" {
		return WorkspaceConfig{}, fmt.Errorf("%w: workspace_root is required", ErrInvalidInput)
	}
	if !filepath.IsAbs(root) {
		return WorkspaceConfig{}, fmt.Errorf("%w: workspace_root must be an absolute path", ErrInvalidInput)
	}
	root = filepath.Clean(root)

	writableRoots := ws.WritableRoots
	if len(writableRoots) == 0 {
		writableRoots = []string{root}
	} else {
		normalized := make([]string, 0, len(writableRoots))
		for _, w := range writableRoots {
			w = strings.TrimSpace(w)
			if w == "" {
				continue
			}
			if !filepath.IsAbs(w) {
				w = filepath.Join(root, w)
			}
			w = filepath.Clean(w)
			if !tool.PathWithinRoot(w, root) {
				w = root
			}
			normalized = append(normalized, w)
		}
		if len(normalized) == 0 {
			normalized = []string{root}
		}
		writableRoots = normalized
	}

	return WorkspaceConfig{
		WorkspaceRoot: root,
		WritableRoots: writableRoots,
	}, nil
}
