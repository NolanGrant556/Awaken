package subagentruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultMaxTasks      = 8
	defaultMaxConcurrent = 4
	defaultCancelGrace   = 2 * time.Second
	maxDescriptionRunes  = 80
	maxPromptBytes       = 32 * 1024
	maxResultBytes       = 32 * 1024
)

type Config struct {
	MaxTasks      int
	MaxConcurrent int
	Starter       Starter
	Persist       Persister
	CancelGrace   time.Duration
}

type managedTask struct {
	snapshot           TaskSnapshot
	prompt             string
	cancel             context.CancelFunc
	mutationMu         sync.Mutex
	seenToolCalls      map[string]struct{}
	seenModelRequests  map[string]struct{}
	terminalPersistErr error
}

type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	mu        sync.RWMutex
	tasks     map[string]*managedTask
	order     []string
	delivered map[string]bool
	reserved  int
	slots     chan struct{}
	notify    chan struct{}
	wg        sync.WaitGroup
}

func NewManager(parent context.Context, cfg Config) *Manager {
	if parent == nil {
		parent = context.Background()
	}
	if cfg.MaxTasks <= 0 {
		cfg.MaxTasks = defaultMaxTasks
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = defaultMaxConcurrent
	}
	if cfg.CancelGrace <= 0 {
		cfg.CancelGrace = defaultCancelGrace
	}
	ctx, cancel := context.WithCancel(parent)
	return &Manager{
		ctx: ctx, cancel: cancel, cfg: cfg,
		tasks:     make(map[string]*managedTask),
		delivered: make(map[string]bool),
		slots:     make(chan struct{}, cfg.MaxConcurrent),
		notify:    make(chan struct{}, 1),
	}
}

func (m *Manager) Spawn(ctx context.Context, request SpawnRequest) (TaskSnapshot, error) {
	if m == nil || m.cfg.Starter == nil {
		return TaskSnapshot{}, runtimeError(ErrorRuntimeUnavailable, "manager starter is unavailable")
	}
	request.Description = strings.TrimSpace(request.Description)
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.AgentType = strings.ToLower(strings.TrimSpace(request.AgentType))
	request.WorkspaceMode = strings.ToLower(strings.TrimSpace(request.WorkspaceMode))
	if request.AgentType == "" {
		request.AgentType = "explore"
	}
	if request.WorkspaceMode == "" {
		request.WorkspaceMode = "shared_readonly"
	}
	if request.Description == "" || request.Prompt == "" {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "description and prompt are required")
	}
	if utf8.RuneCountInString(request.Description) > maxDescriptionRunes || len(request.Prompt) > maxPromptBytes {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "description or prompt exceeds the supported limit")
	}
	if request.AgentType != "explore" && request.AgentType != "review" {
		return TaskSnapshot{}, runtimeError(ErrorInvalidInput, "phase 1 supports explore and review agent types")
	}
	if request.WorkspaceMode != "shared_readonly" {
		return TaskSnapshot{}, runtimeError(ErrorWorkspaceMode, "phase 1 supports shared_readonly only")
	}
	if strings.TrimSpace(request.ParentAgentID) != "" && request.ParentAgentID != "root" {
		return TaskSnapshot{}, runtimeError(ErrorBoundaryViolation, "only the root agent may spawn a subagent")
	}

	m.mu.Lock()
	if len(m.order)+m.reserved >= m.cfg.MaxTasks {
		m.mu.Unlock()
		return TaskSnapshot{}, runtimeError(ErrorLimitReached, "maximum tasks for this run reached")
	}
	m.reserved++
	now := time.Now().UTC()
	taskID := newID("subtask")
	snapshot := TaskSnapshot{
		TaskID: taskID, AgentID: newID("subagent"),
		ParentAgentID:    firstNonBlank(request.ParentAgentID, "root"),
		ParentToolCallID: request.ParentToolCallID,
		ThreadID:         request.ThreadID, RunID: request.RunID, TurnID: request.TurnID,
		Description: request.Description, AgentType: request.AgentType,
		WorkspaceMode: request.WorkspaceMode, ModelProfileID: request.ModelProfileID,
		Status: StatusQueued, SideEffectState: "none", CreatedAt: now,
	}
	m.mu.Unlock()

	if err := m.persist(ctx, "subagent_spawned", snapshot); err != nil {
		m.mu.Lock()
		m.reserved--
		m.mu.Unlock()
		return TaskSnapshot{}, err
	}

	taskCtx, cancel := context.WithCancel(m.ctx)
	task := &managedTask{
		snapshot: snapshot, prompt: request.Prompt, cancel: cancel,
		seenToolCalls: make(map[string]struct{}), seenModelRequests: make(map[string]struct{}),
	}
	m.mu.Lock()
	m.reserved--
	m.tasks[taskID] = task
	m.order = append(m.order, taskID)
	m.mu.Unlock()
	m.signal()

	m.wg.Add(1)
	go m.runTask(taskCtx, taskID)
	return snapshot, nil
}

