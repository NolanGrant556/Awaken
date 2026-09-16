package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"awaken/src/tool/sandbox"
)

type recordingFullVersionProbeSandbox struct {
	applied int
	env     []string
}

func (*recordingFullVersionProbeSandbox) SupportLevel() sandbox.SandboxSupport {
	return sandbox.SandboxFull
}

func (s *recordingFullVersionProbeSandbox) ApplySandbox(cmd *exec.Cmd, _ string, _ string) (sandbox.SandboxResult, error) {
	s.applied++
	s.env = append([]string(nil), cmd.Env...)
	return sandbox.SandboxResult{Support: sandbox.SandboxFull, Method: "test-applied", Applied: true}, nil
}

func TestProbeExecutionEnvironmentIsBoundedDeterministicAndRedacted(t *testing.T) {
	t.Setenv("AWAKEN_USER_DATA_DIR", t.TempDir())
	t.Setenv("HTTP_PROXY", "Bearer proxy-secret-must-not-appear")
	fixedTime := time.Date(2026, 7, 12, 1, 2, 3, 0, time.UTC)
	lookPath := func(name string) (string, error) {
		switch name {
		case "demo":
			return filepath.Join(string(filepath.Separator), "safe", "bin", "demo"), nil
		case "npm":
			return filepath.Join(string(filepath.Separator), "other", "bin", "npm"), nil
		default:
			return "", errors.New("not found")
		}
	}
	probeVersion := func(context.Context, string, int) (string, error) {
		return "demo 1.0 sk-super-secret-token\nignored second line", nil
	}
	options := ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: "/workspace", WorkingDirectory: "/workspace/subdir",
		PATH: "/safe/bin:/other/bin:/safe/bin", PATHSource: "test_profile",
		Shell: "/bin/test-shell", Executables: []string{"demo"}, PackageManagers: []string{"npm"},
		ProbeVersions: true, LookPath: lookPath, ProbeVersion: probeVersion, Now: func() time.Time { return fixedTime },
	}
	first := ProbeExecutionEnvironment(context.Background(), options)
	second := ProbeExecutionEnvironment(context.Background(), options)
	if first.SnapshotID == "" || first.SnapshotID != second.SnapshotID {
		t.Fatalf("expected stable snapshot identity, first=%q second=%q", first.SnapshotID, second.SnapshotID)
	}
	if first.PATHSource != "test_profile" || len(first.PATH) != 2 || len(first.AvailablePackageManagers) != 1 || first.AvailablePackageManagers[0] != "npm" {
		t.Fatalf("unexpected bounded environment facts: %+v", first)
	}
	demo, ok := first.Executable("demo")
	if !ok || strings.Contains(demo.Version, "super-secret-token") || !strings.Contains(demo.Version, "[REDACTED]") {
		t.Fatalf("version probe was not sanitized: %+v", demo)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	wire := string(encoded)
	if strings.Contains(wire, "proxy-secret-must-not-appear") || strings.Contains(wire, "super-secret-token") {
		t.Fatalf("snapshot leaked a secret value: %s", wire)
	}
	if !strings.Contains(wire, "HTTP_PROXY") {
		t.Fatalf("snapshot should preserve only the reference name: %s", wire)
	}
	if !containsEnvironmentReference(first.ProxyReferences, "HTTP_PROXY") {
		t.Fatalf("proxy references were not typed: %+v", first.ProxyReferences)
	}
	if first.NativeArchitecture == "" || first.Translation == "" {
		t.Fatalf("platform architecture facts missing: %+v", first)
	}
}

func TestDefaultEnvironmentExecutablesIncludeWindowsCommandAndPythonLaunchers(t *testing.T) {
	wanted := map[string]bool{"cmd": false, "py": false, "python": false, "python3": false}
	for _, name := range defaultEnvironmentExecutables {
		if _, ok := wanted[name]; ok {
			wanted[name] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Fatalf("default executable %q is missing", name)
		}
	}
}

