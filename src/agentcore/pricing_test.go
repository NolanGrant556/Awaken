package agentcore

import "testing"

func TestApplyPricingUsesFrozenDeepSeekSnapshotAndPreservesUnknown(t *testing.T) {
	cached, uncached, output := 1_000_000, 1_000_000, 1_000_000
	record := ModelUsageRecord{
		ProviderID: "deepseek", EndpointContractID: "openai_chat_completions_v1", ModelID: "deepseek-v4-pro",
		CachedInputTokens: &cached, UncachedInputTokens: &uncached, OutputTokens: &output,
	}
	applyPricing(&record)
	if record.PricingSnapshot == nil || record.PricingSnapshot.EffectiveDate != "2026-07-19" || record.CalculatedCostUSD == nil {
		t.Fatalf("missing frozen pricing: %+v", record)
	}
	if got, want := *record.CalculatedCostUSD, 1.308625; got != want {
		t.Fatalf("cost=%f want=%f", got, want)
	}

	unknown := ModelUsageRecord{ProviderID: "kimi-coding-plan", ModelID: "kimi-for-coding", CachedInputTokens: &cached, UncachedInputTokens: &uncached, OutputTokens: &output}
	applyPricing(&unknown)
	if unknown.PricingSnapshot != nil || unknown.CalculatedCostUSD != nil || unknown.Currency != "" {
		t.Fatalf("package pricing must stay unknown: %+v", unknown)
	}
}
