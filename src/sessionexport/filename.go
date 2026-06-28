package sessionexport

import (
	"strings"
	"time"
	"unicode"
)

const DefaultPromptMaxLength = 50

func ExtractFirstPrompt(messages []Message) string {
	return ExtractFirstPromptLine(messages, DefaultPromptMaxLength)
}

func ExtractFirstPromptLine(messages []Message, maxLength int) string {
	for _, msg := range messages {
		if msg.Role != RoleUser {
			continue
		}
		if text := firstTextPart(msg); text != "" {
			return limitPromptLine(firstLine(text), maxLength)
		}
	}
	return ""
}

func SanitizeFilename(text string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastHyphen = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || (r < unicode.MaxASCII && !isASCIILetterOrDigit(r)):
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func DefaultTextFilename(messages []Message, timestamp time.Time) string {
	return defaultFilename(messages, timestamp, ".txt")
}

func DefaultJSONLFilename(messages []Message, timestamp time.Time) string {
	return defaultFilename(messages, timestamp, ".jsonl")
}

func FormatTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	return timestamp.Format("2006-01-02-150405")
}

func defaultFilename(messages []Message, timestamp time.Time, extension string) string {
	stem := SanitizeFilename(ExtractFirstPrompt(messages))
	if stem == "" {
		stem = "conversation"
	}
	return FormatTimestamp(timestamp) + "-" + stem + extension
}

func firstTextPart(msg Message) string {
	for _, part := range msg.Parts {
		if part.ToolEvent != nil {
			continue
		}
		if part.Type == "" || part.Type == PartText {
			text := strings.TrimSpace(part.Text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func firstLine(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	line, _, _ := strings.Cut(strings.TrimSpace(normalized), "\n")
	return strings.TrimSpace(line)
}

func limitPromptLine(line string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= maxLength {
		return line
	}
	if maxLength <= 3 {
		return string(runes[:maxLength])
	}
	return string(runes[:maxLength-3]) + "..."
}

func isASCIILetterOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}
