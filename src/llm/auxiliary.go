package llm

import "strings"

// AuxiliaryConfig applies the host-owned small-model policy without changing
// credentials, endpoint, protocol, or profile identity. fallback is true when
// the endpoint has no confirmed safe auxiliary model mapping.
func AuxiliaryConfig(cfg Config) (aux Config, fallback bool) {
	aux = cfg
	switch strings.ToLower(strings.TrimSpace(cfg.ProviderID)) {
	case "deepseek":
		aux.Model = "deepseek-v4-flash"
	case "kimi-cn":
		aux.Model = "kimi-k2.6"
	case "kimi-coding-plan":
		aux.Model = "kimi-for-coding"
	default:
		fallback = true
	}
	return aux, fallback
}
