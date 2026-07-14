package runtime

import (
	"context"
	"errors"
	"os"

	"awaken/src/agentcore"
)

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
		restored = restoredTranscriptFromSemantic(records)
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
		if snapshot.RunID == "" && len(restored.RunIDs) > 0 {
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
