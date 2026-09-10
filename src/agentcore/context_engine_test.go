package agentcore

import (
	stdcontext "context"
	"errors"
	"fmt"
	"strings"
	"testing"

	convcontext "awaken/src/context"
)

type fakeContextEngine struct {
	compactCalls int
	evaluateFn   func(ContextEnvelope) ContextWindowState
	compactFn    func(stdcontext.Context, CompactRequest) (CompactResult, error)
}

func (f *fakeContextEngine) Evaluate(envelope ContextEnvelope) ContextWindowState {
	if f.evaluateFn != nil {
		return f.evaluateFn(envelope)
	}
	return ContextWindowState{}
}

func (f *fakeContextEngine) Compact(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
	f.compactCalls++
	if f.compactFn != nil {
		return f.compactFn(ctx, req)
	}
	return CompactResult{}, nil
}

type fakeContextEventSink struct {
	operations []string
	failed     []string
	blocked    []string
}

func (s *fakeContextEventSink) emitContextWindow(state ContextWindowState) {
	s.operations = append(s.operations, "window")
}

func (s *fakeContextEventSink) emitContextBlocked(state ContextWindowState, message string) {
	s.operations = append(s.operations, "blocked")
	s.blocked = append(s.blocked, message)
}

func (s *fakeContextEventSink) emitCompactStarted(state ContextWindowState, mode CompactMode) {
	s.operations = append(s.operations, "compact_started")
}

func (s *fakeContextEventSink) emitCompactFinished(state ContextWindowState, result CompactResult, summaryModel string) {
	s.operations = append(s.operations, "compact_finished")
}

func (s *fakeContextEventSink) emitCompactFailed(state ContextWindowState, mode CompactMode, message string, failureReason string, _ error) {
	s.operations = append(s.operations, "compact_failed")
	s.failed = append(s.failed, message)
}

func TestContextGovernorPrepareRequestAutoCompactsAndRebuilds(t *testing.T) {
	originalMessages := []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}}
	compactedMessages := []convcontext.Message{{
		Role:              convcontext.RoleUser,
		Content:           "summary",
		CompactBoundaryID: "compact-1",
	}}
	originalEnvelope := ContextEnvelope{Messages: originalMessages}
	compactedEnvelope := ContextEnvelope{Messages: compactedMessages}

	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			switch {
			case len(envelope.Messages) > 0 && envelope.Messages[0].CompactBoundaryID != "":
				return testContextWindowState(5400)
			default:
				return testContextWindowState(92000)
			}
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{
				BoundaryID:      "compact-1",
				Messages:        compactedMessages,
				EstimatedTokens: 5400,
			}, nil
		},
	}

	var appliedMessages []convcontext.Message
	rebuildCalls := 0
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		rebuildCalls++
		return LLMRequest{
			Messages:      compactedMessages,
			EnvelopeDebug: &compactedEnvelope,
		}, nil
	}, func(_ stdcontext.Context, result CompactResult) error {
		appliedMessages = cloneContextMessages(result.Messages)
		return nil
	})
	governor.consecutiveCompactFailures = 2

	sink := &fakeContextEventSink{}
	req, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      originalMessages,
		EnvelopeDebug: &originalEnvelope,
	}, sink)
	if err != nil {
		t.Fatalf("PrepareRequest returned error: %v", err)
	}
	if engine.compactCalls != 1 {
		t.Fatalf("expected one compact attempt, got %d", engine.compactCalls)
	}
	if rebuildCalls != 1 {
		t.Fatalf("expected one rebuild after compact, got %d", rebuildCalls)
	}
	if governor.consecutiveCompactFailures != 0 || governor.compactCircuitOpen {
		t.Fatalf("expected compact success to reset failure state, got failures=%d open=%v", governor.consecutiveCompactFailures, governor.compactCircuitOpen)
	}
	if len(appliedMessages) != 1 || appliedMessages[0].CompactBoundaryID != "compact-1" {
		t.Fatalf("expected compacted messages to be applied, got %+v", appliedMessages)
	}
	if req.ContextWindowState.EstimatedTokens != 5400 {
		t.Fatalf("expected rebuilt request state, got %+v", req.ContextWindowState)
	}
	if got := strings.Join(sink.operations, ","); got != "window,compact_started,compact_finished,window" {
		t.Fatalf("unexpected event operation order: %s", got)
	}
}

