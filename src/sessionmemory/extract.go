package sessionmemory

const (
	DefaultMinimumTokensToInit        = 10000
	DefaultMinimumTokensBetweenUpdate = 5000
	DefaultToolCallsBetweenUpdates    = 3
)

func PlanExtraction(stats TranscriptStats, thresholds ExtractionThresholds, candidate ExtractionCandidate) ExtractionPlan {
	stats = normalizeTranscriptStats(stats)
	thresholds = normalizeExtractionThresholds(thresholds)
	candidate = normalizeExtractionCandidate(candidate, stats)

	nextState := ExtractionState{
		MemoryInitialized:      stats.MemoryInitialized,
		TokensAtLastExtraction: stats.TokensAtLastExtraction,
		LastExtractedMessageID: "",
	}

	if stats.MessageCount == 0 {
		return extractionPlan(false, "no transcript messages", candidate, stats, thresholds, nextState)
	}

	if !stats.MemoryInitialized {
		if stats.TokenCount < thresholds.MinimumTokensToInit {
			return extractionPlan(false, "below initialization token threshold", candidate, stats, thresholds, nextState)
		}
		nextState.MemoryInitialized = true
	}

	tokensSinceLastExtraction := stats.TokenCount - stats.TokensAtLastExtraction
	if tokensSinceLastExtraction < 0 {
		tokensSinceLastExtraction = stats.TokenCount
	}
	if tokensSinceLastExtraction < thresholds.MinimumTokensBetweenUpdate {
		return extractionPlan(false, "below update token threshold", candidate, stats, thresholds, nextState)
	}

	hasToolCallThreshold := stats.ToolCallsSinceLastExtraction >= thresholds.ToolCallsBetweenUpdates
	hasNaturalBreak := !stats.LastAssistantHasToolCalls
	if !hasToolCallThreshold && !hasNaturalBreak {
		return extractionPlan(false, "waiting for tool-call threshold or natural break", candidate, stats, thresholds, nextState)
	}

	nextState.TokensAtLastExtraction = stats.TokenCount
	nextState.LastExtractedMessageID = stats.LastMessageID
	if candidate.Reason == "" {
		if hasToolCallThreshold {
			candidate.Reason = "token and tool-call thresholds met"
		} else {
			candidate.Reason = "token threshold met at natural break"
		}
	}
	return extractionPlan(true, candidate.Reason, candidate, stats, thresholds, nextState)
}

func normalizeTranscriptStats(stats TranscriptStats) TranscriptStats {
	if stats.MessageCount < 0 {
		stats.MessageCount = 0
	}
	if stats.TokenCount < 0 {
		stats.TokenCount = 0
	}
	if stats.TokensAtLastExtraction < 0 {
		stats.TokensAtLastExtraction = 0
	}
	if stats.ToolCallsSinceLastExtraction < 0 {
		stats.ToolCallsSinceLastExtraction = 0
	}
	return stats
}

func normalizeExtractionThresholds(thresholds ExtractionThresholds) ExtractionThresholds {
	if thresholds.MinimumTokensToInit <= 0 {
		thresholds.MinimumTokensToInit = DefaultMinimumTokensToInit
	}
	if thresholds.MinimumTokensBetweenUpdate <= 0 {
		thresholds.MinimumTokensBetweenUpdate = DefaultMinimumTokensBetweenUpdate
	}
	if thresholds.ToolCallsBetweenUpdates <= 0 {
		thresholds.ToolCallsBetweenUpdates = DefaultToolCallsBetweenUpdates
	}
	return thresholds
}

func normalizeExtractionCandidate(candidate ExtractionCandidate, stats TranscriptStats) ExtractionCandidate {
	candidate.Scope = NormalizeScope(candidate.Scope)
	if !candidate.Scope.IsValid() {
		candidate.Scope = MemoryScopeSession
	}
	candidate.Key = normalizeKey(candidate.Key)
	if candidate.Key == "" {
		candidate.Key = "session-summary"
	}
	candidate.SessionID = normalizeLabel(candidate.SessionID)
	candidate.AgentID = normalizeLabel(candidate.AgentID)
	candidate.Reason = normalizeLabel(candidate.Reason)
	candidate.Stats = stats
	return candidate
}

func extractionPlan(
	shouldExtract bool,
	reason string,
	candidate ExtractionCandidate,
	stats TranscriptStats,
	thresholds ExtractionThresholds,
	nextState ExtractionState,
) ExtractionPlan {
	return ExtractionPlan{
		ShouldExtract: shouldExtract,
		Candidate:     candidate,
		Reason:        reason,
		Stats:         stats,
		Thresholds:    thresholds,
		NextState:     nextState,
	}
}
