package llm

import (
	"fmt"
	"strings"

	convcontext "awaken/src/context"
)

func userAttachmentPathText(attachments []convcontext.MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, attachment := range attachments {
		path := strings.TrimSpace(attachment.Path)
		if path == "" {
			continue
		}
		label := strings.TrimSpace(attachment.DisplayName)
		if label == "" {
			label = path
		}
		if builder.Len() == 0 {
			builder.WriteString("Files mentioned by the user:\n")
		}
		builder.WriteString(fmt.Sprintf("- %s: %s\n", label, path))
	}
	return strings.TrimRight(builder.String(), "\n")
}

func userTextWithAttachmentPaths(content string, attachments []convcontext.MessageAttachment) string {
	attachmentPaths := userAttachmentPathText(attachments)
	switch {
	case strings.TrimSpace(content) == "":
		return attachmentPaths
	case attachmentPaths == "":
		return content
	default:
		return strings.TrimRight(content, "\n") + "\n\n" + attachmentPaths
	}
}

func userTextWithProviderProjection(content string, attachments []convcontext.MessageAttachment, projection string) string {
	text := userTextWithAttachmentPaths(content, attachments)
	projection = strings.TrimSpace(projection)
	switch {
	case projection == "":
		return text
	case strings.TrimSpace(text) == "":
		return projection
	default:
		return strings.TrimRight(text, "\n") + "\n\n" + projection
	}
}
