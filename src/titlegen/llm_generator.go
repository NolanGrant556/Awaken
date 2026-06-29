package titlegen

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"
	"time"

	"awaken/src/llm"
)

const (
	// defaultLLMTitleMaxTokens is kept low so a one-shot title call stays
	// cheap, but large enough that reasoning models (which emit "thinking"
	// blocks before the title text) still have room to produce output. The
	// title itself is only a few words; the headroom is for reasoning tokens
	// that the transport discards.
	defaultLLMTitleMaxTokens = 512
	defaultLLMTitleTimeout   = 30 * time.Second
)

// LLMGeneratorOptions configures an LLM-backed title generator.
type LLMGeneratorOptions struct {
	Config    llm.Config
	MaxTokens int
	Timeout   time.Duration
}

// LLMGenerator implements Generator by issuing a one-shot LLM completion
// through the src/llm provider transport.
type LLMGenerator struct {
	config    llm.Config
	maxTokens int
	timeout   time.Duration
}

// NewLLMGenerator builds a Generator that calls the LLM via llm.Call with the
// given system+user prompt. Returns nil when opts.Config.APIKey is empty so
// the runtime can fall back to promptTitleGenerator via normalizeTitleGenerator.
func NewLLMGenerator(opts LLMGeneratorOptions) Generator {
	if strings.TrimSpace(opts.Config.APIKey) == "" {
		return nil
	}
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultLLMTitleMaxTokens
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultLLMTitleTimeout
	}
	return &LLMGenerator{
		config:    opts.Config,
		maxTokens: maxTokens,
		timeout:   timeout,
	}
}

// GenerateTitle calls the LLM with prompt.SystemPrompt + prompt.UserPrompt and
// returns the raw text. titlegen.MaybeGenerate + CleanTitle sanitize the
// result downstream. Any error (network, auth, empty response) is returned so
// MaybeGenerate records Generated=false and the runtime falls back gracefully.
func (g *LLMGenerator) GenerateTitle(ctx stdctx.Context, prompt Prompt) (string, error) {
	if g == nil {
		return "", errors.New("llm title generator is nil")
	}

	callCtx, cancel := stdctx.WithTimeout(ctx, g.timeout)
	defer cancel()

	req := llm.Request{
		SystemPrompt: prompt.SystemPrompt,
		UserPrompt:   prompt.UserPrompt,
		MaxTokens:    g.maxTokens,
	}

	content, err := llm.Call(callCtx, g.config, req)
	if err != nil {
		return "", fmt.Errorf("llm title generation failed: %w", err)
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("llm returned empty title")
	}
	return content, nil
}
