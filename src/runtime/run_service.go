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
	"awaken/src/backgroundnotify"
	"awaken/src/busyinput"
	convcontext "awaken/src/context"
	"awaken/src/imagerouting"
	"awaken/src/lifecyclehooks"
	"awaken/src/llm"
	"awaken/src/ratelimitoptions"
	"awaken/src/sessionstate"
	"awaken/src/sessiontags"
	"awaken/src/slashcommands"
	"awaken/src/titlegen"
	"awaken/src/tool"
	"awaken/src/workspaceguard"
)

var (
	ErrInvalidInput              = errors.New("input is required")
	ErrInvalidSlashCommand       = errors.New("invalid slash command")
	ErrRunNotFound               = errors.New("run not found")
	ErrApprovalNotFound          = errors.New("approval not found")
	ErrRunTerminal               = errors.New("run is already finished")
	ErrRunCancelling             = errors.New("run is being cancelled")
	ErrThreadNotFound            = errors.New("thread not found")
	ErrThreadAlreadyExists       = errors.New("thread already exists")
	ErrThreadWorkspaceMismatch   = errors.New("thread workspace mismatch")
	ErrThreadBusy                = errors.New("thread already has an active run")
	ErrParentRunNotFound         = errors.New("parent run not found")
	ErrParentThreadNotFound      = errors.New("parent thread not found")
	ErrDelegationParentMismatch  = errors.New("delegation parent run does not belong to parent thread")
	ErrDelegationParentRequired  = errors.New("delegation parent_run_id or parent_thread_id is required")
	ErrInvalidRunAt              = errors.New("run_at is required")
	ErrScheduledRunNotFound      = errors.New("scheduled run not found")
	ErrScheduledRunNotCancelable = errors.New("scheduled run is not cancelable")
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
	Profile          *agentcore.RunProfile    `json:"profile,omitempty"`
	Delegation       *DelegationMetadata      `json:"delegation,omitempty"`
	PolicyOptions    RunPolicyOptions         `json:"policy_options,omitempty"`
	Attachments      []RunAttachment          `json:"attachments,omitempty"`
}

type RunPolicyOptions struct {
	BusyInputMode              string `json:"busy_input_mode,omitempty"`
	BackgroundNotificationMode string `json:"background_notification_mode,omitempty"`
	ImageInputMode             string `json:"image_input_mode,omitempty"`
}

type RunAttachment struct {
	Path string `json:"path"`
	Type string `json:"type,omitempty"`
}

type Executor interface {
	Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error)
}

type ExecutorFunc func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)

func (f ExecutorFunc) Run(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
	return f(ctx, input, sink)
}

type RunService struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	threads        map[string]*Thread
	scheduledRuns  map[string]*scheduledRunState
	queuedRuns     map[string]queuedRunExecution
	scheduleClock  scheduleClock
	auditLog       *AuditLog
	newAgent       func(workspace WorkspaceConfig, threadID string) *agentcore.Agent
	newExecutor    func(agent *agentcore.Agent) Executor
	store          *fileStore
	hooks          *lifecyclehooks.Executor
	titleGenerator titlegen.Generator
}

type RunServiceConfig struct {
	Hooks          lifecyclehooks.Config
	TitleGenerator titlegen.Generator
}

// Thread holds the reusable Agent and metadata for a multi-turn conversation.
// Runs with the same thread_id reuse the same Agent and Conversation.
type Thread struct {
	threadID     string
	agent        *agentcore.Agent
	conversation *convcontext.Conversation
	workspace    WorkspaceConfig
	label        string
	metadata     sessiontags.Metadata
	branch       *ThreadBranchMetadata
	latestRunID  string
	activeRunID  string
	delegation   *DelegationMetadata
	runIDs       []string
	queuedRunIDs []string
	createdAt    time.Time
	updatedAt    time.Time
}

