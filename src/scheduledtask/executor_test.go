package scheduledtask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/agentcore"
	"awaken/src/modelprofile"
	"awaken/src/runtime"
)

func TestRuntimeComposerTokensPreserveScheduledSkillIdentity(t *testing.T) {
	tokens := []ComposerToken{{
		ID:          "skill:daily-report:project",
		Type:        "skill",
		LeadingText: "请使用 ",
		Label:       "daily-report",
		Payload: ComposerTokenPayload{
			SkillID:       "daily-report",
			SourceOrigin:  "project",
			SkillFilePath: "/workspace/.awaken/skills/daily-report/SKILL.md",
			RootDir:       "/workspace/.awaken/skills/daily-report",
		},
	}}

	converted := runtimeComposerTokens(tokens)
	if len(converted) != 1 {
		t.Fatalf("converted token count = %d, want 1", len(converted))
	}
	if converted[0].ID != tokens[0].ID ||
		converted[0].LeadingText != tokens[0].LeadingText ||
		converted[0].Payload.SkillID != tokens[0].Payload.SkillID ||
		converted[0].Payload.SkillFilePath != tokens[0].Payload.SkillFilePath {
		t.Fatalf("converted token lost identity: %+v", converted[0])
	}
}

func TestRuntimeComposerTokensPreserveScheduledPluginIdentity(t *testing.T) {
	tokens := []ComposerToken{{
		ID: "plugin:acme.tools:digest-v1", Type: "plugin", Label: "acme.tools",
		Payload: ComposerTokenPayload{PluginName: "acme.tools", PluginDigest: "digest-v1"},
	}}
	converted := runtimeComposerTokens(tokens)
	if len(converted) != 1 || converted[0].Payload.PluginName != "acme.tools" || converted[0].Payload.PluginDigest != "digest-v1" {
		t.Fatalf("converted Plugin token lost identity: %+v", converted)
	}
}

func TestRuntimeComposerTokensPreserveScheduledComputerUseIdentity(t *testing.T) {
	tokens := []ComposerToken{{
		ID: "capability:computer-use", Type: "capability", Label: "Computer Use",
		Payload: ComposerTokenPayload{CapabilityID: "computer-use"},
	}}
	converted := runtimeComposerTokens(tokens)
	if len(converted) != 1 || converted[0].Payload.CapabilityID != "computer-use" {
		t.Fatalf("converted Computer Use token lost identity: %+v", converted)
	}
}

// executorProbe captures what the RunService agent factory saw and holds the
// configurable fake run behaviour.
type executorProbe struct {
	mu         sync.Mutex
	agentCalls int
	workspaces []runtime.WorkspaceConfig
	profiles   []runtime.ModelProfileConfig
	run        func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)
}

func (p *executorProbe) snapshot() (int, []runtime.WorkspaceConfig, []runtime.ModelProfileConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.agentCalls,
		append([]runtime.WorkspaceConfig(nil), p.workspaces...),
		append([]runtime.ModelProfileConfig(nil), p.profiles...)
}

func (p *executorProbe) setRun(run func(ctx context.Context, input string, sink agentcore.EventSink) (string, error)) {
	p.mu.Lock()
	p.run = run
	p.mu.Unlock()
}

// executorFixture is a real RunService + profile store on an isolated
// AWAKEN_HOME, wired exactly like cmd/awaken-core assembles the executor.
type executorFixture struct {
	home      string
	service   *runtime.RunService
	profiles  *modelprofile.UserStore
	executor  *RunServiceExecutor
	probe     *executorProbe
	workspace string
	project   runtime.Project
}

const executorTestSecret = "test-secret-DoNotLeak"

