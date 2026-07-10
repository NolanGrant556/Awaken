package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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
	"awaken/src/desktopdata"
	"awaken/src/skillruntime"
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
	ErrPersistenceUnavailable     = agentcore.ErrPersistenceUnavailable
)

const maxRetainedThreads = 100

var threadTitleRetryDelays = []time.Duration{
	2 * time.Second,
	8 * time.Second,
}

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
	Input          string               `json:"input"`
	Attachments    []RunInputAttachment `json:"attachments,omitempty"`
	ComposerTokens []ComposerToken      `json:"composer_tokens,omitempty"`
	Workspace      WorkspaceConfig      `json:"workspace"`
	Client         ClientInfo           `json:"client"`
	ThreadID       string               `json:"thread_id,omitempty"`
	ModelProfile   *ModelProfileConfig  `json:"model_profile,omitempty"`
}

type ComposerToken struct {
	ID      string               `json:"id"`
	Type    string               `json:"type"`
	Label   string               `json:"label"`
	Payload ComposerTokenPayload `json:"payload,omitempty"`
}

type ComposerTokenPayload struct {
	SkillID       string `json:"skillId,omitempty"`
	SourceOrigin  string `json:"sourceOrigin,omitempty"`
	SkillFilePath string `json:"skillFilePath,omitempty"`
	RootDir       string `json:"rootDir,omitempty"`
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
	mu                sync.RWMutex
	sessions          map[string]*Session
	threads           map[string]*Thread
	defaultWorkspace  WorkspaceConfig
	newAgent          func(workspace WorkspaceConfig, threadID string) *agentcore.Agent
	newProfileAgent   func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent
	newExecutor       func(agent *agentcore.Agent) Executor
	threadStore       ThreadStore
	journals          *journalManager
	allowNoopRecorder bool
	skillActivator    *skillruntime.Activator
}

// SetSkillActivator injects the shared SkillActivator used for explicit
// composer-token pre-activation before the first LLM request.
func (s *RunService) SetSkillActivator(activator *skillruntime.Activator) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.skillActivator = activator
	s.mu.Unlock()
}

// Thread holds the reusable Agent and metadata for a multi-turn conversation.
// Runs with the same thread_id reuse the same Agent and Conversation.
type Thread struct {
	threadID        string
	agent           *agentcore.Agent
	workspace       WorkspaceConfig
	label           string
	titleSource     ThreadTitleSource
	titleStatus     ThreadTitleStatus
	titleUpdatedAt  time.Time
	titleSeedRunID  string
	latestRunID     string
	activeRunID     string
	runIDs          []string
	modelProfileKey string
	createdAt       time.Time
	updatedAt       time.Time
}

type ThreadTitleSource string

const (
	ThreadTitleSourceFirstUser ThreadTitleSource = "first_user"
	ThreadTitleSourceLLM       ThreadTitleSource = "llm"
	ThreadTitleSourceManual    ThreadTitleSource = "manual"
)

type ThreadTitleStatus string

const (
	ThreadTitleStatusPending ThreadTitleStatus = "pending"
	ThreadTitleStatusReady   ThreadTitleStatus = "ready"
	ThreadTitleStatusFailed  ThreadTitleStatus = "failed"
)

