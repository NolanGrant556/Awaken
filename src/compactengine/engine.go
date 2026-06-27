package compactengine

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type Engine struct {
	config     Config
	summarizer Summarizer
}

func New(config Config, summarizer Summarizer) Engine {
	config = config.withDefaults()
	if summarizer == nil {
		summarizer = FallbackSummarizer{}
	}
	return Engine{config: config, summarizer: summarizer}
}

func (e Engine) Plan(messages []Message, reason CompactionReason) CompactionPlan {
	return Plan(messages, e.config, reason)
}

func (e Engine) Compact(messages []Message, reason CompactionReason) (CompactionResult, error) {
	plan := e.Plan(messages, reason)
	if !plan.ShouldCompact {
		metrics := CompactionMetrics{
			OriginalTokens:      plan.OriginalTokens,
			PostCompactTokens:   plan.OriginalTokens,
			OriginalMessages:    len(messages),
			PostCompactMessages: len(messages),
			CompressionRatio:    1,
			StillOverLimit:      plan.StillOverLimit,
		}
		return CompactionResult{
			Plan:           plan,
			Messages:       cloneMessages(messages),
			Metrics:        metrics,
			StillOverLimit: plan.StillOverLimit,
		}, nil
	}

	material := BuildSummaryMaterial(messages, plan)
	summary, err := e.summarizer.Summarize(SummaryInput{
		Plan:     plan,
		Messages: cloneMessages(messages[plan.StartIndex:plan.EndIndex]),
		Material: material,
	})
	if err != nil {
		return CompactionResult{}, err
	}
	summary = EnsureSummaryPrefix(summary)

	summaryTokens := EstimateTokens(summary)
	boundary := CompactBoundaryMetadata{
		ID:               compactBoundaryID(plan),
		Reason:           plan.Reason,
		StartIndex:       plan.StartIndex,
		EndIndex:         plan.EndIndex,
		SourceIDs:        cloneStrings(plan.SelectedSourceIDs),
		PreCompactTokens: plan.OriginalTokens,
		SelectedTokens:   plan.SelectedTokens,
		SummaryTokens:    summaryTokens,
		ProtectedIndices: cloneInts(plan.ProtectedIndices),
	}

	boundaryMessage := Message{
		ID:            boundary.ID,
		SourceID:      boundary.ID,
		Role:          RoleSystem,
		Content:       fmt.Sprintf("[compact boundary: %s %d-%d]", boundary.Reason, boundary.StartIndex, boundary.EndIndex),
		TokenEstimate: e.config.BoundaryTokenEstimate,
		CompactBoundary: &CompactBoundaryMetadata{
			ID:               boundary.ID,
			Reason:           boundary.Reason,
			StartIndex:       boundary.StartIndex,
			EndIndex:         boundary.EndIndex,
			SourceIDs:        cloneStrings(boundary.SourceIDs),
			PreCompactTokens: boundary.PreCompactTokens,
			SelectedTokens:   boundary.SelectedTokens,
			SummaryTokens:    boundary.SummaryTokens,
			ProtectedIndices: cloneInts(boundary.ProtectedIndices),
		},
	}
	summaryMessage := Message{
		ID:               boundary.ID + ":summary",
		SourceID:         boundary.ID + ":summary",
		Role:             RoleUser,
		Content:          summary,
		TokenEstimate:    summaryTokens,
		IsCompactSummary: true,
	}

	nextMessages := make([]Message, 0, len(messages)-(plan.EndIndex-plan.StartIndex)+2)
	nextMessages = append(nextMessages, cloneMessages(messages[:plan.StartIndex])...)
	nextMessages = append(nextMessages, boundaryMessage, summaryMessage)
	nextMessages = append(nextMessages, cloneMessages(messages[plan.EndIndex:])...)

	postTokens := CountTokens(nextMessages)
	boundary.PostCompactTokens = postTokens
	if boundaryMessage.CompactBoundary != nil {
		boundaryMessage.CompactBoundary.PostCompactTokens = postTokens
		nextMessages[plan.StartIndex] = boundaryMessage
	}

	metrics := CompactionMetrics{
		OriginalTokens:      plan.OriginalTokens,
		PostCompactTokens:   postTokens,
		TokensSaved:         plan.OriginalTokens - postTokens,
		SummaryTokens:       summaryTokens,
		OriginalMessages:    len(messages),
		PostCompactMessages: len(nextMessages),
		CompactedMessages:   plan.EndIndex - plan.StartIndex,
		WasCompacted:        true,
		StillOverLimit:      postTokens > plan.TargetTokenBudget,
	}
	if plan.OriginalTokens > 0 {
		metrics.CompressionRatio = float64(postTokens) / float64(plan.OriginalTokens)
	}

	return CompactionResult{
		Plan:           plan,
		Messages:       nextMessages,
		Boundary:       boundary,
		SummaryMessage: summaryMessage,
		Metrics:        metrics,
		StillOverLimit: metrics.StillOverLimit,
	}, nil
}

