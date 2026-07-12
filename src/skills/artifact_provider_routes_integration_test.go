package skills

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/mcpclient"
	"awaken/src/tool"
	"awaken/src/tool/trash"
	artifactpublish "awaken/src/tools/artifact_publish"
	_ "awaken/src/tools/artifact_runtime"
	shelltool "awaken/src/tools/shell"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type providerRouteMCPInput struct{}

type providerRouteMCPOutput struct {
	BytesBase64 string `json:"bytes_base64"`
	SHA256      string `json:"sha256"`
	Relative    string `json:"relative_path"`
	ContentType string `json:"content_type"`
	Title       string `json:"title"`
}

type providerRouteTrashAdapter struct {
	directory string
	calls     atomic.Int32
}

func (a *providerRouteTrashAdapter) TrashPath(path string) (trash.TrashResult, error) {
	call := a.calls.Add(1)
	if err := os.MkdirAll(a.directory, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	target := filepath.Join(a.directory, fmt.Sprintf("recovery-%d", call))
	if err := os.Rename(path, target); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: path, TrashedPath: target, Method: "test-rename"}, nil
}

var providerRouteServerSequence atomic.Int64

// TestIsolatedTypedMCPSkillScriptAndShellArtifactRoutes is an isolated
// provider integration fixture. It deliberately is not Desktop acceptance:
// it proves registration, execution, host commit, and final artifact facts in
// a temporary workspace only.
func TestIsolatedTypedMCPSkillScriptAndShellArtifactRoutes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("write-capable Skill script and Shell routes require macOS Seatbelt")
	}
	for _, executable := range []string{"sandbox-exec", "sh"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skipf("%s is unavailable: %v", executable, err)
		}
	}

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	packageRoot := filepath.Join(t.TempDir(), "artifact-route-skill")
	if err := os.MkdirAll(filepath.Join(packageRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for relative, content := range map[string]string{
		"SKILL.md": `---
name: artifact-route-skill
description: Produce the isolated provider-route Markdown artifact.
---
Run the bundled script only when this fixture requests it.
`,
		"scripts/create.sh": "#!/bin/sh\nmkdir -p output\nprintf '# provider route\\n' > output/skill.md\n",
	} {
		if err := os.WriteFile(filepath.Join(packageRoot, relative), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pkg, err := PackageFromPath(packageRoot)
	if err != nil {
		t.Fatalf("package Skill fixture: %v", err)
	}
	installed, err := InstallSkillPackage(InstallRequest{
		Package: pkg, Scope: string(SourceProject), ProjectRoot: workspace, OverwritePolicy: "abort",
	})
	if err != nil || installed.Skill == nil || installed.Skill.ID != "artifact-route-skill" || installed.Skill.Source != SourceProject || installed.Skill.State != StateLoaded {
		t.Fatalf("Skill provider did not register: result=%+v err=%v", installed, err)
	}

	permissionContext := tool.NewTaskScopePermissionContext(
		"provider-route-fixture", workspace, []string{workspace}, []string{outputRoot}, []string{outputRoot}, nil, nil, "",
	)
	trashAdapter := &providerRouteTrashAdapter{directory: filepath.Join(t.TempDir(), "trash")}
	broker := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: t.TempDir(), TrashAdapter: trashAdapter,
	})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	ctx = tool.ContextWithExecutionBroker(ctx, broker)

	shell := &shelltool.ShellTool{}
	publisher := &artifactpublish.ArtifactPublishTool{}
	typedRegistration, err := tool.GetRegisteredTool("document_create")
	if err != nil || typedRegistration.Origin != tool.SourceBuiltin || !tool.HasCapability(typedRegistration.Provider.Capabilities, tool.CapabilityDocumentCreate) || typedRegistration.Definition.Name != "document_create" {
		t.Fatalf("typed document.create provider is not registered in Tool Catalog: registration=%+v err=%v", typedRegistration, err)
	}
	typedRuntime, ok := typedRegistration.Runtime.(tool.OutcomeTool)
	if !ok || shell.Definition().ProviderID != tool.ProviderID("builtin:shell") {
		t.Fatalf("typed or Shell provider execution contract drifted: typed=%T shell=%+v", typedRegistration.Runtime, shell.Definition())
	}

	typedInput := `{"output_path":"output/typed.md","title":"# provider route"}`
	typedOutcome, err := typedRuntime.ExecuteOutcome(ctx, typedInput, nil)
	if err != nil || typedOutcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("typed route failed: outcome=%+v err=%v", typedOutcome, err)
	}
	if len(typedOutcome.Artifacts) != 1 || typedOutcome.Artifacts[0].Role != tool.ArtifactRoleDeliverable || typedOutcome.Artifacts[0].Producer != "document_create" {
		t.Fatalf("typed document.create did not register its deliverable: %+v", typedOutcome.Artifacts)
	}

	mcpAdapter, shutdownMCP := startProviderRouteMCP(t)
	defer shutdownMCP()
	mcpOutcome, err := mcpAdapter.ExecuteOutcome(ctx, `{}`, nil)
	if err != nil || mcpOutcome.Status != tool.OutcomeSucceeded || len(mcpOutcome.Artifacts) != 1 {
		t.Fatalf("MCP route failed: outcome=%+v err=%v", mcpOutcome, err)
	}
	manifest := mcpAdapter.ProviderManifest()
	if manifest.Source != tool.SourceMCP || !tool.HasCapability(manifest.Capabilities, tool.CapabilityDocumentCreate) {
		t.Fatalf("MCP route provider was not registered accurately: %+v", manifest)
	}

	skillCommand, _ := json.Marshal(map[string]any{
		"command": "sh .awaken/skills/artifact-route-skill/scripts/create.sh", "workdir": workspace, "timeout_ms": 30_000,
	})
	skillOutcome, err := shell.ExecuteOutcome(ctx, string(skillCommand), nil)
	if err != nil || skillOutcome.Status != tool.OutcomeSucceeded || skillOutcome.Operation == nil || skillOutcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("installed Skill script route failed: outcome=%+v err=%v", skillOutcome, err)
	}

	shellCommand, _ := json.Marshal(map[string]any{
		"command": "printf '# provider route\\n' > output/shell.md", "workdir": workspace, "timeout_ms": 30_000,
	})
	shellOutcome, err := shell.ExecuteOutcome(ctx, string(shellCommand), nil)
	if err != nil || shellOutcome.Status != tool.OutcomeSucceeded || shellOutcome.Operation == nil || shellOutcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("direct Shell route failed: outcome=%+v err=%v", shellOutcome, err)
	}

	finalArtifacts := []tool.ArtifactDescriptor{mcpOutcome.Artifacts[0]}
	for _, relative := range []string{"output/typed.md", "output/skill.md", "output/shell.md"} {
		input, _ := json.Marshal(map[string]any{"path": relative, "title": "Provider Route " + filepath.Base(relative)})
		publishOutcome, publishErr := publisher.ExecuteOutcome(ctx, string(input), nil)
		if publishErr != nil || publishOutcome.Status != tool.OutcomeSucceeded || len(publishOutcome.Artifacts) != 1 {
			t.Fatalf("publish %s: outcome=%+v err=%v", relative, publishOutcome, publishErr)
		}
		finalArtifacts = append(finalArtifacts, publishOutcome.Artifacts[0])
	}

	wantPaths := map[string]struct{}{
		"output/mcp.md": {}, "output/typed.md": {}, "output/skill.md": {}, "output/shell.md": {},
	}
	for _, artifact := range finalArtifacts {
		if artifact.Role != tool.ArtifactRoleDeliverable || artifact.Kind != "markdown" || artifact.Status != "ready" {
			t.Fatalf("provider route did not complete the same Markdown artifact contract: %+v", artifact)
		}
		if _, ok := wantPaths[artifact.Path]; !ok {
			t.Fatalf("unexpected provider route artifact path: %+v", artifact)
		}
		delete(wantPaths, artifact.Path)
		data, readErr := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(artifact.Path)))
		if readErr != nil || strings.TrimSpace(string(data)) != "# provider route" {
			t.Fatalf("artifact %s bytes=%q err=%v", artifact.Path, data, readErr)
		}
	}
	if len(wantPaths) != 0 || trashAdapter.calls.Load() != 0 {
		t.Fatalf("provider routes incomplete or unexpectedly replaced files: missing=%v trash=%d", wantPaths, trashAdapter.calls.Load())
	}
}

