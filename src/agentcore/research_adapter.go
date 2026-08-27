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
	"awaken/src/desktopdata"
	"awaken/src/llm"
	"awaken/src/researchruntime"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

func (r *Runner) runDeepResearch(ctx stdcontext.Context, events *runEventEmitter) (string, error) {
	if r.config.ResearchIntent == nil {
		return "", fmt.Errorf("research intent is missing")
	}
	if r.subagentController == nil {
		err := fmt.Errorf("deep research requires a subagent controller")
		events.runErrored(ctx, err)
		return "", err
	}
	turnState := newTurnState(events.threadID, events.runID, r.effectiveMaxThoughts(), r.config.ModelConfig.Model)
	events.turnStarted(turnState)

	orch := &researchruntime.Orchestrator{
		Children: &researchChildClient{
			controller: r.subagentController,
			threadID:   r.threadID,
			runID:      r.runID,
			turnID:     events.turnID(),
		},
		Plan: &researchPlanWriter{
			service: r.runPlanService,
			identity: tool.ToolExecutionIdentity{
				RunID:   r.runID,
				TurnID:  events.turnID(),
				AgentID: "root",
			},
		},
		PublishFn: &researchPublisher{runner: r, events: events},
		Synth:     &researchSynthesizer{runner: r},
		Journal:   &researchJournal{runner: r, events: events},
		Cancelled: func() bool { return ctx.Err() != nil },
	}
	result, err := orch.Run(ctx, *r.config.ResearchIntent)
	if err != nil {
		if ctx.Err() != nil {
			events.turnFinished(ctx, TerminalReasonCancelled, "")
			events.runErrored(ctx, ctx.Err(), TerminalReasonCancelled)
			return "", ctx.Err()
		}
		events.turnFinished(ctx, terminalReasonForError(err), "")
		events.runErrored(ctx, err)
		return "", err
	}
	if result.Outcome == researchruntime.OutcomeCancelled || result.Outcome == researchruntime.OutcomeInterrupted {
		events.turnFinished(ctx, TerminalReasonCancelled, "")
		events.runErrored(ctx, stdcontext.Canceled, TerminalReasonCancelled)
		return "", stdcontext.Canceled
	}
	if result.Outcome == researchruntime.OutcomeFailed {
		err := fmt.Errorf("deep research failed")
		if notes := strings.TrimSpace(strings.Join(compactFailedNotes(result.Coverage), "; ")); notes != "" {
			err = fmt.Errorf("deep research failed: %s", notes)
		}
		events.turnFinished(ctx, terminalReasonForError(err), "")
		events.runErrored(ctx, err)
		return "", err
	}

	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = strings.TrimSpace(result.Markdown)
	}
	if result.Outcome == researchruntime.OutcomePartial {
		answer = "Partial\n\n" + answer
	} else if result.Outcome == researchruntime.OutcomeVerified {
		answer = "Verified\n\n" + answer
	}
	if strings.TrimSpace(answer) != "" {
		events.onLLMDelta(answer, false)
		events.finishTextIfOpen()
		if r.conversation != nil {
			r.conversation.AssistantOutput(answer, nil)
		}
	}
	finalState, snapshotErr := r.persistCurrentContextSnapshot(ctx, events)
	if snapshotErr != nil {
		finalState = nil
	}
	events.emitTurnDiff(ctx, r.config.Workspace.Root)
	events.turnFinished(ctx, TerminalReasonCompleted, "")
	events.runFinished(ctx, answer, finalState, TerminalReasonCompleted)
	return answer, nil
}

func compactFailedNotes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		note := strings.TrimSpace(value)
		if note == "" {
			continue
		}
		runes := []rune(note)
		if len(runes) > 160 {
			note = string(runes[:160]) + "…"
		}
		out = append(out, note)
	}
	return out
}

type researchChildClient struct {
	controller subagentruntime.Controller
	threadID   string
	runID      string
	turnID     string
}

