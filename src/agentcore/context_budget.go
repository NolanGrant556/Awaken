package agentcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

const CompactTargetUnachievableCode = "COMPACT_TARGET_UNACHIEVABLE"

var ErrCompactTargetUnachievable = errors.New("compact target unachievable")

type CompactTargetUnachievableError struct {
	Reason            string
	Projected         int
	Target            int
	MandatoryFloor    int
	CategoryBreakdown map[string]int
}

func (err *CompactTargetUnachievableError) Error() string {
	if err == nil {
		return CompactTargetUnachievableCode
	}
	message := fmt.Sprintf("%s: reason=%s projected=%d target=%d mandatory_floor=%d", CompactTargetUnachievableCode, err.Reason, err.Projected, err.Target, err.MandatoryFloor)
	if len(err.CategoryBreakdown) > 0 {
		if encoded, encodeErr := json.Marshal(err.CategoryBreakdown); encodeErr == nil {
			message += " category_breakdown=" + string(encoded)
		}
	}
	return message
}

func (*CompactTargetUnachievableError) Unwrap() error { return ErrCompactTargetUnachievable }

func (r *Runner) evaluatePostCompactBudget(bundle *PostCompactContextBundleV2, result CompactResult, skillBlocks []ContextBlock) (CompactBudgetReceipt, error) {
	if r == nil || bundle == nil {
		return CompactBudgetReceipt{}, fmt.Errorf("post-compact budget requires runner and bundle")
	}
	proposed := r.config
	proposed.SkillContextBlocks = append([]ContextBlock(nil), skillBlocks...)
	proposed.CompactContextBlocks = CompactContextBlocksFromBundleV2(bundle)
	proposed.CompactedMemoryContext = CompactedMemoryContextFromBundleV2(bundle)
	// Successful installation drops the recalled Memory bodies and retains
	// only the bundle's identity projection. The draft budget must model that
	// exact post-commit state instead of joining the soon-to-be-dropped bodies
	// back into the proposed request.
	proposed.MemoryContextProvider = nil
	// The draft bundle marks the exact post-compact projection. Context-aware
	// tool descriptions (notably the Skill catalog) must therefore budget
	// against the hard compact target rather than regenerate a full-window
	// catalog inside a 10% envelope.
	proposed.RestoredContextBundleV2 = bundle
	projected, contributions, err := r.projectConfigTokens(proposed, result.Messages)
	if err != nil {
		return CompactBudgetReceipt{}, err
	}
	mandatoryMessages := result.Messages
	if len(mandatoryMessages) > 1 {
		mandatoryMessages = mandatoryMessages[:1]
	}
	mandatory := proposed
	mandatory.SkillContextBlocks = nil
	mandatoryFloor, _, err := r.projectConfigTokens(mandatory, mandatoryMessages)
	if err != nil {
		return CompactBudgetReceipt{}, err
	}
	state := NewContextWindowStateWithConfig(ContextEnvelope{}, contextWindowConfigFromModelConfig(proposed.ModelConfig))
	target := state.CompactTarget
	effective := state.EffectiveWindow
	ratio := 1.0
	if r.contextGovernor != nil && r.contextGovernor.lastAPIUsageTokens > 0 && r.contextGovernor.lastSentEstimatedInputTokens > 0 {
		candidate := float64(r.contextGovernor.lastAPIUsageTokens) / float64(r.contextGovernor.lastSentEstimatedInputTokens)
		if candidate > ratio {
			ratio = candidate
		}
	}
	conservative := int(math.Ceil(float64(projected) * ratio))
	receipt := CompactBudgetReceipt{
		EffectiveInputTokens: effective, CompactTargetTokens: target,
		LocalProjectedTokens: projected, ConservativeProjected: conservative,
		MandatoryFloorTokens: mandatoryFloor, CalibrationRatio: ratio,
		ContributionTokens: contributions,
	}
	if conservative <= target {
		return receipt, nil
	}
	reason := "post_compact_projection_exceeds_target"
	if int(math.Ceil(float64(mandatoryFloor)*ratio)) > target {
		reason = "mandatory_floor_exceeds_target"
	}
	return receipt, &CompactTargetUnachievableError{
		Reason: reason, Projected: conservative, Target: target,
		MandatoryFloor:    int(math.Ceil(float64(mandatoryFloor) * ratio)),
		CategoryBreakdown: contributions,
	}
}

