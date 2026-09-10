package agentcore

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

func (r *Runner) tryProviderNativeCompaction(ctx stdcontext.Context, result CompactResult) CompactResult {
	if r == nil {
		return result
	}
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return result
	}
	cfg := r.transportConfig(llmReq)
	if _, ok := llm.ResolveNativeCompactionCapability(cfg, llmReq.Model); !ok {
		return result
	}
	result.NativeCompactionAttempted = true
	messages := make([]convcontext.Message, 0, len(llmReq.PrefixMessages)+len(llmReq.Messages))
	messages = append(messages, cloneContextMessages(llmReq.PrefixMessages)...)
	messages = append(messages, cloneContextMessages(llmReq.Messages)...)
	request := llm.Request{
		RequestID: newAwakenID("model_request"), SystemPrompt: llmReq.SystemPrompt, Messages: messages,
		Model: llmReq.Model, Purpose: llm.RequestPurposeNativeCompaction,
		AffinityScopeID: agentAffinityScope(r.threadID, r.config), ReasoningPolicy: r.config.ModelConfig.ReasoningEffort,
		NativeCompaction:      true,
		NativeCompactionState: nativeCompactionStateFromBundle(r.config.ModelConfig, r.config.RestoredContextBundleV2),
	}
	authority := ModelCallAuthority{
		ThreadID: r.threadID, RunID: r.runID, SessionID: r.sessionID, Recorder: r.recorder,
		SubagentTaskID: r.config.SubagentTaskID, AttemptID: r.config.AttemptID, Progress: r.subagentProgress,
	}
	if origin := r.config.SubagentOrigin; origin != nil {
		authority.AgentID, authority.ParentAgentID = origin.AgentID, origin.ParentAgentID
	}
	var response *llm.Response
	if r.nativeCompactionInvoker != nil {
		response, err = r.nativeCompactionInvoker(ctx, authority, cfg, request)
	} else {
		response, err = InvokeRecordedModel(ctx, authority, cfg, request, 0, false)
	}
	if err != nil || response == nil || response.NativeCompactionState == nil {
		result.NativeCompactionReason = "native_compaction_failed"
		return result
	}
	result.NativeCompactionState = cloneNativeCompactionState(response.NativeCompactionState)
	result.NativeCompactionUsed = true
	result.NativeCompactionReason = "native_compaction_succeeded"
	return result
}

func providerOpaqueCompactionRef(state *llm.NativeCompactionState) ProviderOpaqueCompactionRef {
	if state == nil {
		return ProviderOpaqueCompactionRef{}
	}
	return ProviderOpaqueCompactionRef{
		ProviderID: state.ProviderID, ProfileID: state.ProfileID,
		CredentialRevision: state.CredentialRevision, Protocol: string(state.ProtocolFamily),
		Model: state.ModelID, EndpointContractID: state.EndpointContractID,
		Output: cloneRawMessages(state.Output), StateDigest: state.Digest,
	}
}

func nativeCompactionStateFromBundle(config ModelConfig, bundle *PostCompactContextBundleV2) *llm.NativeCompactionState {
	if bundle == nil || bundle.ProviderOpaqueState.State != MaterializationMaterialized || bundle.ProviderOpaqueState.Value == nil {
		return nil
	}
	ref := bundle.ProviderOpaqueState.Value
	state := &llm.NativeCompactionState{
		ProviderID: ref.ProviderID, ProfileID: ref.ProfileID,
		CredentialRevision: ref.CredentialRevision, ModelID: ref.Model,
		ProtocolFamily: llm.Protocol(ref.Protocol), EndpointContractID: ref.EndpointContractID,
		Output: cloneRawMessages(ref.Output), Digest: ref.StateDigest,
	}
	// The llm adapter performs the final exact capability and digest check. This
	// host-side identity check keeps incompatible state out of generic request
	// projections even before protocol lowering.
	if strings.ToLower(strings.TrimSpace(config.ProviderID)) != state.ProviderID ||
		strings.TrimSpace(config.ProfileID) != state.ProfileID ||
		effectiveModelCredentialRevision(config.CredentialRevision) != state.CredentialRevision ||
		strings.TrimSpace(config.Model) != state.ModelID ||
		llm.Protocol(config.Protocol) != state.ProtocolFamily {
		return nil
	}
	return state
}

func validateProviderOpaqueCompactionRef(ref ProviderOpaqueCompactionRef) error {
	if strings.TrimSpace(ref.ProviderID) == "" || strings.TrimSpace(ref.ProfileID) == "" ||
		ref.CredentialRevision == 0 || strings.TrimSpace(ref.Model) == "" ||
		ref.Protocol != string(llm.ProtocolOpenAIResponses) ||
		ref.EndpointContractID != llm.EndpointContractOpenAIResponses || len(ref.Output) < 2 {
		return fmt.Errorf("provider_opaque_state identity is invalid")
	}
	if ref.StateDigest == "" || ref.StateDigest != rawMessagesDigest(ref.Output) {
		return fmt.Errorf("provider_opaque_state digest mismatch")
	}
	compactionCount := 0
	for index, raw := range ref.Output {
		var item struct {
			Type             string `json:"type"`
			Role             string `json:"role"`
			EncryptedContent string `json:"encrypted_content"`
		}
		if json.Unmarshal(raw, &item) != nil {
			return fmt.Errorf("provider_opaque_state output[%d] is invalid", index)
		}
		if item.Type == "compaction" {
			compactionCount++
			if strings.TrimSpace(item.EncryptedContent) == "" || index != len(ref.Output)-1 {
				return fmt.Errorf("provider_opaque_state compaction item is invalid")
			}
		} else if item.Type != "message" || item.Role != "user" || index == len(ref.Output)-1 {
			return fmt.Errorf("provider_opaque_state output[%d] is not a user message", index)
		}
	}
	if compactionCount != 1 {
		return fmt.Errorf("provider_opaque_state requires one compaction item")
	}
	return nil
}

func rawMessagesDigest(output []json.RawMessage) string {
	hash := sha256.New()
	for _, raw := range output {
		canonical, ok := canonicalRawMessage(raw)
		if !ok {
			return ""
		}
		hash.Write(canonical)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(values))
	for index, value := range values {
		if canonical, ok := canonicalRawMessage(value); ok {
			result[index] = append(json.RawMessage(nil), canonical...)
		}
	}
	return result
}

func canonicalRawMessage(raw json.RawMessage) ([]byte, bool) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	encoded, err := json.Marshal(value)
	return encoded, err == nil
}

func cloneNativeCompactionState(state *llm.NativeCompactionState) *llm.NativeCompactionState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Output = cloneRawMessages(state.Output)
	return &cloned
}

func effectiveModelCredentialRevision(value uint64) uint64 {
	if value == 0 {
		return 1
	}
	return value
}