func TestContextGovernorCircuitBreakerStopsRepeatedCompactAttempts(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testContextWindowState(92000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{}, errors.New("compact boom")
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{}, nil
	}, nil)

	sink := &fakeContextEventSink{}
	for i := 0; i < 4; i++ {
		req, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
			Messages:      envelope.Messages,
			EnvelopeDebug: &envelope,
		}, sink)
		if err != nil {
			t.Fatalf("PrepareRequest returned unexpected error on attempt %d: %v", i+1, err)
		}
		if req.ContextWindowState.ConsecutiveCompactFailures != minInt(i+1, 3) {
			t.Fatalf("unexpected failure count on attempt %d: %+v", i+1, req.ContextWindowState)
		}
	}

	if engine.compactCalls != 3 {
		t.Fatalf("expected circuit breaker to stop compact after 3 failures, got %d calls", engine.compactCalls)
	}
	if !governor.compactCircuitOpen || governor.consecutiveCompactFailures != 3 {
		t.Fatalf("expected circuit breaker open after 3 failures, got failures=%d open=%v", governor.consecutiveCompactFailures, governor.compactCircuitOpen)
	}
	if len(sink.failed) != 4 {
		t.Fatalf("expected compact failure event for each governed attempt, got %d", len(sink.failed))
	}
	if !strings.Contains(sink.failed[2], "context compact circuit open") {
		t.Fatalf("expected third failure to report circuit open, got %q", sink.failed[2])
	}
	if len(sink.blocked) != 0 {
		t.Fatalf("did not expect blocked events below threshold, got %+v", sink.blocked)
	}
}

func TestContextGovernorBlocksWhenCompactCircuitIsOpenAboveThreshold(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testBlockedContextWindowState(98000)
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{}, nil
	}, nil)
	governor.consecutiveCompactFailures = 3
	governor.compactCircuitOpen = true

	sink := &fakeContextEventSink{}
	_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      envelope.Messages,
		EnvelopeDebug: &envelope,
	}, sink)
	if err == nil || !errors.Is(err, errContextWindowBlocked) {
		t.Fatalf("expected context blocked error, got %v", err)
	}
	if len(sink.failed) != 1 || len(sink.blocked) != 1 {
		t.Fatalf("expected failed and blocked events, got failed=%v blocked=%v", sink.failed, sink.blocked)
	}
}

func TestContextGovernorRecordResponseUsageSetsLastAPIUsageTokens(t *testing.T) {
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testContextWindowState(2000)
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{}, nil
	}, nil)
	governor.RecordSentRequestEstimate(250)
	governor.RecordResponseUsage(321)

	state := governor.withRunState(testContextWindowState(2000))
	if state.LastAPIUsageTokens != 321 {
		t.Fatalf("expected last API usage tokens to be preserved in state, got %+v", state)
	}
	if state.LastSentEstimatedInputTokens != 250 {
		t.Fatalf("expected matching sent-request estimate to be preserved in state, got %+v", state)
	}
}

func TestUsageAnchorFromOlderEstimatorKeepsTruthWithoutOldCalibration(t *testing.T) {
	config := ModelConfig{Model: "model", ProfileID: "profile"}
	anchor := ContextWindowUsageAnchorFromModelConfig(config, 228782, 83157)
	anchor.EstimatorID = "awaken_leaf_v4"
	governor := newContextGovernor(nil, nil, nil)
	governor.RestoreUsageAnchor(anchor, config)
	if governor.lastAPIUsageTokens != 228782 || governor.lastSentEstimatedInputTokens != 0 {
		t.Fatal("old CJK underestimate was reused to inflate the new mandatory floor")
	}
	anchor.EstimatorID = contextEstimatorID
	governor.RestoreUsageAnchor(anchor, config)
	if governor.lastSentEstimatedInputTokens != 83157 {
		t.Fatal("current estimator lost its paired usage anchor")
	}
}

func TestConservativeProjectedTokensAnchorsProviderUsageToCurrentEstimate(t *testing.T) {
	state := testContextWindowState(14572)
	state.LastAPIUsageTokens = 20224
	state.LastSentEstimatedInputTokens = 16501

	if got := state.ConservativeProjectedTokens(); got != 20224 {
		t.Fatalf("ConservativeProjectedTokens=%d, want monotonic provider anchor 20224", got)
	}
}

func TestConservativeProjectedTokensDoesNotTreatProjectionPhaseMismatchAsShrink(t *testing.T) {
	state := testContextWindowState(146023)
	state.AutoCompactThreshold = 240576
	state.LastAPIUsageTokens = 254928
	state.LastSentEstimatedInputTokens = 162363
	state.EstimatedDeltaTokens = 143

	if got := state.ConservativeProjectedTokens(); got != 255071 {
		t.Fatalf("ConservativeProjectedTokens=%d, want provider anchor plus current delta 255071", got)
	}
	if !state.ShouldAutoCompactByActualUsage() {
		t.Fatalf("expected phase-mismatched projection to trigger proactive compact: %+v", state)
	}
}

