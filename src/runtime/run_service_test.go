package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/tool/profile"
)

func TestRunServiceReplaysHistoryAndStreamsLiveEvents(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType, MessageID: "msg-1", Role: "assistant"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hello"})
			close(started)
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType, MessageID: "msg-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	if len(sub.History) != 3 {
		t.Fatalf("expected 3 replayed events, got %d", len(sub.History))
	}
	if sub.History[0].Kind != EventKindAgent || sub.History[0].AgentEvent.Type != agentcore.RunStartedEventType {
		t.Fatalf("unexpected first replay event: %+v", sub.History[0])
	}

	close(release)

	var live []Event
	for event := range sub.Events {
		live = append(live, event)
	}

	if len(live) != 2 {
		t.Fatalf("expected 2 live events, got %d", len(live))
	}
	if live[1].AgentEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected final RunFinished event, got %+v", live[1])
	}

	finishedDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(finishedDeadline) {
		snapshot, err := service.CancelRun(run.RunID)
		if err != nil {
			t.Fatalf("CancelRun returned error while waiting for terminal state: %v", err)
		}
		if snapshot.Status == RunStatusFinished {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach finished state")
}

func TestRunServiceUsesModelProfileForNewAndExistingThreads(t *testing.T) {
	observed := make(chan agentcore.ModelConfig, 3)
	service := NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			modelCfg := agentcore.ModelConfig{Model: "legacy-model"}
			if profile != nil {
				modelCfg = agentcore.ModelConfig{
					APIKey:             profile.APIKey,
					BaseURL:            profile.BaseURL,
					Model:              profile.ModelID,
					Protocol:           profile.Protocol,
					ProviderID:         profile.ProviderID,
					ProfileID:          profile.ProfileID,
					CredentialRevision: profile.CredentialRevision,
					AuthScheme:         profile.AuthScheme,
					Temperature:        profile.Temperature,
					MaxTokens:          profile.MaxTokens,
					ContextWindow:      profile.ContextWindow,
				}
			}
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				ModelConfig: modelCfg,
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				observed <- agent.GetConfig().ModelConfig
				return "ok", nil
			})
		},
	)

	workspace := t.TempDir()
	first, err := service.StartRun(StartRunRequest{
		Input:     "first",
		Workspace: WorkspaceConfig{WorkspaceRoot: workspace},
		ModelProfile: &ModelProfileConfig{
			ProfileID:          "profile-a",
			ProviderID:         "deepseek",
			ModelID:            "deepseek-v4-pro",
			BaseURL:            "https://api.deepseek.com",
			Protocol:           "openai_chat",
			AuthScheme:         "bearer",
			APIKey:             "secret-a",
			CredentialRevision: 1,
			ContextWindow:      400000,
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	firstCfg := <-observed
	if firstCfg.Model != "deepseek-v4-pro" || firstCfg.ProviderID != "deepseek" || firstCfg.ProfileID != "profile-a" || firstCfg.CredentialRevision != 1 || firstCfg.APIKey != "secret-a" || firstCfg.ContextWindow != 400000 {
		t.Fatalf("unexpected first model config: %+v", firstCfg)
	}
	waitForRunStatus(t, service, first.RunID, "finished")
	waitForThreadIdle(t, service, first.ThreadID)

	_, err = service.StartRun(StartRunRequest{
		Input:     "second",
		ThreadID:  first.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: workspace},
		ModelProfile: &ModelProfileConfig{
			ProfileID:     "profile-b",
			ProviderID:    "kimi-cn",
			ModelID:       "kimi-k2.6",
			BaseURL:       "https://api.moonshot.cn/v1",
			Protocol:      "openai_chat",
			AuthScheme:    "bearer",
			APIKey:        "secret-b",
			ContextWindow: 500000,
		},
	})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	secondCfg := <-observed
	if secondCfg.Model != "kimi-k2.6" || secondCfg.APIKey != "secret-b" || secondCfg.ContextWindow != 500000 {
		t.Fatalf("expected reconfigured model config, got %+v", secondCfg)
	}
	waitForThreadIdle(t, service, first.ThreadID)

	_, err = service.StartRun(StartRunRequest{
		Input:     "third",
		ThreadID:  first.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: workspace},
		ModelProfile: &ModelProfileConfig{
			ProfileID:     "profile-b",
			ProviderID:    "kimi-cn",
			ModelID:       "kimi-k2.6",
			BaseURL:       "https://api.moonshot.cn/v1",
			Protocol:      "openai_chat",
			AuthScheme:    "bearer",
			APIKey:        "secret-c",
			ContextWindow: 700000,
			Temperature:   0.3,
			MaxTokens:     4096,
		},
	})
	if err != nil {
		t.Fatalf("third StartRun returned error: %v", err)
	}
	thirdCfg := <-observed
	if thirdCfg.APIKey != "secret-c" || thirdCfg.ContextWindow != 700000 || thirdCfg.Temperature != 0.3 || thirdCfg.MaxTokens != 4096 {
		t.Fatalf("expected updated same-profile config, got %+v", thirdCfg)
	}
}

func setRuntimeUserDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(awakenHomeEnv, dir)
	return dir
}

func TestRunServiceWithModelProfilesRequiresProfile(t *testing.T) {
	service := NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			t.Fatalf("agent should not be created without a model profile")
			return nil
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "ok", nil
			})
		},
	)

	_, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()},
	})
	if !errors.Is(err, ErrMissingModelProfile) {
		t.Fatalf("expected ErrMissingModelProfile, got %v", err)
	}
}

func waitForRunStatus(t *testing.T, service *RunService, runID string, status RunStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(runID)
		if err == nil && run.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, err := service.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun(%s) returned error: %v", runID, err)
	}
	t.Fatalf("run %s status = %s, want %s", runID, run.Status, status)
}

func waitForThreadIdle(t *testing.T, service *RunService, threadID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		service.mu.RLock()
		thread, ok := service.threads[threadID]
		activeRunID := ""
		if ok {
			activeRunID = thread.activeRunID
		}
		service.mu.RUnlock()
		if ok && activeRunID == "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("thread %s did not become idle", threadID)
}

func TestRunServiceCancelSuppressesRunErrorAndEmitsCancelled(t *testing.T) {
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunErrorEventType, Error: ctx.Err().Error()})
			return "", ctx.Err()
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "cancel me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	snapshot, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
	if snapshot.Status != RunStatusCancelling {
		t.Fatalf("expected cancelling status, got %s", snapshot.Status)
	}

	var got []Event
	got = append(got, sub.History...)
	if sub.Events != nil {
		for event := range sub.Events {
			got = append(got, event)
		}
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 events total, got %d", len(got))
	}
	if got[0].AgentEvent.Type != agentcore.RunStartedEventType {
		t.Fatalf("expected RunStarted event, got %+v", got[0])
	}
	if got[1].Kind != EventKindRunStatusChanged || got[1].Status != string(RunStatusCancelling) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelling), got %+v", got[1])
	}
	if got[2].Kind != EventKindRunStatusChanged || got[2].Status != string(RunStatusCancelled) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelled), got %+v", got[2])
	}
	if got[3].Kind != EventKindRunCancelled {
		t.Fatalf("expected terminal cancelled event, got %+v", got[3])
	}

	final, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun on terminal run returned error: %v", err)
	}
	if final.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", final.Status)
	}
}

func TestRunServiceCompletesWhenExecutorReturnsWithoutTerminalEvent(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			return "done without explicit terminal event", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "finish me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	sub, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) != 2 {
		t.Fatalf("expected RunStarted plus fallback RunFinished, got %d", len(got))
	}
	if got[1].AgentEvent.Type != agentcore.RunFinishedEventType {
		t.Fatalf("expected fallback RunFinished event, got %+v", got[1])
	}

	final, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun on terminal run returned error: %v", err)
	}
	if final.Status != RunStatusFinished {
		t.Fatalf("expected finished status, got %s", final.Status)
	}
}

