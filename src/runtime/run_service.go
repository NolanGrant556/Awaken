package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	ErrThreadLabelInvalid      = errors.New("thread label is required")
	ErrMissingModelProfile     = errors.New("model profile is required")
	ErrUnsupportedInputAttachment = errors.New("model does not support image input")
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
	Input        string              `json:"input"`
	Attachments  []RunInputAttachment `json:"attachments,omitempty"`
	Workspace    WorkspaceConfig     `json:"workspace"`
	Client       ClientInfo          `json:"client"`
	ThreadID     string              `json:"thread_id,omitempty"`
	ModelProfile *ModelProfileConfig `json:"model_profile,omitempty"`
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
	historyWorkspaces []WorkspaceConfig
	newAgent         func(workspace WorkspaceConfig, threadID string) *agentcore.Agent
	newProfileAgent  func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent
	newExecutor      func(agent *agentcore.Agent) Executor
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

func (s *RunService) SetHistoryWorkspaces(workspaces []WorkspaceConfig) error {
	normalized := make([]WorkspaceConfig, 0, len(workspaces))
	seen := make(map[string]struct{}, len(workspaces))
	for _, workspace := range workspaces {
		next, err := normalizeWorkspace(workspace)
		if err != nil {
			return err
		}
		if _, ok := seen[next.WorkspaceRoot]; ok {
			continue
		}
		seen[next.WorkspaceRoot] = struct{}{}
		normalized = append(normalized, next)
	}

	s.mu.Lock()
	s.historyWorkspaces = normalized
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
	if err := validateVisionSupport(req.ModelProfile, req.Attachments); err != nil {
		return Run{}, err
	}

	runID := newRuntimeID("run")
	threadID := req.ThreadID
	now := time.Now().UTC()
	var agent *agentcore.Agent
	modelProfileKey := modelProfileCacheKey(req.ModelProfile)
	var restored *restoredTranscript
	if threadID == "" {
		threadID = newRuntimeID("thread")
		agent = s.createAgent(workspace, threadID, req.ModelProfile)
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
	persistedAttachments, err := persistRunInputAttachments(threadID, req.Attachments)
	if err != nil {
		return Run{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, trimmed, persistedAttachments, cancel)

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
				label:           restored.Label,
				latestRunID:     runID,
				activeRunID:     runID,
				runIDs:          runIDs,
				modelProfileKey: modelProfileKey,
				createdAt:       restored.CreatedAt,
				updatedAt:       now,
			}
			s.sessions[runID] = session
			s.mu.Unlock()
			_ = appendUserTranscriptRecord(workspace, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments)
			executor := s.newExecutor(agent)
			go s.execute(session, executor, agent, trimmed, persistedAttachments, ctx, workspace)
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
		if modelProfileKey != thread.modelProfileKey {
			agent = s.createAgent(workspace, threadID, req.ModelProfile)
			if agent != nil && thread.agent != nil {
				agent.GetConversation().Context.SetMessages(thread.agent.GetConversation().Context.GetMessages())
			}
			thread.agent = agent
			thread.modelProfileKey = modelProfileKey
		}
		thread.activeRunID = runID
		thread.latestRunID = runID
		thread.runIDs = append(thread.runIDs, runID)
		thread.updatedAt = now
	} else {
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
	}
	s.sessions[runID] = session
	s.mu.Unlock()

	_ = appendUserTranscriptRecord(workspace, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments)
	executor := s.newExecutor(agent)
	go s.execute(session, executor, agent, trimmed, persistedAttachments, ctx, workspace)

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
	workspaces := workspaceSearchOrder(s.defaultWorkspace, s.historyWorkspaces)
	s.mu.RUnlock()

	for _, workspace := range workspaces {
		durableThreads, err := loadDurableThreadSnapshots(workspace)
		if err == nil {
			for _, thread := range durableThreads {
				if seen[thread.ThreadID] {
					continue
				}
				threads = append(threads, thread)
				seen[thread.ThreadID] = true
			}
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
		workspace := thread.workspace
		s.mu.Unlock()

		if err := appendThreadLabelTranscriptRecord(workspace, threadID, normalizedLabel, now); err != nil {
			return ThreadSnapshot{}, err
		}
		return snapshot, nil
	}
	workspaces := workspaceSearchOrder(s.defaultWorkspace, s.historyWorkspaces)
	s.mu.Unlock()

	workspace, restored, err := findWorkspaceForThread(workspaces, threadID)
	if err != nil {
		return ThreadSnapshot{}, err
	}
	if err := appendThreadLabelTranscriptRecord(workspace, threadID, normalizedLabel, now); err != nil {
		return ThreadSnapshot{}, err
	}

	snapshot := threadSnapshotFromRestored(threadID, workspace, restored)
	snapshot.Label = normalizedLabel
	snapshot.UpdatedAt = now
	return snapshot, nil
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
		workspace := thread.workspace
		runIDs := append([]string(nil), thread.runIDs...)
		delete(s.threads, threadID)
		for _, runID := range runIDs {
			delete(s.sessions, runID)
		}
		s.mu.Unlock()

		if err := removeThreadTranscript(workspace, threadID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := deleteThreadInputAttachments(threadID); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	workspaces := workspaceSearchOrder(s.defaultWorkspace, s.historyWorkspaces)
	s.mu.Unlock()

	workspace, _, err := findWorkspaceForThread(workspaces, threadID)
	if err != nil {
		return err
	}
	if err := removeThreadTranscript(workspace, threadID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrThreadNotFound
		}
		return err
	}
	if err := deleteThreadInputAttachments(threadID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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
		workspaces := workspaceSearchOrder(s.defaultWorkspace, s.historyWorkspaces)
		s.mu.RUnlock()
		workspace, _, err := findWorkspaceForThread(workspaces, threadID)
		if err != nil {
			return ThreadTranscript{}, err
		}
		return loadDurableThreadTranscript(workspace, threadID)
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

func loadDurableThreadTranscript(workspace WorkspaceConfig, threadID string) (ThreadTranscript, error) {
	restored, err := loadRestoredTranscript(workspace, threadID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}
	durableRuns, err := loadDurableTranscriptRuns(workspace, threadID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}

	runs := make([]ThreadTranscriptRun, 0, len(restored.RunIDs))
	for _, runID := range restored.RunIDs {
		if durable, ok := durableRuns[runID]; ok {
			runs = append(runs, durable)
		}
	}
	return ThreadTranscript{
		Thread: threadSnapshotFromRestored(threadID, workspace, restored),
		Runs:   runs,
	}, nil
}

func workspaceSearchOrder(defaultWorkspace WorkspaceConfig, historyWorkspaces []WorkspaceConfig) []WorkspaceConfig {
	workspaces := make([]WorkspaceConfig, 0, 1+len(historyWorkspaces))
	seen := make(map[string]struct{}, 1+len(historyWorkspaces))
	appendWorkspace := func(workspace WorkspaceConfig) {
		if workspace.WorkspaceRoot == "" {
			return
		}
		if _, ok := seen[workspace.WorkspaceRoot]; ok {
			return
		}
		seen[workspace.WorkspaceRoot] = struct{}{}
		workspaces = append(workspaces, workspace)
	}
	appendWorkspace(defaultWorkspace)
	for _, workspace := range historyWorkspaces {
		appendWorkspace(workspace)
	}
	return workspaces
}

func findWorkspaceForThread(workspaces []WorkspaceConfig, threadID string) (WorkspaceConfig, restoredTranscript, error) {
	for _, workspace := range workspaces {
		restored, err := loadRestoredTranscript(workspace, threadID)
		if err == nil {
			return workspace, restored, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return WorkspaceConfig{}, restoredTranscript{}, err
	}
	return WorkspaceConfig{}, restoredTranscript{}, ErrThreadNotFound
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

func (s *RunService) execute(session *Session, executor Executor, agent *agentcore.Agent, input string, attachments []RunInputAttachment, ctx context.Context, workspace WorkspaceConfig) {
	snapshot := session.Snapshot()
	defer func() {
		s.flushAgentTranscript(session, agent, workspace)
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
