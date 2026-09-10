package runtime

import (
	"context"
	"errors"
	"os"

	"awaken/src/agentcore"
)

// Never open a recovery writer behind the current thread's journal owner.
// In-process projections use that owner's snapshot; cold reads alone perform
// orphan recovery before acquiring a new writer.
func (s *RunService) restoreOwnedThread(threadID string) (storedThreadRecord, restoredTranscript, error) {
	s.mu.RLock()
	thread := s.threads[threadID]
	var meta storedThreadRecord
	if thread != nil {
		meta = storedThreadRecord{Snapshot: thread.snapshot(), ModelProfileKey: thread.modelProfileKey}
	}
	s.mu.RUnlock()
	if thread != nil && s.journals != nil {
		if journal, ok := s.journals.Get(threadID); ok {
			records, err := journal.SnapshotRecords()
			if err != nil {
				return meta, restoredTranscript{}, err
			}
			return meta, restoredTranscriptFromSemantic(activeSemanticRecords(records)), nil
		}
	}
	return s.threadStore.RestoreThread(threadID)
}

// restoreThreadFromDurableRecord assembles the in-memory restoration facts
// from FileStore metadata and its canonical journal.
func restoreThreadFromDurableRecord(record storedThreadRecord, path, threadID string) (storedThreadRecord, restoredTranscript, error) {
	schema, err := DetectJournalSchema(path)
	if err != nil {
		if errors.Is(err, ErrThreadNotFound) || errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return storedThreadRecord{}, restoredTranscript{}, ErrThreadNotFound
		}
		return storedThreadRecord{}, restoredTranscript{}, err
	}
	var restored restoredTranscript
	if schema == agentcore.SemanticSchemaVersion {
		records, loadErr := loadAndRecoverSemanticRecords(path, threadID)
		if loadErr != nil {
			return storedThreadRecord{}, restoredTranscript{}, loadErr
		}
		restored = restoredTranscriptFromSemantic(activeSemanticRecords(records))
	} else {
		restored, err = loadRestoredTranscriptAtPath(path)
		if err != nil {
			if errors.Is(err, ErrThreadNotFound) || errors.Is(err, os.ErrNotExist) {
				return storedThreadRecord{}, restoredTranscript{}, ErrThreadNotFound
			}
			return storedThreadRecord{}, restoredTranscript{}, err
		}
	}
	if record.Snapshot.Label == "" {
		record.Snapshot.Label = restored.Label
	}
	if record.Snapshot.CreatedAt.IsZero() {
		record.Snapshot.CreatedAt = restored.CreatedAt
	}
	if record.Snapshot.UpdatedAt.IsZero() {
		record.Snapshot.UpdatedAt = restored.UpdatedAt
	}
	if schema == agentcore.SemanticSchemaVersion {
		record.Snapshot.RunID = ""
		if len(restored.RunIDs) > 0 {
			record.Snapshot.RunID = restored.RunIDs[len(restored.RunIDs)-1]
		}
	}
	return record, restored, nil
}

// threadTranscriptFromDurableRecord assembles the public transcript view from
// FileStore metadata and its canonical journal.
func threadTranscriptFromDurableRecord(record storedThreadRecord, path, threadID string) (ThreadTranscript, error) {
	schema, err := DetectJournalSchema(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}

	if schema == agentcore.SemanticSchemaVersion {
		records, loadErr := loadAndRecoverSemanticRecords(path, threadID)
		if loadErr != nil {
			return ThreadTranscript{}, loadErr
		}
		records = activeSemanticRecords(records)
		restored := restoredTranscriptFromSemantic(records)
		runs := durableRunsFromSemanticRecords(threadID, records)
		snapshot := record.Snapshot
		if snapshot.Label == "" {
			snapshot.Label = restored.Label
		}
		if snapshot.CreatedAt.IsZero() {
			snapshot.CreatedAt = restored.CreatedAt
		}
		if snapshot.UpdatedAt.IsZero() {
			snapshot.UpdatedAt = restored.UpdatedAt
		}
		snapshot.RunID = ""
		if len(restored.RunIDs) > 0 {
			snapshot.RunID = restored.RunIDs[len(restored.RunIDs)-1]
		}
		ordered := make([]ThreadTranscriptRun, 0, len(restored.RunIDs))
		for _, runID := range restored.RunIDs {
			if durable, ok := runs[runID]; ok {
				ordered = append(ordered, durable)
			}
		}
		return ThreadTranscript{Thread: snapshot, Runs: ordered}, nil
	}

	// V1 JSONL remains readable only inside the active FileStore generation.
	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}
	semanticRecords, _ := (LegacyTranscriptAdapter{}).ReadFile(path)
	runs := durableRunsFromSemanticRecords(threadID, semanticRecords)
	if len(runs) == 0 {
		durableRuns, loadErr := loadDurableTranscriptRunsAtPath(path, threadID)
		if loadErr != nil {
			if errors.Is(loadErr, os.ErrNotExist) {
				return ThreadTranscript{}, ErrThreadNotFound
			}
			return ThreadTranscript{}, loadErr
		}
		runs = durableRuns
	}
	ordered := make([]ThreadTranscriptRun, 0, len(restored.RunIDs))
	for _, runID := range restored.RunIDs {
		if durable, ok := runs[runID]; ok {
			ordered = append(ordered, durable)
		}
	}
	snapshot := record.Snapshot
	if snapshot.Label == "" {
		snapshot.Label = restored.Label
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = restored.CreatedAt
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = restored.UpdatedAt
	}
	if snapshot.RunID == "" && len(restored.RunIDs) > 0 {
		snapshot.RunID = restored.RunIDs[len(restored.RunIDs)-1]
	}
	return ThreadTranscript{Thread: snapshot, Runs: ordered}, nil
}

func loadAndRecoverSemanticRecords(path, threadID string) ([]agentcore.SemanticRecord, error) {
	records, _, err := loadSemanticRecordsForThread(path)
	if err != nil {
		return nil, err
	}
	journal, err := NewExecutionJournal(path, threadID)
	if err != nil {
		return nil, err
	}
	recovered, recoverErr := recoverOrphanRuns(context.Background(), journal, records)
	closeErr := journal.Close()
	if recoverErr != nil {
		return nil, recoverErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return recovered, nil
}
