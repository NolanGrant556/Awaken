package settingssync

import "strings"

const RedactedValue = "[REDACTED]"

var sensitiveKeyFragments = []string{
	"apikey",
	"api_key",
	"auth",
	"bearer",
	"clientsecret",
	"client_secret",
	"credential",
	"keyfile",
	"password",
	"privatekey",
	"private_key",
	"refresh_token",
	"secret",
	"token",
}

func IsSensitiveKey(key string) bool {
	normalized := normalizeKey(key)
	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(normalized, normalizeKey(fragment)) {
			return true
		}
	}
	return false
}

func LooksSensitiveValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if trimmed == "" {
		return false
	}
	if strings.Contains(lower, "begin private key") || strings.Contains(lower, "begin rsa private key") {
		return true
	}
	prefixes := []string{
		"sk-",
		"sk_",
		"sk-proj-",
		"ghp_",
		"gho_",
		"github_pat_",
		"xoxb-",
		"xoxp-",
		"bearer ",
		"akia",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) && len(trimmed) >= len(prefix)+8 {
			return true
		}
	}
	return false
}

func RedactValue(key, value string, sensitive bool) string {
	if sensitive || IsSensitiveKey(key) || LooksSensitiveValue(value) {
		return RedactedValue
	}
	return value
}

func normalizeKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	replacer := strings.NewReplacer(".", "", "-", "", "_", "", " ", "", "/", "", ":", "")
	return replacer.Replace(key)
}
