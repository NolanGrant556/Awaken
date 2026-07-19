package agentcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

const contextEstimatorID = "awaken_leaf_v4"

type ContextContributionKind string

const (
	ContextContributionSystemPrompt    ContextContributionKind = "system_prompt"
	ContextContributionRulesProject    ContextContributionKind = "rules_project"
	ContextContributionRuntimeContext  ContextContributionKind = "runtime_context"
	ContextContributionBuiltinTools    ContextContributionKind = "builtin_tools"
	ContextContributionMCPDynamicTools ContextContributionKind = "mcp_dynamic_tools"
	ContextContributionMCP             ContextContributionKind = "mcp"
	ContextContributionSkills          ContextContributionKind = "skills"
	// ContextContributionSkillActivation marks full Skill instructions and
	// host selection bindings after a Skill is used. They are conversation
	// content, not catalog metadata.
	ContextContributionSkillActivation ContextContributionKind = "skill_activation"
	ContextContributionMemory          ContextContributionKind = "memory"
	ContextContributionConversation    ContextContributionKind = "conversation"
	// ContextContributionSkillsMemory remains read-compatible with v1 records.
	// New snapshots never emit it because Skill and Memory are independently attributed.
	ContextContributionSkillsMemory        ContextContributionKind = "skills_memory"
	ContextContributionSubagentDefinitions ContextContributionKind = "subagent_definitions"
	ContextContributionConversationHistory ContextContributionKind = "conversation_history"
	ContextContributionToolResults         ContextContributionKind = "tool_results"
	ContextContributionAttachmentsMedia    ContextContributionKind = "attachments_media"
	ContextContributionCurrentInput        ContextContributionKind = "current_input"
	ContextContributionOther               ContextContributionKind = "other"
)

var contextContributionOrder = []ContextContributionKind{
	ContextContributionSystemPrompt,
	ContextContributionBuiltinTools,
	ContextContributionMCP,
	ContextContributionSkills,
	ContextContributionMemory,
	ContextContributionConversation,
	// A v1 combined value can only appear through compatibility inputs. Keep it
	// balanced without pretending it can be split into Skill and Memory.
	ContextContributionSkillsMemory,
}

type ContextUsageContribution struct {
	Kind            ContextContributionKind `json:"kind"`
	EstimatedTokens int                     `json:"estimated_tokens"`
	SourceCount     int                     `json:"source_count"`
}

type ContextUsageSnapshot struct {
	SchemaVersion              int                        `json:"schema_version"`
	SnapshotID                 string                     `json:"snapshot_id"`
	EstimatorID                string                     `json:"estimator_id"`
	Basis                      string                     `json:"basis"`
	RequestID                  string                     `json:"request_id,omitempty"`
	RequestSequence            int                        `json:"request_sequence,omitempty"`
	ModelID                    string                     `json:"model_id,omitempty"`
	ProtocolFamily             string                     `json:"protocol_family,omitempty"`
	RequestShapeDigest         string                     `json:"request_shape_digest,omitempty"`
	ModelWindowTokens          int                        `json:"model_window_tokens"`
	OutputReserveTokens        int                        `json:"output_reserve_tokens"`
	EffectiveInputWindowTokens int                        `json:"effective_input_window_tokens"`
	EstimatedInputTokens       int                        `json:"estimated_input_tokens"`
	WarningThresholdTokens     int                        `json:"warning_threshold_tokens"`
	AutoCompactThresholdTokens int                        `json:"auto_compact_threshold_tokens"`
	CompactTargetTokens        int                        `json:"compact_target_tokens"`
	Contributions              []ContextUsageContribution `json:"contributions,omitempty"`
	GeneratedAt                time.Time                  `json:"generated_at"`
}

type ModelUsageStatus string

const (
	ModelUsageSucceeded   ModelUsageStatus = "succeeded"
	ModelUsageFailed      ModelUsageStatus = "failed"
	ModelUsageCancelled   ModelUsageStatus = "cancelled"
	ModelUsageInterrupted ModelUsageStatus = "interrupted"
)

