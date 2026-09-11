package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/researchruntime"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

func hostResearchTeamBlock(intent *researchruntime.Intent, prefetch []researchruntime.PrefetchedSource) (ContextBlock, bool) {
	if intent == nil || strings.TrimSpace(intent.Query) == "" {
		return ContextBlock{}, false
	}
	content := researchruntime.ConductorPrompt(*intent, prefetch)
	return ContextBlock{ID: "research_team", Content: content}, true
}

func (r *Runner) prepareResearchTeam(ctx stdcontext.Context, events *runEventEmitter) error {
	if r == nil || r.config.ResearchIntent == nil || r.config.SubagentOrigin != nil {
		return nil
	}
	if err := r.appendResearchJournal(ctx, events, SemanticKindResearchAccepted, map[string]any{
		"query":       r.config.ResearchIntent.Query,
		"source_hint": string(r.config.ResearchIntent.SourceHint),
		"phase":       "accepted",
	}); err != nil {
		return err
	}
	prefetched, err := r.prefetchResearchSources(ctx, events, *r.config.ResearchIntent)
	if err != nil {
		return err
	}
	r.researchPrefetch = prefetched
	if r.reqBuilder != nil {
		r.reqBuilder.SetResearchPrefetch(prefetched)
	}
	previous := events.afterTerminal
	events.afterTerminal = func(termCtx stdcontext.Context, status string, reason TerminalReason, resultBytes int) {
		r.finishResearchTeam(termCtx, events, status, reason)
		if previous != nil {
			previous(termCtx, status, reason, resultBytes)
		}
	}
	return nil
}

func (r *Runner) finishResearchTeam(ctx stdcontext.Context, events *runEventEmitter, status string, reason TerminalReason) {
	if r == nil || r.config.ResearchIntent == nil || r.config.SubagentOrigin != nil {
		return
	}
	outcome := "finished"
	switch {
	case status == "cancelled" || reason == TerminalReasonCancelled:
		outcome = "cancelled"
	case status == "interrupted":
		outcome = "interrupted"
	case status != "" && status != "finished":
		outcome = "failed"
	}
	_ = r.appendResearchJournal(ctx, events, SemanticKindResearchTerminal, map[string]any{
		"outcome": outcome,
		"phase":   "terminal",
	})
}

func (r *Runner) prefetchResearchSources(ctx stdcontext.Context, events *runEventEmitter, intent researchruntime.Intent) ([]researchruntime.PrefetchedSource, error) {
	if intent.SourceHint == researchruntime.SourceWorkspace {
		return nil, nil
	}
	urls := researchruntime.ExplicitHTTPSURLs(intent.Query)
	if len(urls) == 0 {
		return nil, nil
	}
	prefetcher := &researchSourcePrefetcher{runner: r, events: events}
	results := make([]researchruntime.PrefetchedSource, 0, len(urls))
	for index, rawURL := range urls {
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
		if index >= 4 {
			results = append(results, researchruntime.PrefetchedSource{URL: rawURL, ErrorCode: "explicit_source_limit"})
			continue
		}
		result, err := prefetcher.Fetch(ctx, rawURL)
		if err != nil {
			return results, err
		}
		results = append(results, result)
		if result.Receipt != nil {
			r.noteResearchReceipt(*result.Receipt)
		}
	}
	return results, nil
}

func (r *Runner) noteResearchReceipt(receipt subagentruntime.HostToolReceipt) {
	if r == nil || strings.TrimSpace(receipt.ToolName) == "" {
		return
	}
	r.researchReceipts = append(r.researchReceipts, receipt)
}

func (r *Runner) recordResearchToolReceipt(result ToolExecutionResult) {
	if r == nil || r.config.ResearchIntent == nil || result.IsError {
		return
	}
	if result.Name != "WebFetch" && result.Name != "Read" {
		return
	}
	details := researchReceiptDetails(result.Name, result.Outcome.Details)
	if result.Name == "WebFetch" && strings.TrimSpace(fmt.Sprint(details["content_sha256"])) == "" {
		return
	}
	if result.Name == "Read" && strings.TrimSpace(fmt.Sprint(details["sha256"])) == "" {
		return
	}
	r.noteResearchReceipt(subagentruntime.HostToolReceipt{
		ToolCallID: result.ToolCallID,
		ToolName:   result.Name,
		Details:    details,
	})
}

func (r *Runner) collectResearchReceipts() []subagentruntime.HostToolReceipt {
	out := append([]subagentruntime.HostToolReceipt(nil), r.researchReceipts...)
	if r.subagentController == nil {
		return out
	}
	for _, task := range r.subagentController.List() {
		out = append(out, task.HostToolReceipts...)
	}
	return out
}