func Plan(messages []Message, config Config, reason CompactionReason) CompactionPlan {
	config = config.withDefaults()
	reason = normalizeReason(reason)
	totalTokens := CountTokens(messages)
	targetBudget := config.targetBudget()
	base := CompactionPlan{
		Reason:                reason,
		StartIndex:            -1,
		EndIndex:              -1,
		OriginalTokens:        totalTokens,
		TargetTokenBudget:     targetBudget,
		ContextWindowTokens:   config.ContextWindowTokens,
		SummaryTokenEstimate:  config.SummaryTokenEstimate,
		BoundaryTokenEstimate: config.BoundaryTokenEstimate,
	}

	if targetBudget <= 0 {
		base.SkipReason = SkipReasonMissingBudget
		return base
	}
	if totalTokens <= targetBudget {
		base.SkipReason = SkipReasonUnderBudget
		return base
	}

	groups := buildMessageGroups(messages)
	protected := findProtectedIndices(messages, config.LastTurnsToKeep)
	base.ProtectedIndices = sortedIndices(protected)
	base.ProtectedSourceIDs = sourceIDsForIndices(messages, base.ProtectedIndices)

	indexToGroup := map[int]int{}
	for groupIndex, group := range groups {
		for i := group.start; i < group.end; i++ {
			indexToGroup[i] = groupIndex
		}
	}

	headGroup := -1
	tailGroup := len(groups)
	for idx, reasons := range protected {
		groupIndex, ok := indexToGroup[idx]
		if !ok {
			continue
		}
		if containsReason(reasons, "last_n") {
			if groupIndex < tailGroup {
				tailGroup = groupIndex
			}
			continue
		}
		if groupIndex > headGroup {
			headGroup = groupIndex
		}
	}

	compressStartGroup := headGroup + 1
	compressEndGroup := tailGroup
	if compressStartGroup >= compressEndGroup {
		base.SkipReason = SkipReasonNoCompressibleRegion
		base.StillOverLimit = true
		return base
	}

	tokensToSave := totalTokens - targetBudget
	targetSelectedTokens := tokensToSave + config.SummaryTokenEstimate + config.BoundaryTokenEstimate
	selectedStartGroup, selectedEndGroup, selectedTokens := chooseMiddleGroups(
		groups,
		compressStartGroup,
		compressEndGroup,
		targetSelectedTokens,
	)
	if selectedStartGroup < 0 || selectedEndGroup <= selectedStartGroup {
		base.SkipReason = SkipReasonNoCompressibleRegion
		base.StillOverLimit = true
		return base
	}

	estimatedPostTokens := totalTokens - selectedTokens + config.SummaryTokenEstimate + config.BoundaryTokenEstimate
	if selectedTokens <= config.SummaryTokenEstimate+config.BoundaryTokenEstimate {
		base.SkipReason = SkipReasonNoTokenSavings
		base.StillOverLimit = estimatedPostTokens > targetBudget
		return base
	}

	startIndex := groups[selectedStartGroup].start
	endIndex := groups[selectedEndGroup-1].end
	base.ShouldCompact = true
	base.StartIndex = startIndex
	base.EndIndex = endIndex
	base.SkipReason = SkipReasonNone
	base.TokensToSave = tokensToSave
	base.SelectedTokens = selectedTokens
	base.EstimatedPostTokens = estimatedPostTokens
	base.StillOverLimit = estimatedPostTokens > targetBudget
	base.SelectedSourceIDs = sourceIDsForRange(messages, startIndex, endIndex)
	return base
}

