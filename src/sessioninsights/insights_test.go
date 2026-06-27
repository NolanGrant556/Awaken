package sessioninsights

import (
	"math"
	"testing"
	"time"
)

func TestGenerateOverviewAndBreakdowns(t *testing.T) {
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{
			ID:               "s1",
			Source:           "desktop",
			Platform:         "mac",
			Model:            "gpt-5",
			StartedAt:        base,
			EndedAt:          base.Add(30 * time.Minute),
			MessageCount:     4,
			InputTokens:      100,
			OutputTokens:     50,
			CacheReadTokens:  20,
			CacheWriteTokens: 10,
			EstimatedCostUSD: costPtr(0.30),
			ToolCalls: []ToolCall{
				{Name: "file_read", Count: 2},
				{Name: "shell"},
			},
		},
		{
			ID:           "s2",
			Source:       "desktop",
			Platform:     "mac",
			Model:        "gpt-5",
			StartedAt:    base.Add(time.Hour),
			EndedAt:      base.Add(time.Hour + 10*time.Minute),
			MessageCount: 2,
			InputTokens:  200,
			OutputTokens: 100,
			ToolCalls:    []ToolCall{{Name: "shell"}},
		},
		{
			ID:               "s3",
			Source:           "cli",
			Model:            "claude-sonnet",
			StartedAt:        base.Add(24 * time.Hour),
			EndedAt:          base.Add(26 * time.Hour),
			MessageCount:     8,
			InputTokens:      1000,
			OutputTokens:     500,
			EstimatedCostUSD: costPtr(1.50),
			ToolCalls:        []ToolCall{{Name: "file_read", Count: 3}},
		},
	}

	report := Generate(sessions, Options{
		Now: base.Add(48 * time.Hour),
		ModelCosts: map[string]TokenCost{
			"gpt-5": {
				InputPerMillionUSD:  10,
				OutputPerMillionUSD: 20,
			},
		},
	})

	if report.Empty {
		t.Fatalf("expected non-empty report")
	}
	if report.Overview.TotalSessions != 3 {
		t.Fatalf("expected 3 sessions, got %d", report.Overview.TotalSessions)
	}
	if report.Overview.TotalMessages != 14 {
		t.Fatalf("expected 14 messages, got %d", report.Overview.TotalMessages)
	}
	if report.Overview.TotalToolCalls != 7 {
		t.Fatalf("expected 7 tool calls, got %d", report.Overview.TotalToolCalls)
	}
	if report.Overview.TotalTokens != 1980 {
		t.Fatalf("expected 1980 tokens, got %d", report.Overview.TotalTokens)
	}
	if !almostEqual(report.Overview.EstimatedCostUSD, 1.804) {
		t.Fatalf("expected cost 1.804, got %.6f", report.Overview.EstimatedCostUSD)
	}
	if report.Overview.KnownCostSessions != 3 || report.Overview.UnknownCostSessions != 0 {
		t.Fatalf("unexpected cost counts: known=%d unknown=%d", report.Overview.KnownCostSessions, report.Overview.UnknownCostSessions)
	}
	if report.Overview.TotalDuration != 160*time.Minute {
		t.Fatalf("expected 160m total duration, got %s", report.Overview.TotalDuration)
	}
	if report.Overview.AverageDuration != (160*time.Minute)/3 {
		t.Fatalf("expected average duration over valid sessions, got %s", report.Overview.AverageDuration)
	}
	if !report.Overview.DateRangeStart.Equal(base) || !report.Overview.DateRangeEnd.Equal(base.Add(24*time.Hour)) {
		t.Fatalf("unexpected date range: %s - %s", report.Overview.DateRangeStart, report.Overview.DateRangeEnd)
	}

	if len(report.Models) != 2 {
		t.Fatalf("expected 2 model rows, got %d", len(report.Models))
	}
	if report.Models[0].Name != "gpt-5" || report.Models[0].Sessions != 2 || report.Models[0].TotalTokens != 480 {
		t.Fatalf("unexpected first model row: %+v", report.Models[0])
	}
	if report.Models[1].Name != "claude-sonnet" || report.Models[1].Sessions != 1 || report.Models[1].TotalTokens != 1500 {
		t.Fatalf("unexpected second model row: %+v", report.Models[1])
	}

	if len(report.Sources) != 2 || report.Sources[0].Name != "desktop" || report.Sources[0].Sessions != 2 {
		t.Fatalf("unexpected source breakdown: %+v", report.Sources)
	}
	if len(report.Platforms) != 2 || report.Platforms[0].Name != "mac" || report.Platforms[0].Sessions != 2 {
		t.Fatalf("unexpected platform breakdown: %+v", report.Platforms)
	}
	if report.Platforms[1].Name != "cli" {
		t.Fatalf("expected source fallback for missing platform, got %+v", report.Platforms[1])
	}

	if len(report.Tools) != 2 {
		t.Fatalf("expected 2 tool rows, got %d", len(report.Tools))
	}
	if report.Tools[0].Name != "file_read" || report.Tools[0].Calls != 5 {
		t.Fatalf("unexpected first tool row: %+v", report.Tools[0])
	}
	if report.Tools[1].Name != "shell" || report.Tools[1].Calls != 2 {
		t.Fatalf("unexpected second tool row: %+v", report.Tools[1])
	}

	if len(report.Activity) != 2 {
		t.Fatalf("expected 2 activity rows, got %+v", report.Activity)
	}
	if report.Activity[0].Date != "2026-06-01" || report.Activity[0].Sessions != 2 || report.Activity[0].TotalTokens != 480 {
		t.Fatalf("unexpected first activity row: %+v", report.Activity[0])
	}
	if report.Activity[1].Date != "2026-06-02" || report.Activity[1].Sessions != 1 || report.Activity[1].TotalTokens != 1500 {
		t.Fatalf("unexpected second activity row: %+v", report.Activity[1])
	}
}

