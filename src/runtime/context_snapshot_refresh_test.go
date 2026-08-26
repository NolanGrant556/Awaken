package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
	"awaken/src/computer"
	"awaken/src/computer/fake"
)

func TestRefreshThreadContextSnapshotPersistsV4ForDurableHistory(t *testing.T) {
	setRuntimeUserDataDir(t)
	workspace := t.TempDir()
	service := newContextRefreshTestService()
	profile := &ModelProfileConfig{
		ProfileID: "profile-deepseek", ProviderID: "deepseek", ModelID: "deepseek-v4-pro",
		BaseURL: "https://api.deepseek.com", Protocol: "openai_chat", APIKey: "secret",
		CredentialRevision: 1, ContextWindow: 1_000_000, MaxTokens: 16_000,
	}
	run, err := service.StartRun(StartRunRequest{
		Input: "旧会话需要继续计算", Workspace: WorkspaceConfig{WorkspaceRoot: workspace}, ModelProfile: profile,
	})
	if err != nil {
		t.Fatalf("start seed run: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)

	records, err := ReadExecutionJournal(service.journals.pathFn(run.ThreadID))
	if err != nil {
		t.Fatalf("read seed journal: %v", err)
	}
	if _, _, ok := latestContextUsageSnapshot(records); ok {
		t.Fatal("seed executor must represent an old thread without a context snapshot")
	}

	restarted := newContextRefreshTestService()
	result, err := restarted.RefreshThreadContextSnapshot(context.Background(), run.ThreadID, profile)
	if err != nil {
		t.Fatalf("refresh durable thread context: %v", err)
	}
	if !result.Persisted {
		t.Fatal("expected regenerated context snapshot to be persisted")
	}
	if result.Snapshot.SchemaVersion != 4 || result.Snapshot.EstimatorID != "awaken_leaf_v4" {
		t.Fatalf("expected current v4 estimator, got %+v", result.Snapshot)
	}
	if result.Snapshot.ModelWindowTokens != 1_000_000 || result.Snapshot.EffectiveInputWindowTokens != 984_000 {
		t.Fatalf("selected model window was not applied: %+v", result.Snapshot)
	}
	total := 0
	for _, contribution := range result.Snapshot.Contributions {
		total += contribution.EstimatedTokens
	}
	if total != result.Snapshot.EstimatedInputTokens {
		t.Fatalf("contributions must balance total: contributions=%d total=%d", total, result.Snapshot.EstimatedInputTokens)
	}

	transcript, err := restarted.GetThreadTranscript(run.ThreadID)
	if err != nil {
		t.Fatalf("replay refreshed thread: %v", err)
	}
	if len(transcript.Runs) != 1 {
		t.Fatalf("expected one restored run, got %d", len(transcript.Runs))
	}
	latest, _, ok := latestContextUsageSnapshot(transcript.Runs[0].SemanticRecords)
	if !ok || latest.SchemaVersion != 4 || len(latest.Contributions) == 0 {
		t.Fatalf("durable replay lost regenerated detail: %+v", latest)
	}

	again, err := restarted.RefreshThreadContextSnapshot(context.Background(), run.ThreadID, profile)
	if err != nil {
		t.Fatalf("repeat context refresh: %v", err)
	}
	if again.Persisted {
		t.Fatal("unchanged replay projection must not append duplicate snapshots")
	}
}

// The transcript-triggered rebuild must assemble the same provider-visible
// request sources as StartRun. A rebuild that drops the AGENTS instruction
// snapshot fabricates an `agents_instructions_changed` epoch rotation without
// any real change, and the next live run then rotates back — silently
// invalidating provider usage display twice per open/run cycle.
func TestRefreshThreadContextSnapshotKeepsInstructionEpochIdentity(t *testing.T) {
	setRuntimeUserDataDir(t)
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, ".awaken", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, []byte("# 项目规则\n输出保持简洁。\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var agents []*agentcore.Agent
	service := newContextRefreshTestServiceWithObserver(func(agent *agentcore.Agent) {
		agents = append(agents, agent)
	})
	profile := &ModelProfileConfig{
		ProfileID: "profile-deepseek", ProviderID: "deepseek", ModelID: "deepseek-v4-pro",
		BaseURL: "https://api.deepseek.com", Protocol: "openai_chat", APIKey: "secret",
		CredentialRevision: 1, ContextWindow: 256_000, MaxTokens: 16_000,
	}
	run, err := service.StartRun(StartRunRequest{
		Input: "seed 会话", Workspace: WorkspaceConfig{WorkspaceRoot: workspace}, ModelProfile: profile,
	})
	if err != nil {
		t.Fatalf("start seed run: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)
	if len(agents) != 1 {
		t.Fatalf("expected one live agent, got %d", len(agents))
	}
	live := agents[0]
	liveConfig := live.GetConfig()
	if liveConfig.InstructionSnapshot == nil || liveConfig.InstructionSnapshot.Digest == "" {
		t.Fatal("live run must carry a non-empty instruction digest")
	}
	_, liveEpoch, err := live.ProjectCurrentContextUsage(context.Background())
	if err != nil {
		t.Fatalf("project live context: %v", err)
	}
	if liveEpoch == nil || liveEpoch.State != "created" {
		t.Fatalf("expected created live epoch, got %+v", liveEpoch)
	}

	result, err := service.RefreshThreadContextSnapshot(context.Background(), run.ThreadID, profile)
	if err != nil {
		t.Fatalf("refresh thread context: %v", err)
	}
	if result.Snapshot.ContextEpochBoundaryReason == "agents_instructions_changed" {
		t.Fatal("thread replay fabricated agents_instructions_changed while AGENTS.md was untouched")
	}
	if result.Snapshot.ContextEpochID != liveEpoch.EpochID {
		t.Fatalf("replay projection switched epochs: live=%s replay=%s reason=%q",
			liveEpoch.EpochID, result.Snapshot.ContextEpochID, result.Snapshot.ContextEpochBoundaryReason)
	}

	// The next fully assembled live projection must also stay on the epoch:
	// no rotate-back cycle.
	_, nextEpoch, err := live.ProjectCurrentContextUsage(context.Background())
	if err != nil {
		t.Fatalf("project post-refresh live context: %v", err)
	}
	if nextEpoch == nil || nextEpoch.State != "unchanged" || nextEpoch.EpochID != liveEpoch.EpochID {
		t.Fatalf("post-refresh live projection rotated the epoch: %+v", nextEpoch)
	}
}

func TestLatestDurableRunPrefersJournalReplayOrder(t *testing.T) {
	restored := restoredTranscript{RunIDs: []string{"run-old", "run-latest"}}
	meta := storedThreadRecord{Snapshot: ThreadSnapshot{RunID: "run-old"}}
	runID := latestRunIDForContextRefresh(meta, restored)
	if runID != "run-latest" {
		t.Fatalf("journal replay order must win over stale metadata, got %q", runID)
	}
}

func TestEquivalentSnapshotIsReattachedWhenLatestRecordBelongsToOlderRun(t *testing.T) {
	snapshot := agentcore.ContextUsageSnapshot{
		SchemaVersion: 4, EstimatorID: "awaken_leaf_v4", Basis: "current_projection",
		EstimatedInputTokens: 42,
	}
	if !shouldPersistContextUsageSnapshot(snapshot, "run-old", true, snapshot, "run-latest") {
		t.Fatal("equivalent snapshot on an older run must be reattached to replay tail")
	}
	if shouldPersistContextUsageSnapshot(snapshot, "run-latest", true, snapshot, "run-latest") {
		t.Fatal("equivalent snapshot already on replay tail must not be duplicated")
	}
}

func TestJournalContainsContextEpochUsesDurableIdentity(t *testing.T) {
	receipt := agentcore.ContextEpochReceipt{EpochID: "epoch-current", BaselineSequence: 3}
	records := []agentcore.SemanticRecord{agentcore.ContextEpochSemanticRecord("thread", "run", "", "", receipt)}
	if !journalContainsContextEpoch(records, receipt) {
		t.Fatal("matching epoch identity should suppress a duplicate replay receipt")
	}
	if journalContainsContextEpoch(records, agentcore.ContextEpochReceipt{EpochID: "epoch-next", BaselineSequence: 4}) {
		t.Fatal("different epoch identity must remain durable")
	}
}

func TestRefreshThreadContextSnapshotPreservesComputerToolsWhenHostReady(t *testing.T) {
	setRuntimeUserDataDir(t)
	workspace := t.TempDir()
	createdAgents := make([]*agentcore.Agent, 0, 2)
	service := newContextRefreshTestServiceWithObserver(func(agent *agentcore.Agent) {
		createdAgents = append(createdAgents, agent)
	})
	service.SetComputerSettings(computer.Settings{Enabled: true})
	service.SetComputerDriverFactory(func(computer.Binding) (computer.Driver, error) {
		return fake.NewDriver(), nil
	})
	service.SetComputerHostProbe(computer.HostProbeFunc(func(context.Context) (computer.HostStatus, error) {
		return computer.HostStatus{
			Available:          true,
			Healthy:            true,
			TCCScreenRecording: true,
			TCCAccessibility:   true,
		}, nil
	}))
	profile := &ModelProfileConfig{
		ProfileID: "profile-vision", ProviderID: "test", ModelID: "vision-model",
		BaseURL: "https://example.invalid", Protocol: "openai_chat", APIKey: "secret",
		SupportsVision: true, SupportsComputerUse: true, ContextWindow: 240_000, MaxTokens: 16_000,
	}
	run, err := service.StartRun(StartRunRequest{
		Input: "保留 Computer 工具", Workspace: WorkspaceConfig{WorkspaceRoot: workspace}, ModelProfile: profile,
	})
	if err != nil {
		t.Fatalf("start seed run: %v", err)
	}
	waitForRunTerminal(t, service, run.RunID)
	if len(createdAgents) != 1 {
		t.Fatalf("expected one live agent before replay, got %d", len(createdAgents))
	}
	liveSnapshot, liveEpoch, err := createdAgents[0].ProjectCurrentContextUsage(context.Background())
	if err != nil {
		t.Fatalf("project live Computer context: %v", err)
	}
	if liveEpoch == nil {
		t.Fatal("expected live Computer context epoch")
	}
	recorder, err := service.journals.OpenRecorder(run.ThreadID)
	if err != nil {
		t.Fatalf("open seed journal: %v", err)
	}
	if err := recorder.Append(context.Background(), agentcore.ContextEpochSemanticRecord(run.ThreadID, run.RunID, "", "", *liveEpoch)); err != nil {
		t.Fatalf("persist live Computer context epoch: %v", err)
	}
	if err := recorder.Append(context.Background(), agentcore.ContextSnapshotSemanticRecord(run.ThreadID, run.RunID, "", "", liveSnapshot)); err != nil {
		t.Fatalf("persist live Computer context snapshot: %v", err)
	}
	if err := recorder.Flush(context.Background()); err != nil {
		t.Fatalf("flush live Computer context: %v", err)
	}

	result, err := service.RefreshThreadContextSnapshot(context.Background(), run.ThreadID, profile)
	if err != nil {
		t.Fatalf("refresh durable thread context: %v", err)
	}
	if result.Snapshot.ContextEpochBoundaryReason == "tool_removed" {
		t.Fatal("thread replay fabricated tool_removed while Computer remained ready")
	}
	if len(createdAgents) < 2 {
		t.Fatalf("expected run and replay agents, got %d", len(createdAgents))
	}
	replayAgent := createdAgents[len(createdAgents)-1]
	cfg := replayAgent.GetConfig()
	if cfg.ComputerRuntime != nil {
		t.Fatal("thread replay must not create a run-bound Computer runtime")
	}
	if cfg.ExecutionEnvironmentSnapshot == nil || !cfg.ExecutionEnvironmentSnapshot.ComputerEnabled {
		t.Fatalf("thread replay lost ready Computer Host state: %+v", cfg.ExecutionEnvironmentSnapshot)
	}
	visible := make(map[string]bool)
	for _, name := range replayAgent.ModelVisibleToolPool() {
		visible[name] = true
	}
	for _, name := range []string{"computer_observe", "computer_act", "computer_verify"} {
		if !visible[name] {
			t.Fatalf("thread replay removed %s while Computer remained ready", name)
		}
	}
}

func TestProjectedComputerEnabledRequiresEveryProductGate(t *testing.T) {
	readyStatus := computer.HostStatus{
		Available: true, Healthy: true, TCCScreenRecording: true, TCCAccessibility: true,
	}
	tests := []struct {
		name            string
		settingsEnabled bool
		supportsVision  bool
		status          computer.HostStatus
		want            bool
	}{
		{name: "ready", settingsEnabled: true, supportsVision: true, status: readyStatus, want: true},
		{name: "settings disabled", supportsVision: true, status: readyStatus},
		{name: "vision unsupported", settingsEnabled: true, status: readyStatus},
		{name: "host unavailable", settingsEnabled: true, supportsVision: true, status: computer.HostStatus{}},
		{name: "host unhealthy", settingsEnabled: true, supportsVision: true, status: computer.HostStatus{Available: true, TCCScreenRecording: true, TCCAccessibility: true}},
		{name: "screen recording missing", settingsEnabled: true, supportsVision: true, status: computer.HostStatus{Available: true, Healthy: true, TCCAccessibility: true}},
		{name: "accessibility missing", settingsEnabled: true, supportsVision: true, status: computer.HostStatus{Available: true, Healthy: true, TCCScreenRecording: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &RunService{computerSettings: computer.Settings{Enabled: tc.settingsEnabled}}
			service.SetComputerHostProbe(computer.HostProbeFunc(func(context.Context) (computer.HostStatus, error) {
				return tc.status, nil
			}))
			got := service.projectedComputerEnabled(context.Background(), &ModelProfileConfig{SupportsVision: tc.supportsVision})
			if got != tc.want {
				t.Fatalf("projectedComputerEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func newContextRefreshTestService() *RunService {
	return newContextRefreshTestServiceWithObserver(nil)
}

func newContextRefreshTestServiceWithObserver(observer func(*agentcore.Agent)) *RunService {
	return NewRunServiceWithModelProfiles(
		func(workspace WorkspaceConfig, threadID string, profile *ModelProfileConfig) *agentcore.Agent {
			model := agentcore.ModelConfig{}
			if profile != nil {
				model = agentcore.ModelConfig{
					APIKey: profile.APIKey, BaseURL: profile.BaseURL, Model: profile.ModelID,
					Protocol: profile.Protocol, ProviderID: profile.ProviderID, ProfileID: profile.ProfileID,
					CredentialRevision: profile.CredentialRevision, MaxTokens: profile.MaxTokens,
					ContextWindow: profile.ContextWindow,
				}
			}
			agent := agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				ModelConfig:       model,
				Workspace:         agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot, WritableRoots: workspace.WritableRoots},
				ContextEpochStore: NewContextEpochStore(),
			}, threadID)
			if observer != nil {
				observer(agent)
			}
			return agent
		},
		func(agent *agentcore.Agent) Executor {
			return ExecutorFunc(func(ctx context.Context, input string, sink agentcore.EventSink) (string, error) {
				agent.GetConversation().UserInput(input)
				agent.GetConversation().AssistantOutput("answer: "+input, nil)
				if err := persistTestRunFacts(ctx, agent, input, "answer: "+input); err != nil {
					return "", err
				}
				return "answer: " + input, nil
			})
		},
	)
}
