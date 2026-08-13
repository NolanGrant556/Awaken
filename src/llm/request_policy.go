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
// provider boundary. Auxiliary requests intentionally return no effort even
// when their Config came from the same profile as an agent turn.
func nativeReasoningEffort(cfg Config, req Request) string {
	if !isAgentExecutionPurpose(req.Purpose) {
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
	case (provider == "qwen-token-plan" && model == "qwen3.8-max-preview") ||
		(provider == "qwen" && model == "qwen3.8-max"):
		switch effort {
		case "low":
			return "low"
		case "high":
			return "medium"
		case "max":
			return "xhigh"
		}
	case provider == "doubao" &&
		(model == "doubao-seed-2.1-pro" || model == "doubao-seed-2-1-pro-260628"):
		switch effort {
		case "low":
			return "low"
		case "high":
			return "medium"
		case "max":
			return "high"
		}
	case provider == "kimi-cn" && model == "kimi-k3":
		return effort
	case provider == "kimi-coding-plan" && (model == "k3" || model == "k3-256k"):
		return effort
	case (provider == "glm" || provider == "glm-coding-plan") && model == "glm-5.2":
		if effort == "high" || effort == "max" {
			return effort
		}
	case provider == "deepseek" && strings.HasPrefix(model, "deepseek-v4"):
		if effort == "high" || effort == "max" {
			return effort
		}
	case provider == "volcengine-agent-plan":
		switch model {
		case "kimi-k3":
			return effort
		case "glm-5.2", "deepseek-v4-pro":
			if effort == "high" || effort == "max" {
				return effort
			}
		case "qwen3.8-max-preview":
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
	return purpose == RequestPurposeAgentTurn || purpose == RequestPurposeSubagent || purpose == RequestPurposeScheduledTask || purpose == RequestPurposeHookAgent
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
	key := providerModelKey(cfg, req)

	switch {
	case isKimiK27Code(model):
		// Kimi K2.7 Code always thinks and rejects thinking.disabled.
		req.DisableReasoning = false
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

func isKimiK27Code(model string) bool {
	return model == "kimi-k2.7-code" ||
		model == "kimi-k2.7-code-highspeed" ||
		model == "kimi-for-coding" ||
		model == "kimi-for-coding-highspeed"
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