func TestRunServiceCancelWinsOverLateSuccessfulReturn(t *testing.T) {
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "too late"})
			return "too late", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "cancel me", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	if _, err := service.CancelRun(run.RunID); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}

	var got []Event
	got = append(got, sub.History...)
	for event := range sub.Events {
		got = append(got, event)
	}

	if len(got) != 4 {
		t.Fatalf("expected RunStarted plus RunCancelled, got %d", len(got))
	}
	if got[0].AgentEvent.Type != agentcore.RunStartedEventType {
		t.Fatalf("expected RunStarted event, got %+v", got[0])
	}
	if got[1].Kind != EventKindRunStatusChanged || got[1].Status != string(RunStatusCancelling) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelling), got %+v", got[1])
	}
	if got[2].Kind != EventKindRunStatusChanged || got[2].Status != string(RunStatusCancelled) {
		t.Fatalf("expected RUN_STATUS_CHANGED(cancelled), got %+v", got[2])
	}
	if got[3].Kind != EventKindRunCancelled {
		t.Fatalf("expected cancellation to win over late success, got %+v", got[3])
	}

	final, err := service.CancelRun(run.RunID)
	if err != nil {
		t.Fatalf("CancelRun on terminal run returned error: %v", err)
	}
	if final.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled status, got %s", final.Status)
	}
}

func TestRunServiceValidatesInputAndMissingRun(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "", errors.New("should not run")
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "   ", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if _, err := service.Subscribe("missing", 0); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from Subscribe, got %v", err)
	}
	if _, err := service.CancelRun("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from CancelRun, got %v", err)
	}
}

func TestEffectiveInputForComposerSkillTokensKeepsUserTask(t *testing.T) {
	effective := effectiveInputForComposerTokens("summarize the customer list", []ComposerToken{
		{
			ID:    "skill:daily-followup:project",
			Type:  "skill",
			Label: "Daily Followup",
			Payload: ComposerTokenPayload{
				SkillID:       "daily-followup",
				SourceOrigin:  "project",
				SkillFilePath: "/workspace/.awaken/skills/daily-followup/SKILL.md",
				RootDir:       "/workspace/.awaken/skills/daily-followup",
			},
		},
	})
	if effective != "summarize the customer list" {
		t.Fatalf("expected user task unchanged without duplicate Skill instruction, got %q", effective)
	}
	if strings.Contains(effective, "<awaken_composer_tokens>") {
		t.Fatalf("composer token internals must not enter user text, got %q", effective)
	}
}

func TestEffectiveInputForComposerSkillTokensKeepsEmptyInput(t *testing.T) {
	effective := effectiveInputForComposerTokens("", []ComposerToken{
		{ID: "skill:pptx:user", Type: "skill", Label: "pptx", Payload: ComposerTokenPayload{SkillID: "pptx"}},
	})
	if effective != "" {
		t.Fatalf("token-only turn changed the user-authored empty input: %q", effective)
	}
}

func TestComposerSkillTokenOnlyRequestIsValidAndLabelsThreadFromToken(t *testing.T) {
	token := ComposerToken{
		ID:    "skill:pptx:user",
		Type:  "skill",
		Label: "pptx",
		Payload: ComposerTokenPayload{
			SkillID:       "pptx",
			SourceOrigin:  "user",
			SkillFilePath: "/home/.awaken/skills/pptx/SKILL.md",
		},
	}
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "ok", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{
		Input:          "   ",
		ComposerTokens: []ComposerToken{token},
		Workspace:      WorkspaceConfig{WorkspaceRoot: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if len(run.ComposerTokens) != 1 || run.ComposerTokens[0].Payload.SkillID != "pptx" {
		t.Fatalf("expected run snapshot to keep composer token, got %+v", run.ComposerTokens)
	}
	transcript, err := service.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript returned error: %v", err)
	}
	if len(transcript.Runs) != 1 || len(transcript.Runs[0].Run.ComposerTokens) != 1 {
		t.Fatalf("expected transcript run to keep composer token, got %+v", transcript.Runs)
	}
	threads := service.ListThreads()
	if len(threads) != 1 || threads[0].ThreadID != run.ThreadID {
		t.Fatalf("expected one thread for token-only run, got %+v", threads)
	}
	if threads[0].Label != "pptx" {
		t.Fatalf("expected token label for thread title, got %q", threads[0].Label)
	}
	if got := threadLabelForInputAndTokens("", nil, []ComposerToken{token}); got != "pptx" {
		t.Fatalf("expected token label from helper, got %q", got)
	}
}

func TestModelProfileCacheKeyIncludesNativePDFCapability(t *testing.T) {
	base := &ModelProfileConfig{ProfileID: "profile", ModelID: "model", SupportsVision: true}
	withPDF := *base
	withPDF.SupportsNativePDF = true
	if modelProfileCacheKey(base) == modelProfileCacheKey(&withPDF) {
		t.Fatal("supports_native_pdf must participate in the profile cache key")
	}
}

func TestModelProfileCacheKeyUsesCredentialRevisionButIgnoresLegacyPromptCacheOverride(t *testing.T) {
	base := &ModelProfileConfig{
		ProfileID: "profile", ProviderID: "kimi-cn", ModelID: "kimi-k2.6",
		BaseURL: "https://api.moonshot.cn/v1", CredentialRevision: 1,
		PromptCacheKey: "legacy-one",
	}
	legacyOverride := *base
	legacyOverride.PromptCacheKey = "legacy-two"
	if modelProfileCacheKey(base) != modelProfileCacheKey(&legacyOverride) {
		t.Fatal("legacy prompt_cache_key must not affect active transport identity")
	}
	rotated := *base
	rotated.CredentialRevision = 2
	if modelProfileCacheKey(base) == modelProfileCacheKey(&rotated) {
		t.Fatal("credential revision must participate in active transport identity")
	}
}

func TestRunServiceAllowsImageOnlyInputWhenModelSupportsVision(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	service := NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{
		Input: "",
		Attachments: []RunInputAttachment{{
			ID:          "image-1",
			Kind:        RunInputAttachmentKindImage,
			MimeType:    "image/png",
			DataBase64:  "aGVsbG8=",
			DisplayName: "clip.png",
		}},
		Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile: &ModelProfileConfig{
			ProfileID:      "profile-1",
			ProviderID:     "kimi-cn",
			ModelID:        "kimi-k2.6",
			BaseURL:        "https://api.moonshot.cn/v1",
			Protocol:       "openai_chat",
			AuthScheme:     "bearer",
			APIKey:         "secret",
			SupportsVision: true,
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if len(run.InputAttachments) != 1 {
		t.Fatalf("expected one input attachment, got %+v", run.InputAttachments)
	}
	if run.InputAttachments[0].StorageKey == "" {
		t.Fatalf("expected persisted attachment storage key, got %+v", run.InputAttachments[0])
	}
	waitForRunTerminal(t, service, run.RunID)
}

func TestRunServiceDoesNotBlockImageInputOnProfileVisionFlag(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	service := NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{
		Input: "",
		Attachments: []RunInputAttachment{{
			ID:         "image-1",
			Kind:       RunInputAttachmentKindImage,
			MimeType:   "image/png",
			DataBase64: "aGVsbG8=",
		}},
		Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile: &ModelProfileConfig{
			ProfileID:      "profile-1",
			ProviderID:     "deepseek",
			ModelID:        "deepseek-v4-pro",
			BaseURL:        "https://api.deepseek.com",
			Protocol:       "openai_chat",
			AuthScheme:     "bearer",
			APIKey:         "secret",
			SupportsVision: false,
		},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)
}

func TestRunServiceAllowsManyFileAttachmentsAndMergesTaskScope(t *testing.T) {
	workspaceRoot := t.TempDir()
	externalDir := t.TempDir()
	attachments := make([]RunInputAttachment, 0, 6)
	for i := 0; i < 6; i++ {
		filePath := filepath.Join(externalDir, "attachment-"+string(rune('a'+i))+".txt")
		if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
		attachments = append(attachments, RunInputAttachment{
			ID:          "file-" + string(rune('a'+i)),
			Kind:        RunInputAttachmentKindFile,
			MimeType:    "text/plain",
			Path:        filePath,
			DisplayName: filepath.Base(filePath),
		})
	}

	var gotWorkspace WorkspaceConfig
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			gotWorkspace = workspace
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{
		Input:       "read attachments",
		Attachments: attachments,
		Workspace:   WorkspaceConfig{WorkspaceRoot: workspaceRoot},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if len(run.InputAttachments) != len(attachments) {
		t.Fatalf("expected %d input attachments, got %d", len(attachments), len(run.InputAttachments))
	}
	if len(gotWorkspace.AttachedFiles) != len(attachments) {
		t.Fatalf("expected %d task-scope attachments, got %+v", len(attachments), gotWorkspace.AttachedFiles)
	}
	waitForRunTerminal(t, service, run.RunID)
}

func TestStartRunValidWorkspaceStartsRun(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			if workspace.WorkspaceRoot != root {
				t.Fatalf("expected workspace root %q, got %q", root, workspace.WorkspaceRoot)
			}
			if len(workspace.WritableRoots) != 1 || workspace.WritableRoots[0] != root {
				t.Fatalf("expected writable roots [%q], got %v", root, workspace.WritableRoots)
			}
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.Status != RunStatusRunning {
		t.Fatalf("expected running status, got %s", run.Status)
	}
	waitForRunTerminal(t, service, run.RunID)
}

func TestStartRunEmptyWorkspaceReturnsInvalidInput(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "hello"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for empty workspace, got %v", err)
	}
}

func TestStartRunNonAbsoluteWorkspaceReturnsInvalidInput(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: "relative/path"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for non-absolute workspace, got %v", err)
	}
}

func TestStartRunOmittedWritableRootsDefaultsToWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.RunID == "" {
		t.Fatalf("expected run to be created")
	}
	waitForRunTerminal(t, service, run.RunID)
}

// TestRunServiceGetRunReturnsSnapshot verifies that GetRun returns the current
// run snapshot and that GetRun on a missing run returns ErrRunNotFound.
func TestRunServiceGetRunReturnsSnapshot(t *testing.T) {
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	got, err := service.GetRun(run.RunID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if got.RunID != run.RunID {
		t.Fatalf("expected run id %q, got %q", run.RunID, got.RunID)
	}
	if got.Status != RunStatusRunning {
		t.Fatalf("expected running status, got %s", got.Status)
	}

	if _, err := service.CancelRun(run.RunID); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
}

func TestRunServiceGetRunMissingReturnsNotFound(t *testing.T) {
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			return "done", nil
		})
	})

	if _, err := service.GetRun("missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound from GetRun, got %v", err)
	}
}

