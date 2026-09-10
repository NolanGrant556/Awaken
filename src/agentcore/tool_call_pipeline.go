package agentcore

import (
	convcontext "awaken/src/context"
	"awaken/src/tool"
	"context"
	"fmt"
	"strings"
)

type preparedToolCall struct {
	before     BeforeCallDecision
	result     *ToolExecutionResult
	stopReason TerminalReason
	started    bool
}

// Scheduling may be serial or parallel; the call boundary and failure rules
// stay identical. Hooks still run only on the serial path.
func (r *Runner) prepareToolCall(ctx context.Context, call convcontext.ToolCall, events *runEventEmitter) preparedToolCall {
	p := preparedToolCall{before: BeforeCallDecision{Action: CallExecute}}
	reject := func(result ToolExecutionResult, reason TerminalReason) preparedToolCall {
		p.result, p.stopReason = &result, reason
		return p
	}
	if events != nil {
		if err := events.persistenceFailure(); err != nil {
			return reject(syntheticPersistenceFailureResult(call, err), TerminalReasonPersistenceError)
		}
	}
	if ctx.Err() != nil {
		return reject(syntheticCancelledToolResult(call), "")
	}
	if r.failureSupervisor != nil {
		p.before = r.failureSupervisor.BeforeCall(call)
	}
	if events != nil {
		if err := events.emitToolCall(ctx, call); err != nil {
			return reject(syntheticPersistenceFailureResult(call, err), TerminalReasonPersistenceError)
		}
	}
	p.started = true
	if p.before.Action != CallExecute {
		if events != nil {
			events.emitRecoveryDecision(call, p.before.RecoveryOptions, p.before.ErrorCode, true)
		}
		reason := p.before.TerminalReason
		if p.before.Action == CallStopRun && reason == "" {
			reason = TerminalReasonRepeatedToolFailure
		}
		return reject(SyntheticBlockedResult(call, p.before), reason)
	}
	return p
}

func (r *Runner) toolExecutionIdentity(call convcontext.ToolCall, events *runEventEmitter) tool.ToolExecutionIdentity {
	id := tool.ToolExecutionIdentity{RunID: r.runID, AgentID: "root", SubagentTaskID: strings.TrimSpace(r.config.SubagentTaskID), ModelProfileID: strings.TrimSpace(r.config.ModelConfig.ProfileID), ToolCallID: call.ID, ToolName: call.Function.Name}
	if r.config.SubagentOrigin != nil && strings.TrimSpace(r.config.SubagentOrigin.AgentID) != "" {
		id.AgentID = strings.TrimSpace(r.config.SubagentOrigin.AgentID)
	}
	if events != nil {
		id.TurnID = events.turnID()
	}
	return id
}

func (r *Runner) finishToolOutcome(ctx context.Context, call convcontext.ToolCall, result ToolExecutionResult, events *runEventEmitter) (ToolExecutionResult, AfterOutcomeDecision, error) {
	var after AfterOutcomeDecision
	if err := r.offloadLargeToolResult(ctx, &result); err != nil {
		return syntheticPersistenceFailureResult(call, fmt.Errorf("tool result offload: %w", err)), after, err
	}
	if events != nil {
		if err := events.persistenceFailure(); err != nil {
			return syntheticPersistenceFailureResult(call, err), after, err
		}
	}
	if r.failureSupervisor != nil {
		after = r.failureSupervisor.AfterOutcome(call, result.Outcome)
		if len(after.RecoveryOptions) > 0 || after.RecoveryRequired {
			result.Outcome = ApplyRecoveryToOutcome(result.Outcome, after.RecoveryOptions, after.RecoveryRequired)
			result.IsError = result.Outcome.IsError()
		}
	}
	return result, after, nil
}

func (r *Runner) commitToolOutcome(ctx context.Context, call convcontext.ToolCall, result ToolExecutionResult, after AfterOutcomeDecision, buffer *artifactBufferingSink, events *runEventEmitter) error {
	if events == nil {
		return nil
	}
	var artifacts, facts []AwakenEvent
	if buffer != nil {
		artifacts, facts = buffer.artifacts, buffer.semanticFacts
	}
	if toolResultAppliedMutation(result) && (len(result.AffectedPaths) > 0 || len(result.TrashedPaths) > 0 || len(artifacts) > 0) {
		events.markMutation()
	}
	if err := events.emitToolResultBatch(ctx, result, artifacts, facts); err != nil {
		return err
	}
	if r.subagentController != nil {
		for _, taskID := range result.terminalSubagentTaskIDs {
			r.subagentController.AcknowledgeTerminal(taskID)
		}
	}
	if after.RecoveryRequired {
		events.emitRecoveryDecision(call, after.RecoveryOptions, result.Outcome.ErrorCode, false)
	}
	return nil
}
