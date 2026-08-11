package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/llm"
)

const (
	threadTitleGenerationTimeout        = 8 * time.Second
	threadTitleForcedReasoningTimeout   = 45 * time.Second
	threadTitleMaxTokens                = 48
	threadTitleForcedReasoningMaxTokens = 512
	threadTitleMaxRuneLength            = 24
)

func buildThreadTitlePrompt(firstUserMessage string, attachments []RunInputAttachment, composerTokens []ComposerToken, modelProfile *ModelProfileConfig) string {
	var b strings.Builder
	b.WriteString("You are a concise chat-session title generator.\n\n")
	b.WriteString("Task: Generate a very short title summarizing the user's first message. Name the underlying topic or task; do not copy the user's sentence.\n")
	b.WriteString("- If the main language is Chinese, output 4-12 Chinese characters.\n")
	b.WriteString("- Otherwise, output 3-8 English words.\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Output ONLY the title text.\n")
	b.WriteString("- No quotation marks, no periods, no newlines, no explanations.\n")
	b.WriteString("- Keep it concise and descriptive.\n")
	b.WriteString("- Do not output a question.\n\n")
	b.WriteString("Examples:\n")
	b.WriteString("User message: 告诉我，这个项目有什么文档？主要是什么方向的？内容都是什么\n")
	b.WriteString("Title: 项目文档概览\n")
	b.WriteString("User message: 帮我分析这批订单\n")
	b.WriteString("Title: 订单分析\n\n")

	if modelProfile != nil && strings.TrimSpace(modelProfile.DisplayName) != "" {
		b.WriteString("Model: ")
		b.WriteString(strings.TrimSpace(modelProfile.DisplayName))
		b.WriteString("\n\n")
	} else if modelProfile != nil && strings.TrimSpace(modelProfile.ModelID) != "" {
		b.WriteString("Model: ")
		b.WriteString(strings.TrimSpace(modelProfile.ModelID))
		b.WriteString("\n\n")
	}

	b.WriteString("User message:\n")
	b.WriteString(strings.TrimSpace(firstUserMessage))
	b.WriteString("\n")

	if len(attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for _, attachment := range attachments {
			name := strings.TrimSpace(attachment.DisplayName)
			if name == "" {
				name = strings.TrimSpace(attachment.Path)
			}
			if name == "" {
				name = string(attachment.Kind)
			}
			b.WriteString("- ")
			b.WriteString(name)
			b.WriteString("\n")
		}
	}

	if tokens := skillComposerTokens(composerTokens); len(tokens) > 0 {
		b.WriteString("\nSelected skills:\n")
		for _, token := range tokens {
			label := strings.TrimSpace(token.Label)
			if label == "" {
				label = strings.TrimSpace(token.ID)
			}
			if label == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(label)
			b.WriteString("\n")
		}
	}

	return b.String()
}

