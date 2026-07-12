package mcpclient

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/trash"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type artifactBridgeInput struct {
	Mode string `json:"mode"`
}

type artifactBridgeOutput struct {
	BytesBase64  string `json:"bytes_base64"`
	SHA256       string `json:"sha256"`
	Relative     string `json:"relative_path"`
	ContentType  string `json:"content_type"`
	Title        string `json:"title"`
	ProviderEcho string `json:"provider_echo"`
}

var artifactBridgeServerSequence atomic.Int64

func artifactBridgeTool(contract MCPArtifactOutputContract) *mcp.Tool {
	return &mcp.Tool{
		Name: "create_artifact",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false),
		},
		Meta: providerMetadata(MCPToolProviderMetadata{
			ProviderID: "mcp:artifact-fixture:create", Capabilities: []tool.Capability{tool.CapabilityDocumentCreate},
			ArtifactOutput: &contract,
		}),
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"mode": map[string]any{"type": "string"}},
		},
		OutputSchema: map[string]any{
			"type":     "object",
			"required": []string{"bytes_base64", "sha256", "relative_path", "content_type", "title"},
			"properties": map[string]any{
				"bytes_base64":  map[string]any{"type": "string"},
				"sha256":        map[string]any{"type": "string"},
				"relative_path": map[string]any{"type": "string"},
				"content_type":  map[string]any{"type": "string"},
				"title":         map[string]any{"type": "string"},
				"provider_echo": map[string]any{"type": "string"},
			},
		},
	}
}

func defaultArtifactBridgeContract() MCPArtifactOutputContract {
	return MCPArtifactOutputContract{
		BytesBase64Field: "bytes_base64", SHA256Field: "sha256", RelativePathField: "relative_path",
		ContentTypeField: "content_type", TitleField: "title", MaxBytes: 4096,
	}
}

func artifactBridgeFixtureOutput(mode string) artifactBridgeOutput {
	data := []byte("# MCP report\n")
	output := artifactBridgeOutput{
		BytesBase64:  base64.StdEncoding.EncodeToString(data),
		SHA256:       artifactBridgeDigest(data),
		Relative:     "output/report.md",
		ContentType:  "text/markdown",
		Title:        "MCP Report",
		ProviderEcho: base64.StdEncoding.EncodeToString(data),
	}
	switch mode {
	case "hash_mismatch":
		output.SHA256 = strings.Repeat("0", 64)
	case "oversize":
		data = []byte(strings.Repeat("x", 8192))
		output.BytesBase64 = base64.StdEncoding.EncodeToString(data)
		output.SHA256 = artifactBridgeDigest(data)
	case "docx":
		data = artifactBridgeDOCXBytes()
		output.BytesBase64 = base64.StdEncoding.EncodeToString(data)
		output.SHA256 = artifactBridgeDigest(data)
		output.Relative = "output/report.docx"
		output.ContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		output.Title = "MCP DOCX Report"
	case "traversal":
		output.Relative = "../outside.md"
	case "absolute":
		output.Relative = "/tmp/provider-claimed.md"
	case "out_of_scope":
		output.Relative = "other/report.md"
	case "writable_only":
		output.Relative = "writable-only/report.md"
	case "output_only":
		output.Relative = "output-only/report.md"
	case "symlink":
		output.Relative = "output/link/report.md"
	case "protected":
		output.Relative = "output/protected/report.md"
	case "sensitive":
		output.Relative = "output/.env.md"
	case "unsupported_extension":
		output.Relative = "output/report.txt"
		output.ContentType = "text/plain"
	case "content_mismatch":
		output.ContentType = "application/pdf"
	case "false_success":
		output.BytesBase64 = ""
	}
	return output
}

func artifactBridgeDOCXBytes() []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
		"word/document.xml":   `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`,
	} {
		entry, err := writer.Create(name)
		if err != nil {
			panic(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			panic(err)
		}
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}

func artifactBridgeDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func startArtifactBridgeAdapter(t *testing.T, toolDefinition *mcp.Tool) (*McpToolAdapter, *atomic.Int32) {
	t.Helper()
	serverName := fmt.Sprintf("artifact-bridge-%d", artifactBridgeServerSequence.Add(1))
	var calls atomic.Int32
	registry := NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, toolDefinition, func(_ context.Context, _ *mcp.CallToolRequest, input artifactBridgeInput) (*mcp.CallToolResult, artifactBridgeOutput, error) {
			calls.Add(1)
			return nil, artifactBridgeFixtureOutput(input.Mode), nil
		})
		return server
	}); err != nil {
		t.Fatalf("register artifact MCP: %v", err)
	}
	mgr := NewManager(WithInProcRegistry(registry), WithLogger(noopLogger{}))
	managerCtx, cancel := context.WithCancel(context.Background())
	mgr.Start(managerCtx, []ScopedMcpServerConfig{{
		Name: serverName, Scope: ScopeProject,
		Config: McpServerConfig{InProc: &InProcTransportConfig{ServerName: serverName}},
	}})
	t.Cleanup(func() {
		cancel()
		mgr.Shutdown(context.Background())
		UnregisterMcpToolsForTest(serverName)
	})
	if err := WaitForServerStateForTest(mgr, serverName, StateConnected, 3*time.Second); err != nil {
		t.Fatalf("wait for artifact MCP: %v", err)
	}
	return NewMcpToolAdapter(mgr, serverName, toolDefinition), &calls
}