// TestRunServiceSubscribeAfterSeqFiltersHistory verifies that Subscribe with an
// afterSeq parameter only replays events whose Seq is strictly greater than
// afterSeq, and that live events continue to stream.
func TestRunServiceSubscribeAfterSeqFiltersHistory(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageStartedEventType, MessageID: "msg-1", Role: "assistant"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hello"})
			close(started)
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextMessageFinishedEventType, MessageID: "msg-1"})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	// First subscription captures the first 3 events and records the last seq.
	full, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if len(full.History) != 3 {
		t.Fatalf("expected 3 replayed events, got %d", len(full.History))
	}
	lastSeq := full.History[len(full.History)-1].Seq
	if lastSeq != 3 {
		t.Fatalf("expected last seq 3, got %d", lastSeq)
	}
	if full.Close != nil {
		full.Close()
	}

	// Subscribing with afterSeq=lastSeq should return an empty history but
	// still receive the 2 live events emitted after release.
	incremental, err := service.Subscribe(run.RunID, lastSeq)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if len(incremental.History) != 0 {
		t.Fatalf("expected 0 replayed events after seq=%d, got %d", lastSeq, len(incremental.History))
	}

	close(release)

	var live []Event
	for event := range incremental.Events {
		live = append(live, event)
	}
	if len(live) != 2 {
		t.Fatalf("expected 2 live events, got %d", len(live))
	}
	// Live events must have seq strictly greater than lastSeq.
	for i, event := range live {
		expected := lastSeq + uint64(i+1)
		if event.Seq != expected {
			t.Fatalf("live event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
	}

	finishedDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(finishedDeadline) {
		snapshot, err := service.CancelRun(run.RunID)
		if err != nil {
			t.Fatalf("CancelRun returned error while waiting for terminal state: %v", err)
		}
		if snapshot.Status == RunStatusFinished {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach finished state")
}

// TestRunServiceSubscribeAfterSeqZeroReplaysAllHistory verifies that
// Subscribe(runID, 0) replays the full history (all events have seq > 0).
func TestRunServiceSubscribeAfterSeqZeroReplaysAllHistory(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.TextDeltaEventType, MessageID: "msg-1", Role: "assistant", Delta: "hi"})
			close(started)
			<-release
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
			return "done", nil
		})
	})

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	<-started

	sub, err := service.Subscribe(run.RunID, 0)
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	// History should contain the 2 events emitted before release.
	if len(sub.History) != 2 {
		t.Fatalf("expected 2 replayed events, got %d", len(sub.History))
	}
	for i, event := range sub.History {
		expected := uint64(i + 1)
		if event.Seq != expected {
			t.Fatalf("replay event %d: expected seq %d, got %d", i, expected, event.Seq)
		}
	}

	close(release)

	// Live channel should receive the terminal RunFinished event (seq=3).
	var live []Event
	for event := range sub.Events {
		live = append(live, event)
	}
	if len(live) != 1 {
		t.Fatalf("expected 1 live event, got %d", len(live))
	}
	if live[0].Seq != 3 {
		t.Fatalf("expected live event seq 3, got %d", live[0].Seq)
	}

	finishedDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(finishedDeadline) {
		snapshot, err := service.CancelRun(run.RunID)
		if err != nil {
			t.Fatalf("CancelRun returned error while waiting for terminal state: %v", err)
		}
		if snapshot.Status == RunStatusFinished {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not reach finished state")
}

// TestStartRunWithoutThreadIDCreatesNewThread verifies that StartRun without a
// thread_id creates a new thread and stores it in the threads map.
func TestStartRunWithoutThreadIDCreatesNewThread(t *testing.T) {
	root := t.TempDir()

	var agentCreateCount int
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			agentCreateCount++
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if run.ThreadID == "" {
		t.Fatalf("expected non-empty thread_id, got empty")
	}
	if agentCreateCount != 1 {
		t.Fatalf("expected newAgent to be called once, got %d", agentCreateCount)
	}
	waitForRunTerminal(t, service, run.RunID)

	// Verify the thread is stored and accessible by starting a second run with the thread_id.
	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  run.ThreadID,
	})
	if err != nil {
		t.Fatalf("second StartRun with thread_id returned error: %v", err)
	}
	if secondRun.ThreadID != run.ThreadID {
		t.Fatalf("expected second run to reuse thread_id %q, got %q", run.ThreadID, secondRun.ThreadID)
	}
	waitForRunTerminal(t, service, secondRun.RunID)
	// Agent should not be created again for the second run.
	if agentCreateCount != 1 {
		t.Fatalf("expected newAgent to still be called once after reuse, got %d", agentCreateCount)
	}
}

