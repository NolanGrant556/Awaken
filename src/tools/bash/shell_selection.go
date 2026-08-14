package bash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"awaken/src/tool"
)

const (
	shellHealthMarker          = "awaken-shell-ready"
	shellHealthTimeout         = 3 * time.Second
	maxShellHealthCacheEntries = 128
)

type shellCandidate struct {
	path string
	kind string
}

type shellHealthProbe func(ctx context.Context, shell string, kind string, environment tool.HostEnvironment) error

var shellHealthCache = struct {
	sync.Mutex
	results map[string]string
	order   []string
}{results: make(map[string]string), order: make([]string, 0, maxShellHealthCacheEntries)}

func selectWindowsShell(ctx context.Context, environment tool.HostEnvironment, command string, probe shellHealthProbe) (string, []string, string, error) {
	candidates := windowsShellCandidates(environment)
	failures := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if probe != nil {
			if err := probe(ctx, candidate.path, candidate.kind, environment); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", candidate.path, err))
				continue
			}
		}
		return candidate.path, invocationArgs(candidate.path, candidate.kind, command), candidate.kind, nil
	}
	if len(failures) > 0 {
		return "", nil, "", fmt.Errorf("no healthy supported shell found (Git Bash, pwsh, powershell, cmd); probes: %s", strings.Join(failures, "; "))
	}
	return "", nil, "", fmt.Errorf("no supported shell found (Git Bash, pwsh, powershell, cmd)")
}

func windowsShellCandidates(environment tool.HostEnvironment) []shellCandidate {
	result := make([]shellCandidate, 0, 6)
	seen := make(map[string]struct{})
	add := func(path, kind string) {
		path = strings.TrimSpace(path)
		if path == "" || !shellExecutableFile(path) {
			return
		}
		key := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, shellCandidate{path: filepath.Clean(path), kind: kind})
	}

	for _, path := range gitBashCandidates(environment) {
		add(path, "posix")
	}

	if shellBase(environment.Shell) == "pwsh" {
		add(environment.Shell, "powershell")
	}
	add(findExecutable(environment, "pwsh"), "powershell")
	if shellBase(environment.Shell) == "powershell" {
		add(environment.Shell, "powershell")
	}
	add(findExecutable(environment, "powershell"), "powershell")

	if shellBase(environment.Shell) == "cmd" {
		add(environment.Shell, "cmd")
	}
	add(environmentValue(environment.Environment, "COMSPEC"), "cmd")
	add(findExecutable(environment, "cmd"), "cmd")
	return result
}

func shellExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0)
}

func gitBashCandidates(environment tool.HostEnvironment) []string {
	result := make([]string, 0, 8)
	seen := make(map[string]struct{})
	add := func(path string, requireRecognizedPath bool) {
		path = strings.TrimSpace(path)
		if path == "" || isWindowsSystemBash(path) || (requireRecognizedPath && !looksLikeGitBash(path)) {
			return
		}
		key := strings.ToLower(filepath.Clean(path))
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, filepath.Clean(path))
	}

	add(environmentValue(environment.Environment, "AWAKEN_GIT_BASH_PATH"), false)
	if observed, ok := environment.Snapshot.Executable("git"); ok {
		add(deriveGitBashPath(observed.Path), false)
	}
	add(deriveGitBashPath(findExecutable(environment, "git")), false)

	for _, root := range []string{
		environmentValue(environment.Environment, "ProgramW6432"),
		environmentValue(environment.Environment, "ProgramFiles"),
		environmentValue(environment.Environment, "ProgramFiles(x86)"),
	} {
		if root != "" {
			add(filepath.Join(root, "Git", "bin", "bash.exe"), false)
		}
	}
	if localAppData := environmentValue(environment.Environment, "LOCALAPPDATA"); localAppData != "" {
		add(filepath.Join(localAppData, "Programs", "Git", "bin", "bash.exe"), false)
	}

	add(environment.Shell, true)
	if observed, ok := environment.Snapshot.Executable("bash"); ok {
		add(observed.Path, true)
	}
	add(findExecutable(environment, "bash"), true)
	return result
}

