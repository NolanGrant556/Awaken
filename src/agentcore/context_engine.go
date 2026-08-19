package agentcore

import (
	stdcontext "context"
	"errors"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
)

var (
	errContextCompactCircuitOpen = errors.New("context compact circuit open")
	errInsufficientReduction     = errors.New("insufficient reduction")
	errStillAboveAutoThreshold   = errors.New("still above auto threshold")
	errStillAboveBlockThreshold  = errors.New("still above block threshold")
)

const (
	minCompactableHistoryTokens = 5000
	compactInnerRetryLimit      = 2
	compactMinReductionRatio    = 0.80
)

const (
	compactFailureInsufficientReduction    = "insufficient_reduction"
	compactFailureStillAboveAutoThreshold  = "still_above_auto_threshold"
	compactFailureStillAboveBlockThreshold = "still_above_block_threshold"
	compactFailureOversizedProtectedTail   = "oversized_protected_tail"
	compactFailureNotEnoughHistory         = "not_enough_compactable_history"
)

type ContextEngine interface {
	Evaluate(envelope ContextEnvelope) ContextWindowState
	Compact(ctx stdcontext.Context, req CompactRequest) (CompactResult, error)
}

type defaultContextEngine struct {
	config    ContextWindowConfig
	compactFn func(stdcontext.Context, CompactRequest) (CompactResult, error)
}

type contextWindowEventSink interface {
	emitContextWindow(state ContextWindowState)
	emitContextBlocked(state ContextWindowState, message string)
	emitCompactStarted(state ContextWindowState, mode CompactMode)
	emitCompactFinished(state ContextWindowState, result CompactResult, summaryModel string)
	emitCompactFailed(state ContextWindowState, mode CompactMode, message string, failureReason string)
}

type contextGovernor struct {
	engine                          ContextEngine
	rebuildRequest                  func() (LLMRequest, error)
	applyCompactResult              func(stdcontext.Context, CompactResult) error
	preCompactHook                  func(stdcontext.Context, contextWindowEventSink, CompactMode, ContextWindowState) (string, error)
	postCompactHook                 func(stdcontext.Context, contextWindowEventSink, CompactResult) error
	consecutiveCompactFailures      int
	reactiveCompactAttempted        bool
	compactCircuitOpen              bool
	lastAPIUsageTokens              int
	lastSentEstimatedInputTokens    int
	pendingSentEstimatedInputTokens int
	lastCompactFailureReason        string
}

func NewContextEngine(config ContextWindowConfig) ContextEngine {
	return &defaultContextEngine{
		config:    config,
		compactFn: CompactConversationWithRequest,
	}
}

func (e *defaultContextEngine) Evaluate(envelope ContextEnvelope) ContextWindowState {
	return NewContextWindowStateWithConfig(envelope, e.config)
}

func (e *defaultContextEngine) Compact(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
	if req.Messages == nil {
		req.Messages = req.Envelope.Messages
	}
	return e.compactFn(ctx, req)
}

func newContextGovernor(engine ContextEngine, rebuildRequest func() (LLMRequest, error), applyCompactResult func(stdcontext.Context, CompactResult) error) *contextGovernor {
	if engine == nil {
		engine = NewContextEngine(ContextWindowConfig{})
	}
	return &contextGovernor{
		engine:             engine,
		rebuildRequest:     rebuildRequest,
		applyCompactResult: applyCompactResult,
	}
}