type artifactBridgeTrashAdapter struct {
	directory string
	calls     int
}

func (a *artifactBridgeTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	a.calls++
	if err := os.MkdirAll(a.directory, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.directory, fmt.Sprintf("recovery-%d", a.calls))
	if err := os.Rename(path, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: path, TrashedPath: target, Method: "test-rename"}, nil
}

func artifactBridgeContext(t *testing.T, workspace string, trashAdapter trash.TrashAdapter) (context.Context, string) {
	t.Helper()
	outputRoot := filepath.Join(workspace, "output")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"artifact-test", workspace, []string{workspace}, []string{outputRoot}, []string{outputRoot}, nil, nil, "",
	)
	broker := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, broker)
	return ctx, filepath.Join(outputRoot, "report.md")
}

func TestMCPArtifactBridgePublishesValidatedBytesThroughHostBroker(t *testing.T) {
	definition := artifactBridgeTool(defaultArtifactBridgeContract())
	adapter, calls := startArtifactBridgeAdapter(t, definition)
	workspace := t.TempDir()
	trashAdapter := &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	ctx, target := artifactBridgeContext(t, workspace, trashAdapter)

	outcome, err := adapter.ExecuteOutcome(ctx, `{}`, nil)
	if err != nil || outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("publish outcome=%+v err=%v", outcome, err)
	}
	if calls.Load() != 1 || trashAdapter.calls != 0 {
		t.Fatalf("unexpected provider/Trash calls: provider=%d trash=%d", calls.Load(), trashAdapter.calls)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "# MCP report\n" {
		t.Fatalf("host did not publish exact provider bytes: data=%q err=%v", data, err)
	}
	if len(outcome.Artifacts) != 1 || outcome.Artifacts[0].Role != tool.ArtifactRoleDeliverable || outcome.Artifacts[0].Action != tool.ArtifactActionCreated || outcome.Artifacts[0].Path != "output/report.md" {
		t.Fatalf("unexpected artifact descriptor: %+v", outcome.Artifacts)
	}
	if outcome.Operation == nil || outcome.Operation.Type != "mcp_artifact_publish" || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("unexpected operation: %+v", outcome.Operation)
	}
	if len(outcome.Changes) == 0 || outcome.Changes[len(outcome.Changes)-1].Certainty != tool.ChangeObserved {
		t.Fatalf("missing observed broker change: %+v", outcome.Changes)
	}
	manifest := adapter.ProviderManifest()
	if manifest.ProviderID == "" || len(manifest.SideEffects) != 2 || adapter.fileBridgeScope != tool.FileBridgeTaskScopeWrite || adapter.capabilityClass != tool.CapabilityArtifactWrite {
		t.Fatalf("provider artifact facts are incomplete: manifest=%+v annotations=%+v", manifest, adapter.Annotations())
	}
	if outcome.Diagnostic["provider_id"] != manifest.ProviderID || len(adapter.Definition().SideEffects) != 2 {
		t.Fatalf("Outcome/Definition lost exact provider facts: diagnostic=%+v definition=%+v", outcome.Diagnostic, adapter.Definition())
	}
	receipt, ok := outcome.Diagnostic["broker_receipt"].(tool.BrokerExecutionReceipt)
	if !ok || receipt.CommitStatus != "committed" || receipt.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("Outcome lost host commit receipt: %#v", outcome.Diagnostic["broker_receipt"])
	}
	encoded := artifactBridgeFixtureOutput("").BytesBase64
	serialized, _ := json.Marshal(outcome)
	if strings.Contains(string(serialized), encoded) || outcome.Diagnostic["provider_bytes_exposed"] != false {
		t.Fatalf("provider bytes leaked into outcome: %s", serialized)
	}
	if placeholder, _ := outcome.Details["bytes_base64"].(string); !strings.HasPrefix(placeholder, "[host-consumed ") {
		t.Fatalf("structured result did not redact provider bytes: %+v", outcome.Details)
	}
	if _, leaked := outcome.Details["provider_echo"]; leaked {
		t.Fatalf("host forwarded provider fields outside the artifact contract: %+v", outcome.Details)
	}
}