func TestGenerateTopSessionsSortingAndLimit(t *testing.T) {
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{
			ID:           "low",
			Model:        "m",
			StartedAt:    base,
			EndedAt:      base.Add(time.Hour),
			InputTokens:  100,
			OutputTokens: 100,
		},
		{
			ID:               "lower-cost",
			Model:            "m",
			StartedAt:        base,
			EndedAt:          base.Add(time.Hour),
			InputTokens:      600,
			OutputTokens:     600,
			EstimatedCostUSD: costPtr(0.1),
		},
		{
			ID:               "shorter",
			Model:            "m",
			StartedAt:        base,
			EndedAt:          base.Add(time.Hour),
			InputTokens:      600,
			OutputTokens:     600,
			EstimatedCostUSD: costPtr(0.2),
		},
		{
			ID:               "fewer-tools",
			Model:            "m",
			StartedAt:        base,
			EndedAt:          base.Add(2 * time.Hour),
			InputTokens:      600,
			OutputTokens:     600,
			EstimatedCostUSD: costPtr(0.2),
		},
		{
			ID:               "more-tools",
			Model:            "m",
			StartedAt:        base,
			EndedAt:          base.Add(2 * time.Hour),
			InputTokens:      600,
			OutputTokens:     600,
			EstimatedCostUSD: costPtr(0.2),
			ToolCalls:        []ToolCall{{Name: "shell", Count: 3}},
		},
	}

	report := Generate(sessions, Options{TopSessionsLimit: 3})

	if len(report.TopSessions) != 3 {
		t.Fatalf("expected 3 top sessions, got %d", len(report.TopSessions))
	}
	want := []string{"more-tools", "fewer-tools", "shorter"}
	for i, id := range want {
		if report.TopSessions[i].ID != id {
			t.Fatalf("top session %d: expected %q, got %+v", i, id, report.TopSessions[i])
		}
	}
}