func (r *Runner) rejectResearchPublish(call convcontext.ToolCall) *ToolExecutionResult {
	if r == nil || r.config.ResearchIntent == nil || !strings.EqualFold(call.Function.Name, "artifact_publish") {
		return nil
	}
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil || strings.TrimSpace(params.Path) == "" {
		return nil
	}
	resolved := r.resolveResearchPublishPath(params.Path)
	data, err := os.ReadFile(resolved)
	if err != nil {
		result := syntheticResearchPublishResult(call, "research_citation_unreadable", "cannot read the file to check verified citations: "+err.Error())
		return &result
	}
	text := researchruntime.ExtractPublishText(resolved, data)
	if err := researchruntime.ValidateVerifiedCitations(text, r.collectResearchReceipts()); err != nil {
		result := syntheticResearchPublishResult(call, "research_citation_ungrounded", err.Error())
		return &result
	}
	return nil
}

func (r *Runner) resolveResearchPublishPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	root := strings.TrimSpace(r.config.Workspace.Root)
	if root == "" {
		return filepath.Clean(raw)
	}
	return filepath.Join(root, filepath.FromSlash(raw))
}

func (r *Runner) appendResearchJournal(ctx stdcontext.Context, events *runEventEmitter, kind SemanticKind, payload map[string]any) error {
	if events == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["revision"] = r.nextResearchRevision()
	if strings.TrimSpace(fmt.Sprint(payload["phase"])) == "" {
		payload["phase"] = researchPhaseForKind(kind)
	}
	record := NewSemanticRecord(kind, events.threadID, events.runID, events.turnID(), "", events.sessionID, payload)
	if events.recorder == nil {
		return nil
	}
	return events.persistDurable(ctx, string(kind), record)
}

func (r *Runner) nextResearchRevision() int {
	r.researchRevision++
	return r.researchRevision
}

func researchPhaseForKind(kind SemanticKind) string {
	switch kind {
	case SemanticKindResearchAccepted:
		return "accepted"
	case SemanticKindResearchTerminal:
		return "terminal"
	default:
		return "investigating"
	}
}

func syntheticResearchPublishResult(call convcontext.ToolCall, code, message string) ToolExecutionResult {
	result := ToolExecutionResult{
		ToolCallID: call.ID,
		Name:       call.Function.Name,
		StartedAt:  time.Now().UTC(),
		Status:     ToolExecutionFailed,
		IsError:    true,
		ErrorCode:  code,
		Content:    message,
	}
	result.Outcome = tool.ExecutionOutcome{
		Content:         message,
		Status:          tool.OutcomeFailed,
		ErrorCode:       code,
		ErrorMessage:    message,
		Retryable:       true,
		ErrorCategory:   tool.ErrorCategoryDomain,
		Operation:       &tool.OperationDescriptor{Type: call.Function.Name, SideEffectState: tool.OutcomeSideEffectNotStarted},
		RecoveryOptions: []string{"Fetch or read the cited source", "Relabel the citation as 未核实 or 部分核实"},
	}
	return finalizeToolResult(result)
}

type researchSourcePrefetcher struct {
	runner *Runner
	events *runEventEmitter
}

func (p *researchSourcePrefetcher) Fetch(ctx stdcontext.Context, rawURL string) (researchruntime.PrefetchedSource, error) {
	if p == nil || p.runner == nil || p.runner.toolRuntime == nil {
		return researchruntime.PrefetchedSource{}, fmt.Errorf("research source prefetcher is unavailable")
	}
	arguments, err := json.Marshal(map[string]any{"url": rawURL})
	if err != nil {
		return researchruntime.PrefetchedSource{}, err
	}
	results := p.runner.executeToolCalls(ctx, []convcontext.ToolCall{{
		ID:   convcontext.GenerateToolCallID(),
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "WebFetch",
			Arguments: string(arguments),
		},
	}}, p.events)
	if len(results) == 0 {
		return researchruntime.PrefetchedSource{}, fmt.Errorf("WebFetch returned no result")
	}
	result := results[0]
	prefetched := researchruntime.PrefetchedSource{URL: rawURL, Content: result.Content}
	if result.IsError {
		prefetched.ErrorCode = firstNonEmpty(result.ErrorCode, result.Outcome.ErrorCode, "web_fetch_failed")
		return prefetched, nil
	}
	prefetched.Receipt = &subagentruntime.HostToolReceipt{
		ToolCallID: result.ToolCallID,
		ToolName:   result.Name,
		Details:    researchReceiptDetails(result.Name, result.Outcome.Details),
	}
	return prefetched, nil
}
