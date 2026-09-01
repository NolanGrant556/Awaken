package agentcore

import "strings"

type ContextWindowUsageAnchor struct {
	ProviderID               string `json:"provider_id,omitempty"`
	ProfileID                string `json:"profile_id,omitempty"`
	CredentialRevision       uint64 `json:"credential_revision,omitempty"`
	ModelID                  string `json:"model_id,omitempty"`
	CapabilityModelID        string `json:"capability_model_id,omitempty"`
	EndpointSelector         string `json:"endpoint_selector,omitempty"`
	ModelSelectorKind        string `json:"model_selector_kind,omitempty"`
	ProtocolFamily           string `json:"protocol_family,omitempty"`
	ReasoningPolicy          string `json:"reasoning_policy,omitempty"`
	UsageTokens              int    `json:"usage_tokens,omitempty"`
	SentEstimatedInputTokens int    `json:"sent_estimated_input_tokens,omitempty"`
}

func CloneContextWindowUsageAnchor(anchor *ContextWindowUsageAnchor) *ContextWindowUsageAnchor {
	if anchor == nil {
		return nil
	}
	cloned := *anchor
	return &cloned
}

func ContextWindowUsageAnchorFromModelUsage(usage ModelUsageRecord) *ContextWindowUsageAnchor {
	if strings.TrimSpace(usage.Purpose) != "agent_turn" || usage.Status != ModelUsageSucceeded || !usage.UsageComplete {
		return nil
	}
	usageTokens := 0
	if usage.TotalTokens != nil && *usage.TotalTokens > 0 {
		usageTokens = *usage.TotalTokens
	} else if usage.InputTokens != nil && *usage.InputTokens > 0 {
		usageTokens = *usage.InputTokens
	}
	if usageTokens <= 0 || usage.SentEstimatedInputTokens == nil || *usage.SentEstimatedInputTokens <= 0 {
		return nil
	}
	credentialRevision := usage.CredentialRevision
	if credentialRevision == 0 {
		credentialRevision = 1
	}
	selectorKind := strings.ToLower(strings.TrimSpace(usage.ModelSelectorKind))
	capabilityModelID := strings.TrimSpace(usage.CapabilityModelID)
	if capabilityModelID == "" && selectorKind != "endpoint" {
		capabilityModelID = strings.TrimSpace(usage.ModelID)
	}
	endpointSelector := strings.TrimSpace(usage.EndpointSelector)
	if endpointSelector == "" && selectorKind == "endpoint" {
		endpointSelector = strings.TrimSpace(usage.ModelID)
	}
	reasoningPolicy := strings.ToLower(strings.TrimSpace(usage.ReasoningPolicy))
	if reasoningPolicy == "" {
		reasoningPolicy = "provider_default"
	}
	return &ContextWindowUsageAnchor{
		ProviderID:               strings.TrimSpace(usage.ProviderID),
		ProfileID:                strings.TrimSpace(usage.ProfileID),
		CredentialRevision:       credentialRevision,
		ModelID:                  strings.TrimSpace(usage.ModelID),
		CapabilityModelID:        capabilityModelID,
		EndpointSelector:         endpointSelector,
		ModelSelectorKind:        selectorKind,
		ProtocolFamily:           strings.TrimSpace(usage.ProtocolFamily),
		ReasoningPolicy:          reasoningPolicy,
		UsageTokens:              usageTokens,
		SentEstimatedInputTokens: *usage.SentEstimatedInputTokens,
	}
}

func ContextWindowUsageAnchorFromModelConfig(config ModelConfig, usageTokens, sentEstimatedInputTokens int) *ContextWindowUsageAnchor {
	if usageTokens <= 0 || sentEstimatedInputTokens <= 0 {
		return nil
	}
	credentialRevision := config.CredentialRevision
	if credentialRevision == 0 {
		credentialRevision = 1
	}
	selectorKind := strings.ToLower(strings.TrimSpace(config.ModelSelectorKind))
	capabilityModelID := strings.TrimSpace(config.CapabilityModelID)
	if capabilityModelID == "" && selectorKind != "endpoint" {
		capabilityModelID = strings.TrimSpace(config.Model)
	}
	endpointSelector := strings.TrimSpace(config.EndpointSelector)
	if endpointSelector == "" && selectorKind == "endpoint" {
		endpointSelector = strings.TrimSpace(config.Model)
	}
	reasoningPolicy := strings.ToLower(strings.TrimSpace(config.ReasoningEffort))
	if reasoningPolicy == "" {
		reasoningPolicy = "provider_default"
	}
	return &ContextWindowUsageAnchor{
		ProviderID:               strings.TrimSpace(config.ProviderID),
		ProfileID:                strings.TrimSpace(config.ProfileID),
		CredentialRevision:       credentialRevision,
		ModelID:                  strings.TrimSpace(config.Model),
		CapabilityModelID:        capabilityModelID,
		EndpointSelector:         endpointSelector,
		ModelSelectorKind:        selectorKind,
		ProtocolFamily:           strings.TrimSpace(config.Protocol),
		ReasoningPolicy:          reasoningPolicy,
		UsageTokens:              usageTokens,
		SentEstimatedInputTokens: sentEstimatedInputTokens,
	}
}

func ContextWindowUsageAnchorCompatibleWithModelConfig(anchor *ContextWindowUsageAnchor, config ModelConfig) bool {
	if anchor == nil {
		return false
	}
	expected := ContextWindowUsageAnchorFromModelConfig(config, anchor.UsageTokens, anchor.SentEstimatedInputTokens)
	if expected == nil {
		return false
	}
	return anchor.ProviderID == expected.ProviderID &&
		anchor.ProfileID == expected.ProfileID &&
		anchor.CredentialRevision == expected.CredentialRevision &&
		anchor.ModelID == expected.ModelID &&
		anchor.CapabilityModelID == expected.CapabilityModelID &&
		anchor.EndpointSelector == expected.EndpointSelector &&
		anchor.ModelSelectorKind == expected.ModelSelectorKind &&
		anchor.ProtocolFamily == expected.ProtocolFamily &&
		anchor.ReasoningPolicy == expected.ReasoningPolicy
}
