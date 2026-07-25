package tool

import (
	"os"
	"path/filepath"
	"testing"
)

// Roots describe where a task works by default, not what it is allowed to
// touch. Confining the structured tools was never an effective boundary anyway,
// because Bash runs arbitrary commands with no path check, so the fence only
// pushed out-of-scope work into shell invocations. These tests pin the
// replacement contract: a named path is honoured wherever it points, while the
// shapes a mutation is never meant to hit stay denied.
func rootsAsDefaultsContext(t *testing.T, workspace string) PermissionContext {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return NewTaskScopePermissionContext(
		"scope-roots-as-defaults",
		workspace,
		[]string{workspace},
		[]string{workspace},
		[]string{workspace},
		nil,
		nil,
		filepath.Join(workspace, "tmp"),
	)
}

func TestPathsOutsideTheRootsResolveForReadsAndMutations(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	ctx := rootsAsDefaultsContext(t, workspace)

	outsideFile := filepath.Join(outside, "notes.md")
	if err := os.WriteFile(outsideFile, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ctx.ResolveReadablePathChecked(outsideFile); err != nil {
		t.Fatalf("reading a named path outside the roots must be allowed: %v", err)
	}
	if _, err := ctx.ResolveMutationPathChecked(outsideFile); err != nil {
		t.Fatalf("mutating a named path outside the roots must be allowed: %v", err)
	}
	if _, err := ctx.ResolveMutationPathChecked(filepath.Join(outside, "new", "file.txt")); err != nil {
		t.Fatalf("creating a path outside the roots must be allowed: %v", err)
	}
}

func TestSensitiveShapesStayDeniedForMutationsAnywhere(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	ctx := rootsAsDefaultsContext(t, workspace)

	for name, path := range map[string]string{
		"git internals outside the roots": filepath.Join(outside, ".git", "config"),
		"git internals inside the roots":  filepath.Join(workspace, ".git", "config"),
		"dotenv outside the roots":        filepath.Join(outside, ".env"),
		"private key outside the roots":   filepath.Join(outside, "id_rsa"),
		"key material by extension":       filepath.Join(outside, "server.pem"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ctx.ResolveMutationPathChecked(path); err == nil {
				t.Fatalf("expected %q to stay denied for mutation", path)
			}
		})
	}
}

// Reads keep reaching sensitive files, which is what they already did inside
// the roots; only mutations carry the shape policy.
func TestSensitiveShapesRemainReadable(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	ctx := rootsAsDefaultsContext(t, workspace)

	if _, err := ctx.ResolveReadablePathChecked(filepath.Join(outside, ".env")); err != nil {
		t.Fatalf("reads must not inherit the mutation shape policy: %v", err)
	}
}

// The primary folder still anchors relative paths, which is the part of the
// root model that survives: folders decide where work lands by default.
func TestRelativePathsStillResolveAgainstThePrimaryFolder(t *testing.T) {
	workspace := t.TempDir()
	ctx := rootsAsDefaultsContext(t, workspace)

	resolved, err := ctx.ResolveMutationPathChecked("docs/plan.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(workspace, "docs", "plan.md"); resolved != want {
		t.Fatalf("resolved=%q want=%q", resolved, want)
	}
}
