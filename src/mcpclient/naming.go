package mcpclient

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var providerSafeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func NormalizeNameForMCP(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	normalized := strings.Trim(b.String(), "_")
	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}
	if normalized == "" {
		normalized = "unnamed_" + shortHash(name)
	}
	return normalized
}

func ProviderSafeName(name string) string {
	return truncateWithHash(NormalizeNameForMCP(name), MaxProviderNameLen)
}

func IsProviderSafeName(name string) bool {
	return name != "" && len(name) <= MaxProviderNameLen && providerSafeNamePattern.MatchString(name)
}

func MCPPrefix(serverName string) string {
	return "mcp__" + NormalizeNameForMCP(serverName) + "__"
}

func BuildToolName(serverName string, toolName string) string {
	server := NormalizeNameForMCP(serverName)
	remoteTool := NormalizeNameForMCP(toolName)
	fullName := "mcp__" + server + "__" + remoteTool
	if len(fullName) <= MaxProviderNameLen {
		return fullName
	}

	const delimiterLen = len("mcp____")
	available := MaxProviderNameLen - delimiterLen
	serverBudget := 28
	if len(server) < serverBudget {
		serverBudget = len(server)
	}
	if serverBudget < 8 {
		serverBudget = 8
	}
	if serverBudget > available-8 {
		serverBudget = available - 8
	}
	toolBudget := available - serverBudget

	server = truncateWithHash(server, serverBudget)
	remoteTool = truncateWithHash(remoteTool, toolBudget)
	return "mcp__" + server + "__" + remoteTool
}

func MCPInfoFromString(toolName string) (serverName string, remoteToolName string, ok bool) {
	parts := strings.Split(toolName, "__")
	if len(parts) < 2 || parts[0] != "mcp" || parts[1] == "" {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[1], "", true
	}
	return parts[1], strings.Join(parts[2:], "__"), true
}

func truncateWithHash(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(value) <= maxLen {
		return value
	}
	hash := shortHash(value)
	if maxLen <= len(hash) {
		return hash[:maxLen]
	}
	keep := maxLen - len(hash) - 1
	prefix := strings.TrimRight(value[:keep], "_-")
	if prefix == "" {
		return hash[:maxLen]
	}
	return prefix + "_" + hash
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}
