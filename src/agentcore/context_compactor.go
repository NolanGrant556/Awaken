package agentcore

import (
	stdcontext "context"
	"errors"
	"fmt"
	"math"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

const (
	compactToolResultTokenThreshold  = 256
	compactSummaryPromptRetryLimit   = 2
	compactSummaryPromptPrunePercent = 0.20
	compactSummaryMaxTokens          = 1200
)

type compactSummaryInvoker func(stdcontext.Context, llm.Config, llm.Request) (string, error)

var errNotEnoughCompactableHistory = errors.New("not enough compactable history")

func CompactConversationWithRequest(ctx stdcontext.Context, req CompactRequest) (CompactResult, error) {
	if len(req.Messages) == 0 {
		return CompactResult{}, nil
	}
	if req.Mode == "" {
		req.Mode = CompactModeAuto
	}
	if req.TargetTokens <= 0 {
		req.TargetTokens = defaultModelWindow / 3
	}
	if req.summaryInvoker == nil {
		req.summaryInvoker = llm.Call
	}

	boundaryID := newAwakenID("compact")
	preEstimate := EstimateContextEnvelopeTokens(req.Envelope)
	prefixGroups, tailGroups := splitCompactGroups(req.Messages, req.TargetTokens)
	prefixMessages := flattenCompactGroups(prefixGroups)
	tailMessages := flattenCompactGroups(tailGroups)
	if len(prefixMessages) == 0 && req.Mode == CompactModeReactive && len(req.Messages) > 0 {
		prefixGroups = [][]convcontext.Message{cloneContextMessages(req.Messages)}
		prefixMessages = cloneContextMessages(req.Messages)
		tailMessages = nil
	}
	if len(prefixMessages) == 0 {
		return CompactResult{}, errNotEnoughCompactableHistory
	}

	summaryText, fallback, toolResultsPruned, prunedGroups, prunedMessages := buildSemanticCompactSummary(ctx, req, boundaryID, prefixGroups, tailMessages)
	summary := convcontext.Message{
		Role:              convcontext.RoleUser,
		Content:           buildCompactSummaryMessage(boundaryID, req.Mode, summaryText, fallback, preEstimate, toolResultsPruned, prunedGroups, prunedMessages),
		CompactBoundaryID: boundaryID,
	}
	compacted := append([]convcontext.Message{summary}, tailMessages...)
	if err := convcontext.ValidateToolResultPairing(compacted); err != nil {
		return CompactResult{}, fmt.Errorf("compact would break tool pairing: %w", err)
	}

	compactedEnvelope := req.Envelope
	compactedEnvelope.Messages = compacted
	postEstimate := EstimateContextEnvelopeTokens(compactedEnvelope)
	return CompactResult{
		Mode:                 req.Mode,
		BoundaryID:           boundaryID,
		SummaryMessage:       summary,
		TailMessages:         cloneContextMessages(tailMessages),
		Messages:             cloneContextMessages(compacted),
		EstimatedTokens:      postEstimate,
		PreEstimateTokens:    preEstimate,
		PostEstimateTokens:   postEstimate,
		ToolResultsPruned:    toolResultsPruned,
		PrunedMessageCount:   prunedMessages,
		PrunedSafeGroupCount: prunedGroups,
		Fallback:             fallback,
	}, nil
}

func splitCompactGroups(messages []convcontext.Message, targetTokens int) ([][]convcontext.Message, [][]convcontext.Message) {
	groups := compactSafeGroups(messages)
	if len(groups) == 0 {
		return nil, nil
	}
	if targetTokens <= 0 {
		targetTokens = defaultModelWindow / 3
	}

	start := len(groups)
	total := 0
	for i := len(groups) - 1; i >= 0; i-- {
		groupTokens := estimateMessagesTokens(groups[i])
		if start < len(groups) && total+groupTokens > targetTokens {
			break
		}
		start = i
		total += groupTokens
	}
	if start == len(groups) {
		start = len(groups) - 1
	}
	if start == 0 && len(groups) > 1 {
		start = 1
	}

	return cloneCompactGroupSlice(groups[:start]), cloneCompactGroupSlice(groups[start:])
}

func cloneCompactGroupSlice(groups [][]convcontext.Message) [][]convcontext.Message {
	out := make([][]convcontext.Message, len(groups))
	for i, group := range groups {
		out[i] = cloneContextMessages(group)
	}
	return out
}

func flattenCompactGroups(groups [][]convcontext.Message) []convcontext.Message {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	flat := make([]convcontext.Message, 0, total)
	for _, group := range groups {
		flat = append(flat, cloneContextMessages(group)...)
	}
	return flat
}

func buildSemanticCompactSummary(ctx stdcontext.Context, req CompactRequest, boundaryID string, prefixGroups [][]convcontext.Message, tailMessages []convcontext.Message) (string, CompactFallback, int, int, int) {
	currentGroups := cloneCompactGroupSlice(prefixGroups)
	prunedGroups := 0
	prunedMessages := 0
	toolResultsPruned := 0
	cfg := llmConfigForCompaction(req.Envelope)

	for attempt := 0; attempt <= compactSummaryPromptRetryLimit; attempt++ {
		messages, currentToolResultsPruned := buildCompactSummaryRequestMessages(currentGroups)
		toolResultsPruned = currentToolResultsPruned
		userInstruction := convcontext.Message{
			Role:    convcontext.RoleUser,
			Content: compactSummaryUserInstruction(boundaryID, req.Envelope, tailMessages),
		}
		summaryRequestMessages := append(messages, userInstruction)
		if err := convcontext.ValidateToolResultPairing(summaryRequestMessages); err != nil {
			fallback := CompactFallback{
				Used:   true,
				Marker: "[COMPACT_FALLBACK:tool_pairing_error]",
				Reason: err.Error(),
			}
			return compactFallbackSummary(fallback.Marker, fallback.Reason, flattenCompactGroups(currentGroups), tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages), fallback, toolResultsPruned, prunedGroups, prunedMessages
		}

		reqMaxTokens := req.Envelope.MaxTokens
		if reqMaxTokens <= 0 || reqMaxTokens > compactSummaryMaxTokens {
			reqMaxTokens = compactSummaryMaxTokens
		}
		rawSummary, err := req.summaryInvoker(ctx, cfg, llm.Request{
			SystemPrompt: compactSummarySystemPrompt(),
			Messages:     summaryRequestMessages,
			Model:        cfg.Model,
			Temperature:  req.Envelope.Temperature,
			MaxTokens:    reqMaxTokens,
		})
		if err == nil {
			if strings.TrimSpace(rawSummary) == "" {
				fallback := CompactFallback{
					Used:   true,
					Marker: "[COMPACT_FALLBACK:empty_summary_response]",
					Reason: "provider returned empty summary content",
				}
				return compactFallbackSummary(fallback.Marker, fallback.Reason, flattenCompactGroups(currentGroups), tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages), fallback, toolResultsPruned, prunedGroups, prunedMessages
			}
			return normalizeCompactSummary(rawSummary), CompactFallback{}, toolResultsPruned, prunedGroups, prunedMessages
		}
		if isPromptTooLongError(err) && attempt < compactSummaryPromptRetryLimit {
			nextGroups, droppedGroups, droppedMessages := pruneOldestSafeGroups(currentGroups)
			if droppedGroups == 0 {
				break
			}
			currentGroups = nextGroups
			prunedGroups += droppedGroups
			prunedMessages += droppedMessages
			continue
		}

		fallback := CompactFallback{
			Used:   true,
			Marker: "[COMPACT_FALLBACK:llm_summary_failed]",
			Reason: err.Error(),
		}
		return compactFallbackSummary(fallback.Marker, fallback.Reason, flattenCompactGroups(currentGroups), tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages), fallback, toolResultsPruned, prunedGroups, prunedMessages
	}

	fallback := CompactFallback{
		Used:   true,
		Marker: "[COMPACT_FALLBACK:summary_retry_exhausted]",
		Reason: "summary prompt stayed too long after pruning older safe groups",
	}
	return compactFallbackSummary(fallback.Marker, fallback.Reason, flattenCompactGroups(currentGroups), tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages), fallback, toolResultsPruned, prunedGroups, prunedMessages
}

func buildCompactSummaryRequestMessages(groups [][]convcontext.Message) ([]convcontext.Message, int) {
	flat := make([]convcontext.Message, 0)
	toolResultsPruned := 0
	for _, group := range groups {
		for _, message := range group {
			compactedMessage, compacted := structurallyCompactMessage(message)
			if compacted {
				toolResultsPruned++
			}
			flat = append(flat, compactedMessage)
		}
	}
	return flat, toolResultsPruned
}

func structurallyCompactMessage(message convcontext.Message) (convcontext.Message, bool) {
	if message.Role != convcontext.RoleTool {
		return message, false
	}
	if estimateMessageTokens(message) <= compactToolResultTokenThreshold {
		return message, false
	}
	preview := limitSummaryText(singleLineCompactText(message.Content))
	if preview == "" {
		preview = "empty tool result"
	}
	message.Content = fmt.Sprintf("[STRUCTURAL_COMPACTION] tool=%s tool_call_id=%s tokens~%d preview=%s",
		firstNonEmptyCompactValue(message.Name, "unknown_tool"),
		firstNonEmptyCompactValue(message.ToolCallID, "unknown_call"),
		estimateMessageTokens(message),
		preview,
	)
	return message, true
}

func pruneOldestSafeGroups(groups [][]convcontext.Message) ([][]convcontext.Message, int, int) {
	if len(groups) <= 1 {
		return groups, 0, 0
	}
	dropGroups := int(math.Ceil(float64(len(groups)) * compactSummaryPromptPrunePercent))
	if dropGroups < 1 {
		dropGroups = 1
	}
	if dropGroups >= len(groups) {
		dropGroups = len(groups) - 1
	}
	dropMessages := 0
	for _, group := range groups[:dropGroups] {
		dropMessages += len(group)
	}
	return cloneCompactGroupSlice(groups[dropGroups:]), dropGroups, dropMessages
}

func llmConfigForCompaction(envelope ContextEnvelope) llm.Config {
	modelCfg := envelope.CompactModelConfig
	model := modelCfg.Model
	if strings.TrimSpace(model) == "" {
		model = envelope.Model
	}
	return llm.Config{
		APIKey:                modelCfg.APIKey,
		BaseURL:               modelCfg.BaseURL,
		Model:                 model,
		Protocol:              llm.Protocol(modelCfg.Protocol),
		Temperature:           modelCfg.Temperature,
		MaxTokens:             modelCfg.MaxTokens,
		AuthScheme:            modelCfg.AuthScheme,
		ContextWindow:         modelCfg.ContextWindow,
		AutoCompactTokenLimit: modelCfg.AutoCompactTokenLimit,
	}
}

func buildCompactSummaryMessage(boundaryID string, mode CompactMode, summary string, fallback CompactFallback, preEstimate int, toolResultsPruned int, prunedGroups int, prunedMessages int) string {
	parts := []string{
		fmt.Sprintf("Conversation compact summary boundary=%s.", boundaryID),
		fmt.Sprintf("Mode: %s.", mode),
		fmt.Sprintf("Pre-compact estimated tokens: %d.", preEstimate),
		fmt.Sprintf("Structural tool results pruned: %d.", toolResultsPruned),
		fmt.Sprintf("Summary prompt pruned safe groups: %d messages: %d.", prunedGroups, prunedMessages),
	}
	if fallback.Used {
		parts = append(parts, fmt.Sprintf("Fallback marker: %s.", fallback.Marker))
		parts = append(parts, "Fallback reason: "+singleLineCompactText(fallback.Reason)+".")
	}
	parts = append(parts, summary)
	return strings.Join(parts, "\n")
}

func firstNonEmptyCompactValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
