package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/llm"
)

// newScheduledTestService builds a RunService on an isolated AWAKEN_HOME with
// a profile-aware agent factory; newAgentCalls counts real agent creations
// (reminders must never create one).
func newScheduledTestService(t *testing.T, home string, run func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)) (*RunService, *atomic.Int32) {
	t.Helper()
	t.Setenv("AWAKEN_HOME", home)
	calls := &atomic.Int32{}
	service := NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			calls.Add(1)
			modelCfg := agentcore.ModelConfig{}
			if profile != nil {
				modelCfg = agentcore.ModelConfig{
					APIKey:     profile.APIKey,
					BaseURL:    profile.BaseURL,
					Model:      profile.ModelID,
					ProviderID: profile.ProviderID,
					ProfileID:  profile.ProfileID,
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
			return ExecutorFunc(run)
		},
	)
	t.Cleanup(func() { _ = service.FlushTranscripts() })
	return service, calls
}

func scheduledProfile() *ModelProfileConfig {
	return &ModelProfileConfig{
		ProfileID:          "prof-a",
		ProviderID:         "deepseek",
		ModelID:            "deepseek-v4-pro",
		BaseURL:            "https://api.deepseek.com",
		Protocol:           "openai_chat",
		AuthScheme:         "bearer",
		APIKey:             "scheduled-test-key",
		CredentialRevision: 1,
	}
}

func threadSnapshotOf(t *testing.T, service *RunService, threadID string) ThreadSnapshot {
	t.Helper()
	for _, snapshot := range service.ListThreads() {
		if snapshot.ThreadID == threadID {
			return snapshot
		}
	}
	t.Fatalf("thread %s not listed", threadID)
	return ThreadSnapshot{}
}

func TestStartScheduledRunDedicatedThread(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}

	first, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:       "sched_a",
		ExecutionID:  "exec_1",
		TaskName:     "每日晨报",
		Prompt:       "生成晨报",
		Workspace:    workspace,
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	waitForRunStatus(t, service, first.RunID, RunStatusFinished)
	waitForThreadIdle(t, service, first.ThreadID)

	snapshot := threadSnapshotOf(t, service, first.ThreadID)
	if snapshot.Label != "每日晨报" {
		t.Fatalf("thread label = %q, want task name", snapshot.Label)
	}
	if snapshot.TitleSource != ThreadTitleSourceTask {
		t.Fatalf("title source = %q, want %q", snapshot.TitleSource, ThreadTitleSourceTask)
	}
	service.mu.RLock()
	agentPurpose := service.threads[first.ThreadID].agent.GetConfig().ModelRequestPurpose
	service.mu.RUnlock()
	if agentPurpose != llm.RequestPurposeScheduledTask {
		t.Fatalf("scheduled model request purpose = %q, want %q", agentPurpose, llm.RequestPurposeScheduledTask)
	}
	if snapshot.TitleStatus != ThreadTitleStatusReady {
		t.Fatalf("title status = %q, want %q", snapshot.TitleStatus, ThreadTitleStatusReady)
	}
	if snapshot.Source != ScheduledClientSource {
		t.Fatalf("thread source = %q, want %q", snapshot.Source, ScheduledClientSource)
	}

	// The dedicated thread is stable across executions of the same task.
	second, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:       "sched_a",
		ExecutionID:  "exec_2",
		TaskName:     "每日晨报",
		Prompt:       "生成晨报",
		ThreadID:     first.ThreadID,
		Workspace:    workspace,
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("second StartScheduledRun: %v", err)
	}
	if second.ThreadID != first.ThreadID {
		t.Fatalf("dedicated thread not reused: %q then %q", first.ThreadID, second.ThreadID)
	}
	waitForRunStatus(t, service, second.RunID, RunStatusFinished)

	// A different task gets a different dedicated thread.
	other, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:       "sched_b",
		ExecutionID:  "exec_3",
		TaskName:     "周报",
		Prompt:       "生成周报",
		Workspace:    workspace,
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("other StartScheduledRun: %v", err)
	}
	if other.ThreadID == first.ThreadID {
		t.Fatalf("different tasks must not share the dedicated thread")
	}
	waitForRunStatus(t, service, other.RunID, RunStatusFinished)

	// The task-owned title survives finished runs (no LLM retitling).
	snapshot = threadSnapshotOf(t, service, first.ThreadID)
	if snapshot.Label != "每日晨报" || snapshot.TitleSource != ThreadTitleSourceTask {
		t.Fatalf("title rewritten after runs: label=%q source=%q", snapshot.Label, snapshot.TitleSource)
	}
}

