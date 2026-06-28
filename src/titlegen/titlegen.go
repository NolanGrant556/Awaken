package titlegen

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	convcontext "awaken/src/context"
)

const (
	DefaultMaxSnippetRunes = 500
	DefaultMaxTitleRunes   = 80
)

const systemPrompt = "Generate a short, descriptive title for a conversation that starts with the following exchange. Use 3-7 words for English, or a short Chinese title when Chinese is more natural. Return ONLY the title text. Do not include quotes, prefixes, explanations, or punctuation at the end."

type Config struct {
	MaxSnippetRunes int
	MaxTitleRunes   int
}

type Prompt struct {
	SystemPrompt string
	UserPrompt   string
}

type Generator interface {
	GenerateTitle(ctx stdctx.Context, prompt Prompt) (string, error)
}

type GeneratorFunc func(ctx stdctx.Context, prompt Prompt) (string, error)

func (f GeneratorFunc) GenerateTitle(ctx stdctx.Context, prompt Prompt) (string, error) {
	return f(ctx, prompt)
}

type AutoTitleRequest struct {
	SessionID     string
	ExistingTitle string
	Messages      []convcontext.Message
}

type MaybeGenerateRequest struct {
	SessionID     string
	ExistingTitle string
	Messages      []convcontext.Message
	Generator     Generator
	Config        Config
	OnFailure     FailureCallback
	Metadata      map[string]string
}

type MaybeGenerateResult struct {
	Title      string
	Generated  bool
	Skipped    bool
	SkipReason SkipReason
	Failure    *Failure
}

type SkipReason string

const (
	SkipMissingSessionID SkipReason = "missing_session_id"
	SkipExistingTitle    SkipReason = "existing_title"
	SkipLateConversation SkipReason = "late_conversation"
	SkipMissingExchange  SkipReason = "missing_exchange"
)

type FailureReason string

const (
	FailureMissingGenerator FailureReason = "missing_generator"
	FailureLLMError         FailureReason = "llm_error"
	FailureEmptyTitle       FailureReason = "empty_title"
)

type FailureCallback func(Failure)

type Failure struct {
	SessionID     string
	Reason        FailureReason
	Err           error
	UserTurnCount int
	RawTitle      string
	Metadata      map[string]string
}

func BuildPrompt(userMessage, assistantResponse string, cfg Config) Prompt {
	cfg = normalizeConfig(cfg)
	userSnippet := truncateRunes(strings.TrimSpace(userMessage), cfg.MaxSnippetRunes)
	assistantSnippet := truncateRunes(strings.TrimSpace(assistantResponse), cfg.MaxSnippetRunes)

	return Prompt{
		SystemPrompt: systemPrompt,
		UserPrompt:   fmt.Sprintf("User: %s\n\nAssistant: %s", userSnippet, assistantSnippet),
	}
}

func BuildPromptFromMessages(messages []convcontext.Message, cfg Config) (Prompt, bool) {
	userMessage, assistantResponse, ok := firstExchange(messages)
	if !ok {
		return Prompt{}, false
	}
	return BuildPrompt(userMessage, assistantResponse, cfg), true
}

func CleanTitle(raw string, maxRunes ...int) string {
	limit := DefaultMaxTitleRunes
	if len(maxRunes) > 0 && maxRunes[0] > 0 {
		limit = maxRunes[0]
	}

	title := trimOuterQuotes(strings.TrimSpace(raw))
	title = trimTitlePrefix(title)
	title = trimOuterQuotes(strings.TrimSpace(title))
	title = strings.TrimRight(title, trailingPunctuation)
	title = trimOuterQuotes(strings.TrimSpace(title))
	title = strings.Join(strings.Fields(title), " ")

	if runeCount(title) > limit {
		title = string([]rune(title)[:limit])
		title = strings.TrimRight(strings.TrimSpace(title), trailingPunctuation)
	}

	return strings.TrimSpace(title)
}

func ShouldAutoTitle(req AutoTitleRequest) bool {
	if skipReason(req) != "" {
		return false
	}
	return true
}