// TestStartRunWithValidThreadIDReusesAgent verifies that StartRun with a valid
// thread_id reuses the same Agent instance across runs.
func TestStartRunWithValidThreadIDReusesAgent(t *testing.T) {
	root := t.TempDir()

	var (
		mu               sync.Mutex
		agentCreateCount int
		createdAgent     *agentcore.Agent
		executorAgents   []*agentcore.Agent
	)

	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			mu.Lock()
			defer mu.Unlock()
			agentCreateCount++
			createdAgent = agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
			return createdAgent
		},
		func(agent *agentcore.Agent) Executor {
			mu.Lock()
			executorAgents = append(executorAgents, agent)
			mu.Unlock()
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	// First run: creates a new thread and Agent.
	firstRun, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	// Second run: reuses the same thread and Agent.
	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  firstRun.ThreadID,
	})
	if err != nil {
		t.Fatalf("second StartRun with thread_id returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)

	mu.Lock()
	defer mu.Unlock()
	if agentCreateCount != 1 {
		t.Fatalf("expected newAgent to be called once, got %d", agentCreateCount)
	}
	if len(executorAgents) != 2 {
		t.Fatalf("expected 2 executor agent captures, got %d", len(executorAgents))
	}
	if executorAgents[0] != executorAgents[1] {
		t.Fatalf("expected both runs to receive the same Agent instance")
	}
	if executorAgents[0] != createdAgent {
		t.Fatalf("expected executor to receive the agent created by newAgent")
	}
}

func TestStartRunWithInMemoryThreadDoesNotRequireTranscriptLoad(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	badRecord := TranscriptRecord{
		RecordType:      transcriptRecordTypeMessage,
		ThreadID:        firstRun.ThreadID,
		RunID:           firstRun.RunID,
		MessageID:       "bad-child",
		ParentMessageID: "missing-parent",
		Role:            string(convcontext.RoleUser),
		Content:         "corrupt transcript tail",
		Timestamp:       time.Now().UTC(),
	}
	if err := appendTranscriptRecords(transcriptPath(firstRun.ThreadID), []TranscriptRecord{badRecord}); err != nil {
		t.Fatalf("failed to append corrupt transcript record: %v", err)
	}

	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  firstRun.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("expected in-memory thread reuse to ignore corrupt transcript, got %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)
}

// TestStartRunWithUnknownThreadIDReturnsErrThreadNotFound verifies that
// StartRun with an unknown thread_id returns ErrThreadNotFound.
func TestStartRunWithUnknownThreadIDReturnsErrThreadNotFound(t *testing.T) {
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

	_, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  "thread_unknown_123",
	})
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("expected ErrThreadNotFound, got %v", err)
	}
}

// TestStartRunWithMismatchedWorkspaceReturnsErrThreadWorkspaceMismatch
// verifies that StartRun with a thread_id from a different workspace returns
// ErrThreadWorkspaceMismatch.
func TestStartRunWithMismatchedWorkspaceReturnsErrThreadWorkspaceMismatch(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()

	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root:          workspace.WorkspaceRoot,
					WritableRoots: workspace.WritableRoots,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	// First run: creates a thread with workspace A.
	firstRun, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: rootA}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}

	// Second run: tries to reuse the thread with workspace B.
	_, err = service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: rootB},
		ThreadID:  firstRun.ThreadID,
	})
	if !errors.Is(err, ErrThreadWorkspaceMismatch) {
		t.Fatalf("expected ErrThreadWorkspaceMismatch, got %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)
}

func TestStartRunPassesRuntimeThreadIDToNewAgent(t *testing.T) {
	root := t.TempDir()
	var capturedThreadID string
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			capturedThreadID = threadID
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if capturedThreadID != run.ThreadID {
		t.Fatalf("expected newAgent to receive runtime thread_id %q, got %q", run.ThreadID, capturedThreadID)
	}
	waitForRunTerminal(t, service, run.RunID)
}

func TestStartRunWithActiveThreadReturnsErrThreadBusy(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{})
	var startedOnce sync.Once
	release := make(chan struct{})
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				startedOnce.Do(func() { close(started) })
				<-release
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: "done"})
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{Input: "hello", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	<-started

	_, err = service.StartRun(StartRunRequest{
		Input:     "second",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  firstRun.ThreadID,
	})
	if !errors.Is(err, ErrThreadBusy) {
		t.Fatalf("expected ErrThreadBusy, got %v", err)
	}

	close(release)
	waitForRunTerminal(t, service, firstRun.RunID)
	thirdRun, err := service.StartRun(StartRunRequest{
		Input:     "third",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
		ThreadID:  firstRun.ThreadID,
	})
	if err != nil {
		t.Fatalf("expected thread reuse after first run completed, got %v", err)
	}
	if thirdRun.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected third run to reuse thread %q, got %q", firstRun.ThreadID, thirdRun.ThreadID)
	}
	waitForRunTerminal(t, service, thirdRun.RunID)
}

func TestStartRunWithSameRootDifferentWritableRootsReturnsWorkspaceMismatch(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatalf("failed to create child dir: %v", err)
	}
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root, WritableRoots: []string{root}},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "world",
		Workspace: WorkspaceConfig{WorkspaceRoot: root, WritableRoots: []string{child}},
		ThreadID:  firstRun.ThreadID,
	})
	if err != nil {
		t.Fatalf("expected writable-root scope update to succeed, got %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)
}

func TestListThreadsReturnsRuntimeThreads(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	run, err := service.StartRun(StartRunRequest{Input: "hello thread", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	threads := service.ListThreads()
	if len(threads) != 1 {
		t.Fatalf("expected one thread, got %d", len(threads))
	}
	if threads[0].ThreadID != run.ThreadID {
		t.Fatalf("expected thread id %q, got %q", run.ThreadID, threads[0].ThreadID)
	}
	if threads[0].RunID != run.RunID {
		t.Fatalf("expected latest run id %q, got %q", run.RunID, threads[0].RunID)
	}
	if threads[0].ActiveRunID != "" {
		t.Fatalf("expected inactive terminal thread, got active_run_id %q", threads[0].ActiveRunID)
	}
	if threads[0].Label != "hello thread" {
		t.Fatalf("expected thread label from first input, got %q", threads[0].Label)
	}
}

func TestListThreadsReturnsRecentThreadsFirst(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				return "done", nil
			})
		},
	)

	firstRun, err := service.StartRun(StartRunRequest{Input: "first thread", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)
	time.Sleep(5 * time.Millisecond)

	secondRun, err := service.StartRun(StartRunRequest{Input: "second thread", Workspace: WorkspaceConfig{WorkspaceRoot: root}})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)

	threads := service.ListThreads()
	if len(threads) != 2 {
		t.Fatalf("expected two threads, got %d", len(threads))
	}
	if threads[0].ThreadID != secondRun.ThreadID {
		t.Fatalf("expected most recent thread first, got %q", threads[0].ThreadID)
	}
}

func TestListThreadsRestoresDurableThreadsAfterRestart(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	firstService := newDurableThreadTestService()
	firstRun, err := firstService.StartRun(StartRunRequest{
		Input:     "hello restart",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, firstRun.RunID)
	waitForTranscriptMessageCount(t, root, firstRun.ThreadID, 2)

	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: root}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	threads := restartedService.ListThreads()
	if len(threads) != 1 {
		t.Fatalf("expected one durable thread after restart, got %d", len(threads))
	}
	if threads[0].ThreadID != firstRun.ThreadID {
		t.Fatalf("expected durable thread id %q, got %q", firstRun.ThreadID, threads[0].ThreadID)
	}
	if threads[0].RunID != firstRun.RunID {
		t.Fatalf("expected durable latest run id %q, got %q", firstRun.RunID, threads[0].RunID)
	}
	if threads[0].ActiveRunID != "" {
		t.Fatalf("expected restored durable thread to be inactive, got active_run_id %q", threads[0].ActiveRunID)
	}
	if threads[0].Label != "hello restart" {
		t.Fatalf("expected restored label from first input, got %q", threads[0].Label)
	}
}

