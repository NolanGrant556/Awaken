package agentcore

import (
	"awaken/src/context"
	"awaken/src/llm"
	stdcontext "context"
	"strings"
	"time"
)

func (r *Runner) streamLLMWithTransientRetry(
	ctx stdcontext.Context,
	events *runEventEmitter,
	stream func(stdcontext.Context, *runEventEmitter) (*llm.Response, error),
) (*llm.Response, error) {
	connectionDelay := initialConnectionRetryDelay
	connectionAttempts := 0
	transientAttempts := 0
	emittedReconnect := false
	for {
		response, err := stream(ctx, events)
		if err == nil {
			if emittedReconnect {
				events.providerReconnectRecovered()
			}
			return response, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		kind, reset := llmRetryKind(err, events)
		if kind == llmRetryNone {
			return response, err
		}
		emittedReconnect = true
		if kind == llmRetryConnection {
			if reset {
				events.discardEphemeralStreamOutput()
			}
			connectionAttempts++
			events.providerReconnect(ProviderReconnectWaiting, connectionAttempts, 0, err, reset)
			if err := r.sleepLLMRetry(ctx, connectionDelay); err != nil {
				return nil, err
			}
			if connectionDelay < maxConnectionRetryDelay {
				connectionDelay *= 2
				if connectionDelay > maxConnectionRetryDelay {
					connectionDelay = maxConnectionRetryDelay
				}
			}
			continue
		}
		if transientAttempts >= maxTransientLLMRetries {
			if reset {
				events.discardEphemeralStreamOutput()
				events.providerReconnect(
					ProviderReconnectRetrying,
					maxTransientLLMRetries,
					maxTransientLLMRetries,
					err,
					true,
				)
			}
			return response, err
		}
		if reset {
			events.discardEphemeralStreamOutput()
		}
		transientAttempts++
		events.providerReconnect(ProviderReconnectRetrying, transientAttempts, maxTransientLLMRetries, err, reset)
		delay, ok := llm.RequestRetryAfter(err)
		if !ok {
			delay = time.Duration(transientAttempts) * time.Second
		}
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		if err := r.sleepLLMRetry(ctx, delay); err != nil {
			return nil, err
		}
	}
}

const (
	llmRetryNone       = ""
	llmRetryConnection = "connection"
	llmRetryTransient  = "transient"
)

func llmRetryKind(err error, events *runEventEmitter) (kind string, reset bool) {
	if events != nil && events.toolStreamStarted {
		return llmRetryNone, false
	}
	reset = events != nil && events.hasEphemeralStreamOutput()
	if llm.IsConnectionError(err) {
		return llmRetryConnection, reset
	}
	if llm.IsTransientRequestError(err) {
		return llmRetryTransient, reset
	}
	return llmRetryNone, false
}

func (r *Runner) sleepLLMRetry(ctx stdcontext.Context, delay time.Duration) error {
	sleep := r.llmRetrySleep
	if sleep == nil {
		sleep = sleepWithContext
	}
	return sleep(ctx, delay)
}

func sleepWithContext(ctx stdcontext.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Runner) streamLLM(ctx stdcontext.Context, events *runEventEmitter) (*llm.Response, error) {
	llmReq, err := r.prepareLLMRequest(ctx, events)
	if err != nil {
		return nil, err
	}
	requestID := newAwakenID("model_request")
	if events != nil && events.logicalModelCallID == "" {
		events.logicalModelCallID = newAwakenID("model_call")
	}
	config := r.transportConfig(llmReq)
	messages := make([]context.Message, 0, len(llmReq.PrefixMessages)+len(llmReq.Messages))
	messages = append(messages, cloneContextMessages(llmReq.PrefixMessages)...)
	messages = append(messages, cloneContextMessages(llmReq.Messages)...)
	request := llm.Request{
		RequestID: requestID, SystemPrompt: llmReq.SystemPrompt, Messages: messages,
		Tools: llmReq.Tools, Model: llmReq.Model, Temperature: llmReq.Temperature,
		MaxTokens: llmReq.MaxTokens, Purpose: r.modelRequestPurpose(),
		AffinityScopeID:       agentAffinityScope(r.threadID, r.config),
		ReasoningPolicy:       r.config.ModelConfig.ReasoningEffort,
		NativeCompactionState: nativeCompactionStateFromBundle(r.config.ModelConfig, r.config.RestoredContextBundleV2),
	}
	prepared, err := llm.PrepareRequest(config, request, true)
	if err != nil {
		return nil, err
	}
	protocol := string(prepared.Protocol)
	snapshot := buildContextUsageSnapshot(llmReq, "sent_request", requestID, protocol, prepared.RequestShapeDigest, prepared.Body)
	llmReq.ContextWindowState.EstimatedTokens = snapshot.EstimatedInputTokens
	llmReq.ContextWindowState.UsageSnapshot = &snapshot
	llmReq.ContextUsage = snapshot
	if events != nil {
		if err := events.modelRequest(ctx, snapshot, request.Purpose); err != nil {
			return nil, err
		}
	}
	r.contextGovernor.RecordSentRequestEstimate(snapshot.EstimatedInputTokens)
	observer := llm.StreamObserver{}
	if events != nil {
		observer.OnContent = llm.StreamCallback(events.onLLMDelta)
		observer.OnToolCallDelta = events.onToolCallArgsDelta
		observer.OnEvent = events.onLLMStreamEvent
	}
	started := time.Now()
	response, streamErr := llm.StreamPreparedWithObserver(ctx, config, prepared, observer)
	if response != nil {
		if response.ProviderOpaqueState == nil && prepared.Cache.ReasoningReplay && strings.TrimSpace(response.Thinking) != "" {
			revision := config.CredentialRevision
			if revision == 0 {
				revision = 1
			}
			response.ProviderOpaqueState = &context.ProviderOpaqueMessageState{
				ProviderID: strings.ToLower(strings.TrimSpace(config.ProviderID)), ProfileID: strings.TrimSpace(config.ProfileID),
				CredentialRevision: revision, ModelID: llmReq.Model, ProtocolFamily: protocol,
				EndpointContractID: prepared.Cache.EndpointContract, Content: response.Thinking,
			}
		}
		if response.ProviderOpaqueState != nil && events != nil {
			if persistErr := events.providerOpaqueState(ctx, response.ProviderOpaqueState); persistErr != nil {
				return nil, persistErr
			}
		}
	}
	usage := modelUsageRecord(requestID, llmReq.Model, protocol, config, request.Purpose, prepared, started, response, streamErr, ctx.Err())
	if events != nil {
		usage.LogicalCallID, usage.RetryIndex = events.logicalModelCallID, events.transportAttempt
		events.transportAttempt++
	}
	sentEstimatedInputTokens := snapshot.EstimatedInputTokens
	usage.SentEstimatedInputTokens = &sentEstimatedInputTokens
	usage.InputEstimatorID = snapshot.EstimatorID
	if llmReq.ContextEpoch != nil {
		usage.ContextEpochID = llmReq.ContextEpoch.EpochID
		usage.PrefixDigest = llmReq.ContextEpoch.BaselineDigest
		usage.InvalidationReason = llmReq.ContextEpoch.BoundaryReason
	}
	if events != nil {
		if persistErr := events.modelUsage(ctx, usage); persistErr != nil {
			return nil, persistErr
		}
	}
	return response, streamErr
}

func (r *Runner) modelRequestPurpose() llm.RequestPurpose {
	return effectiveModelRequestPurpose(r.config)
}

func effectiveModelRequestPurpose(config AgentConfig) llm.RequestPurpose {
	purpose := config.ModelRequestPurpose
	if purpose == "" {
		return llm.RequestPurposeAgentTurn
	}
	return purpose
}

func (r *Runner) transportConfig(llmReq LLMRequest) llm.Config {
	return transportConfigForAgentConfig(r.config, llmReq)
}

func transportConfigForAgentConfig(config AgentConfig, llmReq LLMRequest) llm.Config {
	return llm.Config{
		APIKey:              config.ModelConfig.APIKey,
		BaseURL:             config.ModelConfig.BaseURL,
		InferenceEndpoint:   config.ModelConfig.InferenceEndpoint,
		Model:               llmReq.Model,
		CapabilityModelID:   config.ModelConfig.CapabilityModelID,
		EndpointSelector:    config.ModelConfig.EndpointSelector,
		ModelSelectorKind:   config.ModelConfig.ModelSelectorKind,
		Protocol:            llm.Protocol(config.ModelConfig.Protocol),
		ProviderID:          config.ModelConfig.ProviderID,
		ProfileID:           config.ModelConfig.ProfileID,
		CredentialRevision:  config.ModelConfig.CredentialRevision,
		Temperature:         llmReq.Temperature,
		MaxTokens:           llmReq.MaxTokens,
		AuthScheme:          config.ModelConfig.AuthScheme,
		SupportsPromptCache: config.ModelConfig.SupportsPromptCache,
		ReasoningEffort:     config.ModelConfig.ReasoningEffort,
	}
}

func modelUsageRecord(requestID, model, protocol string, config llm.Config, purpose llm.RequestPurpose, prepared llm.PreparedRequest, started time.Time, response *llm.Response, callErr, contextErr error) ModelUsageRecord {
	credentialRevision := config.CredentialRevision
	if credentialRevision == 0 {
		credentialRevision = 1
	}
	selectorKind := strings.ToLower(strings.TrimSpace(config.ModelSelectorKind))
	capabilityModelID := strings.TrimSpace(config.CapabilityModelID)
	if capabilityModelID == "" && selectorKind != "endpoint" {
		capabilityModelID = model
	}
	endpointSelector := strings.TrimSpace(config.EndpointSelector)
	if endpointSelector == "" && selectorKind == "endpoint" {
		endpointSelector = model
	}
	record := ModelUsageRecord{
		SchemaVersion: 1, RequestID: requestID, Purpose: string(purpose),
		ProviderID: config.ProviderID, ProfileID: config.ProfileID, CredentialRevision: credentialRevision,
		ModelID: model, ProtocolFamily: protocol, EndpointContractID: prepared.Cache.EndpointContract,
		CapabilityModelID: capabilityModelID, EndpointSelector: endpointSelector,
		ModelSelectorKind: selectorKind,
		ReasoningPolicy:   normalizedModelUsageReasoningPolicy(config, purpose),
		CacheMode:         string(prepared.Cache.Mode), CacheKeyApplied: prepared.Cache.KeyApplied,
		LatencyMS: time.Since(started).Milliseconds(),
		Status:    ModelUsageSucceeded, CompletedAt: time.Now().UTC(),
	}
	if prepared.ReasoningPolicy != "" {
		record.ReasoningPolicy = prepared.ReasoningPolicy
	}
	if prepared.Timing != nil {
		queue, transport := prepared.Timing.QueueWaitMS, prepared.Timing.TransportMS
		prepare := prepared.Timing.PrepareMS
		record.PrepareMS = &prepare
		record.QueueWaitMS = &queue
		if !prepared.Timing.TransportStarted.IsZero() {
			record.TransportMS = &transport
			record.FirstTokenMS = prepared.Timing.FirstTokenMS
		}
	}
	if response != nil {
		record.ProviderRequestID = response.ProviderRequestID
		record.RateLimitObservation = modelRateLimitObservation(response.RateLimitObservation)
	}
	if response != nil && response.Usage != nil {
		record.UsageComplete = true
		record.InputTokens = cloneTokenPointer(response.Usage.InputTokens)
		record.CachedInputTokens = cloneTokenPointer(response.Usage.CachedInputTokens)
		record.UncachedInputTokens = cloneTokenPointer(response.Usage.UncachedInputTokens)
		record.CacheWriteTokens = cloneTokenPointer(response.Usage.CacheWriteTokens)
		record.OutputTokens = cloneTokenPointer(response.Usage.OutputTokens)
		record.ReasoningTokens = cloneTokenPointer(response.Usage.ReasoningTokens)
		record.TotalTokens = cloneTokenPointer(response.Usage.TotalTokens)
		record.RawUsage = rawModelUsage(response.Usage.Raw)
	}
	if callErr != nil {
		record.Status = ModelUsageFailed
		record.ErrorCode = "request_failed"
		if contextErr != nil {
			record.Status = ModelUsageCancelled
			record.ErrorCode = "cancelled"
		}
	}
	applyPricing(&record)
	return record
}

func modelRateLimitObservation(value *llm.RateLimitObservation) *RateLimitObservation {
	if value == nil {
		return nil
	}
	return &RateLimitObservation{
		RequestLimit: value.RequestLimit, RequestRemaining: value.RequestRemaining, RequestReset: value.RequestReset,
		TokenLimit: value.TokenLimit, TokenRemaining: value.TokenRemaining, TokenReset: value.TokenReset,
		ObservedAt: value.ObservedAt,
	}
}

func normalizedModelUsageReasoningPolicy(config llm.Config, purpose llm.RequestPurpose) string {
	if purpose != llm.RequestPurposeAgentTurn && purpose != llm.RequestPurposeSubagent && purpose != llm.RequestPurposeScheduledTask && purpose != llm.RequestPurposeHookAgent {
		return ""
	}
	value := strings.ToLower(strings.TrimSpace(config.ReasoningEffort))
	if value == "" {
		return "provider_default"
	}
	return value
}

func (r *Runner) syncRestoredContextWindowUsageAnchor() {
	if r == nil {
		return
	}
	if r.contextGovernor == nil {
		r.config.RestoredContextWindowUsageAnchor = nil
		return
	}
	r.config.RestoredContextWindowUsageAnchor = r.contextGovernor.ExportUsageAnchor(r.config.ModelConfig)
}

// BuildModelUsageRecord is shared by non-thread authorities such as DreamTask.
// It contains no journal side effect; the owning authority persists the result.
func BuildModelUsageRecord(requestID, model, protocol string, config llm.Config, purpose llm.RequestPurpose, prepared llm.PreparedRequest, started time.Time, response *llm.Response, callErr, contextErr error) ModelUsageRecord {
	return modelUsageRecord(requestID, model, protocol, config, purpose, prepared, started, response, callErr, contextErr)
}

func cloneTokenPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func rawModelUsage(value llm.RawUsage) RawModelUsage {
	return RawModelUsage{
		NormalizerID: value.NormalizerID, PromptTokens: cloneTokenPointer(value.PromptTokens),
		InputTokens: cloneTokenPointer(value.InputTokens), CompletionTokens: cloneTokenPointer(value.CompletionTokens),
		OutputTokens: cloneTokenPointer(value.OutputTokens), TotalTokens: cloneTokenPointer(value.TotalTokens),
		ReasoningTokens: cloneTokenPointer(value.ReasoningTokens),
		CachedTokens:    cloneTokenPointer(value.CachedTokens), PromptCacheHitTokens: cloneTokenPointer(value.PromptCacheHitTokens),
		PromptCacheMissTokens: cloneTokenPointer(value.PromptCacheMissTokens), CacheReadInputTokens: cloneTokenPointer(value.CacheReadInputTokens),
		CacheCreationInputTokens: cloneTokenPointer(value.CacheCreationInputTokens),
	}
}

func agentAffinityScope(threadID string, config AgentConfig) string {
	if config.SubagentOrigin != nil && config.SubagentOrigin.AgentID != "" {
		return threadID + ":agent:" + config.SubagentOrigin.AgentID
	}
	return threadID
}