type ThreadSnapshot struct {
	ThreadID       string            `json:"thread_id"`
	Label          string            `json:"label"`
	TitleSource    ThreadTitleSource `json:"title_source,omitempty"`
	TitleStatus    ThreadTitleStatus `json:"title_status,omitempty"`
	TitleUpdatedAt time.Time         `json:"title_updated_at,omitempty"`
	TitleSeedRunID string            `json:"title_seed_run_id,omitempty"`
	RunID          string            `json:"run_id,omitempty"`
	ActiveRunID    string            `json:"active_run_id,omitempty"`
	Workspace      WorkspaceConfig   `json:"workspace"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
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
	store := newThreadStore()
	pathFn := func(threadID string) string {
		if sqliteStore, ok := store.(*sqliteThreadStore); ok {
			return sqliteStore.paths.ThreadTranscriptPath(threadID)
		}
		return desktopdata.Detect().ThreadTranscriptPath(threadID)
	}
	return &RunService{
		sessions:    make(map[string]*Session),
		threads:     make(map[string]*Thread),
		newAgent:    newAgent,
		newExecutor: newExecutor,
		threadStore: store,
		journals:    newJournalManager(pathFn),
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
	if trimmed == "" && len(req.Attachments) == 0 && len(req.ComposerTokens) == 0 {
		return Run{}, ErrInvalidInput
	}
	if s.newExecutor == nil {
		return Run{}, fmt.Errorf("run executor is not configured")
	}
	if s.newProfileAgent != nil && req.ModelProfile == nil {
		return Run{}, ErrMissingModelProfile
	}
	if s.journals == nil && !s.allowNoopRecorder {
		return Run{}, ErrPersistenceUnavailable
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
	effectiveInput := effectiveInputForComposerTokens(trimmed, req.ComposerTokens)
	threadLabel := threadLabelForInputAndTokens(trimmed, persistedAttachments, req.ComposerTokens)
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSession(runID, threadID, trimmed, persistedAttachments, req.ComposerTokens, cancel)
	var threadSnapshot ThreadSnapshot
	shouldGenerateThreadTitle := false

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
				titleSource:     restoredMeta.Snapshot.TitleSource,
				titleStatus:     restoredMeta.Snapshot.TitleStatus,
				titleUpdatedAt:  restoredMeta.Snapshot.TitleUpdatedAt,
				titleSeedRunID:  restoredMeta.Snapshot.TitleSeedRunID,
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
			if err := s.bindAgentRecorder(agent, threadID, runID, req.Client.SessionID, threadSnapshot.Label); err != nil {
				return Run{}, err
			}
			if err := s.threadStore.CreateThread(threadSnapshot, modelProfileKey, threadLabel); err != nil {
				return Run{}, err
			}
			if err := s.threadStore.AppendRunInput(threadSnapshot, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments); err != nil {
				return Run{}, err
			}
			executor := s.newExecutor(agent)
			go s.execute(session, executor, agent, effectiveInput, trimmed, persistedAttachments, ctx)
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
		titleStatus := ThreadTitleStatusReady
		if req.ModelProfile != nil && shouldGenerateThreadTitleForClient(req.Client) {
			titleStatus = ThreadTitleStatusPending
			shouldGenerateThreadTitle = true
		}
		s.threads[threadID] = &Thread{
			threadID:        threadID,
			agent:           agent,
			workspace:       workspace,
			label:           threadLabel,
			titleSource:     ThreadTitleSourceFirstUser,
			titleStatus:     titleStatus,
			titleSeedRunID:  runID,
			titleUpdatedAt:  now,
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

	if err := s.bindAgentRecorder(agent, threadID, runID, req.Client.SessionID, threadSnapshot.Label); err != nil {
		return Run{}, err
	}
	createThreadErr := s.threadStore.CreateThread(threadSnapshot, modelProfileKey, threadLabel)
	appendInputErr := s.threadStore.AppendRunInput(threadSnapshot, session.Snapshot(), req.Client.SessionID, trimmed, persistedAttachments)
	if createThreadErr != nil {
		return Run{}, createThreadErr
	}
	if appendInputErr != nil {
		return Run{}, appendInputErr
	}
	if shouldGenerateThreadTitle {
		go s.scheduleThreadTitleGeneration(context.Background(), threadID, runID, trimmed, persistedAttachments, req.ComposerTokens, req.ModelProfile)
	}
	executor := s.newExecutor(agent)
	go s.execute(session, executor, agent, effectiveInput, trimmed, persistedAttachments, ctx)

	return session.Snapshot(), nil
}

func (s *RunService) bindAgentRecorder(agent *agentcore.Agent, threadID, runID, sessionID, label string) error {
	if agent == nil {
		return nil
	}
	agent.SetRunIdentity(runID, sessionID)
	if s.journals == nil {
		if s.allowNoopRecorder {
			agent.SetSemanticRecorder(agentcore.NoopSemanticRecorder{})
			return nil
		}
		return ErrPersistenceUnavailable
	}
	recorder, err := s.journals.OpenRecorder(threadID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
	}
	if journal, ok := recorder.(*ExecutionJournal); ok {
		if err := ensureThreadJournalRecord(context.Background(), journal, threadID, label); err != nil {
			return fmt.Errorf("%w: %v", ErrPersistenceUnavailable, err)
		}
	}
	agent.SetSemanticRecorder(recorder)
	return nil
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

func shouldGenerateThreadTitleForClient(client ClientInfo) bool {
	return strings.EqualFold(strings.TrimSpace(client.EntryPoint), "desktop")
}

func (s *RunService) scheduleThreadTitleGeneration(ctx context.Context, threadID, seedRunID, firstUserMessage string, attachments []RunInputAttachment, composerTokens []ComposerToken, profile *ModelProfileConfig) {
	if profile == nil {
		log.Printf("[thread-title] skip generation: missing model profile thread=%s run=%s", threadID, seedRunID)
		s.markThreadTitleFailed(threadID, seedRunID)
		return
	}

	prompt := buildThreadTitlePrompt(firstUserMessage, attachments, composerTokens, profile)
	attempts := len(threadTitleRetryDelays) + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		if !s.shouldAttemptGeneratedThreadTitle(threadID, seedRunID) {
			return
		}
		titleCtx, cancel := context.WithTimeout(ctx, threadTitleGenerationTimeout)
		title, err := generateThreadTitleWithProfileFn(titleCtx, profile, prompt)
		cancel()
		title = validGeneratedThreadTitle(title, firstUserMessage)
		if err == nil && title != "" {
			s.ApplyGeneratedThreadTitle(threadID, seedRunID, title)
			return
		}

		reason := "empty_or_low_quality_title"
		if err != nil {
			reason = err.Error()
		}
		log.Printf("[thread-title] generation attempt failed thread=%s run=%s attempt=%d/%d reason=%s", threadID, seedRunID, attempt, attempts, reason)
		if attempt == attempts {
			s.markThreadTitleFailed(threadID, seedRunID)
			return
		}
		timer := time.NewTimer(threadTitleRetryDelays[attempt-1])
		select {
		case <-ctx.Done():
			timer.Stop()
			s.markThreadTitleFailed(threadID, seedRunID)
			return
		case <-timer.C:
		}
	}
}

func (s *RunService) ApplyGeneratedThreadTitle(threadID, seedRunID, title string) {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	title = cleanGeneratedThreadTitle(title)
	if threadID == "" || seedRunID == "" || title == "" {
		return
	}

	now := time.Now().UTC()

	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
	} else if thread.titleSource == ThreadTitleSourceManual {
		s.mu.RUnlock()
		return
	} else if thread.titleSource != ThreadTitleSourceFirstUser || thread.titleStatus != ThreadTitleStatusPending || thread.titleSeedRunID != seedRunID {
		s.mu.RUnlock()
		return
	} else {
		s.mu.RUnlock()
	}

	var persisted ThreadSnapshot
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		persisted, err = s.threadStore.UpdateThreadTitleMetadata(threadID, title, ThreadTitleSourceLLM, ThreadTitleStatusReady, seedRunID, now)
		if err == nil &&
			persisted.TitleSource == ThreadTitleSourceLLM &&
			persisted.TitleStatus == ThreadTitleStatusReady &&
			persisted.TitleSeedRunID == seedRunID &&
			persisted.Label == title {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil ||
		persisted.TitleSource != ThreadTitleSourceLLM ||
		persisted.TitleStatus != ThreadTitleStatusReady ||
		persisted.TitleSeedRunID != seedRunID ||
		persisted.Label != title {
		return
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok &&
		thread.titleSource == ThreadTitleSourceFirstUser &&
		thread.titleStatus == ThreadTitleStatusPending &&
		thread.titleSeedRunID == seedRunID {
		thread.label = persisted.Label
		thread.titleSource = persisted.TitleSource
		thread.titleStatus = persisted.TitleStatus
		thread.titleUpdatedAt = persisted.TitleUpdatedAt
		thread.updatedAt = persisted.UpdatedAt
	}
	s.mu.Unlock()
}

func (s *RunService) shouldAttemptGeneratedThreadTitle(threadID, seedRunID string) bool {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	if threadID == "" || seedRunID == "" {
		return false
	}
	s.mu.RLock()
	thread, ok := s.threads[threadID]
	s.mu.RUnlock()
	if !ok {
		return true
	}
	return thread.titleSource == ThreadTitleSourceFirstUser &&
		thread.titleStatus == ThreadTitleStatusPending &&
		thread.titleSeedRunID == seedRunID
}

func (s *RunService) markThreadTitleFailed(threadID, seedRunID string) {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	if threadID == "" || seedRunID == "" {
		return
	}

	now := time.Now().UTC()

	s.mu.RLock()
	thread, ok := s.threads[threadID]
	if !ok {
		s.mu.RUnlock()
	} else if thread.titleSource == ThreadTitleSourceManual {
		s.mu.RUnlock()
		return
	} else if thread.titleSource != ThreadTitleSourceFirstUser || thread.titleStatus != ThreadTitleStatusPending || thread.titleSeedRunID != seedRunID {
		s.mu.RUnlock()
		return
	} else {
		s.mu.RUnlock()
	}

	var persisted ThreadSnapshot
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		persisted, err = s.threadStore.UpdateThreadTitleMetadata(threadID, "", ThreadTitleSourceFirstUser, ThreadTitleStatusFailed, seedRunID, now)
		if err == nil &&
			persisted.TitleSource == ThreadTitleSourceFirstUser &&
			persisted.TitleStatus == ThreadTitleStatusFailed &&
			persisted.TitleSeedRunID == seedRunID {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil ||
		persisted.TitleSource != ThreadTitleSourceFirstUser ||
		persisted.TitleStatus != ThreadTitleStatusFailed ||
		persisted.TitleSeedRunID != seedRunID {
		return
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok &&
		thread.titleSource == ThreadTitleSourceFirstUser &&
		thread.titleStatus == ThreadTitleStatusPending &&
		thread.titleSeedRunID == seedRunID {
		thread.titleStatus = persisted.TitleStatus
		thread.titleUpdatedAt = persisted.TitleUpdatedAt
		thread.updatedAt = persisted.UpdatedAt
	}
	s.mu.Unlock()
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
		thread.titleSource = ThreadTitleSourceManual
		thread.titleStatus = ThreadTitleStatusReady
		thread.titleUpdatedAt = now
		thread.updatedAt = now
		snapshot := thread.snapshot()
		s.mu.Unlock()
		return s.threadStore.RenameThread(threadID, normalizedLabel, snapshot.UpdatedAt)
	}
	s.mu.Unlock()
	return s.threadStore.RenameThread(threadID, normalizedLabel, now)
}

func (s *RunService) FlushTranscripts() error {
	s.mu.RLock()
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	s.mu.RUnlock()

	var firstErr error
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
		if err := s.flushAgentTranscript(session, thread.agent); err != nil && firstErr == nil {
			firstErr = err
		}
		_ = snapshot
	}
	if s.journals != nil {
		if err := s.journals.FlushAll(context.Background()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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

func (s *RunService) execute(session *Session, executor Executor, agent *agentcore.Agent, input string, displayInput string, attachments []RunInputAttachment, ctx context.Context) {
	snapshot := session.Snapshot()
	defer func() {
		restoreAgentDisplayInput(agent, input, displayInput)
		s.flushAgentTranscript(session, agent)
		s.clearActiveRun(snapshot.ThreadID, snapshot.RunID)
	}()

	if executor == nil {
		session.CompleteError("run executor is not configured", "executor_missing")
		return
	}
	if agent != nil {
		agent.SetPendingUserAttachments(inputAttachmentsToContextAttachments(attachments))
		if err := s.preActivateComposerSkills(ctx, agent, snapshot); err != nil {
			session.CompleteError(err.Error(), skillActivationErrorCode(err))
			return
		}
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

func skillActivationErrorCode(err error) string {
	var compat *skillruntime.SkillCompatibilityError
	if errors.As(err, &compat) && compat != nil && strings.TrimSpace(compat.Code) != "" {
		return compat.Code
	}
	return "skill_activation_failed"
}

func (s *RunService) preActivateComposerSkills(ctx context.Context, agent *agentcore.Agent, snapshot Run) error {
	tokens := skillComposerTokens(snapshot.ComposerTokens)
	if len(tokens) == 0 || agent == nil {
		return nil
	}
	s.mu.RLock()
	activator := s.skillActivator
	s.mu.RUnlock()
	if activator == nil {
		return nil
	}

	parentPool := skillruntime.CurrentParentToolPool()
	cfg := agent.GetConfig()
	var blocks []agentcore.ContextBlock
	effective := append([]string(nil), parentPool...)
	recorder := agent.SemanticRecorder()

	for _, token := range tokens {
		skillID := strings.TrimSpace(firstNonEmpty(token.Payload.SkillID, token.ID))
		if skillID == "" {
			continue
		}
		if strings.HasPrefix(skillID, "skill:") {
			parts := strings.Split(skillID, ":")
			if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
				skillID = strings.TrimSpace(parts[1])
			}
		}
		plan, err := activator.Activate(skillruntime.ActivationRequest{
			SkillID:        skillID,
			ParentToolPool: parentPool,
			SkipArgRender:  true,
		})
		if err != nil {
			return err
		}
		effective = skillruntime.IntersectToolPools(effective, plan.EffectiveTools)
		blocks = append(blocks, agentcore.ContextBlock{
			ID:      "skill_activation_" + plan.SkillID,
			Content: plan.MainBody,
		})
		if recorder != nil {
			if err := emitSkillActivationRecords(ctx, recorder, snapshot, plan); err != nil {
				return err
			}
		}
	}

	cfg.ResolvedToolPool = effective
	cfg.SkillContextBlocks = blocks
	agent.Reconfigure(cfg)
	return nil
}

func emitSkillActivationRecords(ctx context.Context, recorder agentcore.SemanticRecorder, snapshot Run, plan *skillruntime.SkillActivationPlan) error {
	if recorder == nil || plan == nil {
		return nil
	}
	activated := agentcore.NewSemanticRecord(
		agentcore.SemanticKindSkillActivated,
		snapshot.ThreadID,
		snapshot.RunID,
		"",
		"",
		"",
		map[string]any{
			"skill_id":              plan.SkillID,
			"skill_version":         plan.SkillVersion,
			"effective_tools":       append([]string(nil), plan.EffectiveTools...),
			"compatibility_status":  plan.CompatibilityStatus,
			"required_tools":        append([]string(nil), plan.RequiredTools...),
			"required_capabilities": capabilityStrings(plan.RequiredCapabilities),
		},
	)
	if err := recorder.Append(ctx, activated); err != nil {
		return err
	}
	for _, receipt := range plan.ResourceReceipts {
		rec := agentcore.NewSemanticRecord(
			agentcore.SemanticKindResourceReceipt,
			snapshot.ThreadID,
			snapshot.RunID,
			"",
			"",
			"",
			map[string]any{
				"skill_id":      receipt.SkillID,
				"skill_version": receipt.SkillVersion,
				"resource":      receipt.Resource,
				"source":        receipt.Source,
				"sha256":        receipt.SHA256,
				"size":          receipt.Size,
				"mode":          receipt.Mode,
				"loaded_at":     receipt.LoadedAt.UTC().Format(time.RFC3339Nano),
			},
		)
		if err := recorder.Append(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

func capabilityStrings(caps []tool.CapabilityClass) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		if strings.TrimSpace(string(c)) == "" {
			continue
		}
		out = append(out, string(c))
	}
	return out
}

func effectiveInputForComposerTokens(input string, tokens []ComposerToken) string {
	skillTokens := skillComposerTokens(tokens)
	if len(skillTokens) == 0 {
		return input
	}

	// Explicit Skill chips are host-pre-activated into typed context + tool
	// pool. Keep the user task text unchanged; do not inject a natural-language
	// instruction that only asks the model to call skill__xxx.
	if strings.TrimSpace(input) != "" {
		return input
	}
	return "Use the selected Skill to help the user."
}

func skillComposerTokens(tokens []ComposerToken) []ComposerToken {
	result := make([]ComposerToken, 0, len(tokens))
	for _, token := range tokens {
		if strings.EqualFold(strings.TrimSpace(token.Type), "skill") {
			result = append(result, token)
		}
	}
	return result
}

func threadLabelForInputAndTokens(input string, attachments []RunInputAttachment, tokens []ComposerToken) string {
	if strings.TrimSpace(input) != "" || len(attachments) > 0 {
		return threadLabelForInput(input, attachments)
	}
	for _, token := range skillComposerTokens(tokens) {
		if label := trimThreadLabel(strings.TrimSpace(token.Label)); label != "" {
			return label
		}
		if label := trimThreadLabel(strings.TrimSpace(firstNonEmpty(token.Payload.SkillID, token.ID))); label != "" {
			return label
		}
	}
	return threadLabelForInput(input, attachments)
}

func restoreAgentDisplayInput(agent *agentcore.Agent, effectiveInput string, displayInput string) {
	if agent == nil || effectiveInput == displayInput {
		return
	}
	messages := agent.GetConversation().Context.GetMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == convcontext.RoleUser && messages[i].Content == effectiveInput {
			messages[i].Content = displayInput
			agent.GetConversation().Context.SetMessages(messages)
			return
		}
	}
}

func (s *RunService) flushAgentTranscript(session *Session, agent *agentcore.Agent) error {
	if session == nil || agent == nil {
		return nil
	}
	run := session.Snapshot()
	snapshot, ok := s.threadSnapshot(run.ThreadID)
	if !ok {
		return nil
	}
	var firstErr error
	if recorder := agent.SemanticRecorder(); recorder != nil {
		if err := recorder.Flush(context.Background()); err != nil {
			firstErr = err
		}
	}
	if err := s.threadStore.AppendConversation(snapshot, run, agent.GetConversation().Context.GetMessages()); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.threadStore.AppendRunEvents(snapshot, run, session.History(0)); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
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
		ThreadID:       t.threadID,
		Label:          t.label,
		TitleSource:    t.titleSource,
		TitleStatus:    t.titleStatus,
		TitleUpdatedAt: t.titleUpdatedAt,
		TitleSeedRunID: t.titleSeedRunID,
		RunID:          t.latestRunID,
		ActiveRunID:    t.activeRunID,
		Workspace:      t.workspace,
		CreatedAt:      t.createdAt,
		UpdatedAt:      t.updatedAt,
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