func TestShouldBlockUsesProviderAnchoredProjection(t *testing.T) {
	state := testContextWindowState(80000)
	state.LastAPIUsageTokens = 100000
	state.LastSentEstimatedInputTokens = 84000

	if !state.ShouldBlock() {
		t.Fatalf("expected provider-anchored projection to cross blocking threshold: %+v", state)
	}
}

func TestShouldAutoCompactByActualUsage(t *testing.T) {
	cases := []struct {
		name     string
		state    ContextWindowState
		expected bool
	}{
		{
			name:     "no prior usage is inert",
			state:    testContextWindowStateWithActual(0, 5000),
			expected: false,
		},
		{
			name:     "v4 estimate below threshold and actual below threshold stays inert",
			state:    testContextWindowStateWithActual(80000, 5000),
			expected: false,
		},
		{
			name:     "actual plus delta crosses threshold even when v4 estimate is below",
			state:    testContextWindowStateWithActual(88000, 3000),
			expected: true,
		},
		{
			name:     "actual alone crosses threshold",
			state:    testContextWindowStateWithActual(91000, 0),
			expected: true,
		},
		{
			name: "delta overflow guards to over threshold",
			state: ContextWindowState{
				AutoCompactThreshold: 90000,
				LastAPIUsageTokens:   int(^uint(0)>>1) - 1,
				EstimatedDeltaTokens: 100,
			},
			expected: true,
		},
		{
			name: "zero threshold is inert",
			state: ContextWindowState{
				AutoCompactThreshold: 0,
				LastAPIUsageTokens:   100000,
				EstimatedDeltaTokens: 0,
			},
			expected: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.state.ShouldAutoCompactByActualUsage(); got != tc.expected {
				t.Fatalf("ShouldAutoCompactByActualUsage=%v, want %v (state=%+v)", got, tc.expected, tc.state)
			}
		})
	}
}

func testContextWindowStateWithActual(lastAPIUsage, delta int) ContextWindowState {
	state := testContextWindowState(2000)
	state.LastAPIUsageTokens = lastAPIUsage
	state.EstimatedDeltaTokens = delta
	return state
}

func TestPrepareRequestAutoCompactsByActualUsageEvenWhenV4EstimateIsBelowThreshold(t *testing.T) {
	originalMessages := []convcontext.Message{{Role: convcontext.RoleUser, Content: "context"}}
	compactedMessages := []convcontext.Message{{
		Role:              convcontext.RoleUser,
		Content:           "summary",
		CompactBoundaryID: "compact-actual-1",
	}}
	originalEnvelope := ContextEnvelope{Messages: originalMessages}
	compactedEnvelope := ContextEnvelope{Messages: compactedMessages}

	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			// v4 estimate stays well below the 90_000 auto-compact threshold so
			// ShouldAutoCompact alone would not fire. The governor must still
			// compact because LastAPIUsageTokens + EstimatedDeltaTokens crosses
			// the threshold.
			state := testContextWindowState(20000)
			state.LastAPIUsageTokens = 88000
			state.EstimatedDeltaTokens = 4000
			return state
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{
				BoundaryID:      "compact-actual-1",
				Messages:        compactedMessages,
				EstimatedTokens: 2000,
			}, nil
		},
	}

	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{
			Messages:      compactedMessages,
			EnvelopeDebug: &compactedEnvelope,
		}, nil
	}, func(_ stdcontext.Context, result CompactResult) error {
		return nil
	})
	// Simulate a prior request whose provider-reported usage is high enough
	// that, combined with the locally-estimated delta, the parallel trigger
	// fires even though the v4 estimate is below threshold.
	governor.RecordResponseUsage(88000)

	sink := &fakeContextEventSink{}
	req, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      originalMessages,
		EnvelopeDebug: &originalEnvelope,
	}, sink)
	if err != nil {
		t.Fatalf("PrepareRequest returned error: %v", err)
	}
	if engine.compactCalls != 1 {
		t.Fatalf("expected one compact attempt driven by actual usage, got %d", engine.compactCalls)
	}
	// Successful compact must reset the actual-usage trigger so the next
	// PrepareRequest does not loop on stale LastAPIUsageTokens.
	if governor.lastAPIUsageTokens != 0 {
		t.Fatalf("expected lastAPIUsageTokens reset after successful compact, got %d", governor.lastAPIUsageTokens)
	}
	if governor.lastSentEstimatedInputTokens != 0 {
		t.Fatalf("expected lastSentEstimatedInputTokens reset after successful compact, got %d", governor.lastSentEstimatedInputTokens)
	}
	if req.ContextWindowState.LastAPIUsageTokens != 0 {
		t.Fatalf("expected rebuilt state to reflect reset lastAPIUsageTokens, got %d", req.ContextWindowState.LastAPIUsageTokens)
	}
}

