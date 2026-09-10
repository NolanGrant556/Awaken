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
	emitCompactFailed(state ContextWindowState, mode CompactMode, message string, failureReason string, failure error)
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
	lastCompactFailureClass         string
	lastCompactBudgetReceipt        *CompactBudgetReceipt
	lastCompactNativeAttempted      bool
	lastCompactNativeUsed           bool
	lastCompactNativeReason         string
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

func (g *contextGovernor) RestoreUsageAnchor(anchor *ContextWindowUsageAnchor, config ModelConfig) {
	if g == nil || !ContextWindowUsageAnchorCompatibleWithModelConfig(anchor, config) {
		return
	}
	g.lastAPIUsageTokens = anchor.UsageTokens
	g.lastSentEstimatedInputTokens = anchor.SentEstimatedInputTokens
	if anchor.EstimatorID != contextEstimatorID {
		// Keep provider truth as a lower bound, but never apply an older
		// estimator's calibration ratio to the new estimator's fixed floor.
		g.lastSentEstimatedInputTokens = 0
	}
	g.pendingSentEstimatedInputTokens = 0
}

func (g *contextGovernor) ExportUsageAnchor(config ModelConfig) *ContextWindowUsageAnchor {
	if g == nil {
		return nil
	}
	return ContextWindowUsageAnchorFromModelConfig(config, g.lastAPIUsageTokens, g.lastSentEstimatedInputTokens)
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
			events.emitCompactFailed(state, mode, err.Error(), err.Error(), err)
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
				events.emitCompactFailed(failedState, mode, hookErr.Error(), hookErr.Error(), hookErr)
			}
			return llmReq, hookErr
		}
	}
	tailBudget := compactRetainedTailBudget(state, llmReq.Messages)
	if !reactive {
		if compactable := compactableHistoryTokens(llmReq.Messages, tailBudget, state.ConservativeProjectedTokens()); compactable < minCompactableHistoryTokens {
			err := errNotEnoughCompactableHistory
			g.recordCompactFailure(err)
			failedState := g.withRunState(llmReq.ContextWindowState)
			if events != nil {
				events.emitCompactFailed(failedState, mode, err.Error(), compactFailureReason(err), err)
			}
			return llmReq, err
		}
	}
	if preflightErr := g.preflightCompactTarget(llmReq, state, tailBudget); preflightErr != nil {
		g.recordCompactFailure(preflightErr)
		failedState := g.withRunState(llmReq.ContextWindowState)
		if events != nil {
			events.emitCompactFailed(failedState, mode, preflightErr.Error(), compactFailureReason(preflightErr), preflightErr)
		}
		return llmReq, preflightErr
	}
	compactRequest := CompactRequest{
		Mode:             mode,
		Envelope:         *llmReq.EnvelopeDebug,
		Messages:         llmReq.Messages,
		TargetTokens:     tailBudget,
		SummaryMaxTokens: compactSummaryBudget(state, llmReq.Messages),
		HookInstruction:  hookInstruction,
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
			if !shouldRetryCompactPostcondition(postErr, result, attempt) {
				break
			}
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
			events.emitCompactFailed(failedState, mode, message, compactFailureReason(err), err)
		}
		return llmReq, err
	}
	// Journal-first: the compacted conversation must be durable before it
	// replaces the in-memory truth. A persistence failure is a compact
	// failure and keeps the pre-compact messages authoritative.
	g.resetCompactDiagnostics()
	if g.applyCompactResult != nil {
		if applyErr := g.applyCompactResult(ctx, result); applyErr != nil {
			g.recordCompactFailure(applyErr)
			failedState := g.withRunState(llmReq.ContextWindowState)
			if events != nil {
				events.emitCompactFailed(failedState, mode, applyErr.Error(), applyErr.Error(), applyErr)
			}
			return llmReq, applyErr
		}
	}
	result.BudgetReceipt = cloneCompactBudgetReceipt(g.lastCompactBudgetReceipt)
	result.NativeCompactionAttempted = g.lastCompactNativeAttempted
	result.NativeCompactionUsed = g.lastCompactNativeUsed
	result.NativeCompactionReason = g.lastCompactNativeReason
	g.consecutiveCompactFailures = 0
	g.lastCompactFailureReason = ""
	g.lastCompactFailureClass = ""
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

func (g *contextGovernor) resetCompactDiagnostics() {
	if g == nil {
		return
	}
	g.lastCompactBudgetReceipt = nil
	g.lastCompactNativeAttempted = false
	g.lastCompactNativeUsed = false
	g.lastCompactNativeReason = ""
}

