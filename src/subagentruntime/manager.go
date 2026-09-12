package subagentruntime

import (
	"awaken/src/runcontrol"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultCancelGrace  = 2 * time.Second
	maxDescriptionRunes = 80
	maxPromptBytes      = 32 * 1024
	maxResultBytes      = 32 * 1024
)

type Config struct {
	ForgetAgent            func(string)
	ScopeDigest            string
	RootContext            context.Context
	RunID                  string
	ThreadID               string
	Retain                 func(TaskSnapshot) func()
	PrepareSpawn           func(context.Context, SpawnRequest) (SpawnPreparation, error)
	ListModels             func(context.Context) ([]ModelOption, error)
	BindRunner             bool
	Policy                 Policy
	ModelID                string
	ReasoningEffort        string
	ModelBindingSource     string
	ConfigSnapshotRevision string
	// ModelProfileID is resolved by the Host for this run. When set it is the
	// only profile identity recorded for spawned tasks; model-visible input
	// cannot override it.
	ModelProfileID string
	// ParentWorkspaceRoot anchors host-owned worktree preparation.
	ParentWorkspaceRoot string
	Starter             Starter
	Persist             Persister
	CancelGrace         time.Duration
}

type managedTask struct {
	stopContext        context.Context
	role               Role
	done               chan struct{}
	releaseResources   func()
	contextSeed        json.RawMessage
	snapshot           TaskSnapshot
	starter            Starter
	prompt             string
	cancel             context.CancelFunc
	mutationMu         sync.Mutex
	seenToolCalls      map[string]struct{}
	seenModelRequests  map[string]struct{}
	messages           []QueuedMessage
	host               *HostSpawnOptions
	terminalPersistErr error
	cancelReason       string
}

type messageReceiptRecord struct {
	taskID  string
	content string
	receipt MessageReceipt
	done    chan struct{}
	err     error
}

type Manager struct {
	inboxMu sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	cfg     Config

	mu                           sync.RWMutex
	tasks                        map[string]*managedTask
	agentExecutions              map[string]chan struct{}
	closedAgents                 map[string]bool
	owners                       map[string]spawnOwner
	order                        []string
	delivered                    map[string]bool
	pendingTerminalNotifications []string
	pendingParentNotices         []ParentNotice
	messageReceipts              map[string]*messageReceiptRecord
	closed                       bool
	stopped                      bool
	reserved                     int
	slots                        chan struct{}
	notify                       chan struct{}
	wg                           sync.WaitGroup
}

func NewManager(parent context.Context, cfg Config) *Manager {
	if parent == nil {
		parent = context.Background()
	}
	cfg.Policy = cfg.Policy.Effective()
	if cfg.CancelGrace <= 0 {
		cfg.CancelGrace = defaultCancelGrace
	}
	ctx, cancel := runcontrol.WithCancel(parent)
	rootCtx := cfg.RootContext
	if rootCtx == nil {
		rootCtx = ctx
	}
	return &Manager{
		ctx: ctx, cancel: cancel, cfg: cfg,
		owners:          map[string]spawnOwner{"root": {ctx: rootCtx, config: cfg}},
		tasks:           make(map[string]*managedTask),
		agentExecutions: make(map[string]chan struct{}), closedAgents: make(map[string]bool),
		delivered:       make(map[string]bool),
		messageReceipts: make(map[string]*messageReceiptRecord),
		slots:           make(chan struct{}, cfg.Policy.MaxActiveAgents),
		notify:          make(chan struct{}),
	}
}