func TestPrepareRequestDoesNotCompactWhenBothEstimateAndActualAreBelowThreshold(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "small"}},
	}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			state := testContextWindowState(20000)
			state.LastAPIUsageTokens = 30000
			state.EstimatedDeltaTokens = 1000
			return state
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{}, nil
	}, nil)
	governor.RecordResponseUsage(30000)

	sink := &fakeContextEventSink{}
	_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      envelope.Messages,
		EnvelopeDebug: &envelope,
	}, sink)
	if err != nil {
		t.Fatalf("PrepareRequest returned error: %v", err)
	}
	if engine.compactCalls != 0 {
		t.Fatalf("expected no compact when both estimate and actual are below threshold, got %d", engine.compactCalls)
	}
}

func testContextWindowState(estimatedTokens int) ContextWindowState {
	return ContextWindowState{
		ModelWindow:          116000,
		OutputReserve:        16000,
		EffectiveWindow:      100000,
		EstimatedTokens:      estimatedTokens,
		WarningThreshold:     85000,
		AutoCompactThreshold: 90000,
		CompactTarget:        10000,
	}
}

func testBlockedContextWindowState(estimatedTokens int) ContextWindowState {
	state := testContextWindowState(estimatedTokens)
	state.EstimatedTokens = estimatedTokens
	return state
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestCompactRetainedTailBudgetTargetsWholeRequest(t *testing.T) {
	// 40_000 单字符正文按 chars/4 估算为 10_000 token 的对话部分。
	messages := []convcontext.Message{{Role: convcontext.RoleUser, Content: strings.Repeat("a", 40_000)}}

	t.Run("subtracts stable prefix overhead and summary reserve", func(t *testing.T) {
		state := ContextWindowState{CompactTarget: 75_000, EstimatedTokens: 43_227}
		if got := compactRetainedTailBudget(state, messages); got != 33_581 {
			t.Fatalf("budget=%d, want 75000-33227-4096-4096=33581", got)
		}
	})

	t.Run("deflates by provider anchor when the provider counted more", func(t *testing.T) {
		state := ContextWindowState{
			CompactTarget: 75_000, EstimatedTokens: 20_000,
			LastAPIUsageTokens: 200_000, LastSentEstimatedInputTokens: 100_000,
		}
		if got := compactRetainedTailBudget(state, messages); got != 19_308 {
			t.Fatalf("budget=%d, want 75000/2-10000-4096-4096=19308", got)
		}
	})

	t.Run("deflates summary reserve when calibrated fixed overhead is tight", func(t *testing.T) {
		state := ContextWindowState{
			CompactTarget: 27_200, EstimatedTokens: 24_000,
			LastAPIUsageTokens: 336_000, LastSentEstimatedInputTokens: 192_000,
		}
		messages := []convcontext.Message{{Role: convcontext.RoleUser, Content: strings.Repeat("a", 40_000)}}
		if got := compactSummaryBudget(state, messages); got != 1_024 {
			t.Fatalf("summary budget=%d, want calibrated minimum 1024", got)
		}
	})

	t.Run("never inflates when the provider counted less", func(t *testing.T) {
		state := ContextWindowState{
			CompactTarget: 75_000, EstimatedTokens: 20_000,
			LastAPIUsageTokens: 90_000, LastSentEstimatedInputTokens: 100_000,
		}
		if got := compactRetainedTailBudget(state, messages); got != 56_808 {
			t.Fatalf("budget=%d, want 75000-10000-4096-4096=56808", got)
		}
	})

	t.Run("clamps to one token instead of flipping to a default window", func(t *testing.T) {
		state := ContextWindowState{CompactTarget: 9_000, EstimatedTokens: 30_000}
		if got := compactRetainedTailBudget(state, messages); got != 1 {
			t.Fatalf("budget=%d, want 1", got)
		}
	})

	t.Run("passes non-positive targets through unchanged", func(t *testing.T) {
		if got := compactRetainedTailBudget(ContextWindowState{}, messages); got != 0 {
			t.Fatalf("budget=%d, want 0", got)
		}
	})
}

func TestContextGovernorHandsWholeRequestBudgetToCompactor(t *testing.T) {
	// 10 条 36_000 字符正文各 ≈ 9_000 token，对话共 90_000 token，
	// 总估算 92_000，因此稳定前缀开销为 2_000。
	originalMessages := make([]convcontext.Message, 0, 10)
	for i := 0; i < 10; i++ {
		originalMessages = append(originalMessages, convcontext.Message{
			Role: convcontext.RoleUser, Content: strings.Repeat("a", 36_000),
		})
	}
	compactedMessages := []convcontext.Message{{
		Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact-budget",
	}}
	originalEnvelope := ContextEnvelope{Messages: originalMessages}
	compactedEnvelope := ContextEnvelope{Messages: compactedMessages}

	var seenTargets []int
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			if len(envelope.Messages) > 0 && envelope.Messages[0].CompactBoundaryID != "" {
				return testContextWindowState(5400)
			}
			return testContextWindowState(92_000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			seenTargets = append(seenTargets, req.TargetTokens)
			return CompactResult{
				BoundaryID:     "compact-budget",
				SummaryMessage: convcontext.Message{Role: convcontext.RoleUser, Content: "summary"},
				Messages:       compactedMessages,
			}, nil
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{Messages: compactedMessages, EnvelopeDebug: &compactedEnvelope}, nil
	}, func(_ stdcontext.Context, result CompactResult) error { return nil })
	// 上一请求供应商实报是发送时估算的两倍：预算按 2 倍通缩。
	governor.RecordSentRequestEstimate(80_000)
	governor.RecordResponseUsage(160_000)

	if _, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      originalMessages,
		EnvelopeDebug: &originalEnvelope,
	}, &fakeContextEventSink{}); err != nil {
		t.Fatalf("PrepareRequest returned error: %v", err)
	}
	// CompactTarget 10_000 ×(80_000/160_000) − 前缀开销 2_000 − 校准后动态摘要预算 1_050
	// − checkpoint projection reserve 1_250，剩余 tail 700 tokens。
	if len(seenTargets) == 0 || seenTargets[0] != 700 {
		t.Fatalf("compactor received %v, want first target 700", seenTargets)
	}
}

