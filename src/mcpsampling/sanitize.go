package mcpsampling

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)sk-[a-z0-9_-]+`),
	regexp.MustCompile(`ghp_[A-Za-z0-9_]+`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]+`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{20,}`),
	regexp.MustCompile(`(?i)(api[_-]?key|token|authorization|password|secret)\s*[:=]\s*["']?[^,\s"']+`),
}

func sanitizeText(value string) string {
	result := value
	for _, pattern := range secretPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}
