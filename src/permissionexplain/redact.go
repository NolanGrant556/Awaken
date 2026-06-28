package permissionexplain

import "regexp"

var (
	authorizationBearerPattern = regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s'",}]+`)
	secretAssignmentPattern    = regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password|passwd|access[_-]?token|refresh[_-]?token)\s*[:=]\s*)[^\s'",}&]+`)
	jsonSecretPattern          = regexp.MustCompile(`(?i)("(?:api[_-]?key|token|secret|password|access[_-]?token|refresh[_-]?token)"\s*:\s*")[^"]+(")`)
	bearerPattern              = regexp.MustCompile(`(?i)(bearer\s+)[^\s'",}]+`)
	openAIKeyPattern           = regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`)
)

func RedactSecrets(value string) string {
	value = authorizationBearerPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = jsonSecretPattern.ReplaceAllString(value, "${1}[REDACTED]${2}")
	value = secretAssignmentPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = bearerPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = openAIKeyPattern.ReplaceAllString(value, "[REDACTED]")
	return value
}
