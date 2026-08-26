package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"awaken/src/agentcore"
)

type ContextSnapshotRefreshResult struct {
	Snapshot  agentcore.ContextUsageSnapshot
	Persisted bool
}

// RefreshThreadContextSnapshot rebuilds the current-next-request projection
// for a durable thread using the currently selected model profile. It never
// calls the provider and never rewrites historical provider usage.
func (s *RunService) RefreshThreadContextSnapshot(ctx context.Context, threadID string, profile *ModelProfileConfig) (ContextSnapshotRefreshResult, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ContextSnapshotRefreshResult{}, ErrThreadNotFound
	}
	if profile == nil || strings.TrimSpace(profile.ModelID) == "" {
		return ContextSnapshotRefreshResult{}, ErrMissingModelProfile
	}
	if s.journals == nil {
		return ContextSnapshotRefreshResult{}, ErrPersistenceUnavailable
	}

	// Serialize replay projections so two renderer loads cannot both append the
	// same regenerated snapshot after reading the same journal tail.
	s.contextSnapshotMu.Lock()
	defer s.contextSnapshotMu.Unlock()

	s.mu.RLock()
	if thread, ok := s.threads[threadID]; ok {
		if thread.deletionRequested {
			s.mu.RUnlock()
			return ContextSnapshotRefreshResult{}, ErrThreadNotFound
		}
		if thread.activeRunID != "" {
			s.mu.RUnlock()
			// The active run owns the live current projection and will persist it
			// at its normal provider-turn boundary.
			return ContextSnapshotRefreshResult{}, nil
		}
	}
	s.mu.RUnlock()

	meta, restored, err := s.threadStore.RestoreThread(threadID)
	if err != nil {
		return ContextSnapshotRefreshResult{}, err
	}
	agent := s.createAgent(meta.Snapshot.Workspace, threadID, profile)
	if agent == nil {
		return ContextSnapshotRefreshResult{}, fmt.Errorf("agent is not configured")
	}
	agent.GetConversation().Context.SetMessages(restored.Messages)
	cfg := agent.GetConfig()
	cfg.SkillContextBlocks = append([]agentcore.ContextBlock(nil), restored.SkillContextBlocks...)
	// The replay projection must assemble the same provider-visible request
	// sources as StartRun. A projection built without the AGENTS instruction
	// snapshot carries an empty instruction digest, which the epoch store
	// would treat as a real change and fabricate an `agents_instructions_changed`
	// rotation, silently invalidating the provider usage shown by Desktop.
	instructionSnapshot := s.instructionLoader.Load(meta.Snapshot.Workspace.WorkspaceRoot, meta.Snapshot.Workspace.SessionWorkspace)
	cfg.InstructionSnapshot = &instructionSnapshot
	agent.Reconfigure(cfg)
	agent.RefreshProjectedRunAvailabilityWithReason(ctx, "thread_replay", s.projectedComputerEnabled(ctx, profile))

	snapshot, epoch, err := agent.ProjectCurrentContextUsage(ctx)
	if err != nil {
		return ContextSnapshotRefreshResult{}, err
	}

	path := s.journals.pathFn(threadID)
	records, _, readErr := loadSemanticRecordsForThread(path)
	if readErr != nil {
		return ContextSnapshotRefreshResult{}, readErr
	}
	// restored.RunIDs is derived from journal order and therefore owns replay
	// ordering. File metadata can lag after recovery and must only be a fallback;
	// otherwise a fresh snapshot attached to an older run is overwritten by a
	// later stale snapshot during Renderer replay.
	runID := latestRunIDForContextRefresh(meta, restored)
	if runID == "" {
		return ContextSnapshotRefreshResult{}, fmt.Errorf("thread %s has no durable run", threadID)
	}
	sessionID, turnID := latestSemanticIdentity(records, runID)
	previous, previousRunID, hasPrevious := latestContextUsageSnapshot(records)

	toAppend := make([]agentcore.SemanticRecord, 0, 2)
	if epoch != nil && ((epoch.State != "" && epoch.State != "unchanged") || !journalContainsContextEpoch(records, *epoch)) {
		toAppend = append(toAppend, agentcore.ContextEpochSemanticRecord(threadID, runID, turnID, sessionID, *epoch))
	}
	if shouldPersistContextUsageSnapshot(previous, previousRunID, hasPrevious, snapshot, runID) {
		toAppend = append(toAppend, agentcore.ContextSnapshotSemanticRecord(threadID, runID, turnID, sessionID, snapshot))
	}
	if len(toAppend) == 0 {
		return ContextSnapshotRefreshResult{Snapshot: snapshot}, nil
	}

	recorder, err := s.journals.OpenRecorder(threadID)
	if err != nil {
		return ContextSnapshotRefreshResult{}, err
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if journal, ok := recorder.(*ExecutionJournal); ok {
		if err := ensureThreadJournalRecord(persistCtx, journal, threadID, meta.Snapshot.Label); err != nil {
			return ContextSnapshotRefreshResult{}, err
		}
	}
	for _, record := range toAppend {
		if err := recorder.Append(persistCtx, record); err != nil {
			return ContextSnapshotRefreshResult{}, err
		}
	}
	if err := recorder.Flush(persistCtx); err != nil {
		return ContextSnapshotRefreshResult{}, err
	}
	return ContextSnapshotRefreshResult{Snapshot: snapshot, Persisted: true}, nil
}