func TestContextGovernorRejectsCompactThatStaysAboveAutoThreshold(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	applyCalled := 0
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testBlockedContextWindowState(98000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{
				BoundaryID:     "compact-stale",
				SummaryMessage: convcontext.Message{Role: convcontext.RoleUser, Content: "summary"},
				Messages: []convcontext.Message{{
					Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact-stale",
				}},
				EstimatedTokens: 95000,
			}, nil
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{Messages: envelope.Messages, EnvelopeDebug: &envelope}, nil
	}, func(_ stdcontext.Context, result CompactResult) error {
		applyCalled++
		return nil
	})
	sink := &fakeContextEventSink{}
	_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      envelope.Messages,
		EnvelopeDebug: &envelope,
	}, sink)
	if err == nil || !errors.Is(err, errStillAboveAutoThreshold) {
		t.Fatalf("expected still-above-auto postcondition failure, got %v", err)
	}
	if applyCalled != 0 {
		t.Fatalf("failed compact must not apply a new epoch, applyCalled=%d", applyCalled)
	}
	if governor.consecutiveCompactFailures != 1 || governor.lastCompactFailureReason != compactFailureStillAboveAutoThreshold {
		t.Fatalf("failure state = failures=%d reason=%q", governor.consecutiveCompactFailures, governor.lastCompactFailureReason)
	}
	if engine.compactCalls != 2 {
		t.Fatalf("expected one retry with a lighter tail, got %d attempts", engine.compactCalls)
	}
	if len(sink.blocked) != 0 {
		t.Fatalf("must not emit CONTEXT_BLOCKED before three failures, got %v", sink.blocked)
	}
}

func TestContextGovernorRejectsCompactAboveHardTargetEvenBelowAutoThreshold(t *testing.T) {
	envelope := ContextEnvelope{Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: strings.Repeat("x", 20_000)}}}
	engine := &fakeContextEngine{
		evaluateFn: func(value ContextEnvelope) ContextWindowState {
			state := testContextWindowState(20_000)
			state.HardCompactTarget = true
			return state
		},
	}
	governor := newContextGovernor(engine, nil, nil)
	preState := testContextWindowState(80_000)
	preState.HardCompactTarget = true
	summary := convcontext.Message{Role: convcontext.RoleUser, Content: "summary", CompactBoundaryID: "compact_hard_post"}
	err := governor.evaluateCompactPostcondition(
		LLMRequest{Messages: envelope.Messages, EnvelopeDebug: &envelope}, preState,
		CompactResult{Mode: CompactModeAuto, BoundaryID: "compact_hard_post", SummaryMessage: summary, Messages: []convcontext.Message{summary}},
	)
	if err == nil || !errors.Is(err, ErrCompactTargetUnachievable) {
		t.Fatalf("expected hard-target failure, got %v", err)
	}
}

