package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecideAllowsMessageSendWithoutHostReview(t *testing.T) {
	decision := Decide(context.Background(), ToolDefinition{
		Name:         "broadcast_message",
		Source:       SourceConnector,
		Capabilities: []Capability{CapabilityMessageSend},
		Annotations: ToolAnnotations{
			CapabilityClass: CapabilityConnectorWrite,
			OpenWorld:       true,
		},
	})
	if decision.Behavior != PermissionAllow {
		t.Fatalf("message sends must not be intercepted by a host review gate: %+v", decision)
	}
}

func TestDecideWithOverrideAcceptsCanonicalAndRawMCPServerWildcards(t *testing.T) {
	definition := ToolDefinition{
		Name: "mcp__plugin__acme_tools__local__echo",
		Annotations: ToolAnnotations{
			ServerName:          "plugin__acme.tools__local",
			CanonicalServerName: "plugin__acme_tools__local",
		},
	}
	for _, wildcard := range []string{
		"mcp__plugin__acme_tools__local__*",
		"mcp__plugin__acme.tools__local__*",
	} {
		decision := DecideWithOverride(context.Background(), definition, map[string]PermissionPolicy{
			"*":      PolicyAlwaysDeny,
			wildcard: PolicyAlwaysAllow,
		})
		if decision.Behavior != PermissionAllow {
			t.Fatalf("wildcard %q did not match: %+v", wildcard, decision)
		}
	}
}

func TestPermissionContextDefaultsWritableRootsToWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	ctx := NewPermissionContext(root, nil)

	if !ctx.IsWithinWorkspace(filepath.Join(root, "file.txt")) {
		t.Fatalf("expected path under workspace root to be in workspace")
	}
	if !ctx.IsWithinWritableRoots(filepath.Join(root, "file.txt")) {
		t.Fatalf("expected workspace root to be writable by default")
	}
	if ctx.IsWithinWorkspace(filepath.Join(t.TempDir(), "file.txt")) {
		t.Fatalf("expected unrelated path to be outside workspace")
	}
}

func TestPermissionContextExplicitWritableRootsCanNarrowWrites(t *testing.T) {
	workspace := t.TempDir()
	writable := t.TempDir()
	ctx := NewPermissionContext(workspace, []string{writable})

	if !ctx.IsWithinWorkspace(filepath.Join(workspace, "file.txt")) {
		t.Fatalf("expected workspace path to be in workspace")
	}
	if ctx.IsWithinWritableRoots(filepath.Join(workspace, "file.txt")) {
		t.Fatalf("expected workspace path outside explicit writable roots to be non-writable")
	}
	if !ctx.IsWithinWritableRoots(filepath.Join(writable, "file.txt")) {
		t.Fatalf("expected explicit writable root to allow writes")
	}
}

func TestPermissionContextResolvesRelativePathsFromWorkspaceRoot(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, []string{"generated"})

	if got := ctx.ResolvePath("src/main.go"); got != filepath.Join(workspace, "src", "main.go") {
		t.Fatalf("unexpected resolved path: %q", got)
	}
	if !ctx.IsWithinWritableRoots("generated/file.txt") {
		t.Fatalf("expected relative writable root to be resolved under workspace")
	}
	if ctx.IsWithinWritableRoots("src/main.go") {
		t.Fatalf("expected relative path outside explicit writable roots to be non-writable")
	}
}

