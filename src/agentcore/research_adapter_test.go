package agentcore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	convcontext "awaken/src/context"
	"awaken/src/llm"
	"awaken/src/researchruntime"
	"awaken/src/subagentruntime"
	"awaken/src/tool"
	_ "awaken/src/tools/artifact_publish"
	_ "awaken/src/tools/subagent"
	_ "awaken/src/tools/write"
)

func TestResearchIntentKeepsRootThoughtLoop(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	config := AgentConfig{
		MaxThoughts: 2,
		ModelConfig: ModelConfig{Model: "test-model"},
		Workspace: WorkspaceConfig{
			Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}, OutputRoots: []string{output},
		},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
		ResearchIntent: &researchruntime.Intent{
			Query: "本仓 WebSearch 渠道是什么", SourceHint: researchruntime.SourceWorkspace,
		},
	}
	conv := convcontext.NewConversation(nil)
	conv.UserInput("本仓 WebSearch 渠道是什么")
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(config, conv, NewRequestBuilder(config))
	runner.threadID = "thread_research"
	runner.runID = "run_research"
	runner.sessionID = "session_research"
	runner.recorder = recorder
	modelCalls := 0
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		modelCalls++
		return &llm.Response{Content: "Exa is the default search channel."}, nil
	}

	var finished bool
	answer, err := runner.RunStreamWithEvents(context.Background(), EventSinkFunc(func(event AwakenEvent) {
		if event.Type == RunFinishedEventType {
			finished = true
		}
	}))
	if err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}
	if modelCalls == 0 {
		t.Fatal("research run must enter the root thought loop")
	}
	if !finished {
		t.Fatal("expected RUN_FINISHED")
	}
	if !strings.Contains(answer, "Exa") {
		t.Fatalf("answer=%q", answer)
	}
	kinds := map[SemanticKind]bool{}
	for _, record := range recorder.snapshot() {
		kinds[record.Kind] = true
	}
	if !kinds[SemanticKindResearchAccepted] || !kinds[SemanticKindResearchTerminal] {
		t.Fatalf("research journal kinds missing: %+v", kinds)
	}
	if _, err := os.Stat(filepath.Join(output, "research-run_research.md")); err == nil {
		t.Fatal("host must not write a fixed research-*.md report")
	}
}

func TestResearchPublishGateRejectsUngroundedVerified(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(output, "brief.md")
	if err := os.WriteFile(report, []byte("已核实 https://missing.example/page\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := AgentConfig{
		Workspace:        WorkspaceConfig{Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}, OutputRoots: []string{output}},
		ToolExposure:     tool.ToolExposurePolicy{Mode: tool.ExposureAll},
		ToolSessionState: tool.NewToolSessionState(root),
		ResearchIntent:   &researchruntime.Intent{Query: "check citations", SourceHint: researchruntime.SourceBoth},
	}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID = "gate-thread", "gate-run"
	args, _ := json.Marshal(map[string]string{"path": "output/brief.md", "title": "Brief"})
	results := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "pub1", Type: "function", Function: convcontext.FunctionCall{Name: "artifact_publish", Arguments: string(args)},
	}}, newRunEventEmitter(runner.threadID, EventSinkFunc(func(AwakenEvent) {})))
	if len(results) != 1 || !results[0].IsError || results[0].ErrorCode != "research_citation_ungrounded" {
		t.Fatalf("expected ungrounded publish rejection, got %+v", results)
	}

	runner.noteResearchReceipt(subagentruntime.HostToolReceipt{
		ToolName: "WebFetch",
		Details:  map[string]any{"final_url": "https://missing.example/page", "content_sha256": "abc"},
	})
	passed := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "pub2", Type: "function", Function: convcontext.FunctionCall{Name: "artifact_publish", Arguments: string(args)},
	}}, newRunEventEmitter(runner.threadID, EventSinkFunc(func(AwakenEvent) {})))
	if len(passed) != 1 || passed[0].IsError {
		t.Fatalf("grounded publish should succeed: %+v", passed)
	}
}

func TestResearchReceiptDetailsExcludeFetchedBody(t *testing.T) {
	details := researchReceiptDetails("WebFetch", map[string]any{
		"final_url":      "https://example.com/",
		"content_sha256": "abc",
		"text":           "untrusted page body",
		"redirects":      []any{"private"},
	})
	if details["final_url"] != "https://example.com/" || details["content_sha256"] != "abc" {
		t.Fatalf("missing evidence metadata: %+v", details)
	}
	if _, ok := details["text"]; ok {
		t.Fatalf("page body entered host receipt: %+v", details)
	}
	if _, ok := details["redirects"]; ok {
		t.Fatalf("redirect details entered host receipt: %+v", details)
	}
}

