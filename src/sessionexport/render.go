package sessionexport

import (
	"encoding/json"
	"strings"
	"time"
)

type PlainTextOptions struct {
	IncludeTimestamps bool
}

func RenderPlainText(messages []Message, opts ...PlainTextOptions) string {
	options := PlainTextOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}

	var b strings.Builder
	for i, msg := range messages {
		if i > 0 {
			b.WriteString("\n\n")
		}

		b.WriteString(renderRoleHeader(msg, options))
		b.WriteByte('\n')

		wrotePart := false
		for _, part := range msg.Parts {
			rendered := renderPart(part)
			if rendered == "" {
				continue
			}
			if wrotePart {
				b.WriteByte('\n')
			}
			b.WriteString(rendered)
			wrotePart = true
		}
	}

	return b.String()
}

func RenderJSONL(messages []Message) (string, error) {
	var b strings.Builder
	for _, msg := range messages {
		line, err := NDJSONSafeStringify(msg)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func NDJSONSafeStringify(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	line := string(bytes)
	line = strings.ReplaceAll(line, "\u2028", `\u2028`)
	line = strings.ReplaceAll(line, "\u2029", `\u2029`)
	return line, nil
}

func renderRoleHeader(msg Message, options PlainTextOptions) string {
	header := "## " + roleLabel(msg.Role)
	if options.IncludeTimestamps && msg.Timestamp != nil && !msg.Timestamp.IsZero() {
		header += " (" + msg.Timestamp.UTC().Format(time.RFC3339) + ")"
	}
	return header
}

func roleLabel(role Role) string {
	switch role {
	case RoleSystem:
		return "System"
	case RoleUser:
		return "User"
	case RoleAssistant:
		return "Assistant"
	case RoleTool:
		return "Tool"
	default:
		if role == "" {
			return "Message"
		}
		return strings.ToUpper(string(role[:1])) + string(role[1:])
	}
}

func renderPart(part Part) string {
	if part.ToolEvent != nil {
		event := *part.ToolEvent
		switch part.Type {
		case PartToolCall:
			event.Kind = ToolEventCall
		case PartToolResult:
			event.Kind = ToolEventResult
		}
		return renderToolEvent(event)
	}

	if part.Type == "" || part.Type == PartText {
		return strings.Trim(part.Text, "\n")
	}
	return ""
}

func renderToolEvent(event ToolEvent) string {
	label := "tool event"
	if event.Kind == ToolEventCall {
		label = "tool call"
	}
	if event.Kind == ToolEventResult {
		label = "tool result"
	}

	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(label)
	b.WriteByte(']')

	if event.Name != "" {
		b.WriteByte(' ')
		b.WriteString(event.Name)
	}
	if event.ID != "" {
		b.WriteString(" (")
		b.WriteString(event.ID)
		b.WriteByte(')')
	}
	if event.Status != "" {
		b.WriteString(" status=")
		b.WriteString(event.Status)
	}

	if summary := toolSummary(event); summary != "" {
		b.WriteString(": ")
		b.WriteString(summary)
	}

	return b.String()
}

func toolSummary(event ToolEvent) string {
	switch {
	case event.Summary != "":
		return compactSummary(event.Summary)
	case event.Error != "":
		return compactSummary("error: " + event.Error)
	case event.Kind == ToolEventResult && event.Output != "":
		return compactSummary(event.Output)
	case event.Input != "":
		return compactSummary(event.Input)
	case event.Output != "":
		return compactSummary(event.Output)
	default:
		return ""
	}
}

func compactSummary(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}

	const maxRunes = 240
	summary := strings.Join(fields, " ")
	runes := []rune(summary)
	if len(runes) <= maxRunes {
		return summary
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}