func newExecutorFixture(t *testing.T) *executorFixture {
	t.Helper()
	// Not t.TempDir(): the run pipeline's deferred transcript flush can
	// outlive the test by a few milliseconds, and a RemoveAll racing it
	// fails the test. The repo's own store fallback leaks the same way.
	home, err := os.MkdirTemp("", "awaken-sched-exec-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Setenv("AWAKEN_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	models := `{"version":1,"profiles":[{"id":"prof-a","providerId":"deepseek","modelId":"deepseek-v4-pro","enabled":true,"isDefault":true,"credentialRevision":1}]}`
	if err := os.WriteFile(filepath.Join(home, "config", "models.json"), []byte(models), 0o600); err != nil {
		t.Fatalf("write models.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("AWAKEN_API_KEY_prof-a="+executorTestSecret+"\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	probe := &executorProbe{}
	probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "ok", nil
	})
	service := runtime.NewRunServiceWithModelProfiles(
		func(workspace runtime.WorkspaceConfig, threadID string, profile *runtime.ModelProfileConfig) *agentcore.Agent {
			probe.mu.Lock()
			probe.agentCalls++
			probe.workspaces = append(probe.workspaces, workspace)
			if profile != nil {
				probe.profiles = append(probe.profiles, *profile)
			}
			probe.mu.Unlock()
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
		func(agent *agentcore.Agent) runtime.Executor {
			return runtime.ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				probe.mu.Lock()
				run := probe.run
				probe.mu.Unlock()
				return run(ctx, input, sink)
			})
		},
	)
	t.Cleanup(func() { _ = service.FlushTranscripts() })

	workspace := t.TempDir()
	project, err := service.ProjectService().CreateProject([]runtime.ProjectFolder{{Path: workspace, IsPrimary: true}}, "定时项目")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	profiles := modelprofile.NewUserStore(home)
	return &executorFixture{
		home:      home,
		service:   service,
		profiles:  profiles,
		executor:  NewRunServiceExecutor(service, profiles),
		probe:     probe,
		workspace: workspace,
		project:   project,
	}
}

func (f *executorFixture) agentTask(name string) Task {
	return Task{
		TaskID:    "sched_" + strings.ReplaceAll(name, " ", "_"),
		Version:   1,
		Name:      name,
		Enabled:   true,
		ProjectID: f.project.ProjectID,
		Schedule:  Schedule{Kind: ScheduleKindCron, Cron: "* * * * *", Timezone: "UTC"},
		Action:    Action{Kind: ActionKindAgent, Content: "执行定时任务提示词"},
		Execution: ExecutionOptions{TimeoutSeconds: 600, MisfireGraceSeconds: 300},
	}
}

func (f *executorFixture) reminderTask(name string) Task {
	task := f.agentTask(name)
	task.Action = Action{Kind: ActionKindReminder, Content: "固定提醒正文"}
	return task
}

func waitForServiceRunStatus(t *testing.T, service *runtime.RunService, runID string, status runtime.RunStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run, err := service.GetRun(runID)
		if err == nil && run.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, _ := service.GetRun(runID)
	t.Fatalf("run %s status = %v, want %v", runID, run.Status, status)
}

// waitForAllThreadsIdle waits until no thread has an active run; a run's
// terminal status and bus close both land slightly before clearActiveRun.
func waitForAllThreadsIdle(t *testing.T, service *runtime.RunService) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		busy := false
		for _, snapshot := range service.ListThreads(runtime.ThreadListViewActive) {
			if snapshot.ActiveRunID != "" {
				busy = true
			}
		}
		if !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("threads still have active runs")
}

// assertNoSecretOnDisk scans the business root (except the .env credential
// file itself, which is the legitimate owner of the key) for the API key.
func assertNoSecretOnDisk(t *testing.T, home string) {
	t.Helper()
	err := filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if info.Name() == ".env" {
			return nil
		}
		if info.Size() > 32<<20 {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), executorTestSecret) {
			t.Errorf("API key leaked into %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan business root: %v", err)
	}
}

func TestExecutorAgentDedicatedThreadAndProfile(t *testing.T) {
	f := newExecutorFixture(t)
	task := f.agentTask("每日晨报")

	runID, threadID, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("ExecuteAgent: %+v", execErr)
	}
	if runID == "" || threadID == "" {
		t.Fatalf("empty run/thread: %q %q", runID, threadID)
	}

	// The profile resolved at trigger time reached the agent in memory.
	calls, workspaces, profiles := f.probe.snapshot()
	if calls != 1 || len(profiles) != 1 {
		t.Fatalf("agent calls = %d, profiles = %d", calls, len(profiles))
	}
	if profiles[0].APIKey != executorTestSecret {
		t.Fatalf("resolved profile did not reach the agent")
	}
	if profiles[0].ProfileID != "prof-a" || profiles[0].ModelID != "deepseek-v4-pro" {
		t.Fatalf("unexpected profile: %+v", profiles[0])
	}
	if len(workspaces) != 1 || workspaces[0].WorkspaceRoot != f.workspace {
		t.Fatalf("workspace = %+v, want project folder %s", workspaces, f.workspace)
	}

	// The second execution of the same task reuses the dedicated thread.
	waitForAllThreadsIdle(t, f.service)
	task.ThreadID = threadID
	_, threadID2, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_2"})
	if execErr != nil {
		t.Fatalf("second ExecuteAgent: %+v", execErr)
	}
	if threadID2 != threadID {
		t.Fatalf("dedicated thread not reused: %q then %q", threadID, threadID2)
	}

	// The thread is titled by the task and carries the scheduled source.
	var found *runtime.ThreadSnapshot
	for _, snapshot := range f.service.ListThreads(runtime.ThreadListViewActive) {
		if snapshot.ThreadID == threadID {
			copy := snapshot
			found = &copy
		}
	}
	if found == nil {
		t.Fatalf("thread %s not listed", threadID)
	}
	if found.Label != "每日晨报" || found.Source != runtime.ScheduledClientSource {
		t.Fatalf("thread = %+v", found)
	}

	assertNoSecretOnDisk(t, f.home)
}