func TestCompactTargetUnachievableOpensCircuitImmediately(t *testing.T) {
	governor := newContextGovernor(&fakeContextEngine{}, nil, nil)
	err := &CompactTargetUnachievableError{Reason: "mandatory_floor_exceeds_target", Projected: 29237, Target: 27704, MandatoryFloor: 29237}

	governor.recordCompactFailure(err)

	if !governor.compactCircuitOpen {
		t.Fatal("compact target unachievable must open the circuit immediately")
	}
	if governor.consecutiveCompactFailures != 1 {
		t.Fatalf("consecutiveCompactFailures=%d, want 1", governor.consecutiveCompactFailures)
	}
	if governor.lastCompactFailureClass != "budget_unresolved" {
		t.Fatalf("lastCompactFailureClass=%q, want budget_unresolved", governor.lastCompactFailureClass)
	}
}

func TestPostProjectionTargetFailureUsesBudgetFamilyBeforeCircuit(t *testing.T) {
	governor := newContextGovernor(&fakeContextEngine{}, nil, nil)
	err := &CompactTargetUnachievableError{
		Reason: "post_compact_projection_exceeds_target", Projected: 29_237,
		Target: 27_704, MandatoryFloor: 21_000,
	}

	governor.recordCompactFailure(err)

	if governor.compactCircuitOpen {
		t.Fatal("a reducible post-projection miss must not open the circuit after one failure")
	}
	if governor.consecutiveCompactFailures != 1 || governor.lastCompactFailureClass != "budget_unresolved" {
		t.Fatalf("unexpected failure state: count=%d class=%q", governor.consecutiveCompactFailures, governor.lastCompactFailureClass)
	}
}

func TestContextGovernorPreflightMandatoryFloorSkipsSummaryModelCall(t *testing.T) {
	originalMessages := []convcontext.Message{
		{Role: convcontext.RoleUser, Content: strings.Repeat("old context ", 4_000)},
		{Role: convcontext.RoleAssistant, Content: "ack"},
		{Role: convcontext.RoleUser, Content: "latest tail"},
	}
	originalEnvelope := ContextEnvelope{Messages: originalMessages}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			if len(envelope.Messages) > 0 && envelope.Messages[0].CompactBoundaryID == "compact-preflight" {
				state := testContextWindowState(12_500)
				state.HardCompactTarget = true
				return state
			}
			state := testContextWindowState(96_000)
			state.HardCompactTarget = true
			return state
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			t.Fatalf("compact summary model must not be called when preflight mandatory floor already exceeds target")
			return CompactResult{}, nil
		},
	}
	governor := newContextGovernor(engine, nil, nil)
	sink := &fakeContextEventSink{}

	_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      originalMessages,
		EnvelopeDebug: &originalEnvelope,
	}, sink)
	if err == nil || !errors.Is(err, errContextWindowBlocked) {
		t.Fatalf("expected preflight compact target failure, got %v", err)
	}
	if engine.compactCalls != 0 {
		t.Fatalf("mandatory-floor preflight must skip summary model call, got %d compact calls", engine.compactCalls)
	}
	if len(sink.failed) != 1 || !strings.Contains(sink.failed[0], "pre_compact_mandatory_floor_exceeds_target") {
		t.Fatalf("expected failed event to expose preflight target reason, got %+v", sink.failed)
	}
	if len(sink.blocked) != 1 || !strings.Contains(sink.blocked[0], "COMPACT_TARGET_UNACHIEVABLE") {
		t.Fatalf("expected context blocked after immediate circuit open, got %+v", sink.blocked)
	}
}

