package feedbackbundle

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultDescription         = "No description provided."
	DefaultMaxMessages         = 20
	DefaultMaxTextRunes        = 2000
	DefaultMaxToolTextRunes    = 600
	DefaultMaxTaskMessageRunes = 300
)

const (
	noteRedacted      = "Sensitive values matching API key, bearer token, password, token, or secret patterns were redacted."
	noteTruncated     = "Long text fields were truncated before rendering."
	noteToolTruncated = "Tool output was summarized to avoid including full command or tool results."
)

type Message struct {
	Role      string
	Text      string
	Timestamp time.Time
	ToolName  string
}

type BackgroundTask struct {
	ID           string
	Type         string
	AgentID      string
	Status       string
	MessageCount int
	LastMessage  string
}

type Input struct {
	InitialDescription string
	Messages           []Message
	BackgroundTasks    []BackgroundTask
	MaxMessages        int
	MaxTextRunes       int
}

type Bundle struct {
	Description       string
	Messages          []Message
	BackgroundTasks   []BackgroundTask
	RedactionNotes    []string
	TotalMessageCount int
	OmittedMessages   int
}

type redactor struct {
	pattern     *regexp.Regexp
	replacement string
}

var redactors = []redactor{
	{
		pattern:     regexp.MustCompile(`(?i)\b(authorization)\b(\s*[:=]\s*)["']?(Bearer\s+)?[^"',\s)}\]]+["']?`),
		replacement: `${1}${2}${3}[REDACTED_TOKEN]`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`),
		replacement: `Bearer [REDACTED_TOKEN]`,
	},
	{
		pattern:     regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{4,}\b`),
		replacement: `[REDACTED_API_KEY]`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API[_-]?KEY|TOKEN|SECRET|PASSWORD|PASSWD|PWD)|password|passwd|pwd|token|secret|api[_-]?key)\b(\s*[:=]\s*)"[^"]*"`),
		replacement: `${1}${2}"[REDACTED]"`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API[_-]?KEY|TOKEN|SECRET|PASSWORD|PASSWD|PWD)|password|passwd|pwd|token|secret|api[_-]?key)\b(\s*[:=]\s*)'[^']*'`),
		replacement: `${1}${2}'[REDACTED]'`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API[_-]?KEY|TOKEN|SECRET|PASSWORD|PASSWD|PWD)|password|passwd|pwd|token|secret|api[_-]?key)\b(\s*[:=]\s*)[^"',\s)}\]]+`),
		replacement: `${1}${2}[REDACTED]`,
	},
}

func Build(input Input) Bundle {
	maxMessages := input.MaxMessages
	if maxMessages <= 0 {
		maxMessages = DefaultMaxMessages
	}
	maxTextRunes := input.MaxTextRunes
	if maxTextRunes <= 0 {
		maxTextRunes = DefaultMaxTextRunes
	}

	notes := noteSet{}
	description := DefaultDescription
	if strings.TrimSpace(input.InitialDescription) != "" {
		var redacted, truncated bool
		description, redacted, truncated = sanitizeAndLimit(input.InitialDescription, maxTextRunes)
		notes.addRedaction(redacted)
		notes.addTruncation(truncated)
	}

	orderedMessages := orderedMessages(input.Messages)
	start := 0
	if len(orderedMessages) > maxMessages {
		start = len(orderedMessages) - maxMessages
	}

	messages := make([]Message, 0, len(orderedMessages)-start)
	for _, item := range orderedMessages[start:] {
		message := item.message
		message.Role = strings.TrimSpace(Redact(message.Role))
		message.ToolName = strings.TrimSpace(Redact(message.ToolName))

		maxRunes := maxTextRunes
		if isToolMessage(message) && maxRunes > DefaultMaxToolTextRunes {
			maxRunes = DefaultMaxToolTextRunes
		}

		text, redacted, truncated := sanitizeAndLimit(message.Text, maxRunes)
		message.Text = text
		notes.addRedaction(redacted)
		if truncated && isToolMessage(message) && maxTextRunes > maxRunes {
			notes.add(noteToolTruncated)
		} else {
			notes.addTruncation(truncated)
		}
		messages = append(messages, message)
	}

	tasks := make([]BackgroundTask, 0, len(input.BackgroundTasks))
	for _, task := range input.BackgroundTasks {
		sanitized := BackgroundTask{
			ID:           sanitizeIdentifier(task.ID, &notes),
			Type:         sanitizeIdentifier(task.Type, &notes),
			AgentID:      sanitizeIdentifier(task.AgentID, &notes),
			Status:       sanitizeIdentifier(task.Status, &notes),
			MessageCount: task.MessageCount,
		}
		if sanitized.MessageCount < 0 {
			sanitized.MessageCount = 0
		}

		maxRunes := maxTextRunes
		if maxRunes > DefaultMaxTaskMessageRunes {
			maxRunes = DefaultMaxTaskMessageRunes
		}

		lastMessage, redacted, truncated := sanitizeAndLimit(task.LastMessage, maxRunes)
		sanitized.LastMessage = lastMessage
		notes.addRedaction(redacted)
		notes.addTruncation(truncated)
		tasks = append(tasks, sanitized)
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		return taskSortKey(tasks[i]) < taskSortKey(tasks[j])
	})

	return Bundle{
		Description:       description,
		Messages:          messages,
		BackgroundTasks:   tasks,
		RedactionNotes:    notes.list(),
		TotalMessageCount: len(input.Messages),
		OmittedMessages:   len(input.Messages) - len(messages),
	}
}

