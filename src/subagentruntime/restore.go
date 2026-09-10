package subagentruntime

import (
	"encoding/json"
)

// RestoredTask contains durable facts only. Restore never schedules a runner.
type RestoredTask struct {
	Snapshot        TaskSnapshot
	Prompt          string
	ContextSeed     json.RawMessage
	PendingMessages []QueuedMessage
	ResultDelivered bool
}

func (m *Manager) Restore(tasks []RestoredTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, saved := range tasks {
		snapshot := saved.Snapshot
		if snapshot.TaskID == "" || snapshot.AgentID == "" || m.tasks[snapshot.TaskID] != nil {
			continue
		}
		if !snapshot.Terminal() {
			snapshot.Status, snapshot.ErrorCode, snapshot.ErrorMessage = StatusInterrupted, ErrorInterrupted, "execution was interrupted by process restart"
		}
		done := make(chan struct{})
		close(done)
		m.tasks[snapshot.TaskID] = &managedTask{snapshot: snapshot, prompt: saved.Prompt, contextSeed: append(json.RawMessage(nil), saved.ContextSeed...), messages: append([]QueuedMessage(nil), saved.PendingMessages...),
			done: done, cancel: func() {}, seenToolCalls: map[string]struct{}{}, seenModelRequests: map[string]struct{}{}}
		m.order = append(m.order, snapshot.TaskID)
		if m.agentExecutions[snapshot.AgentID] == nil {
			m.agentExecutions[snapshot.AgentID] = make(chan struct{}, 1)
		}
		if snapshot.AgentClosed {
			m.closedAgents[snapshot.AgentID] = true
		}
		m.delivered[snapshot.TaskID] = saved.ResultDelivered
		if !saved.ResultDelivered {
			m.pendingTerminalNotifications = append(m.pendingTerminalNotifications, snapshot.TaskID)
		}
	}
}