func (g *contextGovernor) PrepareRequest(ctx stdcontext.Context, llmReq LLMRequest, events contextWindowEventSink) (LLMRequest, error) {
	llmReq = g.evaluateRequest(llmReq)
	if events != nil {
		events.emitContextWindow(llmReq.ContextWindowState)
	}
	var compactErr error
	// ShouldAutoCompact drives the v4-estimator path. ShouldAutoCompactByActualUsage
	// is a parallel trigger based on the last provider-reported usage plus the
	// locally-estimated delta; it catches cases where the v4 estimate is too
	// optimistic. Either trigger fires a proactive compact.
	if llmReq.ContextWindowState.ShouldAutoCompact() || llmReq.ContextWindowState.ShouldAutoCompactByActualUsage() {
		compactedReq, err := g.compactForRequest(ctx, llmReq, events, false)
		if err != nil {
			compactErr = err
			if emitter, ok := events.(*runEventEmitter); ok && emitter != nil && emitter.persistenceFailure() != nil {
				return LLMRequest{}, err
			}
			if llmReq.ContextWindowState.ShouldBlock() && g.compactCircuitOpen {
				blockErr := newContextWindowBlockedError(llmReq.ContextWindowState, err)
				if events != nil {
					events.emitContextBlocked(g.withRunState(llmReq.ContextWindowState), blockErr.Error())
				}
				return LLMRequest{}, blockErr
			}
			if !llmReq.ContextWindowState.ShouldBlock() {
				return g.evaluateRequest(llmReq), nil
			}
		} else {
			llmReq = compactedReq
			if events != nil {
				events.emitContextWindow(llmReq.ContextWindowState)
			}
		}
	}
	if llmReq.ContextWindowState.ShouldBlock() {
		if !g.compactCircuitOpen && g.consecutiveCompactFailures < 3 {
			if compactErr != nil {
				return LLMRequest{}, compactErr
			}
			return g.evaluateRequest(llmReq), fmt.Errorf("%w: estimated %d tokens exceeds blocking threshold %d",
				errStillAboveBlockThreshold,
				llmReq.ContextWindowState.ConservativeProjectedTokens(),
				llmReq.ContextWindowState.blockingThreshold(),
			)
		}
		blockErr := newContextWindowBlockedError(llmReq.ContextWindowState, compactErr)
		if events != nil {
			events.emitContextBlocked(g.withRunState(llmReq.ContextWindowState), blockErr.Error())
		}
		return LLMRequest{}, blockErr
	}
	return llmReq, nil
}

func (g *contextGovernor) TryReactiveCompact(ctx stdcontext.Context, events contextWindowEventSink) (bool, error) {
	if g.reactiveCompactAttempted {
		return false, nil
	}
	g.reactiveCompactAttempted = true
	if g.lastCompactFailureReason == compactFailureOversizedProtectedTail {
		return false, errOversizedProtectedTail
	}
	llmReq, err := g.rebuildRequest()
	if err != nil {
		return false, err
	}
	llmReq = g.evaluateRequest(llmReq)
	if _, err := g.compactForRequest(ctx, llmReq, events, true); err != nil {
		if llmReq.ContextWindowState.ShouldBlock() && g.compactCircuitOpen && events != nil {
			events.emitContextBlocked(g.withRunState(llmReq.ContextWindowState), newContextWindowBlockedError(llmReq.ContextWindowState, err).Error())
		}
		return false, err
	}
	return true, nil
}

func (g *contextGovernor) evaluateRequest(llmReq LLMRequest) LLMRequest {
	if llmReq.EnvelopeDebug != nil {
		llmReq.ContextWindowState = g.engine.Evaluate(*llmReq.EnvelopeDebug)
	}
	llmReq.ContextWindowState = g.withRunState(llmReq.ContextWindowState)
	return llmReq
}

func (g *contextGovernor) withRunState(state ContextWindowState) ContextWindowState {
	state.ConsecutiveCompactFailures = g.consecutiveCompactFailures
	state.HasAttemptedReactiveCompact = g.reactiveCompactAttempted
	state.CompactCircuitOpen = g.compactCircuitOpen
	state.LastAPIUsageTokens = g.lastAPIUsageTokens
	state.LastSentEstimatedInputTokens = g.lastSentEstimatedInputTokens
	return state
}

func (g *contextGovernor) RecordSentRequestEstimate(estimatedInputTokens int) {
	if estimatedInputTokens > 0 {
		g.pendingSentEstimatedInputTokens = estimatedInputTokens
	}
}

func (g *contextGovernor) RecordResponseUsage(responseUsageTokens int) {
	if responseUsageTokens > 0 {
		g.lastAPIUsageTokens = responseUsageTokens
		g.lastSentEstimatedInputTokens = g.pendingSentEstimatedInputTokens
		g.pendingSentEstimatedInputTokens = 0
	}
}

