package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tool/profile"
)

var (
	ErrInvalidInput               = errors.New("input is required")
	ErrRunNotFound                = errors.New("run not found")
	ErrRunTerminal                = errors.New("run is already finished")
	ErrRunCancelling              = errors.New("run is being cancelled")
	ErrThreadNotFound             = errors.New("thread not found")
	ErrThreadWorkspaceMismatch    = errors.New("thread workspace mismatch")
	ErrThreadBusy                 = errors.New("thread already has an active run")
	ErrThreadLabelInvalid         = errors.New("thread label is required")
	ErrMissingModelProfile        = errors.New("model profile is required")
	ErrUnsupportedInputAttachment = errors.New("model does not support image input")
	ErrMassMessageReviewNotFound  = errors.New("mass message review not found")
	ErrInvalidReviewDecision      = errors.New("invalid mass message review decision")
	ErrUserQuestionNotFound       = errors.New("user question not found")
)

const maxRetainedThreads = 100

type TaskScope struct {
	ScopeID           string            `json:"scope_id,omitempty"`
	WorkspaceRoot     string            `json:"workspace_root"`
	ReadableRoots     []string          `json:"readable_roots,omitempty"`
	WritableRoots     []string          `json:"writable_roots,omitempty"`
	OutputRoots       []string          `json:"output_roots,omitempty"`
	AttachedFiles     []tool.ScopedFile `json:"attached_files,omitempty"`
	ProtectedRoots    []string          `json:"protected_roots,omitempty"`
	TemporaryWorkRoot string            `json:"temporary_work_root,omitempty"`
	// ExecutionProfile is the unified policy bundle applied to every tool
	// call made by agents running under this task scope. When nil, the
	// agent falls back to profile.DefaultProfile() (WorkspaceWrite).
	// Callers that want to override the default (e.g. to run a guardian
	// agent with GuardianReadOnly) set this field on StartRunRequest.Workspace.
	ExecutionProfile *profile.ExecutionProfile `json:"execution_profile,omitempty"`
}

type WorkspaceConfig = TaskScope

type ClientInfo struct {
	SessionID  string `json:"session_id"`
	RequestID  string `json:"request_id"`
	EntryPoint string `json:"entrypoint"`
}

type StartRunRequest struct {
	Input        string               `json:"input"`
	Attachments  []RunInputAttachment `json:"attachments,omitempty"`
	Workspace    WorkspaceConfig      `json:"workspace"`
	Client       ClientInfo           `json:"client"`
	ThreadID     string               `json:"thread_id,omitempty"`
	ModelProfile *ModelProfileConfig  `json:"model_profile,omitempty"`
}

