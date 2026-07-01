package agentcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
)

const (
	defaultModelWindow      = 128000
	defaultOutputReserve    = 16000
	defaultBlockingReserved = 4000
)

var errContextWindowBlocked = errors.New("context window blocked")

type ContextWindowState struct {
	ModelWindow                 int
	OutputReserve               int
	EffectiveWindow             int
	EstimatedTokens             int
	WarningThreshold            int
	AutoCompactThreshold        int
	CompactTarget               int
	LastAPIUsageTokens          int
	EstimatedDeltaTokens        int
	LastCompactBoundaryID       string
	ConsecutiveCompactFailures  int
	HasAttemptedReactiveCompact bool
}

type CompactResult struct {
	BoundaryID      string
	SummaryMessage  convcontext.Message
	TailMessages    []convcontext.Message
	Messages        []convcontext.Message
	EstimatedTokens int
}

func NewContextWindowState(envelope ContextEnvelope) ContextWindowState {
	modelWindow := defaultModelWindow
	outputReserve := defaultOutputReserve
	effectiveWindow := modelWindow - outputReserve
	if effectiveWindow < 0 {
		effectiveWindow = 0
	}

	state := ContextWindowState{
		ModelWindow:           modelWindow,
		OutputReserve:         outputReserve,
		EffectiveWindow:       effectiveWindow,
		WarningThreshold:      int(float64(effectiveWindow) * 0.85),
		AutoCompactThreshold:  int(float64(effectiveWindow) * 0.90),
		CompactTarget:         int(float64(effectiveWindow) * 0.30),
		EstimatedTokens:       EstimateContextEnvelopeTokens(envelope),
		EstimatedDeltaTokens:  estimateContextDeltaTokens(envelope),
		LastCompactBoundaryID: lastCompactBoundaryID(envelope.Messages),
	}
	return state
}

func (state ContextWindowState) ShouldWarn() bool {
	return state.WarningThreshold > 0 && state.EstimatedTokens >= state.WarningThreshold
}

func (state ContextWindowState) ShouldAutoCompact() bool {
	return state.AutoCompactThreshold > 0 && state.EstimatedTokens >= state.AutoCompactThreshold
}

func (state ContextWindowState) ShouldBlock() bool {
	return state.blockingThreshold() > 0 && state.EstimatedTokens >= state.blockingThreshold()
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
		"context tokens estimated=%d effective=%d warning=%d autocompact=%d compact_target=%d",
		state.EstimatedTokens,
		state.EffectiveWindow,
		state.WarningThreshold,
		state.AutoCompactThreshold,
		state.CompactTarget,
	)
}

func EstimateContextEnvelopeTokens(envelope ContextEnvelope) int {
	total := 0
	for _, block := range envelope.SystemBlocks {
		total += convcontext.EstimateTokens(block.Content)
	}
	for _, item := range envelope.UserContextItems {
		total += convcontext.EstimateTokens(item.Content)
	}
	for _, item := range envelope.SystemContextItems {
		total += convcontext.EstimateTokens(item.Content)
	}
	for _, attachment := range envelope.Attachments {
		total += convcontext.EstimateTokens(attachment.Kind)
		total += convcontext.EstimateTokens(attachment.Path)
		total += convcontext.EstimateTokens(attachment.Content)
	}
	for _, message := range envelope.Messages {
		total += estimateMessageTokens(message)
	}
	if len(envelope.ToolSchemas) > 0 {
		data, err := json.Marshal(envelope.ToolSchemas)
		if err == nil {
			total += convcontext.EstimateTokens(string(data))
		}
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
	total += convcontext.EstimateTokens(message.Name)
	total += convcontext.EstimateTokens(message.ToolCallID)
	for _, toolCall := range message.ToolCalls {
		total += convcontext.EstimateTokens(toolCall.ID)
		total += convcontext.EstimateTokens(toolCall.Function.Name)
		total += convcontext.EstimateTokens(toolCall.Function.Arguments)
	}
	return total
}

func CompactConversation(messages []convcontext.Message, envelope ContextEnvelope, targetTokens int) (CompactResult, error) {
	if len(messages) == 0 {
		return CompactResult{}, nil
	}
	if targetTokens <= 0 {
		targetTokens = defaultModelWindow / 3
	}

	boundaryID := newAwakenID("compact")
	tailMessages := selectCompactTail(messages, targetTokens)
	summary := convcontext.Message{
		Role:              convcontext.RoleUser,
		Content:           buildCompactSummary(boundaryID, messages, envelope),
		CompactBoundaryID: boundaryID,
	}
	compacted := append([]convcontext.Message{summary}, tailMessages...)
	if err := convcontext.ValidateToolResultPairing(compacted); err != nil {
		return CompactResult{}, fmt.Errorf("compact would break tool pairing: %w", err)
	}
	compactedEnvelope := envelope
	compactedEnvelope.Messages = compacted
	return CompactResult{
		BoundaryID:      boundaryID,
		SummaryMessage:  summary,
		TailMessages:    cloneContextMessages(tailMessages),
		Messages:        cloneContextMessages(compacted),
		EstimatedTokens: EstimateContextEnvelopeTokens(compactedEnvelope),
	}, nil
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
		if messages[i].Role == convcontext.RoleUser && strings.TrimSpace(messages[i].Content) != "" {
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
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"prompt too long",
		"context length",
		"maximum context",
		"context window",
		"too many tokens",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