func (g *contextGovernor) compactForRequest(ctx stdcontext.Context, llmReq LLMRequest, events contextWindowEventSink, reactive bool) (LLMRequest, error) {
	if llmReq.EnvelopeDebug == nil {
		return LLMRequest{}, errors.New("missing context envelope for compact")
	}
	state := g.withRunState(llmReq.ContextWindowState)
	mode := compactMode(reactive)
	if g.compactCircuitOpen {
		err := g.compactCircuitOpenError()
		if events != nil {
			events.emitCompactFailed(state, mode, err.Error(), err.Error())
		}
		return llmReq, err
	}
	if events != nil {
		events.emitCompactStarted(state, mode)
	}
	hookInstruction := ""
	if g.preCompactHook != nil {
		var hookErr error
		hookInstruction, hookErr = g.preCompactHook(ctx, events, mode, state)
		if hookErr != nil {
			g.recordCompactFailure(hookErr)
			failedState := g.withRunState(llmReq.ContextWindowState)
			if events != nil {
				events.emitCompactFailed(failedState, mode, hookErr.Error(), hookErr.Error())
			}
			return llmReq, hookErr
		}
	}
	if !reactive {
		if compactable := compactableHistoryTokens(llmReq.Messages, state.CompactTarget, state.ConservativeProjectedTokens()); compactable < minCompactableHistoryTokens {
			err := errNotEnoughCompactableHistory
			g.recordCompactFailure(err)
			failedState := g.withRunState(llmReq.ContextWindowState)
			if events != nil {
				events.emitCompactFailed(failedState, mode, err.Error(), compactFailureReason(err))
			}
			return llmReq, err
		}
	}
	compactRequest := CompactRequest{
		Mode:            mode,
		Envelope:        *llmReq.EnvelopeDebug,
		Messages:        llmReq.Messages,
		TargetTokens:    llmReq.ContextWindowState.CompactTarget,
		HookInstruction: hookInstruction,
	}
	if emitter, ok := events.(*runEventEmitter); ok && emitter != nil {
		compactRequest.summaryInvoker = emitter.invokeCompactSummary
	}
	var result CompactResult
	var err error
	for attempt := 0; attempt <= compactInnerRetryLimit; attempt++ {
		if attempt > 0 {
			compactRequest.Messages = lightenCompactTail(compactRequest.Messages)
			if compactRequest.TargetTokens > 1 {
				compactRequest.TargetTokens = compactRequest.TargetTokens / 2
				if compactRequest.TargetTokens < 1 {
					compactRequest.TargetTokens = 1
				}
			}
		}
		result, err = g.engine.Compact(ctx, compactRequest)
		if err != nil {
			break
		}
		if postErr := g.evaluateCompactPostcondition(llmReq, state, result); postErr != nil {
			err = postErr
			continue
		}
		err = nil
		break
	}
	if err != nil {
		g.recordCompactFailure(err)
		failedState := g.withRunState(llmReq.ContextWindowState)
		message := err.Error()
		if g.compactCircuitOpen {
			message = fmt.Sprintf("%s: %v", g.compactCircuitOpenError(), err)
		}
		if events != nil {
			events.emitCompactFailed(failedState, mode, message, compactFailureReason(err))
		}
		return llmReq, err
	}
	// Journal-first: the compacted conversation must be durable before it
	// replaces the in-memory truth. A persistence failure is a compact
	// failure and keeps the pre-compact messages authoritative.
	if g.applyCompactResult != nil {
		if applyErr := g.applyCompactResult(ctx, result); applyErr != nil {
			g.recordCompactFailure(applyErr)
			failedState := g.withRunState(llmReq.ContextWindowState)
			if events != nil {
				events.emitCompactFailed(failedState, mode, applyErr.Error(), applyErr.Error())
			}
			return llmReq, applyErr
		}
	}
	g.consecutiveCompactFailures = 0
	g.lastCompactFailureReason = ""
	// A successful compact changes the conversation, so the previous
	// provider-reported usage no longer corresponds to the next request's
	// input. Reset the actual-usage trigger so ShouldAutoCompactByActualUsage
	// stays inert until the next response sets a fresh LastAPIUsageTokens.
	g.lastAPIUsageTokens = 0
	g.lastSentEstimatedInputTokens = 0
	g.pendingSentEstimatedInputTokens = 0
	if events != nil {
		events.emitCompactFinished(state, result, compactSummaryModel(*llmReq.EnvelopeDebug))
	}
	if g.postCompactHook != nil {
		if hookErr := g.postCompactHook(ctx, events, result); hookErr != nil {
			if emitter, ok := events.(*runEventEmitter); ok && emitter != nil {
				_ = emitter.notePersistenceFailure("post_compact_hook", hookErr)
			}
			return LLMRequest{}, hookErr
		}
	}
	rebuilt, err := g.rebuildRequest()
	if err != nil {
		return LLMRequest{}, err
	}
	return g.evaluateRequest(rebuilt), nil
}

func (g *contextGovernor) recordCompactFailure(err error) {
	g.consecutiveCompactFailures++
	g.lastCompactFailureReason = compactFailureReason(err)
	if g.consecutiveCompactFailures >= 3 {
		g.compactCircuitOpen = true
	}
}

