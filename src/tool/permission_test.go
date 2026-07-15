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

	if !ctx.IsWithinWorkspaceResolved("inside.txt") {
		t.Fatalf("expected regular file inside workspace to be allowed")
	}

	externalLink := filepath.Join(workspace, "external-link")
	if err := os.Symlink(outsideFile, externalLink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	if ctx.IsWithinWorkspaceResolved("external-link") {
		t.Fatalf("expected symlink to external file to be rejected")
	}

	relativeLink := filepath.Join(workspace, "relative-link")
	if err := os.Symlink("../outside.txt", relativeLink); err != nil {
		t.Fatalf("failed to create relative symlink: %v", err)
	}
	if ctx.IsWithinWorkspaceResolved("relative-link") {
		t.Fatalf("expected relative symlink escaping workspace to be rejected")
	}

	internalLink := filepath.Join(workspace, "internal-link")
	if err := os.Symlink("inside.txt", internalLink); err != nil {
		t.Fatalf("failed to create internal symlink: %v", err)
	}
	if !ctx.IsWithinWorkspaceResolved("internal-link") {
		t.Fatalf("expected symlink pointing inside workspace to be allowed")
	}

	if ctx.IsWithinWorkspaceResolved("does-not-exist.txt") {
		t.Fatalf("expected non-existent path to be rejected conservatively")
	}

	resolved, err := ctx.ResolvePathChecked("inside.txt")
	if err != nil {
		t.Fatalf("expected ResolvePathChecked to succeed for inside file: %v", err)
	}
	if resolved != insideFile {
		t.Fatalf("expected resolved path %q, got %q", insideFile, resolved)
	}

	resolved, err = ctx.ResolvePathChecked("external-link")
	if err != nil {
		t.Fatalf("expected ResolvePathChecked to allow local read symlink: %v", err)
	}
	if resolved != externalLink {
		t.Fatalf("expected readable path %q, got %q", externalLink, resolved)
	}
}

func TestPermissionContextWriteResolversRejectLeafAndParentSymlinkEscapes(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	ctx := NewPermissionContext(workspace, nil)

	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	leafLink := filepath.Join(workspace, "leaf-link.txt")
	if err := os.Symlink(outsideFile, leafLink); err != nil {
		t.Fatalf("create escaping leaf symlink: %v", err)
	}
	parentLink := filepath.Join(workspace, "parent-link")
	if err := os.Symlink(outside, parentLink); err != nil {
		t.Fatalf("create escaping parent symlink: %v", err)
	}

	resolvers := []struct {
		name string
		fn   func(string) (string, error)
	}{
		{name: "writable", fn: ctx.ResolveWritablePathChecked},
		{name: "output", fn: ctx.ResolveOutputPathChecked},
	}
	for _, resolver := range resolvers {
		t.Run(resolver.name+"/leaf", func(t *testing.T) {
			if _, err := resolver.fn(leafLink); err == nil {
				t.Fatal("expected escaping leaf symlink to be rejected")
			}
		})
		t.Run(resolver.name+"/parent-existing-leaf", func(t *testing.T) {
			if _, err := resolver.fn(filepath.Join(parentLink, filepath.Base(outsideFile))); err == nil {
				t.Fatal("expected escaping parent symlink to be rejected for existing leaf")
			}
		})
		t.Run(resolver.name+"/parent-nonexistent-leaf", func(t *testing.T) {
			if _, err := resolver.fn(filepath.Join(parentLink, "new.txt")); err == nil {
				t.Fatal("expected escaping parent symlink to be rejected for nonexistent leaf")
			}
		})
	}
}

