package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func artifactBrokerTestRequest(workspace string, data []byte) ExecutionBrokerRequest {
	digest := sha256.Sum256(data)
	return ExecutionBrokerRequest{
		ProviderID: "mcp:test:create_artifact", Capability: CapabilityDocumentCreate,
		Operation:     BrokerOperationArtifactPublish,
		WorkspaceRoot: workspace, WorkingDirectory: workspace, WorkspaceScopeDeclared: true,
		WritableRoots: []string{"output"}, ReferencedPaths: []string{filepath.Join("output", "report.md")},
		ArtifactPublish: &BrokerArtifactPublishPlan{
			RelativePath: filepath.Join("output", "report.md"), SHA256: hex.EncodeToString(digest[:]),
			SizeBytes: int64(len(data)), ContentType: "text/markdown", Kind: "markdown",
		},
	}
}

func TestWorkspaceExecutionBrokerHostArtifactPublishCommitsExactFileWithoutChildSandbox(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "output"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("# exact host artifact\n")
	trashAdapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	})
	authorization, err := broker.Authorize(context.Background(), artifactBrokerTestRequest(workspace, data))
	if err != nil {
		t.Fatalf("authorize host artifact: %v", err)
	}
	if authorization.RequiresAppliedSandbox || authorization.SandboxTier != "host_artifact_publish" {
		t.Fatalf("host-only publication must not claim a child sandbox: %+v", authorization)
	}
	stageTarget := filepath.Join(authorization.ExecutionWorkspaceRoot, "output", "report.md")
	if err := os.WriteFile(stageTarget, data, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, err := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessNotApplicable, SideEffectState: OutcomeSideEffectUnknown,
	})
	if err != nil || receipt.CommitStatus != "committed" || receipt.SideEffectState != OutcomeSideEffectApplied {
		t.Fatalf("complete host artifact: receipt=%+v err=%v", receipt, err)
	}
	committed, readErr := os.ReadFile(filepath.Join(workspace, "output", "report.md"))
	if readErr != nil || string(committed) != string(data) || trashAdapter.calls != 0 {
		t.Fatalf("exact artifact was not committed: data=%q err=%v trash=%d", committed, readErr, trashAdapter.calls)
	}
}

func TestWorkspaceExecutionBrokerHostArtifactPublishRejectsUnrelatedDeltaBeforeTrash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "output"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("# exact host artifact\n")
	trashAdapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	})
	authorization, err := broker.Authorize(context.Background(), artifactBrokerTestRequest(workspace, data))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorization.ExecutionWorkspaceRoot, "output", "report.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authorization.ExecutionWorkspaceRoot, "output", "unrelated.md"), []byte("host bug"), 0o644); err != nil {
		t.Fatal(err)
	}
	receipt, completeErr := broker.Complete(context.Background(), authorization, BrokerExecutionResult{
		TransportStatus: TransportCompleted, ProcessStatus: ProcessNotApplicable, SideEffectState: OutcomeSideEffectUnknown,
	})
	if !errors.Is(completeErr, ErrExecutionBrokerDenied) || receipt.CommitStatus != "invalid_host_artifact_delta" || receipt.SideEffectState != OutcomeSideEffectNone {
		t.Fatalf("unrelated delta did not fail closed: receipt=%+v err=%v", receipt, completeErr)
	}
	for _, name := range []string{"report.md", "unrelated.md"} {
		if _, statErr := os.Lstat(filepath.Join(workspace, "output", name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("denied delta published %s: %v", name, statErr)
		}
	}
	if trashAdapter.calls != 0 {
		t.Fatalf("invalid host delta reached Trash: %d", trashAdapter.calls)
	}
}

func TestWorkspaceExecutionBrokerHostArtifactPublishRejectsRawShellForgery(t *testing.T) {
	workspace := t.TempDir()
	data := []byte("# forged host artifact\n")
	request := artifactBrokerTestRequest(workspace, data)
	request.ProviderID = "builtin:shell"
	request.Capability = CapabilityShellExecute
	trashAdapter := &brokerTestTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := NewWorkspaceExecutionBrokerWithOptions(WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	})
	authorization, err := broker.Authorize(context.Background(), request)
	if !errors.Is(err, ErrExecutionBrokerDenied) || authorization.Action != BrokerDeny {
		t.Fatalf("raw Shell forged host artifact operation was not denied: authorization=%+v err=%v", authorization, err)
	}
	if trashAdapter.calls != 0 {
		t.Fatalf("raw Shell forgery reached Trash: %d", trashAdapter.calls)
	}
}