func (g *contextGovernor) evaluateCompactPostcondition(llmReq LLMRequest, preState ContextWindowState, result CompactResult) error {
	if result.Fallback.Used {
		return fmt.Errorf("%w: fallback summary is not a successful compact", errInsufficientReduction)
	}
	summaryText := strings.TrimSpace(result.SummaryMessage.Content)
	if summaryText == "" && len(result.Messages) > 0 {
		summaryText = strings.TrimSpace(result.Messages[0].Content)
	}
	if summaryText == "" {
		return fmt.Errorf("%w: empty summary", errInsufficientReduction)
	}
	if err := convcontext.ValidateToolResultPairing(result.Messages); err != nil {
		return err
	}
	envelope := ContextEnvelope{}
	if llmReq.EnvelopeDebug != nil {
		envelope = *llmReq.EnvelopeDebug
	}
	envelope.Messages = result.Messages
	postState := g.engine.Evaluate(envelope)
	postState.LastAPIUsageTokens = 0
	postState.LastSentEstimatedInputTokens = 0
	post := postState.ConservativeProjectedTokens()
	autoThreshold := preState.AutoCompactThreshold
	if postState.AutoCompactThreshold > 0 {
		autoThreshold = postState.AutoCompactThreshold
	}
	if autoThreshold > 0 && post >= autoThreshold {
		return fmt.Errorf("%w: post=%d auto=%d", errStillAboveAutoThreshold, post, autoThreshold)
	}
	blockThreshold := preState.blockingThreshold()
	if postState.blockingThreshold() > 0 {
		blockThreshold = postState.blockingThreshold()
	}
	if blockThreshold > 0 && post >= blockThreshold {
		return fmt.Errorf("%w: post=%d block=%d", errStillAboveBlockThreshold, post, blockThreshold)
	}
	preMessages := estimateMessagesTokens(llmReq.Messages)
	postMessages := estimateMessagesTokens(result.Messages)
	if preMessages >= minCompactableHistoryTokens && postMessages > int(float64(preMessages)*compactMinReductionRatio) {
		return fmt.Errorf("%w: post=%d pre=%d", errInsufficientReduction, postMessages, preMessages)
	}
	return nil
}

func compactableHistoryTokens(messages []convcontext.Message, targetTokens, projectedTokens int) int {
	prefix, _ := splitCompactGroups(messages, targetTokens)
	localPrefix := estimateMessagesTokens(flattenCompactGroups(prefix))
	localTotal := estimateMessagesTokens(messages)
	if localTotal > 0 && projectedTokens > localTotal*2 {
		return projectedTokens
	}
	return localPrefix
}

func compactFailureReason(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, errInsufficientReduction):
		return compactFailureInsufficientReduction
	case errors.Is(err, errStillAboveAutoThreshold):
		return compactFailureStillAboveAutoThreshold
	case errors.Is(err, errStillAboveBlockThreshold):
		return compactFailureStillAboveBlockThreshold
	case errors.Is(err, errOversizedProtectedTail):
		return compactFailureOversizedProtectedTail
	case errors.Is(err, errNotEnoughCompactableHistory):
		return compactFailureNotEnoughHistory
	default:
		return err.Error()
	}
}

func (g *contextGovernor) compactCircuitOpenError() error {
	return fmt.Errorf("%w after %d consecutive failures", errContextCompactCircuitOpen, g.consecutiveCompactFailures)
}

func contextWindowConfigFromModelConfig(config ModelConfig) ContextWindowConfig {
	modelWindow := config.ContextWindow
	// Some provider catalogs report an input-only ceiling while others report
	// a total context window. Convert the input-only ceiling into the engine's
	// total-window representation so the output reserve is not subtracted twice.
	if config.MaxInputTokens > 0 && (modelWindow <= 0 || config.MaxInputTokens < modelWindow) {
		modelWindow = config.MaxInputTokens + defaultOutputReserve
	}
	return ContextWindowConfig{
		ModelWindow:           modelWindow,
		AutoCompactTokenLimit: config.AutoCompactTokenLimit,
	}
}

func compactMode(reactive bool) CompactMode {
	if reactive {
		return CompactModeReactive
	}
	return CompactModeAuto
}

func compactSummaryModel(envelope ContextEnvelope) string {
	cfg := llmConfigForCompaction(envelope)
	return cfg.Model
}

func newContextWindowBlockedError(state ContextWindowState, cause error) error {
	if cause == nil {
		return fmt.Errorf("%w: estimated %d tokens exceeds blocking threshold %d",
			errContextWindowBlocked,
			state.EstimatedTokens,
			state.blockingThreshold(),
		)
	}
	return fmt.Errorf("%w: estimated %d tokens exceeds blocking threshold %d: %v",
		errContextWindowBlocked,
		state.EstimatedTokens,
		state.blockingThreshold(),
		cause,
	)
}