func TestResearchRootSpawnDoesNotAttachHostAllowlist(t *testing.T) {
	cfg := AgentConfig{
		MaxThoughts:    4,
		ModelConfig:    ModelConfig{Model: "test-model"},
		ToolExposure:   tool.ToolExposurePolicy{Mode: tool.ExposureAll},
		ResearchIntent: &researchruntime.Intent{Query: "team research", SourceHint: researchruntime.SourceWorkspace},
	}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.threadID, runner.runID, runner.sessionID = "thr", "run", "ses"
	var sawHost bool
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Policy:  subagentruntime.Policy{MaxActiveAgents: 4},
		Persist: func(context.Context, subagentruntime.PersistEvent) error { return nil },
		Starter: func(_ context.Context, request subagentruntime.StartRequest) (string, error) {
			if request.Host != nil {
				sawHost = true
			}
			return "done", nil
		},
	})
	runner.subagentController = manager
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Close(closeCtx)
	})
	events := newRunEventEmitter(runner.threadID, EventSinkFunc(func(AwakenEvent) {}))
	events.runID, events.sessionID = runner.runID, runner.sessionID
	spawned := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "spawn1",
		Function: convcontext.FunctionCall{
			Name:      "subagent",
			Arguments: `{"action":"spawn","description":"调研渠道","prompt":"查 WebSearch 默认渠道","agent_type":"general"}`,
		},
	}}, events)
	if len(spawned) != 1 || spawned[0].IsError {
		t.Fatalf("spawn=%+v", spawned)
	}
	if sawHost {
		t.Fatal("research investigators must use ordinary spawn without Host allowlist")
	}
	var spawnPayload struct {
		Task subagentruntime.TaskSnapshot `json:"task"`
	}
	if err := json.Unmarshal([]byte(spawned[0].Content), &spawnPayload); err != nil {
		t.Fatalf("decode spawn: %v", err)
	}
	followArgs, _ := json.Marshal(map[string]any{
		"action": "followup", "task_id": spawnPayload.Task.TaskID, "prompt": "补一眼出处冲突",
	})
	followed := runner.executeToolCalls(context.Background(), []convcontext.ToolCall{{
		ID: "follow1",
		Function: convcontext.FunctionCall{Name: "subagent", Arguments: string(followArgs)},
	}}, events)
	if len(followed) != 1 || followed[0].IsError {
		t.Fatalf("research followup must stay available: %+v", followed)
	}
	if strings.Contains(followed[0].Content, "does not allow automatic follow-up") {
		t.Fatalf("host workflow followup ban leaked onto research investigators: %s", followed[0].Content)
	}
}

func TestCollectResearchReceiptsIncludesDescendants(t *testing.T) {
	cfg := AgentConfig{ResearchIntent: &researchruntime.Intent{Query: "team", SourceHint: researchruntime.SourceWorkspace}}
	runner := NewRunner(cfg, convcontext.NewConversation(nil), NewRequestBuilder(cfg))
	runner.noteResearchReceipt(subagentruntime.HostToolReceipt{
		ToolName: "Read",
		Details:  map[string]any{"file_path": "/workspace/docs/a.md", "sha256": "aaa"},
	})
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		Policy:  subagentruntime.Policy{MaxActiveAgents: 2},
		Persist: func(context.Context, subagentruntime.PersistEvent) error { return nil },
		Starter: func(ctx context.Context, request subagentruntime.StartRequest) (string, error) {
			if err := request.ReportProgress(ctx, subagentruntime.ProgressUpdate{
				HostToolReceipt: &subagentruntime.HostToolReceipt{
					ToolCallID: "child-fetch",
					ToolName:   "WebFetch",
					Details:    map[string]any{"final_url": "https://example.com/docs", "content_sha256": "bbb"},
				},
			}); err != nil {
				return "", err
			}
			return "done", nil
		},
	})
	runner.subagentController = manager
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Close(closeCtx)
	})
	task, err := manager.Spawn(context.Background(), subagentruntime.SpawnRequest{
		Description: "查文件", Prompt: "读 docs/a.md", AgentType: "general", ParentAgentID: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Wait(context.Background(), subagentruntime.WaitRequest{
		TaskIDs: []string{task.TaskID}, ReturnWhen: "all", Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	receipts := runner.collectResearchReceipts()
	if len(receipts) < 2 {
		t.Fatalf("expected parent and child receipts, got %+v", receipts)
	}
	foundChild := false
	for _, receipt := range receipts {
		url, _ := receipt.Details["final_url"].(string)
		if receipt.ToolName == "WebFetch" && url == "https://example.com/docs" {
			foundChild = true
		}
	}
	if !foundChild {
		t.Fatalf("descendant WebFetch receipt missing: %+v", receipts)
	}
}

func TestRequestBuilderInjectsResearchTeamContract(t *testing.T) {
	cfg := AgentConfig{
		ResearchIntent: &researchruntime.Intent{Query: "What is WebSearch?", SourceHint: researchruntime.SourceBoth},
	}
	envelope := NewRequestBuilder(cfg).BuildContextEnvelope(convcontext.New(), nil)
	found := false
	for _, block := range envelope.SystemBlocks {
		if block.ID == "research_team" && strings.Contains(block.Content, "editor-in-chief") && strings.Contains(block.Content, "artifact_publish") {
			found = true
		}
	}
	if !found {
		t.Fatalf("research team block missing: %+v", envelope.SystemBlocks)
	}
}
