package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/workspaceguard"
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
	Input            string                   `json:"input"`
	Workspace        WorkspaceConfig          `json:"workspace"`
	Client           ClientInfo               `json:"client"`
	ThreadID         string                   `json:"thread_id,omitempty"`
	PermissionPolicy tool.PermissionPolicy    `json:"permission_policy,omitempty"`
	ApprovalPolicy   agentcore.ApprovalPolicy `json:"approval_policy,omitempty"`
	WorkspacePolicy  workspaceguard.Policy    `json:"workspace_policy,omitempty"`
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
	store       *fileStore
}

// Thread holds the reusable Agent and metadata for a multi-turn conversation.
// Runs with the same thread_id reuse the same Agent and Conversation.
type Thread struct {
	threadID     string
	agent        *agentcore.Agent
	conversation *convcontext.Conversation
	workspace    WorkspaceConfig
	label        string
	latestRunID  string
	activeRunID  string
	runIDs       []string
	createdAt    time.Time
	updatedAt    time.Time
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

type RunServiceOption func(*RunService)

func WithPersistenceDir(dir string) RunServiceOption {
	return func(service *RunService) {
		service.store = newFileStore(dir)
	}
}

func NewRunService(
	newAgent func(workspace WorkspaceConfig, threadID string) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
) *RunService {
	return NewRunServiceWithOptions(newAgent, newExecutor)
}

func NewRunServiceWithOptions(
	newAgent func(workspace WorkspaceConfig, threadID string) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
	opts ...RunServiceOption,
) *RunService {
	service := &RunService{
		sessions:    make(map[string]*Session),
		threads:     make(map[string]*Thread),
		newAgent:    newAgent,
		newExecutor: newExecutor,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	service.restorePersistedState()
	return service
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
	permissionPolicy := req.PermissionPolicy.Normalize()
	approvalPolicy := req.ApprovalPolicy.Normalize()
	if err := evaluateWorkspacePolicy(workspace.WorkspaceRoot, req.WorkspacePolicy); err != nil {
		return Run{}, err
	}

	runID := newRuntimeID("run")
	threadID := req.ThreadID
	now := time.Now().UTC()
	var agent *agentcore.Agent
	if threadID == "" {
		threadID = newRuntimeID("thread")
		if s.newAgent != nil {
			agent = s.newAgent(workspace, threadID)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSessionWithConfig(runID, threadID, trimmed, cancel, SessionConfig{
		ApprovalPolicy: approvalPolicy,
		Observer:       persistenceObserver{store: s.store},
	})

	s.mu.Lock()
	if req.ThreadID != "" {
		thread, ok := s.threads[req.ThreadID]
		if !ok {
			s.mu.Unlock()
			return Run{}, ErrThreadNotFound
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
		if thread.conversation == nil {
			thread.conversation = conversationForAgent(agent)
		}
		thread.activeRunID = runID
		thread.latestRunID = runID
		thread.runIDs = append(thread.runIDs, runID)
		thread.updatedAt = now
	} else {
		label := trimmed
		if len(label) > 50 {
			label = label[:50]
		}
		s.threads[threadID] = &Thread{
			threadID:     threadID,
			agent:        agent,
			conversation: conversationForAgent(agent),
			workspace:    workspace,
			label:        label,
			latestRunID:  runID,
			activeRunID:  runID,
			runIDs:       []string{runID},
			createdAt:    now,
			updatedAt:    now,
		}
		s.pruneInactiveThreadsLocked()
	}
	s.sessions[runID] = session
	s.mu.Unlock()

	if s.store != nil {
		persistenceObserver{store: s.store}.OnRunUpdated(session.Snapshot())
		s.persistThread(threadID)
	}

	executor := s.newExecutor(agentForRun(agent, threadID, workspace, permissionPolicy, approvalPolicy))
	go s.execute(session, executor, trimmed, ctx)

	return session.Snapshot(), nil
}

func evaluateWorkspacePolicy(workspaceRoot string, policy workspaceguard.Policy) error {
	if policy.IsZero() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := workspaceguard.Evaluate(ctx, workspaceRoot, policy)
	return err
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
	runIDs := append([]string(nil), thread.runIDs...)
	sessions := make([]*Session, 0, len(runIDs))
	for _, runID := range runIDs {
		session, ok := s.sessions[runID]
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}
	s.mu.RUnlock()

	runs := make([]ThreadTranscriptRun, 0, len(sessions))
	for _, session := range sessions {
		runs = append(runs, ThreadTranscriptRun{
			Run:    session.Snapshot(),
			Events: session.History(0),
		})
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

func (s *RunService) execute(session *Session, executor Executor, input string, ctx context.Context) {
	snapshot := session.Snapshot()
	defer s.clearActiveRun(snapshot.ThreadID, snapshot.RunID)

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

func (s *RunService) clearActiveRun(threadID, runID string) {
	if threadID == "" || runID == "" {
		return
	}

	s.mu.Lock()
	thread, ok := s.threads[threadID]
	if !ok || thread.activeRunID != runID {
		s.mu.Unlock()
		return
	}
	thread.activeRunID = ""
	thread.updatedAt = time.Now().UTC()
	thread.conversation = conversationForAgent(thread.agent)
	s.mu.Unlock()

	s.persistThread(threadID)
}

func (s *RunService) restorePersistedState() {
	if s.store == nil {
		return
	}

	threads, err := s.store.loadThreads()
	if err != nil {
		log.Printf("runtime persistence: failed to load threads: %v", err)
		return
	}

	for _, record := range threads {
		thread := &Thread{
			threadID:     record.Thread.ThreadID,
			conversation: restoreConversation(record),
			workspace:    record.Thread.Workspace,
			label:        record.Thread.Label,
			latestRunID:  record.Thread.RunID,
			activeRunID:  "",
			runIDs:       append([]string(nil), record.RunIDs...),
			createdAt:    record.Thread.CreatedAt,
			updatedAt:    record.Thread.UpdatedAt,
		}

		if s.newAgent != nil {
			thread.agent = s.newAgent(thread.workspace, thread.threadID)
			if thread.agent != nil {
				thread.conversation = thread.agent.GetConversation()
				if thread.conversation != nil {
					thread.conversation.Context.SetMessages(append([]convcontext.Message(nil), record.ConversationMessages...))
					if createdAt, ok := parsePersistedTime(record.ConversationCreatedAt); ok {
						thread.conversation.CreatedAt = createdAt
					}
					if updatedAt, ok := parsePersistedTime(record.ConversationUpdatedAt); ok {
						thread.conversation.UpdatedAt = updatedAt
					}
				}
			}
		}

		s.threads[thread.threadID] = thread
		for _, runID := range record.RunIDs {
			runRecord, err := s.store.loadRun(runID)
			if err != nil {
				log.Printf("runtime persistence: failed to load run %s: %v", runID, err)
				continue
			}
			events, err := s.store.loadEvents(runID)
			if err != nil {
				log.Printf("runtime persistence: failed to load events for run %s: %v", runID, err)
				continue
			}
			restoredRun, restoredEvents, dirty := normalizeRestoredRun(runRecord.Run, events)
			if dirty {
				if err := s.store.saveRun(restoredRun); err != nil {
					log.Printf("runtime persistence: failed to repair interrupted run %s: %v", runID, err)
				}
				for _, event := range restoredEvents[len(events):] {
					if err := s.store.appendEvent(event); err != nil {
						log.Printf("runtime persistence: failed to append recovery event for run %s: %v", runID, err)
					}
				}
			}
			s.sessions[runID] = NewSessionFromSnapshot(restoredRun, restoredEvents)
		}
	}
}

func (s *RunService) persistThread(threadID string) {
	if s.store == nil || strings.TrimSpace(threadID) == "" {
		return
	}

	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
		return
	}
	record := newPersistedThread(thread)
	s.mu.RUnlock()

	if err := s.store.saveThread(record); err != nil {
		log.Printf("runtime persistence: failed to save thread %s: %v", threadID, err)
	}
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

func agentForRun(
	agent *agentcore.Agent,
	threadID string,
	workspace WorkspaceConfig,
	permissionPolicy tool.PermissionPolicy,
	approvalPolicy agentcore.ApprovalPolicy,
) *agentcore.Agent {
	if agent == nil {
		return nil
	}

	config := agent.GetConfig()
	config.Workspace = agentcore.WorkspaceConfig{
		Root:          workspace.WorkspaceRoot,
		WritableRoots: append([]string(nil), workspace.WritableRoots...),
	}
	config.PermissionPolicy = permissionPolicy.Normalize()
	config.ApprovalPolicy = approvalPolicy.Normalize()
	agent.SetConfig(config)
	return agent
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

func conversationForAgent(agent *agentcore.Agent) *convcontext.Conversation {
	if agent == nil {
		return nil
	}
	return agent.GetConversation()
}

func normalizeRestoredRun(run Run, events []Event) (Run, []Event, bool) {
	if isTerminalStatus(run.Status) {
		return run, events, false
	}

	now := time.Now().UTC()
	run.Status = RunStatusError
	run.Error = "run interrupted by process restart"
	run.UpdatedAt = now
	run.CancelRequested = false
	run.FinishedAt = &now

	restoredEvents := append([]Event(nil), events...)
	restoredEvents = append(restoredEvents, Event{
		Kind:      EventKindRunError,
		Seq:       maxEventSeq(restoredEvents) + 1,
		Timestamp: now,
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		Message:   run.Error,
		Code:      "RUN_INTERRUPTED",
	})
	return run, restoredEvents, true
}

func maxEventSeq(events []Event) uint64 {
	var maxSeq uint64
	for _, event := range events {
		if event.Seq > maxSeq {
			maxSeq = event.Seq
		}
	}
	return maxSeq
}
