package agentcore

import (
	"fmt"

	"awaken/src/context"
)

type SessionID string
type ThreadID string
type RunID string
type TurnID string
type QueryChainID string
type Depth int
type TransitionReason string
type TerminalReason string

const (
	TransitionInitial                   TransitionReason = "initial"
	TransitionToolResultContinuation    TransitionReason = "tool_result_continuation"
	TransitionSubagentNotification      TransitionReason = "subagent_notification"
	TransitionXMLFallbackContinuation   TransitionReason = "xml_fallback_continuation"
	TransitionReactiveCompactRetry      TransitionReason = "reactive_compact_retry"
	TransitionMaxOutputContinuation     TransitionReason = "max_output_continuation"
	TransitionReasoningOnlyContinuation TransitionReason = "reasoning_only_continuation"
	TransitionBeforeFinishHook          TransitionReason = "before_finish_hook_retry"
)

const (
	TerminalReasonCompleted           TerminalReason = "completed"
	TerminalReasonCancelled           TerminalReason = "cancelled"
	TerminalReasonAbortedStreaming    TerminalReason = "aborted_streaming"
	TerminalReasonAbortedTools        TerminalReason = "aborted_tools"
	TerminalReasonMaxTurns            TerminalReason = "max_turns"
	TerminalReasonContextWarning      TerminalReason = "context_warning"
	TerminalReasonContextBlocked      TerminalReason = "context_blocked"
	TerminalReasonContextEpochError   TerminalReason = "context_epoch_error"
	TerminalReasonPromptTooLong       TerminalReason = "prompt_too_long"
	TerminalReasonCompactFailed       TerminalReason = "compact_failed"
	TerminalReasonModelError          TerminalReason = "model_error"
	TerminalReasonToolPairingError    TerminalReason = "tool_pairing_error"
	TerminalReasonEmptyResponse       TerminalReason = "empty_response"
	TerminalReasonToolPolicyAbort     TerminalReason = "tool_policy_abort"
	TerminalReasonMaxOutputReached    TerminalReason = "max_output_reached"
	TerminalReasonRepeatedToolFailure TerminalReason = "repeated_tool_failure"
	TerminalReasonPersistenceError    TerminalReason = "persistence_error"
	TerminalReasonHookBlocked         TerminalReason = "hook_blocked"
)

type TurnState struct {
	SessionID        SessionID
	ThreadID         ThreadID
	RunID            RunID
	TurnID           TurnID
	QueryChainID     QueryChainID
	Depth            Depth
	Messages         []context.Message
	TurnCount        int
	MaxTurns         int
	MainLoopModel    string
	TransitionReason TransitionReason
	TerminalReason   TerminalReason
}

func newTurnState(threadID, runID string, maxTurns int, model string) TurnState {
	return TurnState{
		ThreadID:      ThreadID(threadID),
		RunID:         RunID(runID),
		QueryChainID:  QueryChainID(newAwakenID("querychain")),
		MaxTurns:      maxTurns,
		MainLoopModel: model,
	}
}

func (state TurnState) nextTurn(messages []context.Message, transition TransitionReason) TurnState {
	state.TurnCount++
	state.Depth = Depth(state.TurnCount)
	state.TurnID = TurnID(fmt.Sprintf("%s_turn_%d", state.RunID, state.TurnCount))
	state.Messages = messages
	state.TransitionReason = transition
	state.TerminalReason = ""
	return state
}
