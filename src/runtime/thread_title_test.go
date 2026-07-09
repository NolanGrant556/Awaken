package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/agentcore"
)

func TestNewThreadFallbackTitleFromFirstUserMessage(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{
		Input:     "这些呢？",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	snapshot, ok := service.threadSnapshot(run.ThreadID)
	if !ok {
		t.Fatalf("thread %q not found after StartRun", run.ThreadID)
	}
	if snapshot.Label != "这些呢？" {
		t.Fatalf("expected fallback label %q, got %q", "这些呢？", snapshot.Label)
	}
	if snapshot.TitleSource != ThreadTitleSourceFirstUser {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceFirstUser, snapshot.TitleSource)
	}
	if snapshot.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, snapshot.TitleStatus)
	}
	if snapshot.TitleSeedRunID != run.RunID {
		t.Fatalf("expected title seed run id %q, got %q", run.RunID, snapshot.TitleSeedRunID)
	}
}

func TestSecondStartRunDoesNotOverwriteThreadTitle(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "first message",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	if _, err := service.RenameThread(firstRun.ThreadID, "manual title"); err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}

	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "second message",
		ThreadID:  firstRun.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)
	waitForThreadIdle(t, service, firstRun.ThreadID)

	snapshot, ok := service.threadSnapshot(firstRun.ThreadID)
	if !ok {
		t.Fatalf("thread %q not found after second StartRun", firstRun.ThreadID)
	}
	if snapshot.Label != "manual title" {
		t.Fatalf("expected label %q, got %q", "manual title", snapshot.Label)
	}
	if snapshot.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceManual, snapshot.TitleSource)
	}
	if snapshot.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, snapshot.TitleStatus)
	}
}

func TestGeneratedTitleOnlyOverwritesFirstUserPending(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "first",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)
	forcePendingThreadTitle(t, service, firstRun.ThreadID, firstRun.RunID)

	// Success case: first_user + pending is overwritten.
	service.ApplyGeneratedThreadTitle(firstRun.ThreadID, firstRun.RunID, "LLM title")
	snapshot, ok := service.threadSnapshot(firstRun.ThreadID)
	if !ok {
		t.Fatalf("thread %q not found", firstRun.ThreadID)
	}
	if snapshot.Label != "LLM title" {
		t.Fatalf("expected label %q, got %q", "LLM title", snapshot.Label)
	}
	if snapshot.TitleSource != ThreadTitleSourceLLM {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceLLM, snapshot.TitleSource)
	}
	if snapshot.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, snapshot.TitleStatus)
	}

	// Already llm + ready: should be ignored.
	service.ApplyGeneratedThreadTitle(firstRun.ThreadID, firstRun.RunID, "Another title")
	snapshot, _ = service.threadSnapshot(firstRun.ThreadID)
	if snapshot.Label != "LLM title" {
		t.Fatalf("expected label to remain %q, got %q", "LLM title", snapshot.Label)
	}

	// Seed run id mismatch: should be ignored.
	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "second",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)
	forcePendingThreadTitle(t, service, secondRun.ThreadID, secondRun.RunID)
	service.ApplyGeneratedThreadTitle(secondRun.ThreadID, firstRun.RunID, "Wrong seed title")
	snapshot, _ = service.threadSnapshot(secondRun.ThreadID)
	if snapshot.Label != "second" {
		t.Fatalf("expected label %q, got %q", "second", snapshot.Label)
	}
	if snapshot.TitleStatus != ThreadTitleStatusPending {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusPending, snapshot.TitleStatus)
	}
}

func TestManualTitleNotOverwrittenByLLMResults(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{
		Input:     "first",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	if _, err := service.RenameThread(run.ThreadID, "manual title"); err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}

	// LLM result with matching seed run id should not overwrite manual title.
	service.ApplyGeneratedThreadTitle(run.ThreadID, run.RunID, "LLM title")
	snapshot, ok := service.threadSnapshot(run.ThreadID)
	if !ok {
		t.Fatalf("thread %q not found", run.ThreadID)
	}
	if snapshot.Label != "manual title" {
		t.Fatalf("expected label %q, got %q", "manual title", snapshot.Label)
	}
	if snapshot.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceManual, snapshot.TitleSource)
	}
	if snapshot.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, snapshot.TitleStatus)
	}
}

