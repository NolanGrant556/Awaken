package llm

import "strings"

func applyRequestPolicy(cfg Config, req Request) Request {
	switch req.Purpose {
	case RequestPurposeThreadTitle, RequestPurposeCompactSummary, RequestPurposeMemoryRecall, RequestPurposeMemoryMaintenance:
		return applyAuxiliaryRequestPolicy(cfg, req)
	default:
		return req
	}
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