func containsEnvironmentReference(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestProbeExecutionEnvironmentFingerprintsGitForTypedRemoteExecution(t *testing.T) {
	gitExecutable := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(gitExecutable, []byte("pinned git bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		PATH: filepath.Dir(gitExecutable), Executables: []string{"git"}, PackageManagers: []string{},
		LookPath: func(name string) (string, error) {
			if name == "git" {
				return gitExecutable, nil
			}
			return "", os.ErrNotExist
		},
	})
	observation, ok := snapshot.Executable("git")
	realGitExecutable, err := filepath.EvalSymlinks(gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	wantProvenance := "path_lookup_resolved"
	if runtime.GOOS == "darwin" {
		realGitExecutable = resolveDarwinDeveloperGit()
		wantProvenance = "host_trusted_developer_tool"
	}
	if !ok || observation.Path != realGitExecutable || observation.Fingerprint == "" || observation.Provenance != wantProvenance {
		t.Fatalf("Git path/fingerprint was not pinned in environment snapshot: %+v", observation)
	}
	want, err := FingerprintExecutable(realGitExecutable)
	if err != nil || observation.Fingerprint != want {
		t.Fatalf("Git fingerprint mismatch: got=%q want=%q err=%v", observation.Fingerprint, want, err)
	}
}

func TestProbeExecutionEnvironmentCanonicalizesPATHSymlinkBeforeFingerprintAndTransport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires Unix executable semantics")
	}
	directory := t.TempDir()
	firstTarget := filepath.Join(directory, "demo-first")
	secondTarget := filepath.Join(directory, "demo-second")
	link := filepath.Join(directory, "demo-link")
	if err := os.WriteFile(firstTarget, []byte("first trusted bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondTarget, []byte("second changed bytes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(firstTarget, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	realFirstTarget, err := filepath.EvalSymlinks(firstTarget)
	if err != nil {
		t.Fatal(err)
	}
	realSecondTarget, err := filepath.EvalSymlinks(secondTarget)
	if err != nil {
		t.Fatal(err)
	}
	probe := func() ExecutionEnvironmentSnapshot {
		return ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
			PATH: directory, Executables: []string{}, PackageManagers: []string{"demo"},
			LookPath: func(string) (string, error) { return link, nil },
		})
	}
	first := probe()
	observedFirst, ok := first.Executable("demo")
	if !ok || observedFirst.Path != realFirstTarget || observedFirst.Provenance != "path_lookup_resolved" || observedFirst.PathIndex != 0 || observedFirst.Fingerprint == "" {
		t.Fatalf("PATH symlink was not pinned to its canonical target: %+v", observedFirst)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secondTarget, link); err != nil {
		t.Fatal(err)
	}
	stillFirst, err := FingerprintExecutable(observedFirst.Path)
	if err != nil || stillFirst != observedFirst.Fingerprint {
		t.Fatalf("swapping the PATH symlink changed the pinned real executable: got=%q want=%q err=%v", stillFirst, observedFirst.Fingerprint, err)
	}
	second := probe()
	observedSecond, ok := second.Executable("demo")
	if !ok || observedSecond.Path != realSecondTarget || observedSecond.Fingerprint == observedFirst.Fingerprint {
		t.Fatalf("new snapshot did not observe the swapped canonical target: first=%+v second=%+v", observedFirst, observedSecond)
	}
}

func TestVersionProbeUsesMinimalEnvironmentWithoutSecretCanary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shebang")
	}
	t.Setenv("SECRET_CANARY", "version-probe-secret-canary")
	trustedFixtureRoot := t.TempDir()
	fixture := filepath.Join(trustedFixtureRoot, "version-fixture")
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\nprintf 'fixture 1.0 secret=%s\\n' \"${SECRET_CANARY-}\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingFullVersionProbeSandbox{}
	sandbox.SetAdapter(adapter)
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		PATH: trustedFixtureRoot, Executables: []string{"fixture"}, PackageManagers: []string{}, ProbeVersions: true,
		ProbeTimeout: 5 * time.Second,
		LookPath:     func(string) (string, error) { return fixture, nil },
		ProbeVersion: func(ctx context.Context, executable string, maxOutputBytes int) (string, error) {
			return boundedVersionProbeWithPolicy(ctx, executable, maxOutputBytes, versionProbePolicy{
				trustedRoots: []string{trustedFixtureRoot}, pathEntries: []string{trustedFixtureRoot},
			})
		},
	})
	observation, ok := snapshot.Executable("fixture")
	if !ok || !strings.Contains(observation.Version, "fixture 1.0 secret=") || adapter.applied != 1 {
		t.Fatalf("minimal environment fixture did not execute through the applied boundary: observation=%+v adapter=%+v", observation, adapter)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	joinedEnvironment := strings.Join(adapter.env, "\n")
	if strings.Contains(string(encoded), "version-probe-secret-canary") || strings.Contains(joinedEnvironment, "SECRET_CANARY") ||
		strings.Contains(observation.Version, "version-probe-secret-canary") || strings.Contains(observation.Warning, "version-probe-secret-canary") {
		t.Fatalf("version probe inherited or recorded the secret canary: snapshot=%s env=%s", encoded, joinedEnvironment)
	}
	for _, required := range []string{"HOME=", "TMPDIR=", "PATH=", "LANG=C", "LC_ALL=C", "NO_COLOR=1"} {
		if !strings.Contains(joinedEnvironment, required) {
			t.Fatalf("minimal version environment missing %q: %s", required, joinedEnvironment)
		}
	}
}

