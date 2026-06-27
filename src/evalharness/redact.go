package evalharness

import "regexp"

var secretRedactions = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{
		pattern: regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s,;]+`),
		replace: `${1}[REDACTED]`,
	},
	{
		pattern: regexp.MustCompile(`(?i)(x-api-key\s*[:=]\s*)[^\s,;]+`),
		replace: `${1}[REDACTED]`,
	},
	{
		pattern: regexp.MustCompile(`(?i)("(?:[^"]*(?:api[_-]?key|token|secret|password|passwd|pwd)[^"]*)"\s*:\s*)("[^"]*"|[0-9A-Za-z._/\-+=]+)`),
		replace: `${1}"[REDACTED]"`,
	},
	{
		pattern: regexp.MustCompile(`(?i)((?:[a-z0-9_]*api[_-]?key|token|secret|password|passwd|pwd)\s*[:=]\s*)("[^"]+"|'[^']+'|[^\s,;]+)`),
		replace: `${1}[REDACTED]`,
	},
	{
		pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9][A-Za-z0-9_-]{6,}\b`),
		replace: `[REDACTED]`,
	},
}

func RedactSecrets(text string) string {
	redacted := text
	for _, redaction := range secretRedactions {
		redacted = redaction.pattern.ReplaceAllString(redacted, redaction.replace)
	}
	return redacted
}

func redactMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(metadata))
	for key, value := range metadata {
		redacted[key] = RedactSecrets(value)
	}
	return redacted
}
