package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var piiSafePlatforms = map[Platform]struct{}{
	PlatformDingTalk:    {},
	PlatformFeishu:      {},
	PlatformTelegram:    {},
	PlatformWeChat:      {},
	PlatformWhatsApp:    {},
	PlatformSignal:      {},
	PlatformBlueBubbles: {},
}

var (
	sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key|authorization|credential)(\s*[:=]\s*)([^\s,;]+)`)
	bearerPattern              = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
)

func shouldRedactPII(platform Platform) bool {
	_, ok := piiSafePlatforms[platform]
	return ok
}

func hashID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func hashUserID(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "user_" + hashID(value)
}

func hashChatID(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if idx := strings.Index(value, ":"); idx > 0 {
		return value[:idx+1] + hashID(value[idx+1:])
	}
	return hashID(value)
}

func redactSensitiveText(value string) string {
	if value == "" {
		return ""
	}
	value = sensitiveAssignmentPattern.ReplaceAllString(value, `${1}${2}[redacted]`)
	return bearerPattern.ReplaceAllString(value, "Bearer [redacted]")
}

func sanitizedMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if isSensitiveKey(key) {
			out[key] = "[redacted]"
			continue
		}
		out[key] = redactSensitiveText(value)
	}
	return out
}

func copyMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"token", "secret", "password", "api_key", "apikey", "authorization", "credential"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
