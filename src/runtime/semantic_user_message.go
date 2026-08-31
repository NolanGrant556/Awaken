package runtime

import (
	"encoding/json"
	"strings"

	"awaken/src/agentcore"
)

func semanticComposerTokensForRun(tokens []ComposerToken) []agentcore.SemanticComposerToken {
	if len(tokens) == 0 {
		return nil
	}
	converted := make([]agentcore.SemanticComposerToken, 0, len(tokens))
	for _, token := range tokens {
		payload := map[string]any{}
		if value := strings.TrimSpace(token.Payload.CapabilityID); value != "" {
			payload["capabilityId"] = value
		}
		if value := strings.TrimSpace(token.Payload.SkillID); value != "" {
			payload["skillId"] = value
		}
		if value := strings.TrimSpace(token.Payload.PluginName); value != "" {
			payload["pluginName"] = value
		}
		if value := strings.TrimSpace(token.Payload.PluginDigest); value != "" {
			payload["pluginDigest"] = value
		}
		if value := strings.TrimSpace(token.Payload.RuleLogicalID); value != "" {
			payload["ruleLogicalId"] = value
		}
		if value := strings.TrimSpace(token.Payload.CommandLogicalID); value != "" {
			payload["commandLogicalId"] = value
		}
		if value := strings.TrimSpace(token.Payload.AgentLogicalID); value != "" {
			payload["agentLogicalId"] = value
		}
		if value := strings.TrimSpace(token.Payload.CommandArgumentsJSON); value != "" {
			payload["commandArguments"] = value
		}
		if value := strings.TrimSpace(token.Payload.SourceOrigin); value != "" {
			payload["sourceOrigin"] = value
		}
		if value := strings.TrimSpace(token.Payload.SkillFilePath); value != "" {
			payload["skillFilePath"] = value
		}
		if value := strings.TrimSpace(token.Payload.RootDir); value != "" {
			payload["rootDir"] = value
		}
		if value := strings.TrimSpace(token.Payload.Path); value != "" {
			payload["path"] = value
		}
		if value := strings.TrimSpace(token.Payload.EntryKind); value != "" {
			payload["entryKind"] = value
		}
		if value := strings.TrimSpace(token.Payload.ProjectID); value != "" {
			payload["projectId"] = value
		}
		if value := strings.TrimSpace(token.Payload.FolderPath); value != "" {
			payload["folderPath"] = value
		}
		if value := strings.TrimSpace(token.Payload.RelativePath); value != "" {
			payload["relativePath"] = value
		}
		if value := strings.TrimSpace(token.Payload.WorkflowID); value != "" {
			payload["workflowId"] = value
		}
		if len(payload) == 0 {
			payload = nil
		}
		converted = append(converted, agentcore.SemanticComposerToken{
			ID:          token.ID,
			Type:        token.Type,
			LeadingText: token.LeadingText,
			Label:       token.Label,
			Payload:     payload,
		})
	}
	return converted
}

func semanticFileLineCommentsForRun(comments []FileLineComment) []agentcore.SemanticFileLineComment {
	if len(comments) == 0 {
		return nil
	}
	converted := make([]agentcore.SemanticFileLineComment, 0, len(comments))
	for _, comment := range comments {
		converted = append(converted, agentcore.SemanticFileLineComment{
			ID: comment.ID, ProjectID: comment.ProjectID, FolderPath: comment.FolderPath,
			RelativePath: comment.RelativePath, DisplayName: comment.DisplayName,
			ContentDigest: comment.ContentDigest, StartLine: comment.StartLine,
			EndLine: comment.EndLine, SelectedText: comment.SelectedText,
			Comment: comment.Comment, CreatedAt: comment.CreatedAt,
		})
	}
	return converted
}

func transcriptAttachmentsFromSemanticUserMessage(payload map[string]any) []transcriptMessageAttachment {
	var attachments []transcriptMessageAttachment
	if !decodeSemanticUserMessageField(payload, "attachments", &attachments) {
		return nil
	}
	valid := attachments[:0]
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.ID) == "" {
			continue
		}
		switch attachment.Kind {
		case RunInputAttachmentKindImage, RunInputAttachmentKindFile:
			valid = append(valid, attachment)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return valid
}

func composerTokensFromSemanticUserMessage(payload map[string]any) []ComposerToken {
	var tokens []ComposerToken
	if !decodeSemanticUserMessageField(payload, "composer_tokens", &tokens) {
		return nil
	}
	valid := tokens[:0]
	for _, token := range tokens {
		if strings.TrimSpace(token.ID) == "" || strings.TrimSpace(token.Type) == "" {
			continue
		}
		valid = append(valid, token)
	}
	if len(valid) == 0 {
		return nil
	}
	return cloneComposerTokens(valid)
}

func fileLineCommentsFromSemanticUserMessage(payload map[string]any) []FileLineComment {
	var comments []FileLineComment
	if !decodeSemanticUserMessageField(payload, "file_comments", &comments) {
		return nil
	}
	valid := comments[:0]
	for _, comment := range comments {
		if strings.TrimSpace(comment.ID) == "" || strings.TrimSpace(comment.RelativePath) == "" ||
			comment.StartLine < 1 || comment.EndLine < comment.StartLine {
			continue
		}
		valid = append(valid, comment)
	}
	return cloneFileLineComments(valid)
}

func decodeSemanticUserMessageField(payload map[string]any, key string, target any) bool {
	if payload == nil {
		return false
	}
	raw, ok := payload[key]
	if !ok || raw == nil {
		return false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		return false
	}
	return true
}
