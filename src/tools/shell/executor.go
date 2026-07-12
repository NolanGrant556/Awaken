package shelltool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tool/sandbox"
)

// ErrSandboxUnavailable is retained for compatibility with older callers.
// New code reports tool.ErrExecutionSandboxRequired when a workspace broker
// cannot prove that its platform write boundary was actually applied.
var ErrSandboxUnavailable = errors.New(
	"open-world shell command requires a trusted sandbox that is not available on this platform",
)

// ShellExecutor wraps shell command execution with classification, local shell
// gates, environment policy, output limits, and timeout handling. The shell
// tool delegates to it so that all execution goes through a single controlled
// layer.
//
// Execution flow:
//  1. Classify the command for observation.
//  2. Authorize through a broker. The default broker executes directly as the
//     current login user; callers may explicitly inject a restricted/staged
//     broker when they need its narrower contract.
//  3. Apply a platform sandbox only when that broker explicitly requests it.
//  4. Execute via the user's shell and return the real process facts.
type ShellExecutor struct {
	// adapter is the sandbox adapter to use. When nil, the executor
	// resolves the platform default at execution time via
	// sandbox.DefaultAdapter(). This lets tests inject a mock with
	// sandbox.SetAdapter without having to construct a new executor.
	adapter sandbox.SandboxAdapter
}

// NewShellExecutor returns a ShellExecutor that resolves the sandbox
// adapter at execution time via sandbox.DefaultAdapter(). Tests that want
// to inject a specific adapter should call sandbox.SetAdapter before
// invoking Execute, or use NewShellExecutorWithAdapter.
func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{}
}

// NewShellExecutorWithAdapter returns a ShellExecutor that always uses
// the given sandbox adapter. This is primarily for tests that want to
// bypass the global override.
func NewShellExecutorWithAdapter(adapter sandbox.SandboxAdapter) *ShellExecutor {
	return &ShellExecutor{adapter: adapter}
}

// ShellExecResult carries the outcome of a shell execution, including the
// classified tier and the sandbox support level that was in effect. The
// shell tool marshals a subset of this (the shellResult struct) to JSON
// for tool callers; the tier and sandbox fields are kept here for
// observability and future audit hooks.
type ShellExecResult struct {
	Stdout              string
	Stderr              string
	ExitCode            int
	TimedOut            bool
	Cancelled           bool
	DurationMS          int64
	Truncated           bool
	Tier                ShellTier
	SandboxSupport      sandbox.SandboxSupport
	EnvironmentSnapshot tool.ExecutionEnvironmentSnapshot
	BrokerAuthorization tool.BrokerAuthorization
	BrokerReceipt       tool.BrokerExecutionReceipt
}