type ModelProfileConfig struct {
	ProfileID      string  `json:"profile_id,omitempty"`
	ProviderID     string  `json:"provider_id,omitempty"`
	DisplayName    string  `json:"display_name,omitempty"`
	ModelID        string  `json:"model_id,omitempty"`
	BaseURL        string  `json:"base_url,omitempty"`
	Protocol       string  `json:"protocol,omitempty"`
	AuthScheme     string  `json:"auth_scheme,omitempty"`
	APIKey         string  `json:"api_key,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	ContextWindow  int     `json:"context_window,omitempty"`
	PromptCacheKey string  `json:"prompt_cache_key,omitempty"`
	SupportsVision bool    `json:"supports_vision,omitempty"`
	SupportsVideo  bool    `json:"supports_video,omitempty"`
}

type Executor interface {
	Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error)
}

type ExecutorFunc func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)

func (f ExecutorFunc) Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
	return f(ctx, input, sink)
}

type RunService struct {
	mu               sync.RWMutex
	sessions         map[string]*Session
	threads          map[string]*Thread
	defaultWorkspace WorkspaceConfig
	newAgent         func(workspace WorkspaceConfig, threadID string) *agentcore.Agent
	newProfileAgent  func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent
	newExecutor      func(agent *agentcore.Agent) Executor
	threadStore      ThreadStore
}

// Thread holds the reusable Agent and metadata for a multi-turn conversation.
// Runs with the same thread_id reuse the same Agent and Conversation.
type Thread struct {
	threadID        string
	agent           *agentcore.Agent
	workspace       WorkspaceConfig
	label           string
	latestRunID     string
	activeRunID     string
	runIDs          []string
	modelProfileKey string
	createdAt       time.Time
	updatedAt       time.Time
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
		threadStore: newThreadStore(),
	}
}

func NewRunServiceWithModelProfiles(
	newAgent func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
) *RunService {
	service := NewRunService(nil, newExecutor)
	service.newProfileAgent = newAgent
	service.newAgent = func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
		return newAgent(workspace, threadID, nil)
	}
	return service
}

func (s *RunService) SetDefaultWorkspace(workspace WorkspaceConfig) error {
	normalized, err := normalizeWorkspace(workspace)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.defaultWorkspace = normalized
	s.mu.Unlock()
	return nil
}

func (s *RunService) StartRun(req StartRunRequest) (Run, error) {
	trimmed := strings.TrimSpace(req.Input)
	if trimmed == "" && len(req.Attachments) == 0 {
		return Run{}, ErrInvalidInput
	}
	if s.newExecutor == nil {
		return Run{}, fmt.Errorf("run executor is not configured")
	}
	if s.newProfileAgent != nil && req.ModelProfile == nil {
		return Run{}, ErrMissingModelProfile
	}

	workspace, err := normalizeWorkspace(req.Workspace)
	if err != nil {
		return Run{}, err
	}
	if err := validateRunInputAttachments(req.Attachments); err != nil {
		return Run{}, err
	}

	runID := newRuntimeID("run")
	threadID := req.ThreadID
	now := time.Now().UTC()
	var agent *agentcore.Agent
	modelProfileKey := modelProfileCacheKey(req.ModelProfile)
	var restored *restoredTranscript
	var restoredMeta *storedThreadRecord
	if threadID == "" {
		threadID = newRuntimeID("thread")
	} else {
		s.mu.RLock()
		_, threadInMemory := s.threads[threadID]
		s.mu.RUnlock()
		if !threadInMemory {
			meta, loaded, err := s.threadStore.RestoreThread(threadID)
			if err == nil {
				restoredMeta = &meta
				restored = &loaded
			} else if !errors.Is(err, ErrThreadNotFound) {
				return Run{}, err
			}
		}
	}
	persistedAttachments, err := persistRunInputAttachments(threadID, req.Attachments)
	if err != nil {
		return Run{}, err
	}
	workspace = mergeTaskScopeAttachments(workspace, persistedAttachments)
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, trimmed, persistedAttachments, cancel)
	var threadSnapshot ThreadSnapshot

	s.mu.Lock()
	if req.ThreadID != "" {
		thread, ok := s.threads[req.ThreadID]
		if !ok {
			if restored == nil {
				s.mu.Unlock()
				return Run{}, ErrThreadNotFound
			}
			agent = s.createAgent(workspace, threadID, req.ModelProfile)
			if agent != nil {
				agent.GetConversation().Context.SetMessages(restored.Messages)
			}
			runIDs := append([]string(nil), restored.RunIDs...)
			runIDs = append(runIDs, runID)
			s.threads[threadID] = &Thread{
				threadID:        threadID,
				agent:           agent,
				workspace:       workspace,
				label:           firstNonEmpty(restored.Label, restoredMeta.Snapshot.Label),
				latestRunID:     runID,
				activeRunID:     runID,
				runIDs:          runIDs,
				modelProfileKey: modelProfileKey,
				createdAt:       firstNonZeroTime(restored.CreatedAt, restoredMeta.Snapshot.CreatedAt, now),
				updatedAt:       now,
			}
			s.sessions[runID] = session
			threadSnapshot = s.threads[threadID].snapshot()
			s.mu.Unlock()
			_ = s.threadStore.CreateThread(threadSnapshot, modelProfileKey, threadLabelForInput(trimmed, persistedAttachments))
			_ = s.threadStore.AppendRunInput(threadSnapshot, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments)
			executor := s.newExecutor(agent)
			go s.execute(session, executor, agent, trimmed, persistedAttachments, ctx)
			return session.Snapshot(), nil
		}
		if !sameWorkspaceRoot(thread.workspace, workspace) {
			s.mu.Unlock()
			return Run{}, ErrThreadWorkspaceMismatch
		}
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return Run{}, ErrThreadBusy
		}
		agent = thread.agent
		if modelProfileKey != thread.modelProfileKey || !sameTaskScope(thread.workspace, workspace) {
			agent = s.createAgent(workspace, threadID, req.ModelProfile)
			if agent != nil && thread.agent != nil {
				agent.GetConversation().Context.SetMessages(thread.agent.GetConversation().Context.GetMessages())
			}
			thread.agent = agent
			thread.modelProfileKey = modelProfileKey
			thread.workspace = workspace
		}
		thread.activeRunID = runID
		thread.latestRunID = runID
		thread.runIDs = append(thread.runIDs, runID)
		thread.updatedAt = now
		threadSnapshot = thread.snapshot()
	} else {
		agent = s.createAgent(workspace, threadID, req.ModelProfile)
		label := threadLabelForInput(trimmed, persistedAttachments)
		s.threads[threadID] = &Thread{
			threadID:        threadID,
			agent:           agent,
			workspace:       workspace,
			label:           label,
			latestRunID:     runID,
			activeRunID:     runID,
			runIDs:          []string{runID},
			modelProfileKey: modelProfileKey,
			createdAt:       now,
			updatedAt:       now,
		}
		s.pruneInactiveThreadsLocked()
		threadSnapshot = s.threads[threadID].snapshot()
	}
	s.sessions[runID] = session
	s.mu.Unlock()

	_ = s.threadStore.CreateThread(threadSnapshot, modelProfileKey, threadLabelForInput(trimmed, persistedAttachments))
	_ = s.threadStore.AppendRunInput(threadSnapshot, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments)
	executor := s.newExecutor(agent)
	go s.execute(session, executor, agent, trimmed, persistedAttachments, ctx)

	return session.Snapshot(), nil
}

func (s *RunService) createAgent(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
	if s.newProfileAgent != nil {
		return s.newProfileAgent(workspace, threadID, profile)
	}
	if s.newAgent != nil {
		return s.newAgent(workspace, threadID)
	}
	return nil
}

func modelProfileCacheKey(profile *ModelProfileConfig) string {
	if profile == nil {
		return ""
	}
	parts := []string{
		profile.ProfileID,
		profile.ProviderID,
		profile.ModelID,
		profile.BaseURL,
		profile.Protocol,
		profile.AuthScheme,
		hashSecret(profile.APIKey),
		strconv.FormatFloat(profile.Temperature, 'g', -1, 64),
		strconv.Itoa(profile.MaxTokens),
		strconv.Itoa(profile.ContextWindow),
		profile.PromptCacheKey,
	}
	return strings.Join(parts, "\x00")
}

func hashSecret(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *RunService) ListThreads() []ThreadSnapshot {
	s.mu.RLock()
	threads := make([]ThreadSnapshot, 0, len(s.threads))
	seen := make(map[string]bool, len(s.threads))
	for _, thread := range s.threads {
		snapshot := thread.snapshot()
		threads = append(threads, snapshot)
		seen[snapshot.ThreadID] = true
	}
	s.mu.RUnlock()

	durableThreads, err := s.threadStore.ListThreads()
	if err == nil {
		for _, thread := range durableThreads {
			if seen[thread.ThreadID] {
				continue
			}
			threads = append(threads, thread)
			seen[thread.ThreadID] = true
		}
	}
	sort.Slice(threads, func(i, j int) bool {
		if !threads[i].UpdatedAt.Equal(threads[j].UpdatedAt) {
			return threads[i].UpdatedAt.After(threads[j].UpdatedAt)
		}
		return threads[i].CreatedAt.After(threads[j].CreatedAt)
	})
	return threads
}

func (s *RunService) RenameThread(threadID, label string) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	normalizedLabel := trimThreadLabel(strings.TrimSpace(label))
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if normalizedLabel == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}

	now := time.Now().UTC()

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok {
		thread.label = normalizedLabel
		thread.updatedAt = now
		snapshot := thread.snapshot()
		s.mu.Unlock()
		return s.threadStore.RenameThread(threadID, normalizedLabel, snapshot.UpdatedAt)
	}
	s.mu.Unlock()
	return s.threadStore.RenameThread(threadID, normalizedLabel, now)
}

func (s *RunService) FlushTranscripts() {
	s.mu.RLock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.RUnlock()

	for _, session := range sessions {
		run := session.Snapshot()
		snapshot, ok := s.threadSnapshot(run.ThreadID)
		if !ok {
			continue
		}
		s.mu.RLock()
		thread, ok := s.threads[run.ThreadID]
		s.mu.RUnlock()
		if !ok || thread.agent == nil {
			continue
		}
		_ = s.threadStore.AppendConversation(snapshot, run, thread.agent.GetConversation().Context.GetMessages())
		_ = s.threadStore.AppendRunEvents(snapshot, run, session.History(0))
	}
}

func (s *RunService) DeleteThread(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ErrThreadNotFound
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok {
		if thread.activeRunID != "" {
			s.mu.Unlock()
			return ErrThreadBusy
		}
		runIDs := append([]string(nil), thread.runIDs...)
		delete(s.threads, threadID)
		for _, runID := range runIDs {
			delete(s.sessions, runID)
		}
		s.mu.Unlock()
		return s.threadStore.DeleteThread(threadID)
	}
	s.mu.Unlock()
	return s.threadStore.DeleteThread(threadID)
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
		return s.threadStore.GetTranscript(threadID)
	}
	snapshot := thread.snapshot()
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
		loaded, err := s.threadStore.GetTranscript(threadID)
		if err == nil {
			for _, run := range loaded.Runs {
				durableRuns[run.Run.RunID] = run
			}
		} else if !errors.Is(err, ErrThreadNotFound) {
			return ThreadTranscript{}, err
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

func threadSnapshotFromRestored(threadID string, workspace WorkspaceConfig, restored restoredTranscript) ThreadSnapshot {
	latestRunID := ""
	if len(restored.RunIDs) > 0 {
		latestRunID = restored.RunIDs[len(restored.RunIDs)-1]
	}
	return ThreadSnapshot{
		ThreadID:  threadID,
		Label:     restored.Label,
		RunID:     latestRunID,
		Workspace: workspace,
		CreatedAt: restored.CreatedAt,
		UpdatedAt: restored.UpdatedAt,
	}
}

func (s *RunService) CancelRun(runID string) (Run, error) {
	session, err := s.session(runID)
	if err != nil {
		return Run{}, err
	}
	return session.RequestCancel(), nil
}

func (s *RunService) ResolveMassMessageReview(runID, reviewID string, decision agentcore.ReviewDecision) error {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return ErrRunNotFound
	}
	return session.ResolveMassMessageReview(reviewID, decision)
}

func (s *RunService) ResolveUserQuestion(runID, questionID, answer string) error {
	s.mu.RLock()
	session, ok := s.sessions[runID]
	s.mu.RUnlock()
	if !ok {
		return ErrRunNotFound
	}
	return session.ResolveUserQuestion(questionID, answer)
}

func partialAssistantResult(agent *agentcore.Agent) string {
	if agent == nil {
		return ""
	}
	messages := agent.GetConversation().Context.GetMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == convcontext.RoleAssistant && strings.TrimSpace(msg.Content) != "" {
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func (s *RunService) execute(session *Session, executor Executor, agent *agentcore.Agent, input string, attachments []RunInputAttachment, ctx context.Context) {
	snapshot := session.Snapshot()
	defer func() {
		s.flushAgentTranscript(session, agent)
		s.clearActiveRun(snapshot.ThreadID, snapshot.RunID)
	}()

	if executor == nil {
		session.CompleteError("run executor is not configured", "executor_missing")
		return
	}
	if agent != nil {
		agent.SetPendingUserAttachments(inputAttachmentsToContextAttachments(attachments))
	}

	result, err := executor.Run(ctx, input, sessionEventSink{session: session})
	if err == nil {
		if session.CancelRequested() {
			session.CompleteCancelled("你停止了生成本次回答", partialAssistantResult(agent))
			s.flushAgentTranscript(session, agent)
			return
		}
		session.CompleteFinished(result)
		return
	}

	if errors.Is(ctx.Err(), context.Canceled) || session.CancelRequested() {
		session.CompleteCancelled("你停止了生成本次回答", partialAssistantResult(agent))
		s.flushAgentTranscript(session, agent)
		return
	}

	session.CompleteError(err.Error(), "")
}

func (s *RunService) flushAgentTranscript(session *Session, agent *agentcore.Agent) {
	if session == nil || agent == nil {
		return
	}
	run := session.Snapshot()
	snapshot, ok := s.threadSnapshot(run.ThreadID)
	if !ok {
		return
	}
	_ = s.threadStore.AppendConversation(snapshot, run, agent.GetConversation().Context.GetMessages())
	_ = s.threadStore.AppendRunEvents(snapshot, run, session.History(0))
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

func (s *RunService) threadSnapshot(threadID string) (ThreadSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread, ok := s.threads[threadID]
	if !ok {
		return ThreadSnapshot{}, false
	}
	return thread.snapshot(), true
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

func sameWorkspaceRoot(a, b WorkspaceConfig) bool {
	if a.WorkspaceRoot != b.WorkspaceRoot {
		return false
	}
	return true
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func sameTaskScope(a, b WorkspaceConfig) bool {
	if !sameWorkspaceRoot(a, b) {
		return false
	}
	if !sameStringSet(a.ReadableRoots, b.ReadableRoots) {
		return false
	}
	if !sameStringSet(a.WritableRoots, b.WritableRoots) {
		return false
	}
	if !sameStringSet(a.OutputRoots, b.OutputRoots) {
		return false
	}
	if !sameStringSet(a.ProtectedRoots, b.ProtectedRoots) {
		return false
	}
	if a.TemporaryWorkRoot != b.TemporaryWorkRoot {
		return false
	}
	if !sameScopedFiles(a.AttachedFiles, b.AttachedFiles) {
		return false
	}
	return reflect.DeepEqual(a.ExecutionProfile, b.ExecutionProfile)
}

func sameScopedFiles(a, b []tool.ScopedFile) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, value := range a {
		seen[value.Path]++
	}
	for _, value := range b {
		if seen[value.Path] == 0 {
			return false
		}
		seen[value.Path]--
	}
	return true
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

func (s sessionEventSink) AwaitUserQuestion(ctx context.Context, request agentcore.AwakenEvent) (string, error) {
	if s.session == nil {
		return "", ErrRunNotFound
	}
	return s.session.AwaitUserQuestion(ctx, request)
}

func (s sessionEventSink) AwaitMassMessageReview(ctx context.Context, request agentcore.AwakenEvent) (agentcore.ReviewDecision, error) {
	if s.session == nil {
		return "", ErrRunNotFound
	}
	return s.session.AwaitMassMessageReview(ctx, request)
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

	// Normalize the client-supplied ExecutionProfile against the server
	// allowlist. NormalizeClientProfile maps nil, tampered, and
	// server-only profile requests onto a known built-in profile value
	// and logs a warning whenever it has to fall back. The pointer we
	// store always points at a fresh built-in value, never at the
	// caller-supplied struct, so clients cannot inject tampered fields.
	normalizedProfile := profile.NormalizeClientProfile(ws.ExecutionProfile)
	readableRoots := normalizeTaskScopeRoots(ws.ReadableRoots, root)
	writableRoots := normalizeTaskScopeRoots(ws.WritableRoots, root)
	outputRoots := normalizeTaskScopeRoots(ws.OutputRoots, root)
	attachedFiles := normalizeTaskScopeFiles(ws.AttachedFiles, root)
	scopeID := buildTaskScopeID(root, readableRoots, writableRoots, outputRoots, attachedFiles)
	protectedRoots := buildProtectedRoots(root)
	temporaryWorkRoot := filepath.Join(os.TempDir(), "awaken-task-scopes", scopeID)
	return WorkspaceConfig{
		ScopeID:           scopeID,
		WorkspaceRoot:     root,
		ReadableRoots:     readableRoots,
		WritableRoots:     writableRoots,
		OutputRoots:       outputRoots,
		AttachedFiles:     attachedFiles,
		ProtectedRoots:    protectedRoots,
		TemporaryWorkRoot: temporaryWorkRoot,
		ExecutionProfile:  &normalizedProfile,
	}, nil
}

func normalizeTaskScopeRoots(values []string, workspaceRoot string) []string {
	if len(values) == 0 {
		return []string{workspaceRoot}
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(workspaceRoot, value)
		}
		value = filepath.Clean(value)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []string{workspaceRoot}
	}
	return normalized
}

func normalizeTaskScopeFiles(values []tool.ScopedFile, workspaceRoot string) []tool.ScopedFile {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]tool.ScopedFile, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value.Path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspaceRoot, path)
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, tool.ScopedFile{
			Path:        path,
			DisplayName: strings.TrimSpace(value.DisplayName),
		})
	}
	return normalized
}

func buildTaskScopeID(
	workspaceRoot string,
	readableRoots []string,
	writableRoots []string,
	outputRoots []string,
	attachedFiles []tool.ScopedFile,
) string {
	parts := []string{workspaceRoot}
	parts = append(parts, readableRoots...)
	parts = append(parts, writableRoots...)
	parts = append(parts, outputRoots...)
	for _, file := range attachedFiles {
		parts = append(parts, file.Path)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "scope_" + hex.EncodeToString(sum[:8])
}

func buildProtectedRoots(workspaceRoot string) []string {
	protected := []string{
		filepath.Join(workspaceRoot, ".git"),
		filepath.Join(workspaceRoot, ".awaken"),
		filepath.Join(workspaceRoot, ".vscode"),
		filepath.Join(workspaceRoot, ".idea"),
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		protected = append(protected,
			filepath.Join(home, ".awaken"),
			filepath.Join(home, ".trae-cn"),
			filepath.Join(home, ".zshrc"),
			filepath.Join(home, ".zprofile"),
			filepath.Join(home, ".zlogin"),
			filepath.Join(home, ".zlogout"),
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".bash_profile"),
			filepath.Join(home, ".bash_logout"),
			filepath.Join(home, ".profile"),
			filepath.Join(home, ".config", "fish", "config.fish"),
			filepath.Join(home, ".vscode"),
			filepath.Join(home, ".cursor"),
			filepath.Join(home, ".idea"),
		)
	}
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		protected = append(protected,
			filepath.Join(configDir, "awaken"),
			filepath.Join(configDir, "Awaken"),
			filepath.Join(configDir, "Code"),
			filepath.Join(configDir, "Cursor"),
		)
	}
	return normalizeTaskScopeRoots(protected, workspaceRoot)
}

func mergeTaskScopeAttachments(scope WorkspaceConfig, attachments []RunInputAttachment) WorkspaceConfig {
	if len(attachments) == 0 {
		return scope
	}
	merged := append([]tool.ScopedFile(nil), scope.AttachedFiles...)
	seen := make(map[string]struct{}, len(merged))
	for _, file := range merged {
		seen[file.Path] = struct{}{}
	}
	for _, attachment := range attachments {
		if attachment.Kind != RunInputAttachmentKindFile {
			continue
		}
		path := strings.TrimSpace(attachment.Path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		merged = append(merged, tool.ScopedFile{
			Path:        path,
			DisplayName: strings.TrimSpace(attachment.DisplayName),
		})
	}
	scope.AttachedFiles = normalizeTaskScopeFiles(merged, scope.WorkspaceRoot)
	scope.ScopeID = buildTaskScopeID(scope.WorkspaceRoot, scope.ReadableRoots, scope.WritableRoots, scope.OutputRoots, scope.AttachedFiles)
	scope.TemporaryWorkRoot = filepath.Join(os.TempDir(), "awaken-task-scopes", scope.ScopeID)
	return scope
}