func TestVersionProbeSkipsTaskOrUserExecutableButRetainsFingerprint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shebang")
	}
	t.Setenv("SECRET_CANARY", "untrusted-version-secret")
	directory := t.TempDir()
	marker := filepath.Join(directory, "executed")
	fixture := filepath.Join(directory, "untrusted")
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\ntouch '"+marker+"'\nprintf '%s\\n' \"${SECRET_CANARY-}\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingFullVersionProbeSandbox{}
	sandbox.SetAdapter(adapter)
	t.Cleanup(func() { sandbox.SetAdapter(nil) })
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		PATH: directory, Executables: []string{"untrusted"}, PackageManagers: []string{}, ProbeVersions: true,
		LookPath: func(string) (string, error) { return fixture, nil },
	})
	observation, ok := snapshot.Executable("untrusted")
	if !ok || observation.Fingerprint == "" || observation.Version != "" || !strings.Contains(observation.Warning, "outside host-trusted") || adapter.applied != 0 {
		t.Fatalf("untrusted executable was not retained without transport: observation=%+v adapter=%+v", observation, adapter)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("untrusted version fixture executed: %v", err)
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "untrusted-version-secret") {
		t.Fatalf("untrusted fixture leaked the process environment: %s", encoded)
	}
}

func TestDarwinDeveloperGitIgnoresMaliciousDeveloperDir(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin developer tool resolution only")
	}
	maliciousRoot := t.TempDir()
	maliciousGit := filepath.Join(maliciousRoot, "usr", "bin", "git")
	if err := os.MkdirAll(filepath.Dir(maliciousGit), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(maliciousGit, []byte("#!/bin/sh\necho malicious\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVELOPER_DIR", maliciousRoot)
	resolved := resolveDarwinDeveloperGit()
	if resolved == maliciousGit || !isAllowedDarwinDeveloperGitPath(resolved) {
		t.Fatalf("malicious DEVELOPER_DIR influenced trusted Git resolution: %q", resolved)
	}
	path, provenance := resolveObservedExecutable("git", maliciousGit)
	if path == maliciousGit || provenance != "host_trusted_developer_tool" || !isAllowedDarwinDeveloperGitPath(path) {
		t.Fatalf("PATH Git was not overridden by a trusted Developer tool: path=%q provenance=%q", path, provenance)
	}
}

func TestProbeExecutionEnvironmentDiscoversWorkspaceMarkersWithoutFollowingSymlinks(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, ".venv")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: workspace, Executables: []string{}, PackageManagers: []string{},
	})
	if len(snapshot.WorkspaceEnvironments) != 1 || snapshot.WorkspaceEnvironments[0].Path != "pyproject.toml" {
		t.Fatalf("unexpected workspace environment facts: %+v", snapshot.WorkspaceEnvironments)
	}
	if len(snapshot.Warnings) == 0 || !strings.Contains(snapshot.Warnings[0], "symlinked") {
		t.Fatalf("expected symlink marker warning: %+v", snapshot.Warnings)
	}
}

