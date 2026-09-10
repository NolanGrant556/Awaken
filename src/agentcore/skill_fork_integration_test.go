package agentcore

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
)

type forkCaptureRecorder struct {
	mu      sync.Mutex
	records []SemanticRecord
}

func (r *forkCaptureRecorder) Append(_ context.Context, record SemanticRecord) error {
	r.mu.Lock()
	r.records = append(r.records, record)
	r.mu.Unlock()
	return nil
}

func (r *forkCaptureRecorder) AppendBatch(ctx context.Context, records []SemanticRecord) error {
	for _, record := range records {
		if err := r.Append(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (*forkCaptureRecorder) Flush(context.Context) error { return nil }

func (r *forkCaptureRecorder) snapshot() []SemanticRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]SemanticRecord(nil), r.records...)
}

func TestUnifiedSkillCatalogRunsForkInSeparateChildContext(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "forked-research")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: forked-research
description: Research in a separate child Agent.
context: fork
agent: general
---
Use the package references and return a concise research result.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceProject: root})
	manager := skillruntime.NewManager(registry, nil)
	t.Cleanup(manager.Shutdown)

	config := AgentConfig{
		Workspace:    WorkspaceConfig{Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
	}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.threadID = "thread_fork"
	runner.runID = "run_fork"
	runner.sessionID = "session_fork"
	runner.recorder = recorder
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{Content: "child research complete"}, nil
	}
	events := newRunEventEmitter("thread_fork", EventSinkFunc(func(AwakenEvent) {}))
	events.runID = "run_fork"
	events.sessionID = "session_fork"
	events.recorder = recorder
	runner.toolRuntime = NewToolRuntimeWithConfig(events.sink, toolRuntimeConfigFromAgentConfig(config))

	results := runner.executeToolCalls(t.Context(), []convcontext.ToolCall{{
		ID: "call_skill_fork",
		Function: convcontext.FunctionCall{
			Name: skillruntime.CatalogToolName, Arguments: `{"skill":"forked-research","args":"market"}`,
		},
	}}, events)
	if len(results) != 1 || results[0].IsError || results[0].Content != "child research complete" {
		t.Fatalf("fork Skill result=%+v", results)
	}
	if len(runner.config.SkillContextBlocks) != 0 {
		t.Fatalf("fork body leaked into parent context: %+v", runner.config.SkillContextBlocks)
	}
	if len(runner.conversation.Context.GetMessages()) != 0 {
		t.Fatalf("child conversation leaked into parent: %+v", runner.conversation.Context.GetMessages())
	}

	records := recorder.snapshot()
	activationIndex, childStartIndex := -1, -1
	for index, record := range records {
		switch record.Kind {
		case SemanticKindSkillActivated:
			activationIndex = index
		case SemanticKindSubagentSpawned:
			childStartIndex = index
		case SemanticKindRunStarted, SemanticKindRunTerminal:
			t.Fatalf("child emitted parent run lifecycle record: %+v", record)
		}
	}
	if activationIndex < 0 || childStartIndex < 0 || activationIndex >= childStartIndex {
		t.Fatalf("activation must be durable before child starts: %+v", records)
	}
	if invokedAt, ok := registry.LastInvokedAt("forked-research"); !ok || invokedAt.IsZero() {
		t.Fatal("successful fork invocation did not update recent use")
	}
}

func TestSkillForkMustPreserveCreatorLifetime(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "review-fork")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review-fork\ndescription: Local audit fork fixture.\ncontext: fork\nagent: general\n---\nReturn the assigned fixture result.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceProject: root})
	skillsManager := skillruntime.NewManager(registry, nil)
	t.Cleanup(skillsManager.Shutdown)
	cfg := AgentConfig{Workspace: WorkspaceConfig{Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}}, ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll}}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID, runner.sessionID = "audit-thread", "audit-run", "audit-session"
	runner.recorder = &forkCaptureRecorder{}
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		return &llm.Response{Content: "fork complete"}, nil
	}
	sink := EventSinkFunc(func(AwakenEvent) {})
	managerCfg := SubagentManagerConfig(runner.config)
	managerCfg.RunID, managerCfg.ThreadID = runner.runID, runner.threadID
	managerCfg.Starter = runner.scopedChildStarter(sink)
	managerCfg.PrepareSpawn = runner.prepareChildSpawn(sink)
	managerCfg.Persist = runner.persistSubagentEvent
	manager := subagentruntime.NewManager(t.Context(), managerCfg)
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	runner.subagentController = manager.Bind(t.Context(), "root", 0, managerCfg)
	events := newRunEventEmitter(runner.threadID, sink)
	events.runID, events.sessionID, events.recorder = runner.runID, runner.sessionID, runner.recorder
	results := runner.executeToolCalls(t.Context(), []convcontext.ToolCall{{ID: "audit-skill-call", Function: convcontext.FunctionCall{Name: skillruntime.CatalogToolName, Arguments: `{"skill":"review-fork","args":"local"}`}}}, events)
	if len(results) != 1 || results[0].IsError {
		t.Fatalf("fixture Skill did not complete: %+v", results)
	}
	t.Logf("Skill completed with %q", results[0].Content)
	next, err := runner.subagentController.Spawn(t.Context(), subagentruntime.SpawnRequest{Description: "after fork", Prompt: "next valid assignment", RunID: runner.runID, ThreadID: runner.threadID})
	if err != nil {
		t.Fatalf("active root lost creator after successful Skill tool batch: %v", err)
	}
	if _, err := runner.subagentController.Wait(t.Context(), subagentruntime.WaitRequest{TaskIDs: []string{next.TaskID}, ReturnWhen: "all", EventDriven: true}); err != nil {
		t.Fatal(err)
	}
	continued, err := runner.subagentController.(subagentruntime.SessionController).Followup(t.Context(), next.TaskID, "continue the same Agent")
	if err != nil {
		t.Fatalf("Skill broke same-Agent followup: %v", err)
	}
	if continued.AgentID != next.AgentID || continued.TaskID == next.TaskID {
		t.Fatal("followup must retain Agent identity with a new assignment")
	}
	if err := manager.EndAttempt(t.Context(), "root", runner.runID); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.subagentController.Spawn(t.Context(), subagentruntime.SpawnRequest{Description: "after end", Prompt: "must be fenced", RunID: runner.runID, ThreadID: runner.threadID}); err == nil {
		t.Fatal("Skill erased the run identity used to fence an ended attempt")
	}
}
