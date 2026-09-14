package llm

import "strings"

func applyRequestPolicy(cfg Config, req Request) Request {
	switch req.Purpose {
	case RequestPurposeThreadTitle, RequestPurposeCompactSummary, RequestPurposeMemoryRecall, RequestPurposeMemoryMaintenance, RequestPurposeHookPrompt:
		return applyAuxiliaryRequestPolicy(cfg, req)
	default:
		return req
	}
}

// nativeReasoningEffort lowers Awaken's stable low/high/max vocabulary at the
// provider boundary. Auxiliary requests normally omit effort, but exact
// provider/model contracts may still force a reduced effort to avoid large
// private-reasoning overhead when reasoning cannot be disabled.
func nativeReasoningEffort(cfg Config, req Request) string {
	if !isAgentExecutionPurpose(req.Purpose) && !supportsAuxiliaryReasoningEffort(cfg, req) {
		return ""
	}
	effort := strings.ToLower(strings.TrimSpace(req.ReasoningPolicy))
	if effort != "low" && effort != "high" && effort != "max" {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	model := capabilityModelID(cfg, req)
	switch {
	case provider == "openai":
		return effort
	case provider == "google" && strings.HasPrefix(model, "gemini-"):
		if effort == "low" || effort == "high" {
			return effort
		}
	case (provider == "qwen-token-plan" && (model == "qwen3.8-max-preview" || model == "qwen3.8-max" || model == "qwen3.8-flash")) ||
		(provider == "qwen" && (model == "qwen3.8-max" || model == "qwen3.8-flash")):
		switch effort {
		case "low":
			return "low"
		case "high":
			return "medium"
		case "max":
			return "xhigh"
		}
	case provider == "doubao" && isDoubaoSeedReasoningEffortModel(model):
		switch effort {
		case "low":
			return "low"
		case "high":
			return "medium"
		case "max":
			return "high"
		}
	case provider == "doubao" && isDoubaoHostedDeepSeekGAModel(model):
		if effort == "low" {
			return "low"
		}
		if effort == "high" || effort == "max" {
			return "high"
		}
	case provider == "doubao" && isDoubaoHostedGlm52Model(model):
		if effort == "high" || effort == "max" {
			return effort
		}
	case provider == "doubao" && isDoubaoHostedDeepSeekPreviewModel(model):
		if effort == "high" || effort == "max" {
			return effort
		}
	case isKimiEffortModel(provider, model):
		if req.DisableReasoning {
			return ""
		}
		return effort
	case (provider == "glm" || provider == "glm-coding-plan") && (model == "glm-5.3" || model == "glm-5.3-flash"):
		return effort
	case (provider == "glm" || provider == "glm-coding-plan") && model == "glm-5.2":
		if effort == "high" || effort == "max" {
			return effort
		}
	case provider == "deepseek" && (strings.HasPrefix(model, "deepseek-v4") || model == "deepseek-flash"):
		if effort == "high" || effort == "max" {
			return effort
		}
	case provider == "volcengine-agent-plan":
		switch model {
		case "kimi-k3":
			return effort
		case "glm-5.3":
			return effort
		case "glm-5.2", "deepseek-v4-pro", "deepseek-flash":
			if effort == "high" || effort == "max" {
				return effort
			}
		case "qwen3.8-max-preview", "qwen3.8-max", "qwen3.8-flash":
			if effort == "low" {
				return "low"
			}
			if effort == "high" {
				return "medium"
			}
			if effort == "max" {
				return "xhigh"
			}
		case "doubao-seed-2.1-pro":
			if effort == "low" {
				return "low"
			}
			if effort == "high" {
				return "medium"
			}
			if effort == "max" {
				return "high"
			}
		}
	}
	return ""
}

func isAgentExecutionPurpose(purpose RequestPurpose) bool {
	return purpose == RequestPurposeAgentTurn || purpose == RequestPurposeSubagent || purpose == RequestPurposeScheduledTask || purpose == RequestPurposeHookAgent || purpose == RequestPurposeResearchSynthesis
}

func supportsAuxiliaryReasoningEffort(cfg Config, req Request) bool {
	if isKimiEffortModel(cfg.ProviderID, capabilityModelID(cfg, req)) && isKimiAuxiliaryPurpose(req.Purpose) {
		return true
	}

	if !isAuxiliaryReasoningBoundedPurpose(req.Purpose) {
		return false
	}
	return supportsLowestEffortReasoningFallback(cfg, req)
}

// capabilityModelID is the documented model-family identity used for
// capability decisions. Endpoint-selector providers deliberately fail closed
// when a host supplies only an opaque ep-* transport selector.
func capabilityModelID(cfg Config, req Request) string {
	if value := strings.ToLower(strings.TrimSpace(cfg.CapabilityModelID)); value != "" {
		return value
	}
	if strings.EqualFold(strings.TrimSpace(cfg.ModelSelectorKind), "endpoint") ||
		strings.TrimSpace(cfg.EndpointSelector) != "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(resolveModel(cfg, req)))
}

func applyAuxiliaryRequestPolicy(cfg Config, req Request) Request {
	model := strings.ToLower(resolveModel(cfg, req))
	if isKimiEffortModel(cfg.ProviderID, model) {
		// API K3 is reasoning-only. Coding Plan publishes an off switch;
		// otherwise bound auxiliary work to the lowest native effort.
		if _, ok := ResolveReasoningControlCapability(cfg, req); !req.DisableReasoning || !ok {
			req.DisableReasoning = false
			req.ReasoningPolicy = "low"
		}
		if req.DisableReasoning {
			// Kimi Code routes off to K2.8 non-thinking; Chat requires 0.6.
			setCompactTemperature(&req, 0.6)
		} else {
			setCompactTemperature(&req, 1.0)
		}
		return req
	}
	key := providerModelKey(cfg, req)

	if ResolveReasoningOffDecision(cfg, req).Kind == ReasoningOffLowestEffort {
		// Unpublished off is not "omit the field". Use the confirmed lowest
		// effort so title/compact budgets are not spent on default reasoning.
		req.DisableReasoning = false
		req.ReasoningPolicy = "low"
	}

	switch {
	case isKimiK27Code(model):
		// Kimi K2.7 Code always thinks and rejects thinking.disabled.
		req.DisableReasoning = false
		setCompactTemperature(&req, 1.0)
	case isKimiK3(model):
		setCompactTemperature(&req, 1.0)
	case isKimiK25OrK26(model):
		if req.DisableReasoning {
			setCompactTemperature(&req, 0.6)
		} else {
			setCompactTemperature(&req, 1.0)
		}
	case isKimiProvider(key):
		setCompactTemperature(&req, 0.6)
	case isDeepSeekProvider(key):
		setCompactTemperature(&req, 0.0)
	case isArkProvider(key):
		setCompactTemperature(&req, 0.1)
	}

	return req
}

func setCompactTemperature(req *Request, value float64) {
	if req.TemperatureOverride != nil {
		return
	}
	req.TemperatureOverride = float64Pointer(value)
	req.OmitTemperature = false
}

func providerModelKey(cfg Config, req Request) string {
	return strings.ToLower(strings.Join([]string{
		cfg.BaseURL,
		cfg.Model,
		req.Model,
		resolveModel(cfg, req),
	}, " "))
}

func isKimiProvider(key string) bool {
	return strings.Contains(key, "kimi") || strings.Contains(key, "moonshot")
}

func isKimiK25OrK26(model string) bool {
	return model == "kimi-k2.5" || model == "kimi-k2.6"
}

func isKimiK3(model string) bool {
	return model == "kimi-k3" || model == "k3" || model == "k3-256k"
}

func isKimiK27Code(model string) bool {
	return model == "kimi-k2.7-code" ||
		model == "kimi-k2.7-code-highspeed" ||
		model == "kimi-for-coding-highspeed"
}

func isDoubaoSeedReasoningEffortModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "doubao-seed-evolving",
		"doubao-seed-2.1-pro",
		"doubao-seed-2-1-pro-260628",
		"doubao-seed-2-1-turbo-260628",
		"doubao-seed-2-0-code-preview-260215",
		"doubao-seed-2-0-pro-260215",
		"doubao-seed-2-0-lite-260428",
		"doubao-seed-2-0-mini-260428":
		return true
	default:
		return false
	}
}

func isDoubaoHostedDeepSeekGAModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "deepseek-v4-pro-ga-260813", "deepseek-v4-flash-ga-260731",
		"deepseek-v4-pro", "deepseek-v4-flash":
		return true
	default:
		return false
	}
}

func isDoubaoHostedDeepSeekPreviewModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "deepseek-v4-pro-260425", "deepseek-v4-flash-260425":
		return true
	default:
		return false
	}
}

func isDoubaoHostedGlm52Model(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "glm-5-2-260617", "glm-5.2":
		return true
	default:
		return false
	}
}

func supportsDoubaoPaygReasoningEffort(model string) bool {
	return isDoubaoSeedReasoningEffortModel(model) ||
		isDoubaoHostedDeepSeekGAModel(model) ||
		isDoubaoHostedDeepSeekPreviewModel(model) ||
		isDoubaoHostedGlm52Model(model)
}

func isDeepSeekProvider(key string) bool {
	return strings.Contains(key, "deepseek")
}

func isArkProvider(key string) bool {
	return strings.Contains(key, "volces") ||
		strings.Contains(key, "volcengine") ||
		strings.Contains(key, "byteplus") ||
		strings.Contains(key, "ark")
}

// kimi-for-coding is a rolling service alias: since 2026-09-11 it is K2.8,
// while kimi-for-coding-highspeed remains K2.7. Do not infer one from the other.
func isKimiEffortModel(provider, model string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	return (provider == "kimi-cn" && model == "kimi-k3") ||
		(provider == "kimi-coding-plan" && (model == "k3" || model == "k3-256k" || model == "kimi-for-coding"))
}

func isKimiAuxiliaryPurpose(purpose RequestPurpose) bool {
	switch purpose {
	case RequestPurposeThreadTitle, RequestPurposeCompactSummary, RequestPurposeMemoryRecall, RequestPurposeMemoryMaintenance, RequestPurposeHookPrompt, RequestPurposeConnectionTest:
		return true
	default:
		return false
	}
}