func cleanGeneratedThreadTitle(title string) string {
	title = strings.TrimSpace(title)
	// Strip common wrappers/explanations on a single line.
	title = strings.TrimPrefix(title, "Title: ")
	title = strings.TrimPrefix(title, "标题：")
	title = strings.TrimPrefix(title, "标题:")
	title = strings.TrimSpace(title)

	// Remove surrounding quotes.
	for {
		stripped := strings.Trim(title, `"`)
		stripped = strings.Trim(stripped, "'")
		stripped = strings.Trim(stripped, "`")
		stripped = strings.Trim(stripped, "「」")
		stripped = strings.Trim(stripped, "『』")
		if stripped == title {
			break
		}
		title = stripped
	}

	// Remove periods, newlines, and explanations after delimiters.
	for _, sep := range []string{"\n", ".", "。", "!", "！", "?", "？", ";", "；", ":", "："} {
		if idx := strings.Index(title, sep); idx >= 0 {
			title = title[:idx]
		}
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	return trimThreadLabel(title)
}

func validGeneratedThreadTitle(title, firstUserMessage string) string {
	title = cleanGeneratedThreadTitle(title)
	if title == "" {
		return ""
	}
	if len([]rune(title)) > threadTitleMaxRuneLength {
		return ""
	}
	if strings.ContainsAny(title, "?？") {
		return ""
	}
	normalizedTitle := normalizeGeneratedTitleText(title)
	normalizedInput := normalizeGeneratedTitleText(firstUserMessage)
	if normalizedTitle == "" || normalizedInput == "" {
		return ""
	}
	if normalizedTitle == normalizedInput {
		return ""
	}
	if len([]rune(normalizedTitle)) >= 12 && strings.Contains(normalizedInput, normalizedTitle) {
		return ""
	}
	return title
}

func normalizeGeneratedTitleText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if strings.ContainsRune(" \t\r\n\"'`“”‘’「」『』()（）[]【】{}<>《》,，.。!！?？;；:：/\\|-_、", r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// generateThreadTitleWithProfileFn is the indirection used by
// scheduleThreadTitleGeneration so tests can mock the LLM call without
// issuing real network requests.
var generateThreadTitleWithProfileFn = generateThreadTitleWithProfile

func generateThreadTitleWithProfile(
	ctx context.Context,
	profile *ModelProfileConfig,
	prompt string,
	authority agentcore.ModelCallAuthority,
	retryIndex int,
) (string, error) {
	if profile == nil {
		return "", ErrMissingModelProfile
	}

	mainCfg := llm.Config{
		APIKey:              profile.APIKey,
		BaseURL:             profile.BaseURL,
		InferenceEndpoint:   profile.InferenceEndpoint,
		Model:               profile.ModelID,
		Protocol:            llm.Protocol(profile.Protocol),
		ProviderID:          profile.ProviderID,
		ProfileID:           profile.ProfileID,
		CredentialRevision:  profile.CredentialRevision,
		Temperature:         profile.Temperature,
		MaxTokens:           profile.MaxTokens,
		AuthScheme:          profile.AuthScheme,
		ContextWindow:       profile.ContextWindow,
		SupportsPromptCache: profile.SupportsPromptCache,
	}
	cfg := mainCfg

	resp, err := agentcore.InvokeRecordedModel(ctx, authority, cfg, llm.Request{
		SystemPrompt:     "You generate very short chat session titles. Output only the title text. No quotes, periods, newlines, or explanations.",
		UserPrompt:       prompt,
		Model:            cfg.Model,
		MaxTokens:        threadTitleMaxTokensForProfile(profile),
		DisableReasoning: true,
		Purpose:          llm.RequestPurposeThreadTitle,
		AffinityScopeID:  authority.ThreadID,
		Messages: []convcontext.Message{
			{Role: convcontext.RoleUser, Content: prompt},
		},
	}, retryIndex, false)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", llm.ErrEmptyResponse
	}
	return cleanGeneratedThreadTitle(resp.Content), nil
}

func threadTitleMaxTokensForProfile(profile *ModelProfileConfig) int {
	if profile != nil && strings.TrimSpace(profile.ReasoningEffort) != "" {
		if profile.MaxTokens > 0 && profile.MaxTokens < threadTitleForcedReasoningMaxTokens {
			return profile.MaxTokens
		}
		return threadTitleForcedReasoningMaxTokens
	}
	return threadTitleMaxTokens
}

func threadTitleGenerationTimeoutForProfile(profile *ModelProfileConfig) time.Duration {
	if profile != nil && strings.TrimSpace(profile.ReasoningEffort) != "" {
		return threadTitleForcedReasoningTimeout
	}
	return threadTitleGenerationTimeout
}

func localThreadTitle(firstUserMessage string, attachments []RunInputAttachment, composerTokens []ComposerToken) string {
	if tokens := skillComposerTokens(composerTokens); len(tokens) > 0 {
		for _, token := range tokens {
			if title := trimThreadLabel(firstNonEmpty(token.Label, token.ID)); title != "" {
				return title
			}
		}
	}
	for _, attachment := range attachments {
		name := strings.TrimSpace(firstNonEmpty(attachment.DisplayName, filepath.Base(attachment.Path)))
		if name != "" && name != "." {
			return trimThreadLabel(name)
		}
	}

	value := strings.TrimSpace(firstUserMessage)
	if value == "" || strings.ContainsAny(value, "\r\n?？") {
		return ""
	}
	hasHan := false
	hanCount := 0
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			hasHan = true
			hanCount++
		}
	}
	if hasHan {
		if hanCount <= 12 && len([]rune(value)) <= 16 {
			return trimThreadLabel(value)
		}
		return ""
	}
	if words := strings.Fields(value); len(words) > 0 && len(words) <= 8 {
		return trimThreadLabel(value)
	}
	return ""
}