func TestProbeExecutionEnvironmentCapsPathAndProbeOutput(t *testing.T) {
	pathEntries := make([]string, 140)
	for index := range pathEntries {
		pathEntries[index] = filepath.Join("/bounded", string(rune('a'+index%26)), string(rune('A'+index/26)))
	}
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		PATH: strings.Join(pathEntries, string(filepath.ListSeparator)), Executables: []string{}, PackageManagers: []string{},
	})
	if len(snapshot.PATH) > 128 {
		t.Fatalf("PATH probe exceeded bound: %d", len(snapshot.PATH))
	}
	if !snapshot.PATHTruncated || snapshot.PATHEntryCount != len(pathEntries) || len(snapshot.PinnedTransportPATHEntries()) != len(pathEntries) {
		t.Fatalf("full transport PATH was not retained behind bounded metadata: preview=%d count=%d transport=%d truncated=%v", len(snapshot.PATH), snapshot.PATHEntryCount, len(snapshot.PinnedTransportPATHEntries()), snapshot.PATHTruncated)
	}
	if !strings.HasPrefix(snapshot.PATHDigest, "sha256:") {
		t.Fatalf("full PATH digest missing: %q", snapshot.PATHDigest)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), pathEntries[len(pathEntries)-1]) {
		t.Fatalf("non-preview transport PATH tail leaked into serialized snapshot: %s", encoded)
	}
	var restored ExecutionEnvironmentSnapshot
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.PinnedTransportPATH() != "" {
		t.Fatal("serialized truncated snapshot fabricated an executable transport PATH from its preview")
	}
	if clone := snapshot.Clone(); clone.PinnedTransportPATH() != snapshot.PinnedTransportPATH() {
		t.Fatal("snapshot clone lost the pinned transport PATH")
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
}

func TestExecutionEnvironmentSnapshotCarriesBoundedRootsAndRefreshIdentityCloneSafely(t *testing.T) {
	fixedTime := time.Date(2026, 7, 12, 3, 0, 0, 0, time.UTC)
	skillRoots := []SkillRootObservation{
		{Source: "project", Path: "/workspace/.awaken/skills"},
		{Source: "managed-sk-super-secret", Path: "/Library/Application Support/Awaken/skills"},
		{Source: "invalid", Path: "relative/skills"},
	}
	options := ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: "/workspace", WorkingDirectory: "/workspace",
		PATH: "/usr/bin", PATHSource: "explicit_test", Shell: "/bin/zsh",
		SkillRoots: skillRoots, TemporaryWorkRoot: "/private/tmp/awaken-work",
		RefreshReason: "dependency_install:plan sk-refresh-secret",
		Executables:   []string{}, PackageManagers: []string{}, Now: func() time.Time { return fixedTime },
	}
	first := ProbeExecutionEnvironment(context.Background(), options)
	options.RefreshReason = "explicit_refresh"
	second := ProbeExecutionEnvironment(context.Background(), options)
	if first.SnapshotID == second.SnapshotID {
		t.Fatalf("refresh reason did not participate in snapshot identity: %q", first.SnapshotID)
	}
	if len(first.SkillRoots) != 2 || first.TemporaryWorkRoot != "/private/tmp/awaken-work" || first.RefreshReason == "" {
		t.Fatalf("environment metadata was not retained: %+v", first)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret") || !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("root source/refresh reason metadata was not redacted: %s", encoded)
	}
	if !strings.Contains(strings.Join(first.Warnings, "\n"), "Skill root") {
		t.Fatalf("invalid relative Skill root was not reported: %+v", first.Warnings)
	}
	clone := first.Clone()
	clone.SkillRoots[0].Path = "/tampered"
	clone.PATH[0] = "/tampered"
	if first.SkillRoots[0].Path == "/tampered" || first.PATH[0] == "/tampered" {
		t.Fatalf("environment clone aliases mutable metadata: original=%+v clone=%+v", first, clone)
	}
}

