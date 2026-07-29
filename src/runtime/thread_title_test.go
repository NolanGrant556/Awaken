package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	generateThreadTitleWithProfileFn = func(ctx context.Context, profile *ModelProfileConfig, prompt string, _ agentcore.ModelCallAuthority, _ int) (string, error) {
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
		Input:     "请根据订单、库存、交期与供应风险生成完整分析报告",
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

func TestLocalThreadTitleUsesShortInputSkillAndAttachmentWithoutModelCall(t *testing.T) {
	if got := localThreadTitle("整理订单风险", nil, nil); got != "整理订单风险" {
		t.Fatalf("short Chinese title=%q", got)
	}
	if got := localThreadTitle("summarize quarterly sales", nil, nil); got != "summarize quarterly sales" {
		t.Fatalf("short English title=%q", got)
	}
	if got := localThreadTitle("这份材料具体说明了什么？", nil, nil); got != "" {
		t.Fatalf("question should require auxiliary title, got %q", got)
	}
	if got := localThreadTitle("long input", nil, []ComposerToken{{ID: "skill", Type: "skill", Label: "供应链复盘"}}); got != "供应链复盘" {
		t.Fatalf("skill title=%q", got)
	}
	if got := localThreadTitle("long input", []RunInputAttachment{{DisplayName: "季度报告.xlsx"}}, nil); got != "季度报告.xlsx" {
		t.Fatalf("attachment title=%q", got)
	}
}

func TestGenerateThreadTitleRecordsPurposeAndTerminalUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"成本治理"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":3,"total_tokens":23}}`))
	}))
	defer server.Close()
	journal, err := NewExecutionJournal(filepath.Join(t.TempDir(), "thread.jsonl"), "thread-title-usage")
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	_, err = generateThreadTitleWithProfile(context.Background(), &ModelProfileConfig{
		APIKey: "test", BaseURL: server.URL, ModelID: "kimi-k3", ProviderID: "kimi-cn", ProfileID: "profile-title",
		CredentialRevision: 7, Protocol: "openai_chat",
	}, "prompt", agentcore.ModelCallAuthority{ThreadID: "thread-title-usage", RunID: "run-title", Recorder: journal}, 0)
	if err != nil {
		t.Fatal(err)
	}
	records, err := journal.SnapshotRecords()
	if err != nil {
		t.Fatal(err)
	}
	var requestID string
	var usage agentcore.ModelUsageRecord
	for _, record := range records {
		switch record.Kind {
		case agentcore.SemanticKindModelRequest:
			if record.Payload["purpose"] == "thread_title" {
				requestID, _ = record.Payload["request_id"].(string)
			}
		case agentcore.SemanticKindModelUsage:
			encoded, _ := json.Marshal(record.Payload)
			_ = json.Unmarshal(encoded, &usage)
		}
	}
	if requestID == "" || usage.RequestID != requestID || usage.Purpose != "thread_title" || usage.InputTokens == nil || *usage.InputTokens != 20 {
		t.Fatalf("request=%q usage=%+v", requestID, usage)
	}
	if usage.ProviderID != "kimi-cn" || usage.ProfileID != "profile-title" || usage.CredentialRevision != 7 || usage.ModelID != "kimi-k2.6" {
		t.Fatalf("thread-title auxiliary config lost provider/profile/credential identity: %+v", usage)
	}
}

func TestThreadTitleGenerationRetriesLowQualityThenPersistsTitle(t *testing.T) {
	setRuntimeUserDataDir(t)
	originalGenerate := generateThreadTitleWithProfileFn
	originalRetryDelays := threadTitleRetryDelays
	threadTitleRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	var calls atomic.Int32
	generateThreadTitleWithProfileFn = func(ctx context.Context, profile *ModelProfileConfig, prompt string, _ agentcore.ModelCallAuthority, _ int) (string, error) {
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
	generateThreadTitleWithProfileFn = func(ctx context.Context, profile *ModelProfileConfig, prompt string, _ agentcore.ModelCallAuthority, _ int) (string, error) {
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
	}, "prompt", agentcore.ModelCallAuthority{}, 0)
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
	if temperature != 1 {
		t.Fatalf("expected Kimi K2.7 request policy temperature 1, got %v in body %+v", temperature, body)
	}
	var maxTokens int
	if raw := body["max_tokens"]; len(raw) == 0 {
		t.Fatalf("expected max_tokens in request body: %+v", body)
	} else if err := json.Unmarshal(raw, &maxTokens); err != nil {
		t.Fatalf("failed to decode max_tokens: %v", err)
	}
	if maxTokens != threadTitleForcedReasoningMaxTokens {
		t.Fatalf("expected forced-reasoning title budget %d, got %d in body %+v", threadTitleForcedReasoningMaxTokens, maxTokens, body)
	}

	if _, ok := body["thinking"]; ok {
		t.Fatalf("expected Kimi K2.7 request to omit thinking.disabled, got %+v", body)
	}
}

func TestThreadTitleMaxTokensForProfileKeepsCompactDefault(t *testing.T) {
	if got := threadTitleMaxTokensForProfile(&ModelProfileConfig{ModelID: "deepseek-v4-flash"}); got != threadTitleMaxTokens {
		t.Fatalf("expected default title budget %d, got %d", threadTitleMaxTokens, got)
	}
	if got := threadTitleMaxTokensForProfile(&ModelProfileConfig{ModelID: "kimi-for-coding-highspeed"}); got != threadTitleForcedReasoningMaxTokens {
		t.Fatalf("expected Kimi forced-reasoning budget %d, got %d", threadTitleForcedReasoningMaxTokens, got)
	}
	if got := threadTitleGenerationTimeoutForProfile(&ModelProfileConfig{ModelID: "deepseek-v4-flash"}); got != threadTitleGenerationTimeout {
		t.Fatalf("expected default title timeout %s, got %s", threadTitleGenerationTimeout, got)
	}
	if got := threadTitleGenerationTimeoutForProfile(&ModelProfileConfig{ModelID: "kimi-for-coding"}); got != threadTitleForcedReasoningTimeout {
		t.Fatalf("expected Kimi forced-reasoning timeout %s, got %s", threadTitleForcedReasoningTimeout, got)
	}
	for _, modelID := range []string{"k3", "k3-256k"} {
		profile := &ModelProfileConfig{ProviderID: "kimi-coding-plan", ModelID: modelID}
		if got := threadTitleMaxTokensForProfile(profile); got != threadTitleForcedReasoningMaxTokens {
			t.Fatalf("expected Kimi Coding Plan %s title budget %d, got %d", modelID, threadTitleForcedReasoningMaxTokens, got)
		}
		if got := threadTitleGenerationTimeoutForProfile(profile); got != threadTitleForcedReasoningTimeout {
			t.Fatalf("expected Kimi Coding Plan %s title timeout %s, got %s", modelID, threadTitleForcedReasoningTimeout, got)
		}
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

	store, ok := service.threadStore.(*FileDesktopStore)
	if !ok {
		t.Fatalf("expected FileDesktopStore")
	}
	meta, err := store.readSessionMetadata(threadID)
	if err != nil {
		t.Fatalf("read session metadata: %v", err)
	}
	meta.TitleSource = string(ThreadTitleSourceFirstUser)
	meta.TitleStatus = string(ThreadTitleStatusPending)
	meta.TitleSeedRunID = seedRunID
	meta.TitleUpdatedAt = now.Format(time.RFC3339Nano)
	if err := store.writeSessionMetadata(meta); err != nil {
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