// ModelUsageRecord is request-scoped provider truth. Nullable token fields
// distinguish a missing provider field from a reported zero.
type ModelUsageRecord struct {
	SchemaVersion       int              `json:"schema_version"`
	RequestID           string           `json:"request_id"`
	Purpose             string           `json:"purpose"`
	ProviderID          string           `json:"provider_id,omitempty"`
	ProfileID           string           `json:"profile_id,omitempty"`
	CredentialRevision  uint64           `json:"credential_revision,omitempty"`
	ModelID             string           `json:"model_id,omitempty"`
	ProtocolFamily      string           `json:"protocol_family,omitempty"`
	EndpointContractID  string           `json:"endpoint_contract_id,omitempty"`
	CacheMode           string           `json:"cache_mode,omitempty"`
	CacheKeyApplied     bool             `json:"cache_key_applied"`
	ContextEpochID      string           `json:"context_epoch_id,omitempty"`
	PrefixDigest        string           `json:"prefix_digest,omitempty"`
	InvalidationReason  string           `json:"invalidation_reason,omitempty"`
	RetryIndex          int              `json:"retry_index,omitempty"`
	AuxiliaryFallback   bool             `json:"auxiliary_fallback,omitempty"`
	InputTokens         *int             `json:"input_tokens,omitempty"`
	CachedInputTokens   *int             `json:"cached_input_tokens,omitempty"`
	UncachedInputTokens *int             `json:"uncached_input_tokens,omitempty"`
	CacheWriteTokens    *int             `json:"cache_write_tokens,omitempty"`
	OutputTokens        *int             `json:"output_tokens,omitempty"`
	ReasoningTokens     *int             `json:"reasoning_tokens,omitempty"`
	TotalTokens         *int             `json:"total_tokens,omitempty"`
	RawUsage            RawModelUsage    `json:"raw_typed_usage"`
	UsageComplete       bool             `json:"usage_complete"`
	CalculatedCostUSD   *float64         `json:"calculated_cost_usd,omitempty"`
	PricingSnapshot     *PricingSnapshot `json:"pricing_snapshot,omitempty"`
	Currency            string           `json:"currency,omitempty"`
	ProviderRequestID   string           `json:"provider_request_id,omitempty"`
	LatencyMS           int64            `json:"latency_ms"`
	Status              ModelUsageStatus `json:"status"`
	ErrorCode           string           `json:"error_code,omitempty"`
	CompletedAt         time.Time        `json:"completed_at"`
}

// PricingSnapshot binds cost calculation to the price list that was current
// when the request completed. Unknown/package pricing deliberately has no
// snapshot and no calculated cost rather than a fabricated zero.
type PricingSnapshot struct {
	SnapshotID              string  `json:"snapshot_id"`
	ProviderID              string  `json:"provider_id"`
	ModelID                 string  `json:"model_id"`
	Currency                string  `json:"currency"`
	UnitTokens              int     `json:"unit_tokens"`
	CachedInputPerUnitUSD   float64 `json:"cached_input_per_unit_usd"`
	UncachedInputPerUnitUSD float64 `json:"uncached_input_per_unit_usd"`
	OutputPerUnitUSD        float64 `json:"output_per_unit_usd"`
	EffectiveDate           string  `json:"effective_date"`
	SourceURL               string  `json:"source_url"`
}

type RawModelUsage struct {
	NormalizerID             string `json:"normalizer_id"`
	PromptTokens             *int   `json:"prompt_tokens,omitempty"`
	InputTokens              *int   `json:"input_tokens,omitempty"`
	CompletionTokens         *int   `json:"completion_tokens,omitempty"`
	OutputTokens             *int   `json:"output_tokens,omitempty"`
	ReasoningTokens          *int   `json:"reasoning_tokens,omitempty"`
	TotalTokens              *int   `json:"total_tokens,omitempty"`
	CachedTokens             *int   `json:"cached_tokens,omitempty"`
	PromptCacheHitTokens     *int   `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens    *int   `json:"prompt_cache_miss_tokens,omitempty"`
	CacheReadInputTokens     *int   `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens *int   `json:"cache_creation_input_tokens,omitempty"`
}

