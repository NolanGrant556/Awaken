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
