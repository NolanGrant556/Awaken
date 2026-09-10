package runtime

import (
	"context"
	"fmt"

	"awaken/src/agentcore"
	"awaken/src/computer"
	"awaken/src/subagentruntime"
)

func (s *RunService) childComputerFactory(session *Session) func(context.Context, subagentruntime.TaskSnapshot, agentcore.ModelConfig) (*computer.Runtime, func(), error) {
	return func(ctx context.Context, task subagentruntime.TaskSnapshot, model agentcore.ModelConfig) (*computer.Runtime, func(), error) {
		s.mu.RLock()
		settings, factory, vault, leases := s.computerSettings, s.computerDriverFactory, s.computerMediaVault, s.computerLeaseManager
		s.mu.RUnlock()
		if !settings.Enabled || !model.Capabilities.SupportsVision {
			return nil, nil, nil
		}
		policy, err := computer.NewPolicySnapshot(computer.PolicyFromSettings(settings))
		if err != nil {
			return nil, nil, err
		}
		binding, err := computer.NewBinding(task.RunID, task.ThreadID, task.AgentID, "host://local/desktop", policy)
		if err != nil {
			return nil, nil, err
		}
		binding.WaitForResource = true
		var driver computer.Driver = computer.NewUnavailableDriver()
		if factory != nil {
			created, err := factory(binding)
			if err != nil {
				return nil, nil, err
			}
			driver = created
		}
		runtime := computer.NewRuntimeWithLeaseManager(binding, driver, leases)
		runtime.SetMediaVault(vault)
		runtime.SetRecordFact(func(ctx context.Context, kind string, payload map[string]any) (string, error) {
			if reviewID, ok := payload["review_id"].(string); ok && reviewID != "" {
				session.mu.Lock()
				if session.computerReviewOwners == nil {
					session.computerReviewOwners = map[string]*computer.Runtime{}
				}
				session.computerReviewOwners[reviewID] = runtime
				session.mu.Unlock()
			}
			payload["agent_id"], payload["parent_agent_id"], payload["subagent_task_id"], payload["attempt_id"] = task.AgentID, task.ParentAgentID, task.TaskID, task.AttemptID
			record := agentcore.NewSemanticRecord(agentcore.SemanticKind(kind), task.ThreadID, task.RunID, "", "", "", payload)
			recorder := session.semanticRecorderSnapshot()
			if recorder == nil {
				return "", agentcore.ErrPersistenceUnavailable
			}
			if flushed, ok := recorder.(agentcore.FlushedSemanticRecorder); ok {
				assigned, err := flushed.AppendFlushed(ctx, record)
				return assigned.RecordID, err
			}
			if err := recorder.Append(ctx, record); err != nil {
				return "", err
			}
			return record.RecordID, recorder.Flush(ctx)
		})
		session.mu.Lock()
		if session.childComputers == nil {
			session.childComputers = map[string]*computer.Runtime{}
		}
		session.childComputers[string(binding.BindingID)] = runtime
		session.mu.Unlock()
		return runtime, func() {
			_ = runtime.Close(context.Background(), "agent_attempt_ended")
			session.mu.Lock()
			delete(session.childComputers, string(binding.BindingID))
			for id, owner := range session.computerReviewOwners {
				if owner == runtime {
					delete(session.computerReviewOwners, id)
				}
			}
			session.mu.Unlock()
		}, nil
	}
}

func (s *RunService) ComputerRuntimeForReview(runID, reviewID string) (*computer.Runtime, error) {
	session, err := s.session(runID)
	if err != nil {
		return nil, err
	}
	session.mu.RLock()
	owner, root := session.computerReviewOwners[reviewID], session.computerRuntime
	session.mu.RUnlock()
	if owner != nil {
		return owner, nil
	}
	if root != nil {
		return root, nil
	}
	return nil, fmt.Errorf("computer review owner was not found")
}

// ComputerRuntimeForSession resolves an explicit session only within its
// original run. Never fall back to the current lease holder for a stale item.
func (s *RunService) ComputerRuntimeForSession(runID, sessionID string) (*computer.Runtime, error) {
	owner, err := s.session(runID)
	if err != nil {
		return nil, err
	}
	if s.computerLeaseManager != nil {
		if lease, held := s.computerLeaseManager.Current("host://local/desktop"); held && string(lease.SessionID) == sessionID {
			owner.mu.RLock()
			child, root := owner.childComputers[string(lease.BindingID)], owner.computerRuntime
			owner.mu.RUnlock()
			if child != nil {
				return child, nil
			}
			if root != nil && root.Binding().BindingID == lease.BindingID {
				return root, nil
			}
		}
	}
	return nil, fmt.Errorf("computer session %s is no longer active in run %s", sessionID, runID)
}