func TestContextGovernorOpensCircuitAndBlocksAfterThreeFailures(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testBlockedContextWindowState(98000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{}, errOversizedProtectedTail
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{Messages: envelope.Messages, EnvelopeDebug: &envelope}, nil
	}, func(_ stdcontext.Context, result CompactResult) error {
		t.Fatal("applyCompactResult must not run after oversized tail failure")
		return nil
	})
	sink := &fakeContextEventSink{}
	for i := 0; i < 2; i++ {
		_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
			Messages:      envelope.Messages,
			EnvelopeDebug: &envelope,
		}, sink)
		if !errors.Is(err, errOversizedProtectedTail) {
			t.Fatalf("attempt %d: expected oversized tail, got %v", i+1, err)
		}
		if errors.Is(err, errContextWindowBlocked) {
			t.Fatalf("attempt %d blocked too early: %v", i+1, err)
		}
		if len(sink.blocked) != 0 {
			t.Fatalf("attempt %d emitted CONTEXT_BLOCKED: %v", i+1, sink.blocked)
		}
	}
	_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      envelope.Messages,
		EnvelopeDebug: &envelope,
	}, sink)
	if !errors.Is(err, errContextWindowBlocked) {
		t.Fatalf("third failure should open the circuit and block, got %v", err)
	}
	if !governor.compactCircuitOpen || governor.consecutiveCompactFailures < 3 {
		t.Fatalf("expected circuit open after 3 failures, got failures=%d open=%v", governor.consecutiveCompactFailures, governor.compactCircuitOpen)
	}
	if isPromptTooLongError(err) {
		t.Fatalf("CONTEXT_BLOCKED must not be classified as prompt-too-long")
	}
}

func TestTryReactiveCompactSkipsSameOversizedTail(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testContextWindowState(92000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			t.Fatal("reactive compact must not run after an oversized-tail auto failure")
			return CompactResult{}, nil
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{Messages: envelope.Messages, EnvelopeDebug: &envelope}, nil
	}, nil)
	governor.lastCompactFailureReason = compactFailureOversizedProtectedTail
	ok, err := governor.TryReactiveCompact(stdcontext.Background(), &fakeContextEventSink{})
	if ok || !errors.Is(err, errOversizedProtectedTail) {
		t.Fatalf("expected oversized tail to suppress reactive compact, ok=%v err=%v", ok, err)
	}
	if engine.compactCalls != 0 {
		t.Fatalf("reactive compact wrote another attempt: %d", engine.compactCalls)
	}
}

func TestContextGovernorFallbackSummaryAppliesWhenWindowDrops(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	compactedMessages := []convcontext.Message{{
		Role: convcontext.RoleUser, Content: "[COMPACT_FALLBACK:llm_summary_failed] host recovered user ask", CompactBoundaryID: "compact-fallback",
	}}
	compactedEnvelope := ContextEnvelope{Messages: compactedMessages}
	applyCalled := 0
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			if len(envelope.Messages) > 0 && envelope.Messages[0].CompactBoundaryID != "" {
				return testContextWindowState(4000)
			}
			return testBlockedContextWindowState(98000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{
				BoundaryID:     "compact-fallback",
				SummaryMessage: compactedMessages[0],
				Messages:       compactedMessages,
				Fallback:       CompactFallback{Used: true, Marker: "[COMPACT_FALLBACK:llm_summary_failed]", Reason: "provider unavailable"},
			}, nil
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{Messages: compactedMessages, EnvelopeDebug: &compactedEnvelope}, nil
	}, func(_ stdcontext.Context, result CompactResult) error {
		applyCalled++
		return nil
	})
	req, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      envelope.Messages,
		EnvelopeDebug: &envelope,
	}, &fakeContextEventSink{})
	if err != nil {
		t.Fatalf("installable fallback that drops below block must succeed, got %v", err)
	}
	if applyCalled != 1 {
		t.Fatalf("expected fallback compact to apply a new epoch, applyCalled=%d", applyCalled)
	}
	if engine.compactCalls != 1 {
		t.Fatalf("successful fallback should stop the current compact cycle, got %d attempts", engine.compactCalls)
	}
	if len(req.Messages) != 1 || req.Messages[0].CompactBoundaryID != "compact-fallback" {
		t.Fatalf("expected compacted conversation after fallback install, got %+v", req.Messages)
	}
	if governor.consecutiveCompactFailures != 0 || governor.compactCircuitOpen {
		t.Fatalf("successful fallback must not count as a compact failure, failures=%d open=%v", governor.consecutiveCompactFailures, governor.compactCircuitOpen)
	}
}

