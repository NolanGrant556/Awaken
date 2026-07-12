package shelltool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"awaken/src/tool"
)

// HostShellEnvironment is the single host-owned environment contract shared
// by run availability probing and Shell process transport. Environment is a
// repaired copy of the awaken-core process environment; PATH receives only
// deterministic CLI additions here. The Desktop supervisor owns any login
// shell environment import before awaken-core starts.
type HostShellEnvironment struct {
	Environment []string
	Shell       string
	PATH        string
	Snapshot    tool.ExecutionEnvironmentSnapshot
}

// ResolveHostShellEnvironment resolves one process environment, shell
// executable, and PATH, then probes the snapshot from those exact values. It
// never starts a shell and never sources or executes shell configuration.
func ResolveHostShellEnvironment(ctx context.Context, options tool.ExecutionEnvironmentProbeOptions) HostShellEnvironment {
	if ctx == nil {
		ctx = context.Background()
	}
	environment := inheritedShellEnvironment()
	shell := resolveHostShellExecutable(options.Shell, environment)
	pathOptions := options
	// awaken-core receives the login-user environment from its Desktop parent.
	// Do not reinterpret that already-resolved PATH by parsing rc files again.
	pathOptions.EnableSafeLoginShellPATHProbe = false
	pathValue, pathSource, pathWarnings := tool.ResolveExecutionEnvironmentPATH(
		ctx,
		pathOptions,
		shell,
		envValue(environment, "PATH"),
	)
	environment = applyShellEnvironmentOverrides(environment, map[string]string{
		"PATH":  pathValue,
		"SHELL": shell,
	})

	probeOptions := options
	probeOptions.PATH = pathValue
	probeOptions.PATHSource = pathSource
	probeOptions.PATHWarnings = append(append([]string(nil), options.PATHWarnings...), pathWarnings...)
	probeOptions.Shell = shell
	// PATH was resolved above. Prevent a second resolver pass from reading a
	// different host state, even if a caller explicitly enabled static probing.
	probeOptions.EnableSafeLoginShellPATHProbe = false
	snapshot := tool.ProbeExecutionEnvironment(ctx, probeOptions)
	exactPATH := snapshot.PinnedTransportPATH()
	environment = applyShellEnvironmentOverrides(environment, map[string]string{"PATH": exactPATH})

	return HostShellEnvironment{
		Environment: append([]string(nil), environment...),
		Shell:       shell,
		PATH:        exactPATH,
		Snapshot:    snapshot,
	}
}

// hostShellEnvironmentFromSnapshot reconstructs the process-visible PATH and
// shell from a run-pinned snapshot. Non-PATH environment values still come
// from the current awaken-core process because the snapshot intentionally does
// not retain secrets or arbitrary user values.
func hostShellEnvironmentFromSnapshot(snapshot tool.ExecutionEnvironmentSnapshot) HostShellEnvironment {
	environment := inheritedShellEnvironment()
	shell := strings.TrimSpace(snapshot.Shell)
	if shell == "" {
		shell = resolveHostShellExecutable("", environment)
	}
	pathValue := snapshot.PinnedTransportPATH()
	environment = applyShellEnvironmentOverrides(environment, map[string]string{
		"PATH":  pathValue,
		"SHELL": shell,
	})
	return HostShellEnvironment{
		Environment: environment,
		Shell:       shell,
		PATH:        pathValue,
		Snapshot:    snapshot.Clone(),
	}
}

func resolveHostShellExecutable(preferred string, environment []string) string {
	candidates := []string{preferred, envValue(environment, "SHELL")}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, envValueForOS(environment, "COMSPEC", "windows"), "sh", "cmd.exe")
	} else {
		candidates = append(candidates, "/bin/sh")
	}
	for _, candidate := range candidates {
		if resolved := resolveShellCandidate(candidate, environment); resolved != "" && shellSupportsNoRCTransport(resolved) {
			return resolved
		}
	}
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "/bin/sh"
}

func shellSupportsNoRCTransport(path string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(path))) {
	case "sh", "sh.exe", "dash", "dash.exe", "ksh", "ksh.exe", "mksh", "mksh.exe",
		"bash", "bash.exe", "zsh", "zsh.exe", "fish", "fish.exe", "nu", "nu.exe",
		"xonsh", "xonsh.exe", "csh", "csh.exe", "tcsh", "tcsh.exe",
		"pwsh", "pwsh.exe", "powershell", "powershell.exe", "cmd", "cmd.exe":
		return true
	default:
		return false
	}
}

func resolveShellCandidate(candidate string, environment []string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || candidate == "cmd.exe_or_sh" {
		return ""
	}
	if filepath.IsAbs(candidate) {
		if shellCandidateExecutable(candidate) {
			return filepath.Clean(candidate)
		}
		return ""
	}
	if strings.ContainsAny(candidate, `/\\`) {
		return ""
	}
	extensions := []string{""}
	if runtime.GOOS == "windows" {
		extensions = windowsExecutableExtensions(environment)
	}
	for _, directory := range filepath.SplitList(envValue(environment, "PATH")) {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		for _, extension := range extensions {
			path := filepath.Join(directory, candidate)
			if extension != "" && !strings.HasSuffix(strings.ToLower(path), strings.ToLower(extension)) {
				path += extension
			}
			if shellCandidateExecutable(path) {
				return filepath.Clean(path)
			}
		}
	}
	return ""
}

func shellCandidateExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}

func windowsExecutableExtensions(environment []string) []string {
	value := strings.TrimSpace(envValueForOS(environment, "PATHEXT", "windows"))
	if value == "" {
		value = ".COM;.EXE;.BAT;.CMD"
	}
	result := []string{""}
	for _, extension := range strings.Split(value, ";") {
		extension = strings.TrimSpace(extension)
		if extension != "" {
			result = append(result, extension)
		}
	}
	return result
}
