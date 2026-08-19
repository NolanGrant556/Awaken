package agentcore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/tool"
	"awaken/src/tools/read"
)

const (
	ErrorCodeContextAdmissionExhausted = "context_admission_exhausted"
	defaultOmittedPDFAdmissionPages    = 10
	// mediaAdmissionTokensPerPart matches the session-level image/PDF page
	// estimate in src/context. Admission uses the same constant so a later
	// hydrate/projection change cannot silently reopen the 19.5K→306K jump.
	mediaAdmissionTokensPerPart = 2000
)

type readAdmissionArgs struct {
	FilePath string `json:"file_path"`
	Pages    string `json:"pages"`
}

func (r *Runner) admitToolCall(call convcontext.ToolCall, executed []ToolExecutionResult) (ToolExecutionResult, bool) {
	if batchHasAdmissionExhaustion(executed) {
		return syntheticAdmissionExhaustedResult(call, 0, estimateReadMediaAdmissionTokens(call.Function.Arguments)), true
	}
	estimated := estimateReadMediaAdmissionTokens(call.Function.Arguments)
	if estimated <= 0 {
		return ToolExecutionResult{}, false
	}
	remaining := r.admissionRemainingTokens(executed)
	if estimated <= remaining {
		return ToolExecutionResult{}, false
	}
	return syntheticAdmissionExhaustedResult(call, remaining, estimated), true
}

func batchHasAdmissionExhaustion(executed []ToolExecutionResult) bool {
	for _, result := range executed {
		if strings.TrimSpace(result.ErrorCode) == ErrorCodeContextAdmissionExhausted {
			return true
		}
		if strings.TrimSpace(result.Outcome.ErrorCode) == ErrorCodeContextAdmissionExhausted {
			return true
		}
	}
	return false
}

func (r *Runner) admissionRemainingTokens(executed []ToolExecutionResult) int {
	if r == nil {
		return 0
	}
	llmReq, err := r.buildLLMRequest()
	if err != nil {
		return 0
	}
	if r.contextGovernor != nil {
		llmReq = r.contextGovernor.evaluateRequest(llmReq)
	} else {
		llmReq.ContextWindowState = NewContextWindowStateWithConfig(
			contextEnvelopeOrEmpty(llmReq),
			contextWindowConfigFromModelConfig(r.config.ModelConfig),
		)
	}
	projected := llmReq.ContextWindowState.ConservativeProjectedTokens()
	for _, result := range executed {
		projected += estimateExecutedToolResultTokens(result)
	}
	remaining := llmReq.ContextWindowState.blockingThreshold() - projected
	if remaining < 0 {
		return 0
	}
	return remaining
}

func contextEnvelopeOrEmpty(llmReq LLMRequest) ContextEnvelope {
	if llmReq.EnvelopeDebug != nil {
		return *llmReq.EnvelopeDebug
	}
	return ContextEnvelope{Messages: llmReq.Messages, ToolSchemas: llmReq.Tools}
}

func estimateExecutedToolResultTokens(result ToolExecutionResult) int {
	content := result.Content
	if result.Outcome.Status != "" {
		if payload := strings.TrimSpace(result.Outcome.ModelPayload()); payload != "" {
			content = payload
		} else if strings.TrimSpace(result.Outcome.Content) != "" {
			content = result.Outcome.Content
		}
	}
	return estimateMessageTokens(convcontext.Message{
		Role:         convcontext.RoleTool,
		Content:      content,
		Name:         result.Name,
		ToolCallID:   result.ToolCallID,
		ModelContent: tool.CloneModelContent(result.Outcome.ModelContent),
	})
}

func estimateReadMediaAdmissionTokens(arguments string) int {
	var parsed readAdmissionArgs
	if err := json.Unmarshal([]byte(arguments), &parsed); err != nil {
		return 0
	}
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(parsed.FilePath))) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return mediaAdmissionTokensPerPart
	case ".pdf":
		pages := defaultOmittedPDFAdmissionPages
		if spec := strings.TrimSpace(parsed.Pages); spec != "" {
			if counted, err := countAdmissionPDFPages(spec); err == nil && counted > 0 {
				pages = counted
			}
		}
		return pages * mediaAdmissionTokensPerPart
	default:
		return 0
	}
}

func countAdmissionPDFPages(spec string) (int, error) {
	seen := make(map[int]struct{})
	for _, token := range strings.Split(spec, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			return 0, fmt.Errorf("empty page token")
		}
		bounds := strings.Split(token, "-")
		if len(bounds) > 2 {
			return 0, fmt.Errorf("invalid page range %q", token)
		}
		first, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
		if err != nil || first < 1 {
			return 0, fmt.Errorf("invalid page %q", token)
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || last < first {
				return 0, fmt.Errorf("invalid page range %q", token)
			}
		}
		for page := first; page <= last; page++ {
			seen[page] = struct{}{}
		}
	}
	return len(seen), nil
}

func syntheticAdmissionExhaustedResult(call convcontext.ToolCall, remaining, estimated int) ToolExecutionResult {
	now := time.Now().UTC()
	message := fmt.Sprintf(
		"上下文剩余预算不足，无法再把这份媒体读进当前窗口（剩余约 %d token，本次 Read 估约 %d token）。请减少页数或条数，或等下一轮压缩后再读。",
		remaining,
		estimated,
	)
	if remaining <= 0 && estimated <= 0 {
		message = "上下文剩余预算不足，同批后续工具已停止执行。请减少页数或条数，或等下一轮压缩后再读。"
	}
	outcome := tool.FailedOutcome(message, ErrorCodeContextAdmissionExhausted, tool.ErrorCategoryPolicy, false)
	outcome.Details = map[string]any{
		"remaining_tokens": remaining,
		"estimated_tokens": estimated,
		"tool":             firstNonEmpty(call.Function.Name, read.ToolName),
	}
	outcome.NextActions = []tool.NextAction{
		{ID: "reduce_read_pages_or_count", Description: "Request fewer PDF pages or fewer media Read calls."},
		{ID: "retry_after_compact", Description: "Wait for the next compact boundary before reading more media."},
	}
	return ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		Content:    message,
		IsError:    true,
		Status:     ToolExecutionDenied,
		ErrorCode:  ErrorCodeContextAdmissionExhausted,
		Outcome:    outcome,
		StartedAt:  now,
		FinishedAt: now,
	}
}