func TestExecutorThreadBusySkipped(t *testing.T) {
	f := newExecutorFixture(t)
	task := f.agentTask("繁忙任务")
	_, threadID, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("ExecuteAgent: %+v", execErr)
	}
	waitForAllThreadsIdle(t, f.service)

	// A user run holding the dedicated thread turns the trigger into a skip.
	release := make(chan struct{})
	f.probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		select {
		case <-release:
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	defer close(release)
	userRun, err := f.service.StartRun(runtime.StartRunRequest{
		Input:        "user run",
		ThreadID:     threadID,
		Workspace:    runtime.WorkspaceConfig{WorkspaceRoot: f.workspace},
		ModelProfile: mustResolveProfile(t, f.profiles, ""),
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	waitForServiceRunStatus(t, f.service, userRun.RunID, runtime.RunStatusRunning)

	task.ThreadID = threadID
	_, _, execErr = f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_2"})
	if execErr == nil {
		t.Fatalf("busy thread must skip, got success")
	}
	if execErr.Code != ErrorCodeTaskThreadBusy || !execErr.Skipped {
		t.Fatalf("execErr = %+v, want skipped/%s", execErr, ErrorCodeTaskThreadBusy)
	}
}

func mustResolveProfile(t *testing.T, store *modelprofile.UserStore, id string) *runtime.ModelProfileConfig {
	t.Helper()
	resolved, err := store.Resolve(id)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", id, err)
	}
	return runtime.ModelProfileConfigFromResolved(resolved)
}

func TestExecutorProjectUnavailable(t *testing.T) {
	f := newExecutorFixture(t)

	// Unknown project.
	task := f.agentTask("无项目")
	task.ProjectID = "project_missing"
	if _, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"}); execErr == nil || execErr.Code != ErrorCodeProjectUnavailable {
		t.Fatalf("unknown project execErr = %+v, want %s", execErr, ErrorCodeProjectUnavailable)
	}

	// Project folder deleted from disk: fail closed, never a stale root.
	if err := os.RemoveAll(f.workspace); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	task = f.agentTask("路径丢失")
	if _, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_2"}); execErr == nil || execErr.Code != ErrorCodeProjectUnavailable {
		t.Fatalf("deleted folder execErr = %+v, want %s", execErr, ErrorCodeProjectUnavailable)
	}

	// Removed project.
	workspace2 := t.TempDir()
	project2, err := f.service.ProjectService().CreateProject([]runtime.ProjectFolder{{Path: workspace2, IsPrimary: true}}, "将被移除")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := f.service.ProjectService().DetachProject(project2.ProjectID, "detach-1"); err != nil {
		t.Fatalf("DetachProject: %v", err)
	}
	task = f.agentTask("已移除项目")
	task.ProjectID = project2.ProjectID
	if _, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_3"}); execErr == nil || execErr.Code != ErrorCodeProjectUnavailable {
		t.Fatalf("removed project execErr = %+v, want %s", execErr, ErrorCodeProjectUnavailable)
	}
}

func TestExecutorFolderlessProjectSessionWorkspace(t *testing.T) {
	f := newExecutorFixture(t)
	project, err := f.service.ProjectService().CreateProject(nil, "零文件夹")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task := f.agentTask("零文件夹任务")
	task.ProjectID = project.ProjectID

	_, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("ExecuteAgent: %+v", execErr)
	}
	_, workspaces, _ := f.probe.snapshot()
	if len(workspaces) != 1 {
		t.Fatalf("workspaces = %+v", workspaces)
	}
	root := workspaces[0].WorkspaceRoot
	if !strings.Contains(root, "sessions") || !strings.Contains(root, project.ProjectID) {
		t.Fatalf("session workspace root = %q, want sessions/<projectID>", root)
	}
}