func deriveGitBashPath(gitPath string) string {
	gitPath = strings.TrimSpace(gitPath)
	if gitPath == "" {
		return ""
	}
	binDir := filepath.Dir(gitPath)
	binName := strings.ToLower(filepath.Base(binDir))
	if binName != "cmd" && binName != "bin" {
		return ""
	}
	root := filepath.Dir(binDir)
	if binName == "bin" && strings.EqualFold(filepath.Base(root), "mingw64") {
		return ""
	}
	return filepath.Join(root, "bin", "bash.exe")
}

func looksLikeGitBash(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(filepath.Clean(path), `\`, "/"))
	if isWindowsSystemBash(normalized) {
		return false
	}
	base := normalized[strings.LastIndex(normalized, "/")+1:]
	if base != "bash.exe" && base != "bash" {
		return false
	}
	return strings.Contains(normalized, "/git/") ||
		strings.Contains(normalized, "/portablegit/") ||
		strings.Contains(normalized, "/gitforwindows/") ||
		strings.Contains(normalized, "/scoop/apps/git/")
}

func isWindowsSystemBash(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), `\`, "/"))
	return strings.Contains(normalized, "/windows/system32/bash.exe") ||
		strings.Contains(normalized, "/windows/sysnative/bash.exe")
}

func shellBase(path string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	base := normalized[strings.LastIndex(normalized, "/")+1:]
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}

func environmentValue(environment []string, key string) string {
	for _, entry := range environment {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], key) {
			return parts[1]
		}
	}
	return ""
}

func cachedShellHealthProbe(ctx context.Context, shell string, kind string, environment tool.HostEnvironment) error {
	cacheKey := strings.TrimSpace(environment.Snapshot.SnapshotID)
	if cacheKey == "" {
		return runShellHealthProbe(ctx, shell, kind, environment)
	}
	cacheKey += "\x00" + strings.ToLower(filepath.Clean(shell)) + "\x00" + kind
	shellHealthCache.Lock()
	message, exists := shellHealthCache.results[cacheKey]
	shellHealthCache.Unlock()
	if exists {
		if message == "" {
			return nil
		}
		return errors.New(message)
	}
	err := runShellHealthProbe(ctx, shell, kind, environment)
	if ctx != nil && ctx.Err() != nil {
		return err
	}
	message = ""
	if err != nil {
		message = err.Error()
	}
	shellHealthCache.Lock()
	if _, exists := shellHealthCache.results[cacheKey]; !exists {
		if len(shellHealthCache.order) >= maxShellHealthCacheEntries {
			oldest := shellHealthCache.order[0]
			shellHealthCache.order = shellHealthCache.order[1:]
			delete(shellHealthCache.results, oldest)
		}
		shellHealthCache.order = append(shellHealthCache.order, cacheKey)
	}
	shellHealthCache.results[cacheKey] = message
	shellHealthCache.Unlock()
	return err
}

func runShellHealthProbe(ctx context.Context, shell string, kind string, environment tool.HostEnvironment) error {
	probeCommand := "printf '" + shellHealthMarker + "'"
	switch kind {
	case "powershell":
		probeCommand = "[Console]::Out.Write('" + shellHealthMarker + "')"
	case "cmd":
		probeCommand = "echo " + shellHealthMarker
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeContext, cancel := context.WithTimeout(ctx, shellHealthTimeout)
	defer cancel()
	command := exec.CommandContext(probeContext, shell, invocationArgs(shell, kind, probeCommand)...)
	if len(environment.Environment) > 0 {
		command.Env = append([]string(nil), environment.Environment...)
	}
	if directory := strings.TrimSpace(environment.Snapshot.WorkingDirectory); directory != "" {
		if info, err := os.Stat(directory); err == nil && info.IsDir() {
			command.Dir = directory
		}
	}
	stdout := newLimitedWriter(4096, nil, "stdout")
	stderr := newLimitedWriter(4096, nil, "stderr")
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if errors.Is(probeContext.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("health check timed out")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("health check cancelled: %w", err)
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 240 {
			detail = detail[:240] + "…"
		}
		if detail != "" {
			return fmt.Errorf("health check failed: %s", detail)
		}
		return fmt.Errorf("health check failed: %w", err)
	}
	if !strings.Contains(stdout.String(), shellHealthMarker) {
		return fmt.Errorf("health check marker missing")
	}
	return nil
}
