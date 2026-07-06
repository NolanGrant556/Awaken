package tool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// withIsolatedHome returns a PermissionContext whose workspace is a temp dir
// and a matcher whose HomeDir/ConfigDir point at a separate temp dir tree,
// so tests do not accidentally treat the developer's real ~/.awaken as
// sensitive.
func withIsolatedHome(t *testing.T) (PermissionContext, SensitivePathMatcher) {
	t.Helper()

	workspace := t.TempDir()
	home := t.TempDir()
	// os.UserConfigDir on macOS resolves to $HOME/Library/Application Support.
	// Set HOME so both os.UserHomeDir and os.UserConfigDir resolve under the
	// isolated temp dir. Also override XDG_CONFIG_HOME for Linux-shaped envs.
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	ctx := NewPermissionContext(workspace, nil)
	matcher := SensitivePathMatcher{
		HomeDir:   home,
		ConfigDir: filepath.Join(home, "Library", "Application Support"),
	}
	return ctx, matcher
}

func TestSensitivePathMatcherDeniesGitDirectory(t *testing.T) {
	ctx, _ := withIsolatedHome(t)
	cases := []string{
		".git/config",
		".git/HEAD",
		".git/hooks/pre-commit",
		"subdir/.git/objects/ab/cdef",
	}
	for _, rel := range cases {
		if !ctx.IsSensitivePath(rel) {
			t.Fatalf("expected %q to be sensitive", rel)
		}
		if err := ctx.CheckSensitivePath(rel); err == nil {
			t.Fatalf("expected CheckSensitivePath to return error for %q", rel)
		}
	}
}

func TestSensitivePathMatcherDeniesEnvFiles(t *testing.T) {
	ctx, _ := withIsolatedHome(t)
	cases := []string{
		".env",
		".env.local",
		".env.production",
		".env.example",
		"config/.env",
		"apps/web/.env.development",
	}
	for _, rel := range cases {
		if !ctx.IsSensitivePath(rel) {
			t.Fatalf("expected %q to be sensitive", rel)
		}
	}
}

func TestSensitivePathMatcherDeniesKeyAndCertificateFiles(t *testing.T) {
	ctx, _ := withIsolatedHome(t)
	cases := []string{
		"id_rsa",
		"id_rsa.pub",
		"id_ed25519",
		"id_ecdsa",
		"~/.ssh/id_rsa",
		"server.pem",
		"client.key",
		"cert.crt",
		"keystore.p12",
		"archive.pfx",
		"app.keystore",
		"java.jks",
		"server.pem.bak",
		"my-credentials.json",
		"service-secrets.yaml",
	}
	for _, rel := range cases {
		if !ctx.IsSensitivePath(rel) {
			t.Fatalf("expected %q to be sensitive", rel)
		}
	}
}

func TestSensitivePathMatcherDeniesAgentMetadataDir(t *testing.T) {
	ctx, matcher := withIsolatedHome(t)

	// ~/.awaken/permissions.json
	awakenPath := filepath.Join(matcher.HomeDir, ".awaken", "permissions.json")
	if !ctx.IsSensitivePath(awakenPath) {
		t.Fatalf("expected ~/.awaken/permissions.json to be sensitive")
	}
	// ~/.trae-cn/state.json
	traeCnPath := filepath.Join(matcher.HomeDir, ".trae-cn", "state.json")
	if !ctx.IsSensitivePath(traeCnPath) {
		t.Fatalf("expected ~/.trae-cn/state.json to be sensitive")
	}
}

func TestSensitivePathMatcherDeniesRuntimeConfigDir(t *testing.T) {
	ctx, matcher := withIsolatedHome(t)
	if matcher.ConfigDir == "" {
		t.Skip("os.UserConfigDir not available in this environment")
	}

	for _, name := range []string{"awaken", "Awaken"} {
		configPath := filepath.Join(matcher.ConfigDir, name, "config.toml")
		if !ctx.IsSensitivePath(configPath) {
			t.Fatalf("expected %q to be sensitive", configPath)
		}
	}
}

func TestSensitivePathMatcherAllowsNormalFiles(t *testing.T) {
	ctx, _ := withIsolatedHome(t)
	cases := []string{
		"src/main.go",
		"README.md",
		"docs/architecture.md",
		"package.json",
		"config/settings.toml",
		"test_data/fixtures.txt",
		"nested/deep/path/file.go",
	}
	for _, rel := range cases {
		if ctx.IsSensitivePath(rel) {
			t.Fatalf("expected %q to NOT be sensitive", rel)
		}
		if err := ctx.CheckSensitivePath(rel); err != nil {
			t.Fatalf("expected no error for %q, got %v", rel, err)
		}
	}
}

