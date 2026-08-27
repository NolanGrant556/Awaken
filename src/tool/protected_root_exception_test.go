package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// awakenHomeExceptionContext mirrors the task scope of a run whose working
// directory is an Awaken-managed project workspace: the business-data root is
// protected as a whole and only the projects subtree is released.
func awakenHomeExceptionContext(t *testing.T) (PermissionContext, string, string) {
	t.Helper()
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	projectsRoot := filepath.Join(awakenHome, "projects")
	managed := filepath.Join(projectsRoot, "project_1", "workspace")
	if err := os.MkdirAll(managed, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"config", "credentials", "store-generations", "memory"} {
		if err := os.MkdirAll(filepath.Join(awakenHome, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	context := PermissionContext{
		ScopeID:       "scope_managed",
		WorkspaceRoot: managed,
		ReadableRoots: []string{managed},
		WritableRoots: []string{managed},
		OutputRoots:   []string{managed},
		ProtectedRoots: []string{
			awakenHome,
			filepath.Join(managed, ".git"),
			filepath.Join(managed, ".awaken"),
		},
		ProtectedRootExceptions: []string{projectsRoot},
		TemporaryWorkRoot:       filepath.Join(t.TempDir(), "scope"),
	}.Normalize()
	return context, awakenHome, managed
}

func TestProtectedRootExceptionAllowsWritesInsideManagedWorkspace(t *testing.T) {
	context, _, managed := awakenHomeExceptionContext(t)

	for _, relative := range []string{"notes.md", filepath.Join("src", "main.go")} {
		target := filepath.Join(managed, relative)
		if err := context.CheckSensitivePath(target); err != nil {
			t.Fatalf("managed workspace must be writable, %s rejected: %v", relative, err)
		}
		if _, err := context.ResolveMutationPathChecked(relative); err != nil {
			t.Fatalf("mutation inside the managed workspace must be allowed, %s rejected: %v", relative, err)
		}
	}
	if _, err := context.ResolveACPCwd(managed); err != nil {
		t.Fatalf("the managed workspace must be usable as a working directory: %v", err)
	}
}

func TestProtectedRootExceptionStaysFailClosedForTheRestOfTheAwakenHome(t *testing.T) {
	context, awakenHome, _ := awakenHomeExceptionContext(t)

	for _, relative := range []string{
		filepath.Join("config", "mcp-servers.json"),
		filepath.Join("credentials", "token.json"),
		filepath.Join("store-generations", "gen-001", "lifecycle", "state.json"),
		filepath.Join("memory", "notes.jsonl"),
	} {
		target := filepath.Join(awakenHome, relative)
		if err := context.CheckSensitivePath(target); err == nil {
			t.Fatalf("%s must stay protected outside the projects carve-out", relative)
		}
		if _, err := context.ResolveMutationPathChecked(target); err == nil {
			t.Fatalf("mutation of %s must stay denied", relative)
		}
	}
	if _, err := context.ResolveACPCwd(filepath.Join(awakenHome, "config")); err == nil {
		t.Fatal("a protected subtree must not become a working directory")
	}
}

// The carve-out releases the enclosing business-data root, not the rules that
// apply inside any working directory.
func TestProtectedRootExceptionKeepsWorkingDirectoryRulesInsideTheCarveOut(t *testing.T) {
	context, _, managed := awakenHomeExceptionContext(t)

	for _, relative := range []string{
		filepath.Join(".git", "config"),
		filepath.Join(".awaken", "skills", "leak.md"),
		".env",
		filepath.Join("keys", "id_rsa"),
		filepath.Join("certs", "server.pem"),
		filepath.Join("deploy", "credentials.json"),
	} {
		if err := context.CheckSensitivePath(filepath.Join(managed, relative)); err == nil {
			t.Fatalf("%s must stay protected inside the managed workspace", relative)
		}
	}
}

func TestWithoutAnExceptionTheAwakenHomeRemainsFullyProtected(t *testing.T) {
	context, _, managed := awakenHomeExceptionContext(t)
	context.ProtectedRootExceptions = nil
	context = context.Normalize()

	if err := context.CheckSensitivePath(filepath.Join(managed, "notes.md")); err == nil {
		t.Fatal("without the carve-out the managed workspace must be denied like the rest of the root")
	}
}

func TestPluginRepositoryExceptionAllowsPackageAndDataWritesButNotStateOrSensitiveShapes(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	pluginsRoot := filepath.Join(awakenHome, "plugins")
	workspace := filepath.Join(home, "workspace")
	for _, dir := range []string{
		workspace,
		filepath.Join(pluginsRoot, "packages", "acme", "skills", "helper"),
		filepath.Join(pluginsRoot, "data", "acme"),
		filepath.Join(awakenHome, "config"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	permission := PermissionContext{
		ScopeID:                 "scope_plugin_authoring",
		WorkspaceRoot:           workspace,
		ReadableRoots:           []string{workspace},
		WritableRoots:           []string{workspace},
		OutputRoots:             []string{workspace},
		ProtectedRoots:          []string{awakenHome},
		ProtectedRootExceptions: []string{pluginsRoot},
		TemporaryWorkRoot:       filepath.Join(t.TempDir(), "scope"),
	}.Normalize()

	for _, target := range []string{
		filepath.Join(pluginsRoot, "packages", "acme", "plugin.json"),
		filepath.Join(pluginsRoot, "packages", "acme", "skills", "helper", "SKILL.md"),
		filepath.Join(pluginsRoot, "data", "acme", "cache.json"),
	} {
		if _, err := permission.ResolveMutationPathChecked(target); err != nil {
			t.Fatalf("Plugin repository target %q must be writable: %v", target, err)
		}
	}

	for _, target := range []string{
		filepath.Join(awakenHome, "config", "plugins.json"),
		filepath.Join(pluginsRoot, "packages", "acme", ".env"),
		filepath.Join(pluginsRoot, "packages", "acme", ".git", "config"),
		filepath.Join(pluginsRoot, "data", "acme", "credentials.json"),
	} {
		if _, err := permission.ResolveMutationPathChecked(target); err == nil {
			t.Fatalf("sensitive or Store-owned target %q must stay protected", target)
		}
	}

	outside := t.TempDir()
	ancestorEscape := filepath.Join(pluginsRoot, "packages", "escape")
	if err := os.Symlink(outside, ancestorEscape); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	leafEscape := filepath.Join(pluginsRoot, "packages", "leaf-escape.txt")
	if err := os.Symlink(outsideFile, leafEscape); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		filepath.Join(ancestorEscape, "new.txt"),
		leafEscape,
	} {
		if _, err := permission.ResolveMutationPathChecked(target); err == nil {
			t.Fatalf("Plugin repository symlink escape %q must stay protected", target)
		}
	}
}

func TestPluginRepositoryExceptionRootCannotBeReplacedBySymlink(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	if err := os.MkdirAll(awakenHome, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	pluginsRoot := filepath.Join(awakenHome, "plugins")
	if err := os.Symlink(outside, pluginsRoot); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	permission := PermissionContext{
		ScopeID:                 "scope_plugin_root_symlink",
		WorkspaceRoot:           workspace,
		ReadableRoots:           []string{workspace},
		WritableRoots:           []string{workspace},
		OutputRoots:             []string{workspace},
		ProtectedRoots:          []string{awakenHome},
		ProtectedRootExceptions: []string{pluginsRoot},
		TemporaryWorkRoot:       filepath.Join(t.TempDir(), "scope"),
	}.Normalize()
	if _, err := permission.ResolveMutationPathChecked(filepath.Join(pluginsRoot, "packages", "escape", "plugin.json")); err == nil {
		t.Fatal("a symlinked Plugin repository root must not project write access outside AWAKEN_HOME")
	}
}

func TestPluginRepositoryExceptionRejectsExternalSymlinkIngress(t *testing.T) {
	home := t.TempDir()
	awakenHome := filepath.Join(home, ".awaken")
	pluginsRoot := filepath.Join(awakenHome, "plugins")
	packageRoot := filepath.Join(pluginsRoot, "packages", "acme")
	workspace := filepath.Join(home, "workspace")
	for _, dir := range []string{packageRoot, workspace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(home, "plugins-alias")
	if err := os.Symlink(pluginsRoot, alias); err != nil {
		t.Fatal(err)
	}
	permission := PermissionContext{
		ScopeID:                 "scope_plugin_symlink_ingress",
		WorkspaceRoot:           workspace,
		ReadableRoots:           []string{workspace},
		WritableRoots:           []string{workspace},
		OutputRoots:             []string{workspace},
		ProtectedRoots:          []string{awakenHome},
		ProtectedRootExceptions: []string{pluginsRoot},
		TemporaryWorkRoot:       filepath.Join(t.TempDir(), "scope"),
	}.Normalize()
	if _, err := permission.ResolveMutationPathChecked(filepath.Join(alias, "packages", "acme", "plugin.json")); err == nil {
		t.Fatal("an external symlink must not provide an alternate lexical path into the Plugin repository")
	}
}