func (g *contextGovernor) captureCompactDiagnostics(result CompactResult, bundle PostCompactContextBundleV2) {
	if g == nil {
		return
	}
	g.lastCompactNativeAttempted = result.NativeCompactionAttempted
	g.lastCompactNativeUsed = result.NativeCompactionUsed
	g.lastCompactNativeReason = result.NativeCompactionReason
	if bundle.BudgetReceipt.State == MaterializationMaterialized && bundle.BudgetReceipt.Value != nil {
		receipt := *bundle.BudgetReceipt.Value
		receipt.ContributionTokens = cloneStringIntMap(receipt.ContributionTokens)
		g.lastCompactBudgetReceipt = &receipt
	}
}

func cloneCompactBudgetReceipt(receipt *CompactBudgetReceipt) *CompactBudgetReceipt {
	if receipt == nil {
		return nil
	}
	cloned := *receipt
	cloned.ContributionTokens = cloneStringIntMap(receipt.ContributionTokens)
	return &cloned
}

func cloneStringIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	result := make(map[string]int, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func (g *contextGovernor) recordCompactFailure(err error) {
	reason := compactFailureReason(err)
	class := compactFailureClass(err)
	if compactMandatoryFloorUnreachable(err) {
		g.consecutiveCompactFailures = 1
		g.lastCompactFailureReason = reason
		g.lastCompactFailureClass = class
		g.compactCircuitOpen = true
		return
	}
	if class == g.lastCompactFailureClass {
		g.consecutiveCompactFailures++
	} else {
		g.consecutiveCompactFailures = 1
	}
	g.lastCompactFailureReason = reason
	g.lastCompactFailureClass = class
	if g.consecutiveCompactFailures >= 3 {
		g.compactCircuitOpen = true
	}
}

func compactMandatoryFloorUnreachable(err error) bool {
	var targetErr *CompactTargetUnachievableError
	if !errors.As(err, &targetErr) || targetErr == nil {
		return false
	}
	return targetErr.Target > 0 && targetErr.MandatoryFloor > targetErr.Target
}

func (g *contextGovernor) evaluateCompactPostcondition(llmReq LLMRequest, preState ContextWindowState, result CompactResult) error {
	if result.Fallback.Used {
		return fmt.Errorf("%w: fallback summary is not a successful compact: %s", errInsufficientReduction, strings.TrimSpace(result.Fallback.Reason))
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
	target := preState.CompactTarget
	if postState.CompactTarget > 0 {
		target = postState.CompactTarget
	}
	if preState.HardCompactTarget && target > 0 && post > target {
		return &CompactTargetUnachievableError{
			Reason: "post_compact_projection_exceeds_target", Projected: post, Target: target, MandatoryFloor: post - estimateMessagesTokens(result.Messages),
		}
	}
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

// compactRetainedTailBudget converts the whole-request CompactTarget into the
// conversation-tail budget handed to the compactor. CompactTarget is defined
// against the effective input window, but splitCompactGroups only constrains
// the retained message tail: the stable prefix (system prompt, tools, MCP,
// Skill catalog, Memory, instruction prefix) and the new summary re-enter the
// post-compact request on top of that tail. Subtracting them keeps the target
// meaning "post-compact request size", not "retained tail size".
//
// When the provider reported more tokens for the last sent request than the
// local estimate captured at send time, the budget is additionally deflated by
// that ratio: the local estimator is not a provider tokenizer, and right after
// a compact the provider anchor is reset, so this is the only moment the known
// under-estimation bias can be applied. The deflation never inflates the
// budget and the result never drops below one token, which still retains the
// protected latest group.
func compactRetainedTailBudget(state ContextWindowState, messages []convcontext.Message) int {
	budget := calibratedCompactTarget(state)
	if budget <= 0 {
		return budget
	}
	if overhead := state.EstimatedTokens - estimateMessagesTokens(messages); overhead > 0 {
		budget -= overhead
	}
	budget -= compactSummaryBudget(state, messages)
	budget -= compactCheckpointProjectionReserve(state)
	if budget < 1 {
		return 1
	}
	return budget
}

func calibratedCompactTarget(state ContextWindowState) int {
	target := state.CompactTarget
	if target > 0 && state.LastAPIUsageTokens > 0 && state.LastSentEstimatedInputTokens > 0 &&
		state.LastAPIUsageTokens > state.LastSentEstimatedInputTokens {
		target = int(int64(target) * int64(state.LastSentEstimatedInputTokens) / int64(state.LastAPIUsageTokens))
	}
	return target
}

func compactCheckpointProjectionReserve(state ContextWindowState) int {
	if state.CompactTarget <= 0 {
		return 0
	}
	reserve := state.CompactTarget / 8
	if reserve < 1024 {
		reserve = 1024
	}
	if reserve > 4096 {
		reserve = 4096
	}
	return reserve
}

func compactSummaryBudget(state ContextWindowState, messages []convcontext.Message) int {
	remaining := calibratedCompactTarget(state)
	if overhead := state.EstimatedTokens - estimateMessagesTokens(messages); overhead > 0 && overhead < remaining {
		remaining -= overhead
	}
	budget := int(float64(remaining) * 0.35)
	if budget < 1024 {
		budget = 1024
	}
	if budget > 4096 {
		budget = 4096
	}
	return budget
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

func (g *contextGovernor) preflightCompactTarget(llmReq LLMRequest, state ContextWindowState, targetTokens int) error {
	if llmReq.EnvelopeDebug == nil || !state.HardCompactTarget || state.CompactTarget <= 0 {
		return nil
	}
	prefixGroups, tailGroups := splitCompactGroups(llmReq.Messages, targetTokens)
	prefixMessages := flattenCompactGroups(prefixGroups)
	tailMessages := flattenCompactGroups(tailGroups)
	if len(prefixMessages) == 0 {
		return nil
	}
	req := CompactRequest{
		Mode:         CompactModeAuto,
		Envelope:     *llmReq.EnvelopeDebug,
		Messages:     llmReq.Messages,
		TargetTokens: targetTokens,
	}
	if over, limit := compactTailExceedsBudget(req, tailMessages); over {
		tailMessages = lightenCompactTail(tailMessages)
		if estimateMessagesTokens(tailMessages) > limit {
			absorbed, protected, err := absorbOversizedCompactTail(prefixMessages, tailMessages, limit)
			if err != nil {
				return nil
			}
			prefixMessages = absorbed
			tailMessages = protected
		}
	}
	preflight := convcontext.Message{
		Role:              convcontext.RoleUser,
		Content:           "compact preflight summary",
		CompactBoundaryID: "compact-preflight",
	}
	envelope := *llmReq.EnvelopeDebug
	envelope.Messages = append([]convcontext.Message{preflight}, stripProviderOpaqueCompactionState(tailMessages)...)
	postState := g.engine.Evaluate(envelope)
	projected := postState.ConservativeProjectedTokens()
	target := state.CompactTarget
	if postState.CompactTarget > 0 {
		target = postState.CompactTarget
	}
	if projected <= target {
		return nil
	}
	return &CompactTargetUnachievableError{
		Reason:         "pre_compact_mandatory_floor_exceeds_target",
		Projected:      projected,
		Target:         target,
		MandatoryFloor: projected,
	}
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
	case errors.Is(err, ErrCompactTargetUnachievable):
		return "compact_target_unachievable"
	default:
		return err.Error()
	}
}

func compactFailureClass(err error) string {
	reason := compactFailureReason(err)
	switch reason {
	case compactFailureInsufficientReduction,
		compactFailureStillAboveAutoThreshold,
		compactFailureStillAboveBlockThreshold,
		"compact_target_unachievable":
		return "budget_unresolved"
	default:
		return reason
	}
}

func shouldRetryCompactPostcondition(err error, result CompactResult, attempt int) bool {
	if err == nil {
		return false
	}
	if result.Fallback.Used {
		return false
	}
	if errors.Is(err, errInsufficientReduction) {
		return false
	}
	var targetErr *CompactTargetUnachievableError
	if errors.As(err, &targetErr) {
		if compactMandatoryFloorUnreachable(targetErr) {
			return false
		}
		return attempt < 1
	}
	if attempt >= 1 {
		return false
	}
	return errors.Is(err, errStillAboveAutoThreshold) || errors.Is(err, errStillAboveBlockThreshold)
}

func (g *contextGovernor) compactCircuitOpenError() error {
	return fmt.Errorf("%w after %d consecutive failures", errContextCompactCircuitOpen, g.consecutiveCompactFailures)
}

func contextWindowConfigFromModelConfig(config ModelConfig) ContextWindowConfig {
	outputReserve := defaultOutputReserve
	if config.MaxTokens > 0 {
		outputReserve = config.MaxTokens
	}
	inputWindow := config.ContextWindow
	if config.MaxInputTokens > 0 && (inputWindow <= 0 || config.MaxInputTokens < inputWindow) {
		inputWindow = config.MaxInputTokens
	}
	return ContextWindowConfig{
		ModelWindow:           inputWindow,
		OutputReserve:         outputReserve,
		EffectiveInputWindow:  inputWindow,
		AutoCompactTokenLimit: config.AutoCompactTokenLimit,
		HardCompactTarget:     config.ContextWindow > 0 || config.MaxInputTokens > 0,
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
