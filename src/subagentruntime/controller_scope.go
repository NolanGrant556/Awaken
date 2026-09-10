package subagentruntime

import (
	"context"
	"errors"
)

func (m *Manager) EffectivePolicy() Policy          { return m.cfg.Policy }
func (s *scopedController) EffectivePolicy() Policy { return s.manager.EffectivePolicy() }

func (m *Manager) RetryTerminalDelivery(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delivered[taskID] = false
	for _, pending := range m.pendingTerminalNotifications {
		if pending == taskID {
			return
		}
	}
	m.pendingTerminalNotifications = append(m.pendingTerminalNotifications, taskID)
}
func (s *scopedController) RetryTerminalDelivery(taskID string) {
	if s.owns(taskID) {
		s.manager.RetryTerminalDelivery(taskID)
	}
}

// EndAttempt fences creation and waits for actual execution/resource cleanup.
// No child assignment may outlive its owning root execution.
func (m *Manager) EndAttempt(ctx context.Context, parentID, runID string) error {
	m.mu.Lock()
	owner := m.owners[parentID]
	if owner.config.RunID != runID {
		m.mu.Unlock()
		return nil
	}
	owner.finished = true
	m.owners[parentID] = owner
	m.mu.Unlock()
	if parentID == "root" {
		stopErr := m.StopRun(ctx, runID)
		done := make(chan struct{})
		go func() { m.wg.Wait(); close(done) }()
		select {
		case <-done:
			return errors.Join(stopErr, m.terminalPersistenceError())
		case <-ctx.Done():
			return errors.Join(stopErr, ctx.Err())
		}
	}
	var waits []<-chan struct{}
	for _, task := range m.List() {
		if task.ParentAgentID == parentID && task.RunID == runID {
			if !task.Terminal() {
				if _, err := m.Cancel(ctx, task.TaskID, "parent_attempt_ended"); err != nil {
					return err
				}
			}
			if entry := m.task(task.TaskID); entry != nil && entry.done != nil {
				waits = append(waits, entry.done)
			}
		}
	}
	for _, done := range waits {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// HasLiveWork includes a cancelled runner that has not actually returned.
// A forced interrupted status must not release storage or project ownership.
func (m *Manager) HasLiveWork(runID string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, task := range m.tasks {
		if runID != "" && task.snapshot.RunID != runID {
			continue
		}
		if !task.snapshot.Terminal() {
			return true
		}
		if task.done != nil {
			select {
			case <-task.done:
			default:
				return true
			}
		}
	}
	return m.reserved > 0
}

func (s *scopedController) EndAttempt(ctx context.Context, runID string) error {
	return s.manager.EndAttempt(ctx, s.agentID, runID)
}

type spawnOwner struct {
	finished bool
	ctx      context.Context
	depth    int
	config   Config
}

// Bind registers one immutable attempt's construction services. All levels use
// the same task registry, queue and execution pool; tool callers cannot supply
// their own creator or raise the tree depth limit.
func (m *Manager) Bind(ctx context.Context, agentID string, depth int, cfg Config) Controller {
	m.mu.Lock()
	if agentID == "root" && cfg.RunID != "" && cfg.RunID != m.owners[agentID].config.RunID {
		m.stopped = false
	}
	if agentID == "root" && !m.cfg.BindRunner {
		cfg = m.cfg
	}
	m.owners[agentID] = spawnOwner{ctx: ctx, depth: depth, config: cfg}
	m.mu.Unlock()
	return &scopedController{manager: m, agentID: agentID}
}

type scopedController struct {
	manager *Manager
	agentID string
}

func (s *scopedController) RetireCreator(cfg Config) {
	s.manager.mu.Lock()
	defer s.manager.mu.Unlock()
	owner, ok := s.manager.owners[s.agentID]
	if !ok || !owner.finished || !s.manager.cfg.BindRunner || owner.config.RunID != cfg.RunID {
		return
	}
	owner.config = cfg
	s.manager.owners[s.agentID] = owner
}

func (s *scopedController) Bind(ctx context.Context, id string, depth int, cfg Config) Controller {
	return s.manager.Bind(ctx, id, depth, cfg)
}
func (s *scopedController) EventDrivenWaits() bool { return s.manager.EventDrivenWaits() }
func (s *scopedController) Models(ctx context.Context) ([]ModelOption, error) {
	s.manager.mu.RLock()
	owner := s.manager.owners[s.agentID]
	s.manager.mu.RUnlock()
	if owner.config.ListModels == nil {
		return nil, runtimeError(ErrorRuntimeUnavailable, "configured model catalog is unavailable")
	}
	return owner.config.ListModels(ctx)
}
func (s *scopedController) Spawn(ctx context.Context, req SpawnRequest) (TaskSnapshot, error) {
	req.ParentAgentID = s.agentID
	return s.manager.Spawn(ctx, req)
}
func (s *scopedController) SpawnFromHost(ctx context.Context, req SpawnRequest) (TaskSnapshot, error) {
	return s.Spawn(ctx, req)
}
func (s *scopedController) List() []TaskSnapshot {
	var result []TaskSnapshot
	for _, task := range s.manager.List() {
		if task.ParentAgentID == s.agentID {
			result = append(result, task)
		}
	}
	return result
}
func (s *scopedController) owns(id string) bool {
	task, err := s.manager.Status(id)
	for err == nil {
		if task.ParentAgentID == s.agentID {
			return true
		}
		if task.ParentAgentID == "root" {
			break
		}
		found := false
		for _, parent := range s.manager.List() {
			if parent.AgentID == task.ParentAgentID {
				task = parent
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return false
}
func (s *scopedController) Status(id string) (TaskSnapshot, error) {
	if !s.owns(id) {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "task is outside the delegated branch")
	}
	return s.manager.Status(id)
}
func (s *scopedController) Wait(ctx context.Context, req WaitRequest) (WaitResult, error) {
	for _, id := range req.TaskIDs {
		if !s.owns(id) {
			return WaitResult{}, runtimeError(ErrorNotFound, "task is outside the delegated branch")
		}
	}
	return s.manager.Wait(ctx, req)
}
func (s *scopedController) DrainTerminalNotifications() []TaskSnapshot {
	return s.manager.drainTerminalForParent(s.agentID)
}
func (s *scopedController) AcknowledgeTerminal(id string) {
	if s.owns(id) {
		s.manager.AcknowledgeTerminal(id)
	}
}
func (s *scopedController) Message(ctx context.Context, id, body string) (MessageReceipt, error) {
	if !s.owns(id) {
		return MessageReceipt{}, runtimeError(ErrorNotFound, "task is outside the delegated branch")
	}
	return s.manager.Message(ctx, id, body)
}
func (s *scopedController) Cancel(ctx context.Context, id, reason string) (TaskSnapshot, error) {
	if !s.owns(id) {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "task is outside the delegated branch")
	}
	return s.manager.Cancel(ctx, id, reason)
}
func (s *scopedController) Followup(ctx context.Context, id, prompt string) (TaskSnapshot, error) {
	if !s.owns(id) {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "task is outside the delegated branch")
	}
	return s.manager.Followup(ctx, id, prompt)
}
func (s *scopedController) CloseAgent(ctx context.Context, id string) (TaskSnapshot, error) {
	if !s.owns(id) {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "task is outside the delegated branch")
	}
	return s.manager.CloseAgent(ctx, id)
}
func (s *scopedController) Close(ctx context.Context) error {
	if s.agentID == "root" {
		return s.manager.Close(ctx)
	}
	for _, task := range s.List() {
		if !task.Terminal() {
			if _, err := s.manager.Cancel(ctx, task.TaskID, "parent_attempt_ended"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) drainTerminalForParent(parentID string) []TaskSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []TaskSnapshot
	pending := make([]string, 0, len(m.pendingTerminalNotifications))
	for _, id := range m.pendingTerminalNotifications {
		task := m.tasks[id]
		if task == nil || m.delivered[id] {
			continue
		}
		if task.snapshot.ParentAgentID != parentID {
			pending = append(pending, id)
			continue
		}
		if task.snapshot.Terminal() {
			result = append(result, cloneTaskSnapshot(task.snapshot))
			m.delivered[id] = true
		}
	}
	m.pendingTerminalNotifications = pending
	return result
}