func TestContextGovernorRecoverableCompactFailureAtBlockContinuesUntilCircuitOpens(t *testing.T) {
	envelope := ContextEnvelope{
		Messages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "large context"}},
	}
	engine := &fakeContextEngine{
		evaluateFn: func(envelope ContextEnvelope) ContextWindowState {
			return testBlockedContextWindowState(98000)
		},
		compactFn: func(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
			return CompactResult{
				Fallback: CompactFallback{Used: true, Reason: "compact summary has no final text"},
			}, nil
		},
	}
	governor := newContextGovernor(engine, func() (LLMRequest, error) {
		return LLMRequest{Messages: envelope.Messages, EnvelopeDebug: &envelope}, nil
	}, func(_ stdcontext.Context, result CompactResult) error {
		t.Fatal("applyCompactResult must not run after a recoverable summary failure")
		return nil
	})
	sink := &fakeContextEventSink{}
	for i := 0; i < 2; i++ {
		req, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
			Messages:      envelope.Messages,
			EnvelopeDebug: &envelope,
		}, sink)
		if err != nil {
			t.Fatalf("attempt %d: recoverable compact failure must not abort the run, got %v", i+1, err)
		}
		if errors.Is(err, errContextWindowBlocked) {
			t.Fatalf("attempt %d blocked too early: %v", i+1, err)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("attempt %d replaced conversation: %+v", i+1, req.Messages)
		}
		if len(sink.blocked) != 0 {
			t.Fatalf("attempt %d emitted CONTEXT_BLOCKED: %v", i+1, sink.blocked)
		}
	}
	_, err := governor.PrepareRequest(stdcontext.Background(), LLMRequest{
		Messages:      envelope.Messages,
		EnvelopeDebug: &envelope,
	}, sink)
	if !errors.Is(err, errContextWindowBlocked) {
		t.Fatalf("third recoverable failure should open the circuit and block, got %v", err)
	}
	if !governor.compactCircuitOpen || governor.consecutiveCompactFailures < 3 {
		t.Fatalf("expected circuit open after 3 failures, got failures=%d open=%v", governor.consecutiveCompactFailures, governor.compactCircuitOpen)
	}
}

func TestContextGovernorTreatsMixedBudgetFailuresAsOneCircuitFamily(t *testing.T) {
	governor := newContextGovernor(&fakeContextEngine{}, nil, nil)

	governor.recordCompactFailure(fmt.Errorf("%w: post=95000 pre=98000", errInsufficientReduction))
	if governor.consecutiveCompactFailures != 1 || governor.lastCompactFailureClass != "budget_unresolved" {
		t.Fatalf("first budget failure not tracked as a family: failures=%d class=%q reason=%q", governor.consecutiveCompactFailures, governor.lastCompactFailureClass, governor.lastCompactFailureReason)
	}

	governor.recordCompactFailure(fmt.Errorf("%w: post=93000 auto=91000", errStillAboveAutoThreshold))
	governor.recordCompactFailure(fmt.Errorf("%w: post=97000 block=96000", errStillAboveBlockThreshold))
	if !governor.compactCircuitOpen || governor.consecutiveCompactFailures != 3 {
		t.Fatalf("mixed budget failures should still open the circuit, got failures=%d open=%v", governor.consecutiveCompactFailures, governor.compactCircuitOpen)
	}
	if governor.lastCompactFailureReason != compactFailureStillAboveBlockThreshold {
		t.Fatalf("last compact failure reason should still expose the specific failure, got %q", governor.lastCompactFailureReason)
	}
}

func TestShouldRetryCompactPostconditionStopsOnMandatoryFloorFailure(t *testing.T) {
	if shouldRetryCompactPostcondition(nil, CompactResult{}, 0) {
		t.Fatal("nil postcondition error must not retry")
	}
	if shouldRetryCompactPostcondition(errInsufficientReduction, CompactResult{Fallback: CompactFallback{Used: true}}, 0) {
		t.Fatal("insufficient reduction must not trigger another compact-summary request")
	}
	if !shouldRetryCompactPostcondition(fmt.Errorf("%w: post=95000 auto=91000", errStillAboveAutoThreshold), CompactResult{Fallback: CompactFallback{Used: true}}, 0) {
		t.Fatal("fallback that still sits above auto should allow one lighter-tail retry")
	}
	if shouldRetryCompactPostcondition(&CompactTargetUnachievableError{
		Reason: "mandatory_floor_exceeds_target", Projected: 29237, Target: 27704, MandatoryFloor: 29237,
	}, CompactResult{}, 0) {
		t.Fatal("mandatory floor above target must stop inner retries")
	}
	if !shouldRetryCompactPostcondition(&CompactTargetUnachievableError{
		Reason: "post_compact_projection_exceeds_target", Projected: 29237, Target: 27704, MandatoryFloor: 21000,
	}, CompactResult{}, 0) {
		t.Fatal("a reducible post-projection miss should allow one lighter-tail retry")
	}
	if !shouldRetryCompactPostcondition(fmt.Errorf("%w: post=95000 auto=91000", errStillAboveAutoThreshold), CompactResult{}, 0) {
		t.Fatal("retryable postcondition should still allow one lighter-tail retry")
	}
	if shouldRetryCompactPostcondition(fmt.Errorf("%w: post=95000 auto=91000", errStillAboveAutoThreshold), CompactResult{}, 1) {
		t.Fatal("retryable postcondition should stop after one lighter-tail retry")
	}
}