func TestPermissionContextResolvesSymlinksForWorkspaceBoundary(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	insideFile := filepath.Join(workspace, "inside.txt")
	if err := os.WriteFile(insideFile, []byte("inside"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	symlinkInside := filepath.Join(workspace, "link-outside")
	if err := os.Symlink(outsideFile, symlinkInside); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	if ctx.IsWithinWorkspaceResolved(symlinkInside) {
		t.Fatalf("expected symlink target outside workspace to fail boundary check")
	}
	if !ctx.IsWithinWorkspaceResolved(insideFile) {
		t.Fatalf("expected real file inside workspace to pass boundary check")
	}
}

func TestPermissionContextRecoversableDeleteRejectsSymlinks(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	target := filepath.Join(workspace, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	link := filepath.Join(workspace, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	if _, err := ctx.ResolveRecoverableDeletePathChecked(link); err == nil {
		t.Fatalf("expected symlink to be rejected for recoverable delete")
	}
}

func TestPermissionContextAttachedFileReferenceByDisplayName(t *testing.T) {
	workspace := t.TempDir()
	attached := filepath.Join(t.TempDir(), "客户名单.xlsx")
	if err := os.WriteFile(attached, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write attached file: %v", err)
	}
	ctx := NewTaskScopePermissionContext(
		"scope-test",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		[]ScopedFile{{Path: attached, DisplayName: "客户名单.xlsx"}},
		nil,
		filepath.Join(t.TempDir(), "tmp"),
	)

	resolved, err := ctx.ResolveReadablePathChecked("客户名单.xlsx")
	if err != nil {
		t.Fatalf("expected display name to resolve: %v", err)
	}
	if resolved != attached {
		t.Fatalf("expected %q, got %q", attached, resolved)
	}
}

func TestPermissionContextAttachedFileDisplayNameAmbiguity(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(t.TempDir(), "report.csv")
	second := filepath.Join(t.TempDir(), "report.csv")
	ctx := NewTaskScopePermissionContext(
		"scope-test",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		[]ScopedFile{
			{Path: first, DisplayName: "report.csv"},
			{Path: second, DisplayName: "report.csv"},
		},
		nil,
		filepath.Join(t.TempDir(), "tmp"),
	)

	if _, err := ctx.ResolveReadablePathChecked("report.csv"); err == nil || !strings.Contains(err.Error(), "多个文件") {
		t.Fatalf("expected ambiguous attached file error, got %v", err)
	}
}

func TestResolveACPCwdDefaultsToWorkspaceRoot(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	cwd, err := ctx.ResolveACPCwd("")
	if err != nil {
		t.Fatalf("empty cwd should resolve to workspace root: %v", err)
	}
	if cwd != workspace {
		t.Fatalf("expected %q, got %q", workspace, cwd)
	}

	cwd, err = ctx.ResolveACPCwd(".")
	if err != nil {
		t.Fatalf(". cwd should resolve to workspace root: %v", err)
	}
	if cwd != workspace {
		t.Fatalf("expected %q, got %q", workspace, cwd)
	}
}

func TestResolveACPCwdResolvesRelativePaths(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	cwd, err := ctx.ResolveACPCwd("sub/project")
	if err != nil {
		t.Fatalf("relative cwd should resolve: %v", err)
	}
	if cwd != filepath.Join(workspace, "sub", "project") {
		t.Fatalf("unexpected resolved cwd: %q", cwd)
	}
}

func TestResolveACPCwdRejectsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	if _, err := ctx.ResolveACPCwd(t.TempDir()); err == nil {
		t.Fatal("expected absolute cwd outside workspace to be rejected")
	}
	if _, err := ctx.ResolveACPCwd(".."); err == nil {
		t.Fatal("expected .. to escape workspace and be rejected")
	}
}

func TestResolveACPCwdRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	outside := t.TempDir()
	symlink := filepath.Join(workspace, "link-out")
	if err := os.Symlink(outside, symlink); err != nil {
		if strings.Contains(err.Error(), "privilege") || strings.Contains(err.Error(), "A required privilege") {
			t.Skipf("symlinks require elevated privileges on this host: %v", err)
		}
		t.Fatalf("failed to create symlink: %v", err)
	}

	if _, err := ctx.ResolveACPCwd(symlink); err == nil {
		t.Fatal("expected symlink escaping workspace to be rejected")
	}
}

func TestResolveACPCwdRejectsProtectedRoot(t *testing.T) {
	workspace := t.TempDir()
	protected := filepath.Join(workspace, "protected")
	if err := os.MkdirAll(protected, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ctx := NewTaskScopePermissionContext(
		"scope-test",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		nil,
		[]string{protected},
		filepath.Join(t.TempDir(), "tmp"),
	)

	if _, err := ctx.ResolveACPCwd(protected); err == nil {
		t.Fatal("expected cwd inside protected root to be rejected")
	}
}