func (m *Manager) Spawn(ctx context.Context, request SpawnRequest) (TaskSnapshot, error) {
	if m == nil || m.cfg.Starter == nil {
		return TaskSnapshot{}, runtimeError(ErrorRuntimeUnavailable, "manager starter is unavailable")
	}
	// Historical requests may contain background, but execution always belongs to the parent.
	request.Background = false
	if err := ctx.Err(); err != nil {
		return TaskSnapshot{}, err
	}
	request.Description = strings.TrimSpace(request.Description)
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.AgentType = NormalizeAgentType(request.AgentType)
	request.WorkspaceMode = NormalizeWorkspaceMode(request.AgentType, request.WorkspaceMode)
	if request.Description == "" || request.Prompt == "" {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "description and prompt are required")
	}
	if utf8.RuneCountInString(request.Description) > maxDescriptionRunes || len(request.Prompt) > maxPromptBytes {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "description or prompt exceeds the supported limit")
	}
	if err := ValidateSpawnCombo(request.AgentType, request.WorkspaceMode); err != nil {
		return TaskSnapshot{}, err
	}
	role, _ := LookupRole(request.AgentType)
	role.AllowedTools = append([]string(nil), role.AllowedTools...)
	role.DisallowedTools = append([]string(nil), role.DisallowedTools...)
	request.RoleSnapshot = &role
	roleBytes, _ := json.Marshal(role)
	roleHash := sha256.Sum256(roleBytes)
	parentID := firstNonBlank(request.ParentAgentID, "root")
	m.mu.RLock()
	owner, known := m.owners[parentID]
	m.mu.RUnlock()
	if !known || owner.config.Starter == nil || owner.finished || owner.ctx.Err() != nil {
		return TaskSnapshot{}, runtimeError(ErrorBoundaryViolation, "creator is not an active Agent in this task tree")
	}
	if owner.config.RunID != "" {
		request.RunID = owner.config.RunID
	}
	if owner.config.ThreadID != "" {
		request.ThreadID = owner.config.ThreadID
	}
	if owner.depth >= m.cfg.Policy.MaxDepth {
		return TaskSnapshot{}, runtimeError(ErrorLimitReached, "agent tree depth limit reached")
	}
	preparation := SpawnPreparation{Starter: owner.config.Starter, Model: ModelBinding{
		ProfileID: owner.config.ModelProfileID, ModelID: owner.config.ModelID,
		ReasoningEffort: owner.config.ReasoningEffort, Source: owner.config.ModelBindingSource, Revision: owner.config.ConfigSnapshotRevision,
	}}
	if request.ContextMode == "" {
		request.ContextMode = "fresh"
	}
	if request.ContextMode != "fresh" && request.ContextMode != "full" && request.ContextMode != "recent" {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "context_mode must be fresh, full, or recent")
	}
	if request.ContextMode == "recent" && (request.RecentTurns < 1 || request.RecentTurns > 100) {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "recent context requires recent_turns between 1 and 100")
	}
	if request.Host != nil && request.ContextMode != "fresh" {
		return TaskSnapshot{}, runtimeError(ErrorBoundaryViolation, "this Host workflow requires fresh context")
	}
	if owner.config.PrepareSpawn != nil {
		var err error
		preparation, err = owner.config.PrepareSpawn(ctx, request)
		if err != nil {
			return TaskSnapshot{}, err
		}
		request.ModelProfileID = preparation.Model.ProfileID
	} else if modelProfileID := strings.TrimSpace(owner.config.ModelProfileID); modelProfileID != "" {
		request.ModelProfileID = modelProfileID
	}

	m.mu.Lock()
	if current := m.owners[parentID]; m.closed || m.stopped || m.ctx.Err() != nil || m.closedAgents[request.reuseAgentID] || current.finished || current.ctx.Err() != nil || current.config.RunID != owner.config.RunID || ctx.Err() != nil {
		m.mu.Unlock()
		return TaskSnapshot{}, runtimeError(ErrorRuntimeUnavailable, "manager is closed")
	}
	queued := m.reserved
	for _, task := range m.tasks {
		if task.snapshot.Status == StatusQueued {
			queued++
		}
	}
	if queued >= m.cfg.Policy.MaxQueuedAssignments {
		m.mu.Unlock()
		return TaskSnapshot{}, runtimeError("subagent_concurrency_queue_full", "assignment queue is full; wait for active work before retrying")
	}
	m.reserved++
	now := time.Now().UTC()
	taskID := newID("subtask")
	m.wg.Add(1)
	snapshot := TaskSnapshot{
		ScopeDigest: owner.config.ScopeDigest,
		RoleDigest:  hex.EncodeToString(roleHash[:]),
		Background:  request.Background, ContinuesFrom: request.ContinuesFrom,
		ContextMode: request.ContextMode, RecentTurns: request.RecentTurns,
		TaskID: taskID, AgentID: firstNonBlank(request.reuseAgentID, newID("subagent")), AttemptID: newID("attempt"),
		ParentAgentID:    firstNonBlank(request.ParentAgentID, "root"),
		ParentToolCallID: request.ParentToolCallID,
		ThreadID:         request.ThreadID, RunID: request.RunID, TurnID: request.TurnID,
		Description: request.Description, AgentType: request.AgentType,
		WorkspaceMode: request.WorkspaceMode, ModelProfileID: request.ModelProfileID,
		ModelID: preparation.Model.ModelID, ReasoningEffort: preparation.Model.ReasoningEffort,
		ModelBindingSource: preparation.Model.Source, ConfigSnapshotRevision: preparation.Model.Revision,
		Status: StatusQueued, SideEffectState: "none", CreatedAt: now,
	}
	if request.Host != nil {
		snapshot.Workflow = "host_restricted"
		if request.Host.Skill != nil {
			snapshot.Workflow = "skill"
		}
	}
	m.mu.Unlock()

	if err := m.persistEvent(ctx, PersistEvent{Kind: "subagent_spawned", Snapshot: snapshot, Prompt: request.Prompt, ContextSeed: preparation.ContextSeed}); err != nil {
		m.mu.Lock()
		m.reserved--
		m.mu.Unlock()
		m.wg.Done()
		return TaskSnapshot{}, err
	}

	taskCtx, cancel := runcontrol.WithCancel(owner.ctx)
	task := &managedTask{
		stopContext: taskCtx,
		role:        role,
		done:        make(chan struct{}),
		snapshot:    snapshot, prompt: request.Prompt, cancel: cancel, starter: preparation.Starter, contextSeed: preparation.ContextSeed,
		seenToolCalls: make(map[string]struct{}), seenModelRequests: make(map[string]struct{}),
		host: request.Host,
	}
	if m.cfg.Retain != nil {
		task.releaseResources = m.cfg.Retain(snapshot)
	}
	if owner.config.Retain != nil {
		ownerRelease := owner.config.Retain(snapshot)
		sharedRelease := task.releaseResources
		task.releaseResources = func() {
			if ownerRelease != nil {
				ownerRelease()
			}
			if sharedRelease != nil {
				sharedRelease()
			}
		}
	}
	m.mu.Lock()
	if current := m.owners[parentID]; m.stopped || m.closed || current.finished || current.ctx.Err() != nil || current.config.RunID != owner.config.RunID {
		cancel()
	}
	m.reserved--
	m.tasks[taskID] = task
	if m.agentExecutions[snapshot.AgentID] == nil {
		m.agentExecutions[snapshot.AgentID] = make(chan struct{}, 1)
	}
	m.order = append(m.order, taskID)
	m.mu.Unlock()
	m.signal()

	go m.runTask(taskCtx, taskID)
	return snapshot, nil
}

