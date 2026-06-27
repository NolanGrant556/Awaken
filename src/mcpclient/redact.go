package mcpclient

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var (
	bearerTokenPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
	sensitiveKVPattern = regexp.MustCompile(`(?i)\b(authorization|api[-_]?key|token|secret|password|cookie)\b(\s*[:=]\s*)([^\s,;]+)`)
)

func RedactConfigSecrets(message string, configs ...ServerConfig) string {
	redacted := message
	for _, config := range configs {
		for _, secret := range config.secretValues() {
			if shouldRedactValue(secret) {
				redacted = strings.ReplaceAll(redacted, secret, "[redacted]")
			}
		}
	}
	redacted = bearerTokenPattern.ReplaceAllString(redacted, "Bearer [redacted]")
	redacted = sensitiveKVPattern.ReplaceAllString(redacted, "$1$2[redacted]")
	return redacted
}

func SafeEndpoint(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "<invalid-url>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (c ServerConfig) secretValues() []string {
	var values []string
	if c.Stdio != nil {
		values = appendMapValues(values, c.Stdio.Env)
	}
	values = appendRemoteSecrets(values, c.HTTP)
	values = appendRemoteSecrets(values, c.SSE)
	values = appendRemoteSecrets(values, c.WS)
	sort.Strings(values)
	return values
}

func appendRemoteSecrets(values []string, remote *RemoteConfig) []string {
	if remote == nil {
		return values
	}
	values = appendMapValues(values, remote.Headers)
	if remote.OAuth != nil {
		values = append(values, remote.OAuth.ClientSecret)
	}
	return values
}

func appendMapValues(values []string, source map[string]string) []string {
	for _, value := range source {
		values = append(values, value)
	}
	return values
}

func shouldRedactValue(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 3
}