type queuedRunExecution struct {
	executorInput    string
	workspace        WorkspaceConfig
	ctx              context.Context
	permissionPolicy tool.PermissionPolicy
	approvalPolicy   agentcore.ApprovalPolicy
	profile          *agentcore.RunProfile
	policyOptions    RunPolicyOptions
	attachments      []RunAttachment
}

type ThreadSnapshot struct {
	ThreadID      string                              `json:"thread_id"`
	Label         string                              `json:"label"`
	State         sessionstate.State                  `json:"state"`
	PendingAction *sessionstate.RequiresActionDetails `json:"pending_action,omitempty"`
	Metadata      ThreadMetadata                      `json:"metadata"`
	RunID         string                              `json:"run_id,omitempty"`
	ActiveRunID   string                              `json:"active_run_id,omitempty"`
	Delegation    *DelegationMetadata                 `json:"delegation,omitempty"`
	LatestRun     *ThreadRunBrief                     `json:"latest_run,omitempty"`
	Workspace     WorkspaceConfig                     `json:"workspace"`
	CreatedAt     time.Time                           `json:"created_at"`
	UpdatedAt     time.Time                           `json:"updated_at"`
}

type ThreadRunBrief struct {
	RunID      string     `json:"run_id"`
	Status     RunStatus  `json:"status"`
	Error      string     `json:"error,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
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
	return NewRunServiceWithConfig(newAgent, newExecutor, RunServiceConfig{}, opts...)
}

func NewRunServiceWithConfig(
	newAgent func(workspace WorkspaceConfig, threadID string) *agentcore.Agent,
	newExecutor func(agent *agentcore.Agent) Executor,
	config RunServiceConfig,
	opts ...RunServiceOption,
) *RunService {
	service := &RunService{
		sessions:       make(map[string]*Session),
		threads:        make(map[string]*Thread),
		scheduledRuns:  make(map[string]*scheduledRunState),
		queuedRuns:     make(map[string]queuedRunExecution),
		scheduleClock:  realScheduleClock{},
		auditLog:       NewAuditLog(defaultAuditLogCapacity),
		newAgent:       newAgent,
		newExecutor:    newExecutor,
		hooks:          lifecyclehooks.NewExecutor(config.Hooks),
		titleGenerator: normalizeTitleGenerator(config.TitleGenerator),
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
	expansion, err := slashcommands.ExpandInput(req.Input)
	if err != nil {
		return Run{}, fmt.Errorf("%w: %v", ErrInvalidSlashCommand, err)
	}
	executorInput := trimmed
	if expansion.Applied {
		executorInput = expansion.AgentInput
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
	if req.Profile != nil {
		normalized := req.Profile.Normalize()
		if err := normalized.Validate(); err != nil {
			return Run{}, fmt.Errorf("%w: invalid profile: %v", ErrInvalidInput, err)
		}
		req.Profile = &normalized
	}

	runID := newRuntimeID("run")
	reuseThreadID := strings.TrimSpace(req.ThreadID)
	threadID := reuseThreadID
	now := time.Now().UTC()
	var agent *agentcore.Agent
	if threadID == "" {
		threadID = newRuntimeID("thread")
		if s.newAgent != nil {
			agent = s.newAgent(workspace, threadID)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	delegation, err := s.resolveDelegationLocked(req.Delegation, trimmed)
	if err != nil {
		s.mu.Unlock()
		cancel()
		return Run{}, err
	}
	if reuseThreadID != "" {
		thread, ok := s.threads[threadID]
		if !ok {
			s.mu.Unlock()
			cancel()
			return Run{}, ErrThreadNotFound
		}
		if !sameWorkspace(thread.workspace, workspace) {
			s.mu.Unlock()
			cancel()
			return Run{}, ErrThreadWorkspaceMismatch
		}
		if thread.activeRunID != "" {
			if strings.TrimSpace(req.PolicyOptions.BusyInputMode) == "" {
				s.mu.Unlock()
				cancel()
				return Run{}, ErrThreadBusy
			}
			activeSession := s.sessions[thread.activeRunID]
			decision := busyinput.Decide(busyinput.Request{
				Mode:     busyinput.NormalizeMode(req.PolicyOptions.BusyInputMode),
				State:    busyinput.StateBusy,
				CanSteer: false,
				Now:      now,
			})
			if !decision.Accept || !decision.QueuePending {
				s.mu.Unlock()
				cancel()
				return Run{}, ErrThreadBusy
			}
			if delegation == nil && thread.delegation != nil {
				delegation = cloneDelegationMetadata(thread.delegation)
			}
			session := NewSessionWithConfig(runID, threadID, trimmed, cancel, SessionConfig{
				ApprovalPolicy: approvalPolicy,
				Observer:       persistenceObserver{store: s.store},
				Delegation:     delegation,
				InitialStatus:  RunStatusQueued,
			})
			session.SetSecurityAuditRecorder(s.auditLog)
			s.sessions[runID] = session
			s.queuedRuns[runID] = queuedRunExecution{
				executorInput:    executorInput,
				workspace:        workspace,
				ctx:              ctx,
				permissionPolicy: permissionPolicy,
				approvalPolicy:   approvalPolicy,
				profile:          req.Profile,
				policyOptions:    req.PolicyOptions,
				attachments:      append([]RunAttachment(nil), req.Attachments...),
			}
			thread.latestRunID = runID
			thread.queuedRunIDs = append(thread.queuedRunIDs, runID)
			thread.runIDs = append(thread.runIDs, runID)
			thread.updatedAt = now
			s.mu.Unlock()

			if s.store != nil {
				persistenceObserver{store: s.store}.OnRunUpdated(session.Snapshot())
				s.persistThread(threadID)
			}
			session.RecordAgentEvent(policyDecisionEvent(
				"busy_input",
				string(decision.Action),
				decision.Reason,
				decision.Status,
				map[string]interface{}{"busy_input": decision},
			))
			if decision.InterruptRequested && activeSession != nil {
				activeSession.RequestCancel()
			}
			return session.Snapshot(), nil
		}
		if delegation == nil && thread.delegation != nil {
			delegation = cloneDelegationMetadata(thread.delegation)
		}
		agent = thread.agent
		if thread.conversation == nil {
			thread.conversation = conversationForAgent(agent)
		}
		thread.activeRunID = runID
		thread.latestRunID = runID
		if thread.delegation == nil && delegation != nil {
			thread.delegation = cloneDelegationMetadata(delegation)
		}
		thread.metadata = applyThreadPromptMetadata(thread.metadata, threadID, trimmed, now)
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
			metadata:     applyThreadPromptMetadata(sessiontags.Metadata{SessionID: threadID, Mode: sessiontags.ModeNormal}, threadID, trimmed, now),
			latestRunID:  runID,
			activeRunID:  runID,
			delegation:   cloneDelegationMetadata(delegation),
			runIDs:       []string{runID},
			createdAt:    now,
			updatedAt:    now,
		}
		s.pruneInactiveThreadsLocked()
	}
	session := NewSessionWithConfig(runID, threadID, trimmed, cancel, SessionConfig{
		ApprovalPolicy: approvalPolicy,
		Observer:       persistenceObserver{store: s.store},
		Delegation:     delegation,
	})
	session.SetSecurityAuditRecorder(s.auditLog)
	s.sessions[runID] = session
	s.mu.Unlock()

	if s.store != nil {
		persistenceObserver{store: s.store}.OnRunUpdated(session.Snapshot())
		s.persistThread(threadID)
	}

	effectiveAgent, err := effectiveAgentForRun(agent, workspace, permissionPolicy, approvalPolicy, req.Profile)
	if err != nil {
		return Run{}, fmt.Errorf("%w: invalid profile: %v", ErrInvalidInput, err)
	}
	executor := s.newExecutor(effectiveAgent)
	ctx, executorInput = s.prepareRunExecution(session, effectiveAgent, executorInput, req.PolicyOptions, req.Attachments, ctx)
	s.recordRunAudit(session.Snapshot(), agentcore.SecurityAuditEvent{
		Type:    agentcore.SecurityAuditRunStarted,
		Action:  agentcore.SecurityAuditActionRun,
		Outcome: agentcore.SecurityAuditOutcomeStarted,
		Summary: "run started",
	})
	go s.execute(session, executor, executorInput, workspace, ctx)

	return session.Snapshot(), nil
}

func (s *RunService) prepareRunExecution(
	session *Session,
	agent *agentcore.Agent,
	input string,
	options RunPolicyOptions,
	attachments []RunAttachment,
	ctx context.Context,
) (context.Context, string) {
	if strings.TrimSpace(options.BackgroundNotificationMode) != "" {
		bgMode, _ := backgroundnotify.NormalizeMode(options.BackgroundNotificationMode)
		if bgMode == "" {
			bgMode = backgroundnotify.ModeAll
		}
		session.RecordAgentEvent(policyDecisionEvent(
			"background_notification",
			string(bgMode),
			"run notification policy resolved",
			"notification mode: "+string(bgMode),
			map[string]interface{}{"mode": string(bgMode)},
		))
	}

	imagePaths := imageAttachmentPaths(attachments)
	if len(imagePaths) == 0 || agent == nil {
		return ctx, input
	}

	config := agent.GetConfig()
	protocol := llm.ResolveProtocol(llm.Config{
		BaseURL: config.ModelConfig.BaseURL,
		Model:   config.ModelConfig.Model,
	})
	decision := imagerouting.Decide(string(protocol), config.ModelConfig.Model, imagerouting.Config{
		ImageInputMode: imagerouting.Mode(options.ImageInputMode),
	}, modelVisionCapabilities)

	nativeSupported := protocol == llm.ProtocolOpenAIChat && decision.Mode == imagerouting.ModeNative
	route := "text_fallback"
	status := "image paths added as text references"
	reason := decision.Reason
	if nativeSupported {
		partsResult := imagerouting.BuildContentParts(input, imagePaths)
		parts := make([]convcontext.ContentPart, 0, len(partsResult.Parts))
		for _, part := range partsResult.Parts {
			converted := convcontext.ContentPart{Type: part.Type, Text: part.Text}
			if part.ImageURL != nil {
				converted.ImageURL = &convcontext.ImageURL{URL: part.ImageURL.URL}
			}
			parts = append(parts, converted)
		}
		if len(partsResult.Images) > 0 {
			ctx = agentcore.ContextWithUserContentParts(ctx, parts)
			route = "native"
			status = "images routed as OpenAI-compatible image parts"
		}
		if len(partsResult.Skipped) > 0 {
			route = "partial_native"
			status = "some images could not be read"
		}
		session.RecordAgentEvent(policyDecisionEvent(
			"image_routing",
			route,
			reason,
			status,
			map[string]interface{}{
				"mode":    string(decision.Mode),
				"routes":  imageRoutes(imagePaths, route, reason),
				"skipped": partsResult.Skipped,
			},
		))
		return ctx, input
	}

	if decision.Mode == imagerouting.ModeNative {
		route = "unsupported"
		status = "native image routing unavailable for this provider; image paths added as text references"
		reason = "native image routing requires OpenAI-compatible chat vision"
	}
	input = prependImagePathRefs(input, imagePaths)
	session.RecordAgentEvent(policyDecisionEvent(
		"image_routing",
		route,
		reason,
		status,
		map[string]interface{}{
			"mode":   string(decision.Mode),
			"routes": imageRoutes(imagePaths, route, reason),
		},
	))
	return ctx, input
}

func (s *RunService) startNextQueuedRun(threadID string) {
	s.mu.Lock()
	thread, ok := s.threads[threadID]
	if !ok || thread.activeRunID != "" || len(thread.queuedRunIDs) == 0 {
		s.mu.Unlock()
		return
	}

	var runID string
	var queued queuedRunExecution
	var session *Session
	for len(thread.queuedRunIDs) > 0 {
		runID = thread.queuedRunIDs[0]
		thread.queuedRunIDs = thread.queuedRunIDs[1:]
		var found bool
		queued, found = s.queuedRuns[runID]
		if !found {
			continue
		}
		session = s.sessions[runID]
		if session == nil {
			delete(s.queuedRuns, runID)
			continue
		}
		if isTerminalStatus(session.Snapshot().Status) {
			delete(s.queuedRuns, runID)
			continue
		}
		break
	}
	if session == nil {
		s.mu.Unlock()
		return
	}
	delete(s.queuedRuns, runID)
	thread.activeRunID = runID
	thread.latestRunID = runID
	thread.updatedAt = time.Now().UTC()
	agent := thread.agent
	s.mu.Unlock()

	session.StartQueuedRun()
	effectiveAgent, err := effectiveAgentForRun(agent, queued.workspace, queued.permissionPolicy, queued.approvalPolicy, queued.profile)
	if err != nil {
		session.CompleteError(fmt.Sprintf("%v: invalid profile: %v", ErrInvalidInput, err), "INVALID_PROFILE")
		return
	}
	executor := s.newExecutor(effectiveAgent)
	ctx, input := s.prepareRunExecution(session, effectiveAgent, queued.executorInput, queued.policyOptions, queued.attachments, queued.ctx)
	s.recordRunAudit(session.Snapshot(), agentcore.SecurityAuditEvent{
		Type:    agentcore.SecurityAuditRunStarted,
		Action:  agentcore.SecurityAuditActionRun,
		Outcome: agentcore.SecurityAuditOutcomeStarted,
		Summary: "queued run started",
	})
	go s.execute(session, executor, input, queued.workspace, ctx)
}

func imageAttachmentPaths(attachments []RunAttachment) []string {
	paths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.Path) == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(attachment.Type))
		if kind != "" && kind != "image" {
			continue
		}
		paths = append(paths, strings.TrimSpace(attachment.Path))
	}
	return paths
}

func prependImagePathRefs(input string, imagePaths []string) string {
	if len(imagePaths) == 0 {
		return input
	}
	refs := make([]string, 0, len(imagePaths))
	for _, path := range imagePaths {
		refs = append(refs, fmt.Sprintf("@%q", path))
	}
	return strings.TrimSpace(strings.Join(refs, " ") + " " + input)
}

func imageRoutes(paths []string, route, reason string) []map[string]string {
	routes := make([]map[string]string, 0, len(paths))
	for _, path := range paths {
		routes = append(routes, map[string]string{
			"path":   path,
			"route":  route,
			"reason": reason,
		})
	}
	return routes
}

func modelVisionCapabilities(provider, model string) (imagerouting.ModelCapabilities, bool) {
	if provider != string(llm.ProtocolOpenAIChat) {
		return imagerouting.ModelCapabilities{SupportsVision: false}, true
	}
	normalized := strings.ToLower(strings.TrimSpace(model))
	for _, token := range []string{"gpt-4o", "gpt-4.1", "gpt-4.5", "o3", "o4", "vision", "gemini"} {
		if strings.Contains(normalized, token) {
			return imagerouting.ModelCapabilities{SupportsVision: true}, true
		}
	}
	return imagerouting.ModelCapabilities{SupportsVision: false}, true
}

func policyDecisionEvent(category, decision, reason, status string, payload map[string]interface{}) agentcore.AwakenEvent {
	return agentcore.AwakenEvent{
		Type:           agentcore.PolicyDecisionEventType,
		PolicyCategory: category,
		PolicyDecision: decision,
		PolicyReason:   reason,
		PolicyStatus:   status,
		PolicyPayload:  payload,
	}
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

func (s *RunService) AuditEvents(filter AuditEventFilter) []agentcore.SecurityAuditEvent {
	if s == nil || s.auditLog == nil {
		return nil
	}
	return s.auditLog.Events(filter)
}

func (s *RunService) ListThreads() []ThreadSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	threads := make([]ThreadSnapshot, 0, len(s.threads))
	for _, thread := range s.threads {
		threads = append(threads, s.snapshotThreadLocked(thread))
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
	snapshot := s.snapshotThreadLocked(thread)
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

type DrainResult struct {
	ActiveRuns int           `json:"active_runs"`
	QueuedRuns int           `json:"queued_runs"`
	TimedOut   bool          `json:"timed_out"`
	Timeout    time.Duration `json:"-"`
	TimeoutMS  int64         `json:"timeout_ms"`
}

func (s *RunService) Drain(ctx context.Context, timeout time.Duration) DrainResult {
	if s == nil {
		return DrainResult{}
	}
	if timeout < 0 {
		timeout = 0
	}
	if timeout == 0 {
		active, queued := s.activeAndQueuedCounts()
		return DrainResult{ActiveRuns: active, QueuedRuns: queued, TimedOut: active+queued > 0, Timeout: timeout, TimeoutMS: 0}
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		active, queued := s.activeAndQueuedCounts()
		if active+queued == 0 {
			return DrainResult{Timeout: timeout, TimeoutMS: timeout.Milliseconds()}
		}
		select {
		case <-ctx.Done():
			return DrainResult{ActiveRuns: active, QueuedRuns: queued, TimedOut: true, Timeout: timeout, TimeoutMS: timeout.Milliseconds()}
		case <-deadline.C:
			active, queued = s.activeAndQueuedCounts()
			return DrainResult{ActiveRuns: active, QueuedRuns: queued, TimedOut: active+queued > 0, Timeout: timeout, TimeoutMS: timeout.Milliseconds()}
		case <-ticker.C:
		}
	}
}

func (s *RunService) activeAndQueuedCounts() (int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active := 0
	queued := 0
	for _, thread := range s.threads {
		if thread.activeRunID != "" {
			active++
		}
		for _, runID := range thread.queuedRunIDs {
			session := s.sessions[runID]
			if session != nil && session.Snapshot().Status == RunStatusQueued {
				queued++
			}
		}
	}
	return active, queued
}

func (s *RunService) ResolveApproval(runID, approvalID string, decision agentcore.ApprovalDecision) error {
	session, err := s.session(runID)
	if err != nil {
		return err
	}
	return session.ResolveApproval(approvalID, decision)
}

func (s *RunService) execute(session *Session, executor Executor, input string, workspace WorkspaceConfig, ctx context.Context) {
	snapshot := session.Snapshot()
	defer s.clearActiveRun(snapshot.ThreadID, snapshot.RunID)

	metadata := runHookMetadata(snapshot, workspace)
	s.executeHook(ctx, lifecyclehooks.EventBeforeRun, metadata)

	runStatus := string(RunStatusError)
	defer func() {
		afterMetadata := cloneHookMetadata(metadata)
		afterMetadata["run_status"] = runStatus
		go s.executeHook(context.Background(), lifecyclehooks.EventAfterRun, afterMetadata)
	}()

	if executor == nil {
		session.CompleteError("run executor is not configured", "executor_missing")
		return
	}

	result, err := executor.Run(ctx, input, sessionEventSink{session: session})
	if err == nil {
		if session.CancelRequested() {
			runStatus = string(RunStatusCancelled)
			session.CompleteCancelled("run cancelled")
			return
		}
		runStatus = string(RunStatusFinished)
		session.CompleteFinished(result)
		return
	}

	if errors.Is(ctx.Err(), context.Canceled) || session.CancelRequested() {
		runStatus = string(RunStatusCancelled)
		session.CompleteCancelled("run cancelled")
		return
	}

	runStatus = string(RunStatusError)
	session.CompleteErrorWithInfo(err.Error(), "", providerErrorInfo(err))
}

func providerErrorInfo(err error) ProviderErrorInfo {
	var requestErr *llm.RequestError
	if !errors.As(err, &requestErr) {
		return ProviderErrorInfo{}
	}
	reason := strings.TrimSpace(requestErr.ProviderReason)
	if reason == "" {
		reason = string(requestErr.Kind)
	}
	info := ProviderErrorInfo{
		ProviderReason: reason,
		Retryable:      requestErr.Retryable(),
	}
	if requestErr.Kind == llm.ErrorKindRateLimit || reason == "rate_limit" {
		for _, option := range ratelimitoptions.BuildOptions(ratelimitoptions.MenuInput{
			SubscriptionType:            ratelimitoptions.SubscriptionPro,
			ExtraUsageFeatureEnabled:    true,
			AccountHasExtraUsageEnabled: false,
			HasBillingAccess:            true,
			UpgradeEnabled:              true,
		}) {
			info.RateLimitOptions = append(info.RateLimitOptions, agentcore.RateLimitOption{
				Label: option.Label,
				Value: string(option.Value),
			})
		}
	}
	return info
}

func (s *RunService) recordRunAudit(run Run, event agentcore.SecurityAuditEvent) {
	if s == nil || s.auditLog == nil {
		return
	}
	event.RunID = firstNonEmpty(event.RunID, run.RunID)
	event.ThreadID = firstNonEmpty(event.ThreadID, run.ThreadID)
	s.auditLog.RecordSecurityAuditEvent(event)
}

func (s *RunService) executeHook(ctx context.Context, event lifecyclehooks.Event, metadata map[string]string) {
	if s.hooks == nil || !s.hooks.HasHooks() {
		return
	}
	s.hooks.Execute(ctx, event, metadata)
}

func runHookMetadata(run Run, workspace WorkspaceConfig) map[string]string {
	return map[string]string{
		"run_id":         run.RunID,
		"thread_id":      run.ThreadID,
		"workspace_root": workspace.WorkspaceRoot,
	}
}

func cloneHookMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func (s *RunService) clearActiveRun(threadID, runID string) {
	if threadID == "" || runID == "" {
		return
	}

	s.mu.RLock()
	session := s.sessions[runID]
	s.mu.RUnlock()

	var run Run
	if session != nil {
		run = session.Snapshot()
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
	titleInputs := s.updateThreadAfterRunLocked(thread, run)
	s.mu.Unlock()

	s.persistThread(threadID)

	// Run the (potentially slow) LLM title generator without holding s.mu so
	// that thread listing, context reads, branching, and the next StartRun
	// are not blocked while the LLM responds.
	if titleInputs != nil {
		go s.generateThreadTitleAsync(*titleInputs)
	}
	s.startNextQueuedRun(threadID)
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
		restoredConversation := restoreConversation(record)
		thread := &Thread{
			threadID:     record.Thread.ThreadID,
			conversation: restoredConversation,
			workspace:    record.Thread.Workspace,
			label:        record.Thread.Label,
			metadata:     metadataFromSnapshot(record.Thread),
			branch:       cloneThreadBranchMetadata(record.Thread.Metadata.Branch),
			latestRunID:  record.Thread.RunID,
			activeRunID:  "",
			delegation:   cloneDelegationMetadata(record.Thread.Delegation),
			runIDs:       append([]string(nil), record.RunIDs...),
			createdAt:    record.Thread.CreatedAt,
			updatedAt:    record.Thread.UpdatedAt,
		}

		if s.newAgent != nil {
			thread.agent = s.newAgent(thread.workspace, thread.threadID)
			if thread.agent != nil {
				thread.conversation = thread.agent.GetConversation()
				if thread.conversation != nil {
					thread.conversation.Context.SetMessages(restoredConversation.Context.GetMessages())
					thread.conversation.CreatedAt = restoredConversation.CreatedAt
					thread.conversation.UpdatedAt = restoredConversation.UpdatedAt
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

func (s *RunService) resolveDelegationLocked(delegation *DelegationMetadata, input string) (*DelegationMetadata, error) {
	if delegation == nil {
		return nil, nil
	}

	normalized := DelegationMetadata{
		ParentRunID:      strings.TrimSpace(delegation.ParentRunID),
		ParentThreadID:   strings.TrimSpace(delegation.ParentThreadID),
		DelegateLabel:    strings.TrimSpace(delegation.DelegateLabel),
		DelegateRole:     strings.TrimSpace(delegation.DelegateRole),
		DelegationPrompt: strings.TrimSpace(delegation.DelegationPrompt),
	}
	if !hasDelegationFields(normalized) {
		return nil, nil
	}
	if normalized.ParentRunID == "" && normalized.ParentThreadID == "" {
		return nil, ErrDelegationParentRequired
	}

	if normalized.ParentRunID != "" {
		parentSession, ok := s.sessions[normalized.ParentRunID]
		if !ok {
			return nil, ErrParentRunNotFound
		}
		parentRun := parentSession.Snapshot()
		if normalized.ParentThreadID == "" {
			normalized.ParentThreadID = parentRun.ThreadID
		} else if normalized.ParentThreadID != parentRun.ThreadID {
			return nil, ErrDelegationParentMismatch
		}
	}
	if normalized.ParentThreadID != "" && normalized.ParentRunID == "" {
		if _, ok := s.threads[normalized.ParentThreadID]; !ok {
			return nil, ErrParentThreadNotFound
		}
	}
	if normalized.DelegationPrompt == "" {
		normalized.DelegationPrompt = input
	}

	return &normalized, nil
}

func hasDelegationFields(delegation DelegationMetadata) bool {
	return delegation.ParentRunID != "" ||
		delegation.ParentThreadID != "" ||
		delegation.DelegateLabel != "" ||
		delegation.DelegateRole != "" ||
		delegation.DelegationPrompt != ""
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

func (s *RunService) latestRunBriefLocked(thread *Thread) *ThreadRunBrief {
	if thread == nil || thread.latestRunID == "" {
		return nil
	}
	session, ok := s.sessions[thread.latestRunID]
	if !ok {
		return nil
	}
	run := session.Snapshot()
	return &ThreadRunBrief{
		RunID:      run.RunID,
		Status:     run.Status,
		Error:      run.Error,
		UpdatedAt:  run.UpdatedAt,
		FinishedAt: run.FinishedAt,
	}
}

func (t *Thread) snapshot(latestRun *ThreadRunBrief) ThreadSnapshot {
	metadata := t.metadataSnapshot()
	label := firstNonEmpty(metadata.DisplayTitle, t.label, "Untitled thread")
	return ThreadSnapshot{
		ThreadID:    t.threadID,
		Label:       label,
		State:       threadState(latestRun),
		Metadata:    metadata,
		RunID:       t.latestRunID,
		ActiveRunID: t.activeRunID,
		Delegation:  cloneDelegationMetadata(t.delegation),
		LatestRun:   latestRun,
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

func effectiveAgentForRun(
	agent *agentcore.Agent,
	workspace WorkspaceConfig,
	permissionPolicy tool.PermissionPolicy,
	approvalPolicy agentcore.ApprovalPolicy,
	profile *agentcore.RunProfile,
) (*agentcore.Agent, error) {
	configured := agentForRun(agent, "", workspace, permissionPolicy, approvalPolicy)
	if configured == nil || profile == nil {
		return configured, nil
	}
	return configured.WithRunProfile(profile)
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

func (s sessionEventSink) RecordSecurityAuditEvent(event agentcore.SecurityAuditEvent) {
	if s.session == nil {
		return
	}
	snapshot := s.session.Snapshot()
	event.ThreadID = firstNonEmpty(event.ThreadID, snapshot.ThreadID)
	event.RunID = firstNonEmpty(event.RunID, snapshot.RunID)
	s.session.RecordSecurityAuditEvent(event)
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