func TestGenerateFiltersSinceUntilLookbackSourceAndPlatform(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "old", Source: "desktop", Platform: "mac", StartedAt: now.Add(-10 * 24 * time.Hour), InputTokens: 10},
		{ID: "desktop", Source: "desktop", Platform: "mac", StartedAt: now.Add(-48 * time.Hour), InputTokens: 20},
		{ID: "cli", Source: "cli", Platform: "terminal", StartedAt: now.Add(-24 * time.Hour), InputTokens: 30},
		{ID: "missing-start", Source: "desktop", Platform: "mac", InputTokens: 40},
	}

	report := Generate(sessions, Options{
		Now:      now,
		Lookback: 72 * time.Hour,
		Source:   "desktop",
		Platform: "mac",
	})

	if report.Overview.TotalSessions != 1 {
		t.Fatalf("expected 1 filtered session, got %+v", report.Overview)
	}
	if len(report.TopSessions) != 1 || report.TopSessions[0].ID != "desktop" {
		t.Fatalf("expected desktop session after filters, got %+v", report.TopSessions)
	}
	if !report.Filters.Since.Equal(now.Add(-72 * time.Hour)) {
		t.Fatalf("expected lookback-derived since, got %s", report.Filters.Since)
	}

	windowReport := Generate(sessions, Options{
		Since: now.Add(-49 * time.Hour),
		Until: now.Add(-47 * time.Hour),
	})
	if windowReport.Overview.TotalSessions != 1 || windowReport.TopSessions[0].ID != "desktop" {
		t.Fatalf("expected since/until window to include only desktop, got %+v", windowReport.TopSessions)
	}
}

func TestGenerateEmpty(t *testing.T) {
	report := Generate(nil, Options{})

	if !report.Empty {
		t.Fatalf("expected empty report")
	}
	if report.Overview.TotalSessions != 0 {
		t.Fatalf("expected zero overview, got %+v", report.Overview)
	}
	if len(report.Models) != 0 || len(report.Platforms) != 0 || len(report.Tools) != 0 || len(report.Activity) != 0 || len(report.TopSessions) != 0 {
		t.Fatalf("expected no report rows, got %+v", report)
	}
}

func TestGenerateMissingAndAbnormalTimes(t *testing.T) {
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{
			ID:           "missing-end",
			StartedAt:    base,
			InputTokens:  -10,
			OutputTokens: 5,
			ToolCalls:    []ToolCall{{}},
		},
		{
			ID:          "negative-duration",
			Model:       "custom",
			Source:      "cli",
			StartedAt:   base.Add(time.Hour),
			EndedAt:     base,
			InputTokens: 10,
		},
		{
			ID:          "valid-duration",
			Model:       "custom",
			Source:      "cli",
			StartedAt:   base.Add(2 * time.Hour),
			EndedAt:     base.Add(2*time.Hour + 15*time.Minute),
			InputTokens: 20,
		},
		{
			ID:          "no-start",
			Model:       "custom",
			Source:      "cli",
			InputTokens: 30,
		},
	}

	report := Generate(sessions, Options{})

	if report.Overview.TotalSessions != 4 {
		t.Fatalf("expected 4 sessions, got %d", report.Overview.TotalSessions)
	}
	if report.Overview.TotalTokens != 65 {
		t.Fatalf("expected negative tokens to clamp to 65 total, got %d", report.Overview.TotalTokens)
	}
	if report.Overview.DurationSessionCount != 1 {
		t.Fatalf("expected only one valid duration, got %d", report.Overview.DurationSessionCount)
	}
	if report.Overview.TotalDuration != 15*time.Minute || report.Overview.AverageDuration != 15*time.Minute {
		t.Fatalf("unexpected duration totals: total=%s avg=%s", report.Overview.TotalDuration, report.Overview.AverageDuration)
	}
	if report.Overview.KnownCostSessions != 0 || report.Overview.UnknownCostSessions != 4 {
		t.Fatalf("expected all costs unknown, got known=%d unknown=%d", report.Overview.KnownCostSessions, report.Overview.UnknownCostSessions)
	}
	if len(report.Activity) != 1 || report.Activity[0].Sessions != 3 {
		t.Fatalf("expected only started sessions in activity, got %+v", report.Activity)
	}
	if len(report.Models) != 2 || report.Models[0].Name != "custom" || report.Models[1].Name != "unknown" {
		t.Fatalf("unexpected model normalization or ordering: %+v", report.Models)
	}
	if len(report.Tools) != 1 || report.Tools[0].Name != "unknown" || report.Tools[0].Calls != 1 {
		t.Fatalf("expected blank tool name to become unknown, got %+v", report.Tools)
	}
}

func costPtr(cost float64) *float64 {
	return &cost
}

func almostEqual(left, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
