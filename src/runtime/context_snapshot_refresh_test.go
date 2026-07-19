package runtime

import (
	"context"
	"testing"

	"awaken/src/agentcore"
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

func newContextRefreshTestService() *RunService {
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
			return agentcore.NewAgentWithThreadID(agentcore.AgentConfig{
				ModelConfig:       model,
				Workspace:         agentcore.WorkspaceConfig{Root: workspace.WorkspaceRoot, WritableRoots: workspace.WritableRoots},
				ContextEpochStore: NewContextEpochStore(),
			}, threadID)
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