func (m *Manager) runTask(ctx context.Context, taskID string) {
	defer m.wg.Done()
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		m.finishCancelled(taskID)
		return
	}

	started := time.Now().UTC()
	task := m.task(taskID)
	if task == nil {
		return
	}
	task.mutationMu.Lock()
	snapshot := task.snapshot
	if ctx.Err() != nil || snapshot.Status == StatusCancelling {
		task.mutationMu.Unlock()
		m.finishCancelled(taskID)
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
		m.finish(taskID, StatusFailed, "", ErrorPersistence, err.Error())
		return
	}
	if !m.replaceSnapshot(taskID, snapshot) {
		task.mutationMu.Unlock()
		return
	}
	task.mutationMu.Unlock()

	result, err := m.cfg.Starter(ctx, StartRequest{
		Task: snapshot, Prompt: m.prompt(taskID),
		ReportProgress: func(progressCtx context.Context, update ProgressUpdate) error {
			return m.reportProgress(progressCtx, taskID, update)
		},
	})
	if ctx.Err() != nil {
		m.finishCancelled(taskID)
		return
	}
	if err != nil {
		code := ErrorCode(err)
		if code == "" {
			code = "subagent_failed"
		}
		m.finish(taskID, StatusFailed, "", code, err.Error())
		return
	}
	m.finish(taskID, StatusCompleted, bounded(result, maxResultBytes), "", "")
}

func (m *Manager) finishCancelled(taskID string) {
	m.finish(taskID, StatusCancelled, "", ErrorCancelled, "subagent was cancelled")
}

