package agentcore

import "strings"

const deepSeekPricingSource = "https://api-docs.deepseek.com/quick_start/pricing/"

func pricingSnapshotForUsage(providerID, endpointContractID, modelID string) *PricingSnapshot {
	if strings.ToLower(strings.TrimSpace(providerID)) != "deepseek" ||
		strings.TrimSpace(endpointContractID) != "openai_chat_completions_v1" {
		return nil
	}
	base := PricingSnapshot{
		ProviderID: "deepseek", ModelID: strings.TrimSpace(modelID), Currency: "USD", UnitTokens: 1_000_000,
		EffectiveDate: "2026-07-19", SourceURL: deepSeekPricingSource,
	}
	switch base.ModelID {
	case "deepseek-v4-flash":
		base.SnapshotID = "deepseek-2026-07-19-v4-flash-usd"
		base.CachedInputPerUnitUSD = 0.0028
		base.UncachedInputPerUnitUSD = 0.14
		base.OutputPerUnitUSD = 0.28
	case "deepseek-v4-pro":
		base.SnapshotID = "deepseek-2026-07-19-v4-pro-usd"
		base.CachedInputPerUnitUSD = 0.003625
		base.UncachedInputPerUnitUSD = 0.435
		base.OutputPerUnitUSD = 0.87
	default:
		return nil
	}
	return &base
}

func applyPricing(record *ModelUsageRecord) {
	if record == nil {
		return
	}
	snapshot := pricingSnapshotForUsage(record.ProviderID, record.EndpointContractID, record.ModelID)
	record.PricingSnapshot = snapshot
	if snapshot == nil || record.CachedInputTokens == nil || record.UncachedInputTokens == nil || record.OutputTokens == nil {
		return
	}
	unit := float64(snapshot.UnitTokens)
	cost := float64(*record.CachedInputTokens)*snapshot.CachedInputPerUnitUSD/unit +
		float64(*record.UncachedInputTokens)*snapshot.UncachedInputPerUnitUSD/unit +
		float64(*record.OutputTokens)*snapshot.OutputPerUnitUSD/unit
	record.CalculatedCostUSD = &cost
	record.Currency = snapshot.Currency
}