func TestMCPArtifactBridgePublishesValidDOCXContainer(t *testing.T) {
	adapter, _ := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	workspace := t.TempDir()
	ctx, _ := artifactBridgeContext(t, workspace, &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")})
	outcome, err := adapter.ExecuteOutcome(ctx, `{"mode":"docx"}`, nil)
	if err != nil || outcome.Status != tool.OutcomeSucceeded || len(outcome.Artifacts) != 1 {
		t.Fatalf("DOCX artifact outcome=%+v err=%v", outcome, err)
	}
	artifact := outcome.Artifacts[0]
	if artifact.Kind != "document" || artifact.Path != "output/report.docx" || artifact.Role != tool.ArtifactRoleDeliverable {
		t.Fatalf("DOCX artifact descriptor=%+v", artifact)
	}
	data, readErr := os.ReadFile(filepath.Join(workspace, "output", "report.docx"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	reader, zipErr := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if zipErr != nil || len(reader.File) != 2 {
		t.Fatalf("host-published DOCX is not the validated container: entries=%d err=%v", len(reader.File), zipErr)
	}
}

func TestMCPArtifactBridgeRejectsUnverifiedOrOutOfScopeOutputs(t *testing.T) {
	definition := artifactBridgeTool(defaultArtifactBridgeContract())
	adapter, calls := startArtifactBridgeAdapter(t, definition)
	tests := []struct {
		mode string
		code string
	}{
		{mode: "hash_mismatch", code: ErrorCodeArtifactOutputHashMismatch},
		{mode: "oversize", code: ErrorCodeArtifactOutputOversize},
		{mode: "traversal", code: ErrorCodeArtifactOutputPathDenied},
		{mode: "absolute", code: ErrorCodeArtifactOutputPathDenied},
		{mode: "out_of_scope", code: ErrorCodeArtifactOutputPathDenied},
		{mode: "sensitive", code: ErrorCodeArtifactOutputPathDenied},
		{mode: "unsupported_extension", code: ErrorCodeArtifactOutputPathDenied},
		{mode: "content_mismatch", code: ErrorCodeArtifactOutputContentMismatch},
		{mode: "false_success", code: ErrorCodeArtifactOutputInvalid},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			workspace := t.TempDir()
			trashAdapter := &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
			ctx, target := artifactBridgeContext(t, workspace, trashAdapter)
			outcome, err := adapter.ExecuteOutcome(ctx, fmt.Sprintf(`{"mode":%q}`, test.mode), nil)
			if err != nil || outcome.ErrorCode != test.code || len(outcome.Artifacts) != 0 {
				t.Fatalf("mode=%s outcome=%+v err=%v", test.mode, outcome, err)
			}
			if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("mode=%s created a pseudo artifact: %v", test.mode, statErr)
			}
			if trashAdapter.calls != 0 {
				t.Fatalf("mode=%s reached Trash before a validated replacement", test.mode)
			}
		})
	}
	if calls.Load() != int32(len(tests)) {
		t.Fatalf("expected each false-success case to complete transport, got %d calls", calls.Load())
	}
}

func TestMCPArtifactBridgeRejectsSymlinkPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	adapter, _ := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	workspace := t.TempDir()
	ctx, target := artifactBridgeContext(t, workspace, &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")})
	realOutput := filepath.Join(workspace, "real-output")
	if err := os.MkdirAll(realOutput, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realOutput, filepath.Join(workspace, "output", "link")); err != nil {
		t.Fatal(err)
	}
	outcome, err := adapter.ExecuteOutcome(ctx, `{"mode":"symlink"}`, nil)
	if err != nil || outcome.ErrorCode != ErrorCodeArtifactOutputPathDenied || len(outcome.Artifacts) != 0 {
		t.Fatalf("symlink outcome=%+v err=%v", outcome, err)
	}
	if _, statErr := os.Lstat(filepath.Join(realOutput, filepath.Base(target))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("symlink target was mutated: %v", statErr)
	}
}