func TestExecutionEnvironmentSnapshotCapsSkillRootsAndRejectsInvalidTemporaryRoot(t *testing.T) {
	roots := make([]SkillRootObservation, 0, maxEnvironmentSkillRoots+5)
	for index := 0; index < maxEnvironmentSkillRoots+5; index++ {
		roots = append(roots, SkillRootObservation{Source: "additional", Path: fmt.Sprintf("/skills/%03d", index)})
	}
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		PATH: "/usr/bin", SkillRoots: roots, TemporaryWorkRoot: "relative/tmp",
		Executables: []string{}, PackageManagers: []string{},
	})
	if len(snapshot.SkillRoots) != maxEnvironmentSkillRoots || snapshot.TemporaryWorkRoot != "" {
		t.Fatalf("root bounds were not enforced: roots=%d temp=%q", len(snapshot.SkillRoots), snapshot.TemporaryWorkRoot)
	}
	warnings := strings.Join(snapshot.Warnings, "\n")
	if !strings.Contains(warnings, "capped") || !strings.Contains(warnings, "temporary work root") {
		t.Fatalf("root bound diagnostics missing: %s", warnings)
	}
}

func TestSafeLoginShellPATHProbeParsesOnlyLiteralAssignmentsWithoutExecutingRC(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "must-not-exist")
	config := filepath.Join(home, ".zprofile")
	content := strings.Join([]string{
		`export PATH="$HOME/safe/bin:$PATH"`,
		`PATH="$(touch ` + marker + `):$PATH"`,
		`PATH='$HOME/single-quoted:$PATH'`,
		`echo this-file-must-never-run`,
	}, "\n")
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot := ProbeExecutionEnvironment(context.Background(), ExecutionEnvironmentProbeOptions{
		PATHSource: "process_environment", Shell: "/bin/zsh",
		EnableSafeLoginShellPATHProbe: true, LoginShellHome: home,
		LoginShellConfigPaths: []string{config},
		Executables:           []string{}, PackageManagers: []string{},
	})
	if snapshot.PATHSource != "shell_config_static_literal" || len(snapshot.PATH) == 0 || snapshot.PATH[0] != filepath.Join(home, "safe", "bin") {
		t.Fatalf("strict literal login PATH was not derived: %+v", snapshot)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shell rc command executed during static PATH probe: %v", err)
	}
	warnings := strings.Join(snapshot.Warnings, "\n")
	if !strings.Contains(warnings, "never executed") || !strings.Contains(warnings, "ignored 2") {
		t.Fatalf("login PATH provenance/rejection warning missing: %s", warnings)
	}
}

func TestSafeLoginShellPATHProbeCancellationAndInvalidConfigFallBackToProcessEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", "/process/only")
	ctx, cancel := context.WithCancel(context.Background())
	readerStarted := make(chan struct{})
	reader := func(ctx context.Context, _ string, _ int) ([]byte, error) {
		close(readerStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	go func() {
		<-readerStarted
		cancel()
	}()
	snapshot := ProbeExecutionEnvironment(ctx, ExecutionEnvironmentProbeOptions{
		Shell: "/bin/zsh", EnableSafeLoginShellPATHProbe: true, LoginShellHome: home,
		LoginShellConfigPaths: []string{filepath.Join(home, ".zprofile"), "/outside/home/.zshrc"},
		ReadLoginShellFile:    reader, LoginShellProbeTimeout: time.Second,
		Executables: []string{}, PackageManagers: []string{},
	})
	if snapshot.PATHSource != "process_environment" || len(snapshot.PATH) != 1 || snapshot.PATH[0] != "/process/only" {
		t.Fatalf("cancelled probe did not fall back to process PATH: %+v", snapshot)
	}
	warnings := strings.Join(snapshot.Warnings, "\n")
	if !strings.Contains(warnings, "cancel") || !strings.Contains(warnings, "never sourcing shell rc") {
		t.Fatalf("cancel/invalid-config provenance missing: %s", warnings)
	}
}