// Execute runs the shell command through the executor.
//
// params is expected to be already parsed from the tool input. The
// executor trims and validates the command field. permissionContext is
// used to resolve the working directory (workdir or workspace root).
// observer, if non-nil, receives streamed stdout/stderr deltas.
func (e *ShellExecutor) Execute(
	ctx context.Context,
	params shellParams,
	permissionContext tool.PermissionContext,
	observer tool.ToolObserver,
) (ShellExecResult, error) {
	permissionContext = permissionContext.Normalize()
	command := strings.TrimSpace(params.Command)
	if command == "" {
		return ShellExecResult{}, fmt.Errorf("缺少必需参数: command")
	}

	cwd := resolveExecutorCwd(params.Workdir, permissionContext)
	tier := ClassifyShellCommand(command, cwd)
	policy := EvaluateShellExecutionPolicy(command, cwd, permissionContext)
	if strings.TrimSpace(policy.DenyReason) != "" {
		return ShellExecResult{
			Tier: tier,
		}, fmt.Errorf("shell policy denied command: %s", policy.DenyReason)
	}
	activeSandbox := e.adapter
	if activeSandbox == nil {
		activeSandbox = sandbox.DefaultAdapter()
	}

	environmentSnapshot, hasSnapshot := tool.ExecutionEnvironmentSnapshotFromContext(ctx)
	var hostEnvironment HostShellEnvironment
	if hasSnapshot {
		if environmentSnapshot.PATHTruncated && strings.TrimSpace(environmentSnapshot.PinnedTransportPATH()) == "" {
			return ShellExecResult{Tier: tier, EnvironmentSnapshot: environmentSnapshot}, fmt.Errorf("pinned environment full PATH is unavailable for process transport")
		}
		hostEnvironment = hostShellEnvironmentFromSnapshot(environmentSnapshot)
	} else {
		executableNames := shellExecutableNames(command)
		hostEnvironment = ResolveHostShellEnvironment(ctx, tool.ExecutionEnvironmentProbeOptions{
			WorkspaceRoot: permissionContext.WorkspaceRoot, WorkingDirectory: cwd,
			Executables: executableNames, ProbeVersions: false,
		})
		environmentSnapshot = hostEnvironment.Snapshot
	}

	// EnvPolicy enforcement: when the execution profile's EnvPolicy is
	// EnvMinimal, constrain cmd.Env to a minimal allowlist of environment
	// variables (PATH, HOME, USER, SHELL, TMPDIR, LANG, LC_ALL) so secrets
	// and project-local tokens are not leaked to the child process. Filtering
	// happens after host resolution so even GUI-launched children retain the
	// exact pinned PATH/SHELL and repaired HOME.
	executionEnv := append([]string(nil), hostEnvironment.Environment...)
	if profile.EnvPolicyFromContext(ctx) == profile.EnvMinimal {
		executionEnv = minimalEnvAllowlistFrom(executionEnv)
	}

	operation, referencedPaths := ClassifyShellBrokerOperation(command, cwd, permissionContext)
	broker, explicitlyConfigured := tool.ExecutionBrokerFromContext(ctx)
	if !explicitlyConfigured {
		broker = tool.NewLocalUserExecutionBroker()
	}
	if broker == nil {
		return ShellExecResult{Tier: tier, EnvironmentSnapshot: environmentSnapshot}, tool.ErrExecutionBrokerUnavailable
	}
	brokerRequest := tool.ExecutionBrokerRequest{
		ProviderID: tool.ProviderID("builtin:shell"), Capability: tool.CapabilityShellExecute,
		Operation: operation, Command: command, WorkingDirectory: cwd,
		WorkspaceRoot:         permissionContext.WorkspaceRoot,
		ReferencedPaths:       referencedPaths,
		EnvironmentSnapshotID: environmentSnapshot.SnapshotID,
	}
	if scopedBroker, ok := broker.(tool.WorkspaceScopeExecutionBroker); ok && scopedBroker.EnforcesWorkspaceScope() {
		brokerScope, scopeErr := tool.CanonicalBrokerWorkspaceScope(permissionContext)
		if scopeErr != nil {
			return ShellExecResult{Tier: tier, EnvironmentSnapshot: environmentSnapshot}, fmt.Errorf("%w: canonicalize broker workspace scope: %v", tool.ErrExecutionBrokerDenied, scopeErr)
		}
		brokerRequest.WorkspaceScopeDeclared = true
		brokerRequest.WritableRoots = brokerScope.WritableRoots
		brokerRequest.ProtectedRoots = brokerScope.ProtectedRoots
	}
	authorization, brokerErr := broker.Authorize(ctx, brokerRequest)
	if brokerErr != nil || authorization.Action != tool.BrokerAllow {
		if brokerErr == nil {
			brokerErr = tool.ErrExecutionBrokerDenied
		}
		return ShellExecResult{
			Tier: tier, EnvironmentSnapshot: environmentSnapshot, BrokerAuthorization: authorization,
		}, fmt.Errorf("execution broker %s rejected command: %w", broker.ID(), brokerErr)
	}
	executionCwd := cwd
	if strings.TrimSpace(authorization.ExecutionWorkingDirectory) != "" {
		executionCwd = authorization.ExecutionWorkingDirectory
	}
	executionEnv = applyShellEnvironmentOverrides(executionEnv, authorization.EnvironmentOverrides)
	executionEnv = stripImplicitShellStartupEnvironment(executionEnv)

	timeout := normalizeTimeout(params.TimeoutMS)
	maxOutputChars := normalizeMaxOutputChars(params.MaxOutputChars)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := shellCommand(runCtx, command, hostEnvironment.Shell, executionEnv)
	if err != nil {
		receipt, _ := completeShellBroker(broker, authorization, tool.BrokerExecutionResult{
			TransportStatus: tool.TransportNotStarted, ProcessStatus: tool.ProcessNotApplicable,
			SideEffectState: tool.OutcomeSideEffectNone,
		})
		return ShellExecResult{
			Tier: tier, EnvironmentSnapshot: environmentSnapshot, BrokerAuthorization: authorization, BrokerReceipt: receipt,
		}, fmt.Errorf("resolve shell: %w", err)
	}
	cmd.Dir = executionCwd
	cmd.Env = executionEnv

	effectiveSupport := sandbox.SandboxNone
	sandboxRequested := authorization.RequiresAppliedSandbox ||
		strings.TrimSpace(authorization.SandboxTier) != "" ||
		strings.TrimSpace(authorization.SandboxRoot) != ""
	if sandboxRequested && activeSandbox != nil {
		sandboxTier := string(tier)
		if strings.TrimSpace(authorization.SandboxTier) != "" {
			sandboxTier = authorization.SandboxTier
		}
		sandboxRoot := permissionContext.WorkspaceRoot
		if strings.TrimSpace(authorization.SandboxRoot) != "" {
			sandboxRoot = authorization.SandboxRoot
		}
		sandboxResult, sandboxErr := activeSandbox.ApplySandbox(cmd, sandboxTier, sandboxRoot)
		if sandboxErr != nil {
			receipt, _ := completeShellBroker(broker, authorization, tool.BrokerExecutionResult{
				TransportStatus: tool.TransportNotStarted, ProcessStatus: tool.ProcessNotApplicable,
				SideEffectState: tool.OutcomeSideEffectNone,
			})
			return ShellExecResult{
				Tier: tier, SandboxSupport: sandboxResult.Support,
				EnvironmentSnapshot: environmentSnapshot, BrokerAuthorization: authorization, BrokerReceipt: receipt,
			}, fmt.Errorf("apply shell sandbox: %w", sandboxErr)
		}
		effectiveSupport = sandboxResult.Support
		if authorization.RequiresAppliedSandbox && !sandboxResult.Applied {
			receipt, _ := completeShellBroker(broker, authorization, tool.BrokerExecutionResult{
				TransportStatus: tool.TransportNotStarted, ProcessStatus: tool.ProcessNotApplicable,
				SideEffectState: tool.OutcomeSideEffectNone,
			})
			return ShellExecResult{
				Tier: tier, SandboxSupport: sandboxResult.Support,
				EnvironmentSnapshot: environmentSnapshot, BrokerAuthorization: authorization, BrokerReceipt: receipt,
			}, fmt.Errorf("%w: broker %s requires a real sandbox boundary", tool.ErrExecutionSandboxRequired, broker.ID())
		}
	} else if authorization.RequiresAppliedSandbox {
		receipt, _ := completeShellBroker(broker, authorization, tool.BrokerExecutionResult{
			TransportStatus: tool.TransportNotStarted, ProcessStatus: tool.ProcessNotApplicable,
			SideEffectState: tool.OutcomeSideEffectNone,
		})
		return ShellExecResult{
			Tier: tier, EnvironmentSnapshot: environmentSnapshot,
			BrokerAuthorization: authorization, BrokerReceipt: receipt,
		}, fmt.Errorf("%w: no sandbox adapter", tool.ErrExecutionSandboxRequired)
	}
	configureShellProcessTree(cmd)

	stdout := newShellOutputWriter("stdout", maxOutputChars, observer)
	stderr := newShellOutputWriter("stderr", maxOutputChars, observer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	runErr := cmd.Run()
	terminateShellProcessTree(cmd)
	duration := time.Since(started)
	timedOut := runCtx.Err() == context.DeadlineExceeded
	cancelled := runCtx.Err() == context.Canceled
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if timedOut {
			exitCode = -1
		} else {
			partial := ShellExecResult{
				Stdout:              rewriteBrokerExecutionPath(stdout.Result().Content, authorization, permissionContext.WorkspaceRoot),
				Stderr:              rewriteBrokerExecutionPath(stderr.Result().Content, authorization, permissionContext.WorkspaceRoot),
				DurationMS:          duration.Milliseconds(),
				Cancelled:           cancelled,
				Tier:                tier,
				SandboxSupport:      effectiveSupport,
				EnvironmentSnapshot: environmentSnapshot,
				BrokerAuthorization: authorization,
			}
			partial.BrokerReceipt, _ = completeShellBroker(broker, authorization, tool.BrokerExecutionResult{
				TransportStatus: tool.TransportFailed, ProcessStatus: tool.ProcessSpawnFailed,
				SideEffectState: tool.OutcomeSideEffectUnknown,
			})
			return partial, fmt.Errorf("执行命令失败: %w", runErr)
		}
	}

	stdoutResult := stdout.Result()
	stderrResult := stderr.Result()
	result := ShellExecResult{
		Stdout:              rewriteBrokerExecutionPath(stdoutResult.Content, authorization, permissionContext.WorkspaceRoot),
		Stderr:              rewriteBrokerExecutionPath(stderrResult.Content, authorization, permissionContext.WorkspaceRoot),
		ExitCode:            exitCode,
		TimedOut:            timedOut,
		Cancelled:           cancelled,
		DurationMS:          duration.Milliseconds(),
		Truncated:           stdoutResult.Truncated || stderrResult.Truncated,
		Tier:                tier,
		SandboxSupport:      effectiveSupport,
		EnvironmentSnapshot: environmentSnapshot,
		BrokerAuthorization: authorization,
	}
	processStatus := tool.ProcessExited
	transportStatus := tool.TransportCompleted
	if timedOut {
		processStatus = tool.ProcessTimedOut
		transportStatus = tool.TransportFailed
	} else if cancelled {
		processStatus = tool.ProcessSignaled
		transportStatus = tool.TransportCancelled
	}
	sideEffectState := tool.OutcomeSideEffectNone
	if operation != tool.BrokerOperationInspect {
		sideEffectState = tool.OutcomeSideEffectUnknown
	}
	result.BrokerReceipt, err = completeShellBroker(broker, authorization, tool.BrokerExecutionResult{
		TransportStatus: transportStatus, ProcessStatus: processStatus, ExitCode: &exitCode,
		SideEffectState: sideEffectState,
	})
	if err != nil {
		return result, fmt.Errorf("execution broker completion failed: %w", err)
	}
	return result, nil
}