func TestMCPArtifactBridgeRequiresWritableOutputIntersectionAndRejectsProtectedRoot(t *testing.T) {
	adapter, _ := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	workspace := t.TempDir()
	writableOnly := filepath.Join(workspace, "writable-only")
	outputOnly := filepath.Join(workspace, "output-only")
	protected := filepath.Join(workspace, "output", "protected")
	for _, directory := range []string{writableOnly, outputOnly, protected} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	permissionContext := tool.NewTaskScopePermissionContext(
		"artifact-intersection-test", workspace, []string{workspace},
		[]string{writableOnly, filepath.Join(workspace, "output")},
		[]string{outputOnly, filepath.Join(workspace, "output")}, nil, []string{protected}, "",
	)
	trashAdapter := &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	}))
	for _, mode := range []string{"writable_only", "output_only", "protected"} {
		t.Run(mode, func(t *testing.T) {
			outcome, err := adapter.ExecuteOutcome(ctx, fmt.Sprintf(`{"mode":%q}`, mode), nil)
			if err != nil || outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != ErrorCodeArtifactOutputPathDenied || len(outcome.Artifacts) != 0 {
				t.Fatalf("mode=%s outcome=%+v err=%v", mode, outcome, err)
			}
		})
	}
	if trashAdapter.calls != 0 {
		t.Fatalf("intersection/protected denials reached Trash: %d", trashAdapter.calls)
	}
}

func TestMCPArtifactBridgeReplacementMovesPreimageToTrash(t *testing.T) {
	adapter, _ := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	workspace := t.TempDir()
	trashAdapter := &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	ctx, target := artifactBridgeContext(t, workspace, trashAdapter)
	if err := os.WriteFile(target, []byte("old user bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outcome, err := adapter.ExecuteOutcome(ctx, `{}`, nil)
	if err != nil || outcome.Status != tool.OutcomeSucceeded || trashAdapter.calls != 1 {
		t.Fatalf("replacement outcome=%+v err=%v trash=%d", outcome, err, trashAdapter.calls)
	}
	if len(outcome.Artifacts) != 1 || outcome.Artifacts[0].Action != tool.ArtifactActionUpdated {
		t.Fatalf("replacement artifact action is inaccurate: %+v", outcome.Artifacts)
	}
	var recoveryPath string
	for _, change := range outcome.Changes {
		candidate, _ := change.Details["recovery_path"].(string)
		if filepath.Base(change.Resource) == filepath.Base(target) && candidate != "" {
			recoveryPath = candidate
			if change.Reversibility != tool.ChangeReversible {
				t.Fatalf("replacement lost reversible fact: %+v", change)
			}
		}
	}
	recovered, readErr := os.ReadFile(recoveryPath)
	if readErr != nil || string(recovered) != "old user bytes\n" {
		t.Fatalf("replacement preimage is not recoverable: path=%q data=%q err=%v", recoveryPath, recovered, readErr)
	}
	second, secondErr := adapter.ExecuteOutcome(ctx, `{}`, nil)
	if secondErr != nil || second.Status != tool.OutcomeSucceeded || trashAdapter.calls != 1 || len(second.Artifacts) != 1 || second.Artifacts[0].Action != tool.ArtifactActionPublished {
		t.Fatalf("identical retry should verify/publish without replacing bytes again: outcome=%+v err=%v trash=%d", second, secondErr, trashAdapter.calls)
	}
}

func TestMCPArtifactContractMissingOutputSchemaFailsBeforeTransport(t *testing.T) {
	definition := artifactBridgeTool(defaultArtifactBridgeContract())
	properties := definition.OutputSchema.(map[string]any)["properties"].(map[string]any)
	delete(properties, "content_type")
	adapter, calls := startArtifactBridgeAdapter(t, definition)
	decision := adapter.CheckPermission(context.Background(), `{}`, tool.PermissionContext{})
	if decision.Behavior != tool.PermissionUnsupported || decision.ErrorCode != ErrorCodeUnsupportedArtifactOutput {
		t.Fatalf("missing output schema field must be unsupported: %+v", decision)
	}
	outcome, err := adapter.ExecuteOutcome(context.Background(), `{}`, nil)
	if err != nil || outcome.Status != tool.OutcomeUnsupported || outcome.TransportStatus != tool.TransportNotStarted || calls.Load() != 0 {
		t.Fatalf("schema failure reached transport: outcome=%+v err=%v calls=%d", outcome, err, calls.Load())
	}
}

func TestMCPArtifactDirectAdapterRequiresExplicitTaskScope(t *testing.T) {
	adapter, calls := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	outcome, err := adapter.ExecuteOutcome(context.Background(), `{}`, nil)
	if err != nil || outcome.Status != tool.OutcomeDenied || outcome.ErrorCode != ErrorCodeArtifactOutputPathDenied || len(outcome.Artifacts) != 0 {
		t.Fatalf("direct adapter without TaskScope must fail closed: outcome=%+v err=%v", outcome, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider transport should complete before host result publication validation, calls=%d", calls.Load())
	}
}

type cancelAfterArtifactAuthorizationBroker struct {
	delegate tool.ExecutionBroker
	cancel   context.CancelFunc
}

func (b *cancelAfterArtifactAuthorizationBroker) ID() string { return b.delegate.ID() }
func (b *cancelAfterArtifactAuthorizationBroker) Authorize(ctx context.Context, request tool.ExecutionBrokerRequest) (tool.BrokerAuthorization, error) {
	authorization, err := b.delegate.Authorize(ctx, request)
	if err == nil {
		b.cancel()
	}
	return authorization, err
}
func (b *cancelAfterArtifactAuthorizationBroker) Complete(ctx context.Context, authorization tool.BrokerAuthorization, result tool.BrokerExecutionResult) (tool.BrokerExecutionReceipt, error) {
	return b.delegate.Complete(ctx, authorization, result)
}

func TestMCPArtifactCancellationDiscardsStageWithoutLiveDelta(t *testing.T) {
	adapter, _ := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	stagingParent := t.TempDir()
	trashAdapter := &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	delegate := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: stagingParent, TrashAdapter: trashAdapter,
	})
	baseCtx, cancel := context.WithCancel(context.Background())
	permissionContext := tool.NewTaskScopePermissionContext(
		"artifact-cancel-test", workspace, []string{workspace}, []string{outputRoot}, []string{outputRoot}, nil, nil, "",
	)
	ctx := tool.ContextWithPermissionContext(baseCtx, permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, &cancelAfterArtifactAuthorizationBroker{delegate: delegate, cancel: cancel})
	outcome, err := adapter.ExecuteOutcome(ctx, `{}`, nil)
	if err != nil || outcome.Status != tool.OutcomeCancelled || len(outcome.Artifacts) != 0 {
		t.Fatalf("cancelled publication outcome=%+v err=%v", outcome, err)
	}
	if _, statErr := os.Lstat(filepath.Join(outputRoot, "report.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled publication changed live workspace: %v", statErr)
	}
	entries, readErr := os.ReadDir(stagingParent)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("cancelled publication leaked private stage: entries=%v err=%v", entries, readErr)
	}
	if trashAdapter.calls != 0 {
		t.Fatalf("cancelled new artifact should not invoke Trash, calls=%d", trashAdapter.calls)
	}
}