func startProviderRouteMCP(t *testing.T) (*mcpclient.McpToolAdapter, func()) {
	t.Helper()
	serverName := fmt.Sprintf("provider-route-artifact-%d", providerRouteServerSequence.Add(1))
	data := []byte("# provider route\n")
	digest := sha256.Sum256(data)
	definition := &mcp.Tool{
		Name: "create_markdown_artifact",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false, DestructiveHint: providerRouteBool(false), OpenWorldHint: providerRouteBool(false),
		},
		Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
			ProviderID: "mcp:provider-route:create", Capabilities: []tool.Capability{tool.CapabilityDocumentCreate},
			ArtifactOutput: &mcpclient.MCPArtifactOutputContract{
				BytesBase64Field: "bytes_base64", SHA256Field: "sha256", RelativePathField: "relative_path",
				ContentTypeField: "content_type", TitleField: "title", MaxBytes: 1024,
			},
		}},
		InputSchema: map[string]any{"type": "object"},
		OutputSchema: map[string]any{
			"type": "object", "required": []string{"bytes_base64", "sha256", "relative_path", "content_type", "title"},
			"properties": map[string]any{
				"bytes_base64": map[string]any{"type": "string"}, "sha256": map[string]any{"type": "string"},
				"relative_path": map[string]any{"type": "string"}, "content_type": map[string]any{"type": "string"},
				"title": map[string]any{"type": "string"},
			},
		},
	}
	registry := mcpclient.NewInProcRegistry()
	if err := registry.Register(serverName, func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "v1"}, nil)
		mcp.AddTool(server, definition, func(_ context.Context, _ *mcp.CallToolRequest, _ providerRouteMCPInput) (*mcp.CallToolResult, providerRouteMCPOutput, error) {
			return nil, providerRouteMCPOutput{
				BytesBase64: base64.StdEncoding.EncodeToString(data), SHA256: hex.EncodeToString(digest[:]),
				Relative: "output/mcp.md", ContentType: "text/markdown", Title: "Provider Route mcp.md",
			}, nil
		})
		return server
	}); err != nil {
		t.Fatalf("register provider-route MCP: %v", err)
	}
	mgr := mcpclient.NewManager(mcpclient.WithInProcRegistry(registry))
	managerCtx, cancel := context.WithCancel(context.Background())
	mgr.Start(managerCtx, []mcpclient.ScopedMcpServerConfig{{
		Name: serverName, Scope: mcpclient.ScopeProject,
		Config: mcpclient.McpServerConfig{InProc: &mcpclient.InProcTransportConfig{ServerName: serverName}},
	}})
	if err := mcpclient.WaitForServerStateForTest(mgr, serverName, mcpclient.StateConnected, 3*time.Second); err != nil {
		cancel()
		mgr.Shutdown(context.Background())
		t.Fatalf("wait for provider-route MCP: %v", err)
	}
	shutdown := func() {
		cancel()
		mgr.Shutdown(context.Background())
		mcpclient.UnregisterMcpToolsForTest(serverName)
	}
	return mcpclient.NewMcpToolAdapter(mgr, serverName, definition), shutdown
}

func providerRouteBool(value bool) *bool { return &value }
