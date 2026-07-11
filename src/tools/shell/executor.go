package shelltool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/profile"
	"awaken/src/tool/sandbox"
)

// ErrSandboxUnavailable is retained for compatibility with older tests and
// callers. Local-computer shell no longer fails open-world commands because a
// platform sandbox is unavailable; the active hard gates are implemented by
// EvaluateShellExecutionPolicy.
var ErrSandboxUnavailable = errors.New(
	"open-world shell command requires a trusted sandbox that is not available on this platform",
)

// ShellExecutor wraps shell command execution with classification, local shell
// gates, environment policy, output limits, and timeout handling. The shell
// tool delegates to it so that all execution goes through a single controlled
// layer.
//
// Execution flow:
//  1. Classify the command via ClassifyShellCommand (Task 5).
//  2. Evaluate the local-computer shell gates (Trash-only deletes,
//     mass-message review, and worker-owned typed-runtime executables).
//  3. Execute via the platform shell (/bin/sh on Unix, sh/cmd.exe on Windows),
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
	policy := EvaluateShellExecutionPolicy(command, cwd, permissionContext)
	if strings.TrimSpace(policy.DenyReason) != "" {
		return ShellExecResult{
			Tier: tier,
		}, fmt.Errorf("shell policy denied command: %s", policy.DenyReason)
	}

	support := sandbox.SandboxNone
	if adapter := e.adapter; adapter != nil {
		support = adapter.SupportLevel()
	} else if defaultAdapter := sandbox.DefaultAdapter(); defaultAdapter != nil {
		support = defaultAdapter.SupportLevel()
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
	// values are filtered out. When EnvInherit (the default), the child gets
	// the runtime's full environment with PATH augmented for common user CLI
	// install locations such as ~/.local/bin and Homebrew.
	envPolicy := profile.EnvPolicyFromContext(ctx)
	if envPolicy == profile.EnvMinimal {
		cmd.Env = augmentShellPath(minimalEnvAllowlist())
	} else {
		cmd.Env = augmentShellPath(os.Environ())
	}

	effectiveSupport := support

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
				SandboxSupport: effectiveSupport,
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
		SandboxSupport: effectiveSupport,
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
