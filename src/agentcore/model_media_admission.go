package agentcore

import (
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

const unsupportedVisionProjectionPrefix = "[Awaken attachment projection: image data retained locally but not sent because the current model profile does not support vision"

// projectMessagesForModelCapabilities creates the provider-admitted view of a
// conversation. Durable history remains lossless; only the request clone loses
// media bytes that the currently selected model cannot accept.
func projectMessagesForModelCapabilities(messages []convcontext.Message, capabilities tool.ModelCapabilities) []convcontext.Message {
	projected := cloneContextMessages(messages)
	for index := range projected {
		message := &projected[index]
		// A projection belongs to one pinned model request. Recompute it from the
		// current capability snapshot instead of inheriting an earlier run's note.
		message.ProviderProjectionText = ""
		omittedImages := make([]string, 0)
		if !capabilities.SupportsVision {
			for attachmentIndex := range message.Attachments {
				attachment := &message.Attachments[attachmentIndex]
				if !strings.EqualFold(strings.TrimSpace(attachment.Kind), "image") {
					continue
				}
				attachment.DataBase64 = ""
				omittedImages = appendUniqueMediaLabel(omittedImages, attachment.DisplayName, attachment.ID)
			}
		}

		for partIndex := range message.ModelContent {
			part := &message.ModelContent[partIndex]
			switch {
			case part.Type == "image" && !capabilities.SupportsVision:
				part.DataBase64 = ""
			case part.Type == "file" && strings.EqualFold(strings.TrimSpace(part.MimeType), "application/pdf") && !capabilities.SupportsNativePDF:
				part.DataBase64 = ""
			}
		}

		// Tool ModelPayload already carries stable media metadata. User image-only
		// turns need an explicit text leaf so text-only providers never receive an
		// empty user message after the binary block is removed.
		if message.Role == convcontext.RoleUser && len(omittedImages) > 0 {
			note := unsupportedVisionProjectionPrefix + ": " + strings.Join(omittedImages, ", ") + "]"
			if !strings.Contains(message.ProviderProjectionText, unsupportedVisionProjectionPrefix) {
				message.ProviderProjectionText = note
			}
		}
	}
	return projected
}

func appendUniqueMediaLabel(labels []string, values ...string) []string {
	label := ""
	for _, value := range values {
		if normalized := strings.Join(strings.Fields(value), " "); normalized != "" {
			label = normalized
			break
		}
	}
	if label == "" {
		label = "unnamed image"
	}
	for _, existing := range labels {
		if existing == label {
			return labels
		}
	}
	return append(labels, fmt.Sprintf("%q", label))
}