func TestRenameThreadPersistsDurableLabel(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	service := newDurableThreadTestService()
	run, err := service.StartRun(StartRunRequest{
		Input:     "original thread label",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)
	waitForTranscriptMessageCount(t, root, run.ThreadID, 2)

	renamed, err := service.RenameThread(run.ThreadID, "renamed session")
	if err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}
	if renamed.Label != "renamed session" {
		t.Fatalf("expected renamed label, got %q", renamed.Label)
	}

	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: root}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}
	threads := restartedService.ListThreads()
	if len(threads) != 1 {
		t.Fatalf("expected one durable thread, got %d", len(threads))
	}
	if threads[0].Label != "renamed session" {
		t.Fatalf("expected durable renamed label, got %q", threads[0].Label)
	}
}

func TestDeleteThreadRemovesMemoryAndDurableTranscript(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	service := newDurableThreadTestService()
	run, err := service.StartRun(StartRunRequest{
		Input:     "delete me",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)
	waitForTranscriptMessageCount(t, root, run.ThreadID, 2)

	if err := service.DeleteThread(run.ThreadID); err != nil {
		t.Fatalf("DeleteThread returned error: %v", err)
	}
	if got := service.ListThreads(); len(got) != 0 {
		t.Fatalf("expected no in-memory threads after delete, got %+v", got)
	}
	if _, err := os.Stat(transcriptPath(run.ThreadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected transcript to be removed, got %v", err)
	}

	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: root}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}
	if got := restartedService.ListThreads(); len(got) != 0 {
		t.Fatalf("expected no durable threads after restart, got %+v", got)
	}
}

func TestGetThreadTranscriptReturnsOrderedRunsAndInputs(t *testing.T) {
	root := t.TempDir()
	service := NewRunService(nil, func(agent *agentcore.Agent) Executor {
		return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
			sink.Emit(agentcore.AwakenEvent{
				Type:      agentcore.TextMessageStartedEventType,
				MessageID: "msg-1",
				Role:      "assistant",
			})
			sink.Emit(agentcore.AwakenEvent{
				Type:      agentcore.TextDeltaEventType,
				MessageID: "msg-1",
				Role:      "assistant",
				Delta:     input,
			})
			sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunFinishedEventType, Content: input})
			return input, nil
		})
	})

	firstRun, err := service.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, firstRun.RunID)

	secondRun, err := service.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  firstRun.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("second StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, service, secondRun.RunID)

	transcript, err := service.GetThreadTranscript(firstRun.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript returned error: %v", err)
	}
	if transcript.Thread.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected thread id %q, got %q", firstRun.ThreadID, transcript.Thread.ThreadID)
	}
	if len(transcript.Runs) != 2 {
		t.Fatalf("expected 2 transcript runs, got %d", len(transcript.Runs))
	}
	if transcript.Runs[0].Run.RunID != firstRun.RunID || transcript.Runs[0].Run.Input != "hello" {
		t.Fatalf("unexpected first transcript run: %+v", transcript.Runs[0].Run)
	}
	if transcript.Runs[1].Run.RunID != secondRun.RunID || transcript.Runs[1].Run.Input != "follow up" {
		t.Fatalf("unexpected second transcript run: %+v", transcript.Runs[1].Run)
	}
	if len(transcript.Runs[1].Events) == 0 {
		t.Fatalf("expected transcript history for second run")
	}
}

func TestGetThreadTranscriptRestoresDurableThreadWithoutMemory(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	firstService := newDurableThreadTestService()
	firstRun, err := firstService.StartRun(StartRunRequest{
		Input:     "hello durable transcript",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, firstRun.RunID)
	waitForTranscriptMessageCount(t, root, firstRun.ThreadID, 2)

	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: root}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	transcript, err := restartedService.GetThreadTranscript(firstRun.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript after restart returned error: %v", err)
	}
	if transcript.Thread.ThreadID != firstRun.ThreadID {
		t.Fatalf("expected thread id %q, got %q", firstRun.ThreadID, transcript.Thread.ThreadID)
	}
	if transcript.Thread.RunID != firstRun.RunID {
		t.Fatalf("expected durable latest run id %q, got %q", firstRun.RunID, transcript.Thread.RunID)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("expected one durable transcript run, got %d", len(transcript.Runs))
	}
	if transcript.Runs[0].Run.RunID != firstRun.RunID || transcript.Runs[0].Run.Input != "hello durable transcript" {
		t.Fatalf("unexpected durable transcript run: %+v", transcript.Runs[0].Run)
	}
	if len(transcript.Runs[0].Events) == 0 {
		t.Fatalf("expected durable transcript events")
	}
}

func TestGetThreadTranscriptRecoversOrphanWithoutStartingFollowUp(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	firstService := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, _ agentcore.EventSink) (string, error) {
				threadID, runID, sessionID := agent.ExecutionIdentity()
				records := []agentcore.SemanticRecord{
					agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, threadID, runID, "turn_orphan", "", sessionID, nil),
					agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, threadID, runID, "turn_orphan", "", sessionID, map[string]any{"content": input}),
					agentcore.NewSemanticRecord(agentcore.SemanticKindToolCall, threadID, runID, "turn_orphan", "", sessionID, map[string]any{
						"tool_call_id": "call_orphan_get",
						"tool_name":    "shell",
						"arguments":    `{"command":"touch maybe-ran"}`,
					}),
				}
				if err := agent.SemanticRecorder().AppendBatch(ctx, records); err != nil {
					return "", err
				}
				if err := agent.SemanticRecorder().Flush(ctx); err != nil {
					return "", err
				}
				// Deliberately omit tool_result and terminal to simulate a process exit.
				return "in-memory completion only", nil
			})
		},
	)
	run, err := firstService.StartRun(StartRunRequest{
		Input:     "simulate orphan",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, firstService, run.RunID)

	restarted := NewRunService(nil, nil)
	transcript, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("GET-only orphan recovery failed: %v", err)
	}
	if len(transcript.Runs) != 1 || transcript.Runs[0].Run.Status != RunStatusError {
		t.Fatalf("orphan was not projected as interrupted: %+v", transcript.Runs)
	}
	var toolResults, terminals int
	for _, record := range transcript.Runs[0].SemanticRecords {
		switch record.Kind {
		case agentcore.SemanticKindToolResult:
			toolResults++
		case agentcore.SemanticKindRunTerminal:
			terminals++
		}
	}
	if toolResults != 1 || terminals != 1 {
		t.Fatalf("GET-only recovery missing durable synthetic facts: results=%d terminals=%d", toolResults, terminals)
	}
	again, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	secondResults := 0
	for _, record := range again.Runs[0].SemanticRecords {
		if record.Kind == agentcore.SemanticKindToolResult {
			secondResults++
		}
	}
	if secondResults != 1 {
		t.Fatalf("orphan recovery was not idempotent: %d tool results", secondResults)
	}
}

