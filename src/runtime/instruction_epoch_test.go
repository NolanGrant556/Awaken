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
	// Real removal is observed through the loader as a different non-empty
	// digest (the loader digests zero sources too); it still rotates.
	candidate.InstructionDigest = "digest-after-removal"
	candidate.PrefixMessages = nil
	removed, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Receipt.BoundaryReason != "agents_instructions_changed" || len(removed.PrefixMessages) != 0 {
		t.Fatalf("removed instruction source left old content in the new epoch: %+v", removed)
	}
}

// An empty candidate digest means the projection did not evaluate the
// instruction snapshot (partial replay assembly), not that the AGENTS
// instructions changed. It must never fabricate a rotation.
func TestContextEpochIgnoresUnevaluatedInstructionDigest(t *testing.T) {
	t.Setenv(awakenHomeEnv, t.TempDir())
	store := NewContextEpochStore()
	candidate := agentcore.ContextEpochCandidate{
		ThreadID: "thread-agents-unevaluated", WorkspaceRoot: t.TempDir(), ProviderID: "provider", ProfileID: "profile",
		ModelID: "model", ProtocolFamily: "openai_responses", PromptSchemaVersion: "awaken_prompt_v2",
		ExactSystemPrompt: "system", InstructionDigest: "digest-live",
		PrefixMessages: []convcontext.Message{{Role: convcontext.RoleUser, Content: "frozen", Origin: convcontext.MessageOriginSynthetic}},
	}
	first, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.State != "created" {
		t.Fatalf("first=%+v", first.Receipt)
	}
	candidate.InstructionDigest = ""
	candidate.PrefixMessages = nil
	unevaluated, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if unevaluated.Receipt.State != "unchanged" || unevaluated.Receipt.EpochID != first.Receipt.EpochID {
		t.Fatalf("unevaluated instruction digest fabricated an epoch change: %+v", unevaluated.Receipt)
	}
	if len(unevaluated.PrefixMessages) != 1 || unevaluated.PrefixMessages[0].Content != "frozen" {
		t.Fatalf("unchanged epoch lost its frozen instruction prefix: %+v", unevaluated.PrefixMessages)
	}
	// The next fully assembled projection with the original digest stays on
	// the same epoch instead of rotating back and forth.
	candidate.InstructionDigest = "digest-live"
	candidate.PrefixMessages = []convcontext.Message{{Role: convcontext.RoleUser, Content: "frozen", Origin: convcontext.MessageOriginSynthetic}}
	restored, err := store.Project(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Receipt.State != "unchanged" || restored.Receipt.EpochID != first.Receipt.EpochID {
		t.Fatalf("restored projection rotated instead of reusing the epoch: %+v", restored.Receipt)
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