func RenderMarkdown(bundle Bundle) string {
	var b strings.Builder

	b.WriteString("## Description\n\n")
	b.WriteString(blankFallback(bundle.Description, DefaultDescription))
	b.WriteString("\n\n")

	b.WriteString("## Recent Messages\n\n")
	if len(bundle.Messages) == 0 {
		b.WriteString("- No recent messages.\n")
	} else {
		for _, message := range bundle.Messages {
			b.WriteString("- **")
			b.WriteString(blankFallback(message.Role, "unknown"))
			b.WriteString("**")
			if !message.Timestamp.IsZero() {
				b.WriteString(" ")
				b.WriteString(message.Timestamp.UTC().Format(time.RFC3339))
			}
			if message.ToolName != "" {
				b.WriteString(" `")
				b.WriteString(inlineCode(message.ToolName))
				b.WriteString("`")
			}
			b.WriteString("\n")
			appendIndentedText(&b, message.Text)
		}
	}
	if bundle.OmittedMessages > 0 {
		b.WriteString(fmt.Sprintf("\n_Omitted %d older message(s)._\n", bundle.OmittedMessages))
	}
	b.WriteString("\n")

	b.WriteString("## Background Tasks\n\n")
	if len(bundle.BackgroundTasks) == 0 {
		b.WriteString("- No background tasks.\n")
	} else {
		for _, task := range bundle.BackgroundTasks {
			b.WriteString("- `")
			b.WriteString(inlineCode(blankFallback(task.ID, "unknown-task")))
			b.WriteString("`")
			b.WriteString(" type=")
			b.WriteString(blankFallback(task.Type, "unknown"))
			b.WriteString(" agent=")
			b.WriteString(blankFallback(task.AgentID, "unknown"))
			b.WriteString(" status=")
			b.WriteString(blankFallback(task.Status, "unknown"))
			b.WriteString(fmt.Sprintf(" messages=%d\n", task.MessageCount))
			if strings.TrimSpace(task.LastMessage) != "" {
				b.WriteString("  Last message:\n")
				appendIndentedText(&b, task.LastMessage)
			}
		}
	}
	b.WriteString("\n")

	b.WriteString("## Redaction Notes\n\n")
	if len(bundle.RedactionNotes) == 0 {
		b.WriteString("- No sensitive values were detected by the bundle redactor.\n")
	} else {
		for _, note := range bundle.RedactionNotes {
			b.WriteString("- ")
			b.WriteString(note)
			b.WriteString("\n")
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func Redact(text string) string {
	redacted := text
	for _, r := range redactors {
		redacted = r.pattern.ReplaceAllString(redacted, r.replacement)
	}
	return redacted
}

type indexedMessage struct {
	index   int
	message Message
}

func orderedMessages(messages []Message) []indexedMessage {
	ordered := make([]indexedMessage, 0, len(messages))
	allHaveTimestamps := len(messages) > 0
	for i, message := range messages {
		if message.Timestamp.IsZero() {
			allHaveTimestamps = false
		}
		ordered = append(ordered, indexedMessage{index: i, message: message})
	}

	if allHaveTimestamps {
		sort.SliceStable(ordered, func(i, j int) bool {
			left := ordered[i].message.Timestamp
			right := ordered[j].message.Timestamp
			if left.Equal(right) {
				return ordered[i].index < ordered[j].index
			}
			return left.Before(right)
		})
	}

	return ordered
}

func sanitizeIdentifier(value string, notes *noteSet) string {
	redacted := Redact(strings.TrimSpace(value))
	notes.addRedaction(redacted != strings.TrimSpace(value))
	return redacted
}

func sanitizeAndLimit(text string, maxRunes int) (string, bool, bool) {
	redacted := Redact(text)
	limited, truncated := limitRunes(redacted, maxRunes)
	return limited, redacted != text, truncated
}

func limitRunes(text string, maxRunes int) (string, bool) {
	if maxRunes <= 0 || utf8.RuneCountInString(text) <= maxRunes {
		return text, false
	}

	runes := []rune(text)
	if maxRunes <= 3 {
		return string(runes[:maxRunes]), true
	}

	return string(runes[:maxRunes-3]) + "...", true
}

func isToolMessage(message Message) bool {
	return strings.EqualFold(strings.TrimSpace(message.Role), "tool") || strings.TrimSpace(message.ToolName) != ""
}

func appendIndentedText(b *strings.Builder, text string) {
	if strings.TrimSpace(text) == "" {
		b.WriteString("  _(empty)_\n")
		return
	}
	for _, line := range strings.Split(text, "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func blankFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func inlineCode(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}

func taskSortKey(task BackgroundTask) string {
	return task.ID + "\x00" + task.AgentID + "\x00" + task.Type
}

type noteSet map[string]struct{}

func (n *noteSet) add(note string) {
	if note == "" {
		return
	}
	if *n == nil {
		*n = noteSet{}
	}
	(*n)[note] = struct{}{}
}

func (n *noteSet) addRedaction(redacted bool) {
	if redacted {
		n.add(noteRedacted)
	}
}

func (n *noteSet) addTruncation(truncated bool) {
	if truncated {
		n.add(noteTruncated)
	}
}

func (n noteSet) list() []string {
	if len(n) == 0 {
		return nil
	}
	notes := make([]string, 0, len(n))
	for note := range n {
		notes = append(notes, note)
	}
	sort.Strings(notes)
	return notes
}