// SpawnFromHost is the host-owned spawn entry used by Skill fork and other
// host workflows. Deep Research investigators use the ordinary subagent tool
// instead. It writes the same subagent_spawned record as Spawn and does not
// change the model schema.
func (m *Manager) SpawnFromHost(ctx context.Context, request SpawnRequest) (TaskSnapshot, error) {
	request.ParentAgentID = firstNonBlank(request.ParentAgentID, "root")
	return m.Spawn(ctx, request)
}

func (m *Manager) runTask(ctx context.Context, taskID string) {
	defer m.wg.Done()
	defer func() {
		if task := m.task(taskID); task != nil {
			close(task.done)
			m.signal()
		}
	}()
	// Publish terminal only after execution and resource cleanup have returned.
	status, result, code, message := StatusCancelled, "", ErrorCancelled, "subagent was cancelled"
	finish := func(next TaskStatus, text, errorCode, errorMessage string) {
		status, result, code, message = next, text, errorCode, errorMessage
	}
	defer func() { m.finish(taskID, status, result, code, message) }()
	defer func() {
		if task := m.task(taskID); task != nil {
			task.cancel()
			if task.releaseResources != nil {
				task.releaseResources()
			}
		}
		if task := m.task(taskID); task != nil {
			task.mutationMu.Lock()
			task.starter, task.contextSeed, task.prompt, task.releaseResources = nil, nil, "", nil
			task.mutationMu.Unlock()
		}
	}()
	m.mu.RLock()
	agentID := m.tasks[taskID].snapshot.AgentID
	serial := m.agentExecutions[agentID]
	m.mu.RUnlock()
	select {
	case serial <- struct{}{}:
		defer func() { <-serial }()
	case <-ctx.Done():
		finish(StatusCancelled, "", ErrorCancelled, "subagent was cancelled")
		return
	}
	var lease *executionLease
	var err error
	ctx, lease, err = acquireExecution(ctx, m.slots)
	if err != nil {
		finish(StatusCancelled, "", ErrorCancelled, "subagent was cancelled")
		return
	}
	defer lease.release()

	started := time.Now().UTC()
	task := m.task(taskID)
	if task == nil {
		return
	}
	task.mutationMu.Lock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	if ctx.Err() != nil || snapshot.Status == StatusCancelling || !m.canStartAssignment(snapshot) {
		task.mutationMu.Unlock()
		finish(StatusCancelled, "", ErrorCancelled, "subagent was cancelled")
		return
	}
	if snapshot.Terminal() {
		task.mutationMu.Unlock()
		return
	}
	snapshot.Status = StatusRunning
	snapshot.StartedAt = &started
	snapshot.CurrentActivity = "正在执行分配的任务"
	if err := m.persist(ctx, "subagent_status_changed", snapshot); err != nil {
		task.mutationMu.Unlock()
		finish(StatusFailed, "", ErrorPersistence, err.Error())
		return
	}
	if !m.replaceSnapshot(taskID, snapshot) {
		task.mutationMu.Unlock()
		return
	}
	task.mutationMu.Unlock()

	// Stop may arrive while the running transition is being persisted. A
	// released slot is not permission to start a queued assignment after the
	// owning attempt has been fenced, even before cancellation reaches it.
	if ctx.Err() != nil || !m.canStartAssignment(snapshot) {
		finish(StatusCancelled, "", ErrorCancelled, "subagent was cancelled")
		return
	}

	startCtx := ctx
	cancel := func() {}
	if task.role.Timeout > 0 {
		startCtx, cancel = context.WithTimeout(ctx, task.role.Timeout)
	}
	defer cancel()
	result, err = task.starter(startCtx, StartRequest{
		RoleSnapshot:        &task.role,
		IncorporateMessages: func(ids []string) { m.incorporateMessages(agentID, ids) },
		ContextSeed:         append(json.RawMessage(nil), task.contextSeed...),
		Task:                snapshot, Prompt: m.prompt(taskID), Host: m.hostOptions(taskID),
		ReportProgress: func(progressCtx context.Context, update ProgressUpdate) error {
			return m.reportProgress(progressCtx, taskID, update)
		},
		DrainMessages: func() []QueuedMessage {
			return m.takeMessages(taskID)
		},
		AttachWorktree: func(receipt *WorktreeReceipt) {
			m.attachWorktree(taskID, receipt)
		},
	})
	if startCtx.Err() != nil {
		if errors.Is(startCtx.Err(), context.DeadlineExceeded) {
			finish(StatusFailed, "", "subagent_timeout", "subagent exceeded its role timeout")
			return
		}
		finish(StatusCancelled, "", ErrorCancelled, "subagent was cancelled")
		return
	}
	if err != nil {
		code := ErrorCode(err)
		if code == "" {
			code = "subagent_failed"
		}
		finish(StatusFailed, "", code, err.Error())
		return
	}
	finish(StatusCompleted, result, "", "")
}