func BuildSummaryMaterial(messages []Message, plan CompactionPlan) string {
	if !plan.ShouldCompact || plan.StartIndex < 0 || plan.EndIndex > len(messages) || plan.StartIndex >= plan.EndIndex {
		return ""
	}

	var b strings.Builder
	for i := plan.StartIndex; i < plan.EndIndex; i++ {
		msg := messages[i]
		sourceID := msg.SourceID
		if sourceID == "" {
			sourceID = msg.ID
		}
		if sourceID == "" {
			sourceID = fmt.Sprintf("message-%d", i)
		}
		fmt.Fprintf(&b, "[%d] role=%s source=%s\n", i, msg.Role, sourceID)
		if strings.TrimSpace(msg.Content) != "" {
			b.WriteString(msg.Content)
			b.WriteByte('\n')
		}
		for _, call := range msg.ToolCalls {
			fmt.Fprintf(&b, "[tool_call:%s name=%s]\n", call.ID, call.Name)
			if strings.TrimSpace(call.Arguments) != "" {
				fmt.Fprintf(&b, "arguments: %s\n", call.Arguments)
			}
		}
		if msg.ToolResult != nil {
			fmt.Fprintf(&b, "[tool_result:%s name=%s error=%t]\n", msg.ToolResult.ID, msg.ToolResult.Name, msg.ToolResult.IsError)
		}
		for _, attachment := range msg.Attachments {
			fmt.Fprintf(&b, "[attachment:%s id=%s name=%s]\n", attachment.Kind, attachment.ID, attachment.Name)
		}
		for _, media := range msg.Media {
			fmt.Fprintf(&b, "%s\n", mediaMarkerText(media))
		}
		if msg.CompactBoundary != nil {
			fmt.Fprintf(&b, "[compact_boundary:%s reason=%s]\n", msg.CompactBoundary.ID, msg.CompactBoundary.Reason)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

type FallbackSummarizer struct {
	Prefix string
}

func (s FallbackSummarizer) Summarize(input SummaryInput) (string, error) {
	prefix := s.Prefix
	if strings.TrimSpace(prefix) == "" {
		prefix = FallbackSummaryPrefix
	}
	sourceIDs := input.Plan.SelectedSourceIDs
	if len(sourceIDs) == 0 {
		sourceIDs = sourceIDsForMessages(input.Messages)
	}
	details := "earlier conversation turns were compacted to preserve context"
	if len(sourceIDs) > 0 {
		details = fmt.Sprintf("compacted %d message(s) from sources: %s", len(input.Messages), strings.Join(sourceIDs, ", "))
	}
	return prefix + " " + details + ".", nil
}

func EnsureSummaryPrefix(summary string) string {
	text := strings.TrimSpace(summary)
	if text == "" {
		return FallbackSummaryPrefix
	}
	if strings.HasPrefix(text, FallbackSummaryPrefix) {
		return text
	}
	return FallbackSummaryPrefix + " " + text
}

func CountTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += msg.EstimatedTokens()
	}
	return total
}

func (m Message) EstimatedTokens() int {
	if m.TokenEstimate > 0 {
		return m.TokenEstimate
	}
	total := EstimateTokens(m.Content)
	for _, call := range m.ToolCalls {
		if call.TokenEstimate > 0 {
			total += call.TokenEstimate
			continue
		}
		total += EstimateTokens(call.ID)
		total += EstimateTokens(call.Name)
		total += EstimateTokens(call.Arguments)
	}
	if m.ToolResult != nil {
		if m.ToolResult.TokenEstimate > 0 {
			total += m.ToolResult.TokenEstimate
		} else {
			total += EstimateTokens(m.ToolResult.ID)
			total += EstimateTokens(m.ToolResult.Name)
		}
	}
	for _, attachment := range m.Attachments {
		if attachment.TokenEstimate > 0 {
			total += attachment.TokenEstimate
		} else {
			total += EstimateTokens(attachment.ID)
			total += EstimateTokens(attachment.Kind)
			total += EstimateTokens(attachment.Name)
		}
	}
	total += len(m.Media)
	if m.CompactBoundary != nil {
		total++
	}
	return total
}

func EstimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	words := len(strings.Fields(trimmed))
	if words == 0 {
		return 0
	}
	return int(math.Ceil(float64(words) * 4 / 3))
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.SummaryTokenEstimate <= 0 {
		c.SummaryTokenEstimate = defaults.SummaryTokenEstimate
	}
	if c.BoundaryTokenEstimate <= 0 {
		c.BoundaryTokenEstimate = defaults.BoundaryTokenEstimate
	}
	if c.LastTurnsToKeep <= 0 {
		c.LastTurnsToKeep = defaults.LastTurnsToKeep
	}
	return c
}

