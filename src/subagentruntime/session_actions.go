package subagentruntime

import (
	"context"
	"errors"
	"strings"
)

// SessionController extends the legacy task API without making old callers
// invent stable Agent state. A follow-up creates a new assignment/task ID.
type SessionController interface {
	Followup(context.Context, string, string) (TaskSnapshot, error)
	CloseAgent(context.Context, string) (TaskSnapshot, error)
}

func (m *Manager) Followup(ctx context.Context, taskID, prompt string) (TaskSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return TaskSnapshot{}, err
	}
	task, ok := m.snapshot(strings.TrimSpace(taskID))
	if !ok {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "agent assignment was not found")
	}
	if m.hostOptions(taskID) != nil || task.Workflow != "" {
		return TaskSnapshot{}, runtimeError(ErrorBoundaryViolation, "this Host workflow does not allow automatic follow-up")
	}
	m.mu.Lock()
	owner, known := m.owners[task.ParentAgentID]
	if !known || owner.ctx == nil || owner.finished || owner.ctx.Err() != nil || m.stopped {
		m.mu.Unlock()
		return TaskSnapshot{}, runtimeError(ErrorBoundaryViolation, "saved creator must be revalidated before continuing this Agent")
	}
	if task.ScopeDigest != "" && task.ScopeDigest != owner.config.ScopeDigest {
		m.mu.Unlock()
		return TaskSnapshot{}, runtimeError(ErrorBoundaryViolation, "saved Agent workspace or execution permissions changed; start a new Agent")
	}
	m.mu.Unlock()
	return m.Spawn(ctx, SpawnRequest{
		ContinuesFrom: taskID,
		Description:   task.Description, Prompt: prompt, AgentType: task.AgentType,
		WorkspaceMode: task.WorkspaceMode, ParentAgentID: task.ParentAgentID,
		ThreadID: task.ThreadID, RunID: task.RunID, TurnID: task.TurnID,
		ModelProfileID: task.ModelProfileID, ReasoningEffort: task.ReasoningEffort, reuseAgentID: task.AgentID,
		Host: m.hostOptions(taskID),
	})
}

func (m *Manager) StopAll(ctx context.Context) error {
	return m.stopTasks(ctx, "")
}

// StopRun never cancels a later execution sharing the same conversation.
func (m *Manager) StopRun(ctx context.Context, runID string) error {
	return m.stopTasks(ctx, runID)
}

func (m *Manager) stopTasks(ctx context.Context, runID string) error {
	m.mu.Lock()
	if runID != "" && m.owners["root"].config.RunID != runID {
		m.mu.Unlock()
		return nil
	}
	m.stopped = true
	var cancels []context.CancelFunc
	var ids []string
	for id, task := range m.tasks {
		if !task.snapshot.Terminal() && (runID == "" || task.snapshot.RunID == runID) {
			cancels = append(cancels, task.cancel)
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	var stopErr error
	for _, id := range ids {
		if _, err := m.Cancel(ctx, id, "stop_all"); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
	}
	return stopErr
}

func (m *Manager) CloseAgent(ctx context.Context, taskID string) (TaskSnapshot, error) {
	task, ok := m.snapshot(strings.TrimSpace(taskID))
	if !ok {
		return TaskSnapshot{}, runtimeError(ErrorNotFound, "agent assignment was not found")
	}
	entry := m.task(taskID)
	entry.mutationMu.Lock()
	task = cloneTaskSnapshot(entry.snapshot)
	task.AgentClosed = true
	if err := m.persist(ctx, "subagent_status_changed", task); err != nil {
		entry.mutationMu.Unlock()
		return TaskSnapshot{}, err
	}
	m.replaceSnapshot(taskID, task)
	m.mu.Lock()
	m.closedAgents[task.AgentID] = true
	m.mu.Unlock()
	entry.mutationMu.Unlock()
	for _, candidate := range m.List() {
		if candidate.AgentID == task.AgentID && !candidate.Terminal() {
			if _, err := m.Cancel(ctx, candidate.TaskID, "agent_closed"); err != nil {
				return TaskSnapshot{}, err
			}
		}
	}
	if m.cfg.ForgetAgent != nil {
		m.cfg.ForgetAgent(task.AgentID)
	}
	return m.Status(taskID)
}