func (m *Manager) canStartAssignment(task TaskSnapshot) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	owner, known := m.owners[task.ParentAgentID]
	return !m.closed && !m.stopped && m.ctx.Err() == nil &&
		!m.closedAgents[task.AgentID] && known && !owner.finished &&
		owner.ctx != nil && owner.ctx.Err() == nil &&
		(owner.config.RunID == "" || owner.config.RunID == task.RunID)
}

func (m *Manager) finish(taskID string, status TaskStatus, result, code, message string) {
	task := m.task(taskID)
	if task == nil {
		return
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	if snapshot.Terminal() {
		return
	}
	if snapshot.Status == StatusCancelling {
		status, result, code, message = StatusCancelled, "", ErrorCancelled, cancelMessageForReason(task.cancelReason)
	}
	if status == StatusCancelled {
		snapshot.StopSource = string(runcontrol.FromContext(task.stopContext))
		message = runcontrol.Message(runcontrol.Source(snapshot.StopSource))
	}
	finished := time.Now().UTC()
	snapshot.Status = status
	snapshot.CurrentActivity = ""
	snapshot.ResultPreview = bounded(result, maxResultBytes)
	snapshot.ResultContent = result
	digest := sha256.Sum256([]byte(result))
	snapshot.ResultDigest = hex.EncodeToString(digest[:])
	snapshot.ErrorCode = code
	snapshot.ErrorMessage = message
	snapshot.FinishedAt = &finished
	persistCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := m.persist(persistCtx, "subagent_terminal", snapshot)
	cancel()
	if err != nil {
		task.terminalPersistErr = err
		m.signal()
		return
	}
	task.terminalPersistErr = nil
	m.replaceSnapshot(taskID, snapshot)
}

func (m *Manager) reportProgress(ctx context.Context, taskID string, update ProgressUpdate) error {
	task := m.task(taskID)
	if task == nil {
		return runtimeError(ErrorNotFound, "task does not exist in this run")
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	if snapshot.Terminal() || snapshot.Status == StatusCancelling {
		return nil
	}
	changed := false
	if toolCallID := strings.TrimSpace(update.ToolCallID); toolCallID != "" {
		if _, seen := task.seenToolCalls[toolCallID]; !seen {
			snapshot.ToolUseCount++
			changed = true
			if toolName := strings.TrimSpace(update.ToolName); toolName != "" {
				snapshot.CurrentActivity = bounded("正在使用 "+toolName, 160)
			}
		}
	}
	if receipt := update.HostToolReceipt; receipt != nil && strings.TrimSpace(receipt.ToolCallID) != "" {
		found := false
		for _, existing := range snapshot.HostToolReceipts {
			if existing.ToolCallID == receipt.ToolCallID {
				found = true
				break
			}
		}
		if !found {
			copied := *receipt
			copied.AgentID, copied.TaskID, copied.AttemptID = snapshot.AgentID, snapshot.TaskID, snapshot.AttemptID
			copied.Details = cloneAnyMap(receipt.Details)
			snapshot.HostToolReceipts = append(snapshot.HostToolReceipts, copied)
			changed = true
		}
	}
	if requestID := strings.TrimSpace(update.ModelRequestID); requestID != "" {
		if _, seen := task.seenModelRequests[requestID]; !seen {
			if update.HasInputTokens && update.InputTokens > 0 {
				snapshot.InputTokens += update.InputTokens
			}
			if update.HasOutputTokens && update.OutputTokens > 0 {
				snapshot.OutputTokens += update.OutputTokens
			}
			total := 0
			if update.HasTotalTokens && update.TotalTokens > 0 {
				total = update.TotalTokens
			} else {
				total = max(update.InputTokens, 0) + max(update.OutputTokens, 0)
			}
			if total > 0 {
				snapshot.TotalTokens += total
				snapshot.EstimatedTokens = snapshot.TotalTokens
			}
			changed = true
			if strings.TrimSpace(update.Activity) == "" {
				snapshot.CurrentActivity = "正在整理子任务结果"
			}
		}
	}
	if activity := strings.TrimSpace(update.Activity); activity != "" {
		snapshot.CurrentActivity = bounded(activity, 160)
		changed = true
	}
	if !changed {
		return nil
	}
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	persistCtx, cancel := context.WithTimeout(base, 2*time.Second)
	err := m.persist(persistCtx, "subagent_progress", snapshot)
	cancel()
	if err != nil {
		return err
	}
	if id := strings.TrimSpace(update.ToolCallID); id != "" {
		task.seenToolCalls[id] = struct{}{}
	}
	if id := strings.TrimSpace(update.ModelRequestID); id != "" {
		task.seenModelRequests[id] = struct{}{}
	}
	m.replaceSnapshot(taskID, snapshot)
	return nil
}

func (m *Manager) Status(taskID string) (TaskSnapshot, error) {
	if snapshot, ok := m.snapshot(strings.TrimSpace(taskID)); ok {
		return snapshot, nil
	}
	return TaskSnapshot{}, runtimeError(ErrorNotFound, "task does not exist in this run")
}

func (m *Manager) List() []TaskSnapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TaskSnapshot, 0, len(m.order))
	for _, id := range m.order {
		if task := m.tasks[id]; task != nil {
			snapshot := cloneTaskSnapshot(task.snapshot)
			snapshot.AgentClosed = snapshot.AgentClosed || m.closedAgents[snapshot.AgentID]
			result = append(result, snapshot)
		}
	}
	return result
}