type removeCommittedArtifactBroker struct {
	delegate tool.ExecutionBroker
	target   string
}

func (b *removeCommittedArtifactBroker) ID() string { return b.delegate.ID() }
func (b *removeCommittedArtifactBroker) Authorize(ctx context.Context, request tool.ExecutionBrokerRequest) (tool.BrokerAuthorization, error) {
	return b.delegate.Authorize(ctx, request)
}
func (b *removeCommittedArtifactBroker) Complete(ctx context.Context, authorization tool.BrokerAuthorization, result tool.BrokerExecutionResult) (tool.BrokerExecutionReceipt, error) {
	receipt, err := b.delegate.Complete(ctx, authorization, result)
	if err == nil && receipt.CommitStatus == "committed" {
		_ = os.Remove(b.target)
	}
	return receipt, err
}

func TestMCPArtifactPostCommitVerificationFailureRetainsAppliedReceiptTruth(t *testing.T) {
	adapter, _ := startArtifactBridgeAdapter(t, artifactBridgeTool(defaultArtifactBridgeContract()))
	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outputRoot, "report.md")
	permissionContext := tool.NewTaskScopePermissionContext(
		"artifact-verify-test", workspace, []string{workspace}, []string{outputRoot}, []string{outputRoot}, nil, nil, "",
	)
	delegate := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: &artifactBridgeTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")},
	})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, &removeCommittedArtifactBroker{delegate: delegate, target: target})
	outcome, err := adapter.ExecuteOutcome(ctx, `{}`, nil)
	if err != nil || outcome.ErrorCode != ErrorCodeArtifactOutputCommitFailed || len(outcome.Artifacts) != 0 {
		t.Fatalf("post-commit verification outcome=%+v err=%v", outcome, err)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied || len(outcome.Changes) == 0 {
		t.Fatalf("committed receipt truth was overwritten by failure defaults: operation=%+v changes=%+v", outcome.Operation, outcome.Changes)
	}
}
