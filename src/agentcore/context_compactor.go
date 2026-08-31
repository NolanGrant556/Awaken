package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
)

const (
	compactToolResultTokenThreshold  = 256
	compactSummaryPromptRetryLimit   = 2
	compactSummaryPromptPrunePercent = 0.20
	compactSummaryDefaultMaxTokens   = 4096
	compactSummaryStreamIdleTimeout  = 5 * time.Minute
)

type compactSummaryInvoker func(stdcontext.Context, llm.Config, llm.Request) (string, error)

var (
	errNotEnoughCompactableHistory = errors.New("not enough compactable history")
	errOversizedProtectedTail      = errors.New("oversized protected tail")
)

type compactSummaryBuildResult struct {
	Text                 string
	Fallback             CompactFallback
	ToolResultsPruned    int
	PrunedGroups         int
	PrunedMessages       int
	PrunedDigestMessages int
	SummaryMaxTokens     int
	SummaryRetryCount    int
	Diagnostics          compactSummaryDiagnostics
}

type compactSummaryDiagnostics struct {
	RawChars           int
	ParsedSections     int
	MeaningfulSections int
	QualityIssue       string
	SourceHadUser      bool
	LatestUserPinned   bool
}

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
		req.summaryInvoker = compactSummaryLLMCall
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
	if over, limit := compactTailExceedsBudget(req, tailMessages); over {
		tailMessages = lightenCompactTail(tailMessages)
		if estimateMessagesTokens(tailMessages) > limit {
			var absorbErr error
			prefixMessages, tailMessages, absorbErr = absorbOversizedCompactTail(prefixMessages, tailMessages, limit)
			if absorbErr != nil {
				return CompactResult{}, absorbErr
			}
			prefixGroups = compactSafeGroups(prefixMessages)
		}
	}

	summaryResult := buildSemanticCompactSummary(ctx, req, boundaryID, prefixGroups, tailMessages)
	summary := convcontext.Message{
		Role:              convcontext.RoleUser,
		Content:           buildCompactSummaryMessage(boundaryID, req.Mode, summaryResult, preEstimate),
		CompactBoundaryID: boundaryID,
	}
	tailMessages = stripProviderOpaqueCompactionState(tailMessages)
	compacted := append([]convcontext.Message{summary}, tailMessages...)
	if err := convcontext.ValidateToolResultPairing(compacted); err != nil {
		return CompactResult{}, fmt.Errorf("compact would break tool pairing: %w", err)
	}

	compactedEnvelope := req.Envelope
	compactedEnvelope.Messages = compacted
	postEstimate := EstimateContextEnvelopeTokens(compactedEnvelope)
	return CompactResult{
		Mode:                      req.Mode,
		BoundaryID:                boundaryID,
		SummaryMessage:            summary,
		TailMessages:              cloneContextMessages(tailMessages),
		Messages:                  cloneContextMessages(compacted),
		EstimatedTokens:           postEstimate,
		PreEstimateTokens:         preEstimate,
		PostEstimateTokens:        postEstimate,
		ToolResultsPruned:         summaryResult.ToolResultsPruned,
		ToolResultsPrunedCount:    summaryResult.ToolResultsPruned,
		PrunedMessageCount:        summaryResult.PrunedMessages,
		PrunedSafeGroupCount:      summaryResult.PrunedGroups,
		PrunedDigestMessageCount:  summaryResult.PrunedDigestMessages,
		SummaryMaxTokens:          summaryResult.SummaryMaxTokens,
		SummaryRetryCount:         summaryResult.SummaryRetryCount,
		SummaryRawChars:           summaryResult.Diagnostics.RawChars,
		SummaryParsedSections:     summaryResult.Diagnostics.ParsedSections,
		SummaryMeaningfulSections: summaryResult.Diagnostics.MeaningfulSections,
		SummaryQualityIssue:       summaryResult.Diagnostics.QualityIssue,
		SummarySourceHadUser:      summaryResult.Diagnostics.SourceHadUser,
		SummaryLatestUserPinned:   summaryResult.Diagnostics.LatestUserPinned,
		FallbackMarker:            summaryResult.Fallback.Marker,
		Fallback:                  summaryResult.Fallback,
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

	prefix := cloneCompactGroupSlice(groups[:start])
	tail := cloneCompactGroupSlice(groups[start:])
	if !compactGroupsContainUserBoundary(tail) {
		if anchor, ok := latestCompactUserAnchor(prefix); ok {
			tail = append([][]convcontext.Message{{anchor}}, tail...)
		}
	}
	return prefix, tail
}