func (m *Manager) Wait(ctx context.Context, request WaitRequest) (result WaitResult, waitErr error) {
	if request.EventDriven || request.Timeout > 0 {
		resume := SuspendExecution(ctx)
		defer func() {
			if err := resume(); err != nil && waitErr == nil {
				waitErr = err
			}
		}()
	}
	if m == nil {
		return WaitResult{}, runtimeError(ErrorRuntimeUnavailable, "manager is unavailable")
	}
	if request.ReturnWhen == "" {
		request.ReturnWhen = "all"
	}
	if request.ReturnWhen != "any" && request.ReturnWhen != "all" {
		return WaitResult{}, runtimeError(ErrorInvalidInput, "return_when must be any or all")
	}
	if len(request.TaskIDs) == 0 {
		return WaitResult{}, runtimeError(ErrorInvalidInput, "task_ids is required")
	}
	if request.Timeout < 0 || request.Timeout > 60*time.Second {
		return WaitResult{}, runtimeError(ErrorInvalidInput, "timeout must be between 0 and 60 seconds")
	}
	waitCtx := ctx
	cancel := func() {}
	if request.Timeout > 0 && !request.EventDriven {
		waitCtx, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()

	for {
		m.mu.RLock()
		notify := m.notify
		m.mu.RUnlock()
		snapshots, ready, err := m.waitState(request, !request.DeferAcknowledgement)
		if err != nil {
			return WaitResult{}, err
		}
		if ready {
			return WaitResult{Tasks: snapshots, RetrievalStatus: RetrievalCompleted}, nil
		}
		if request.Timeout == 0 && !request.EventDriven {
			return WaitResult{Tasks: snapshots, RetrievalStatus: RetrievalRunning}, nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				snapshots, _, _ = m.waitState(request, false)
				return WaitResult{Tasks: snapshots, RetrievalStatus: RetrievalTimeout}, nil
			}
			return WaitResult{}, waitCtx.Err()
		case <-notify:
		}
	}
}