func (m *Manager) finish(taskID string, status TaskStatus, result, code, message string) {
	task := m.task(taskID)
	if task == nil {
		return
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := task.snapshot
	if snapshot.Terminal() {
		return
	}
	finished := time.Now().UTC()
	snapshot.Status = status
	snapshot.CurrentActivity = ""
	snapshot.ResultPreview = result
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
	snapshot := task.snapshot
	if snapshot.Terminal() || snapshot.Status == StatusCancelling {
		return nil
	}
	changed := false
	if toolCallID := strings.TrimSpace(update.ToolCallID); toolCallID != "" {
		if _, seen := task.seenToolCalls[toolCallID]; !seen {
			task.seenToolCalls[toolCallID] = struct{}{}
			snapshot.ToolUseCount++
			changed = true
			if toolName := strings.TrimSpace(update.ToolName); toolName != "" {
				snapshot.CurrentActivity = bounded("正在使用 "+toolName, 160)
			}
		}
	}
	if requestID := strings.TrimSpace(update.ModelRequestID); requestID != "" {
		if _, seen := task.seenModelRequests[requestID]; !seen {
			task.seenModelRequests[requestID] = struct{}{}
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
	if err := m.persist(ctx, "subagent_progress", snapshot); err != nil {
		return err
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
			result = append(result, task.snapshot)
		}
	}
	return result
}

func (m *Manager) Wait(ctx context.Context, request WaitRequest) (WaitResult, error) {
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
	if request.Timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()

	for {
		snapshots, ready, err := m.waitState(request, true)
		if err != nil {
			return WaitResult{}, err
		}
		if ready {
			return WaitResult{Tasks: snapshots, RetrievalStatus: RetrievalCompleted}, nil
		}
		if request.Timeout == 0 {
			return WaitResult{Tasks: snapshots, RetrievalStatus: RetrievalRunning}, nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				snapshots, _, _ = m.waitState(request, false)
				return WaitResult{Tasks: snapshots, RetrievalStatus: RetrievalTimeout}, nil
			}
			return WaitResult{}, waitCtx.Err()
		case <-m.notify:
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
		snapshot := task.snapshot
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
			return all, false, nil
		}
		if consume {
			for _, snapshot := range newlyCompleted {
				m.delivered[snapshot.TaskID] = true
			}
		}
		return newlyCompleted, true, nil
	}
	return all, terminal == len(all), nil
}

func (m *Manager) Message(context.Context, string, string) (MessageReceipt, error) {
	return MessageReceipt{}, runtimeError(ErrorNotRunning, "subagent messages are not available in phase 1")
}

func (m *Manager) Cancel(ctx context.Context, taskID, _ string) (TaskSnapshot, error) {
	taskID = strings.TrimSpace(taskID)
	task := m.task(taskID)
	if task == nil {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "task does not exist in this run")
	}
	task.mutationMu.Lock()
	defer task.mutationMu.Unlock()
	snapshot := task.snapshot
	if snapshot.Terminal() {
		return snapshot, nil
	}
	snapshot.Status = StatusCancelling
	snapshot.CurrentActivity = "正在取消"
	if err := m.persist(ctx, "subagent_status_changed", snapshot); err != nil {
		return TaskSnapshot{}, err
	}
	m.replaceSnapshot(taskID, snapshot)
	task.cancel()
	go m.interruptAfterCancelGrace(taskID)
	return snapshot, nil
}

func (m *Manager) interruptAfterCancelGrace(taskID string) {
	timer := time.NewTimer(m.cfg.CancelGrace)
	defer timer.Stop()
	select {
	case <-timer.C:
		m.finish(taskID, StatusInterrupted, "", ErrorInterrupted, "subagent did not stop within the cancellation grace period")
	case <-m.ctx.Done():
		return
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
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return m.terminalPersistenceError()
	case <-ctx.Done():
		m.interruptNonTerminal("subagent manager close timed out")
		if err := m.terminalPersistenceError(); err != nil {
			return errors.Join(ctx.Err(), err)
		}
		return ctx.Err()
	}
}

func (m *Manager) interruptNonTerminal(message string) {
	for _, snapshot := range m.List() {
		if snapshot.Terminal() {
			continue
		}
		m.finish(snapshot.TaskID, StatusInterrupted, "", ErrorInterrupted, message)
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
	if m.cfg.Persist == nil {
		return nil
	}
	if err := m.cfg.Persist(ctx, PersistEvent{Kind: kind, Snapshot: snapshot}); err != nil {
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
	return task.snapshot, true
}

func (m *Manager) replaceSnapshot(taskID string, snapshot TaskSnapshot) bool {
	m.mu.Lock()
	task := m.tasks[taskID]
	if task == nil {
		m.mu.Unlock()
		return false
	}
	task.snapshot = snapshot
	m.mu.Unlock()
	m.signal()
	return true
}

func (m *Manager) prompt(taskID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if task := m.tasks[taskID]; task != nil {
		return task.prompt
	}
	return ""
}

func (m *Manager) signal() {
	select {
	case m.notify <- struct{}{}:
	default:
	}
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
	return value[:limit] + "\n\n[truncated]"
}