func buildContextUsageSnapshot(req LLMRequest, basis, requestID, protocol, shapeDigest string, preparedBody []byte) ContextUsageSnapshot {
	counts := map[ContextContributionKind]*ContextUsageContribution{}
	add := func(kind ContextContributionKind, tokens int) {
		if tokens <= 0 {
			return
		}
		entry := counts[kind]
		if entry == nil {
			entry = &ContextUsageContribution{Kind: kind}
			counts[kind] = entry
		}
		entry.EstimatedTokens += tokens
		entry.SourceCount++
	}

	for _, section := range req.PromptSections {
		add(contextUsageKindForPromptSection(section.Kind), convcontext.EstimateTokens(section.Content))
	}
	preparedTools, preparedToolsOK := preparedToolJSONLeaves(preparedBody, len(req.AttributedTools))
	for index, attributed := range req.AttributedTools {
		tokens := 0
		if preparedToolsOK {
			tokens = convcontext.EstimateTokens(string(preparedTools[index]))
		} else if data, err := json.Marshal(attributed.Schema); err == nil {
			tokens = convcontext.EstimateTokens(string(data))
		}
		switch attributed.Provenance.Origin {
		case tool.SourceMCP:
			add(ContextContributionMCP, tokens)
		case tool.SourceSkill:
			skillTokens := estimateSkillCatalogTokens(preparedTools, preparedToolsOK, index, attributed, tokens)
			add(ContextContributionSkills, skillTokens)
			add(ContextContributionBuiltinTools, tokens-skillTokens)
		default:
			add(ContextContributionBuiltinTools, tokens)
		}
	}
	for _, message := range req.Messages {
		kind := ContextContributionConversation
		add(kind, estimateMessageTextTokens(message))
		for _, attachment := range message.Attachments {
			tokens := convcontext.EstimateTokens(attachment.Kind)
			tokens += convcontext.EstimateTokens(attachment.Path)
			tokens += convcontext.EstimateAttachmentPayloadTokens(attachment.Kind, attachment.DataBase64)
			add(ContextContributionConversation, tokens)
		}
		for _, part := range message.ModelContent {
			add(ContextContributionConversation, convcontext.EstimateModelContentTokens(part))
		}
	}

	known := 0
	for _, entry := range counts {
		known += entry.EstimatedTokens
	}
	total := known
	if len(preparedBody) > 0 {
		if preparedEstimate := estimatePreparedJSONTokens(preparedBody); preparedEstimate > total {
			total = preparedEstimate
		}
	}
	if total > known {
		add(ContextContributionConversation, total-known)
	}

	contributions := make([]ContextUsageContribution, 0, len(counts))
	for _, kind := range contextContributionOrder {
		if entry := counts[kind]; entry != nil && entry.EstimatedTokens > 0 {
			contributions = append(contributions, *entry)
		}
	}
	state := req.ContextWindowState
	generatedAt := time.Now().UTC()
	snapshotSeed, _ := json.Marshal(struct {
		RequestID string
		Digest    string
		Total     int
		At        int64
	}{requestID, shapeDigest, total, generatedAt.UnixNano()})
	digest := sha256.Sum256(snapshotSeed)
	return ContextUsageSnapshot{
		SchemaVersion: 4, SnapshotID: "ctx_" + hex.EncodeToString(digest[:12]), EstimatorID: contextEstimatorID,
		Basis: basis, RequestID: requestID, ModelID: req.Model, ProtocolFamily: protocol, RequestShapeDigest: shapeDigest,
		ModelWindowTokens: state.ModelWindow, OutputReserveTokens: state.OutputReserve,
		EffectiveInputWindowTokens: state.EffectiveWindow, EstimatedInputTokens: total,
		WarningThresholdTokens: state.WarningThreshold, AutoCompactThresholdTokens: state.AutoCompactThreshold,
		CompactTargetTokens: state.CompactTarget, Contributions: contributions, GeneratedAt: generatedAt,
	}
}