func (m *Manager) waitState(request WaitRequest, consume bool) ([]TaskSnapshot, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	all := make([]TaskSnapshot, 0, len(request.TaskIDs))
	newlyCompleted := make([]TaskSnapshot, 0, len(request.TaskIDs))
	terminal := 0
	for _, id := range request.TaskIDs {
		task := m.tasks[strings.TrimSpace(id)]
		if task == nil {
			return nil, false, runtimeError(ErrorNotFound, "task does not exist in this run")
		}
		snapshot := cloneTaskSnapshot(task.snapshot)
		all = append(all, snapshot)
		if snapshot.Terminal() {
			terminal++
			if !m.delivered[snapshot.TaskID] {
				newlyCompleted = append(newlyCompleted, snapshot)
			}
		}
	}
	if request.ReturnWhen == "any" {
		if len(newlyCompleted) == 0 {
			// There can be no later completion once every requested task is terminal.
			return all, terminal == len(all), nil
		}
		if consume {
			for _, snapshot := range newlyCompleted {
				m.delivered[snapshot.TaskID] = true
			}
		}
		return newlyCompleted, true, nil
	}
	if consume && terminal == len(all) {
		for _, snapshot := range all {
			m.delivered[snapshot.TaskID] = true
		}
	}
	return all, terminal == len(all), nil
}

// DrainTerminalNotifications returns each durable terminal snapshot at most
// once. Explicit wait/status consumption and automatic root delivery share the
// same delivered set, so the parent model never receives a duplicate handoff.
func (m *Manager) DrainTerminalNotifications() []TaskSnapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pendingTerminalNotifications) == 0 {
		return nil
	}
	pending := m.pendingTerminalNotifications
	m.pendingTerminalNotifications = nil
	result := make([]TaskSnapshot, 0, len(pending))
	for _, taskID := range pending {
		task := m.tasks[taskID]
		if task == nil || !task.snapshot.Terminal() || m.delivered[taskID] {
			continue
		}
		m.delivered[taskID] = true
		result = append(result, cloneTaskSnapshot(task.snapshot))
	}
	return result
}

// AcknowledgeTerminal records that the root model explicitly retrieved a
// terminal snapshot through the status tool. It intentionally does not affect
// non-terminal status reads.
func (m *Manager) AcknowledgeTerminal(taskID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	taskID = strings.TrimSpace(taskID)
	if task := m.tasks[taskID]; task != nil && task.snapshot.Terminal() {
		m.delivered[taskID] = true
	}
}

func (m *Manager) Message(ctx context.Context, taskID, content string) (MessageReceipt, error) {
	return m.MessageIdempotent(ctx, taskID, content, "")
}