func TestExecutorProfileUnavailable(t *testing.T) {
	f := newExecutorFixture(t)

	// An explicitly selected profile that is not ready fails closed.
	task := f.agentTask("显式失效")
	task.Execution.ProfileID = "prof-missing"
	_, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr == nil || execErr.Code != ErrorCodeModelProfileUnavailable {
		t.Fatalf("missing profile execErr = %+v, want %s", execErr, ErrorCodeModelProfileUnavailable)
	}

	// The profile is re-resolved on every trigger: working, then broken.
	task = f.agentTask("重新解析")
	if _, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_2"}); execErr != nil {
		t.Fatalf("ExecuteAgent with default profile: %+v", execErr)
	}
	broken := `{"version":1,"profiles":[{"id":"prof-a","providerId":"deepseek","modelId":"deepseek-v4-pro","enabled":false,"isDefault":true,"credentialRevision":1}]}`
	if err := os.WriteFile(filepath.Join(f.home, "config", "models.json"), []byte(broken), 0o600); err != nil {
		t.Fatalf("rewrite models.json: %v", err)
	}
	_, _, execErr = f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_3"})
	if execErr == nil || execErr.Code != ErrorCodeModelProfileUnavailable {
		t.Fatalf("disabled profile execErr = %+v, want %s", execErr, ErrorCodeModelProfileUnavailable)
	}
}

func TestExecutorReminderWritesNotice(t *testing.T) {
	f := newExecutorFixture(t)
	task := f.reminderTask("喝水提醒")

	threadID, execErr := f.executor.ExecuteReminder(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("ExecuteReminder: %+v", execErr)
	}
	if threadID == "" {
		t.Fatalf("empty thread")
	}
	if calls, _, _ := f.probe.snapshot(); calls != 0 {
		t.Fatalf("reminder created %d agents, want 0 (no model call)", calls)
	}

	transcript, err := f.service.GetThreadTranscript(threadID)
	if err != nil {
		t.Fatalf("GetThreadTranscript: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(transcript.Runs))
	}
	kinds := make([]agentcore.SemanticKind, 0, 3)
	var notice *agentcore.SemanticRecord
	for i := range transcript.Runs[0].SemanticRecords {
		record := &transcript.Runs[0].SemanticRecords[i]
		kinds = append(kinds, record.Kind)
		if record.Kind == agentcore.SemanticKindScheduledNotice {
			notice = record
		}
	}
	if len(kinds) != 3 || kinds[0] != agentcore.SemanticKindRunStarted || kinds[1] != agentcore.SemanticKindScheduledNotice || kinds[2] != agentcore.SemanticKindRunTerminal {
		t.Fatalf("journal kinds = %v", kinds)
	}
	if notice == nil || notice.Payload["content"] != "固定提醒正文" || notice.Payload["execution_id"] != "exec_1" {
		t.Fatalf("notice = %+v", notice)
	}
	assertNoSecretOnDisk(t, f.home)
}

func TestExecutorAgentRecreatesDeletedDedicatedThread(t *testing.T) {
	f := newExecutorFixture(t)
	task := f.agentTask("重建 Agent 线程")

	_, deletedThreadID, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("first ExecuteAgent: %+v", execErr)
	}
	waitForAllThreadsIdle(t, f.service)
	if err := f.service.DeleteThread(deletedThreadID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}

	task.ThreadID = deletedThreadID
	_, recreatedThreadID, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_2"})
	if execErr != nil {
		t.Fatalf("ExecuteAgent after thread deletion: %+v", execErr)
	}
	if recreatedThreadID == "" || recreatedThreadID == deletedThreadID {
		t.Fatalf("recreated thread = %q, deleted thread = %q", recreatedThreadID, deletedThreadID)
	}
}

