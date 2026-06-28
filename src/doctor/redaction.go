package doctor

import (
	"regexp"
	"strings"
)

var (
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API[_-]?KEY|TOKEN|SECRET|PASSWORD)[A-Z0-9_]*)\s*[:=]\s*("[^"\r\n]*"|'[^'\r\n]*'|[^\s\r\n]+)`)
	bearerTokenPattern      = regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	skTokenPattern          = regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{6})[A-Za-z0-9_-]{6,}\b`)
)

// Redact removes common provider credentials from diagnostic output.
func Redact(text string) string {
	if text == "" {
		return ""
	}
	redacted := secretAssignmentPattern.ReplaceAllString(text, "$1=<redacted>")
	redacted = bearerTokenPattern.ReplaceAllString(redacted, "$1<redacted>")
	redacted = skTokenPattern.ReplaceAllString(redacted, "$1<redacted>")
	return redacted
}

func redactLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	redacted := make([]string, 0, len(lines))
	for _, line := range lines {
		redacted = append(redacted, Redact(line))
	}
	return redacted
}

func redactCommandOutput(result CommandResult) string {
	var parts []string
	if strings.TrimSpace(result.Stdout) != "" {
		parts = append(parts, "stdout:\n"+strings.TrimSpace(result.Stdout))
	}
	if strings.TrimSpace(result.Stderr) != "" {
		parts = append(parts, "stderr:\n"+strings.TrimSpace(result.Stderr))
	}
	if len(parts) == 0 {
		return ""
	}
	return Redact(strings.Join(parts, "\n"))
}