func TestStartScheduledRunCarriesComposerTokens(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	token := ComposerToken{
		ID:          "skill:daily-report:project",
		Type:        "skill",
		LeadingText: "请使用 ",
		Label:       "daily-report",
		Payload: ComposerTokenPayload{
			SkillID:      "daily-report",
			SourceOrigin: "project",
		},
	}

	run, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:         "sched_skill",
		ExecutionID:    "exec_skill",
		TaskName:       "Skill 日报",
		Prompt:         "请使用 生成日报",
		ComposerTokens: []ComposerToken{token},
		Workspace:      WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile:   scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	if len(run.ComposerTokens) != 1 || run.ComposerTokens[0] != token {
		t.Fatalf("scheduled composer token changed: %+v", run.ComposerTokens)
	}
	// This isolated service intentionally has no Skill activator. The run can
	// fail asynchronously, but the host-only request must still preserve the
	// structured token in its durable run snapshot.
	waitForRunStatus(t, service, run.RunID, RunStatusError)
	stored, err := service.GetRun(run.RunID)
	if err != nil || len(stored.ComposerTokens) != 1 || stored.ComposerTokens[0] != token {
		t.Fatalf("stored scheduled composer token changed: %+v", stored.ComposerTokens)
	}
}

func TestStartScheduledRunReanchorsWorkspace(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	rootA := t.TempDir()
	rootB := t.TempDir()
	rootC := t.TempDir()

	first, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:       "sched_a",
		ExecutionID:  "exec_1",
		TaskName:     "任务",
		Prompt:       "跑",
		Workspace:    WorkspaceConfig{WorkspaceRoot: rootA},
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	waitForRunStatus(t, service, first.RunID, RunStatusFinished)
	waitForThreadIdle(t, service, first.ThreadID)

	// The dedicated thread follows its project when the folders moved.
	second, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:       "sched_a",
		ExecutionID:  "exec_2",
		TaskName:     "任务",
		Prompt:       "跑",
		ThreadID:     first.ThreadID,
		Workspace:    WorkspaceConfig{WorkspaceRoot: rootB},
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("reanchored StartScheduledRun: %v", err)
	}
	waitForRunStatus(t, service, second.RunID, RunStatusFinished)

	// A user run may not silently move the same thread to another root.
	if _, err := service.StartRun(StartRunRequest{
		Input:        "user run",
		ThreadID:     first.ThreadID,
		Workspace:    WorkspaceConfig{WorkspaceRoot: rootC},
		ModelProfile: scheduledProfile(),
	}); !errors.Is(err, ErrThreadWorkspaceMismatch) {
		t.Fatalf("user StartRun on moved root = %v, want %v", err, ErrThreadWorkspaceMismatch)
	}
}

