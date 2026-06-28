package effort

import "strings"

type ProviderPayloadPlan struct {
	Provider  Provider
	Omit      bool
	Reason    string
	Resolved  ResolvedEffort
	Anthropic *AnthropicPayload
	OpenAI    *OpenAIPayload
	LMStudio  *LMStudioPayload
}

type AnthropicPayload struct {
	ThinkingType       string
	ThinkingDisplay    string
	OutputConfigEffort string
}

type OpenAIPayload struct {
	ReasoningEffort string
}

type LMStudioPayload struct {
	ReasoningEffort string
	AllowedOptions  []string
}

func ResolveProviderPayload(input ResolutionInput) (ProviderPayloadPlan, error) {
	resolved, err := ResolveAppliedEffort(input)
	plan := ProviderPayloadPlan{
		Provider: input.Provider,
		Omit:     true,
		Reason:   resolved.Reason,
		Resolved: resolved,
	}
	if err != nil {
		return plan, err
	}
	if resolved.Omit || resolved.Value == nil {
		if plan.Reason == "" {
			plan.Reason = "effort omitted"
		}
		return plan, nil
	}

	switch input.Provider {
	case ProviderFirstParty, ProviderAnthropic:
		plan.Omit = false
		plan.Reason = ""
		plan.Anthropic = &AnthropicPayload{
			ThinkingType:       "adaptive",
			ThinkingDisplay:    "summarized",
			OutputConfigEffort: anthropicAdaptiveEffort(*resolved.Value, input.IsAnt),
		}
	case ProviderOpenAI:
		plan.Omit = false
		plan.Reason = ""
		plan.OpenAI = &OpenAIPayload{
			ReasoningEffort: openAIReasoningEffort(*resolved.Value, input.IsAnt),
		}
	case ProviderLMStudio:
		effort := lmStudioReasoningEffort(*resolved.Value, input.IsAnt)
		if allowed := normalizedLMStudioAllowed(input.Capabilities.LMStudioAllowed); len(allowed) > 0 {
			if _, ok := allowed[effort]; !ok {
				plan.Omit = true
				plan.Reason = "lmstudio effort is not in allowed options"
				return plan, nil
			}
		}
		plan.Omit = false
		plan.Reason = ""
		plan.LMStudio = &LMStudioPayload{
			ReasoningEffort: effort,
			AllowedOptions:  append([]string(nil), input.Capabilities.LMStudioAllowed...),
		}
	default:
		plan.Omit = true
		if plan.Reason == "" {
			plan.Reason = "provider does not have an effort payload mapping"
		}
	}

	return plan, nil
}

func anthropicAdaptiveEffort(value Value, isAnt bool) string {
	level := value.DisplayLevel(isAnt)
	switch level {
	case LevelLow, LevelMedium, LevelHigh, LevelMax:
		return string(level)
	default:
		return string(LevelMedium)
	}
}

func openAIReasoningEffort(value Value, isAnt bool) string {
	level := value.DisplayLevel(isAnt)
	switch level {
	case LevelLow, LevelMedium, LevelHigh:
		return string(level)
	case LevelMax:
		return string(LevelHigh)
	default:
		return string(LevelMedium)
	}
}

func lmStudioReasoningEffort(value Value, isAnt bool) string {
	level := value.DisplayLevel(isAnt)
	switch level {
	case LevelLow:
		return "low"
	case LevelMedium:
		return "medium"
	case LevelHigh:
		return "high"
	case LevelMax:
		return "xhigh"
	default:
		return "medium"
	}
}

func normalizedLMStudioAllowed(options []string) map[string]struct{} {
	if len(options) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(options))
	for _, option := range options {
		normalized := normalizeLMStudioEffort(option)
		if normalized != "" {
			allowed[normalized] = struct{}{}
		}
	}
	return allowed
}

func normalizeLMStudioEffort(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "off":
		return "none"
	case "on":
		return "medium"
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return normalized
	default:
		return ""
	}
}
