package activitynotify

import "regexp"

var secretRedactors = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{
		pattern:     regexp.MustCompile(`(?i)([A-Z0-9_]*(?:API[_-]?KEY|TOKEN|SECRET|PASSWORD)[A-Z0-9_]*\s*[:=]\s*)("[^"]+"|'[^']+'|[^\s,;]+)`),
		replacement: `${1}[REDACTED]`,
	},
	{
		pattern:     regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_-]{8,}`),
		replacement: `[REDACTED]`,
	},
	{
		pattern:     regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{8,}`),
		replacement: `[REDACTED]`,
	},
	{
		pattern:     regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		replacement: `[REDACTED]`,
	},
}

func RedactSecrets(text string) string {
	for _, redactor := range secretRedactors {
		text = redactor.pattern.ReplaceAllString(text, redactor.replacement)
	}
	return text
}
