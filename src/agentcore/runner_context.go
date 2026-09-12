package agentcore

import (
	"awaken/src/context"
	"awaken/src/llm"
	"awaken/src/tool"
	stdcontext "context"
	"fmt"
	"os"
	"strings"
)

func contextCompactionV2Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AWAKEN_CONTEXT_COMPACTION_V2"))) {
	case "0", "false", "off", "disabled":
		return false
	default:
		return true
	}
}

func usesLegacyContextCompactionRecorder(recorder SemanticRecorder) bool {
	legacy, ok := recorder.(LegacyContextCompactionRecorder)
	return ok && legacy.UsesLegacyContextCompactionRecord()
}

// SemanticCompactedMessage is the durable, blob-free form of one message in a
// post-compact conversation. Attachment bytes never enter the journal; only
// their identity and storage key are kept so replay can rehydrate owned blobs.
type SemanticCompactedMessage struct {
	OriginAgentID       string                              `json:"origin_agent_id,omitempty"`
	Role                string                              `json:"role"`
	Content             string                              `json:"content"`
	ToolCalls           []context.ToolCall                  `json:"tool_calls,omitempty"`
	ToolCallID          string                              `json:"tool_call_id,omitempty"`
	Name                string                              `json:"name,omitempty"`
	IsError             bool                                `json:"is_error,omitempty"`
	CompactBoundaryID   string                              `json:"compact_boundary_id,omitempty"`
	Attachments         []SemanticUserMessageAttachment     `json:"attachments,omitempty"`
	ModelContent        []tool.ModelContentPart             `json:"model_content,omitempty"`
	Origin              context.MessageOrigin               `json:"origin,omitempty"`
	MessageID           string                              `json:"message_id,omitempty"`
	TurnID              string                              `json:"turn_id,omitempty"`
	ProviderOpaqueState *context.ProviderOpaqueMessageState `json:"provider_opaque_state,omitempty"`
}

func semanticCompactedMessages(messages []context.Message) []SemanticCompactedMessage {
	out := make([]SemanticCompactedMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, SemanticCompactedMessage{
			OriginAgentID:       message.OriginAgentID,
			Role:                string(message.Role),
			Content:             message.Content,
			ToolCalls:           append([]context.ToolCall(nil), message.ToolCalls...),
			ToolCallID:          message.ToolCallID,
			Name:                message.Name,
			IsError:             message.IsError,
			CompactBoundaryID:   message.CompactBoundaryID,
			Attachments:         semanticUserMessageAttachments(message.Attachments),
			ModelContent:        tool.CloneModelContent(message.ModelContent),
			Origin:              message.Origin,
			MessageID:           message.MessageID,
			TurnID:              message.TurnID,
			ProviderOpaqueState: nil,
		})
	}
	return out
}

// persistContextCompacted remains the narrow test/helper entrypoint. New
// production compactions persist the stable V2 bundle; legacy v1 records are
// read-only compatibility input.
func (r *Runner) persistContextCompacted(ctx stdcontext.Context, result CompactResult) error {
	skillBlocks := compactInvokedSkillContextBlocks(r.config.SkillContextBlocks)
	bundle, err := r.buildPostCompactContextBundleV2(result, skillBlocks)
	if err != nil {
		return err
	}
	return r.persistContextCompactedV2(ctx, bundle)
}

func (r *Runner) persistContextCompactedV2(ctx stdcontext.Context, bundle PostCompactContextBundleV2) error {
	if r == nil || r.recorder == nil {
		return nil
	}
	if err := bundle.Validate(); err != nil {
		return fmt.Errorf("validate context compaction v2: %w", err)
	}
	payload := map[string]any{
		"bundle_v2": bundle,
	}
	if r.config.SubagentOrigin != nil && strings.TrimSpace(r.config.SubagentOrigin.AgentID) != "" {
		payload["agent_id"] = r.config.SubagentOrigin.AgentID
		payload["parent_agent_id"], payload["subagent_task_id"], payload["attempt_id"] = r.config.SubagentOrigin.ParentAgentID, r.config.SubagentTaskID, r.config.AttemptID
	}
	record := NewSemanticRecord(SemanticKindContextCompactedV2, r.threadID, r.runID, "", "", r.sessionID, payload)
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if flushed, ok := r.recorder.(FlushedSemanticRecorder); ok {
		if _, err := flushed.AppendFlushed(persistCtx, record); err != nil {
			return fmt.Errorf("append+flush context compaction: %w", err)
		}
		return nil
	}
	if err := r.recorder.Append(persistCtx, record); err != nil {
		return fmt.Errorf("persist context compaction: %w", err)
	}
	if err := r.recorder.Flush(persistCtx); err != nil {
		return fmt.Errorf("flush context compaction: %w", err)
	}
	return nil
}

