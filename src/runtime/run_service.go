package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/agentcore"
	"awaken/src/tool"
)

var (
	ErrInvalidInput            = errors.New("input is required")
	ErrRunNotFound             = errors.New("run not found")
	ErrApprovalNotFound        = errors.New("approval not found")
	ErrRunTerminal             = errors.New("run is already finished")
	ErrRunCancelling           = errors.New("run is being cancelled")
	ErrThreadNotFound          = errors.New("thread not found")
	ErrThreadWorkspaceMismatch = errors.New("thread workspace mismatch")
	ErrThreadBusy              = errors.New("thread already has an active run")
)

const maxRetainedThreads = 100

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
	ThreadID  string          `json:"thread_id,omitempty"`
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
	threads     map[string]*Thread
	newAgent    func(workspace WorkspaceConfig, threadID string) *agentcore.Agent
	newExecutor func(agent *agentcore.Agent) Executor
}

// Thread holds the reusable Agent and metadata for a multi-turn conversation.
// Runs with the same thread_id reuse the same Agent and Conversation.
type Thread struct {
	threadID    string
	agent       *agentcore.Agent
	workspace   WorkspaceConfig
	label       string
	latestRunID string
	activeRunID string
	runIDs      []string
	createdAt   time.Time
	updatedAt   time.Time
}

