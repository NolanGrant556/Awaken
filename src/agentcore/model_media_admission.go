package agentcore

import (
	stdcontext "context"
	"fmt"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

const (
	unsupportedVisionProjectionPrefix = "[Awaken attachment projection: image data retained locally but not sent because the current model profile does not support vision"
	mediaStubPrefix                   = "[MEDIA_STUB]"
)

type requestProjectionOptions struct {
	capabilities  tool.ModelCapabilities
	windowConfig  ContextWindowConfig
	windowState   ContextWindowState
	threadID      string
	store         tool.ModelContentStore
	stripAllMedia bool
}

// projectMessagesForModelCapabilities creates the provider-admitted view of a
// conversation. Durable history remains lossless; only the request clone loses
// media bytes that the currently selected model cannot accept.
func projectMessagesForModelCapabilities(messages []convcontext.Message, capabilities tool.ModelCapabilities) []convcontext.Message {
	return projectMessagesForRequest(messages, requestProjectionOptions{capabilities: capabilities})
}

func projectMessagesForRequest(messages []convcontext.Message, opts requestProjectionOptions) []convcontext.Message {
	projected := projectMessagesForCapabilities(messages, opts.capabilities)
	if opts.stripAllMedia {
		return stripImagesFromMessages(projected)
	}
	return applyWindowRequestProjection(projected, opts)
}

func projectMessagesForCapabilities(messages []convcontext.Message, capabilities tool.ModelCapabilities) []convcontext.Message {
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
				clearAttachmentMediaAdmission(attachment)
				omittedImages = appendUniqueMediaLabel(omittedImages, attachment.DisplayName, attachment.ID)
			}
		}

		for partIndex := range message.ModelContent {
			part := &message.ModelContent[partIndex]
			switch {
			case part.Type == "image" && !capabilities.SupportsVision:
				clearModelContentMediaAdmission(part)
			case part.Type == "file" && strings.EqualFold(strings.TrimSpace(part.MimeType), "application/pdf") && !capabilities.SupportsNativePDF:
				clearModelContentMediaAdmission(part)
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

func applyWindowRequestProjection(messages []convcontext.Message, opts requestProjectionOptions) []convcontext.Message {
	if len(messages) == 0 {
		return messages
	}
	projected := cloneContextMessages(messages)
	applyPagedRetrievalProjection(projected)
	lastStart := lastSafeGroupStart(projected)
	for i := 0; i < lastStart; i++ {
		stubOlderMessageMedia(&projected[i], opts.capabilities)
	}

	state := opts.windowState
	if state.EffectiveWindow == 0 && state.ModelWindow == 0 {
		state = NewContextWindowStateWithConfig(ContextEnvelope{Messages: projected}, opts.windowConfig)
	}
	budget := state.blockingThreshold() - state.ConservativeProjectedTokens()
	if budget < 0 {
		budget = 0
	}
	for i := len(projected) - 1; i >= lastStart; i-- {
		hydrateNearTailMessageMedia(&projected[i], &budget, opts)
	}
	return projected
}

func lastSafeGroupStart(messages []convcontext.Message) int {
	groups := compactSafeGroups(messages)
	if len(groups) == 0 {
		return 0
	}
	start := len(messages) - len(groups[len(groups)-1])
	if start < 0 {
		return 0
	}
	return start
}

func stubOlderMessageMedia(message *convcontext.Message, capabilities tool.ModelCapabilities) {
	stubs := make([]string, 0)
	for i := range message.Attachments {
		attachment := &message.Attachments[i]
		if !isAttachmentSendable(attachment, capabilities) {
			continue
		}
		if !convcontext.MessageAttachmentHasAdmittedMedia(*attachment) && strings.TrimSpace(attachment.DataBase64) == "" {
			continue
		}
		stubs = append(stubs, attachmentMediaStub(*attachment))
		attachment.DataBase64 = ""
	}
	for i := range message.ModelContent {
		part := &message.ModelContent[i]
		if !isModelPartSendable(*part, capabilities) {
			continue
		}
		if !convcontext.ModelContentPartHasAdmittedMedia(*part) && strings.TrimSpace(part.DataBase64) == "" {
			continue
		}
		stubs = append(stubs, modelContentMediaStub(*part))
		part.DataBase64 = ""
	}
	appendMediaStubs(message, stubs)
}

func hydrateNearTailMessageMedia(message *convcontext.Message, budget *int, opts requestProjectionOptions) {
	if message == nil || budget == nil {
		return
	}
	for i := len(message.Attachments) - 1; i >= 0; i-- {
		attachment := &message.Attachments[i]
		if !isAttachmentSendable(attachment, opts.capabilities) {
			attachment.DataBase64 = ""
			continue
		}
		if !convcontext.MessageAttachmentHasAdmittedMedia(*attachment) && strings.TrimSpace(attachment.DataBase64) == "" {
			continue
		}
		if *budget < convcontext.MediaPartTokenEstimate {
			stubs := []string{attachmentMediaStub(*attachment)}
			attachment.DataBase64 = ""
			appendMediaStubs(message, stubs)
			continue
		}
		if strings.TrimSpace(attachment.DataBase64) != "" {
			*budget -= convcontext.MediaPartTokenEstimate
		} else {
			attachment.DataBase64 = ""
		}
	}

	for i := len(message.ModelContent) - 1; i >= 0; i-- {
		part := &message.ModelContent[i]
		if !isModelPartSendable(*part, opts.capabilities) {
			part.DataBase64 = ""
			continue
		}
		if !convcontext.ModelContentPartHasAdmittedMedia(*part) && strings.TrimSpace(part.DataBase64) == "" {
			continue
		}
		if *budget < convcontext.MediaPartTokenEstimate {
			stubs := []string{modelContentMediaStub(*part)}
			part.DataBase64 = ""
			appendMediaStubs(message, stubs)
			continue
		}
		if strings.TrimSpace(part.DataBase64) == "" {
			hydrated, warnings := hydrateModelContentParts(opts, []tool.ModelContentPart{*part})
			if len(hydrated) == 1 {
				part.DataBase64 = hydrated[0].DataBase64
				if hydrated[0].ByteSize > 0 {
					part.ByteSize = hydrated[0].ByteSize
				}
			}
			appendHydrateWarnings(message, warnings)
		}
		if strings.TrimSpace(part.DataBase64) == "" {
			appendMediaStubs(message, []string{modelContentMediaStub(*part)})
			continue
		}
		*budget -= convcontext.MediaPartTokenEstimate
	}
}

func hydrateModelContentParts(opts requestProjectionOptions, parts []tool.ModelContentPart) ([]tool.ModelContentPart, []tool.OutcomeNotice) {
	if opts.store == nil || strings.TrimSpace(opts.threadID) == "" {
		return parts, nil
	}
	return opts.store.HydrateToolResult(stdcontext.Background(), opts.threadID, parts)
}

func appendHydrateWarnings(message *convcontext.Message, warnings []tool.OutcomeNotice) {
	if message == nil {
		return
	}
	for _, warning := range warnings {
		note := fmt.Sprintf("[%s: %s]", warning.Code, warning.Message)
		if strings.Contains(message.Content, note) {
			continue
		}
		if strings.TrimSpace(message.Content) == "" {
			message.Content = note
			continue
		}
		message.Content += "\n" + note
	}
}

func stripImagesFromMessages(messages []convcontext.Message) []convcontext.Message {
	projected := cloneContextMessages(messages)
	for i := range projected {
		stripMessageMediaPayloads(&projected[i])
	}
	return projected
}

func stripMessageMediaPayloads(message *convcontext.Message) {
	if message == nil {
		return
	}
	stubs := make([]string, 0)
	for i := range message.Attachments {
		attachment := &message.Attachments[i]
		if strings.EqualFold(strings.TrimSpace(attachment.Kind), "image") &&
			(strings.TrimSpace(attachment.DataBase64) != "" || convcontext.MessageAttachmentHasAdmittedMedia(*attachment)) {
			stubs = append(stubs, attachmentMediaStub(*attachment))
		}
		attachment.DataBase64 = ""
	}
	kept := make([]tool.ModelContentPart, 0, len(message.ModelContent))
	for _, part := range message.ModelContent {
		if part.Type == "image" || part.Type == "file" {
			if strings.TrimSpace(part.DataBase64) != "" || convcontext.ModelContentPartHasAdmittedMedia(part) {
				stubs = append(stubs, modelContentMediaStub(part))
			}
			part.DataBase64 = ""
			kept = append(kept, part)
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		message.ModelContent = nil
	} else {
		message.ModelContent = kept
	}
	appendMediaStubs(message, stubs)
}

func stubMessageMedia(message *convcontext.Message) {
	if message == nil {
		return
	}
	stubs := make([]string, 0)
	for i := range message.Attachments {
		attachment := &message.Attachments[i]
		if convcontext.MessageAttachmentHasAdmittedMedia(*attachment) || strings.TrimSpace(attachment.DataBase64) != "" {
			stubs = append(stubs, attachmentMediaStub(*attachment))
		}
		attachment.DataBase64 = ""
	}
	for i := range message.ModelContent {
		part := &message.ModelContent[i]
		if convcontext.ModelContentPartHasAdmittedMedia(*part) || strings.TrimSpace(part.DataBase64) != "" {
			stubs = append(stubs, modelContentMediaStub(*part))
		}
		part.DataBase64 = ""
	}
	appendMediaStubs(message, stubs)
}

func appendMediaStubs(message *convcontext.Message, stubs []string) {
	if message == nil || len(stubs) == 0 {
		return
	}
	note := strings.Join(stubs, "\n")
	if message.Role == convcontext.RoleUser {
		if strings.Contains(message.ProviderProjectionText, mediaStubPrefix) {
			return
		}
		if strings.TrimSpace(message.ProviderProjectionText) == "" {
			message.ProviderProjectionText = note
			return
		}
		message.ProviderProjectionText += "\n" + note
		return
	}
	if strings.Contains(message.Content, mediaStubPrefix) {
		return
	}
	if strings.TrimSpace(message.Content) == "" {
		message.Content = note
		return
	}
	message.Content += "\n" + note
}

func isAttachmentSendable(attachment *convcontext.MessageAttachment, capabilities tool.ModelCapabilities) bool {
	if attachment == nil || !strings.EqualFold(strings.TrimSpace(attachment.Kind), "image") {
		return false
	}
	return capabilities.SupportsVision
}

func isModelPartSendable(part tool.ModelContentPart, capabilities tool.ModelCapabilities) bool {
	switch part.Type {
	case "image":
		return capabilities.SupportsVision
	case "file":
		if strings.EqualFold(strings.TrimSpace(part.MimeType), "application/pdf") {
			return capabilities.SupportsNativePDF
		}
		return false
	default:
		return false
	}
}

func attachmentMediaStub(attachment convcontext.MessageAttachment) string {
	label := firstNonEmptyCompactValue(attachment.DisplayName, attachment.Path, attachment.ID, "unnamed image")
	return fmt.Sprintf("%s type=image name=%s (payload omitted; re-Read the file if you need the visual again)", mediaStubPrefix, label)
}

func modelContentMediaStub(part tool.ModelContentPart) string {
	label := firstNonEmptyCompactValue(part.DisplayName, part.MimeType, part.Type)
	stub := fmt.Sprintf("%s type=%s name=%s", mediaStubPrefix, firstNonEmptyCompactValue(part.Type, "media"), label)
	if part.Page > 0 {
		stub += fmt.Sprintf(" page=%d", part.Page)
	}
	stub += " (payload omitted; re-Read the file if you need the visual again)"
	return stub
}

func clearAttachmentMediaAdmission(attachment *convcontext.MessageAttachment) {
	if attachment == nil {
		return
	}
	attachment.DataBase64 = ""
	attachment.StorageKey = ""
	attachment.ByteSize = 0
}

func clearModelContentMediaAdmission(part *tool.ModelContentPart) {
	if part == nil {
		return
	}
	part.DataBase64 = ""
	part.StorageKey = ""
	part.ByteSize = 0
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
