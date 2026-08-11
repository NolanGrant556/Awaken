package agentcore

import (
	stdcontext "context"
	"errors"
	"fmt"
)

var errContextCompactCircuitOpen = errors.New("context compact circuit open")

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
	engine                     ContextEngine
	rebuildRequest             func() (LLMRequest, error)
	applyCompactResult         func(stdcontext.Context, CompactResult) error
	consecutiveCompactFailures int
	reactiveCompactAttempted   bool
	compactCircuitOpen         bool
	lastAPIUsageTokens         int
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
	// ShouldAutoCompact drives the v4-estimator path. ShouldAutoCompactByActualUsage
	// is a parallel trigger based on the last provider-reported usage plus the
	// locally-estimated delta; it catches cases where the v4 estimate is too
	// optimistic. Either trigger fires a proactive compact.
	if llmReq.ContextWindowState.ShouldAutoCompact() || llmReq.ContextWindowState.ShouldAutoCompactByActualUsage() {
		compactedReq, err := g.compactForRequest(ctx, llmReq, events, false)
		if err != nil {
			if llmReq.ContextWindowState.ShouldBlock() && g.compactCircuitOpen {
				blockErr := newContextWindowBlockedError(llmReq.ContextWindowState, err)
				if events != nil {
					events.emitContextBlocked(g.withRunState(llmReq.ContextWindowState), blockErr.Error())
				}
				return LLMRequest{}, blockErr
			}
			return g.evaluateRequest(llmReq), nil
		}
		llmReq = compactedReq
		if events != nil {
			events.emitContextWindow(llmReq.ContextWindowState)
		}
	}
	if llmReq.ContextWindowState.ShouldBlock() {
		blockErr := newContextWindowBlockedError(llmReq.ContextWindowState, nil)
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
	llmReq, err := g.rebuildRequest()
	if err != nil {
		return false, err
	}
	llmReq = g.evaluateRequest(llmReq)
	if _, err := g.compactForRequest(ctx, llmReq, events, true); err != nil {
		if llmReq.ContextWindowState.ShouldBlock() && events != nil {
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
	return state
}

func (g *contextGovernor) RecordResponseUsage(responseUsageTokens int) {
	if responseUsageTokens > 0 {
		g.lastAPIUsageTokens = responseUsageTokens
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
	compactRequest := CompactRequest{
		Mode:         mode,
		Envelope:     *llmReq.EnvelopeDebug,
		Messages:     llmReq.Messages,
		TargetTokens: llmReq.ContextWindowState.CompactTarget,
	}
	if emitter, ok := events.(*runEventEmitter); ok && emitter != nil {
		compactRequest.summaryInvoker = emitter.invokeCompactSummary
	}
	result, err := g.engine.Compact(ctx, compactRequest)
	if err != nil {
		g.recordCompactFailure()
		failedState := g.withRunState(llmReq.ContextWindowState)
		message := err.Error()
		if g.compactCircuitOpen {
			message = fmt.Sprintf("%s: %v", g.compactCircuitOpenError(), err)
		}
		if events != nil {
			events.emitCompactFailed(failedState, mode, message, err.Error())
		}
		return llmReq, err
	}
	// Journal-first: the compacted conversation must be durable before it
	// replaces the in-memory truth. A persistence failure is a compact
	// failure and keeps the pre-compact messages authoritative.
	if g.applyCompactResult != nil {
		if applyErr := g.applyCompactResult(ctx, result); applyErr != nil {
			g.recordCompactFailure()
			failedState := g.withRunState(llmReq.ContextWindowState)
			if events != nil {
				events.emitCompactFailed(failedState, mode, applyErr.Error(), applyErr.Error())
			}
			return llmReq, applyErr
		}
	}
	g.consecutiveCompactFailures = 0
	// A successful compact changes the conversation, so the previous
	// provider-reported usage no longer corresponds to the next request's
	// input. Reset the actual-usage trigger so ShouldAutoCompactByActualUsage
	// stays inert until the next response sets a fresh LastAPIUsageTokens.
	g.lastAPIUsageTokens = 0
	if events != nil {
		events.emitCompactFinished(state, result, compactSummaryModel(*llmReq.EnvelopeDebug))
	}
	rebuilt, err := g.rebuildRequest()
	if err != nil {
		return LLMRequest{}, err
	}
	return g.evaluateRequest(rebuilt), nil
}

func (g *contextGovernor) recordCompactFailure() {
	g.consecutiveCompactFailures++
	if g.consecutiveCompactFailures >= 3 {
		g.compactCircuitOpen = true
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
