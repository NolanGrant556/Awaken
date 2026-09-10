package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
)

func TestJournalExternalReplacementRebuildsReadsButFencesOldWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j, err := NewExecutionJournal(path, "thread")
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	r := agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, "thread", "run", "", "", "", map[string]any{"content": "old"})
	if err := j.Append(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if _, err := j.SemanticRecords(1, 1); err != nil {
		t.Fatal(err)
	}
	r.Seq, r.Payload["content"] = 1, "replacement"
	encoded, _ := encodeExecutionJournal([]agentcore.SemanticRecord{r})
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	records, err := j.SemanticRecords(1, 1)
	if err != nil || len(records) != 1 || records[0].Payload["content"] != "replacement" {
		t.Fatalf("stale index: %v %+v", err, records)
	}
	if err := j.Append(context.Background(), agentcore.NewSemanticRecord(agentcore.SemanticKindUserMessage, "thread", "run", "", "", "", nil)); !errors.Is(err, ErrJournalExternalChange) {
		t.Fatalf("stale writer accepted replacement: %v", err)
	}
}

func TestJournalEncodingFailureDoesNotPoisonDedupOrSequence(t *testing.T) {
	j, err := NewExecutionJournal(filepath.Join(t.TempDir(), "journal.jsonl"), "thread")
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	r := agentcore.NewSemanticRecord(agentcore.SemanticKindRuntimeNotice, "thread", "run", "", "", "", map[string]any{"bad": make(chan int)})
	if err := j.Append(context.Background(), r); !errors.Is(err, ErrJournalEncoding) {
		t.Fatalf("encoding error=%v", err)
	}
	if j.LastAssignedSequence() != 0 {
		t.Fatal("invalid record reserved a sequence")
	}
	r.Payload = map[string]any{"content": "valid"}
	if err := j.Append(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if j.LastAssignedSequence() != 1 {
		t.Fatal("invalid record poisoned its retry ID")
	}
}