func TestRenameThreadPersistsManualMetadata(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	service := newDurableThreadTestService()

	run, err := service.StartRun(StartRunRequest{
		Input:     "original label",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)
	waitForTranscriptMessageCount(t, root, run.ThreadID, 2)

	renamed, err := service.RenameThread(run.ThreadID, "renamed manual")
	if err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}
	if renamed.Label != "renamed manual" {
		t.Fatalf("expected renamed label %q, got %q", "renamed manual", renamed.Label)
	}
	if renamed.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceManual, renamed.TitleSource)
	}
	if renamed.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, renamed.TitleStatus)
	}

	// Verify the durable store carries manual metadata.
	store := newThreadStore()
	threads, err := store.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads returned error: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected one thread in store, got %d", len(threads))
	}
	if threads[0].Label != "renamed manual" {
		t.Fatalf("expected durable label %q, got %q", "renamed manual", threads[0].Label)
	}
	if threads[0].TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected durable title source %q, got %q", ThreadTitleSourceManual, threads[0].TitleSource)
	}
	if threads[0].TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected durable title status %q, got %q", ThreadTitleStatusReady, threads[0].TitleStatus)
	}
}

func TestTitleMetadataRestoredAfterRestart(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	firstService := newDurableThreadTestService()

	run, err := firstService.StartRun(StartRunRequest{
		Input:     "first",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, run.RunID)
	waitForTranscriptMessageCount(t, root, run.ThreadID, 2)

	if _, err := firstService.RenameThread(run.ThreadID, "manual after restart"); err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}

	// Simulate restart: a new service instance reads from the durable store.
	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: root}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	threads := restartedService.ListThreads()
	if len(threads) != 1 {
		t.Fatalf("expected one thread after restart, got %d", len(threads))
	}
	if threads[0].Label != "manual after restart" {
		t.Fatalf("expected restored label %q, got %q", "manual after restart", threads[0].Label)
	}
	if threads[0].TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected restored title source %q, got %q", ThreadTitleSourceManual, threads[0].TitleSource)
	}
	if threads[0].TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected restored title status %q, got %q", ThreadTitleStatusReady, threads[0].TitleStatus)
	}

	// Starting a follow-up run on the restored thread must preserve manual title.
	followUp, err := restartedService.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  run.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("follow-up StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, restartedService, followUp.RunID)
	waitForThreadIdle(t, restartedService, run.ThreadID)

	snapshot, ok := restartedService.threadSnapshot(run.ThreadID)
	if !ok {
		t.Fatalf("thread %q not found after follow-up StartRun", run.ThreadID)
	}
	if snapshot.Label != "manual after restart" {
		t.Fatalf("expected label %q, got %q", "manual after restart", snapshot.Label)
	}
	if snapshot.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("expected title source %q, got %q", ThreadTitleSourceManual, snapshot.TitleSource)
	}
	if snapshot.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected title status %q, got %q", ThreadTitleStatusReady, snapshot.TitleStatus)
	}
}