func (r *Runner) persistContextCompactedV1(ctx stdcontext.Context, result CompactResult) error {
	if r == nil || r.recorder == nil {
		return nil
	}
	payload := map[string]any{
		"compact_boundary_id": result.BoundaryID, "mode": string(result.Mode),
		"messages":            semanticCompactedMessages(result.Messages),
		"pre_estimate_tokens": result.PreEstimateTokens, "post_estimate_tokens": result.PostEstimateTokens,
	}
	if r.config.SubagentOrigin != nil && strings.TrimSpace(r.config.SubagentOrigin.AgentID) != "" {
		payload["agent_id"] = r.config.SubagentOrigin.AgentID
	}
	record := NewSemanticRecord(SemanticKindContextCompacted, r.threadID, r.runID, "", "", r.sessionID, payload)
	persistCtx, cancel := durablePersistenceContext(ctx)
	defer cancel()
	if err := r.recorder.Append(persistCtx, record); err != nil {
		return fmt.Errorf("persist v1 context compaction: %w", err)
	}
	if err := r.recorder.Flush(persistCtx); err != nil {
		return fmt.Errorf("flush v1 context compaction: %w", err)
	}
	return nil
}

func (r *Runner) mergeMemoryRecallSnapshot() {
	if r == nil || r.config.CaptureMemoryRecallSnapshot == nil {
		return
	}
	incoming := r.config.CaptureMemoryRecallSnapshot()
	if incoming == nil || len(incoming.Items) == 0 {
		return
	}
	if r.memoryRecallSnapshot == nil {
		copied := *incoming
		copied.Items = append([]MemoryRecallSnapshotItem(nil), incoming.Items...)
		r.memoryRecallSnapshot = &copied
		return
	}
	r.memoryRecallSnapshot.MergePreserveExisting(incoming)
}

func (r *Runner) dropCompactedMemoryBodies(memoryIDs []string) {
	if r == nil || r.memoryRecallSnapshot == nil || len(memoryIDs) == 0 {
		return
	}
	compacted := make(map[string]struct{}, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		if memoryID = strings.TrimSpace(memoryID); memoryID != "" {
			compacted[memoryID] = struct{}{}
		}
	}
	if len(compacted) == 0 {
		return
	}
	items := r.memoryRecallSnapshot.Items[:0]
	for _, item := range r.memoryRecallSnapshot.Items {
		if _, exists := compacted[strings.TrimSpace(item.MemoryID)]; exists {
			continue
		}
		items = append(items, item)
	}
	r.memoryRecallSnapshot.Items = items
	if len(items) == 0 {
		r.memoryRecallSnapshot = nil
	}
}

func (r *Runner) buildLLMRequest() (LLMRequest, error) {
	llmReq := r.reqBuilder.Build(r.conversation.Context, nil)
	r.progressNudge.project(&llmReq)
	if err := context.ValidateToolResultPairing(llmReq.Messages); err != nil {
		return LLMRequest{}, fmt.Errorf("invalid tool result pairing: %w", err)
	}
	return llmReq, nil
}

func (r *Runner) rebuildRequestBuilder() {
	if r == nil {
		return
	}
	r.reqBuilder = NewRequestBuilder(r.config).BindThread(r.threadID)
}

func (r *Runner) persistCurrentContextSnapshot(ctx stdcontext.Context, events *runEventEmitter) (*ContextWindowState, error) {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return nil, err
	}
	llmReq, err = r.applyContextEpoch(ctx, llmReq, events)
	if err != nil {
		return nil, err
	}
	protocol := string(llm.ResolveProtocol(r.transportConfig(llmReq)))
	snapshot := buildContextUsageSnapshot(llmReq, "current_projection", "", protocol, "", nil)
	llmReq.ContextWindowState.EstimatedTokens = snapshot.EstimatedInputTokens
	llmReq.ContextWindowState.UsageSnapshot = &snapshot
	if events != nil {
		if err := events.contextSnapshot(ctx, snapshot); err != nil {
			return nil, err
		}
	}
	state := llmReq.ContextWindowState
	return &state, nil
}