func compactGroupsContainUserBoundary(groups [][]convcontext.Message) bool {
	for _, group := range groups {
		for _, message := range group {
			if message.Role == convcontext.RoleUser && message.Origin != convcontext.MessageOriginSynthetic &&
				message.Origin != convcontext.MessageOriginCompactSummary && strings.TrimSpace(message.CompactBoundaryID) == "" {
				return true
			}
		}
	}
	return false
}

func latestCompactUserAnchor(groups [][]convcontext.Message) (convcontext.Message, bool) {
	for groupIndex := len(groups) - 1; groupIndex >= 0; groupIndex-- {
		group := groups[groupIndex]
		for messageIndex := len(group) - 1; messageIndex >= 0; messageIndex-- {
			message := group[messageIndex]
			if message.Role != convcontext.RoleUser || message.Origin == convcontext.MessageOriginSynthetic ||
				message.Origin == convcontext.MessageOriginCompactSummary || strings.TrimSpace(message.CompactBoundaryID) != "" {
				continue
			}
			return cloneContextMessages([]convcontext.Message{message})[0], true
		}
	}
	return convcontext.Message{}, false
}

func cloneCompactGroupSlice(groups [][]convcontext.Message) [][]convcontext.Message {
	out := make([][]convcontext.Message, len(groups))
	for i, group := range groups {
		out[i] = cloneContextMessages(group)
	}
	return out
}

func compactTailExceedsBudget(req CompactRequest, tail []convcontext.Message) (bool, int) {
	if len(tail) == 0 {
		return false, 0
	}
	state := NewContextWindowStateWithConfig(req.Envelope, contextWindowConfigFromModelConfig(req.Envelope.CompactModelConfig))
	limit := req.TargetTokens
	if limit <= 0 || (state.CompactTarget > 0 && state.CompactTarget < limit) {
		limit = state.CompactTarget
	}
	if reserved := state.blockingThreshold() - compactSummaryDefaultMaxTokens; reserved > 0 && reserved < limit {
		limit = reserved
	}
	if user := lastUserMessage(req.Messages); user != nil {
		if userFloor := estimateMessageTokens(*user); userFloor > limit {
			limit = userFloor
		}
	}
	if limit <= 0 {
		return false, limit
	}
	return estimateMessagesTokens(tail) > limit, limit
}

func lightenCompactTail(messages []convcontext.Message) []convcontext.Message {
	out := cloneContextMessages(messages)
	for i := range out {
		stubMessageMedia(&out[i])
	}
	return out
}

func absorbOversizedCompactTail(prefix, tail []convcontext.Message, limit int) ([]convcontext.Message, []convcontext.Message, error) {
	all := append(cloneContextMessages(prefix), cloneContextMessages(tail)...)
	protected := protectedCompactTail(all, tail)
	if estimateMessagesTokens(protected) > limit {
		return nil, nil, errOversizedProtectedTail
	}
	absorbed := messagesExceptProtectedTail(all, protected)
	if len(absorbed) == 0 {
		return nil, nil, errOversizedProtectedTail
	}
	if compactMessagesEqual(absorbed, prefix) && compactMessagesEqual(protected, tail) {
		return nil, nil, errOversizedProtectedTail
	}
	if err := convcontext.ValidateToolResultPairing(append([]convcontext.Message{{
		Role:    convcontext.RoleUser,
		Content: "compact-boundary-placeholder",
	}}, protected...)); err != nil {
		return nil, nil, fmt.Errorf("compact would break tool pairing: %w", err)
	}
	return absorbed, protected, nil
}