func TestSensitivePathMatcherAppliesRegardlessOfWorkspaceMembership(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	ctx := NewPermissionContext(workspace, nil)

	// A .git/config INSIDE the workspace is still sensitive.
	insideGit := filepath.Join(workspace, ".git", "config")
	if !ctx.IsSensitivePath(insideGit) {
		t.Fatalf("expected .git inside workspace to still be sensitive")
	}

	insideEnv := filepath.Join(workspace, "apps", "web", ".env")
	if !ctx.IsSensitivePath(insideEnv) {
		t.Fatalf("expected .env inside workspace to still be sensitive")
	}

	insideKey := filepath.Join(workspace, "deploy", "server.pem")
	if !ctx.IsSensitivePath(insideKey) {
		t.Fatalf("expected key file inside workspace to still be sensitive")
	}
}

func TestCheckSensitivePathReturnsStructuredError(t *testing.T) {
	ctx, _ := withIsolatedHome(t)

	err := ctx.CheckSensitivePath(".git/config")
	if err == nil {
		t.Fatalf("expected error for .git/config")
	}

	var sensitiveErr *SensitivePathError
	if !errors.As(err, &sensitiveErr) {
		t.Fatalf("expected *SensitivePathError, got %T: %v", err, err)
	}
	if sensitiveErr.Path == "" {
		t.Fatalf("expected Path to be populated, got %+v", sensitiveErr)
	}
	if sensitiveErr.Reason == "" {
		t.Fatalf("expected Reason to be populated, got %+v", sensitiveErr)
	}

	// errors.Is should match via the Is method.
	if !errors.Is(err, &SensitivePathError{Path: sensitiveErr.Path, Reason: sensitiveErr.Reason}) {
		t.Fatalf("errors.Is should match a SensitivePathError with same Path and Reason")
	}
	// errors.Is should not match a different SensitivePathError.
	if errors.Is(err, &SensitivePathError{Path: "/other", Reason: "other"}) {
		t.Fatalf("errors.Is should not match a different SensitivePathError")
	}
}

func TestResolveMutationPathCheckedRejectsSensitivePath(t *testing.T) {
	ctx, _ := withIsolatedHome(t)

	// .git/config inside workspace — ResolvePathChecked alone would allow it
	// (it's inside workspace), but ResolveMutationPathChecked must reject it.
	if _, err := ctx.ResolveMutationPathChecked(".git/config"); err == nil {
		t.Fatalf("expected ResolveMutationPathChecked to reject .git/config")
	}

	// A normal file resolves successfully. ResolvePathChecked invokes
	// EvalSymlinks, so the file must exist on disk.
	srcDir := filepath.Join(ctx.WorkspaceRoot, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}
	mainFile := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	resolved, err := ctx.ResolveMutationPathChecked("src/main.go")
	if err != nil {
		t.Fatalf("expected ResolveMutationPathChecked to allow src/main.go, got %v", err)
	}
	if resolved != mainFile {
		t.Fatalf("expected resolved path %q, got %q", mainFile, resolved)
	}
}

func TestSensitivePathMatcherSkipsHomeChecksWhenHomeUnknown(t *testing.T) {
	// When HomeDir is empty, the matcher should not panic and should still
	// apply lexical checks (.git, .env, extensions, tokens).
	matcher := SensitivePathMatcher{HomeDir: "", ConfigDir: ""}

	if sensitive, _ := matcher.IsSensitive("/tmp/.git/config"); !sensitive {
		t.Fatalf("expected .git to be sensitive even without HomeDir")
	}
	if sensitive, _ := matcher.IsSensitive("/tmp/server.pem"); !sensitive {
		t.Fatalf("expected .pem to be sensitive even without HomeDir")
	}
	if sensitive, _ := matcher.IsSensitive("/tmp/normal.txt"); sensitive {
		t.Fatalf("expected normal file to NOT be sensitive")
	}
}

func TestSensitivePathMatcherDoesNotBlockReads(t *testing.T) {
	// Reads use ResolvePathChecked (not the mutation variant) and must
	// continue to work for sensitive paths — only mutations are denied.
	ctx, _ := withIsolatedHome(t)

	// Create a real .git/config file so EvalSymlinks can resolve it.
	gitDir := filepath.Join(ctx.WorkspaceRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	gitConfig := filepath.Join(gitDir, "config")
	if err := os.WriteFile(gitConfig, []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("failed to write .git/config: %v", err)
	}

	// Reads via ResolvePathChecked must succeed.
	resolved, err := ctx.ResolvePathChecked(".git/config")
	if err != nil {
		t.Fatalf("expected ResolvePathChecked to allow reading .git/config, got %v", err)
	}
	if resolved != gitConfig {
		t.Fatalf("expected resolved %q, got %q", gitConfig, resolved)
	}

	// Mutations via ResolveMutationPathChecked must fail.
	if _, err := ctx.ResolveMutationPathChecked(".git/config"); err == nil {
		t.Fatalf("expected ResolveMutationPathChecked to deny .git/config")
	}
}
