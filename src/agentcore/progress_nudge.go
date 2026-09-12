package agentcore

import (
	"encoding/json"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// ProgressNudgePolicy is host configuration, never a model tool or user gate.
// Nil selects the production defaults; tests can inject a clock.
type ProgressNudgePolicy struct {
	Enabled       bool
	QuietWindow   time.Duration
	ToolThreshold int
	Now           func() time.Time
}

const progressNudgeText = "<awaken_runtime_notice>若用户没有要求静默或限定输出格式，请用 1–3 句简短说明已确认的发现与下一步；不要复述工具日志、披露内部推理或编造进度。仍需工具时将说明与下一批工具调用放在同一响应；任务已完成则直接给出最终结果。不要为了汇报增加工具调用。本提醒不改变用户约束、权限或任务范围。</awaken_runtime_notice>"

type progressNudge struct {
	policy      ProgressNudgePolicy
	lastVisible time.Time
	lastNudge   time.Time
	seen        map[string]bool
	calls       int
	notice      string
}

func newProgressNudge(config AgentConfig) *progressNudge {
	policy := ProgressNudgePolicy{Enabled: true, QuietWindow: time.Minute, ToolThreshold: 8, Now: time.Now}
	if config.ProgressNudgePolicy != nil {
		policy = *config.ProgressNudgePolicy
	}
	if policy.Now == nil {
		policy.Now = time.Now
	}
	if policy.QuietWindow <= 0 {
		policy.QuietWindow = time.Minute
	}
	if policy.ToolThreshold <= 0 {
		policy.ToolThreshold = 8
	}
	if config.SubagentOrigin != nil || config.SubagentTaskID != "" || config.SubagentBoundary != nil {
		policy.Enabled = false
	}
	return &progressNudge{policy: policy, lastVisible: policy.Now(), seen: map[string]bool{}}
}

func (p *progressNudge) visible(content string) {
	if p == nil || strings.TrimSpace(content) == "" {
		return
	}
	p.lastVisible = p.policy.Now()
	p.calls = 0
}

func (p *progressNudge) observe(results []ToolExecutionResult, calls ...convcontext.ToolCall) {
	if p == nil {
		return
	}
	polling := map[string]bool{}
	for _, call := range calls {
		if call.Function.Name != "subagent" {
			continue
		}
		var input struct {
			Action string `json:"action"`
		}
		if json.Unmarshal([]byte(call.Function.Arguments), &input) == nil {
			polling[call.ID] = input.Action == "wait" || input.Action == "status" || input.Action == "models"
		}
	}
	for _, result := range results {
		if result.ToolCallID == "" || p.seen[result.ToolCallID] {
			continue
		}
		p.seen[result.ToolCallID] = true
		if result.Outcome.TransportStatus != tool.TransportCompleted || polling[result.ToolCallID] {
			continue
		}
		switch result.Name {
		case "ask_user", "subagent_wait", "subagent_status", "wait", "task_status", "task_output":
			continue
		}
		if strings.Contains(result.Name, "_status") || strings.Contains(result.Name, "_poll") {
			continue
		}
		p.calls++
	}
}

// Called once at a natural turn boundary, not for transport retry/compaction.
func (p *progressNudge) prepare(transition TransitionReason) {
	if p == nil {
		return
	}
	p.notice = ""
	if !p.policy.Enabled || transition != TransitionToolResultContinuation || p.calls < p.policy.ToolThreshold {
		return
	}
	now := p.policy.Now()
	if now.Sub(p.lastVisible) < p.policy.QuietWindow || (!p.lastNudge.IsZero() && now.Sub(p.lastNudge) < p.policy.QuietWindow) {
		return
	}
	p.notice = progressNudgeText
	p.lastNudge = now
	p.calls = 0
}

func (p *progressNudge) project(req *LLMRequest) {
	if p == nil || p.notice == "" {
		return
	}
	// The paired tool result carries a request-only host annotation. Conversation
	// messages and raw tool results are untouched; no synthetic user is created.
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == convcontext.RoleTool {
			existing := req.Messages[index].ProviderProjectionText
			if !strings.Contains(existing, progressNudgeText) {
				req.Messages[index].ProviderProjectionText = strings.TrimSpace(existing + "\n\n" + p.notice)
			}
			return
		}
	}
}