func TestGetThreadTranscriptRestoresCancelledRunPartialResult(t *testing.T) {
	storeRoot, err := os.MkdirTemp("", "awaken-cancel-restore-test-*")
	if err != nil {
		t.Fatalf("failed to create user data dir: %v", err)
	}
	t.Setenv(awakenHomeEnv, storeRoot)
	root := t.TempDir()

	deltaEmitted := make(chan struct{})

	firstService := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root: workspace.WorkspaceRoot,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				agent.GetConversation().UserInput(input)
				sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
				sink.Emit(agentcore.AwakenEvent{
					Type:      agentcore.TextMessageStartedEventType,
					MessageID: "msg-1",
					Role:      "assistant",
				})
				sink.Emit(agentcore.AwakenEvent{
					Type:      agentcore.TextDeltaEventType,
					MessageID: "msg-1",
					Role:      "assistant",
					Delta:     "partial answer ",
				})
				agent.GetConversation().AssistantOutput("partial answer", nil)
				close(deltaEmitted)
				<-ctx.Done()
				if err := persistTestRunFactsWithStatus(context.Background(), agent, input, "partial answer", "cancelled"); err != nil {
					return "", err
				}
				return "", ctx.Err()
			})
		},
	)

	run, err := firstService.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	<-deltaEmitted
	if _, err := firstService.CancelRun(run.RunID); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}

	run = waitForRunTerminal(t, firstService, run.RunID)
	if run.Status != RunStatusCancelled {
		t.Fatalf("expected cancelled status, got %q", run.Status)
	}
	if run.Result != "partial answer" {
		t.Fatalf("expected partial result %q, got %q", "partial answer", run.Result)
	}

	waitForTranscriptMessageCount(t, root, run.ThreadID, 2)

	restartedService := NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				Workspace: agentcore.WorkspaceConfig{
					Root: workspace.WorkspaceRoot,
				},
			}, threadID)
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				return "done", nil
			})
		},
	)
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: root}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	transcript, err := restartedService.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript returned error: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("expected one run, got %d", len(transcript.Runs))
	}
	if transcript.Runs[0].Run.Status != RunStatusCancelled {
		t.Fatalf("expected restored status cancelled, got %q", transcript.Runs[0].Run.Status)
	}
	if transcript.Runs[0].Run.Result != "partial answer" {
		t.Fatalf("expected restored result %q, got %q", "partial answer", transcript.Runs[0].Run.Result)
	}

	var found bool
	for _, event := range transcript.Runs[0].Events {
		if event.Kind == EventKindAgent && event.AgentEvent.Type == agentcore.RunErrorEventType {
			if event.AgentEvent.Content == "partial answer" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected RunError event with content %q, got events: %+v", "partial answer", transcript.Runs[0].Events)
	}
}