func shellExecutableNames(command string) []string {
	tokens := shellTokens(command)
	if len(tokens) == 0 {
		return []string{}
	}
	name := filepath.Base(strings.ReplaceAll(tokens[0], `\`, "/"))
	if name == "" {
		return []string{}
	}
	return []string{name}
}

func completeShellBroker(broker tool.ExecutionBroker, authorization tool.BrokerAuthorization, result tool.BrokerExecutionResult) (tool.BrokerExecutionReceipt, error) {
	completeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return broker.Complete(completeCtx, authorization, result)
}

func applyShellEnvironmentOverrides(environment []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return environment
	}
	result := append([]string(nil), environment...)
	for key, value := range overrides {
		replaced := false
		for index, entry := range result {
			existingKey, _, ok := splitEnvEntry(entry)
			if ok && envKeyEqual(runtime.GOOS, existingKey, key) {
				result[index] = existingKey + "=" + value
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, key+"="+value)
		}
	}
	return result
}

func rewriteBrokerExecutionPath(value string, authorization tool.BrokerAuthorization, workspaceRoot string) string {
	stagedRoot := strings.TrimSpace(authorization.ExecutionWorkspaceRoot)
	if stagedRoot == "" || strings.TrimSpace(workspaceRoot) == "" {
		return value
	}
	return strings.ReplaceAll(value, stagedRoot, workspaceRoot)
}

// resolveExecutorCwd resolves the working directory the command should
// run in. When workdir is empty, the workspace root (or CWD fallback) is
// used; otherwise workdir is resolved against the permission context so
// workspace-relative paths land inside the workspace.
func resolveExecutorCwd(workdir string, permissionContext tool.PermissionContext) string {
	if strings.TrimSpace(workdir) != "" {
		return permissionContext.ResolvePath(workdir)
	}
	return permissionContext.WorkspaceRootOrCWD()
}

// minimalEnvAllowlist returns the allowlist of environment variables a
// shell command may observe when the execution profile's EnvPolicy is
// EnvMinimal. Only variables needed for basic command execution are
// included; secrets and project-local tokens are stripped. Variables whose
// value is empty are omitted so the child does not see "VAR=" entries.
func minimalEnvAllowlist() []string {
	return minimalEnvAllowlistFrom(os.Environ())
}

func minimalEnvAllowlistFrom(source []string) []string {
	keys := []string{"PATH", "HOME", "USER", "SHELL", "TMPDIR", "LANG", "LC_ALL"}
	if runtime.GOOS == "windows" {
		keys = append(keys, "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "PATHEXT", "SYSTEMROOT")
	}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		value := envValue(source, key)
		if value == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

// stripImplicitShellStartupEnvironment prevents non-interactive shells from
// executing user-selected startup hook files such as BASH_ENV or ENV. The
// command itself can still source a file explicitly when that is part of the
// requested workflow.
func stripImplicitShellStartupEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := splitEnvEntry(entry)
		if ok && (envKeyEqual(runtime.GOOS, key, "BASH_ENV") || envKeyEqual(runtime.GOOS, key, "ENV")) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// inheritedShellEnvironment preserves the complete awaken-core process
// environment (including proxy, certificate, registry, cache, and tool-specific
// variables) while repairing the essential values GUI-launched processes may
// omit. It does not source shell rc files or replace an existing user value.
func inheritedShellEnvironment() []string {
	home := currentUserHomeDirectory()
	environment := ensureEssentialShellEnvironmentForOS(os.Environ(), runtime.GOOS, home, os.TempDir())
	return augmentShellPathForOS(environment, runtime.GOOS, string(os.PathListSeparator))
}

func currentUserHomeDirectory() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return home
	}
	if current, err := user.Current(); err == nil && current != nil {
		return strings.TrimSpace(current.HomeDir)
	}
	return ""
}

func ensureEssentialShellEnvironmentForOS(environment []string, goos string, fallbackHome string, fallbackTemp string) []string {
	result := append([]string(nil), environment...)
	setDefault := func(key string, value string) {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(envValueForOS(result, key, goos)) != "" {
			return
		}
		replaced := false
		for index, entry := range result {
			existingKey, _, ok := splitEnvEntry(entry)
			if ok && envKeyEqual(goos, existingKey, key) {
				result[index] = existingKey + "=" + value
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, key+"="+value)
		}
	}

	setDefault("HOME", fallbackHome)
	setDefault("TMPDIR", fallbackTemp)
	if goos == "windows" {
		setDefault("USERPROFILE", fallbackHome)
		systemRoot := envValueForOS(result, "SYSTEMROOT", goos)
		if systemRoot != "" {
			setDefault("PATH", joinPathForOS(goos, systemRoot, "System32"))
		}
	} else {
		setDefault("SHELL", "/bin/sh")
		setDefault("PATH", "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	}
	return result
}

func augmentShellPath(env []string) []string {
	return augmentShellPathForOS(env, runtime.GOOS, string(os.PathListSeparator))
}

func augmentShellPathForOS(env []string, goos string, pathListSeparator string) []string {
	if pathListSeparator == "" {
		pathListSeparator = string(os.PathListSeparator)
	}
	pathIndex := -1
	pathKey := "PATH"
	currentPath := ""
	for index, entry := range env {
		key, value, ok := splitEnvEntry(entry)
		if !ok {
			continue
		}
		if envKeyEqual(goos, key, "PATH") {
			pathIndex = index
			pathKey = key
			currentPath = value
			break
		}
	}

	paths := splitPathListForSeparator(currentPath, pathListSeparator)
	seen := make(map[string]bool, len(paths)+4)
	for _, path := range paths {
		if path != "" {
			seen[pathSeenKey(goos, path)] = true
		}
	}
	for _, path := range shellPathAdditionsForOS(goos, env) {
		seenKey := pathSeenKey(goos, path)
		if path == "" || seen[seenKey] {
			continue
		}
		paths = append(paths, path)
		seen[seenKey] = true
	}
	nextPath := strings.Join(paths, pathListSeparator)
	if nextPath == "" {
		return env
	}
	if pathIndex >= 0 {
		out := append([]string(nil), env...)
		out[pathIndex] = pathKey + "=" + nextPath
		return out
	}
	out := append([]string(nil), env...)
	out = append(out, "PATH="+nextPath)
	return out
}

func shellPathAdditions(env []string) []string {
	return shellPathAdditionsForOS(runtime.GOOS, env)
}

func shellPathAdditionsForOS(goos string, env []string) []string {
	if goos == "windows" {
		return windowsShellPathAdditions(env)
	}
	home := envValue(env, "HOME")
	if home == "" {
		home = os.Getenv("HOME")
	}
	var additions []string
	if home != "" {
		additions = append(additions, filepath.Join(home, ".local", "bin"))
	}
	additions = append(additions, "/opt/homebrew/bin", "/usr/local/bin")
	return additions
}

func windowsShellPathAdditions(env []string) []string {
	userProfile := envValueForOS(env, "USERPROFILE", "windows")
	appData := envValueForOS(env, "APPDATA", "windows")
	localAppData := envValueForOS(env, "LOCALAPPDATA", "windows")
	if appData == "" && userProfile != "" {
		appData = joinPathForOS("windows", userProfile, "AppData", "Roaming")
	}
	if localAppData == "" && userProfile != "" {
		localAppData = joinPathForOS("windows", userProfile, "AppData", "Local")
	}

	var additions []string
	if appData != "" {
		// npm global binaries on Windows, including lark-cli.cmd when installed globally.
		additions = append(additions, joinPathForOS("windows", appData, "npm"))
	}
	if localAppData != "" {
		additions = append(additions,
			joinPathForOS("windows", localAppData, "pnpm"),
			joinPathForOS("windows", localAppData, "Microsoft", "WindowsApps"),
		)
	}
	if userProfile != "" {
		additions = append(additions,
			joinPathForOS("windows", userProfile, ".local", "bin"),
			joinPathForOS("windows", userProfile, "scoop", "shims"),
		)
	}
	return additions
}

func splitPathList(pathValue string) []string {
	return splitPathListForSeparator(pathValue, string(os.PathListSeparator))
}

func splitPathListForSeparator(pathValue string, separator string) []string {
	if pathValue == "" {
		return nil
	}
	if separator == "" {
		separator = string(os.PathListSeparator)
	}
	return strings.Split(pathValue, separator)
}

func envValue(env []string, key string) string {
	return envValueForOS(env, key, runtime.GOOS)
}

func envValueForOS(env []string, key string, goos string) string {
	for _, entry := range env {
		entryKey, value, ok := splitEnvEntry(entry)
		if !ok {
			continue
		}
		if envKeyEqual(goos, entryKey, key) {
			return value
		}
	}
	return ""
}

func splitEnvEntry(entry string) (string, string, bool) {
	key, value, ok := strings.Cut(entry, "=")
	if !ok || key == "" {
		return "", "", false
	}
	return key, value, true
}

func envKeyEqual(goos string, a string, b string) bool {
	if goos == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathSeenKey(goos string, path string) string {
	if goos == "windows" {
		return strings.ToLower(strings.TrimRight(path, `\/`))
	}
	return path
}

func joinPathForOS(goos string, elems ...string) string {
	if goos != "windows" {
		return filepath.Join(elems...)
	}
	result := ""
	for _, elem := range elems {
		if elem == "" {
			continue
		}
		if result == "" {
			result = strings.TrimRight(elem, `\/`)
			continue
		}
		result = strings.TrimRight(result, `\/`) + `\` + strings.Trim(elem, `\/`)
	}
	return result
}

// shellCommand builds an *exec.Cmd from the exact shell recorded by the host
// environment snapshot. Known shells receive their no-rc/no-profile flags so
// transport never executes arbitrary user startup files merely to run a tool.
func shellCommand(ctx context.Context, script string, shell string, environment []string) (*exec.Cmd, error) {
	shell = resolveShellCandidate(shell, environment)
	if shell == "" {
		return nil, fmt.Errorf("no executable shell found in pinned environment")
	}
	base := strings.ToLower(filepath.Base(shell))
	switch base {
	case "bash", "bash.exe":
		return exec.CommandContext(ctx, shell, "--noprofile", "--norc", "-c", script), nil
	case "zsh", "zsh.exe":
		return exec.CommandContext(ctx, shell, "-f", "-c", script), nil
	case "fish", "fish.exe":
		return exec.CommandContext(ctx, shell, "--no-config", "-c", script), nil
	case "nu", "nu.exe":
		return exec.CommandContext(ctx, shell, "--no-config-file", "-c", script), nil
	case "xonsh", "xonsh.exe":
		return exec.CommandContext(ctx, shell, "--no-rc", "-c", script), nil
	case "csh", "csh.exe", "tcsh", "tcsh.exe":
		return exec.CommandContext(ctx, shell, "-f", "-c", script), nil
	case "pwsh", "pwsh.exe", "powershell", "powershell.exe":
		return exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script), nil
	case "cmd", "cmd.exe":
		return exec.CommandContext(ctx, shell, "/d", "/s", "/c", script), nil
	case "sh", "sh.exe", "dash", "dash.exe", "ksh", "ksh.exe", "mksh", "mksh.exe":
		return exec.CommandContext(ctx, shell, "-c", script), nil
	default:
		return nil, fmt.Errorf("shell %q has no supported no-rc transport mode", shell)
	}
}