func protectedCompactTail(all, tail []convcontext.Message) []convcontext.Message {
	protected := make([]convcontext.Message, 0, 4)
	if user := lastUserMessage(all); user != nil {
		protected = append(protected, *user)
	}
	if pairing := trailingUnclosedPairing(tail); len(pairing) > 0 {
		if len(protected) > 0 && compactMessageIdentity(protected[0], pairing[0]) {
			protected = pairing
		} else {
			protected = append(protected, pairing...)
		}
	}
	return cloneContextMessages(protected)
}

func lastUserMessage(messages []convcontext.Message) *convcontext.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == convcontext.RoleUser && messages[i].Origin != convcontext.MessageOriginSynthetic {
			message := messages[i]
			return &message
		}
	}
	return nil
}

func trailingUnclosedPairing(messages []convcontext.Message) []convcontext.Message {
	groups := compactSafeGroups(messages)
	if len(groups) == 0 {
		return nil
	}
	last := groups[len(groups)-1]
	if isIncompleteToolGroup(last) {
		return cloneContextMessages(last)
	}
	return nil
}

func isIncompleteToolGroup(group []convcontext.Message) bool {
	if len(group) == 0 || group[0].Role != convcontext.RoleAssistant || len(group[0].ToolCalls) == 0 {
		return false
	}
	pending := make(map[string]bool, len(group[0].ToolCalls))
	for _, call := range group[0].ToolCalls {
		pending[call.ID] = true
	}
	for _, message := range group[1:] {
		if message.Role != convcontext.RoleTool {
			return true
		}
		delete(pending, message.ToolCallID)
	}
	return len(pending) > 0
}

func messagesExceptProtectedTail(all, protected []convcontext.Message) []convcontext.Message {
	if len(protected) == 0 {
		return cloneContextMessages(all)
	}
	out := make([]convcontext.Message, 0, len(all))
	for _, message := range all {
		skip := false
		for _, keep := range protected {
			if compactMessageIdentity(message, keep) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, message)
		}
	}
	return cloneContextMessages(out)
}

func compactMessageIdentity(left, right convcontext.Message) bool {
	return left.Role == right.Role &&
		left.Content == right.Content &&
		left.ToolCallID == right.ToolCallID &&
		left.Name == right.Name &&
		left.MessageID == right.MessageID &&
		len(left.ToolCalls) == len(right.ToolCalls)
}

func compactMessagesEqual(left, right []convcontext.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !compactMessageIdentity(left[i], right[i]) {
			return false
		}
	}
	return true
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