func (c Config) targetBudget() int {
	if c.TokenBudget > 0 {
		return c.TokenBudget
	}
	return c.ContextWindowTokens
}

type messageGroup struct {
	start  int
	end    int
	tokens int
}

func buildMessageGroups(messages []Message) []messageGroup {
	groups := make([]messageGroup, 0, len(messages))
	for i := 0; i < len(messages); {
		end := i + 1
		if messages[i].Role == RoleAssistant && len(messages[i].ToolCalls) > 0 {
			pending := map[string]struct{}{}
			for _, call := range messages[i].ToolCalls {
				if call.ID != "" {
					pending[call.ID] = struct{}{}
				}
			}
			for end < len(messages) && messages[end].Role == RoleTool {
				if messages[end].ToolResult != nil && messages[end].ToolResult.ID != "" {
					delete(pending, messages[end].ToolResult.ID)
				}
				end++
				if len(pending) == 0 {
					break
				}
			}
		}
		groups = append(groups, messageGroup{
			start:  i,
			end:    end,
			tokens: CountTokens(messages[i:end]),
		})
		i = end
	}
	return groups
}

func findProtectedIndices(messages []Message, lastN int) map[int][]string {
	protected := map[int][]string{}
	add := func(index int, reason string) {
		if index < 0 || index >= len(messages) {
			return
		}
		protected[index] = append(protected[index], reason)
	}

	firstSystem := -1
	firstUser := -1
	firstAssistant := -1
	firstTool := -1
	for i, msg := range messages {
		if firstSystem == -1 && msg.Role == RoleSystem {
			firstSystem = i
		}
		if firstUser == -1 && msg.Role == RoleUser {
			firstUser = i
		}
		if firstAssistant == -1 && msg.Role == RoleAssistant {
			firstAssistant = i
		}
		if firstTool == -1 && (msg.Role == RoleTool || msg.ToolResult != nil) {
			firstTool = i
		}
		if msg.CompactBoundary != nil {
			add(i, "compact_boundary")
		}
	}
	add(firstSystem, "first_system")
	add(firstUser, "first_user")
	add(firstAssistant, "first_assistant")
	add(firstTool, "first_tool")

	if lastN > 0 {
		for i := max(0, len(messages)-lastN); i < len(messages); i++ {
			add(i, "last_n")
		}
	}
	return protected
}