func TestGeneratedTitlePersistsForNewThread(t *testing.T) {
	setRuntimeUserDataDir(t)
	originalGenerate := generateThreadTitleWithProfileFn
	generateThreadTitleWithProfileFn = func(ctx context.Context, profile *ModelProfileConfig, prompt string) (string, error) {
		return "订单分析", nil
	}
	t.Cleanup(func() {
		generateThreadTitleWithProfileFn = originalGenerate
	})

	root := t.TempDir()
	service := NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				agent.GetConversation().UserInput(input)
				agent.GetConversation().AssistantOutput("answer: "+input, nil)
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				sink.Emit(agentcore.AwakenEvent{
					Type:           agentcore.RunFinishedEventType,
					Content:        "answer: " + input,
					TerminalReason: agentcore.TerminalReasonCompleted,
				})
				return "answer: " + input, nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{
		Input:     "帮我分析这批订单",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		Client:    ClientInfo{EntryPoint: "desktop"},
		ModelProfile: &ModelProfileConfig{
			ProfileID:  "profile-title",
			ProviderID: "mock",
			ModelID:    "mock-title-model",
			Protocol:   "openai_chat",
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	stored := waitForStoredThreadTitle(t, run.ThreadID, "订单分析", ThreadTitleSourceLLM, ThreadTitleStatusReady)
	if stored.TitleSeedRunID != run.RunID {
		t.Fatalf("expected title seed run id %q, got %q", run.RunID, stored.TitleSeedRunID)
	}

	restartedService := newDurableThreadTestService()
	threads := restartedService.ListThreads()
	if len(threads) != 1 {
		t.Fatalf("expected one restored thread, got %d", len(threads))
	}
	if threads[0].Label != "订单分析" {
		t.Fatalf("expected restored generated title %q, got %q", "订单分析", threads[0].Label)
	}
	if threads[0].TitleSource != ThreadTitleSourceLLM {
		t.Fatalf("expected restored title source %q, got %q", ThreadTitleSourceLLM, threads[0].TitleSource)
	}
	if threads[0].TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("expected restored title status %q, got %q", ThreadTitleStatusReady, threads[0].TitleStatus)
	}
}

func TestThreadTitleGenerationRetriesLowQualityThenPersistsTitle(t *testing.T) {
	setRuntimeUserDataDir(t)
	originalGenerate := generateThreadTitleWithProfileFn
	originalRetryDelays := threadTitleRetryDelays
	threadTitleRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	var calls atomic.Int32
	generateThreadTitleWithProfileFn = func(ctx context.Context, profile *ModelProfileConfig, prompt string) (string, error) {
		switch calls.Add(1) {
		case 1:
			return "", errors.New("transient provider error")
		case 2:
			return "告诉我，这个项目有什么文档？主要是什么方向的？内容都是什么", nil
		default:
			return "项目文档概览", nil
		}
	}
	t.Cleanup(func() {
		threadTitleRetryDelays = originalRetryDelays
		generateThreadTitleWithProfileFn = originalGenerate
	})

	root := t.TempDir()
	service := newProfileTitleTestService()
	run, err := service.StartRun(StartRunRequest{
		Input:     "告诉我，这个项目有什么文档？主要是什么方向的？内容都是什么",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		Client:    ClientInfo{EntryPoint: "desktop"},
		ModelProfile: &ModelProfileConfig{
			ProfileID:  "profile-title",
			ProviderID: "mock",
			ModelID:    "mock-title-model",
			Protocol:   "openai_chat",
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	stored := waitForStoredThreadTitle(t, run.ThreadID, "项目文档概览", ThreadTitleSourceLLM, ThreadTitleStatusReady)
	if stored.TitleSeedRunID != run.RunID {
		t.Fatalf("expected title seed run id %q, got %q", run.RunID, stored.TitleSeedRunID)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 title generation attempts, got %d", got)
	}
}

func TestThreadTitleGenerationFailureKeepsFallbackTitle(t *testing.T) {
	setRuntimeUserDataDir(t)
	originalGenerate := generateThreadTitleWithProfileFn
	originalRetryDelays := threadTitleRetryDelays
	threadTitleRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	generateThreadTitleWithProfileFn = func(ctx context.Context, profile *ModelProfileConfig, prompt string) (string, error) {
		return "这些呢？", nil
	}
	t.Cleanup(func() {
		threadTitleRetryDelays = originalRetryDelays
		generateThreadTitleWithProfileFn = originalGenerate
	})

	root := t.TempDir()
	service := newProfileTitleTestService()
	run, err := service.StartRun(StartRunRequest{
		Input:     "这些呢？",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		Client:    ClientInfo{EntryPoint: "desktop"},
		ModelProfile: &ModelProfileConfig{
			ProfileID:  "profile-title",
			ProviderID: "mock",
			ModelID:    "mock-title-model",
			Protocol:   "openai_chat",
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	stored := waitForStoredThreadTitle(t, run.ThreadID, "这些呢？", ThreadTitleSourceFirstUser, ThreadTitleStatusFailed)
	if stored.TitleSeedRunID != run.RunID {
		t.Fatalf("expected title seed run id %q, got %q", run.RunID, stored.TitleSeedRunID)
	}
}

func TestGeneratedThreadTitleQualityGateRejectsQuestionEcho(t *testing.T) {
	if got := validGeneratedThreadTitle("告诉我，这个项目有什么文档？主要是什么方向的？内容都是什么", "告诉我，这个项目有什么文档？主要是什么方向的？内容都是什么"); got != "" {
		t.Fatalf("expected echoed question to be rejected, got %q", got)
	}
	if got := validGeneratedThreadTitle("项目文档概览", "告诉我，这个项目有什么文档？主要是什么方向的？内容都是什么"); got != "项目文档概览" {
		t.Fatalf("expected concise topic title, got %q", got)
	}
}

func TestGenerateThreadTitleWithProfileUsesProviderRequestPolicy(t *testing.T) {
	var body map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"项目文档概览"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	title, err := generateThreadTitleWithProfile(context.Background(), &ModelProfileConfig{
		APIKey:   "test-key",
		BaseURL:  server.URL,
		ModelID:  "kimi-for-coding",
		Protocol: "openai_chat",
	}, "prompt")
	if err != nil {
		t.Fatalf("generateThreadTitleWithProfile returned error: %v", err)
	}
	if title != "项目文档概览" {
		t.Fatalf("expected generated title, got %q", title)
	}

	var temperature float64
	if raw := body["temperature"]; len(raw) == 0 {
		t.Fatalf("expected temperature in request body: %+v", body)
	} else if err := json.Unmarshal(raw, &temperature); err != nil {
		t.Fatalf("failed to decode temperature: %v", err)
	}
	if temperature != 0.6 {
		t.Fatalf("expected kimi request policy temperature 0.6, got %v in body %+v", temperature, body)
	}

	var thinking struct {
		Type string `json:"type"`
	}
	if raw := body["thinking"]; len(raw) == 0 {
		t.Fatalf("expected thinking controls in request body: %+v", body)
	} else if err := json.Unmarshal(raw, &thinking); err != nil {
		t.Fatalf("failed to decode thinking controls: %v", err)
	}
	if thinking.Type != "disabled" {
		t.Fatalf("expected thinking.type disabled, got %+v", thinking)
	}
}

func newProfileTitleTestService() *RunService {
	return NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				agent.GetConversation().UserInput(input)
				agent.GetConversation().AssistantOutput("answer: "+input, nil)
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				sink.Emit(agentcore.AwakenEvent{
					Type:           agentcore.RunFinishedEventType,
					Content:        "answer: " + input,
					TerminalReason: agentcore.TerminalReasonCompleted,
				})
				return "answer: " + input, nil
			})
		},
	)
}

func forcePendingThreadTitle(t *testing.T, service *RunService, threadID, seedRunID string) {
	t.Helper()
	now := time.Now().UTC()
	service.mu.Lock()
	thread, ok := service.threads[threadID]
	if !ok {
		service.mu.Unlock()
		t.Fatalf("thread %q not found", threadID)
	}
	thread.titleSource = ThreadTitleSourceFirstUser
	thread.titleStatus = ThreadTitleStatusPending
	thread.titleSeedRunID = seedRunID
	thread.titleUpdatedAt = now
	service.mu.Unlock()

	store, ok := service.threadStore.(*sqliteThreadStore)
	if !ok {
		t.Fatalf("expected sqlite thread store")
	}
	db, err := store.open()
	if err != nil {
		t.Fatalf("open thread store: %v", err)
	}
	if _, err := db.Exec(`
UPDATE threads
SET title_source = ?, title_status = ?, title_seed_run_id = ?, title_updated_at = ?
WHERE thread_id = ?
`, string(ThreadTitleSourceFirstUser), string(ThreadTitleStatusPending), seedRunID, now.Format(time.RFC3339Nano), threadID); err != nil {
		t.Fatalf("force pending durable title: %v", err)
	}
}

func waitForStoredThreadTitle(t *testing.T, threadID, label string, source ThreadTitleSource, status ThreadTitleStatus) ThreadSnapshot {
	t.Helper()
	store := newThreadStore()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		threads, err := store.ListThreads()
		if err == nil {
			for _, thread := range threads {
				if thread.ThreadID == threadID && thread.Label == label && thread.TitleSource == source && thread.TitleStatus == status {
					return thread
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	threads, err := store.ListThreads()
	if err != nil {
		t.Fatalf("ListThreads returned error while waiting for title: %v", err)
	}
	t.Fatalf("thread %s did not reach title %q/%s/%s; threads=%+v", threadID, label, source, status, threads)
	return ThreadSnapshot{}
}