func buildSemanticCompactSummary(ctx stdcontext.Context, req CompactRequest, boundaryID string, prefixGroups [][]convcontext.Message, tailMessages []convcontext.Message) compactSummaryBuildResult {
	currentGroups := cloneCompactGroupSlice(prefixGroups)
	prunedGroups := 0
	prunedMessages := 0
	toolResultsPruned := 0
	cfg := llmConfigForCompaction(req.Envelope)
	summaryMaxTokens := compactSummaryMaxTokensForRequest(req)
	prunedDigestMessages := make([]convcontext.Message, 0)

	for attempt := 0; attempt <= compactSummaryPromptRetryLimit; attempt++ {
		messages, currentToolResultsPruned := buildCompactSummaryRequestMessages(currentGroups)
		toolResultsPruned = currentToolResultsPruned
		if len(prunedDigestMessages) > 0 {
			messages = append(cloneContextMessages(prunedDigestMessages), messages...)
		}
		userInstruction := convcontext.Message{
			Role:    convcontext.RoleUser,
			Content: compactSummaryUserInstruction(boundaryID, req.Envelope, messages, tailMessages, req.HookInstruction),
		}
		summaryRequestMessages := append(messages, userInstruction)
		if err := convcontext.ValidateToolResultPairing(summaryRequestMessages); err != nil {
			fallback := CompactFallback{
				Used:   true,
				Marker: "[COMPACT_FALLBACK:tool_pairing_error]",
				Reason: err.Error(),
			}
			return compactFallbackBuildResult(fallback, currentGroups, prunedDigestMessages, tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages, summaryMaxTokens, attempt)
		}

		rawSummary, err := req.summaryInvoker(ctx, cfg, llm.Request{
			SystemPrompt:      compactSummarySystemPrompt(),
			Messages:          summaryRequestMessages,
			Model:             cfg.Model,
			Temperature:       req.Envelope.Temperature,
			MaxTokens:         summaryMaxTokens,
			Stream:            true,
			StreamIdleTimeout: compactSummaryStreamIdleTimeout,
			DisableReasoning:  true,
			Purpose:           llm.RequestPurposeCompactSummary,
		})
		if err == nil {
			if strings.TrimSpace(rawSummary) == "" {
				fallback := CompactFallback{
					Used:   true,
					Marker: "[COMPACT_FALLBACK:empty_summary_response]",
					Reason: "provider returned empty summary content",
				}
				return compactFallbackBuildResult(fallback, currentGroups, prunedDigestMessages, tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages, summaryMaxTokens, attempt)
			}
			normalizedSummary := normalizeCompactSummary(rawSummary)
			normalizedSummary = boundCompactSummaryToBudget(normalizedSummary, summaryMaxTokens)
			diagnostics := compactSummaryDiagnosticsFor(rawSummary, normalizedSummary, messages, userInstruction.Content)
			if diagnostics.QualityIssue != "" {
				fallback := CompactFallback{
					Used:   true,
					Marker: "[COMPACT_FALLBACK:low_information_summary]",
					Reason: diagnostics.QualityIssue,
				}
				return compactFallbackBuildResult(fallback, currentGroups, prunedDigestMessages, tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages, summaryMaxTokens, attempt, diagnostics)
			}
			return compactSummaryBuildResult{
				Text:                 normalizedSummary,
				ToolResultsPruned:    toolResultsPruned,
				PrunedGroups:         prunedGroups,
				PrunedMessages:       prunedMessages,
				PrunedDigestMessages: len(prunedDigestMessages),
				SummaryMaxTokens:     summaryMaxTokens,
				SummaryRetryCount:    attempt,
				Diagnostics:          diagnostics,
			}
		}
		if isPromptTooLongError(err) && attempt < compactSummaryPromptRetryLimit {
			nextGroups, droppedGroups, droppedMessages, droppedDigest := pruneOldestSafeGroups(currentGroups, len(prunedDigestMessages)+1)
			if droppedGroups == 0 {
				break
			}
			prunedDigestMessages = append(prunedDigestMessages, droppedDigest...)
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
		return compactFallbackBuildResult(fallback, currentGroups, prunedDigestMessages, tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages, summaryMaxTokens, attempt)
	}

	fallback := CompactFallback{
		Used:   true,
		Marker: "[COMPACT_FALLBACK:summary_retry_exhausted]",
		Reason: "summary prompt stayed too long after pruning older safe groups",
	}
	return compactFallbackBuildResult(fallback, currentGroups, prunedDigestMessages, tailMessages, req.Envelope, toolResultsPruned, prunedGroups, prunedMessages, summaryMaxTokens, compactSummaryPromptRetryLimit)
}

func compactSummaryLLMCall(ctx stdcontext.Context, cfg llm.Config, req llm.Request) (string, error) {
	req.Stream = true
	req.StreamIdleTimeout = compactSummaryStreamIdleTimeout
	req.DisableReasoning = true
	req.Purpose = llm.RequestPurposeCompactSummary
	resp, err := llm.StreamInvoke(ctx, cfg, req, nil)
	if err != nil {
		if requiredTemperature, ok := llm.RequiredTemperatureFromError(err); ok {
			retryReq := req
			retryReq.TemperatureOverride = &requiredTemperature
			retryReq.OmitTemperature = false
			resp, retryErr := llm.StreamInvoke(ctx, cfg, retryReq, nil)
			if retryErr == nil {
				if resp == nil {
					return "", nil
				}
				return resp.Content, nil
			}
		}
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
}

func (e *runEventEmitter) invokeCompactSummary(ctx stdcontext.Context, cfg llm.Config, req llm.Request) (string, error) {
	auxCfg := cfg
	req.Model = auxCfg.Model
	if auxCfg.MaxTokens > 0 && (req.MaxTokens <= 0 || req.MaxTokens > auxCfg.MaxTokens) {
		req.MaxTokens = auxCfg.MaxTokens
	}
	req.Purpose = llm.RequestPurposeCompactSummary
	req.AffinityScopeID = e.threadID
	req.RequestID = ""
	retryIndex := e.auxiliaryRequestIndex
	e.auxiliaryRequestIndex++
	resp, err := StreamRecordedModel(ctx, ModelCallAuthority{
		ThreadID: e.threadID, RunID: e.runID, TurnID: e.turnID(), SessionID: e.sessionID,
		Recorder: e.recorder, Sink: e.sink,
	}, auxCfg, req, llm.StreamObserver{}, retryIndex, false)
	if err != nil {
		if requiredTemperature, ok := llm.RequiredTemperatureFromError(err); ok {
			retryReq := req
			retryReq.RequestID = ""
			retryReq.TemperatureOverride = &requiredTemperature
			retryReq.OmitTemperature = false
			retryIndex = e.auxiliaryRequestIndex
			e.auxiliaryRequestIndex++
			resp, err = StreamRecordedModel(ctx, ModelCallAuthority{
				ThreadID: e.threadID, RunID: e.runID, TurnID: e.turnID(), SessionID: e.sessionID,
				Recorder: e.recorder, Sink: e.sink,
			}, auxCfg, retryReq, llm.StreamObserver{}, retryIndex, false)
		}
	}
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
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
	return stripImagesFromMessages(flat), toolResultsPruned
}

func structurallyCompactMessage(message convcontext.Message) (convcontext.Message, bool) {
	message.ProviderOpaqueState = nil
	if message.Role != convcontext.RoleTool {
		stripMessageMediaPayloads(&message)
		return message, false
	}
	if estimateMessageTokens(message) <= compactToolResultTokenThreshold {
		stripMessageMediaPayloads(&message)
		return message, false
	}
	message.Content = structuralToolResultDigest(message)
	message.ModelContent = nil
	for i := range message.Attachments {
		message.Attachments[i].DataBase64 = ""
	}
	return message, true
}

func stripProviderOpaqueCompactionState(messages []convcontext.Message) []convcontext.Message {
	out := cloneContextMessages(messages)
	for index := range out {
		out[index].ProviderOpaqueState = nil
	}
	return out
}

func structuralToolResultDigest(message convcontext.Message) string {
	content := strings.TrimSpace(message.Content)
	if content == "" {
		content = "empty tool result"
	}
	parts := []string{
		fmt.Sprintf(
			"[STRUCTURAL_COMPACTION] tool=%s tool_call_id=%s tokens~%d truncated=true",
			firstNonEmptyCompactValue(message.Name, "unknown_tool"),
			firstNonEmptyCompactValue(message.ToolCallID, "unknown_call"),
			estimateMessageTokens(message),
		),
	}
	if files := extractRelevantFiles([]convcontext.Message{message}); len(files) > 0 {
		parts = append(parts, "paths="+strings.Join(files, ", "))
	}
	if jsonFacts := compactJSONTopLevelFacts(content); len(jsonFacts) > 0 {
		parts = append(parts, "json_facts="+strings.Join(jsonFacts, " | "))
	}
	if keyLines := compactInformativeLines(content, 8); len(keyLines) > 0 {
		parts = append(parts, "key_lines:")
		for _, line := range keyLines {
			parts = append(parts, "- "+line)
		}
	}
	head, tail := compactHeadTail(content)
	if head != "" {
		parts = append(parts, "head="+head)
	}
	if tail != "" && tail != head {
		parts = append(parts, "tail="+tail)
	}
	return strings.Join(parts, "\n")
}

func compactJSONTopLevelFacts(content string) []string {
	var value any
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &value); err != nil {
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	facts := make([]string, 0, 8)
	for _, key := range keys {
		facts = append(facts, key+"="+compactJSONValueSummary(object[key]))
		if len(facts) >= 8 {
			break
		}
	}
	return facts
}

func compactJSONValueSummary(value any) string {
	switch typed := value.(type) {
	case string:
		return compactFieldText(typed, 160)
	case float64, bool, nil:
		return fmt.Sprintf("%v", typed)
	case []any:
		return fmt.Sprintf("array(len=%d)", len(typed))
	case map[string]any:
		return fmt.Sprintf("object(keys=%d)", len(typed))
	default:
		return compactFieldText(fmt.Sprintf("%v", typed), 160)
	}
}

func compactInformativeLines(content string, limit int) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, limit)
	seen := make(map[string]struct{})
	for _, line := range lines {
		line = singleLineCompactText(line)
		if line == "" || !isInformativeCompactLine(line) {
			continue
		}
		line = compactFieldText(line, 260)
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func isInformativeCompactLine(line string) bool {
	lower := strings.ToLower(line)
	keywords := []string{
		"error", "failed", "failure", "exception", "traceback", "timeout", "denied",
		"warning", "critical", "important", "canary", "result", "status", "total",
		"count", "path", "file", "version", "todo", "pending", "blocked", "decision",
		"preference", "requirement", "must", "should",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	if strings.Contains(line, ":") || strings.Contains(line, "=") {
		return len([]rune(line)) <= 320
	}
	return false
}

func compactHeadTail(content string) (string, string) {
	normalized := singleLineCompactText(content)
	if normalized == "" {
		return "", ""
	}
	runes := []rune(normalized)
	if len(runes) <= 360 {
		return string(runes), ""
	}
	return string(runes[:360]) + "...", "..." + string(runes[len(runes)-360:])
}

func compactFieldText(text string, maxRunes int) string {
	text = singleLineCompactText(text)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func pruneOldestSafeGroups(groups [][]convcontext.Message, nextDigestIndex int) ([][]convcontext.Message, int, int, []convcontext.Message) {
	if len(groups) <= 1 {
		return groups, 0, 0, nil
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
	return cloneCompactGroupSlice(groups[dropGroups:]), dropGroups, dropMessages, []convcontext.Message{
		{
			Role: convcontext.RoleUser,
			Content: compactPrunedGroupDigest(
				nextDigestIndex,
				groups[:dropGroups],
			),
		},
	}
}

func compactPrunedGroupDigest(index int, groups [][]convcontext.Message) string {
	messages := flattenCompactGroups(groups)
	parts := []string{
		fmt.Sprintf("[PRUNED_SAFE_GROUP_DIGEST index=%d groups=%d messages=%d]", index, len(groups), len(messages)),
	}
	if latestUser := latestMessageContent(messages, convcontext.RoleUser); latestUser != "" {
		parts = append(parts, "latest_user="+compactFieldText(latestUser, 360))
	}
	if files := extractRelevantFiles(messages); len(files) > 0 {
		parts = append(parts, "paths="+strings.Join(files, ", "))
	}
	if tools := collectCompactToolNames(messages); len(tools) > 0 {
		parts = append(parts, "tools="+strings.Join(tools, ", "))
	}
	if keyLines := compactMessagesKeyLines(messages, 12); len(keyLines) > 0 {
		parts = append(parts, "key_lines:")
		for _, line := range keyLines {
			parts = append(parts, "- "+line)
		}
	}
	return strings.Join(parts, "\n")
}

func compactMessagesKeyLines(messages []convcontext.Message, limit int) []string {
	out := make([]string, 0, limit)
	seen := make(map[string]struct{})
	for _, message := range messages {
		lines := compactInformativeLines(message.Content, limit-len(out))
		for _, line := range lines {
			if _, exists := seen[line]; exists {
				continue
			}
			seen[line] = struct{}{}
			out = append(out, line)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
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
		InferenceEndpoint:     modelCfg.InferenceEndpoint,
		Model:                 model,
		Protocol:              llm.Protocol(modelCfg.Protocol),
		ProviderID:            modelCfg.ProviderID,
		ProfileID:             modelCfg.ProfileID,
		CredentialRevision:    modelCfg.CredentialRevision,
		Temperature:           modelCfg.Temperature,
		MaxTokens:             modelCfg.MaxTokens,
		AuthScheme:            modelCfg.AuthScheme,
		ContextWindow:         modelCfg.ContextWindow,
		SupportsPromptCache:   modelCfg.SupportsPromptCache,
		AutoCompactTokenLimit: modelCfg.AutoCompactTokenLimit,
	}
}

func compactSummaryMaxTokensForRequest(req CompactRequest) int {
	if req.SummaryMaxTokens > 0 {
		return req.SummaryMaxTokens
	}
	return compactSummaryDefaultMaxTokens
}

func compactSummaryDiagnosticsFor(rawSummary string, normalizedSummary string, sourceMessages []convcontext.Message, userInstruction string) compactSummaryDiagnostics {
	rawSections := parseCompactSummarySections(compactSummaryPayload(rawSummary))
	parsedSections := 0
	meaningfulSections := 0
	for _, section := range compactSummarySections {
		lines, ok := rawSections[section]
		if !ok {
			continue
		}
		parsedSections++
		if compactSummarySectionHasMeaning(lines) {
			meaningfulSections++
		}
	}

	latestUser := latestMessageContent(sourceMessages, convcontext.RoleUser)
	sourceHadUser := latestUser != ""
	latestUserPinned := false
	if sourceHadUser {
		latestUserPinned = strings.Contains(userInstruction, "Latest older user request that MUST be preserved") &&
			strings.Contains(userInstruction, limitSummaryText(latestUser))
	}

	return compactSummaryDiagnostics{
		RawChars:           len([]rune(rawSummary)),
		ParsedSections:     parsedSections,
		MeaningfulSections: meaningfulSections,
		QualityIssue:       compactSummaryQualityIssue(normalizedSummary, sourceMessages),
		SourceHadUser:      sourceHadUser,
		LatestUserPinned:   latestUserPinned,
	}
}

func compactFallbackBuildResult(fallback CompactFallback, currentGroups [][]convcontext.Message, prunedDigestMessages []convcontext.Message, tailMessages []convcontext.Message, envelope ContextEnvelope, toolResultsPruned int, prunedGroups int, prunedMessages int, summaryMaxTokens int, retryCount int, diagnostics ...compactSummaryDiagnostics) compactSummaryBuildResult {
	compactedMessages := append(flattenCompactGroups(currentGroups), cloneContextMessages(prunedDigestMessages)...)
	diagnostic := compactSummaryDiagnostics{}
	if len(diagnostics) > 0 {
		diagnostic = diagnostics[0]
	}
	return compactSummaryBuildResult{
		Text:                 compactFallbackSummary(fallback.Marker, fallback.Reason, compactedMessages, tailMessages, envelope, toolResultsPruned, prunedGroups, prunedMessages),
		Fallback:             fallback,
		ToolResultsPruned:    toolResultsPruned,
		PrunedGroups:         prunedGroups,
		PrunedMessages:       prunedMessages,
		PrunedDigestMessages: len(prunedDigestMessages),
		SummaryMaxTokens:     summaryMaxTokens,
		SummaryRetryCount:    retryCount,
		Diagnostics:          diagnostic,
	}
}

func buildCompactSummaryMessage(boundaryID string, mode CompactMode, summary compactSummaryBuildResult, preEstimate int) string {
	parts := []string{
		fmt.Sprintf("Conversation compact summary boundary=%s.", boundaryID),
		fmt.Sprintf("Mode: %s.", mode),
		fmt.Sprintf("Pre-compact estimated tokens: %d.", preEstimate),
		fmt.Sprintf("Compact summary max tokens: %d.", summary.SummaryMaxTokens),
		fmt.Sprintf("Compact summary retry count: %d.", summary.SummaryRetryCount),
		fmt.Sprintf("Structural tool results pruned: %d.", summary.ToolResultsPruned),
		fmt.Sprintf("Summary prompt pruned safe groups: %d messages: %d digest_messages: %d.", summary.PrunedGroups, summary.PrunedMessages, summary.PrunedDigestMessages),
	}
	if summary.Fallback.Used {
		parts = append(parts, fmt.Sprintf("Fallback marker: %s.", summary.Fallback.Marker))
		parts = append(parts, "Fallback reason: "+singleLineCompactText(summary.Fallback.Reason)+".")
	}
	parts = append(parts, summary.Text)
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