func MaybeGenerate(ctx stdctx.Context, req MaybeGenerateRequest) MaybeGenerateResult {
	autoReq := AutoTitleRequest{
		SessionID:     req.SessionID,
		ExistingTitle: req.ExistingTitle,
		Messages:      req.Messages,
	}
	if reason := skipReason(autoReq); reason != "" {
		return MaybeGenerateResult{Skipped: true, SkipReason: reason}
	}

	if req.Generator == nil {
		failure := req.failure(FailureMissingGenerator, errors.New("title generator is nil"), "")
		req.reportFailure(failure)
		return MaybeGenerateResult{Failure: &failure}
	}

	prompt, ok := BuildPromptFromMessages(req.Messages, req.Config)
	if !ok {
		return MaybeGenerateResult{Skipped: true, SkipReason: SkipMissingExchange}
	}

	rawTitle, err := req.Generator.GenerateTitle(ctx, prompt)
	if err != nil {
		failure := req.failure(FailureLLMError, err, rawTitle)
		req.reportFailure(failure)
		return MaybeGenerateResult{Failure: &failure}
	}

	title := CleanTitle(rawTitle, normalizeConfig(req.Config).MaxTitleRunes)
	if title == "" {
		failure := req.failure(FailureEmptyTitle, nil, rawTitle)
		req.reportFailure(failure)
		return MaybeGenerateResult{Failure: &failure}
	}

	return MaybeGenerateResult{Title: title, Generated: true}
}

func skipReason(req AutoTitleRequest) SkipReason {
	if strings.TrimSpace(req.SessionID) == "" {
		return SkipMissingSessionID
	}
	if strings.TrimSpace(req.ExistingTitle) != "" {
		return SkipExistingTitle
	}
	if userTurnCount(req.Messages) > 2 {
		return SkipLateConversation
	}
	if _, _, ok := firstExchange(req.Messages); !ok {
		return SkipMissingExchange
	}
	return ""
}

func (req MaybeGenerateRequest) failure(reason FailureReason, err error, rawTitle string) Failure {
	return Failure{
		SessionID:     req.SessionID,
		Reason:        reason,
		Err:           err,
		UserTurnCount: userTurnCount(req.Messages),
		RawTitle:      rawTitle,
		Metadata:      cloneMetadata(req.Metadata),
	}
}

func (req MaybeGenerateRequest) reportFailure(failure Failure) {
	if req.OnFailure != nil {
		req.OnFailure(failure)
	}
}

func firstExchange(messages []convcontext.Message) (string, string, bool) {
	userMessage := ""
	for _, message := range messages {
		switch message.Role {
		case convcontext.RoleUser:
			if strings.TrimSpace(message.Content) != "" && userMessage == "" {
				userMessage = message.Content
			}
		case convcontext.RoleAssistant:
			if userMessage != "" && strings.TrimSpace(message.Content) != "" {
				return userMessage, message.Content, true
			}
		}
	}
	return "", "", false
}

func userTurnCount(messages []convcontext.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == convcontext.RoleUser && strings.TrimSpace(message.Content) != "" {
			count++
		}
	}
	return count
}

func normalizeConfig(cfg Config) Config {
	if cfg.MaxSnippetRunes <= 0 {
		cfg.MaxSnippetRunes = DefaultMaxSnippetRunes
	}
	if cfg.MaxTitleRunes <= 0 {
		cfg.MaxTitleRunes = DefaultMaxTitleRunes
	}
	return cfg
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || runeCount(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit]) + "..."
}

func runeCount(value string) int {
	return utf8.RuneCountInString(value)
}

func trimTitlePrefix(title string) string {
	lower := strings.ToLower(title)
	for _, prefix := range []string{"title:", "title：", "标题:", "标题："} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(title[len(prefix):])
		}
	}
	return title
}

func trimOuterQuotes(title string) string {
	return strings.Trim(title, " \t\r\n\"'`“”‘’「」『』《》")
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

const trailingPunctuation = ".。:：;；!！?？,，、"