func TestExecutorReminderRecreatesDeletedDedicatedThread(t *testing.T) {
	f := newExecutorFixture(t)
	task := f.reminderTask("重建提醒线程")

	deletedThreadID, execErr := f.executor.ExecuteReminder(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("first ExecuteReminder: %+v", execErr)
	}
	if err := f.service.DeleteThread(deletedThreadID); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}

	task.ThreadID = deletedThreadID
	recreatedThreadID, execErr := f.executor.ExecuteReminder(context.Background(), task, Execution{ExecutionID: "exec_2"})
	if execErr != nil {
		t.Fatalf("ExecuteReminder after thread deletion: %+v", execErr)
	}
	if recreatedThreadID == "" || recreatedThreadID == deletedThreadID {
		t.Fatalf("recreated thread = %q, deleted thread = %q", recreatedThreadID, deletedThreadID)
	}
}

func TestExecutorInteractionGates(t *testing.T) {
	gates := []struct {
		name      string
		eventType agentcore.AwakenEventType
		want      string
	}{
		{"ask_user", agentcore.UserQuestionRequestedType, "user_question"},
		{"acp_permission", agentcore.ACPPermissionRequestedEventType, "acp_permission"},
		{"external_interaction", agentcore.ExternalInteractionRequestedType, "external_interaction"},
		{"auth_required", agentcore.AuthRequiredEventType, "auth_required"},
	}
	for _, gate := range gates {
		t.Run(gate.name, func(t *testing.T) {
			f := newExecutorFixture(t)
			f.probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				sink.Emit(agentcore.AwakenEvent{Type: gate.eventType})
				<-ctx.Done()
				return "", ctx.Err()
			})
			task := f.agentTask("门禁任务 " + gate.name)
			runID, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
			if execErr == nil {
				t.Fatalf("interaction gate must fail fast, got success")
			}
			if execErr.Code != ErrorCodeScheduledInteractionRequired {
				t.Fatalf("code = %q, want %q", execErr.Code, ErrorCodeScheduledInteractionRequired)
			}
			if !strings.Contains(execErr.Message, gate.want) {
				t.Fatalf("message = %q, want gate type %q", execErr.Message, gate.want)
			}
			if strings.Contains(execErr.Message, task.Action.Content) {
				t.Fatalf("prompt leaked into error message: %q", execErr.Message)
			}
			waitForServiceRunStatus(t, f.service, runID, runtime.RunStatusCancelled)
		})
	}
}