// projectedComputerEnabled resolves the same product gates used by StartRun
// without constructing a run-bound Binding or Runtime. The global Host probe
// reports driver/TCC readiness for inactive-thread projections.
func (s *RunService) projectedComputerEnabled(ctx context.Context, profile *ModelProfileConfig) bool {
	if s == nil || profile == nil || !profile.SupportsVision {
		return false
	}
	s.mu.RLock()
	settings := s.computerSettings
	probe := s.computerHostProbe
	s.mu.RUnlock()
	if !settings.Enabled || probe == nil {
		return false
	}
	status, err := probe.Probe(ctx)
	return err == nil && status.Ready()
}

func latestRunIDForContextRefresh(meta storedThreadRecord, restored restoredTranscript) string {
	if len(restored.RunIDs) > 0 {
		if runID := strings.TrimSpace(restored.RunIDs[len(restored.RunIDs)-1]); runID != "" {
			return runID
		}
	}
	return strings.TrimSpace(meta.Snapshot.RunID)
}

func latestSemanticIdentity(records []agentcore.SemanticRecord, runID string) (sessionID, turnID string) {
	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		if record.RunID != runID {
			continue
		}
		if sessionID == "" {
			sessionID = record.SessionID
		}
		if turnID == "" {
			turnID = record.TurnID
		}
		if sessionID != "" && turnID != "" {
			break
		}
	}
	return sessionID, turnID
}

func latestContextUsageSnapshot(records []agentcore.SemanticRecord) (agentcore.ContextUsageSnapshot, string, bool) {
	for index := len(records) - 1; index >= 0; index-- {
		if records[index].Kind != agentcore.SemanticKindContextSnapshot {
			continue
		}
		encoded, err := json.Marshal(records[index].Payload)
		if err != nil {
			continue
		}
		var snapshot agentcore.ContextUsageSnapshot
		if json.Unmarshal(encoded, &snapshot) == nil {
			return snapshot, records[index].RunID, true
		}
	}
	return agentcore.ContextUsageSnapshot{}, "", false
}

func equivalentContextUsageSnapshot(left, right agentcore.ContextUsageSnapshot) bool {
	left.SnapshotID, right.SnapshotID = "", ""
	left.GeneratedAt, right.GeneratedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(left, right)
}

func shouldPersistContextUsageSnapshot(previous agentcore.ContextUsageSnapshot, previousRunID string, hasPrevious bool, current agentcore.ContextUsageSnapshot, currentRunID string) bool {
	return !hasPrevious || previousRunID != currentRunID || !equivalentContextUsageSnapshot(previous, current)
}

func journalContainsContextEpoch(records []agentcore.SemanticRecord, receipt agentcore.ContextEpochReceipt) bool {
	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		if record.Kind != agentcore.SemanticKindContextEpoch {
			continue
		}
		if stringPayload(record.Payload, "epoch_id") == receipt.EpochID && uint64(intPayload(record.Payload, "baseline_sequence")) == receipt.BaselineSequence {
			return true
		}
	}
	return false
}