func contextUsageKindForPromptSection(kind ContextContributionKind) ContextContributionKind {
	switch kind {
	case ContextContributionSystemPrompt,
		ContextContributionRulesProject,
		ContextContributionSubagentDefinitions:
		return ContextContributionSystemPrompt
	case ContextContributionBuiltinTools:
		return ContextContributionBuiltinTools
	case ContextContributionMCP, ContextContributionMCPDynamicTools:
		return ContextContributionMCP
	case ContextContributionSkills, ContextContributionSkillActivation:
		return ContextContributionConversation
	case ContextContributionMemory:
		return ContextContributionMemory
	case ContextContributionSkillsMemory:
		return ContextContributionSkillsMemory
	default:
		return ContextContributionConversation
	}
}

func estimateMessageTextTokens(message convcontext.Message) int {
	total := convcontext.EstimateTokens(message.Content)
	total += convcontext.EstimateTokens(message.ProviderProjectionText)
	total += convcontext.EstimateTokens(message.Name)
	total += convcontext.EstimateTokens(message.ToolCallID)
	for _, call := range message.ToolCalls {
		total += convcontext.EstimateTokens(call.ID)
		total += convcontext.EstimateTokens(call.Function.Name)
		total += convcontext.EstimateTokens(call.Function.Arguments)
	}
	return total
}

// preparedToolJSONLeaves returns the exact provider-lowered JSON leaf for
// every model-visible tool. Tool order is preserved by both supported
// protocols, so host provenance can classify MCP, Skill catalog and builtin
// definitions without exposing provenance on the wire.
func preparedToolJSONLeaves(body []byte, expected int) ([]json.RawMessage, bool) {
	if len(body) == 0 || expected == 0 {
		return nil, false
	}
	var request struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(body, &request); err != nil || len(request.Tools) != expected {
		return nil, false
	}
	return request.Tools, true
}

func estimateSkillCatalogTokens(preparedTools []json.RawMessage, preparedToolsOK bool, index int, attributed AttributedToolSchema, total int) int {
	catalog := strings.TrimSpace(attributed.Provenance.SkillCatalog)
	if catalog == "" || total <= 0 {
		return 0
	}
	if !preparedToolsOK || index >= len(preparedTools) {
		return min(total, convcontext.EstimateTokens(catalog))
	}
	var value any
	if json.Unmarshal(preparedTools[index], &value) != nil {
		return min(total, convcontext.EstimateTokens(catalog))
	}
	found := false
	var removeCatalog func(any) any
	removeCatalog = func(current any) any {
		switch typed := current.(type) {
		case map[string]any:
			out := make(map[string]any, len(typed))
			for key, item := range typed {
				out[key] = removeCatalog(item)
			}
			return out
		case []any:
			out := make([]any, len(typed))
			for itemIndex, item := range typed {
				out[itemIndex] = removeCatalog(item)
			}
			return out
		case string:
			if !found && strings.Contains(typed, catalog) {
				found = true
				return strings.Replace(typed, catalog, "", 1)
			}
		}
		return current
	}
	withoutCatalog, err := json.Marshal(removeCatalog(value))
	if err != nil || !found {
		return min(total, convcontext.EstimateTokens(catalog))
	}
	delta := total - convcontext.EstimateTokens(string(withoutCatalog))
	if delta < 0 {
		return 0
	}
	return min(total, delta)
}

// estimatePreparedJSONTokens estimates provider-visible JSON while replacing
// large encoded media leaves with the same bounded local media estimate used
// by conversation accounting.
func estimatePreparedJSONTokens(body []byte) int {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return convcontext.EstimateTokens(string(body))
	}
	var mediaTokens int
	var scrub func(any) any
	scrub = func(current any) any {
		switch typed := current.(type) {
		case map[string]any:
			out := make(map[string]any, len(typed))
			for key, item := range typed {
				out[key] = scrub(item)
			}
			return out
		case []any:
			out := make([]any, len(typed))
			for index, item := range typed {
				out[index] = scrub(item)
			}
			return out
		case string:
			if strings.HasPrefix(typed, "data:") && strings.Contains(typed, ";base64,") {
				mediaTokens += convcontext.EstimateAttachmentPayloadTokens("image", typed)
				return "[media]"
			}
			return typed
		default:
			return current
		}
	}
	scrubbed, err := json.Marshal(scrub(value))
	if err != nil {
		return convcontext.EstimateTokens(string(body))
	}
	return convcontext.EstimateTokens(string(scrubbed)) + mediaTokens
}