func TestScheduledThreadMetadataSurvivesRestart(t *testing.T) {
	home := t.TempDir()
	service, _ := newScheduledTestService(t, home, func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	run, err := service.StartScheduledRun(ScheduledRunRequest{
		TaskID:       "sched_a",
		ExecutionID:  "exec_1",
		TaskName:     "每日晨报",
		Prompt:       "生成晨报",
		Workspace:    WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	waitForRunStatus(t, service, run.RunID, RunStatusFinished)
	if err := service.FlushTranscripts(); err != nil {
		t.Fatalf("FlushTranscripts: %v", err)
	}

	// A "restarted" Core on the same AWAKEN_HOME sees the same metadata.
	restarted, _ := newScheduledTestService(t, home, func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	snapshot := threadSnapshotOf(t, restarted, run.ThreadID)
	if snapshot.Source != ScheduledClientSource {
		t.Fatalf("restored source = %q, want %q", snapshot.Source, ScheduledClientSource)
	}
	if snapshot.TitleSource != ThreadTitleSourceTask || snapshot.Label != "每日晨报" {
		t.Fatalf("restored title = %q/%q", snapshot.Label, snapshot.TitleSource)
	}
}

func TestPublishScheduledNoticeJournal(t *testing.T) {
	service, agentCalls := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}

	threadID, runID, err := service.PublishScheduledNotice(context.Background(), ScheduledNoticeRequest{
		TaskID:      "sched_r",
		ExecutionID: "exec_1",
		TaskName:    "喝水提醒",
		Content:     "起来喝杯水",
		Workspace:   workspace,
	})
	if err != nil {
		t.Fatalf("PublishScheduledNotice: %v", err)
	}
	if threadID == "" || runID == "" {
		t.Fatalf("empty thread/run id: %q %q", threadID, runID)
	}
	if got := agentCalls.Load(); got != 0 {
		t.Fatalf("reminder created %d agents, want 0 (no model call)", got)
	}

	assertNoticeTranscript := func(wantRuns int) {
		t.Helper()
		transcript, err := service.GetThreadTranscript(threadID)
		if err != nil {
			t.Fatalf("GetThreadTranscript: %v", err)
		}
		if len(transcript.Runs) != wantRuns {
			t.Fatalf("transcript runs = %d, want %d", len(transcript.Runs), wantRuns)
		}
		latest := transcript.Runs[len(transcript.Runs)-1]
		if latest.Run.Status != RunStatusFinished {
			t.Fatalf("notice run status = %q, want finished", latest.Run.Status)
		}
		kinds := make([]agentcore.SemanticKind, 0, len(latest.SemanticRecords))
		var notice *agentcore.SemanticRecord
		for i := range latest.SemanticRecords {
			record := &latest.SemanticRecords[i]
			kinds = append(kinds, record.Kind)
			if record.Kind == agentcore.SemanticKindScheduledNotice {
				notice = record
			}
		}
		want := []agentcore.SemanticKind{agentcore.SemanticKindRunStarted, agentcore.SemanticKindScheduledNotice, agentcore.SemanticKindRunTerminal}
		if fmt.Sprint(kinds) != fmt.Sprint(want) {
			t.Fatalf("journal kinds = %v, want %v", kinds, want)
		}
		if notice == nil {
			t.Fatalf("no scheduled_notice record in %v", kinds)
		}
		if notice.Payload["task_id"] != "sched_r" || notice.Payload["content"] != "起来喝杯水" || notice.Payload["task_name"] != "喝水提醒" {
			t.Fatalf("notice payload = %+v", notice.Payload)
		}
		if _, ok := notice.Payload["execution_id"]; !ok {
			t.Fatalf("notice payload misses execution_id: %+v", notice.Payload)
		}
	}
	assertNoticeTranscript(1)

	snapshot := threadSnapshotOf(t, service, threadID)
	if snapshot.Label != "喝水提醒" || snapshot.Source != ScheduledClientSource || snapshot.TitleSource != ThreadTitleSourceTask {
		t.Fatalf("notice thread = %+v", snapshot)
	}

	// A second reminder appends to the same dedicated thread.
	threadID2, _, err := service.PublishScheduledNotice(context.Background(), ScheduledNoticeRequest{
		TaskID:      "sched_r",
		ExecutionID: "exec_2",
		TaskName:    "喝水提醒",
		Content:     "起来喝杯水",
		ThreadID:    threadID,
		Workspace:   workspace,
	})
	if err != nil {
		t.Fatalf("second PublishScheduledNotice: %v", err)
	}
	if threadID2 != threadID {
		t.Fatalf("reminder thread not reused: %q then %q", threadID, threadID2)
	}
	assertNoticeTranscript(2)
}

func TestPublishScheduledNoticeThreadBusy(t *testing.T) {
	release := make(chan struct{})
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		select {
		case <-release:
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	defer close(release)
	workspace := WorkspaceConfig{WorkspaceRoot: t.TempDir()}

	threadID, _, err := service.PublishScheduledNotice(context.Background(), ScheduledNoticeRequest{
		TaskID:      "sched_r",
		ExecutionID: "exec_1",
		TaskName:    "提醒",
		Content:     "内容",
		Workspace:   workspace,
	})
	if err != nil {
		t.Fatalf("PublishScheduledNotice: %v", err)
	}

	// A user run holding the dedicated thread blocks the next notice.
	userRun, err := service.StartRun(StartRunRequest{Input: "user", ThreadID: threadID, Workspace: workspace, ModelProfile: scheduledProfile()})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForRunStatus(t, service, userRun.RunID, RunStatusRunning)
	if _, _, err := service.PublishScheduledNotice(context.Background(), ScheduledNoticeRequest{
		TaskID:      "sched_r",
		ExecutionID: "exec_2",
		TaskName:    "提醒",
		Content:     "内容",
		ThreadID:    threadID,
		Workspace:   workspace,
	}); !errors.Is(err, ErrThreadBusy) {
		t.Fatalf("notice on busy thread = %v, want %v", err, ErrThreadBusy)
	}
}

func TestUpdateScheduledThreadTitle(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	threadID, _, err := service.PublishScheduledNotice(context.Background(), ScheduledNoticeRequest{
		TaskID:      "sched_r",
		ExecutionID: "exec_1",
		TaskName:    "旧名字",
		Content:     "内容",
		Workspace:   WorkspaceConfig{WorkspaceRoot: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("PublishScheduledNotice: %v", err)
	}

	// A task rename follows onto the dedicated thread.
	if err := service.UpdateScheduledThreadTitle(threadID, "新名字"); err != nil {
		t.Fatalf("UpdateScheduledThreadTitle: %v", err)
	}
	snapshot := threadSnapshotOf(t, service, threadID)
	if snapshot.Label != "新名字" || snapshot.TitleSource != ThreadTitleSourceTask {
		t.Fatalf("after rename label=%q source=%q", snapshot.Label, snapshot.TitleSource)
	}

	// A user-manual title is never overwritten by later task renames.
	if _, err := service.RenameThread(threadID, "用户改的标题"); err != nil {
		t.Fatalf("RenameThread: %v", err)
	}
	if err := service.UpdateScheduledThreadTitle(threadID, "任务又改名"); err != nil {
		t.Fatalf("UpdateScheduledThreadTitle after manual rename: %v", err)
	}
	snapshot = threadSnapshotOf(t, service, threadID)
	if snapshot.Label != "用户改的标题" || snapshot.TitleSource != ThreadTitleSourceManual {
		t.Fatalf("manual title overwritten: label=%q source=%q", snapshot.Label, snapshot.TitleSource)
	}
}

func TestStartRunHostTitlePinsThread(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	run, err := service.StartRun(StartRunRequest{
		Input:        "随便一段用户输入不应该变成标题",
		Workspace:    WorkspaceConfig{WorkspaceRoot: t.TempDir()},
		Title:        "固定标题",
		ModelProfile: scheduledProfile(),
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForRunStatus(t, service, run.RunID, RunStatusFinished)
	snapshot := threadSnapshotOf(t, service, run.ThreadID)
	if snapshot.Label != "固定标题" || snapshot.TitleSource != ThreadTitleSourceTask {
		t.Fatalf("pinned title = %q/%q", snapshot.Label, snapshot.TitleSource)
	}
}

func TestPublishScheduledNoticeValidatesContent(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	if _, _, err := service.PublishScheduledNotice(context.Background(), ScheduledNoticeRequest{
		TaskID:      "sched_r",
		ExecutionID: "exec_1",
		TaskName:    "提醒",
		Content:     "   ",
		Workspace:   WorkspaceConfig{WorkspaceRoot: t.TempDir()},
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty content = %v, want %v", err, ErrInvalidInput)
	}
}

func TestUpdateScheduledThreadTitleUnknownThreadNoop(t *testing.T) {
	service, _ := newScheduledTestService(t, t.TempDir(), func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	if err := service.UpdateScheduledThreadTitle("thread_missing", "名字"); err != nil {
		t.Fatalf("unknown thread must be a no-op, got %v", err)
	}
	// Give the deferred title persistence a moment; nothing should appear.
	time.Sleep(50 * time.Millisecond)
	if got := len(service.ListThreads()); got != 0 {
		t.Fatalf("no-op title update created %d threads", got)
	}
}