func TestListThreadsRestoresDurableThreadsAcrossStoredWorkspaces(t *testing.T) {
	setRuntimeUserDataDir(t)
	rootA := t.TempDir()
	rootB := t.TempDir()
	firstService := newDurableThreadTestService()

	runA, err := firstService.StartRun(StartRunRequest{
		Input:     "hello workspace a",
		Workspace: WorkspaceConfig{WorkspaceRoot: rootA},
	})
	if err != nil {
		t.Fatalf("StartRun for workspace A returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, runA.RunID)
	waitForTranscriptMessageCount(t, rootA, runA.ThreadID, 2)

	runB, err := firstService.StartRun(StartRunRequest{
		Input:     "hello workspace b",
		Workspace: WorkspaceConfig{WorkspaceRoot: rootB},
	})
	if err != nil {
		t.Fatalf("StartRun for workspace B returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, runB.RunID)
	waitForTranscriptMessageCount(t, rootB, runB.ThreadID, 2)

	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: rootA}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	threads := restartedService.ListThreads()
	if len(threads) != 2 {
		t.Fatalf("expected two durable threads across workspaces, got %d", len(threads))
	}

	threadsByID := make(map[string]ThreadSnapshot, len(threads))
	for _, thread := range threads {
		threadsByID[thread.ThreadID] = thread
	}

	if threadsByID[runA.ThreadID].Workspace.WorkspaceRoot != rootA {
		t.Fatalf("expected workspace A root %q, got %+v", rootA, threadsByID[runA.ThreadID])
	}
	if threadsByID[runB.ThreadID].Workspace.WorkspaceRoot != rootB {
		t.Fatalf("expected workspace B root %q, got %+v", rootB, threadsByID[runB.ThreadID])
	}
}

func TestDurableThreadOperationsUseStoredWorkspaceMetadata(t *testing.T) {
	setRuntimeUserDataDir(t)
	rootA := t.TempDir()
	rootB := t.TempDir()
	firstService := newDurableThreadTestService()
	firstRun, err := firstService.StartRun(StartRunRequest{
		Input:     "hello stored workspace",
		Workspace: WorkspaceConfig{WorkspaceRoot: rootB},
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, firstRun.RunID)
	waitForTranscriptMessageCount(t, rootB, firstRun.ThreadID, 2)

	restartedService := newDurableThreadTestService()
	if err := restartedService.SetDefaultWorkspace(WorkspaceConfig{WorkspaceRoot: rootA}); err != nil {
		t.Fatalf("SetDefaultWorkspace returned error: %v", err)
	}

	transcript, err := restartedService.GetThreadTranscript(firstRun.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript returned error: %v", err)
	}
	if transcript.Thread.Workspace.WorkspaceRoot != rootB {
		t.Fatalf("expected transcript workspace root %q, got %+v", rootB, transcript.Thread)
	}

	renamed, err := restartedService.RenameThread(firstRun.ThreadID, "renamed from history")
	if err != nil {
		t.Fatalf("RenameThread returned error: %v", err)
	}
	if renamed.Workspace.WorkspaceRoot != rootB {
		t.Fatalf("expected renamed thread workspace root %q, got %+v", rootB, renamed)
	}

	if err := restartedService.DeleteThread(firstRun.ThreadID); err != nil {
		t.Fatalf("DeleteThread returned error: %v", err)
	}
	if _, err := os.Stat(transcriptPath(firstRun.ThreadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workspace B transcript to be removed, got %v", err)
	}
}

func TestRunServiceRestoresThreadConversationFromTranscript(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	restoredHistory := make(chan []convcontext.Message, 1)

	newService := func(captureRestored bool) *RunService {
		return NewRunService(
			func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
				return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
					Workspace: agentcore.WorkspaceConfig{
						Root:          workspace.WorkspaceRoot,
						WritableRoots: workspace.WritableRoots,
					},
				}, threadID)
			},
			func(agent *agentcore.Agent) Executor {
				return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
					if captureRestored {
						restoredHistory <- agent.GetConversation().Context.GetMessages()
					}
					agent.GetConversation().UserInput(input)
					agent.GetConversation().AssistantOutput("answer: "+input, nil)
					if err := persistTestRunFacts(ctx, agent, input, "answer: "+input); err != nil {
						return "", err
					}
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

	firstService := newService(false)
	firstRun, err := firstService.StartRun(StartRunRequest{
		Input:     "hello",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, firstRun.RunID)
	waitForTranscriptMessageCount(t, root, firstRun.ThreadID, 2)

	secondService := newService(true)
	secondRun, err := secondService.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  firstRun.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("second StartRun after restart returned error: %v", err)
	}

	var history []convcontext.Message
	select {
	case history = <-restoredHistory:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for restored conversation capture")
	}
	if len(history) != 2 {
		t.Fatalf("expected restored conversation to contain 2 messages, got %+v", history)
	}
	if history[0].Role != convcontext.RoleUser || history[0].Content != "hello" {
		t.Fatalf("unexpected restored first message: %+v", history[0])
	}
	if history[1].Role != convcontext.RoleAssistant || history[1].Content != "answer: hello" {
		t.Fatalf("unexpected restored assistant message: %+v", history[1])
	}
	waitForRunTerminal(t, secondService, secondRun.RunID)

	transcript, err := secondService.GetThreadTranscript(firstRun.ThreadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript after restart returned error: %v", err)
	}
	if len(transcript.Runs) != 2 {
		t.Fatalf("expected durable first run plus current run, got %+v", transcript.Runs)
	}
	if transcript.Runs[0].Run.RunID != firstRun.RunID || transcript.Runs[0].Run.Input != "hello" {
		t.Fatalf("unexpected durable first transcript run: %+v", transcript.Runs[0].Run)
	}
	if transcript.Runs[1].Run.RunID != secondRun.RunID || transcript.Runs[1].Run.Input != "follow up" {
		t.Fatalf("unexpected current transcript run: %+v", transcript.Runs[1].Run)
	}
}

func TestRunServiceRestoresCancelledThreadUserTranscript(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	started := make(chan struct{})
	restoredHistory := make(chan []convcontext.Message, 1)

	newService := func(captureRestored bool) *RunService {
		return NewRunService(
			func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
				return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
					Workspace: agentcore.WorkspaceConfig{
						Root:          workspace.WorkspaceRoot,
						WritableRoots: workspace.WritableRoots,
					},
				}, threadID)
			},
			func(agent *agentcore.Agent) Executor {
				return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
					if captureRestored {
						restoredHistory <- agent.GetConversation().Context.GetMessages()
						agent.GetConversation().UserInput(input)
						agent.GetConversation().AssistantOutput("answer: "+input, nil)
						if err := persistTestRunFacts(ctx, agent, input, "answer: "+input); err != nil {
							return "", err
						}
						sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
						sink.Emit(agentcore.AwakenEvent{
							Type:           agentcore.RunFinishedEventType,
							Content:        "answer: " + input,
							TerminalReason: agentcore.TerminalReasonCompleted,
						})
						return "answer: " + input, nil
					}

					agent.GetConversation().UserInput(input)
					sink.Emit(agentcore.AwakenEvent{Type: agentcore.RunStartedEventType})
					close(started)
					<-ctx.Done()
					if err := persistTestRunFactsWithStatus(context.Background(), agent, input, "", "cancelled"); err != nil {
						return "", err
					}
					return "", ctx.Err()
				})
			},
		)
	}

	firstService := newService(false)
	firstRun, err := firstService.StartRun(StartRunRequest{
		Input:     "cancel me",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("first StartRun returned error: %v", err)
	}
	<-started
	if _, err := firstService.CancelRun(firstRun.RunID); err != nil {
		t.Fatalf("CancelRun returned error: %v", err)
	}
	waitForRunTerminal(t, firstService, firstRun.RunID)
	waitForTranscriptMessageCount(t, root, firstRun.ThreadID, 1)

	secondService := newService(true)
	secondRun, err := secondService.StartRun(StartRunRequest{
		Input:     "follow up",
		ThreadID:  firstRun.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("second StartRun after cancelled restart returned error: %v", err)
	}

	var history []convcontext.Message
	select {
	case history = <-restoredHistory:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for restored cancelled conversation capture")
	}
	if len(history) != 1 {
		t.Fatalf("expected restored cancelled conversation to contain accepted user message, got %+v", history)
	}
	if history[0].Role != convcontext.RoleUser || history[0].Content != "cancel me" {
		t.Fatalf("unexpected restored cancelled user message: %+v", history[0])
	}
	waitForRunTerminal(t, secondService, secondRun.RunID)
}

func newDurableThreadTestService() *RunService {
	return NewRunService(
		func(workspace WorkspaceConfig, threadID string) *agentcore.Agent {
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
				if err := persistTestRunFacts(ctx, agent, input, "answer: "+input); err != nil {
					return "", err
				}
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

type appendFailingThreadStore struct {
	ThreadStore
	fail bool
}

func (s *appendFailingThreadStore) Prepare() error {
	if preparer, ok := s.ThreadStore.(interface{ Prepare() error }); ok {
		return preparer.Prepare()
	}
	return nil
}

func (s *appendFailingThreadStore) AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID, input string, attachments []RunInputAttachment) error {
	if s.fail {
		return errors.New("injected append run input failure")
	}
	return s.ThreadStore.AppendRunInput(snapshot, run, sessionID, input, attachments)
}

func TestStartRunStorageFailureRollsBackBusyAndGhostState(t *testing.T) {
	setRuntimeUserDataDir(t)
	root := t.TempDir()
	service := newDurableThreadTestService()
	first, err := service.StartRun(StartRunRequest{
		Input:     "first",
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunTerminal(t, service, first.RunID)

	originalStore := service.threadStore
	failing := &appendFailingThreadStore{ThreadStore: originalStore, fail: true}
	service.threadStore = failing
	if _, err := service.StartRun(StartRunRequest{
		Input:     "must fail",
		ThreadID:  first.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	}); err == nil {
		t.Fatal("expected injected storage failure")
	}
	snapshot, ok := service.threadSnapshot(first.ThreadID)
	if !ok || snapshot.ActiveRunID != "" || snapshot.RunID != first.RunID {
		t.Fatalf("failed follow-up left thread busy or latest run corrupted: %+v", snapshot)
	}

	failing.fail = false
	retry, err := service.StartRun(StartRunRequest{
		Input:     "retry succeeds",
		ThreadID:  first.ThreadID,
		Workspace: WorkspaceConfig{WorkspaceRoot: root},
	})
	if err != nil {
		t.Fatalf("retry remained blocked after rollback: %v", err)
	}
	waitForRunTerminal(t, service, retry.RunID)

	t.Setenv(awakenHomeEnv, t.TempDir())
	secondService := newDurableThreadTestService()
	secondFailing := &appendFailingThreadStore{ThreadStore: secondService.threadStore, fail: true}
	secondService.threadStore = secondFailing
	if _, err := secondService.StartRun(StartRunRequest{
		Input:     "new thread fails",
		Workspace: WorkspaceConfig{WorkspaceRoot: t.TempDir()},
	}); err == nil {
		t.Fatal("expected new-thread storage failure")
	}
	if threads := secondService.ListThreads(); len(threads) != 0 {
		t.Fatalf("failed new run left ghost thread: %+v", threads)
	}
	if len(secondService.sessions) != 0 || len(secondService.threads) != 0 {
		t.Fatalf("failed new run left in-memory ghost: sessions=%d threads=%d", len(secondService.sessions), len(secondService.threads))
	}
}

type canonicalFlushSpyRecorder struct {
	flushCalls int
}

func (r *canonicalFlushSpyRecorder) Append(context.Context, agentcore.SemanticRecord) error {
	return nil
}

func (r *canonicalFlushSpyRecorder) AppendBatch(context.Context, []agentcore.SemanticRecord) error {
	return nil
}

func (r *canonicalFlushSpyRecorder) Flush(context.Context) error {
	r.flushCalls++
	return errors.New("canonical recorder must not be flushed by legacy transcript projection")
}

func (r *canonicalFlushSpyRecorder) BroadcastsCanonicalSemanticRecords() bool { return true }

type legacyTranscriptFlushSpyStore struct {
	ThreadStore
	conversationCalls int
	runEventCalls     int
}

func (s *legacyTranscriptFlushSpyStore) AppendConversation(
	ThreadSnapshot,
	Run,
	[]convcontext.Message,
) error {
	s.conversationCalls++
	return nil
}

func (s *legacyTranscriptFlushSpyStore) AppendRunEvents(ThreadSnapshot, Run, []Event) error {
	s.runEventCalls++
	return nil
}

func TestFlushAgentTranscriptDoesNotFlushCanonicalRecorder(t *testing.T) {
	const (
		threadID = "thread_flush_ownership"
		runID    = "run_flush_ownership"
	)
	now := time.Now().UTC()
	recorder := &canonicalFlushSpyRecorder{}
	agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{}, threadID)
	agent.SetSemanticRecorder(recorder)
	store := &legacyTranscriptFlushSpyStore{}
	service := &RunService{
		threads: map[string]*Thread{
			threadID: {
				threadID:    threadID,
				agent:       agent,
				workspace:   WorkspaceConfig{WorkspaceRoot: t.TempDir()},
				label:       "flush ownership",
				latestRunID: runID,
				runIDs:      []string{runID},
				createdAt:   now,
				updatedAt:   now,
			},
		},
		threadStore: store,
	}
	session := NewSession(runID, threadID, "input", nil, nil, nil)

	if err := service.flushAgentTranscript(session, agent); err != nil {
		t.Fatalf("legacy transcript projection returned error: %v", err)
	}
	if recorder.flushCalls != 0 {
		t.Fatalf("legacy transcript projection flushed canonical recorder %d times", recorder.flushCalls)
	}
	if store.conversationCalls != 1 || store.runEventCalls != 1 {
		t.Fatalf(
			"expected one legacy conversation and event projection, got conversation=%d events=%d",
			store.conversationCalls,
			store.runEventCalls,
		)
	}
}

func persistTestRunFacts(ctx context.Context, agent *agentcore.Agent, userInput, assistantContent string) error {
	return persistTestRunFactsWithStatus(ctx, agent, userInput, assistantContent, "finished")
}

func persistTestRunFactsWithStatus(ctx context.Context, agent *agentcore.Agent, userInput, assistantContent, status string) error {
	if agent == nil || agent.SemanticRecorder() == nil {
		return nil
	}
	recorder := agent.SemanticRecorder()
	threadID, runID, sessionID := agent.ExecutionIdentity()
	records := []agentcore.SemanticRecord{agentcore.NewSemanticRecord(
		agentcore.SemanticKindUserMessage, threadID, runID, "", "", sessionID, map[string]any{
			"content": userInput,
			"role":    "user",
		},
	), agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunStarted, threadID, runID, "", "", sessionID, map[string]any{},
	)}
	if strings.TrimSpace(assistantContent) != "" {
		records = append(records, agentcore.NewSemanticRecord(
			agentcore.SemanticKindAssistantMessage, threadID, runID, "", "", sessionID, map[string]any{
				"content": assistantContent,
				"role":    "assistant",
			},
		))
	}
	records = append(records, agentcore.NewSemanticRecord(
		agentcore.SemanticKindRunTerminal, threadID, runID, "", "", sessionID, map[string]any{
			"status": status,
			"result": assistantContent,
			"code":   map[bool]string{true: "RUN_CANCELLED"}[status == "cancelled"],
		},
	))
	if err := recorder.AppendBatch(ctx, records); err != nil {
		return err
	}
	return recorder.Flush(ctx)
}

func waitForRunTerminal(t *testing.T, service *RunService, runID string) Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(runID)
		if err != nil {
			t.Fatalf("GetRun returned error: %v", err)
		}
		if isTerminalStatus(run.Status) && !threadHasActiveRun(service, run.ThreadID, run.RunID) {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach terminal status", runID)
	return Run{}
}

func threadHasActiveRun(service *RunService, threadID, runID string) bool {
	for _, thread := range service.ListThreads() {
		if thread.ThreadID == threadID {
			return thread.ActiveRunID == runID
		}
	}
	return false
}

func waitForTranscriptMessageCount(t *testing.T, workspaceRoot, threadID string, want int) []TranscriptRecord {
	t.Helper()
	_ = workspaceRoot
	deadline := time.Now().Add(2 * time.Second)
	path := transcriptPath(threadID)
	for time.Now().Before(deadline) {
		schema, err := DetectJournalSchema(path)
		if err == nil && schema == agentcore.SemanticSchemaVersion {
			semantic, readErr := ReadExecutionJournal(path)
			if readErr == nil {
				count := 0
				for _, record := range semantic {
					switch record.Kind {
					case agentcore.SemanticKindUserMessage, agentcore.SemanticKindAssistantMessage:
						count++
					}
				}
				if count >= want {
					return nil
				}
			}
		} else {
			records, err := readTranscriptRecords(path)
			if err == nil && transcriptMessageCount(records) >= want {
				return records
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("transcript for thread %s did not reach %d message records", threadID, want)
	return nil
}

// --- normalizeWorkspace ExecutionProfile allowlist tests (Part 0) ---

// TestNormalizeWorkspaceNilProfileDefaultsToWorkspaceWrite verifies that
// a workspace with no ExecutionProfile lands on profile.DefaultProfile()
// (WorkspaceWrite) after normalization.
func TestNormalizeWorkspaceNilProfileDefaultsToWorkspaceWrite(t *testing.T) {
	ws, err := normalizeWorkspace(WorkspaceConfig{WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("normalizeWorkspace returned error: %v", err)
	}
	if ws.ExecutionProfile == nil {
		t.Fatalf("expected ExecutionProfile to be non-nil (default applied)")
	}
	expected := profile.DefaultProfile()
	if !reflect.DeepEqual(*ws.ExecutionProfile, expected) {
		t.Fatalf("expected default profile %+v, got %+v", expected, *ws.ExecutionProfile)
	}
}

// TestNormalizeWorkspaceAllowedProfilePassesThrough verifies that a
// client-supplied profile whose name and fields match a built-in
// client-allowed profile is preserved (as the built-in value) after
// normalization.
func TestNormalizeWorkspaceAllowedProfilePassesThrough(t *testing.T) {
	requested := profile.WorkspaceWrite()
	ws, err := normalizeWorkspace(WorkspaceConfig{
		WorkspaceRoot:    t.TempDir(),
		ExecutionProfile: &requested,
	})
	if err != nil {
		t.Fatalf("normalizeWorkspace returned error: %v", err)
	}
	if ws.ExecutionProfile == nil {
		t.Fatalf("expected ExecutionProfile to be non-nil")
	}
	expected := profile.WorkspaceWrite()
	if !reflect.DeepEqual(*ws.ExecutionProfile, expected) {
		t.Fatalf("expected workspace_write profile %+v, got %+v", expected, *ws.ExecutionProfile)
	}
}

// TestNormalizeWorkspaceDangerFullAccessFallsBackWhenDisabled verifies
// that a client request for danger_full_access is normalized back to
// DefaultProfile when AWAKEN_ALLOW_DANGER_PROFILE is not enabled.
func TestNormalizeWorkspaceDangerFullAccessFallsBackWhenDisabled(t *testing.T) {
	t.Setenv("AWAKEN_ALLOW_DANGER_PROFILE", "")
	requested := profile.DangerFullAccess()
	ws, err := normalizeWorkspace(WorkspaceConfig{
		WorkspaceRoot:    t.TempDir(),
		ExecutionProfile: &requested,
	})
	if err != nil {
		t.Fatalf("normalizeWorkspace returned error: %v", err)
	}
	if ws.ExecutionProfile == nil {
		t.Fatalf("expected ExecutionProfile to be non-nil")
	}
	expected := profile.DefaultProfile()
	if !reflect.DeepEqual(*ws.ExecutionProfile, expected) {
		t.Fatalf("expected danger request to fall back to default %+v, got %+v", expected, *ws.ExecutionProfile)
	}
	if ws.ExecutionProfile.Name == profile.DangerFullAccess().Name {
		t.Fatalf("danger_full_access leaked through normalizeWorkspace while disabled")
	}
}

// TestNormalizeWorkspaceGuardianReadOnlyFallsBackToDefault verifies that
// a client request for guardian_read_only (a server-only profile) is
// normalized back to DefaultProfile.
func TestNormalizeWorkspaceGuardianReadOnlyFallsBackToDefault(t *testing.T) {
	requested := profile.GuardianReadOnly()
	ws, err := normalizeWorkspace(WorkspaceConfig{
		WorkspaceRoot:    t.TempDir(),
		ExecutionProfile: &requested,
	})
	if err != nil {
		t.Fatalf("normalizeWorkspace returned error: %v", err)
	}
	if ws.ExecutionProfile == nil {
		t.Fatalf("expected ExecutionProfile to be non-nil")
	}
	expected := profile.DefaultProfile()
	if !reflect.DeepEqual(*ws.ExecutionProfile, expected) {
		t.Fatalf("expected guardian request to fall back to default %+v, got %+v", expected, *ws.ExecutionProfile)
	}
	if ws.ExecutionProfile.Name == profile.GuardianReadOnly().Name {
		t.Fatalf("guardian_read_only leaked through normalizeWorkspace from a client request")
	}
}

// Compile-time assertions that sessionEventSink implements the awaiter
// interface expected by agentcore.ToolRuntime. If this fails, ask_user will
// fall back to the "not supported" error path instead of awaiting input.
var _ agentcore.UserQuestionAwaiter = sessionEventSink{}
