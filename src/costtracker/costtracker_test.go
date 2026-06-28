package costtracker

import (
	"strings"
	"testing"
	"time"
)

func TestCalculateCostInputOutput(t *testing.T) {
	result := CalculateCost("claude-sonnet-4-20250514", Usage{
		InputTokens:  1_000_000,
		OutputTokens: 2_000_000,
	}, Pricing{
		InputMicrosPerMillion:  Micros(3_000_000),
		OutputMicrosPerMillion: Micros(15_000_000),
		Status:                 CostStatusEstimated,
		Source:                 CostSourceOfficialDocsSnapshot,
	})

	if result.Status != CostStatusEstimated {
		t.Fatalf("status = %q, want %q", result.Status, CostStatusEstimated)
	}
	if result.AmountMicros != 33_000_000 {
		t.Fatalf("amount = %d, want 33000000", result.AmountMicros)
	}
	if result.CanonicalModel != "claude-sonnet-4" {
		t.Fatalf("canonical = %q, want claude-sonnet-4", result.CanonicalModel)
	}
}

func TestCalculateCostCachePricing(t *testing.T) {
	result := CalculateCost("anthropic/claude-sonnet-4-20250514", Usage{
		CacheReadTokens:  1_000_000,
		CacheWriteTokens: 2_000_000,
	}, Pricing{
		CacheReadMicrosPerMillion:  Micros(300_000),
		CacheWriteMicrosPerMillion: Micros(3_750_000),
		Source:                     CostSourceOfficialDocsSnapshot,
	})

	if result.AmountMicros != 7_800_000 {
		t.Fatalf("amount = %d, want 7800000", result.AmountMicros)
	}
}

func TestCalculateCostRequestCost(t *testing.T) {
	result := CalculateCost("gpt-4o", Usage{
		RequestCount: 3,
	}, Pricing{
		RequestMicros: Micros(2_500),
		Source:        CostSourceProviderModelsAPI,
	})

	if result.AmountMicros != 7_500 {
		t.Fatalf("amount = %d, want 7500", result.AmountMicros)
	}
}

func TestCalculateCostUnknownPricing(t *testing.T) {
	result := CalculateCost("gpt-4o", Usage{
		OutputTokens: 10,
	}, Pricing{
		InputMicrosPerMillion: Micros(2_500_000),
		Source:                CostSourceProviderModelsAPI,
	})

	if result.Status != CostStatusUnknown {
		t.Fatalf("status = %q, want %q", result.Status, CostStatusUnknown)
	}
	if result.AmountMicros != 0 {
		t.Fatalf("amount = %d, want 0 for unknown pricing", result.AmountMicros)
	}
	if len(result.Notes) == 0 || !strings.Contains(result.Notes[0], "output") {
		t.Fatalf("notes = %#v, want output pricing note", result.Notes)
	}
}

func TestTrackerSnapshotRestore(t *testing.T) {
	tracker := NewTracker()
	tracker.AddUsage("gpt-4o", Usage{InputTokens: 1_000_000}, Pricing{
		InputMicrosPerMillion: Micros(2_500_000),
		Source:                CostSourceUserOverride,
	})

	restored := Restore(tracker.Snapshot())
	restored.AddUsage("gpt-4o", Usage{OutputTokens: 1_000_000}, Pricing{
		OutputMicrosPerMillion: Micros(10_000_000),
		Source:                 CostSourceUserOverride,
	})
	snapshot := restored.Snapshot()

	if snapshot.Totals.CostMicros != 12_500_000 {
		t.Fatalf("cost = %d, want 12500000", snapshot.Totals.CostMicros)
	}
	if snapshot.Models["gpt-4o"].InputTokens != 1_000_000 || snapshot.Models["gpt-4o"].OutputTokens != 1_000_000 {
		t.Fatalf("model usage = %#v, want restored input and new output", snapshot.Models["gpt-4o"])
	}
}

func TestTrackerDurationAndLines(t *testing.T) {
	tracker := NewTracker()
	tracker.AddDuration(1500*time.Millisecond, 3*time.Second)
	tracker.AddDuration(500*time.Millisecond, 2*time.Second)
	tracker.AddLinesChanged(4, 1)
	tracker.AddLinesChanged(6, 0)
	snapshot := tracker.Snapshot()

	if snapshot.Totals.APIDuration != 2*time.Second {
		t.Fatalf("api duration = %s, want 2s", snapshot.Totals.APIDuration)
	}
	if snapshot.Totals.WallDuration != 5*time.Second {
		t.Fatalf("wall duration = %s, want 5s", snapshot.Totals.WallDuration)
	}
	if snapshot.Totals.LinesAdded != 10 || snapshot.Totals.LinesRemoved != 1 {
		t.Fatalf("lines = +%d -%d, want +10 -1", snapshot.Totals.LinesAdded, snapshot.Totals.LinesRemoved)
	}
}

func TestTrackerModelAggregation(t *testing.T) {
	tracker := NewTracker()
	pricing := Pricing{InputMicrosPerMillion: Micros(3_000_000)}

	tracker.AddUsage("claude-sonnet-4-20250514", Usage{InputTokens: 1_000_000}, pricing)
	tracker.AddUsage("anthropic/claude-sonnet-4-20250514", Usage{InputTokens: 2_000_000}, pricing)
	snapshot := tracker.Snapshot()

	if len(snapshot.Models) != 1 {
		t.Fatalf("model count = %d, want 1", len(snapshot.Models))
	}
	usage := snapshot.Models["claude-sonnet-4"]
	if usage.InputTokens != 3_000_000 || usage.CostMicros != 9_000_000 {
		t.Fatalf("usage = %#v, want aggregated tokens and cost", usage)
	}
}

func TestTrackerMarksUnknownModelCost(t *testing.T) {
	tracker := NewTracker()
	tracker.AddUsage("unpriced-model", Usage{InputTokens: 10}, Pricing{})
	snapshot := tracker.Snapshot()

	if !snapshot.Totals.HasUnknownModelCost {
		t.Fatalf("HasUnknownModelCost = false, want true")
	}
	if !snapshot.Models["unpriced-model"].HasUnknownCost {
		t.Fatalf("model HasUnknownCost = false, want true")
	}
}

func TestFormatting(t *testing.T) {
	if got := FormatCost(123); got != "$0.0001" {
		t.Fatalf("FormatCost(123) = %q, want $0.0001", got)
	}
	if got := FormatCost(1_234_567); got != "$1.23" {
		t.Fatalf("FormatCost(1234567) = %q, want $1.23", got)
	}

	tracker := NewTracker()
	tracker.AddUsage("gpt-4o-2024-08-06", Usage{
		InputTokens:      1,
		OutputTokens:     2,
		CacheReadTokens:  3,
		CacheWriteTokens: 4,
		WebSearches:      1,
	}, Pricing{
		InputMicrosPerMillion:      Micros(1_000_000),
		OutputMicrosPerMillion:     Micros(1_000_000),
		CacheReadMicrosPerMillion:  Micros(1_000_000),
		CacheWriteMicrosPerMillion: Micros(1_000_000),
	})
	tracker.AddDuration(61*time.Second, 2*time.Hour+3*time.Minute)
	tracker.AddLinesChanged(1, 2)

	summary := FormatSummary(tracker.Snapshot())
	for _, want := range []string{
		"Total cost:",
		"Total duration (API):  1m",
		"Total duration (wall): 2h 3m",
		"Total code changes:    1 line added, 2 lines removed",
		"gpt-4o: 1 input, 2 output, 3 cache read, 4 cache write, 1 web search",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}
