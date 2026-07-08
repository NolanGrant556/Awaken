package shelltool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tool/sandbox"
)

// ErrSandboxUnavailable is returned when an open-world shell command is
// refused because the current platform does not provide a trusted sandbox.
// This is a defense-in-depth check: even if the policy layer allowed the
// call, the executor refuses to run it without isolation.
//
// The permission layer (CheckPermission + AutoToolPolicy) runs BEFORE the
// executor and may already deny open-world commands. This error is the
// last line of defense for direct tool execution paths that bypass the
// policy, and for platforms where the adapter has not yet been upgraded
// from SandboxDegraded/SandboxNone to SandboxFull.
var ErrSandboxUnavailable = errors.New(
	"open-world shell command requires a trusted sandbox that is not available on this platform",
)

// ShellExecutor wraps shell command execution with classification and
// sandbox-aware fail-closed behavior. The shell tool delegates to it so
// that all execution goes through a single controlled layer.
//
// Execution flow:
//  1. Classify the command via ClassifyShellCommand (Task 5).
//  2. Look up the platform sandbox adapter and its support level.
//  3. If the command is open_world_system and sandbox support is anything
//     less than SandboxFull, fail-closed with ErrSandboxUnavailable.
//  4. Apply sandbox constraints to the exec.Cmd (no-op for degraded/none).
//  5. Execute via the platform shell (/bin/sh on Unix, sh/cmd.exe on Windows),
//     capturing stdout/stderr with optional observer streaming.
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
	Stdout         string
	Stderr         string
	ExitCode       int
	TimedOut       bool
	DurationMS     int64
	Truncated      bool
	Tier           ShellTier
	SandboxSupport sandbox.SandboxSupport
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
	command := strings.TrimSpace(params.Command)
	if command == "" {
		return ShellExecResult{}, fmt.Errorf("缺少必需参数: command")
	}

	cwd := resolveExecutorCwd(params.Workdir, permissionContext)
	tier := ClassifyShellCommand(command, cwd)
	workspaceRoot := permissionContext.WorkspaceRootOrCWD()
	policy := EvaluateShellExecutionPolicy(command, cwd, permissionContext)
	if strings.TrimSpace(policy.DenyReason) != "" {
		return ShellExecResult{
			Tier: tier,
		}, fmt.Errorf("shell policy denied command: %s", policy.DenyReason)
	}

	adapter := e.adapter
	if adapter == nil {
		adapter = sandbox.DefaultAdapter()
	}
	support := adapter.SupportLevel()

	// Defense in depth: open_world_system commands require a real
	// sandbox. If the platform only offers degraded or no isolation,
	// refuse to execute rather than letting the command run unsandboxed.
	// The policy layer (AutoToolPolicy.decideShellTransform) already
	// denies these, but this guard catches direct execution paths and
	// any future platform where the adapter has not landed.
	if tier == ShellTierOpenWorldSystem && support != sandbox.SandboxFull {
		return ShellExecResult{
			Tier:           tier,
			SandboxSupport: support,
		}, ErrSandboxUnavailable
	}

	timeout := normalizeTimeout(params.TimeoutMS)
	maxOutputChars := normalizeMaxOutputChars(params.MaxOutputChars)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := shellCommand(runCtx, command)
	if err != nil {
		return ShellExecResult{}, fmt.Errorf("resolve shell: %w", err)
	}
	cmd.Dir = cwd

	// EnvPolicy enforcement: when the execution profile's EnvPolicy is
	// EnvMinimal, constrain cmd.Env to a minimal allowlist of environment
	// variables (PATH, HOME, USER, SHELL, TMPDIR, LANG, LC_ALL) so secrets
	// and project-local tokens are not leaked to the child process. Empty
	// values are filtered out. When EnvInherit (the default), cmd.Env is
	// left nil so the child inherits the runtime's full environment.
	envPolicy := profile.EnvPolicyFromContext(ctx)
	if envPolicy == profile.EnvMinimal {
		cmd.Env = minimalEnvAllowlist()
	}

	sandboxResult, err := adapter.ApplySandbox(cmd, string(tier), workspaceRoot)
	if err != nil {
		return ShellExecResult{
			Tier:           tier,
			SandboxSupport: support,
		}, fmt.Errorf("apply sandbox: %w", err)
	}
	_ = sandboxResult // observability hook; not yet surfaced to callers

	stdout := newShellOutputWriter("stdout", maxOutputChars, observer)
	stderr := newShellOutputWriter("stderr", maxOutputChars, observer)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	runErr := cmd.Run()
	duration := time.Since(started)
	timedOut := runCtx.Err() == context.DeadlineExceeded
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if timedOut {
			exitCode = -1
		} else {
			return ShellExecResult{
				Stdout:         stdout.Result().Content,
				Stderr:         stderr.Result().Content,
				DurationMS:     duration.Milliseconds(),
				Tier:           tier,
				SandboxSupport: support,
			}, fmt.Errorf("执行命令失败: %w", runErr)
		}
	}

	stdoutResult := stdout.Result()
	stderrResult := stderr.Result()
	return ShellExecResult{
		Stdout:         stdoutResult.Content,
		Stderr:         stderrResult.Content,
		ExitCode:       exitCode,
		TimedOut:       timedOut,
		DurationMS:     duration.Milliseconds(),
		Truncated:      stdoutResult.Truncated || stderrResult.Truncated,
		Tier:           tier,
		SandboxSupport: support,
	}, nil
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
	keys := []string{"PATH", "HOME", "USER", "SHELL", "TMPDIR", "LANG", "LC_ALL"}
	if runtime.GOOS == "windows" {
		keys = append(keys, "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "PATHEXT", "SYSTEMROOT")
	}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

// shellCommand builds an *exec.Cmd that runs the given shell script on the
// current platform. On Unix it uses /bin/sh. On Windows it first tries to
// find a POSIX sh in PATH (e.g., Git Bash); if none is available it falls
// back to cmd.exe /c so common commands like "npx ..." still work.
func shellCommand(ctx context.Context, script string) (*exec.Cmd, error) {
	if runtime.GOOS != "windows" {
		return exec.CommandContext(ctx, "/bin/sh", "-c", script), nil
	}
	if shPath, err := exec.LookPath("sh"); err == nil {
		return exec.CommandContext(ctx, shPath, "-c", script), nil
	}
	cmdPath, err := exec.LookPath("cmd.exe")
	if err != nil {
		return nil, fmt.Errorf("no shell found in PATH")
	}
	return exec.CommandContext(ctx, cmdPath, "/c", script), nil
}
