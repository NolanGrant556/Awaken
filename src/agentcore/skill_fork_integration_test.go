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
agent: Researcher
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
		case SemanticKindRuntimeNotice:
			if record.Payload["notice"] == "skill_child_started" {
				childStartIndex = index
			}
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