func TestExecutorReportsRunAndThreadBeforeWaitingForTerminal(t *testing.T) {
	f := newExecutorFixture(t)
	release := make(chan struct{})
	f.probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		select {
		case <-release:
			return "done", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	type startedIdentity struct {
		runID    string
		threadID string
	}
	started := make(chan startedIdentity, 1)
	f.executor.SetExecutionStartedHandler(func(task Task, exec Execution, runID, threadID string) {
		if task.TaskID == "sched_sidebar-visible" && exec.ExecutionID == "exec_1" {
			started <- startedIdentity{runID: runID, threadID: threadID}
		}
	})
	task := f.agentTask("sidebar-visible")
	done := make(chan *ExecError, 1)
	go func() {
		_, _, execErr := f.executor.ExecuteAgent(
			context.Background(),
			task,
			Execution{ExecutionID: "exec_1"},
		)
		done <- execErr
	}()

	select {
	case identity := <-started:
		if identity.runID == "" || identity.threadID == "" {
			t.Fatalf("reported identity = %+v", identity)
		}
		select {
		case execErr := <-done:
			t.Fatalf("ExecuteAgent returned before release: %+v", execErr)
		default:
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("executor did not report started identity")
	}

	close(release)
	if execErr := <-done; execErr != nil {
		t.Fatalf("ExecuteAgent: %+v", execErr)
	}
}

func TestExecutorTimeout(t *testing.T) {
	f := newExecutorFixture(t)
	timer := NewManualTimer()
	f.executor.NewTimer = func(time.Duration) Timer { return timer }
	f.probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	task := f.agentTask("超时任务")
	task.Execution.TimeoutSeconds = 60
	type outcome struct {
		runID   string
		execErr *ExecError
	}
	done := make(chan outcome, 1)
	go func() {
		runID, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
		done <- outcome{runID: runID, execErr: execErr}
	}()
	// The run must be started before the timeout can fire.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if calls, _, _ := f.probe.snapshot(); calls == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !timer.Fire(time.Now()) {
		t.Fatalf("timeout timer not armed")
	}
	result := <-done
	if result.execErr == nil || result.execErr.Code != ErrorCodeExecutionTimeout {
		t.Fatalf("execErr = %+v, want %s", result.execErr, ErrorCodeExecutionTimeout)
	}
	waitForServiceRunStatus(t, f.service, result.runID, runtime.RunStatusCancelled)
}

func TestExecutorCancel(t *testing.T) {
	f := newExecutorFixture(t)
	started := make(chan struct{})
	f.probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	task := f.agentTask("取消任务")
	type outcome struct {
		runID   string
		execErr *ExecError
	}
	done := make(chan outcome, 1)
	go func() {
		runID, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
		done <- outcome{runID: runID, execErr: execErr}
	}()

	// ActiveRunID is cleared as soon as the cancelled run finishes, so the
	// id has to be latched while the probe is still blocked. registerActive
	// happens after StartScheduledRun returns and can lag this signal by one
	// statement; Cancel before that is a no-op and must be retried.
	select {
	case <-started:
	case result := <-done:
		t.Fatalf("ExecuteAgent finished before the run blocked: runID=%s execErr=%+v", result.runID, result.execErr)
	case <-time.After(15 * time.Second):
		t.Fatal("run never started")
	}

	deadline := time.Now().Add(15 * time.Second)
	var runID string
	for {
		if runID == "" {
			for _, snapshot := range f.service.ListThreads(runtime.ThreadListViewActive) {
				if snapshot.ActiveRunID != "" {
					runID = snapshot.ActiveRunID
					break
				}
			}
		}
		if runID != "" {
			run, err := f.service.GetRun(runID)
			if err == nil && run.CancelRequested {
				break
			}
		}
		select {
		case result := <-done:
			if result.execErr != nil && result.execErr.Code == ErrorCodeRunCancelled && result.execErr.Cancelled {
				waitForServiceRunStatus(t, f.service, result.runID, runtime.RunStatusCancelled)
				return
			}
			t.Fatalf("ExecuteAgent finished before cancel: runID=%s execErr=%+v", result.runID, result.execErr)
		default:
		}
		if time.Now().After(deadline) {
			run, err := f.service.GetRun(runID)
			t.Fatalf("run never took the cancel: runID=%q err=%v status=%s cancelRequested=%v", runID, err, run.Status, run.CancelRequested)
		}
		f.executor.Cancel(task.TaskID)
		time.Sleep(10 * time.Millisecond)
	}
	result := <-done
	if result.execErr == nil || result.execErr.Code != ErrorCodeRunCancelled || !result.execErr.Cancelled {
		t.Fatalf("execErr = %+v, want cancelled/%s", result.execErr, ErrorCodeRunCancelled)
	}
	waitForServiceRunStatus(t, f.service, result.runID, runtime.RunStatusCancelled)
}

func TestExecutorRunFailureSanitized(t *testing.T) {
	f := newExecutorFixture(t)
	f.probe.setRun(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
		return "", context.DeadlineExceeded
	})
	task := f.agentTask("失败任务")
	_, _, execErr := f.executor.ExecuteAgent(context.Background(), task, Execution{ExecutionID: "exec_1"})
	if execErr == nil || execErr.Code != ErrorCodeRunFailed {
		t.Fatalf("execErr = %+v, want %s", execErr, ErrorCodeRunFailed)
	}
	if execErr.Message != "agent run failed" {
		t.Fatalf("failure message = %q, want sanitized static text", execErr.Message)
	}
	if strings.Contains(execErr.Message, task.Action.Content) {
		t.Fatalf("prompt leaked into error message: %q", execErr.Message)
	}
	assertNoSecretOnDisk(t, f.home)
}

func TestDeleteTaskKeepsDedicatedThread(t *testing.T) {
	f := newExecutorFixture(t)
	repo, err := OpenFileRepository(t.TempDir(), RealClock{})
	if err != nil {
		t.Fatalf("OpenFileRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	service := NewService(repo, Validator{}, f.executor)

	created, err := service.Create(f.agentTask("将被删除的任务"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, threadID, execErr := f.executor.ExecuteAgent(context.Background(), created, Execution{ExecutionID: "exec_1"})
	if execErr != nil {
		t.Fatalf("ExecuteAgent: %+v", execErr)
	}
	waitForAllThreadsIdle(t, f.service)

	if err := service.Delete(created.TaskID, created.Version, "exec-del-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// Deleting the task tombstones the definition only; the dedicated thread
	// and its artifacts stay.
	transcript, err := f.service.GetThreadTranscript(threadID)
	if err != nil {
		t.Fatalf("dedicated thread deleted with the task: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("transcript runs = %d, want 1", len(transcript.Runs))
	}
}