func (r *Runner) projectConfigTokens(config AgentConfig, messages []convcontext.Message) (int, map[string]int, error) {
	conversation := convcontext.NewConversation(nil)
	conversation.Context.SetMessages(stripProviderOpaqueCompactionState(messages))
	request := NewRequestBuilder(config).BindThread(r.threadID).Build(conversation.Context, nil)
	if request.EnvelopeDebug == nil {
		return 0, nil, fmt.Errorf("post-compact request projection is unavailable")
	}
	wireMessages := make([]convcontext.Message, 0, len(request.PrefixMessages)+len(request.Messages))
	wireMessages = append(wireMessages, cloneContextMessages(request.PrefixMessages)...)
	wireMessages = append(wireMessages, cloneContextMessages(request.Messages)...)
	wireRequest := llm.Request{
		SystemPrompt: request.SystemPrompt, Messages: wireMessages, Tools: request.Tools,
		Model: request.Model, Temperature: request.Temperature, MaxTokens: request.MaxTokens,
		Purpose: effectiveModelRequestPurpose(config), AffinityScopeID: agentAffinityScope(r.threadID, config),
		ReasoningPolicy:       config.ModelConfig.ReasoningEffort,
		NativeCompactionState: nativeCompactionStateFromBundle(config.ModelConfig, config.RestoredContextBundleV2),
	}
	transport := transportConfigForAgentConfig(config, request)
	// Protocol lowering does not use credentials to shape the JSON body. Unit
	// projections and offline validation may therefore lower with explicit
	// no-auth when no secret is available, while real sends retain exact auth.
	if strings.TrimSpace(transport.APIKey) == "" {
		transport.AuthScheme = "none"
	}
	prepared, err := llm.PrepareRequest(transport, wireRequest, true)
	if err != nil {
		return 0, nil, fmt.Errorf("prepare post-compact provider projection: %w", err)
	}
	snapshot := buildContextUsageSnapshot(request, "post_compact_projection", "", string(prepared.Protocol), prepared.RequestShapeDigest, prepared.Body)
	contributions := map[string]int{}
	for _, contribution := range snapshot.Contributions {
		contributions[string(contribution.Kind)] += contribution.EstimatedTokens
	}
	return snapshot.EstimatedInputTokens, contributions, nil
}

func compactMemoryReferenceText(ref MemoryRecallSnapshotRef) string {
	if ref.State != ReferenceAvailable || len(ref.Items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ref.Items))
	for _, item := range ref.Items {
		parts = append(parts, fmt.Sprintf("%s@%d:%s", item.MemoryID, item.Revision, item.ContentDigest))
	}
	return strings.Join(parts, "\n")
}

// projectPostCompactBudget builds a disposable draft using the same V2
// allocator as commit. It neither invokes a model nor persists/installs a
// boundary. Preflight supplies a minimum summary; the postcondition supplies
// the actual summary and retained tail, both including durable environment refs.
func (r *Runner) projectPostCompactBudget(result CompactResult) (bool, CompactBudgetReceipt, error) {
	if !contextCompactionV2Enabled() || usesLegacyContextCompactionRecorder(r.recorder) || (r.config.ModelConfig.ContextWindow <= 0 && r.config.ModelConfig.MaxInputTokens <= 0) {
		return false, CompactBudgetReceipt{}, nil
	}
	bundle, err := r.buildPostCompactContextBundleV2(result, compactInvokedSkillContextBlocks(r.config.SkillContextBlocks))
	if err != nil {
		return true, CompactBudgetReceipt{}, err
	}
	if bundle.BudgetReceipt.Value == nil {
		return true, CompactBudgetReceipt{}, fmt.Errorf("post-compact budget receipt is missing")
	}
	return true, *bundle.BudgetReceipt.Value, nil
}