func chooseMiddleGroups(groups []messageGroup, startGroup, endGroup, targetTokens int) (int, int, int) {
	if startGroup >= endGroup {
		return -1, -1, 0
	}
	center := startGroup + (endGroup-startGroup-1)/2
	left := center
	right := center + 1
	selectedTokens := groups[center].tokens

	for selectedTokens < targetTokens && (left > startGroup || right < endGroup) {
		if left <= startGroup {
			selectedTokens += groups[right].tokens
			right++
			continue
		}
		if right >= endGroup {
			left--
			selectedTokens += groups[left].tokens
			continue
		}

		leftSpan := center - left
		rightSpan := right - center - 1
		if leftSpan <= rightSpan {
			left--
			selectedTokens += groups[left].tokens
		} else {
			selectedTokens += groups[right].tokens
			right++
		}
	}

	return left, right, selectedTokens
}

func compactBoundaryID(plan CompactionPlan) string {
	return fmt.Sprintf("compact:%s:%d:%d", plan.Reason, plan.StartIndex, plan.EndIndex)
}

func normalizeReason(reason CompactionReason) CompactionReason {
	if reason == "" {
		return CompactionReasonManual
	}
	return reason
}

func mediaMarkerText(media MediaMarker) string {
	kind := media.Kind
	if kind == "" {
		kind = MediaUnknown
	}
	id := media.ID
	if id == "" {
		id = "unknown"
	}
	if strings.TrimSpace(media.Description) == "" {
		return fmt.Sprintf("[%s:%s]", kind, id)
	}
	return fmt.Sprintf("[%s:%s %s]", kind, id, media.Description)
}

func containsReason(reasons []string, reason string) bool {
	for _, candidate := range reasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

func sortedIndices(indices map[int][]string) []int {
	result := make([]int, 0, len(indices))
	for index := range indices {
		result = append(result, index)
	}
	sort.Ints(result)
	return result
}

func sourceIDsForIndices(messages []Message, indices []int) []string {
	sourceIDs := make([]string, 0, len(indices))
	for _, index := range indices {
		if index < 0 || index >= len(messages) {
			continue
		}
		sourceIDs = append(sourceIDs, sourceIDForMessage(messages[index], index))
	}
	return sourceIDs
}

func sourceIDsForRange(messages []Message, start, end int) []string {
	sourceIDs := make([]string, 0, end-start)
	for i := start; i < end && i < len(messages); i++ {
		sourceIDs = append(sourceIDs, sourceIDForMessage(messages[i], i))
	}
	return sourceIDs
}

func sourceIDsForMessages(messages []Message) []string {
	sourceIDs := make([]string, 0, len(messages))
	for i, msg := range messages {
		sourceIDs = append(sourceIDs, sourceIDForMessage(msg, i))
	}
	return sourceIDs
}

func sourceIDForMessage(msg Message, index int) string {
	if msg.SourceID != "" {
		return msg.SourceID
	}
	if msg.ID != "" {
		return msg.ID
	}
	return fmt.Sprintf("message-%d", index)
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]Message, len(messages))
	for i, msg := range messages {
		result[i] = cloneMessage(msg)
	}
	return result
}

func cloneMessage(msg Message) Message {
	next := msg
	next.ToolCalls = append([]ToolCallMarker(nil), msg.ToolCalls...)
	if msg.ToolResult != nil {
		toolResult := *msg.ToolResult
		next.ToolResult = &toolResult
	}
	next.Attachments = append([]AttachmentMarker(nil), msg.Attachments...)
	next.Media = append([]MediaMarker(nil), msg.Media...)
	if msg.CompactBoundary != nil {
		boundary := *msg.CompactBoundary
		boundary.SourceIDs = cloneStrings(msg.CompactBoundary.SourceIDs)
		boundary.ProtectedIndices = cloneInts(msg.CompactBoundary.ProtectedIndices)
		next.CompactBoundary = &boundary
	}
	return next
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneInts(values []int) []int {
	return append([]int(nil), values...)
}