type ThreadSnapshot struct {
	ThreadID    string          `json:"thread_id"`
	Label       string          `json:"label"`
	RunID       string          `json:"run_id,omitempty"`
	ActiveRunID string          `json:"active_run_id,omitempty"`
	Workspace   WorkspaceConfig `json:"workspace"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type ThreadTranscriptRun struct {
	Run    Run
	Events []Event
}

type ThreadTranscript struct {
	Thread ThreadSnapshot
	Runs   []ThreadTranscriptRun
}

func NewRunService(
	newAgent func(workspace WorkspaceConfig, threadID string) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
) *RunService {
	return &RunService{
		sessions:    make(map[string]*Session),
		threads:     make(map[string]*Thread),
		newAgent:    newAgent,
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
	threadID := req.ThreadID
	now := time.Now().UTC()
	var agent *agentcore.Agent
	var restored *restoredTranscript
	if threadID == "" {
		threadID = newRuntimeID("thread")
		if s.newAgent != nil {
			agent = s.newAgent(workspace, threadID)
		}
	} else {
		s.mu.RLock()
		_, threadInMemory := s.threads[threadID]
		s.mu.RUnlock()
		if !threadInMemory {
			loaded, err := loadRestoredTranscript(workspace, threadID)
			if err == nil {
				restored = &loaded
			} else if !errors.Is(err, os.ErrNotExist) {
				return Run{}, err
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, trimmed, cancel)

	s.mu.Lock()
	if req.ThreadID != "" {
		thread, ok := s.threads[req.ThreadID]
		if !ok {
			if restored == nil {
				s.mu.Unlock()
				return Run{}, ErrThreadNotFound
			}
			if s.newAgent != nil {
				agent = s.newAgent(workspace, threadID)
				if agent != nil {
					agent.GetConversation().Context.SetMessages(restored.Messages)
				}
			}
			runIDs := append([]string(nil), restored.RunIDs...)
			runIDs = append(runIDs, runID)
			s.threads[threadID] = &Thread{
				threadID:    threadID,
				agent:       agent,
				workspace:   workspace,
				label:       restored.Label,
				latestRunID: runID,
				activeRunID: runID,
				runIDs:      runIDs,
				createdAt:   restored.CreatedAt,
				updatedAt:   now,
			}
			s.sessions[runID] = session
			s.mu.Unlock()
			_ = appendUserTranscriptRecord(workspace, session.Snapshot(), req.Client.SessionID, trimmed)
			executor := s.newExecutor(agent)
			go s.execute(session, executor, agent, trimmed, ctx, workspace)
			return session.Snapshot(), nil
		}
		if !sameWorkspace(thread.workspace, workspace) {
			s.mu.Unlock()
			return Run{}, ErrThreadWorkspaceMismatch
		}
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return Run{}, ErrThreadBusy
		}
		agent = thread.agent
		thread.activeRunID = runID
		thread.latestRunID = runID
		thread.runIDs = append(thread.runIDs, runID)
		thread.updatedAt = now
	} else {
		label := trimThreadLabel(trimmed)
		s.threads[threadID] = &Thread{
			threadID:    threadID,
			agent:       agent,
			workspace:   workspace,
			label:       label,
			latestRunID: runID,
			activeRunID: runID,
			runIDs:      []string{runID},
			createdAt:   now,
			updatedAt:   now,
		}
		s.pruneInactiveThreadsLocked()
	}
	s.sessions[runID] = session
	s.mu.Unlock()

	_ = appendUserTranscriptRecord(workspace, session.Snapshot(), req.Client.SessionID, trimmed)
	executor := s.newExecutor(agent)
	go s.execute(session, executor, agent, trimmed, ctx, workspace)

	return session.Snapshot(), nil
}

func (s *RunService) ListThreads() []ThreadSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threads := make([]ThreadSnapshot, 0, len(s.threads))
	for _, thread := range s.threads {
		threads = append(threads, thread.snapshot())
	}
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].CreatedAt.Before(threads[j].CreatedAt)
	})
	return threads
}

func (s *RunService) Subscribe(runID string, afterSeq uint64) (Subscription, error) {
	session, err := s.session(runID)
	if err != nil {
		return Subscription{}, err
	}
	return session.Subscribe(afterSeq), nil
}

func (s *RunService) GetRun(runID string) (Run, error) {
	session, err := s.session(runID)
	if err != nil {
		return Run{}, err
	}
	return session.Snapshot(), nil
}

func (s *RunService) GetThreadTranscript(threadID string) (ThreadTranscript, error) {
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return ThreadTranscript{}, ErrThreadNotFound
	}
	snapshot := thread.snapshot()
	workspace := thread.workspace
	runIDs := append([]string(nil), thread.runIDs...)
	sessionByRunID := make(map[string]*Session, len(runIDs))
	for _, runID := range runIDs {
		session, ok := s.sessions[runID]
		if !ok {
			continue
		}
		sessionByRunID[runID] = session
	}
	s.mu.RUnlock()

	durableRuns := map[string]ThreadTranscriptRun{}
	if len(sessionByRunID) < len(runIDs) {
		loaded, err := loadDurableTranscriptRuns(workspace, threadID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, err
		}
		if err == nil {
			durableRuns = loaded
		}
	}

	runs := make([]ThreadTranscriptRun, 0, len(runIDs))
	for _, runID := range runIDs {
		if session, ok := sessionByRunID[runID]; ok {
			runs = append(runs, ThreadTranscriptRun{
				Run:    session.Snapshot(),
				Events: session.History(0),
			})
			continue
		}
		if durable, ok := durableRuns[runID]; ok {
			runs = append(runs, durable)
		}
	}

	return ThreadTranscript{
		Thread: snapshot,
		Runs:   runs,
	}, nil
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

func (s *RunService) execute(session *Session, executor Executor, agent *agentcore.Agent, input string, ctx context.Context, workspace WorkspaceConfig) {
	snapshot := session.Snapshot()
	defer func() {
		s.flushAgentTranscript(session, agent, workspace)
		s.clearActiveRun(snapshot.ThreadID, snapshot.RunID)
	}()

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

func (s *RunService) flushAgentTranscript(session *Session, agent *agentcore.Agent, workspace WorkspaceConfig) {
	if session == nil || agent == nil {
		return
	}
	run := session.Snapshot()
	_ = appendConversationTranscript(workspace, run, agent.GetConversation().Context.GetMessages())
	_ = appendRunEventTranscript(workspace, run, session.History(0))
}

func (s *RunService) clearActiveRun(threadID, runID string) {
	if threadID == "" || runID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	thread, ok := s.threads[threadID]
	if !ok || thread.activeRunID != runID {
		return
	}
	thread.activeRunID = ""
	thread.updatedAt = time.Now().UTC()
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

func (t *Thread) snapshot() ThreadSnapshot {
	return ThreadSnapshot{
		ThreadID:    t.threadID,
		Label:       t.label,
		RunID:       t.latestRunID,
		ActiveRunID: t.activeRunID,
		Workspace:   t.workspace,
		CreatedAt:   t.createdAt,
		UpdatedAt:   t.updatedAt,
	}
}

func (s *RunService) pruneInactiveThreadsLocked() {
	if len(s.threads) <= maxRetainedThreads {
		return
	}

	inactive := make([]*Thread, 0, len(s.threads))
	for _, thread := range s.threads {
		if thread.activeRunID == "" {
			inactive = append(inactive, thread)
		}
	}
	sort.Slice(inactive, func(i, j int) bool {
		return inactive[i].updatedAt.Before(inactive[j].updatedAt)
	})
	for len(s.threads) > maxRetainedThreads && len(inactive) > 0 {
		thread := inactive[0]
		inactive = inactive[1:]
		delete(s.threads, thread.threadID)
	}
}

func sameWorkspace(a, b WorkspaceConfig) bool {
	if a.WorkspaceRoot != b.WorkspaceRoot {
		return false
	}
	return sameStringSet(a.WritableRoots, b.WritableRoots)
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, value := range a {
		seen[value]++
	}
	for _, value := range b {
		if seen[value] == 0 {
			return false
		}
		seen[value]--
	}
	return true
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
