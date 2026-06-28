package awaysummary

import (
	stdctx "context"
	"errors"
	"strings"

	convcontext "awaken/src/context"
)

const (
	DefaultRecentMessageWindow = 30
	SmallFastModelMarker       = "small-fast-model"
	QuerySourceAwaySummary     = "away_summary"
)

var ErrAborted = errors.New("away summary aborted")

type Options struct {
	RecentMessageWindow int
	SessionMemory       string
}

type Request struct {
	SystemPrompt          string
	Messages              []convcontext.Message
	Tools                 []string
	Model                 string
	QuerySource           string
	ThinkingDisabled      bool
	SkipCacheWrite        bool
	NonInteractiveSession bool
	HasAppendSystemPrompt bool
	ToolChoice            string
	Agents                []string
	MCPTools              []string
}

type Response struct {
	Content    string
	APIError   bool
	UserAbort  bool
	SkipResult bool
}

type Result struct {
	Summary string
}

type Runner func(stdctx.Context, Request) (Response, error)

func BuildRequest(messages []convcontext.Message, opts Options) (Request, bool) {
	if len(messages) == 0 {
		return Request{}, false
	}

	recent := recentMessages(messages, opts.window())
	recent = append(recent, convcontext.Message{
		Role:    convcontext.RoleUser,
		Content: BuildPrompt(opts.SessionMemory),
	})

	return Request{
		SystemPrompt:          "",
		Messages:              recent,
		Tools:                 []string{},
		Model:                 SmallFastModelMarker,
		QuerySource:           QuerySourceAwaySummary,
		ThinkingDisabled:      true,
		SkipCacheWrite:        true,
		NonInteractiveSession: false,
		HasAppendSystemPrompt: false,
		ToolChoice:            "",
		Agents:                []string{},
		MCPTools:              []string{},
	}, true
}

func BuildPrompt(memory string) string {
	memory = strings.TrimSpace(memory)
	memoryBlock := ""
	if memory != "" {
		memoryBlock = "Session memory (broader context):\n" + memory + "\n\n"
	}

	return memoryBlock + "The user stepped away and is coming back. Write exactly 1-3 short sentences. Start by stating the high-level task: what they are building or debugging, not implementation details. Next, give the concrete next step. Skip status reports and commit recaps."
}

func Generate(ctx stdctx.Context, messages []convcontext.Message, opts Options, runner Runner) *Result {
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if ctx.Err() != nil || runner == nil {
		return nil
	}

	req, ok := BuildRequest(messages, opts)
	if !ok {
		return nil
	}

	resp, err := runner(ctx, req)
	if err != nil || resp.APIError || resp.UserAbort || resp.SkipResult || ctx.Err() != nil {
		return nil
	}

	summary := CleanSummary(resp.Content)
	if summary == "" {
		return nil
	}
	return &Result{Summary: summary}
}

func CleanSummary(raw string) string {
	text := strings.Trim(raw, " \t\r\n\"'`")
	if text == "" {
		return ""
	}

	text = strings.NewReplacer("\r\n", " ", "\n", " ", "\t", " ").Replace(text)
	text = collapseWhitespace(text)
	text = trimStatusPrefix(text)
	text = collapseWhitespace(text)

	return strings.Join(firstSentences(text, 3), " ")
}

func recentMessages(messages []convcontext.Message, window int) []convcontext.Message {
	if window <= 0 {
		window = DefaultRecentMessageWindow
	}
	start := len(messages) - window
	if start < 0 {
		start = 0
	}

	recent := make([]convcontext.Message, len(messages[start:]))
	copy(recent, messages[start:])
	return recent
}

func (opts Options) window() int {
	if opts.RecentMessageWindow <= 0 {
		return DefaultRecentMessageWindow
	}
	return opts.RecentMessageWindow
}

func collapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func trimStatusPrefix(text string) string {
	candidates := []string{
		"status update:",
		"current status:",
		"status report:",
		"progress update:",
		"commit/status recap:",
		"status:",
		"update:",
		"recap:",
		"summary:",
		"状态更新：",
		"当前状态：",
		"进度汇报：",
		"状态：",
		"更新：",
		"摘要：",
		"总结：",
	}

	lower := strings.ToLower(text)
	for _, candidate := range candidates {
		if strings.HasPrefix(lower, strings.ToLower(candidate)) {
			return strings.TrimSpace(text[len(candidate):])
		}
	}

	if idx, width := firstColon(text); idx > 0 && idx <= 120 {
		prefix := strings.ToLower(text[:idx])
		if strings.Contains(prefix, "status") ||
			strings.Contains(prefix, "progress") ||
			strings.Contains(prefix, "recap") ||
			strings.Contains(prefix, "summary") ||
			strings.Contains(prefix, "commit") ||
			strings.Contains(prefix, "状态") ||
			strings.Contains(prefix, "进度") ||
			strings.Contains(prefix, "汇报") ||
			strings.Contains(prefix, "摘要") ||
			strings.Contains(prefix, "总结") {
			return strings.TrimSpace(text[idx+width:])
		}
	}

	return text
}

func firstColon(text string) (int, int) {
	ascii := strings.Index(text, ":")
	fullWidth := strings.Index(text, "：")
	switch {
	case ascii == -1:
		return fullWidth, len("：")
	case fullWidth == -1:
		return ascii, len(":")
	case ascii < fullWidth:
		return ascii, len(":")
	default:
		return fullWidth, len("：")
	}
}

func firstSentences(text string, limit int) []string {
	if limit <= 0 || text == "" {
		return nil
	}

	sentences := make([]string, 0, limit)
	start := 0
	for idx, r := range text {
		if !isSentenceEnd(r) {
			continue
		}

		sentence := strings.TrimSpace(text[start : idx+len(string(r))])
		if sentence != "" {
			sentences = append(sentences, sentence)
			if len(sentences) == limit {
				return sentences
			}
		}
		start = idx + len(string(r))
	}

	remainder := strings.TrimSpace(text[start:])
	if remainder != "" && len(sentences) < limit {
		sentences = append(sentences, remainder)
	}
	return sentences
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？':
		return true
	default:
		return false
	}
}
