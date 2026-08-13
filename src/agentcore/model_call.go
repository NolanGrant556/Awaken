package agentcore

import (
	stdcontext "context"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

// ModelCallAuthority selects exactly one durable accounting scope. Thread
// calls use SemanticJournal; Dream and profile connection tests use their own
// authorities and therefore do not call these helpers.
type ModelCallAuthority struct {
	ThreadID         string
	RunID            string
	TurnID           string
	SessionID        string
	Recorder         SemanticRecorder
	Sink             EventSink
	HookInvocationID string
}

func (a ModelCallAuthority) valid() bool {
	return strings.TrimSpace(a.ThreadID) != "" && strings.TrimSpace(a.RunID) != "" && a.Recorder != nil
}

func prepareRecordedModelCall(
	ctx stdcontext.Context,
	authority ModelCallAuthority,
	cfg llm.Config,
	req llm.Request,
	stream bool,
	retryIndex int,
) (llm.PreparedRequest, error) {
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = newAwakenID("model_request")
	}
	prepared, err := llm.PrepareRequest(cfg, req, stream)
	if err != nil {
		return llm.PreparedRequest{}, err
	}
	if !authority.valid() {
		return prepared, nil
	}
	record := AuxiliaryModelRequestSemanticRecord(
		authority.ThreadID, authority.RunID, authority.TurnID, authority.SessionID,
		prepared.RequestID, req.Purpose, firstNonEmptyModelValue(req.Model, cfg.Model), string(prepared.Protocol),
		prepared.RequestShapeDigest, convcontext.EstimateTokens(string(prepared.Body)), retryIndex,
	)
	if strings.TrimSpace(authority.HookInvocationID) != "" {
		record.Payload["hook_invocation_id"] = authority.HookInvocationID
	}
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := authority.Recorder.Append(persistCtx, record); err != nil {
		return llm.PreparedRequest{}, err
	}
	if err := authority.Recorder.Flush(persistCtx); err != nil {
		return llm.PreparedRequest{}, err
	}
	return prepared, nil
}

func persistRecordedModelUsage(ctx stdcontext.Context, authority ModelCallAuthority, usage ModelUsageRecord) error {
	if !authority.valid() {
		return nil
	}
	emitter := newRunEventEmitter(authority.ThreadID, authority.Sink)
	emitter.runID = authority.RunID
	emitter.sessionID = authority.SessionID
	emitter.recorder = authority.Recorder
	emitter.turnState.TurnID = TurnID(authority.TurnID)
	emitter.hookInvocationID = authority.HookInvocationID
	return emitter.modelUsage(ctx, usage)
}

// InvokeRecordedModel performs one non-streaming exact-body call and records
// request-before-side-effect plus its terminal usage under the same request ID.
func InvokeRecordedModel(
	ctx stdcontext.Context,
	authority ModelCallAuthority,
	cfg llm.Config,
	req llm.Request,
	retryIndex int,
	auxiliaryFallback bool,
) (*llm.Response, error) {
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = newAwakenID("model_request")
	}
	prepared, err := prepareRecordedModelCall(ctx, authority, cfg, req, false, retryIndex)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	response, callErr := llm.InvokePrepared(ctx, cfg, prepared)
	usage := modelUsageRecord(prepared.RequestID, firstNonEmptyModelValue(req.Model, cfg.Model), string(prepared.Protocol), cfg, req.Purpose, prepared, started, response, callErr, ctx.Err())
	usage.RetryIndex = retryIndex
	usage.AuxiliaryFallback = auxiliaryFallback
	if persistErr := persistRecordedModelUsage(ctx, authority, usage); persistErr != nil {
		return response, persistErr
	}
	return response, callErr
}

// StreamRecordedModel is the streaming counterpart used by compact summary.
func StreamRecordedModel(
	ctx stdcontext.Context,
	authority ModelCallAuthority,
	cfg llm.Config,
	req llm.Request,
	observer llm.StreamObserver,
	retryIndex int,
	auxiliaryFallback bool,
) (*llm.Response, error) {
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = newAwakenID("model_request")
	}
	prepared, err := prepareRecordedModelCall(ctx, authority, cfg, req, true, retryIndex)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	response, callErr := llm.StreamPreparedWithObserver(ctx, cfg, prepared, observer)
	usage := modelUsageRecord(prepared.RequestID, firstNonEmptyModelValue(req.Model, cfg.Model), string(prepared.Protocol), cfg, req.Purpose, prepared, started, response, callErr, ctx.Err())
	usage.RetryIndex = retryIndex
	usage.AuxiliaryFallback = auxiliaryFallback
	if persistErr := persistRecordedModelUsage(ctx, authority, usage); persistErr != nil {
		return response, persistErr
	}
	return response, callErr
}

func firstNonEmptyModelValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
