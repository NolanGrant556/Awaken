package gitremote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/sandbox"
	"awaken/src/tool/trash"
)

const successfulGitScript = `mode=""
destination=""
for argument in "$@"; do
  if [ "$argument" = "ls-remote" ] || [ "$argument" = "clone" ]; then mode="$argument"; fi
  destination="$argument"
done
if [ "$mode" = "ls-remote" ]; then
  printf '0123456789abcdef0123456789abcdef01234567\trefs/heads/main\n'
  exit 0
fi
test "$mode" = "clone"
mkdir -p "$destination/.git"
printf 'ref: refs/heads/main\n' > "$destination/.git/HEAD"
printf '[remote "origin"]\n' > "$destination/.git/config"
printf 'safe clone\n' > "$destination/README.md"
printf '%s\n' "$@" > "$destination/argv.txt"
env | sort > "$destination/env.txt"
`

type appliedSandbox struct {
	mu    sync.Mutex
	tiers []string
}

func (*appliedSandbox) SupportLevel() sandbox.SandboxSupport { return sandbox.SandboxFull }
func (s *appliedSandbox) ApplySandbox(_ *exec.Cmd, tier string, _ string) (sandbox.SandboxResult, error) {
	s.mu.Lock()
	s.tiers = append(s.tiers, tier)
	s.mu.Unlock()
	return sandbox.SandboxResult{Support: sandbox.SandboxFull, Applied: true, Method: "test-applied"}, nil
}

type degradedSandbox struct{}

func (*degradedSandbox) SupportLevel() sandbox.SandboxSupport { return sandbox.SandboxDegraded }
func (*degradedSandbox) ApplySandbox(*exec.Cmd, string, string) (sandbox.SandboxResult, error) {
	return sandbox.SandboxResult{Support: sandbox.SandboxDegraded, Applied: false, Method: "test-degraded"}, nil
}

type movingTrash struct {
	root  string
	mu    sync.Mutex
	calls int
}

type observedOutput struct {
	mu     sync.Mutex
	values []tool.CommandOutput
}

func (o *observedOutput) OnCommandOutput(value tool.CommandOutput) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.values = append(o.values, value)
}

func (a *movingTrash) TrashPath(path string) (trash.TrashResult, error) {
	a.mu.Lock()
	a.calls++
	call := a.calls
	a.mu.Unlock()
	destination := filepath.Join(a.root, fmt.Sprintf("recovery-%d", call))
	if err := os.Rename(path, destination); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: path, TrashedPath: destination, Method: "test-rename"}, nil
}

type gitFixture struct {
	base        string
	workspace   string
	git         string
	environment tool.ExecutionEnvironmentSnapshot
	snapshot    tool.RunToolAvailabilitySnapshot
	permission  tool.PermissionContext
	broker      *tool.WorkspaceExecutionBroker
	trash       *movingTrash
	adapter     *appliedSandbox
}

