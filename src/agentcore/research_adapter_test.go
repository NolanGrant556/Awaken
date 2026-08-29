package agentcore

import (
	"context"
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
	_ "awaken/src/tools/write"
)

func TestRunnerHandsControlToDeepResearchWithoutRootLLM(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "output")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	config := AgentConfig{
		Workspace: WorkspaceConfig{
			Root: root, ReadableRoots: []string{root}, WritableRoots: []string{root}, OutputRoots: []string{output},
		},
		ToolExposure: tool.ToolExposurePolicy{Mode: tool.ExposureAll},
		ResearchIntent: &researchruntime.Intent{
			Query: "本仓 WebSearch 渠道是什么", Breadth: 2, SourceHint: researchruntime.SourceWorkspace,
		},
	}
	recorder := &forkCaptureRecorder{}
	runner := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config))
	runner.threadID = "thread_research"
	runner.runID = "run_research"
	runner.sessionID = "session_research"
	runner.recorder = recorder
	runner.streamLLMFunc = func(context.Context, *runEventEmitter) (*llm.Response, error) {
		t.Fatal("root model loop must not run for Deep Research")
		return nil, nil
	}
	manager := subagentruntime.NewManager(context.Background(), subagentruntime.Config{
		MaxConcurrent: 4,
		Persist:       func(context.Context, subagentruntime.PersistEvent) error { return nil },
		Starter: func(_ context.Context, request subagentruntime.StartRequest) (string, error) {
			switch request.Task.AgentType {
			case subagentruntime.AgentTypeExplore:
				return `{"claims":[{"id":"claim-0","question_id":"q1","statement":"WebSearch uses Exa by default.","confidence":"high","evidence":[{"kind":"workspace","title":"tools doc","locator":"docs/tools.md:1","source_type":"primary"}]}],"uncertainties":[]}`, nil
			case subagentruntime.AgentTypeReview:
				return `{"verdicts":[{"claim_id":"claim-0","verdict":"partial","reason":"single workspace source"}]}`, nil
			default:
				if strings.Contains(request.Prompt, "planning") || strings.Contains(request.Task.Description, "规划") {
					return `{"questions":[{"id":"q1","text":"What search providers exist?"}]}`, nil
				}
				return `{"answer":"Exa is the default WebSearch channel.","claim_ids":["claim-0"],"limitations":["needs live web confirmation"]}`, nil
			}
		},
	})
	runner.config.SubagentController = manager
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Close(closeCtx)
	})

	var finished bool
	sink := EventSinkFunc(func(event AwakenEvent) {
		if event.Type == RunFinishedEventType {
			finished = true
		}
	})
	answer, err := runner.RunStreamWithEvents(context.Background(), sink)
	if err != nil {
		t.Fatalf("RunStreamWithEvents: %v", err)
	}
	if !finished {
		t.Fatal("expected RUN_FINISHED")
	}
	if !strings.Contains(answer, "Partial") || !strings.Contains(answer, "No verified claims were produced") {
		t.Fatalf("answer=%q", answer)
	}
	kinds := map[SemanticKind]bool{}
	for _, record := range recorder.snapshot() {
		kinds[record.Kind] = true
	}
	if !kinds[SemanticKindResearchAccepted] || !kinds[SemanticKindResearchTerminal] {
		t.Fatalf("research journal kinds missing: %+v", kinds)
	}
	report := filepath.Join(output, "research-run_research.md")
	if _, err := os.Stat(report); err != nil {
		t.Fatalf("expected published report %s: %v", report, err)
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
