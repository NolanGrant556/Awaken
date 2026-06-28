package sessioninsights

import (
	"sort"
	"strings"
	"time"
)

const (
	unknownName             = "unknown"
	defaultTopSessionsLimit = 5
)

type Session struct {
	ID string `json:"id"`

	Source   string `json:"source,omitempty"`
	Platform string `json:"platform,omitempty"`
	Model    string `json:"model,omitempty"`

	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`

	MessageCount int `json:"message_count,omitempty"`

	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`

	EstimatedCostUSD *float64   `json:"estimated_cost_usd,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
}

type TokenCost struct {
	InputPerMillionUSD      float64 `json:"input_per_million_usd,omitempty"`
	OutputPerMillionUSD     float64 `json:"output_per_million_usd,omitempty"`
	CacheReadPerMillionUSD  float64 `json:"cache_read_per_million_usd,omitempty"`
	CacheWritePerMillionUSD float64 `json:"cache_write_per_million_usd,omitempty"`
}

type Options struct {
	Since    time.Time     `json:"since,omitempty"`
	Until    time.Time     `json:"until,omitempty"`
	Lookback time.Duration `json:"lookback,omitempty"`
	Now      time.Time     `json:"now,omitempty"`

	Source   string `json:"source,omitempty"`
	Platform string `json:"platform,omitempty"`

	TopSessionsLimit int                  `json:"top_sessions_limit,omitempty"`
	ModelCosts       map[string]TokenCost `json:"model_costs,omitempty"`
}

type InsightOptions = Options

type Report struct {
	Empty       bool      `json:"empty"`
	GeneratedAt time.Time `json:"generated_at"`
	Filters     Filters   `json:"filters"`

	Overview    Overview           `json:"overview"`
	Models      []ModelInsight     `json:"models"`
	Sources     []DimensionInsight `json:"sources"`
	Platforms   []DimensionInsight `json:"platforms"`
	Tools       []ToolInsight      `json:"tools"`
	Activity    []ActivityDay      `json:"activity"`
	TopSessions []SessionInsight   `json:"top_sessions"`
}

type Filters struct {
	Since    time.Time     `json:"since,omitempty"`
	Until    time.Time     `json:"until,omitempty"`
	Lookback time.Duration `json:"lookback,omitempty"`
	Source   string        `json:"source,omitempty"`
	Platform string        `json:"platform,omitempty"`
}

type Overview struct {
	TotalSessions  int `json:"total_sessions"`
	TotalMessages  int `json:"total_messages"`
	TotalToolCalls int `json:"total_tool_calls"`

	TotalInputTokens      int64 `json:"total_input_tokens"`
	TotalOutputTokens     int64 `json:"total_output_tokens"`
	TotalCacheReadTokens  int64 `json:"total_cache_read_tokens"`
	TotalCacheWriteTokens int64 `json:"total_cache_write_tokens"`
	TotalTokens           int64 `json:"total_tokens"`

	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
	KnownCostSessions   int     `json:"known_cost_sessions"`
	UnknownCostSessions int     `json:"unknown_cost_sessions"`

	TotalDuration        time.Duration `json:"total_duration"`
	AverageDuration      time.Duration `json:"average_duration"`
	DurationSessionCount int           `json:"duration_session_count"`

	AverageMessagesPerSession float64 `json:"average_messages_per_session"`
	AverageTokensPerSession   float64 `json:"average_tokens_per_session"`

	DateRangeStart time.Time `json:"date_range_start,omitempty"`
	DateRangeEnd   time.Time `json:"date_range_end,omitempty"`
}

type ModelInsight struct {
	Name      string `json:"name"`
	Sessions  int    `json:"sessions"`
	Messages  int    `json:"messages"`
	ToolCalls int    `json:"tool_calls"`

	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	TotalTokens      int64 `json:"total_tokens"`

	EstimatedCostUSD    float64 `json:"estimated_cost_usd"`
	KnownCostSessions   int     `json:"known_cost_sessions"`
	UnknownCostSessions int     `json:"unknown_cost_sessions"`
}

type DimensionInsight struct {
	Name      string `json:"name"`
	Sessions  int    `json:"sessions"`
	Messages  int    `json:"messages"`
	ToolCalls int    `json:"tool_calls"`

	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	TotalTokens      int64 `json:"total_tokens"`

	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type ToolInsight struct {
	Name       string  `json:"name"`
	Calls      int     `json:"calls"`
	Percentage float64 `json:"percentage"`
}

type ActivityDay struct {
	Date      string `json:"date"`
	Sessions  int    `json:"sessions"`
	Messages  int    `json:"messages"`
	ToolCalls int    `json:"tool_calls"`

	TotalTokens      int64         `json:"total_tokens"`
	EstimatedCostUSD float64       `json:"estimated_cost_usd"`
	Duration         time.Duration `json:"duration"`
}

type SessionInsight struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Platform string `json:"platform"`
	Model    string `json:"model"`

	StartedAt time.Time     `json:"started_at,omitempty"`
	Duration  time.Duration `json:"duration"`
	Messages  int           `json:"messages"`
	ToolCalls int           `json:"tool_calls"`

	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	CostKnown        bool    `json:"cost_known"`
}

type normalizedSession struct {
	id       string
	source   string
	platform string
	model    string

	startedAt   time.Time
	duration    time.Duration
	hasDuration bool

	messages  int
	toolCalls int

	inputTokens      int64
	outputTokens     int64
	cacheReadTokens  int64
	cacheWriteTokens int64
	totalTokens      int64

	estimatedCostUSD float64
	costKnown        bool

	tools []toolCount
}

type toolCount struct {
	name  string
	count int
}

// Generate creates a deterministic insights report from already-loaded sessions.
func Generate(sessions []Session, opts Options) Report {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	since := opts.Since
	if since.IsZero() && opts.Lookback > 0 {
		since = now.Add(-opts.Lookback)
	}
	until := opts.Until

	report := Report{
		GeneratedAt: now,
		Filters: Filters{
			Since:    since,
			Until:    until,
			Lookback: opts.Lookback,
			Source:   strings.TrimSpace(opts.Source),
			Platform: strings.TrimSpace(opts.Platform),
		},
	}

	normalized := make([]normalizedSession, 0, len(sessions))
	for _, session := range sessions {
		if !matchesFilters(session, since, until, opts.Source, opts.Platform) {
			continue
		}
		normalized = append(normalized, normalizeSession(session, opts))
	}

	report.Empty = len(normalized) == 0
	if report.Empty {
		return report
	}

	report.Overview = buildOverview(normalized)
	report.Models = buildModels(normalized)
	report.Sources = buildDimensions(normalized, func(session normalizedSession) string {
		return session.source
	})
	report.Platforms = buildDimensions(normalized, func(session normalizedSession) string {
		return session.platform
	})
	report.Tools = buildTools(normalized)
	report.Activity = buildActivity(normalized)
	report.TopSessions = buildTopSessions(normalized, opts.TopSessionsLimit)

	return report
}

func matchesFilters(session Session, since, until time.Time, sourceFilter, platformFilter string) bool {
	hasTimeFilter := !since.IsZero() || !until.IsZero()
	if hasTimeFilter {
		if session.StartedAt.IsZero() {
			return false
		}
		startedAt := session.StartedAt
		if !since.IsZero() && startedAt.Before(since) {
			return false
		}
		if !until.IsZero() && startedAt.After(until) {
			return false
		}
	}

	source := normalizeName(session.Source)
	if filter := strings.TrimSpace(sourceFilter); filter != "" && source != normalizeName(filter) {
		return false
	}

	platform := normalizePlatform(session)
	if filter := strings.TrimSpace(platformFilter); filter != "" && platform != normalizeName(filter) {
		return false
	}

	return true
}

func normalizeSession(session Session, opts Options) normalizedSession {
	inputTokens := nonNegativeInt64(session.InputTokens)
	outputTokens := nonNegativeInt64(session.OutputTokens)
	cacheReadTokens := nonNegativeInt64(session.CacheReadTokens)
	cacheWriteTokens := nonNegativeInt64(session.CacheWriteTokens)
	totalTokens := inputTokens + outputTokens + cacheReadTokens + cacheWriteTokens
	costUSD, costKnown := estimateCost(session, opts.ModelCosts, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens)
	duration, hasDuration := sessionDuration(session)
	tools, toolCalls := normalizeTools(session.ToolCalls)

	id := strings.TrimSpace(session.ID)
	if id == "" {
		id = unknownName
	}

	return normalizedSession{
		id:               id,
		source:           normalizeName(session.Source),
		platform:         normalizePlatform(session),
		model:            normalizeName(session.Model),
		startedAt:        session.StartedAt,
		duration:         duration,
		hasDuration:      hasDuration,
		messages:         nonNegativeInt(session.MessageCount),
		toolCalls:        toolCalls,
		inputTokens:      inputTokens,
		outputTokens:     outputTokens,
		cacheReadTokens:  cacheReadTokens,
		cacheWriteTokens: cacheWriteTokens,
		totalTokens:      totalTokens,
		estimatedCostUSD: costUSD,
		costKnown:        costKnown,
		tools:            tools,
	}
}

func estimateCost(session Session, modelCosts map[string]TokenCost, inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) (float64, bool) {
	if session.EstimatedCostUSD != nil {
		return nonNegativeFloat(*session.EstimatedCostUSD), true
	}

	model := normalizeName(session.Model)
	rate, ok := modelCosts[model]
	if !ok {
		rate, ok = modelCosts[strings.TrimSpace(session.Model)]
	}
	if !ok {
		return 0, false
	}

	cost := float64(inputTokens) / 1_000_000 * rate.InputPerMillionUSD
	cost += float64(outputTokens) / 1_000_000 * rate.OutputPerMillionUSD
	cost += float64(cacheReadTokens) / 1_000_000 * rate.CacheReadPerMillionUSD
	cost += float64(cacheWriteTokens) / 1_000_000 * rate.CacheWritePerMillionUSD
	return nonNegativeFloat(cost), true
}

func sessionDuration(session Session) (time.Duration, bool) {
	if session.StartedAt.IsZero() || session.EndedAt.IsZero() {
		return 0, false
	}
	duration := session.EndedAt.Sub(session.StartedAt)
	if duration <= 0 {
		return 0, false
	}
	return duration, true
}

func normalizeTools(calls []ToolCall) ([]toolCount, int) {
	tools := make([]toolCount, 0, len(calls))
	total := 0
	for _, call := range calls {
		count := call.Count
		if count <= 0 {
			count = 1
		}
		name := normalizeName(call.Name)
		tools = append(tools, toolCount{name: name, count: count})
		total += count
	}
	return tools, total
}

func buildOverview(sessions []normalizedSession) Overview {
	var overview Overview
	overview.TotalSessions = len(sessions)

	for _, session := range sessions {
		overview.TotalMessages += session.messages
		overview.TotalToolCalls += session.toolCalls
		overview.TotalInputTokens += session.inputTokens
		overview.TotalOutputTokens += session.outputTokens
		overview.TotalCacheReadTokens += session.cacheReadTokens
		overview.TotalCacheWriteTokens += session.cacheWriteTokens
		overview.TotalTokens += session.totalTokens
		overview.EstimatedCostUSD += session.estimatedCostUSD
		if session.costKnown {
			overview.KnownCostSessions++
		} else {
			overview.UnknownCostSessions++
		}
		if session.hasDuration {
			overview.TotalDuration += session.duration
			overview.DurationSessionCount++
		}
		if !session.startedAt.IsZero() {
			if overview.DateRangeStart.IsZero() || session.startedAt.Before(overview.DateRangeStart) {
				overview.DateRangeStart = session.startedAt
			}
			if overview.DateRangeEnd.IsZero() || session.startedAt.After(overview.DateRangeEnd) {
				overview.DateRangeEnd = session.startedAt
			}
		}
	}

	if overview.DurationSessionCount > 0 {
		overview.AverageDuration = overview.TotalDuration / time.Duration(overview.DurationSessionCount)
	}
	if overview.TotalSessions > 0 {
		overview.AverageMessagesPerSession = float64(overview.TotalMessages) / float64(overview.TotalSessions)
		overview.AverageTokensPerSession = float64(overview.TotalTokens) / float64(overview.TotalSessions)
	}

	return overview
}

func buildModels(sessions []normalizedSession) []ModelInsight {
	byModel := make(map[string]*ModelInsight)
	for _, session := range sessions {
		insight := byModel[session.model]
		if insight == nil {
			insight = &ModelInsight{Name: session.model}
			byModel[session.model] = insight
		}
		insight.Sessions++
		insight.Messages += session.messages
		insight.ToolCalls += session.toolCalls
		insight.InputTokens += session.inputTokens
		insight.OutputTokens += session.outputTokens
		insight.CacheReadTokens += session.cacheReadTokens
		insight.CacheWriteTokens += session.cacheWriteTokens
		insight.TotalTokens += session.totalTokens
		insight.EstimatedCostUSD += session.estimatedCostUSD
		if session.costKnown {
			insight.KnownCostSessions++
		} else {
			insight.UnknownCostSessions++
		}
	}

	models := make([]ModelInsight, 0, len(byModel))
	for _, insight := range byModel {
		models = append(models, *insight)
	}
	sort.Slice(models, func(i, j int) bool {
		return modelLess(models[i].Sessions, models[j].Sessions, models[i].TotalTokens, models[j].TotalTokens, models[i].EstimatedCostUSD, models[j].EstimatedCostUSD, models[i].Name, models[j].Name)
	})
	return models
}

func buildDimensions(sessions []normalizedSession, name func(normalizedSession) string) []DimensionInsight {
	byName := make(map[string]*DimensionInsight)
	for _, session := range sessions {
		dimensionName := name(session)
		insight := byName[dimensionName]
		if insight == nil {
			insight = &DimensionInsight{Name: dimensionName}
			byName[dimensionName] = insight
		}
		insight.Sessions++
		insight.Messages += session.messages
		insight.ToolCalls += session.toolCalls
		insight.InputTokens += session.inputTokens
		insight.OutputTokens += session.outputTokens
		insight.CacheReadTokens += session.cacheReadTokens
		insight.CacheWriteTokens += session.cacheWriteTokens
		insight.TotalTokens += session.totalTokens
		insight.EstimatedCostUSD += session.estimatedCostUSD
	}

	dimensions := make([]DimensionInsight, 0, len(byName))
	for _, insight := range byName {
		dimensions = append(dimensions, *insight)
	}
	sort.Slice(dimensions, func(i, j int) bool {
		return modelLess(dimensions[i].Sessions, dimensions[j].Sessions, dimensions[i].TotalTokens, dimensions[j].TotalTokens, dimensions[i].EstimatedCostUSD, dimensions[j].EstimatedCostUSD, dimensions[i].Name, dimensions[j].Name)
	})
	return dimensions
}

func buildTools(sessions []normalizedSession) []ToolInsight {
	counts := make(map[string]int)
	total := 0
	for _, session := range sessions {
		for _, tool := range session.tools {
			counts[tool.name] += tool.count
			total += tool.count
		}
	}

	tools := make([]ToolInsight, 0, len(counts))
	for name, calls := range counts {
		percentage := 0.0
		if total > 0 {
			percentage = float64(calls) / float64(total) * 100
		}
		tools = append(tools, ToolInsight{
			Name:       name,
			Calls:      calls,
			Percentage: percentage,
		})
	}
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Calls != tools[j].Calls {
			return tools[i].Calls > tools[j].Calls
		}
		return tools[i].Name < tools[j].Name
	})
	return tools
}

func buildActivity(sessions []normalizedSession) []ActivityDay {
	byDate := make(map[string]*ActivityDay)
	for _, session := range sessions {
		if session.startedAt.IsZero() {
			continue
		}
		date := session.startedAt.UTC().Format("2006-01-02")
		day := byDate[date]
		if day == nil {
			day = &ActivityDay{Date: date}
			byDate[date] = day
		}
		day.Sessions++
		day.Messages += session.messages
		day.ToolCalls += session.toolCalls
		day.TotalTokens += session.totalTokens
		day.EstimatedCostUSD += session.estimatedCostUSD
		day.Duration += session.duration
	}

	activity := make([]ActivityDay, 0, len(byDate))
	for _, day := range byDate {
		activity = append(activity, *day)
	}
	sort.Slice(activity, func(i, j int) bool {
		return activity[i].Date < activity[j].Date
	})
	return activity
}

func buildTopSessions(sessions []normalizedSession, limit int) []SessionInsight {
	if limit <= 0 {
		limit = defaultTopSessionsLimit
	}

	sortedSessions := append([]normalizedSession(nil), sessions...)
	sort.Slice(sortedSessions, func(i, j int) bool {
		left := sortedSessions[i]
		right := sortedSessions[j]
		if left.totalTokens != right.totalTokens {
			return left.totalTokens > right.totalTokens
		}
		if left.estimatedCostUSD != right.estimatedCostUSD {
			return left.estimatedCostUSD > right.estimatedCostUSD
		}
		if left.duration != right.duration {
			return left.duration > right.duration
		}
		if left.toolCalls != right.toolCalls {
			return left.toolCalls > right.toolCalls
		}
		return left.id < right.id
	})

	if limit > len(sortedSessions) {
		limit = len(sortedSessions)
	}
	top := make([]SessionInsight, 0, limit)
	for _, session := range sortedSessions[:limit] {
		top = append(top, SessionInsight{
			ID:               session.id,
			Source:           session.source,
			Platform:         session.platform,
			Model:            session.model,
			StartedAt:        session.startedAt,
			Duration:         session.duration,
			Messages:         session.messages,
			ToolCalls:        session.toolCalls,
			TotalTokens:      session.totalTokens,
			EstimatedCostUSD: session.estimatedCostUSD,
			CostKnown:        session.costKnown,
		})
	}
	return top
}

func modelLess(leftSessions, rightSessions int, leftTokens, rightTokens int64, leftCost, rightCost float64, leftName, rightName string) bool {
	if leftSessions != rightSessions {
		return leftSessions > rightSessions
	}
	if leftTokens != rightTokens {
		return leftTokens > rightTokens
	}
	if leftCost != rightCost {
		return leftCost > rightCost
	}
	return leftName < rightName
}

func normalizePlatform(session Session) string {
	if platform := strings.TrimSpace(session.Platform); platform != "" {
		return platform
	}
	return normalizeName(session.Source)
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return unknownName
	}
	return name
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}
