package terminaltext

import "strings"

const (
	BashInputTag          = "bash-input"
	BashStdoutTag         = "bash-stdout"
	BashStderrTag         = "bash-stderr"
	LocalCommandStdoutTag = "local-command-stdout"
	LocalCommandStderrTag = "local-command-stderr"
	LocalCommandCaveatTag = "local-command-caveat"
)

// TerminalOutputTags are XML-like tags that wrap terminal activity rather than
// user-authored prompts.
var TerminalOutputTags = []string{
	BashInputTag,
	BashStdoutTag,
	BashStderrTag,
	LocalCommandStdoutTag,
	LocalCommandStderrTag,
	LocalCommandCaveatTag,
}

type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeAssistant MessageType = "assistant"
	MessageTypeTool      MessageType = "tool"
)

type ContentBlockType string

const (
	ContentBlockTypeText       ContentBlockType = "text"
	ContentBlockTypeImage      ContentBlockType = "image"
	ContentBlockTypeDocument   ContentBlockType = "document"
	ContentBlockTypeToolResult ContentBlockType = "tool_result"
)

// Message is a lightweight attribution/counting input. Use Content for plain
// string messages, or ContentBlocks for structured user content.
type Message struct {
	Type          MessageType
	Content       string
	ContentBlocks []ContentBlock
}

// ContentBlock represents the subset of structured message blocks needed to
// distinguish user prompts from tool results and terminal activity.
type ContentBlock struct {
	Type ContentBlockType
	Text string
}

// IsTerminalOutput reports whether text contains one of Awaken's terminal
// output XML-like tags.
func IsTerminalOutput(text string) bool {
	for _, tag := range TerminalOutputTags {
		if containsXMLLikeTag(text, tag) {
			return true
		}
	}
	return false
}

// CountUserPrompts counts real user prompts, excluding empty content,
// tool_result blocks, and messages that only contain terminal activity.
func CountUserPrompts(messages []Message) int {
	count := 0
	for _, message := range messages {
		if message.Type != MessageTypeUser {
			continue
		}
		if hasUserPromptContent(message) {
			count++
		}
	}
	return count
}

func hasUserPromptContent(message Message) bool {
	if hasUserPromptText(message.Content) {
		return true
	}

	for _, block := range message.ContentBlocks {
		if isUserPromptBlock(block) {
			return true
		}
	}

	return false
}

func hasUserPromptText(text string) bool {
	text = strings.TrimSpace(text)
	return text != "" && !IsTerminalOutput(text)
}

func isUserPromptBlock(block ContentBlock) bool {
	switch block.Type {
	case ContentBlockTypeText:
		return hasUserPromptText(block.Text)
	case ContentBlockTypeImage, ContentBlockTypeDocument:
		return true
	default:
		return false
	}
}

func containsXMLLikeTag(text, tag string) bool {
	for start := strings.IndexByte(text, '<'); start >= 0; {
		nameStart := start + 1
		if nameStart < len(text) && text[nameStart] == '/' {
			nameStart++
		}

		if strings.HasPrefix(text[nameStart:], tag) {
			nameEnd := nameStart + len(tag)
			if nameEnd < len(text) && isTagBoundary(text[nameEnd]) {
				return true
			}
		}

		next := start + 1
		if next >= len(text) {
			return false
		}
		offset := strings.IndexByte(text[next:], '<')
		if offset == -1 {
			return false
		}
		start = next + offset
	}
	return false
}

func isTagBoundary(ch byte) bool {
	switch ch {
	case '>', '/', ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