func (m *Manager) MessageIdempotent(ctx context.Context, taskID, content, idempotencyKey string) (MessageReceipt, error) {
	if m == nil {
		return MessageReceipt{}, runtimeError(ErrorRuntimeUnavailable, "manager is unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if taskID == "" || content == "" {
		return MessageReceipt{}, runtimeError(ErrorInvalidInput, "task_id and content are required")
	}
	if len(content) > maxPromptBytes {
		return MessageReceipt{}, runtimeError(ErrorInvalidInput, "message exceeds the supported limit")
	}
	if idempotencyKey == "" {
		return m.queueMessage(ctx, taskID, content)
	}
	m.mu.Lock()
	if recorded, ok := m.messageReceipts[idempotencyKey]; ok {
		m.mu.Unlock()
		if recorded.taskID != taskID || recorded.content != content {
			return MessageReceipt{}, runtimeError(ErrorIdempotency, "idempotency key was reused with a different task or body")
		}
		select {
		case <-recorded.done:
			return recorded.receipt, recorded.err
		case <-ctx.Done():
			return MessageReceipt{}, ctx.Err()
		}
	}
	recorded := &messageReceiptRecord{taskID: taskID, content: content, done: make(chan struct{})}
	m.messageReceipts[idempotencyKey] = recorded
	m.mu.Unlock()
	receipt, err := m.queueMessage(ctx, taskID, content)
	m.mu.Lock()
	recorded.receipt, recorded.err = receipt, err
	if err != nil {
		delete(m.messageReceipts, idempotencyKey)
	}
	close(recorded.done)
	m.mu.Unlock()
	return receipt, err
}

func (m *Manager) queueMessage(ctx context.Context, taskID, content string) (MessageReceipt, error) {
	m.inboxMu.Lock()
	defer m.inboxMu.Unlock()
	task := m.task(taskID)
	if task == nil {
		return MessageReceipt{}, runtimeError(ErrorNotFound, "task does not exist in this run")
	}
	identity, _ := m.snapshot(taskID)
	pending := 0
	for _, assignment := range m.List() {
		if assignment.AgentID != identity.AgentID {
			continue
		}
		entry := m.task(assignment.TaskID)
		entry.mutationMu.Lock()
		pending += len(entry.messages)
		entry.mutationMu.Unlock()
	}
	task.mutationMu.Lock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	m.mu.RLock()
	closed := m.closedAgents[snapshot.AgentID] || m.closed
	m.mu.RUnlock()
	if closed || snapshot.Status == StatusCancelling || snapshot.AgentClosed {
		task.mutationMu.Unlock()
		return MessageReceipt{}, runtimeError(ErrorNotRunning, "subagent is not running")
	}
	if pending >= 128 {
		task.mutationMu.Unlock()
		return MessageReceipt{}, runtimeError(ErrorLimitReached, "agent inbox is full")
	}
	queued := QueuedMessage{ID: newID("submsg"), Content: content, QueuedAt: time.Now().UTC()}
	if err := m.persistMessage(ctx, snapshot, queued); err != nil {
		task.mutationMu.Unlock()
		return MessageReceipt{}, err
	}
	task.messages = append(task.messages, queued)
	task.mutationMu.Unlock()
	receipt := MessageReceipt{TaskID: taskID, QueuedAt: queued.QueuedAt, MessageID: queued.ID}
	m.signal()
	return receipt, nil
}

func (m *Manager) persistMessage(ctx context.Context, snapshot TaskSnapshot, message QueuedMessage) error {
	if m.cfg.Persist == nil {
		return nil
	}
	if err := m.cfg.Persist(ctx, PersistEvent{Kind: "subagent_message_queued", Snapshot: snapshot, Message: &message}); err != nil {
		return runtimeError(ErrorPersistence, err.Error())
	}
	return nil
}

func (m *Manager) takeMessages(taskID string) []QueuedMessage {
	current, ok := m.snapshot(taskID)
	if !ok {
		return nil
	}
	var messages []QueuedMessage
	for _, snapshot := range m.List() {
		if snapshot.AgentID != current.AgentID {
			continue
		}
		entry := m.task(snapshot.TaskID)
		entry.mutationMu.Lock()
		messages = append(messages, entry.messages...)
		entry.mutationMu.Unlock()
	}
	return messages
}

func (m *Manager) incorporateMessages(agentID string, ids []string) {
	consumed := map[string]bool{}
	for _, id := range ids {
		consumed[id] = true
	}
	for _, snapshot := range m.List() {
		if snapshot.AgentID != agentID {
			continue
		}
		task := m.task(snapshot.TaskID)
		task.mutationMu.Lock()
		pending := task.messages[:0]
		for _, message := range task.messages {
			if !consumed[message.ID] {
				pending = append(pending, message)
			}
		}
		task.messages = pending
		task.mutationMu.Unlock()
	}
}

func (m *Manager) attachWorktree(taskID string, receipt *WorktreeReceipt) {
	task := m.task(taskID)
	if task == nil || receipt == nil {
		return
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	snapshot.Worktree = receipt
	m.replaceSnapshot(taskID, snapshot)
}

func (m *Manager) Cancel(ctx context.Context, taskID, reason string) (TaskSnapshot, error) {
	taskID = strings.TrimSpace(taskID)
	task := m.task(taskID)
	if task == nil {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "task does not exist in this run")
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	if snapshot.Terminal() {
		return snapshot, nil
	}
	if task.cancelReason == "" {
		task.cancelReason = strings.TrimSpace(reason)
	}
	runcontrol.SetSource(task.stopContext, runcontrol.RequestSource(ctx, runcontrol.Parent))
	snapshot.StopSource = string(runcontrol.FromContext(task.stopContext))
	snapshot.Status = StatusCancelling
	snapshot.CurrentActivity = "正在取消"
	if err := m.persist(ctx, "subagent_status_changed", snapshot); err != nil {
		return TaskSnapshot{}, err
	}
	m.replaceSnapshot(taskID, snapshot)
	task.cancel()
	go m.noteSlowCancellation(taskID)
	return snapshot, nil
}

func (m *Manager) noteSlowCancellation(taskID string) {
	task := m.task(taskID)
	if task == nil {
		return
	}
	timer := time.NewTimer(m.cfg.CancelGrace)
	defer timer.Stop()
	select {
	case <-timer.C:
		m.markStillStopping(taskID, "正在停止，等待执行与资源清理退出")
	case <-task.done:
	}
}

func (m *Manager) markStillStopping(taskID, message string) {
	task := m.task(taskID)
	if task == nil {
		return
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := cloneTaskSnapshot(task.snapshot)
	if snapshot.Terminal() {
		return
	}
	snapshot.Status, snapshot.CurrentActivity = StatusCancelling, message
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.persist(ctx, "subagent_status_changed", snapshot); err == nil {
		m.replaceSnapshot(taskID, snapshot)
	}
}

func (m *Manager) task(taskID string) *managedTask {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

func (m *Manager) Close(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.closed = true
	runcontrol.SetSource(m.ctx, runcontrol.RequestSource(ctx, runcontrol.Parent))
	m.cancel()
	m.mu.Unlock()
	stopErr := m.StopAll(ctx)
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return errors.Join(stopErr, m.terminalPersistenceError())
	case <-ctx.Done():
		m.markRemainingStopping("subagent manager close timed out")
		if err := m.terminalPersistenceError(); err != nil {
			return errors.Join(ctx.Err(), err)
		}
		return ctx.Err()
	}
}

func (m *Manager) markRemainingStopping(message string) {
	for _, snapshot := range m.List() {
		if snapshot.Terminal() {
			continue
		}
		m.markStillStopping(snapshot.TaskID, message)
	}
}

func (m *Manager) terminalPersistenceError() error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	tasks := make([]*managedTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	m.mu.RUnlock()
	for _, task := range tasks {
		task.mutationMu.Lock()
		err := task.terminalPersistErr
		task.mutationMu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) persist(ctx context.Context, kind string, snapshot TaskSnapshot) error {
	return m.persistEvent(ctx, PersistEvent{Kind: kind, Snapshot: snapshot})
}

func (m *Manager) persistEvent(ctx context.Context, event PersistEvent) error {
	if m.cfg.Persist == nil {
		return nil
	}
	if err := m.cfg.Persist(ctx, event); err != nil {
		return runtimeError(ErrorPersistence, err.Error())
	}
	return nil
}

func (m *Manager) snapshot(taskID string) (TaskSnapshot, bool) {
	if m == nil {
		return TaskSnapshot{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	task := m.tasks[taskID]
	if task == nil {
		return TaskSnapshot{}, false
	}
	snapshot := cloneTaskSnapshot(task.snapshot)
	snapshot.AgentClosed = snapshot.AgentClosed || m.closedAgents[snapshot.AgentID]
	return snapshot, true
}

func (m *Manager) replaceSnapshot(taskID string, snapshot TaskSnapshot) bool {
	m.mu.Lock()
	task := m.tasks[taskID]
	if task == nil {
		m.mu.Unlock()
		return false
	}
	task.snapshot = cloneTaskSnapshot(snapshot)
	if snapshot.Terminal() && !m.delivered[taskID] {
		m.pendingTerminalNotifications = append(m.pendingTerminalNotifications, taskID)
	}
	m.mu.Unlock()
	m.signal()
	return true
}

func cloneTaskSnapshot(snapshot TaskSnapshot) TaskSnapshot {
	if snapshot.Worktree != nil {
		receipt := *snapshot.Worktree
		receipt.Changed = append([]string(nil), receipt.Changed...)
		snapshot.Worktree = &receipt
	}
	if snapshot.StartedAt != nil {
		value := *snapshot.StartedAt
		snapshot.StartedAt = &value
	}
	if snapshot.FinishedAt != nil {
		value := *snapshot.FinishedAt
		snapshot.FinishedAt = &value
	}

	snapshot.HostToolReceipts = append([]HostToolReceipt(nil), snapshot.HostToolReceipts...)
	for i := range snapshot.HostToolReceipts {
		snapshot.HostToolReceipts[i].Details = cloneAnyMap(snapshot.HostToolReceipts[i].Details)
	}
	return snapshot
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneDetailValue(value)
	}
	return cloned
}

func cloneDetailValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneAnyMap(value)
	case []any:
		copied := make([]any, len(value))
		for i, item := range value {
			copied[i] = cloneDetailValue(item)
		}
		return copied
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}

func (m *Manager) prompt(taskID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if task := m.tasks[taskID]; task != nil {
		return task.prompt
	}
	return ""
}

func (m *Manager) hostOptions(taskID string) *HostSpawnOptions {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if task := m.tasks[taskID]; task != nil {
		return task.host
	}
	return nil
}

func (m *Manager) signal() {
	m.mu.Lock()
	close(m.notify)
	m.notify = make(chan struct{})
	m.mu.Unlock()
}

func runtimeError(code, message string) error {
	return &Error{Code: code, Message: message}
}

func newID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + "_" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func bounded(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	const suffix = "\n\n[truncated]"
	if limit <= len(suffix) {
		return suffix[:max(0, limit)]
	}
	end := limit - len(suffix)
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + suffix
}

func (m *Manager) EventDrivenWaits() bool { return m != nil }
