package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
)

func TestContextEpochRotatesWhenInstructionDigestChanges(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := agentcore.ContextEpochCandidate{
		ThreadID: "thread-agents", WorkspaceRoot: t.TempDir(), ProviderID: "provider", ProfileID: "profile",
		ModelID: "model", ProtocolFamily: "openai_responses", PromptSchemaVersion: "awaken_prompt_v2",
		ExactSystemPrompt: "system", InstructionDigest: "digest-one",
		PrefixMessages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "first", Origin: convcontext.MessageOriginSynthetic}},
	}
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.State != "created" || len(first.PrefixMessages) != 1 {
		t.Fatalf("first=%+v", first)
	}
	candidate.InstructionDigest = "digest-two"
	candidate.PrefixMessages[0].Content = "second"
	second, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if second.Receipt.State != "rotated" || second.Receipt.BoundaryReason != "agents_instructions_changed" {
		t.Fatalf("receipt=%+v", second.Receipt)
	}
	if len(second.PrefixMessages) != 1 || second.PrefixMessages[0].Content != "second" {
		t.Fatalf("old instruction prefix survived rotation: %+v", second.PrefixMessages)
	}
	candidate.CompactBoundaryID = "compact-success"
	compacted, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if compacted.Receipt.State != "rotated" || compacted.Receipt.BoundaryReason != "compact_succeeded" || len(compacted.PrefixMessages) != 1 || compacted.PrefixMessages[0].Content != "second" {
		t.Fatalf("successful compact did not inherit the frozen instruction prefix: %+v", compacted)
	}
	candidate.InstructionDigest = ""
	candidate.PrefixMessages = nil
	removed, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Receipt.BoundaryReason != "agents_instructions_changed" || len(removed.PrefixMessages) != 0 {
		t.Fatalf("removed instruction source left old content in the new epoch: %+v", removed)
	}
}

func TestContextEpochRejectsCorruptInstructionPrefixDigestWithoutOverwrite(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := agentcore.ContextEpochCandidate{
		ThreadID: "thread-agents-corrupt", WorkspaceRoot: t.TempDir(), ProviderID: "provider", ProfileID: "profile",
		ModelID: "model", ProtocolFamily: "openai_responses", PromptSchemaVersion: "awaken_prompt_v2",
		ExactSystemPrompt: "system", InstructionDigest: "digest",
		PrefixMessages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "frozen", Origin: convcontext.MessageOriginSynthetic}},
	}
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(activeSessionsDir(), safeTranscriptFileName(candidate.ThreadID), "context", "epochs")
	path := filepath.Join(dir, first.Receipt.EpochID+".json")
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sidecar map[string]any
	if err := json.Unmarshal(encoded, &sidecar); err != nil {
		t.Fatal(err)
	}
	sidecar["baseline_digest"] = "corrupt"
	corrupt, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	corrupt = append(corrupt, '\n')
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewContextEpochStore().Project(context.Background(), candidate); err == nil {
		t.Fatal("corrupt epoch digest must block projection")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, corrupt) {
		t.Fatal("corrupt sidecar was overwritten instead of preserved")
	}
}