func TestPermissionContextWriteResolversAllowNonexistentLeafAndInsideSymlinks(t *testing.T) {
	workspace := t.TempDir()
	insideDir := filepath.Join(workspace, "inside")
	if err := os.MkdirAll(insideDir, 0o755); err != nil {
		t.Fatalf("create inside directory: %v", err)
	}
	insideFile := filepath.Join(insideDir, "existing.txt")
	if err := os.WriteFile(insideFile, []byte("inside"), 0o644); err != nil {
		t.Fatalf("write inside file: %v", err)
	}
	leafLink := filepath.Join(workspace, "inside-leaf-link.txt")
	if err := os.Symlink(insideFile, leafLink); err != nil {
		t.Fatalf("create inside leaf symlink: %v", err)
	}
	parentLink := filepath.Join(workspace, "inside-parent-link")
	if err := os.Symlink(insideDir, parentLink); err != nil {
		t.Fatalf("create inside parent symlink: %v", err)
	}

	ctx := NewPermissionContext(workspace, nil)
	resolvers := []struct {
		name string
		fn   func(string) (string, error)
	}{
		{name: "writable", fn: ctx.ResolveWritablePathChecked},
		{name: "output", fn: ctx.ResolveOutputPathChecked},
	}
	for _, resolver := range resolvers {
		t.Run(resolver.name+"/nonexistent-leaf", func(t *testing.T) {
			path := filepath.Join(insideDir, "new.txt")
			resolved, err := resolver.fn(path)
			if err != nil {
				t.Fatalf("expected nonexistent leaf under real parent to be allowed: %v", err)
			}
			if resolved != path {
				t.Fatalf("expected lexical path %q, got %q", path, resolved)
			}
		})
		t.Run(resolver.name+"/inside-leaf-symlink", func(t *testing.T) {
			resolved, err := resolver.fn(leafLink)
			if err != nil {
				t.Fatalf("expected leaf symlink whose target remains inside root to be allowed: %v", err)
			}
			if resolved != leafLink {
				t.Fatalf("expected lexical symlink path %q, got %q", leafLink, resolved)
			}
		})
		t.Run(resolver.name+"/inside-parent-symlink", func(t *testing.T) {
			path := filepath.Join(parentLink, "new.txt")
			resolved, err := resolver.fn(path)
			if err != nil {
				t.Fatalf("expected parent symlink whose target remains inside root to be allowed: %v", err)
			}
			if resolved != path {
				t.Fatalf("expected lexical path %q, got %q", path, resolved)
			}
		})
	}
}

func TestPermissionContextWriteResolversAllowNonexistentConfiguredRoot(t *testing.T) {
	workspace := t.TempDir()
	generated := filepath.Join(workspace, "generated")
	ctx := NewTaskScopePermissionContext(
		"scope-nonexistent-root",
		workspace,
		nil,
		[]string{generated},
		[]string{generated},
		nil,
		nil,
		filepath.Join(workspace, "tmp"),
	)
	path := filepath.Join(generated, "nested", "new.txt")
	for name, resolver := range map[string]func(string) (string, error){
		"writable": ctx.ResolveWritablePathChecked,
		"output":   ctx.ResolveOutputPathChecked,
	} {
		t.Run(name, func(t *testing.T) {
			resolved, err := resolver(path)
			if err != nil {
				t.Fatalf("expected path below nonexistent configured root to be allowed: %v", err)
			}
			if resolved != path {
				t.Fatalf("expected %q, got %q", path, resolved)
			}
		})
	}
}

func TestPermissionContextReadablePathsAllowOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "report.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0644); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	ctx := NewPermissionContext(workspace, []string{workspace})
	if !ctx.IsWithinReadableRootsResolved(outsideFile) {
		t.Fatalf("expected local read path outside workspace to be allowed")
	}
	resolved, err := ctx.ResolveReadablePathChecked(outsideFile)
	if err != nil {
		t.Fatalf("expected readable path outside workspace to resolve: %v", err)
	}
	if resolved != outsideFile {
		t.Fatalf("expected %q, got %q", outsideFile, resolved)
	}
}

func TestPermissionContextResolvesAttachedFileDisplayName(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	attached := filepath.Join(outside, "客户名单.xlsx")
	if err := os.WriteFile(attached, []byte("xlsx"), 0644); err != nil {
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
