package agentcore

import (
	stdcontext "context"
	"errors"
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

func (s *fakeContextEventSink) emitCompactFailed(state ContextWindowState, mode CompactMode, message string, failureReason string) {
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
	governor.RecordResponseUsage(321)

	state := governor.withRunState(testContextWindowState(2000))
	if state.LastAPIUsageTokens != 321 {
		t.Fatalf("expected last API usage tokens to be preserved in state, got %+v", state)
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
		CompactTarget:        30000,
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
