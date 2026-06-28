package promptsuggest

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	MinAssistantTurns       = 2
	MaxSuggestionLength     = 100
	ReasonDisabled          = Reason("disabled")
	ReasonPendingPermission = Reason("pending_permission")
	ReasonElicitationActive = Reason("elicitation_active")
	ReasonPlanMode          = Reason("plan_mode")
	ReasonRateLimited       = Reason("rate_limit")
	ReasonEarlyConversation = Reason("early_conversation")
	ReasonLastResponseError = Reason("last_response_error")
	ReasonAborted           = Reason("aborted")
	ReasonEmpty             = Reason("empty")
	ReasonTooLong           = Reason("too_long")
	ReasonCodeBlock         = Reason("code_block")
	ReasonFilePath          = Reason("file_path")
)

var ErrNoGenerator = errors.New("promptsuggest: generator is nil")

type Reason string

type Conversation struct {
	Messages []Message
}

type Message struct {
	Role    string
	Content string
}

type Suggestion struct {
	Text string
}

type Guard struct {
	Enabled              bool
	PendingPermission    bool
	ElicitationActive    bool
	PlanMode             bool
	RateLimited          bool
	AssistantTurnCount   int
	LastAssistantErrored bool
}

type Input struct {
	Guard        Guard
	Conversation Conversation
}

type Result struct {
	Suggestion Suggestion
	Suppressed Reason
}

type Generator interface {
	Generate(context.Context, Conversation) (Suggestion, error)
}

type Engine struct {
	generator Generator
}

func NewEngine(generator Generator) *Engine {
	return &Engine{generator: generator}
}

func SuppressReason(ctx context.Context, input Input) Reason {
	if ctx != nil && ctx.Err() != nil {
		return ReasonAborted
	}
	return input.Guard.SuppressReason()
}

func (input Input) SuppressReason(ctx context.Context) Reason {
	return SuppressReason(ctx, input)
}

func (guard Guard) SuppressReason() Reason {
	switch {
	case !guard.Enabled:
		return ReasonDisabled
	case guard.PendingPermission:
		return ReasonPendingPermission
	case guard.ElicitationActive:
		return ReasonElicitationActive
	case guard.PlanMode:
		return ReasonPlanMode
	case guard.RateLimited:
		return ReasonRateLimited
	case guard.AssistantTurnCount < MinAssistantTurns:
		return ReasonEarlyConversation
	case guard.LastAssistantErrored:
		return ReasonLastResponseError
	default:
		return ""
	}
}

func (engine *Engine) TryGenerate(ctx context.Context, input Input) (Result, error) {
	if reason := SuppressReason(ctx, input); reason != "" {
		return Result{Suppressed: reason}, nil
	}
	if engine == nil || engine.generator == nil {
		return Result{}, ErrNoGenerator
	}

	suggestion, err := engine.generator.Generate(ctx, input.Conversation)
	if ctx != nil && ctx.Err() != nil {
		return Result{Suppressed: ReasonAborted}, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("generate prompt suggestion: %w", err)
	}

	suggestion.Text = strings.TrimSpace(suggestion.Text)
	if reason := FilterReason(suggestion); reason != "" {
		return Result{Suppressed: reason}, nil
	}
	return Result{Suggestion: suggestion}, nil
}

func FilterReason(suggestion Suggestion) Reason {
	text := strings.TrimSpace(suggestion.Text)
	switch {
	case text == "":
		return ReasonEmpty
	case len(text) >= MaxSuggestionLength:
		return ReasonTooLong
	case looksLikeCodeBlock(text):
		return ReasonCodeBlock
	case looksLikeFilePath(text):
		return ReasonFilePath
	default:
		return ""
	}
}

func looksLikeCodeBlock(text string) bool {
	return strings.HasPrefix(text, "```") || strings.Contains(text, "\n```")
}

func looksLikeFilePath(text string) bool {
	if strings.ContainsAny(text, " \t\r\n") {
		return false
	}

	trimmed := strings.TrimRight(text, ":,.;")
	if strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "~/") {
		return true
	}
	if strings.HasPrefix(trimmed, "/") && strings.Count(trimmed, "/") >= 2 {
		return true
	}
	if len(trimmed) >= 3 && trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/') {
		return true
	}

	if !strings.ContainsAny(trimmed, `/\`) {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, ext := range pathLikeExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

var pathLikeExtensions = []string{
	".c",
	".cc",
	".cpp",
	".css",
	".go",
	".h",
	".hpp",
	".html",
	".java",
	".js",
	".json",
	".jsx",
	".kt",
	".md",
	".py",
	".rs",
	".sh",
	".sql",
	".swift",
	".toml",
	".ts",
	".tsx",
	".yaml",
	".yml",
}