// ProjectCurrentContextUsage builds the exact current-next-request projection
// without issuing a provider request. Thread replay uses it to replace missing
// or obsolete local estimates while preserving provider usage as a separate
// historical fact.
func (a *Agent) ProjectCurrentContextUsage(ctx stdcontext.Context) (ContextUsageSnapshot, *ContextEpochReceipt, error) {
	if a == nil {
		return ContextUsageSnapshot{}, nil, fmt.Errorf("agent unavailable")
	}
	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	req, err := runner.buildLLMRequest()
	if err != nil {
		return ContextUsageSnapshot{}, nil, err
	}
	req, err = runner.applyContextEpoch(ctx, req, nil)
	if err != nil {
		return ContextUsageSnapshot{}, nil, err
	}
	protocol := string(llm.ResolveProtocol(runner.transportConfig(req)))
	snapshot := buildContextUsageSnapshot(req, "current_projection", "", protocol, "", nil)
	return snapshot, req.ContextEpoch, nil
}

func (r *Runner) prepareLLMRequest(ctx stdcontext.Context, events *runEventEmitter) (LLMRequest, error) {
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return LLMRequest{}, err
	}
	if events != nil && r.config.MemoryRecallDrain != nil {
		if receipts := r.config.MemoryRecallDrain(); len(receipts) > 0 {
			if err := events.memoryRecalled(ctx, receipts); err != nil {
				return LLMRequest{}, err
			}
		}
	}
	r.mergeMemoryRecallSnapshot()
	if r.failureSupervisor != nil {
		if recovery := FormatRecoveryContext(r.failureSupervisor.PendingRecovery()); recovery != "" {
			llmReq.SystemPrompt = strings.TrimSpace(llmReq.SystemPrompt) +
				"\n\nRuntime recovery directive (must be followed before the next tool call):\n" + recovery
			llmReq.PromptSections = append(llmReq.PromptSections, PromptSection{
				ID: "runtime_recovery", Kind: ContextContributionOther, Content: recovery,
			})
			llmReq.EpochSources = append(llmReq.EpochSources, ContextEpochSourceCandidate{
				SourceKey: "runtime_recovery", Kind: ContextContributionOther,
				Content: recovery, State: ContextEpochSourceUpdated,
			})
		}
	}
	var preparedRequest LLMRequest
	if events != nil {
		preparedRequest, err = r.contextGovernor.PrepareRequest(ctx, llmReq, events)
	} else {
		preparedRequest, err = r.contextGovernor.PrepareRequest(ctx, llmReq, nil)
	}
	if err != nil {
		return LLMRequest{}, err
	}
	return r.applyContextEpoch(ctx, preparedRequest, events)
}

func (r *Runner) applyContextEpoch(ctx stdcontext.Context, req LLMRequest, events *runEventEmitter) (LLMRequest, error) {
	if r == nil || r.config.ContextEpochStore == nil {
		return req, nil
	}
	config := r.transportConfig(req)
	candidate := contextEpochCandidate(
		r.threadID, r.config.ModelConfig, req,
		llm.EndpointContractIDForConfig(config), llm.NormalizeBaseURLForContract(config.BaseURL),
	)
	if r.config.SubagentOrigin != nil {
		candidate.AgentID = r.config.SubagentOrigin.AgentID
	}
	projection, err := r.config.ContextEpochStore.Project(ctx, candidate)
	if err != nil {
		return LLMRequest{}, fmt.Errorf("%w: %w", ErrContextEpochProjection, err)
	}
	req.SystemPrompt = projection.SystemPrompt
	req.PromptSections = projection.PromptSections
	req.PrefixMessages = projection.PrefixMessages
	req.Tools = projection.Tools
	req.AttributedTools = projection.AttributedTools
	req.ContextEpoch = &projection.Receipt
	snapshot := buildContextUsageSnapshot(req, "current_projection", "", string(llm.ResolveProtocol(config)), "", nil)
	req.ContextUsage = snapshot
	req.ContextWindowState.EstimatedTokens = snapshot.EstimatedInputTokens
	req.ContextWindowState.UsageSnapshot = &snapshot
	if events != nil && projection.Receipt.State != "unchanged" {
		if err := events.contextEpoch(ctx, projection.Receipt); err != nil {
			return LLMRequest{}, err
		}
	}
	return req, nil
}

func (r *Runner) compactConversationForRetry(ctx stdcontext.Context, events *runEventEmitter) (bool, error) {
	return r.contextGovernor.TryReactiveCompact(ctx, events)
}