func newGitFixture(t *testing.T, script string, writableRoots, protectedRoots []string) gitFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix fake executable fixture; Windows production typed Git remains fail-closed without a full sandbox")
	}
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	gitExecutable := filepath.Join(bin, "git")
	if err := os.WriteFile(gitExecutable, []byte("#!/bin/sh\nset -eu\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	remoteHelper := filepath.Join(bin, "git-remote-https")
	if err := os.WriteFile(remoteHelper, []byte("#!/bin/sh\nprintf 'trusted-helper\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := tool.FingerprintExecutable(gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	environment := tool.ExecutionEnvironmentSnapshot{
		SnapshotID: "env_git_" + filepath.Base(base), OS: runtime.GOOS, Architecture: runtime.GOARCH,
		WorkspaceRoot: workspace, WorkingDirectory: workspace,
		PATH: []string{bin, "/usr/bin", "/bin"}, PATHSource: "test",
		Executables: []tool.ExecutableObservation{{Name: "git", Path: gitExecutable, Provenance: "test", Fingerprint: fingerprint}},
		Confidence:  "observed",
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	entry, err := snapshot.Lookup(toolName)
	if err != nil || entry.Readiness.Readiness != tool.ProviderReady {
		t.Fatalf("typed Git fixture is not ready: entry=%+v err=%v", entry, err)
	}
	permission := tool.NewTaskScopePermissionContext("git-remote-test", workspace, []string{workspace}, writableRoots, nil, nil, protectedRoots, "")
	trashAdapter := &movingTrash{root: filepath.Join(base, "trash")}
	if err := os.MkdirAll(trashAdapter.root, 0o700); err != nil {
		t.Fatal(err)
	}
	broker := tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{
		StagingParent: filepath.Join(base, "staging"), TrashAdapter: trashAdapter,
	})
	return gitFixture{
		base: base, workspace: workspace, git: gitExecutable, environment: environment, snapshot: snapshot,
		permission: permission, broker: broker, trash: trashAdapter, adapter: &appliedSandbox{},
	}
}

func (f gitFixture) context() context.Context {
	ctx := tool.ContextWithRunAvailability(context.Background(), f.snapshot, f.environment)
	ctx = tool.ContextWithExecutionEnvironmentSnapshot(ctx, f.environment)
	ctx = tool.ContextWithPermissionContext(ctx, f.permission)
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_git", ToolCallID: "call_git", ToolName: toolName})
	return tool.ContextWithExecutionBroker(ctx, f.broker)
}

func publicResolver(context.Context, string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("93.184.216.34")}, nil
}

func noProxyEnv(string) (string, bool) { return "", false }

func allowTestGit(observation tool.ExecutableObservation) (gitRuntimeTrust, error) {
	execPath := filepath.Dir(observation.Path)
	helper := filepath.Join(execPath, "git-remote-https")
	fingerprint, err := tool.FingerprintExecutable(helper)
	if err != nil {
		return gitRuntimeTrust{}, err
	}
	return gitRuntimeTrust{ExecPath: execPath, RemoteHelper: helper, HelperFingerprint: fingerprint}, nil
}

func executeFixture(t *testing.T, fixture gitFixture, params input, adapter sandbox.SandboxAdapter) tool.ExecutionOutcome {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	if adapter == nil {
		adapter = fixture.adapter
	}
	outcome, err := (&Tool{adapter: adapter, resolveHost: publicResolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(fixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestGitRemoteURLAndDNSBoundaryRejectsUnsafeTargetsBeforeTransport(t *testing.T) {
	valid, err := normalizeRemoteURL("https://github.com/example/project.git")
	if err != nil || valid != "https://github.com/example/project.git" {
		t.Fatalf("valid URL rejected: value=%q err=%v", valid, err)
	}
	unsafe := []string{
		"http://github.com/example/project.git", "ssh://git@github.com/example/project.git",
		"git://github.com/example/project.git", "file:///tmp/project", "https://user@github.com/example/project.git",
		"https://user:secret@github.com/example/project.git", "https://github.com/example/project.git?token=secret",
		"https://github.com/example/project.git#main", "https://localhost/project.git", "https://127.0.0.1/project.git",
		"https://10.0.0.2/project.git", "https://100.64.0.1/project.git", "https://198.18.0.1/project.git",
		"https://203.0.113.10/project.git", "https://[fc00::1]/project.git", "https://metadata.internal/project.git",
	}
	for _, value := range unsafe {
		if _, err := normalizeRemoteURL(value); err == nil {
			t.Errorf("unsafe URL accepted: %s", value)
		}
	}

	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	resolverCalls := 0
	resolver := func(context.Context, string) ([]net.IP, error) {
		resolverCalls++
		return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("127.0.0.1")}, nil
	}
	raw := `{"operation":"ls_remote","url":"https://github.com/example/project.git"}`
	outcome, err := (&Tool{adapter: fixture.adapter, resolveHost: resolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(fixture.context(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolverCalls != 1 || outcome.TransportStatus != tool.TransportNotStarted || outcome.Status != tool.OutcomeDenied {
		t.Fatalf("mixed public/private DNS did not fail before Git transport: calls=%d outcome=%+v", resolverCalls, outcome)
	}
}

func TestGitRemoteExplicitProxyRoutesFakeIPAndClearsBypassWithoutLeakingSecret(t *testing.T) {
	proxyScript := `mode=""
for argument in "$@"; do
  if [ "$argument" = "ls-remote" ]; then mode="$argument"; fi
done
test "$mode" = "ls-remote"
printf '0123456789abcdef0123456789abcdef01234567\trefs/heads/main\n'
printf 'proxy=%s no_proxy=[%s] lower_no_proxy=[%s]\n' "$HTTPS_PROXY" "${NO_PROXY-}" "${no_proxy-}" >&2`
	fixture := newGitFixture(t, proxyScript, nil, nil)
	values := map[string]string{
		"HTTPS_PROXY": "http://alice:proxy-secret@127.0.0.1:7897",
		"NO_PROXY":    "github.com,127.0.0.1",
	}
	lookup := func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
	resolverCalls := 0
	fakeIPResolver := func(context.Context, string) ([]net.IP, error) {
		resolverCalls++
		return []net.IP{net.ParseIP("198.18.0.41")}, nil
	}
	observer := &observedOutput{}
	raw := `{"operation":"ls_remote","url":"https://github.com/example/project.git"}`
	outcome, err := (&Tool{adapter: fixture.adapter, resolveHost: fakeIPResolver, lookupEnv: lookup, trustGit: allowTestGit}).ExecuteOutcome(fixture.context(), raw, observer)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != tool.OutcomeSucceeded || resolverCalls != 0 {
		t.Fatalf("explicit local proxy did not bypass target fake-IP DNS safely: calls=%d outcome=%+v", resolverCalls, outcome)
	}
	if outcome.Diagnostic["proxy_source"] != "HTTPS_PROXY" || outcome.Diagnostic["proxy_routed"] != true {
		t.Fatalf("proxy route provenance is missing or contains the wrong fact: %+v", outcome.Diagnostic)
	}
	observer.mu.Lock()
	streamed := fmt.Sprint(observer.values)
	observer.mu.Unlock()
	combined := outcome.Content + fmt.Sprint(outcome.Details) + fmt.Sprint(outcome.Diagnostic) + streamed
	if strings.Contains(combined, "proxy-secret") || strings.Contains(combined, "alice:") {
		t.Fatalf("proxy credential leaked into outcome/observer: %s", combined)
	}
	if !strings.Contains(combined, "[REDACTED]") || !strings.Contains(combined, "no_proxy=[]") || !strings.Contains(combined, "lower_no_proxy=[]") {
		t.Fatalf("proxy secret redaction or NO_PROXY clearing evidence missing: %s", combined)
	}
	for _, invalid := range []string{"socks5://127.0.0.1:7897", "http://127.0.0.1", "http://127.0.0.1:7897/path", "file:///tmp/proxy"} {
		if _, err := parseExplicitGitProxy(invalid); err == nil {
			t.Errorf("unsafe/ambiguous proxy accepted: %s", invalid)
		}
	}
}

func TestGitRemoteLSRemoteUsesPinnedDirectArgvAndZeroPublishScope(t *testing.T) {
	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	outcome := executeFixture(t, fixture, input{Operation: operationLSRemote, URL: "https://github.com/example/project.git", Ref: "refs/heads/main"}, nil)
	if outcome.Status != tool.OutcomeSucceeded || outcome.TransportStatus != tool.TransportCompleted || outcome.ProcessStatus != tool.ProcessExited || outcome.ExitCode == nil || *outcome.ExitCode != 0 {
		t.Fatalf("unexpected ls_remote outcome: %+v", outcome)
	}
	refs, ok := outcome.Details["refs"].([]remoteRef)
	if !ok || len(refs) != 1 || refs[0].Name != "refs/heads/main" {
		t.Fatalf("ls_remote refs were not structured: %#v", outcome.Details["refs"])
	}
	if len(outcome.Changes) != 0 || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("ls_remote reported workspace mutation: %+v", outcome)
	}
	entries, err := os.ReadDir(fixture.workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("ls_remote changed live workspace: entries=%v err=%v", entries, err)
	}
	fixture.adapter.mu.Lock()
	tiers := append([]string(nil), fixture.adapter.tiers...)
	fixture.adapter.mu.Unlock()
	if len(tiers) != 1 || tiers[0] != sandbox.TierOpenWorldSystem {
		t.Fatalf("ls_remote did not use full open-world sandbox: %v", tiers)
	}
}

func TestGitRemoteCloneCommitsOnlyDestinationWithHardenedEnvironment(t *testing.T) {
	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	if err := os.MkdirAll(filepath.Join(fixture.workspace, "vendor"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_TOKEN", "must-not-reach-git")
	t.Setenv("GIT_CONFIG_COUNT", "99")
	outcome := executeFixture(t, fixture, input{
		Operation: operationClone, URL: "https://github.com/example/project.git", Ref: "main", Destination: "vendor/project",
	}, nil)
	if outcome.Status != tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("unexpected clone outcome: %+v", outcome)
	}
	readme, err := os.ReadFile(filepath.Join(fixture.workspace, "vendor", "project", "README.md"))
	if err != nil || string(readme) != "safe clone\n" {
		t.Fatalf("clone destination was not committed: content=%q err=%v", readme, err)
	}
	argv, err := os.ReadFile(filepath.Join(fixture.workspace, "vendor", "project", "argv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"credential.helper=", "protocol.allow=never", "protocol.https.allow=always", "--no-recurse-submodules", "--depth=1", "--branch", "main"} {
		if !strings.Contains(string(argv), required) {
			t.Errorf("clone argv lacks %q:\n%s", required, argv)
		}
	}
	environment, err := os.ReadFile(filepath.Join(fixture.workspace, "vendor", "project", "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	joined := string(environment)
	for _, forbidden := range []string{"GITHUB_TOKEN", "GIT_CONFIG_COUNT=99", "must-not-reach-git"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("ambient credential/config leaked to Git: %s", joined)
		}
	}
	for _, required := range []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1"} {
		if !strings.Contains(joined, required) {
			t.Errorf("isolated Git environment lacks %q: %s", required, joined)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.workspace, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clone wrote outside destination: %v", err)
	}
}

func TestGitRemoteCloneReplaceIsRecoverableAndRepeatable(t *testing.T) {
	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	destination := filepath.Join(fixture.workspace, "project")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "old.txt"), []byte("old bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	params := input{Operation: operationClone, URL: "https://github.com/example/project.git", Destination: "project", ReplaceExisting: true}
	for iteration := 0; iteration < 2; iteration++ {
		outcome := executeFixture(t, fixture, params, nil)
		if outcome.Status != tool.OutcomeSucceeded {
			t.Fatalf("replace iteration %d failed: %+v", iteration, outcome)
		}
		if iteration == 0 {
			foundRecovery := false
			for _, change := range outcome.Changes {
				if strings.HasSuffix(change.Resource, "old.txt") && change.Reversibility == tool.ChangeReversible {
					foundRecovery = true
				}
			}
			if !foundRecovery {
				t.Fatalf("replaced preimage lacks Trash recovery evidence: %+v", outcome.Changes)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "README.md")); err != nil {
		t.Fatalf("repeatable replacement lost clone: %v", err)
	}
	fixture.trash.mu.Lock()
	trashCalls := fixture.trash.calls
	fixture.trash.mu.Unlock()
	if trashCalls < 2 {
		t.Fatalf("repeat replacements did not Trash preimages: calls=%d", trashCalls)
	}
}

func TestGitRemoteCloneNonzeroTimeoutAndCancellationDiscardPartialStage(t *testing.T) {
	partialScript := `mode=""
destination=""
for argument in "$@"; do
  if [ "$argument" = "clone" ]; then mode="$argument"; fi
  destination="$argument"
done
test "$mode" = "clone"
mkdir -p "$destination/.git"
printf partial > "$destination/partial.txt"
exit 7`
	fixture := newGitFixture(t, partialScript, nil, nil)
	outcome := executeFixture(t, fixture, input{Operation: operationClone, URL: "https://github.com/example/project.git", Destination: "project"}, nil)
	if outcome.Status != tool.OutcomeFailed || outcome.ErrorCode != tool.ErrorCodeProcessExitNonzero || outcome.TransportStatus != tool.TransportCompleted {
		t.Fatalf("nonzero Git process truth lost: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, "project")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nonzero partial clone reached live workspace: %v", err)
	}

	timeoutScript := strings.Replace(partialScript, "exit 7", "sleep 5", 1)
	timeoutFixture := newGitFixture(t, timeoutScript, nil, nil)
	raw, _ := json.Marshal(input{Operation: operationClone, URL: "https://github.com/example/project.git", Destination: "project"})
	timeoutOutcome, err := (&Tool{adapter: timeoutFixture.adapter, timeout: 25 * time.Millisecond, resolveHost: publicResolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(timeoutFixture.context(), string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if timeoutOutcome.ErrorCode != tool.ErrorCodeProcessTimeout || timeoutOutcome.ProcessStatus != tool.ProcessTimedOut {
		t.Fatalf("timeout Git process truth lost: %+v", timeoutOutcome)
	}
	if _, err := os.Lstat(filepath.Join(timeoutFixture.workspace, "project")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("timeout partial clone reached live workspace: %v", err)
	}

	cancelFixture := newGitFixture(t, timeoutScript, nil, nil)
	ctx, cancel := context.WithCancel(cancelFixture.context())
	timer := time.AfterFunc(25*time.Millisecond, cancel)
	defer timer.Stop()
	cancelOutcome, err := (&Tool{adapter: cancelFixture.adapter, resolveHost: publicResolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(ctx, string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	if cancelOutcome.Status != tool.OutcomeCancelled || cancelOutcome.ErrorCode != tool.ErrorCodeToolCancelled || cancelOutcome.ProcessStatus != tool.ProcessSignaled {
		t.Fatalf("cancelled Git process truth lost: %+v", cancelOutcome)
	}
	if _, err := os.Lstat(filepath.Join(cancelFixture.workspace, "project")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled partial clone reached live workspace: %v", err)
	}
}

func TestGitRemoteRejectsForgedSnapshotPathReplacementFreeArgsAndUnsafeScope(t *testing.T) {
	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	invalidInputs := []string{
		`{"operation":"clone","url":"https://github.com/example/project.git","destination":"project","args":["push"]}`,
		`{"operation":"clone","url":"https://github.com/example/project.git","destination":"../outside"}`,
		`{"operation":"ls_remote","url":"https://github.com/example/project.git","destination":"project"}`,
		`{"operation":"clone","url":"https://github.com/example/project.git","destination":"project"} {}`,
	}
	for _, raw := range invalidInputs {
		outcome, err := (&Tool{adapter: fixture.adapter, resolveHost: publicResolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(fixture.context(), raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.TransportStatus != tool.TransportNotStarted {
			t.Fatalf("invalid/free input started Git: raw=%s outcome=%+v", raw, outcome)
		}
	}

	staleEnvironment := fixture.environment.Clone()
	staleEnvironment.SnapshotID = "env_stale"
	staleCtx := tool.ContextWithRunAvailability(context.Background(), fixture.snapshot, staleEnvironment)
	staleCtx = tool.ContextWithPermissionContext(staleCtx, fixture.permission)
	staleCtx = tool.ContextWithExecutionBroker(staleCtx, fixture.broker)
	raw := `{"operation":"ls_remote","url":"https://github.com/example/project.git"}`
	staleOutcome, _ := (&Tool{adapter: fixture.adapter, resolveHost: publicResolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(staleCtx, raw, nil)
	if staleOutcome.TransportStatus != tool.TransportNotStarted {
		t.Fatalf("stale snapshot started Git: %+v", staleOutcome)
	}

	if err := os.WriteFile(fixture.git, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	replacedOutcome, _ := (&Tool{adapter: fixture.adapter, resolveHost: publicResolver, lookupEnv: noProxyEnv, trustGit: allowTestGit}).ExecuteOutcome(fixture.context(), raw, nil)
	if replacedOutcome.TransportStatus != tool.TransportNotStarted || replacedOutcome.ErrorCode != "git_snapshot_invalid" {
		t.Fatalf("replaced Git executable started transport: %+v", replacedOutcome)
	}

	narrow := newGitFixture(t, successfulGitScript, []string{"allowed"}, nil)
	if err := os.MkdirAll(filepath.Join(narrow.workspace, "allowed"), 0o700); err != nil {
		t.Fatal(err)
	}
	narrowOutcome := executeFixture(t, narrow, input{Operation: operationClone, URL: "https://github.com/example/project.git", Destination: "forbidden/project"}, nil)
	if narrowOutcome.TransportStatus != tool.TransportNotStarted || narrowOutcome.Status != tool.OutcomeDenied {
		t.Fatalf("clone outside TaskScope writable roots started transport: %+v", narrowOutcome)
	}
}

func TestGitRemoteBrokerRejectsOutsideDeltaAndRawShellForgeryBeforeTrash(t *testing.T) {
	escapeScript := `mode=""
destination=""
for argument in "$@"; do
  if [ "$argument" = "clone" ]; then mode="$argument"; fi
  destination="$argument"
done
test "$mode" = "clone"
mkdir -p "$destination/.git"
printf safe > "$destination/README.md"
workspace_root=$(dirname "$(dirname "$destination")")
printf escape > "$workspace_root/escape.txt"`
	fixture := newGitFixture(t, escapeScript, nil, nil)
	if err := os.MkdirAll(filepath.Join(fixture.workspace, "allowed"), 0o700); err != nil {
		t.Fatal(err)
	}
	outcome := executeFixture(t, fixture, input{Operation: operationClone, URL: "https://github.com/example/project.git", Destination: "allowed/project"}, nil)
	if outcome.Status == tool.OutcomeSucceeded || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectNone {
		t.Fatalf("outside-destination delta was not denied: %+v", outcome)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, "allowed", "project")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("denied clone reached live destination: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(fixture.workspace, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside delta reached live workspace: %v", err)
	}
	fixture.trash.mu.Lock()
	trashCalls := fixture.trash.calls
	fixture.trash.mu.Unlock()
	if trashCalls != 0 {
		t.Fatalf("scope denial invoked Trash %d time(s)", trashCalls)
	}

	fingerprint, err := tool.FingerprintExecutable(fixture.git)
	if err != nil {
		t.Fatal(err)
	}
	projection := &tool.BrokerGitRemotePlan{
		Operation: operationLSRemote, RemoteURL: "https://github.com/example/project.git", GitExecutable: fixture.git,
		GitExecutableFingerprint: fingerprint, ToolSnapshotID: fixture.snapshot.SnapshotID,
		EnvironmentSnapshotID: fixture.environment.SnapshotID,
	}
	trust, err := allowTestGit(tool.ExecutableObservation{Path: fixture.git})
	if err != nil {
		t.Fatal(err)
	}
	projection.GitExecPath = trust.ExecPath
	projection.GitRemoteHelper = trust.RemoteHelper
	projection.GitRemoteHelperFingerprint = trust.HelperFingerprint
	authorization, err := fixture.broker.Authorize(context.Background(), tool.ExecutionBrokerRequest{
		ProviderID: "builtin:shell", Capability: tool.CapabilityShellExecute, Operation: tool.BrokerOperationGitRemote,
		WorkspaceRoot: fixture.workspace, WorkingDirectory: fixture.workspace, WorkspaceScopeDeclared: true,
		EnvironmentSnapshotID: fixture.environment.SnapshotID, GitRemote: projection,
	})
	if !errors.Is(err, tool.ErrExecutionBrokerDenied) || authorization.Action != tool.BrokerDeny {
		t.Fatalf("raw Shell acquired typed Git broker operation: auth=%+v err=%v", authorization, err)
	}
	projection.RemoteURL = "http://attacker.invalid/project.git"
	authorization, err = fixture.broker.Authorize(context.Background(), tool.ExecutionBrokerRequest{
		ProviderID: tool.ProviderID(providerID), Capability: tool.CapabilityGitRemoteRead, Operation: tool.BrokerOperationGitRemote,
		WorkspaceRoot: fixture.workspace, WorkingDirectory: fixture.workspace, WorkspaceScopeDeclared: true,
		EnvironmentSnapshotID: fixture.environment.SnapshotID, GitRemote: projection,
	})
	if !errors.Is(err, tool.ErrExecutionBrokerDenied) || authorization.Action != tool.BrokerDeny {
		t.Fatalf("unsafe URL bypassed broker-side typed validation: auth=%+v err=%v", authorization, err)
	}
}

func TestGitRemoteFiltersTaskControlledPATHAndPinsHTTPSHelper(t *testing.T) {
	helperScript := `mode=""
destination=""
for argument in "$@"; do
  if [ "$argument" = "clone" ]; then mode="$argument"; fi
  destination="$argument"
done
test "$mode" = "clone"
selected=$(command -v git-remote-https)
helper_output=$("$selected")
mkdir -p "$destination/.git"
printf 'ref: refs/heads/main\n' > "$destination/.git/HEAD"
printf '%s\n' "$selected" > "$destination/helper-path.txt"
printf '%s\n' "$helper_output" > "$destination/helper-output.txt"
printf '%s\n' "$PATH" > "$destination/path.txt"
printf '%s\n' "$GIT_EXEC_PATH" > "$destination/git-exec-path.txt"`
	fixture := newGitFixture(t, helperScript, nil, nil)
	workspaceBin := filepath.Join(fixture.workspace, "bin")
	if err := os.MkdirAll(workspaceBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceBin, "git-remote-https"), []byte("#!/bin/sh\nprintf 'malicious-helper\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.environment.PATH = append([]string{workspaceBin}, fixture.environment.PATH...)
	fixture.environment.SkillRoots = []tool.SkillRootObservation{{Source: "project", Path: filepath.Join(fixture.workspace, ".awaken", "skills")}}
	fixture.environment.TemporaryWorkRoot = filepath.Join(fixture.workspace, ".awaken", "tmp")
	fixture.environment.SnapshotID = "env_git_helper_filter"
	fixture.snapshot = tool.NewRunToolAvailabilitySnapshot(fixture.environment)
	outcome := executeFixture(t, fixture, input{Operation: operationClone, URL: "https://github.com/example/project.git", Destination: "project"}, nil)
	if outcome.Status != tool.OutcomeSucceeded {
		t.Fatalf("helper-path clone failed: %+v", outcome)
	}
	read := func(name string) string {
		value, err := os.ReadFile(filepath.Join(fixture.workspace, "project", name))
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(value))
	}
	if output := read("helper-output.txt"); output != "trusted-helper" {
		t.Fatalf("task-controlled Git helper executed: %q", output)
	}
	if selected := read("helper-path.txt"); strings.HasPrefix(selected, fixture.workspace) {
		t.Fatalf("task-controlled helper remained selectable: %q", selected)
	}
	if pathValue := read("path.txt"); strings.Contains(pathValue, workspaceBin) {
		t.Fatalf("task-controlled PATH entry reached Git child: %q", pathValue)
	}
	if execPath := read("git-exec-path.txt"); execPath != filepath.Dir(fixture.git) {
		t.Fatalf("GIT_EXEC_PATH was not pinned to trusted helper root: %q", execPath)
	}
}

func TestGitRemoteDarwinRejectsHashMatchedUserGitBeforeTransport(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin Developer-toolchain trust gate")
	}
	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	raw := `{"operation":"ls_remote","url":"https://github.com/example/project.git"}`
	outcome, err := (&Tool{adapter: fixture.adapter, resolveHost: publicResolver, lookupEnv: noProxyEnv}).ExecuteOutcome(fixture.context(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.TransportStatus != tool.TransportNotStarted || outcome.ErrorCode != "git_snapshot_invalid" {
		t.Fatalf("hash-matched user Git bypassed host toolchain trust: %+v", outcome)
	}
	fixture.adapter.mu.Lock()
	tiers := append([]string(nil), fixture.adapter.tiers...)
	fixture.adapter.mu.Unlock()
	if len(tiers) != 0 {
		t.Fatalf("untrusted Git reached sandbox/process setup: %v", tiers)
	}
}

func TestGitRemoteFailsClosedWithoutFullSandbox(t *testing.T) {
	fixture := newGitFixture(t, successfulGitScript, nil, nil)
	outcome := executeFixture(t, fixture, input{Operation: operationLSRemote, URL: "https://github.com/example/project.git"}, &degradedSandbox{})
	if outcome.Status != tool.OutcomeUnsupported || outcome.TransportStatus != tool.TransportNotStarted || outcome.ErrorCode != tool.ErrorCodeSupervisedExecutionRequired {
		t.Fatalf("no-full-sandbox platform did not fail closed: %+v", outcome)
	}
	entries, err := os.ReadDir(fixture.workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("degraded sandbox changed workspace: entries=%v err=%v", entries, err)
	}
}

func TestGitRemoteIsOrdinaryZeroApprovalButRetainsHardExecutorGates(t *testing.T) {
	decision := tool.Decide(context.Background(), (&Tool{}).Definition())
	if decision.Behavior != tool.PermissionAllow {
		t.Fatalf("typed Git ordinary operation unexpectedly requested generic approval: %+v", decision)
	}
}

// TestGitRemoteLiveHTTPSManifestSmoke is intentionally opt-in. It uses one
// public repository already pinned in the 20-package market manifest, but it
// never runs in the default offline suite. The caller must provide a new empty
// absolute /tmp workspace and may keep it as Desktop acceptance evidence.
func TestGitRemoteLiveHTTPSManifestSmoke(t *testing.T) {
	if os.Getenv("AWAKEN_GIT_REMOTE_LIVE") != "1" {
		t.Skip("set AWAKEN_GIT_REMOTE_LIVE=1 and AWAKEN_GIT_REMOTE_LIVE_WORKSPACE=/tmp/<empty-dir> to opt in")
	}
	workspace := strings.TrimSpace(os.Getenv("AWAKEN_GIT_REMOTE_LIVE_WORKSPACE"))
	cleanWorkspace := filepath.Clean(workspace)
	isTempChild := strings.HasPrefix(cleanWorkspace, filepath.Clean(os.TempDir())+string(filepath.Separator)) ||
		strings.HasPrefix(cleanWorkspace, "/tmp/") || strings.HasPrefix(cleanWorkspace, "/private/tmp/")
	if workspace == "" || !filepath.IsAbs(workspace) || !isTempChild {
		t.Fatal("AWAKEN_GIT_REMOTE_LIVE_WORKSPACE must be an absolute child of the system temp directory")
	}
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("live workspace must be empty: entries=%v err=%v", entries, err)
	}
	environment := tool.ProbeExecutionEnvironment(context.Background(), tool.ExecutionEnvironmentProbeOptions{
		WorkspaceRoot: workspace, WorkingDirectory: workspace, ProbeVersions: false,
	})
	gitObservation, ok := environment.Executable("git")
	if !ok || gitObservation.Fingerprint == "" {
		t.Fatal("live environment lacks fingerprinted git")
	}
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	permission := tool.NewTaskScopePermissionContext("git-live-smoke", workspace, []string{workspace}, []string{workspace}, nil, nil, nil, "")
	ctx := tool.ContextWithRunAvailability(context.Background(), snapshot, environment)
	ctx = tool.ContextWithPermissionContext(ctx, permission)
	ctx = tool.ContextWithExecutionBroker(ctx, tool.NewWorkspaceExecutionBrokerWithOptions(tool.WorkspaceExecutionBrokerOptions{StagingParent: filepath.Join(os.TempDir(), "awaken-git-live-staging")}))
	ctx = tool.ContextWithToolExecutionIdentity(ctx, tool.ToolExecutionIdentity{RunID: "run_git_live", ToolCallID: "call_git_live", ToolName: toolName})
	remote := "https://github.com/arvindrk/extract-design-system.git"
	expectedCommit := "1873741ba8dea755e35e6e15134f7918cd58e036"
	gitTool := &Tool{}
	lsRaw, _ := json.Marshal(input{Operation: operationLSRemote, URL: remote})
	lsOutcome, err := gitTool.ExecuteOutcome(ctx, string(lsRaw), nil)
	if err != nil || lsOutcome.Status != tool.OutcomeSucceeded || len(lsOutcome.Changes) != 0 {
		t.Fatalf("live ls-remote failed: outcome=%+v err=%v", lsOutcome, err)
	}
	refs, ok := lsOutcome.Details["refs"].([]remoteRef)
	if !ok {
		t.Fatalf("live ls-remote did not return typed refs: %#v", lsOutcome.Details["refs"])
	}
	foundPinnedCommit := false
	for _, ref := range refs {
		if ref.ObjectID == expectedCommit {
			foundPinnedCommit = true
			break
		}
	}
	if !foundPinnedCommit {
		t.Fatalf("market manifest pinned commit %s is no longer advertised by the remote refs", expectedCommit)
	}
	cloneRaw, _ := json.Marshal(input{Operation: operationClone, URL: remote, Destination: "extract-design-system"})
	cloneOutcome, err := gitTool.ExecuteOutcome(ctx, string(cloneRaw), nil)
	if err != nil || cloneOutcome.Status != tool.OutcomeSucceeded || cloneOutcome.Operation == nil || cloneOutcome.Operation.SideEffectState != tool.OutcomeSideEffectApplied {
		t.Fatalf("live shallow clone failed: outcome=%+v err=%v", cloneOutcome, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "extract-design-system", ".git", "HEAD")); err != nil {
		t.Fatalf("live clone was not published: %v", err)
	}
	branchCommit, err := os.ReadFile(filepath.Join(workspace, "extract-design-system", ".git", "refs", "heads", "main"))
	if err != nil || strings.TrimSpace(string(branchCommit)) != expectedCommit {
		t.Fatalf("live clone does not match market manifest commit: got=%q want=%q err=%v", strings.TrimSpace(string(branchCommit)), expectedCommit, err)
	}
	t.Logf("live typed Git evidence retained at %s; receipt=%+v", workspace, cloneOutcome.Details["broker_receipt"])
}
