package agentcore

import (
	stdcontext "context"
	"errors"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

const (
	defaultModelWindow      = 128000
	defaultOutputReserve    = 16000
	defaultBlockingReserved = 4000
)

var errContextWindowBlocked = errors.New("context window blocked")

type CompactMode string

const (
	CompactModeAuto     CompactMode = "auto"
	CompactModeReactive CompactMode = "reactive"
	CompactModeManual   CompactMode = "manual"
)

type ContextWindowConfig struct {
	ModelWindow           int
	OutputReserve         int
	EffectiveInputWindow  int
	AutoCompactTokenLimit int
	HardCompactTarget     bool
}

type ContextWindowState struct {
	ModelWindow                  int
	OutputReserve                int
	EffectiveWindow              int
	EstimatedTokens              int
	WarningThreshold             int
	AutoCompactThreshold         int
	CompactTarget                int
	HardCompactTarget            bool
	LastAPIUsageTokens           int
	LastSentEstimatedInputTokens int
	EstimatedDeltaTokens         int
	LastCompactBoundaryID        string
	ConsecutiveCompactFailures   int
	HasAttemptedReactiveCompact  bool
	CompactCircuitOpen           bool
	UsageSnapshot                *ContextUsageSnapshot
}

type CompactResult struct {
	Mode                      CompactMode
	BoundaryID                string
	SummaryMessage            convcontext.Message
	TailMessages              []convcontext.Message
	Messages                  []convcontext.Message
	EstimatedTokens           int
	PreEstimateTokens         int
	PostEstimateTokens        int
	ToolResultsPruned         int
	ToolResultsPrunedCount    int
	PrunedMessageCount        int
	PrunedSafeGroupCount      int
	PrunedDigestMessageCount  int
	SummaryMaxTokens          int
	SummaryRetryCount         int
	SummaryRawChars           int
	SummaryParsedSections     int
	SummaryMeaningfulSections int
	SummaryQualityIssue       string
	SummarySourceHadUser      bool
	SummaryLatestUserPinned   bool
	FallbackMarker            string
	Fallback                  CompactFallback
	NativeCompactionState     *llm.NativeCompactionState
	NativeCompactionAttempted bool
	NativeCompactionUsed      bool
	NativeCompactionReason    string
	BudgetReceipt             *CompactBudgetReceipt
}

type CompactFallback struct {
	Used   bool
	Marker string
	Reason string
}

type CompactRequest struct {
	Mode             CompactMode
	Messages         []convcontext.Message
	Envelope         ContextEnvelope
	TargetTokens     int
	SummaryMaxTokens int
	HookInstruction  string

	summaryInvoker compactSummaryInvoker
}

func NewContextWindowState(envelope ContextEnvelope) ContextWindowState {
	return NewContextWindowStateWithConfig(envelope, ContextWindowConfig{})
}

func NewContextWindowStateWithConfig(envelope ContextEnvelope, config ContextWindowConfig) ContextWindowState {
	modelWindow := defaultModelWindow
	if config.ModelWindow > 0 {
		modelWindow = config.ModelWindow
	}
	outputReserve := defaultOutputReserve
	if config.OutputReserve > 0 {
		outputReserve = config.OutputReserve
	}
	effectiveWindow := modelWindow - outputReserve
	if config.EffectiveInputWindow > 0 {
		effectiveWindow = config.EffectiveInputWindow
		if config.ModelWindow <= 0 {
			modelWindow = effectiveWindow
		}
	}
	if effectiveWindow < 0 {
		effectiveWindow = 0
	}
	autoCompactThreshold := int(float64(effectiveWindow) * 0.90)
	if config.AutoCompactTokenLimit > 0 {
		autoCompactThreshold = config.AutoCompactTokenLimit
		if autoCompactThreshold > effectiveWindow {
			autoCompactThreshold = effectiveWindow
		}
	}

	state := ContextWindowState{
		ModelWindow:           modelWindow,
		OutputReserve:         outputReserve,
		EffectiveWindow:       effectiveWindow,
		WarningThreshold:      int(float64(effectiveWindow) * 0.85),
		AutoCompactThreshold:  autoCompactThreshold,
		CompactTarget:         int(float64(effectiveWindow) * 0.10),
		HardCompactTarget:     config.HardCompactTarget,
		EstimatedTokens:       EstimateContextEnvelopeTokens(envelope),
		EstimatedDeltaTokens:  estimateContextDeltaTokens(envelope),
		LastCompactBoundaryID: lastCompactBoundaryID(envelope.Messages),
	}
	projection := contextProjectionRequest(envelope, state)
	snapshot := buildContextUsageSnapshot(projection, "current_projection", "", "", "", nil)
	state.EstimatedTokens = snapshot.EstimatedInputTokens
	state.UsageSnapshot = &snapshot
	return state
}

func contextProjectionRequest(envelope ContextEnvelope, state ContextWindowState) LLMRequest {
	req := LLMRequest{
		SystemPrompt: envelopeProjectedSystemPrompt(envelope), PromptSections: promptSectionsFromEnvelope(envelope),
		Messages: envelope.Messages, Tools: envelope.ToolSchemas, AttributedTools: envelope.AttributedTools,
		Model: envelope.Model, CapabilityModelID: envelope.CompactModelConfig.CapabilityModelID,
		EndpointSelector: envelope.CompactModelConfig.EndpointSelector, ModelSelectorKind: envelope.CompactModelConfig.ModelSelectorKind,
		ContextWindowState: state,
	}
	for _, item := range envelope.UserContextItems {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		req.PrefixMessages = append(req.PrefixMessages, convcontext.Message{
			Role: convcontext.RoleUser, Content: item.Content,
			Origin: convcontext.MessageOriginSynthetic, MessageID: item.ID,
		})
	}
	if len(req.AttributedTools) == 0 {
		for _, schema := range envelope.ToolSchemas {
			req.AttributedTools = append(req.AttributedTools, AttributedToolSchema{Schema: schema})
		}
	}
	return req
}

func (state ContextWindowState) ShouldWarn() bool {
	return state.WarningThreshold > 0 && state.ConservativeProjectedTokens() >= state.WarningThreshold
}

func (state ContextWindowState) ShouldAutoCompact() bool {
	return state.AutoCompactThreshold > 0 && state.EstimatedTokens >= state.AutoCompactThreshold
}

// ShouldAutoCompactByActualUsage is a parallel proactive-compact trigger that
// anchors the current local projection to the last provider-reported usage.
// The delta between the current estimate and the exact estimate captured when
// that provider request was sent preserves legitimate tool/message changes
// without pretending the local estimator is an authoritative tokenizer.
//
// This trigger is intentionally parallel and non-replacing:
//   - it never overrides EstimatedTokens or the v4 estimator;
//   - it does not change the ContextUsageSnapshot contributions or their sum;
//   - LastAPIUsageTokens == 0 (no prior request, or just after a successful
//     compact) leaves this trigger inert, so only ShouldAutoCompact drives
//     the decision in those cases.
func (state ContextWindowState) ShouldAutoCompactByActualUsage() bool {
	if state.AutoCompactThreshold <= 0 || state.LastAPIUsageTokens <= 0 {
		return false
	}
	return state.actualUsageProjectedTokens() >= state.AutoCompactThreshold
}

func (state ContextWindowState) ShouldBlock() bool {
	return state.blockingThreshold() > 0 && state.ConservativeProjectedTokens() >= state.blockingThreshold()
}

// ConservativeProjectedTokens returns the safer of the current local estimate
// and a provider-anchored projection. Old records that predate the sent-request
// estimate fall back to the narrower host-delta behavior.
func (state ContextWindowState) ConservativeProjectedTokens() int {
	projected := state.EstimatedTokens
	if state.LastAPIUsageTokens <= 0 {
		return projected
	}
	anchored := state.actualUsageProjectedTokens()
	if anchored > projected {
		return anchored
	}
	return projected
}

func (state ContextWindowState) actualUsageProjectedTokens() int {
	delta := state.EstimatedDeltaTokens
	if state.LastSentEstimatedInputTokens > 0 {
		delta = state.EstimatedTokens - state.LastSentEstimatedInputTokens
	}
	if delta > 0 && state.LastAPIUsageTokens > int(^uint(0)>>1)-delta {
		return int(^uint(0) >> 1)
	}
	projected := state.LastAPIUsageTokens + delta
	if projected < 0 {
		return 0
	}
	return projected
}

func (state ContextWindowState) blockingThreshold() int {
	threshold := state.EffectiveWindow - defaultBlockingReserved
	if threshold < 0 {
		return state.EffectiveWindow
	}
	return threshold
}

func (state ContextWindowState) Summary() string {
	return fmt.Sprintf(
		"context tokens estimated=%d projected=%d effective=%d warning=%d autocompact=%d compact_target=%d",
		state.EstimatedTokens,
		state.ConservativeProjectedTokens(),
		state.EffectiveWindow,
		state.WarningThreshold,
		state.AutoCompactThreshold,
		state.CompactTarget,
	)
}

func EstimateContextEnvelopeTokens(envelope ContextEnvelope) int {
	req := contextProjectionRequest(envelope, ContextWindowState{})
	snapshot := buildContextUsageSnapshot(req, "current_projection", "", "", "", nil)
	total := snapshot.EstimatedInputTokens
	messageAttachments := make(map[string]struct{})
	for _, message := range envelope.Messages {
		for _, attachment := range message.Attachments {
			messageAttachments[attachment.ID] = struct{}{}
		}
	}
	for _, attachment := range envelope.Attachments {
		if _, counted := messageAttachments[attachment.ID]; counted && attachment.ID != "" {
			continue
		}
		total += convcontext.EstimateTokens(attachment.Kind)
		total += convcontext.EstimateTokens(attachment.Path)
		total += convcontext.EstimateAttachmentPayloadTokens(attachment.Kind, attachment.Content)
	}
	return total
}

func estimateContextDeltaTokens(envelope ContextEnvelope) int {
	total := 0
	for _, delta := range envelope.Deltas {
		total += convcontext.EstimateTokens(string(delta.Type))
		total += convcontext.EstimateTokens(delta.Summary)
	}
	return total
}

func estimateMessageTokens(message convcontext.Message) int {
	total := convcontext.EstimateTokens(message.Content)
	total += convcontext.EstimateTokens(message.ProviderProjectionText)
	total += convcontext.EstimateTokens(message.Name)
	total += convcontext.EstimateTokens(message.ToolCallID)
	for _, toolCall := range message.ToolCalls {
		total += convcontext.EstimateTokens(toolCall.ID)
		total += convcontext.EstimateTokens(toolCall.Function.Name)
		total += convcontext.EstimateTokens(toolCall.Function.Arguments)
	}
	for _, part := range message.ModelContent {
		total += convcontext.EstimateModelContentTokens(part)
	}
	return total
}

func CompactConversation(messages []convcontext.Message, envelope ContextEnvelope, targetTokens int) (CompactResult, error) {
	return CompactConversationWithRequest(stdcontext.Background(), CompactRequest{
		Mode:         CompactModeAuto,
		Messages:     messages,
		Envelope:     envelope,
		TargetTokens: targetTokens,
	})
}

func selectCompactTail(messages []convcontext.Message, targetTokens int) []convcontext.Message {
	groups := compactSafeGroups(messages)
	selected := make([]convcontext.Message, 0)
	total := 0
	for i := len(groups) - 1; i >= 0; i-- {
		groupTokens := estimateMessagesTokens(groups[i])
		if len(selected) > 0 && total+groupTokens > targetTokens {
			break
		}
		selected = append(cloneContextMessages(groups[i]), selected...)
		total += groupTokens
	}
	if len(selected) == 0 && len(groups) > 0 {
		selected = cloneContextMessages(groups[len(groups)-1])
	}
	return selected
}

func compactSafeGroups(messages []convcontext.Message) [][]convcontext.Message {
	groups := make([][]convcontext.Message, 0, len(messages))
	for i := 0; i < len(messages); {
		start := i
		i++
		if messages[start].Role == convcontext.RoleAssistant && len(messages[start].ToolCalls) > 0 {
			pending := make(map[string]bool, len(messages[start].ToolCalls))
			for _, call := range messages[start].ToolCalls {
				pending[call.ID] = true
			}
			for i < len(messages) && len(pending) > 0 && messages[i].Role == convcontext.RoleTool {
				delete(pending, messages[i].ToolCallID)
				i++
			}
		}
		groups = append(groups, cloneContextMessages(messages[start:i]))
	}
	return groups
}

func estimateMessagesTokens(messages []convcontext.Message) int {
	total := 0
	for _, message := range messages {
		total += estimateMessageTokens(message)
	}
	return total
}

func buildCompactSummary(boundaryID string, messages []convcontext.Message, envelope ContextEnvelope) string {
	parts := []string{
		fmt.Sprintf("Conversation compact summary boundary=%s.", boundaryID),
		fmt.Sprintf("Compacted %d earlier messages.", len(messages)),
	}
	if envelope.RuntimeSnapshot.WorkspaceRoot != "" {
		parts = append(parts, "Workspace root: "+envelope.RuntimeSnapshot.WorkspaceRoot+".")
	}
	lastUser := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == convcontext.RoleUser && messages[i].Origin != convcontext.MessageOriginSynthetic && strings.TrimSpace(messages[i].Content) != "" {
			lastUser = strings.TrimSpace(messages[i].Content)
			break
		}
	}
	if lastUser != "" {
		parts = append(parts, "Most recent user message before compact: "+limitSummaryText(lastUser)+".")
	}
	return strings.Join(parts, "\n")
}

func limitSummaryText(text string) string {
	const max = 240
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "..."
}

func lastCompactBoundaryID(messages []convcontext.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].CompactBoundaryID) != "" {
			return messages[i].CompactBoundaryID
		}
	}
	return ""
}

func isPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errContextWindowBlocked) ||
		errors.Is(err, errContextCompactCircuitOpen) ||
		errors.Is(err, errInsufficientReduction) ||
		errors.Is(err, errStillAboveAutoThreshold) ||
		errors.Is(err, errStillAboveBlockThreshold) ||
		errors.Is(err, errOversizedProtectedTail) ||
		errors.Is(err, errNotEnoughCompactableHistory) {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"prompt too long",
		"context length",
		"maximum context",
		"too many tokens",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
