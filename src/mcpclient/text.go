package mcpclient

const truncatedSuffix = "... [truncated]"

func CapDescription(description string) string {
	return capRunes(description, MaxDescriptionLength)
}

func capStatusMessage(message string) string {
	return capRunes(message, MaxStatusMessageLen)
}

func capRunes(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	suffix := []rune(truncatedSuffix)
	if maxLen <= len(suffix) {
		return string(suffix[:maxLen])
	}
	return string(runes[:maxLen-len(suffix)]) + truncatedSuffix
}