func (c *researchChildClient) Run(ctx stdcontext.Context, spec researchruntime.ChildSpec) (string, error) {
	if c == nil || c.controller == nil {
		return "", fmt.Errorf("research child controller is unavailable")
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	host := spec.Host
	if host == nil {
		host = researchruntime.ChildHostSpawnOptions(researchruntime.SourceBoth)
	}
	snapshot, err := c.controller.SpawnFromHost(ctx, subagentruntime.SpawnRequest{
		Description:   spec.Description,
		Prompt:        spec.Prompt,
		AgentType:     spec.AgentType,
		WorkspaceMode: subagentruntime.WorkspaceSharedReadOnly,
		ParentAgentID: "root",
		ThreadID:      c.threadID,
		RunID:         c.runID,
		TurnID:        c.turnID,
		Host:          host,
	})
	if err != nil {
		return "", err
	}
	for {
		if ctx.Err() != nil {
			_, _ = c.controller.Cancel(ctx, snapshot.TaskID, "research_stop")
			return "", ctx.Err()
		}
		wait, waitErr := c.controller.Wait(ctx, subagentruntime.WaitRequest{
			TaskIDs:    []string{snapshot.TaskID},
			ReturnWhen: "all",
			Timeout:    60 * time.Second,
		})
		if waitErr != nil {
			return "", waitErr
		}
		if wait.RetrievalStatus != subagentruntime.RetrievalCompleted || len(wait.Tasks) == 0 {
			continue
		}
		task := wait.Tasks[0]
		switch task.Status {
		case subagentruntime.StatusCompleted:
			return task.ResultPreview, nil
		case subagentruntime.StatusCancelled, subagentruntime.StatusInterrupted:
			return "", stdcontext.Canceled
		default:
			msg := strings.TrimSpace(task.ErrorMessage)
			if msg == "" {
				msg = string(task.Status)
			}
			return "", fmt.Errorf("%s", msg)
		}
	}
}

type researchPlanWriter struct {
	service  tool.RunPlanService
	identity tool.ToolExecutionIdentity
	version  uint64
}

func (w *researchPlanWriter) Apply(ctx stdcontext.Context, items []researchruntime.PlanItem) error {
	if w == nil || w.service == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	update := tool.RunPlanUpdate{
		Explanation: "deep research",
		BaseVersion: w.version,
		Items:       make([]tool.RunPlanItem, 0, len(items)),
	}
	for _, item := range items {
		update.Items = append(update.Items, tool.RunPlanItem{
			Step:       item.Step,
			ActiveForm: researchActiveForm(item.Step, item.Status),
			Status:     tool.RunPlanStatus(item.Status),
		})
	}
	result, err := w.service.ApplyUpdate(ctx, w.identity, update)
	if err != nil {
		return err
	}
	w.version = result.Snapshot.Version
	return nil
}

func researchActiveForm(step, status string) string {
	verb := strings.TrimSpace(step)
	switch tool.RunPlanStatus(status) {
	case tool.RunPlanInProgress:
		return "正在" + verb
	case tool.RunPlanCompleted:
		return "已完成" + verb
	default:
		return "等待" + verb
	}
}

type researchPublisher struct {
	runner *Runner
	events *runEventEmitter
}

func (p *researchPublisher) Publish(ctx stdcontext.Context, markdown string) (string, string, error) {
	if p == nil || p.runner == nil || p.runner.toolRuntime == nil {
		return "", "", fmt.Errorf("research publisher is unavailable")
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	runID := strings.TrimSpace(p.runner.runID)
	relPath := filepath.ToSlash(filepath.Join("output", "research-"+runID+".md"))
	writeArgs, err := json.Marshal(map[string]any{"file_path": relPath, "content": markdown})
	if err != nil {
		return "", "", err
	}
	writeResults := p.runner.executeToolCalls(ctx, []convcontext.ToolCall{{
		ID:   convcontext.GenerateToolCallID(),
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "Write",
			Arguments: string(writeArgs),
		},
	}}, p.events)
	if len(writeResults) == 0 || writeResults[0].IsError {
		msg := "write research report failed"
		if len(writeResults) > 0 && strings.TrimSpace(writeResults[0].Content) != "" {
			msg = writeResults[0].Content
		}
		return "", "", fmt.Errorf("%s", msg)
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	publishArgs, err := json.Marshal(map[string]any{"path": relPath, "title": "Deep Research Report"})
	if err != nil {
		return "", "", err
	}
	publishResults := p.runner.executeToolCalls(ctx, []convcontext.ToolCall{{
		ID:   convcontext.GenerateToolCallID(),
		Type: "function",
		Function: convcontext.FunctionCall{
			Name:      "artifact_publish",
			Arguments: string(publishArgs),
		},
	}}, p.events)
	if len(publishResults) == 0 || publishResults[0].IsError {
		msg := "publish research report failed"
		if len(publishResults) > 0 && strings.TrimSpace(publishResults[0].Content) != "" {
			msg = publishResults[0].Content
		}
		return "", "", fmt.Errorf("%s", msg)
	}
	artifactID := ""
	if len(publishResults[0].Outcome.Artifacts) > 0 {
		artifactID = publishResults[0].Outcome.Artifacts[0].ID
	}
	return relPath, artifactID, nil
}

type researchSynthesizer struct {
	runner *Runner
}

func (s *researchSynthesizer) Synthesize(ctx stdcontext.Context, prompt string) (string, error) {
	if s == nil || s.runner == nil {
		return "", fmt.Errorf("research synthesizer is unavailable")
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	model := strings.TrimSpace(s.runner.config.ModelConfig.Model)
	if model == "" {
		return "", fmt.Errorf("synthesis model is unavailable")
	}
	req := LLMRequest{Model: model, Temperature: s.runner.config.ModelConfig.Temperature, MaxTokens: s.runner.config.ModelConfig.MaxTokens}
	return llm.Call(ctx, s.runner.transportConfig(req), llm.Request{
		RequestID:       newAwakenID("research_synth"),
		SystemPrompt:    "Return JSON only. Do not call tools or invent citation IDs.",
		UserPrompt:      prompt,
		Model:           model,
		Purpose:         llm.RequestPurposeAgentTurn,
		AffinityScopeID: s.runner.threadID,
	})
}

type researchJournal struct {
	runner   *Runner
	events   *runEventEmitter
	revision int
	ledger   map[string]any
}

func (j *researchJournal) Append(ctx stdcontext.Context, kind string, payload map[string]any) error {
	if j == nil || j.events == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	semanticKind, ok := researchSemanticKind(kind)
	if !ok {
		return nil
	}
	j.revision++
	if payload == nil {
		payload = map[string]any{}
	}
	payload["revision"] = j.revision
	payload["phase"] = researchPhaseForKind(semanticKind)
	if j.ledger == nil {
		j.ledger = map[string]any{
			"schema_version": 1,
			"run_id":         j.events.runID,
			"thread_id":      j.events.threadID,
		}
	}
	for key, value := range payload {
		j.ledger[key] = value
	}
	record := NewSemanticRecord(semanticKind, j.events.threadID, j.events.runID, j.events.turnID(), "", j.events.sessionID, payload)
	if j.events.recorder == nil {
		writeResearchLedger(j.runner, j.ledger)
		return nil
	}
	if err := j.events.persistDurable(ctx, kind, record); err != nil {
		return err
	}
	writeResearchLedger(j.runner, j.ledger)
	return nil
}

func researchSemanticKind(kind string) (SemanticKind, bool) {
	switch strings.TrimSpace(kind) {
	case string(SemanticKindResearchAccepted):
		return SemanticKindResearchAccepted, true
	case string(SemanticKindResearchPlan):
		return SemanticKindResearchPlan, true
	case string(SemanticKindResearchClaimBatch):
		return SemanticKindResearchClaimBatch, true
	case string(SemanticKindResearchVerification):
		return SemanticKindResearchVerification, true
	case string(SemanticKindResearchReport):
		return SemanticKindResearchReport, true
	case string(SemanticKindResearchTerminal):
		return SemanticKindResearchTerminal, true
	default:
		return "", false
	}
}

func researchPhaseForKind(kind SemanticKind) string {
	switch kind {
	case SemanticKindResearchAccepted:
		return "accepted"
	case SemanticKindResearchPlan:
		return "planning"
	case SemanticKindResearchClaimBatch:
		return "researching"
	case SemanticKindResearchVerification:
		return "verifying"
	case SemanticKindResearchReport:
		return "reporting"
	case SemanticKindResearchTerminal:
		return "terminal"
	default:
		return ""
	}
}

func writeResearchLedger(runner *Runner, payload map[string]any) {
	if runner == nil {
		return
	}
	home := desktopdata.AwakenHomeRoot()
	if strings.TrimSpace(home) == "" || strings.TrimSpace(runner.threadID) == "" || strings.TrimSpace(runner.runID) == "" {
		return
	}
	path := desktopdata.ResearchLedgerPath(home, runner.threadID, runner.runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return
	}
	_ = file.Sync()
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Rename(tmp, path)
}
