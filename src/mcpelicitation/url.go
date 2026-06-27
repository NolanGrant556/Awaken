package mcpelicitation

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

type URLPolicy struct {
	AllowedSchemes []string
}

type URLInfo struct {
	URL         string
	RedactedURL string
	Scheme      string
	Host        string
}

func DefaultURLPolicy() URLPolicy {
	return URLPolicy{AllowedSchemes: []string{"http", "https"}}
}

func ValidateURL(raw string) (URLInfo, error) {
	return DefaultURLPolicy().Validate(raw)
}

func (p URLPolicy) Validate(raw string) (URLInfo, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return URLInfo{}, fmt.Errorf("url is required")
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return URLInfo{}, fmt.Errorf("url contains control characters")
		}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return URLInfo{}, fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return URLInfo{}, fmt.Errorf("url scheme is required")
	}
	if !p.schemeAllowed(scheme) {
		return URLInfo{}, fmt.Errorf("url scheme %q is not allowed", scheme)
	}
	if parsed.Host == "" {
		return URLInfo{}, fmt.Errorf("url host is required")
	}
	if parsed.User != nil {
		return URLInfo{}, fmt.Errorf("url user info is not allowed")
	}

	return URLInfo{
		URL:         trimmed,
		RedactedURL: RedactURL(trimmed),
		Scheme:      scheme,
		Host:        parsed.Host,
	}, nil
}

func (p URLPolicy) schemeAllowed(scheme string) bool {
	allowed := p.AllowedSchemes
	if len(allowed) == 0 {
		allowed = DefaultURLPolicy().AllowedSchemes
	}
	for _, candidate := range allowed {
		if strings.EqualFold(candidate, scheme) {
			return true
		}
	}
	return false
}

func RedactURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "<invalid-url>"
	}

	redacted := *parsed
	if redacted.User != nil {
		redacted.User = url.User("[redacted]")
	}

	query := redacted.Query()
	if len(query) > 0 {
		keys := make([]string, 0, len(query))
		for key := range query {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			values := query[key]
			for i, value := range values {
				if sensitiveQueryKey(key) || sensitiveQueryValue(value) {
					values[i] = "[redacted]"
				}
			}
			query[key] = values
		}
		redacted.RawQuery = query.Encode()
	}

	if redacted.Fragment != "" && sensitiveQueryValue(redacted.Fragment) {
		redacted.Fragment = "[redacted]"
	}

	return redacted.String()
}

func sensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.NewReplacer("-", "_", ".", "_").Replace(normalized)
	switch normalized {
	case "access_token", "refresh_token", "id_token", "token", "secret",
		"client_secret", "password", "passwd", "pwd", "api_key", "apikey",
		"access_key", "auth", "authorization", "credential", "credentials",
		"session", "session_id", "sessionid", "sid", "signature", "sig", "code":
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "authorization")
}

func sensitiveQueryValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"bearer ", "sk-", "sk_", "ghp_", "gho_", "github_pat_", "xoxb-", "xoxp-"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	if len(trimmed) >= 20 && strings.Count(trimmed, ".") >= 2 {
		return true
	}
	if len(trimmed) >= 32 {
		var hasLetter, hasDigit bool
		for _, r := range trimmed {
			hasLetter = hasLetter || unicode.IsLetter(r)
			hasDigit = hasDigit || unicode.IsDigit(r)
		}
		return hasLetter && hasDigit
	}
	return false
}
