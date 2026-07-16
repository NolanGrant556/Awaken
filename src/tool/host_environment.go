package tool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HostEnvironment is the neutral current-user process environment used by
// process-backed tools. It never starts a login shell or sources rc files.
type HostEnvironment struct {
	Environment []string
	Shell       string
	PATH        string
	Snapshot    ExecutionEnvironmentSnapshot
}

func ResolveHostEnvironment(ctx context.Context, options ExecutionEnvironmentProbeOptions) HostEnvironment {
	if ctx == nil {
		ctx = context.Background()
	}
	environment := append([]string(nil), os.Environ()...)
	shell := resolveHostShell(options.Shell, environment)
	pathOptions := options
	pathOptions.EnableSafeLoginShellPATHProbe = false
	pathValue, pathSource, pathWarnings := ResolveExecutionEnvironmentPATH(ctx, pathOptions, shell, environmentValue(environment, "PATH"))
	environment = overrideEnvironment(environment, map[string]string{"PATH": pathValue, "SHELL": shell})
	probeOptions := options
	probeOptions.PATH = pathValue
	probeOptions.PATHSource = pathSource
	probeOptions.PATHWarnings = append(append([]string(nil), options.PATHWarnings...), pathWarnings...)
	probeOptions.Shell = shell
	probeOptions.EnableSafeLoginShellPATHProbe = false
	snapshot := ProbeExecutionEnvironment(ctx, probeOptions)
	exactPATH := snapshot.PinnedTransportPATH()
	environment = overrideEnvironment(environment, map[string]string{"PATH": exactPATH, "SHELL": shell})
	return HostEnvironment{Environment: environment, Shell: shell, PATH: exactPATH, Snapshot: snapshot}
}

func HostEnvironmentFromSnapshot(snapshot ExecutionEnvironmentSnapshot) HostEnvironment {
	environment := append([]string(nil), os.Environ()...)
	shell := strings.TrimSpace(snapshot.Shell)
	if shell == "" {
		shell = resolveHostShell("", environment)
	}
	pathValue := snapshot.PinnedTransportPATH()
	environment = overrideEnvironment(environment, map[string]string{"PATH": pathValue, "SHELL": shell})
	return HostEnvironment{Environment: environment, Shell: shell, PATH: pathValue, Snapshot: snapshot.Clone()}
}

func resolveHostShell(preferred string, environment []string) string {
	candidates := []string{preferred, environmentValue(environment, "SHELL")}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, environmentValue(environment, "COMSPEC"), "bash.exe", "pwsh.exe", "powershell.exe", "cmd.exe")
	} else {
		candidates = append(candidates, "/bin/bash", "/bin/sh")
	}
	for _, candidate := range candidates {
		if resolved := resolveHostExecutable(candidate, environment); resolved != "" {
			return resolved
		}
	}
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "/bin/sh"
}

func resolveHostExecutable(candidate string, environment []string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || candidate == "cmd.exe_or_sh" {
		return ""
	}
	if filepath.IsAbs(candidate) {
		if executableFile(candidate) {
			return filepath.Clean(candidate)
		}
		return ""
	}
	if strings.ContainsAny(candidate, `/\\`) {
		return ""
	}
	extensions := []string{""}
	if runtime.GOOS == "windows" {
		value := environmentValue(environment, "PATHEXT")
		if value == "" {
			value = ".COM;.EXE;.BAT;.CMD"
		}
		for _, extension := range strings.Split(value, ";") {
			if strings.TrimSpace(extension) != "" {
				extensions = append(extensions, extension)
			}
		}
	}
	for _, directory := range filepath.SplitList(environmentValue(environment, "PATH")) {
		for _, extension := range extensions {
			path := filepath.Join(directory, candidate)
			if extension != "" && !strings.HasSuffix(strings.ToLower(path), strings.ToLower(extension)) {
				path += extension
			}
			if executableFile(path) {
				return filepath.Clean(path)
			}
		}
	}
	return ""
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0)
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

func overrideEnvironment(environment []string, values map[string]string) []string {
	result := make([]string, 0, len(environment)+len(values))
	seen := make(map[string]bool)
	for _, entry := range environment {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(parts[0])
		if value, ok := values[key]; ok {
			if !seen[key] {
				result = append(result, parts[0]+"="+value)
				seen[key] = true
			}
			continue
		}
		result = append(result, entry)
	}
	for key, value := range values {
		upper := strings.ToUpper(key)
		if !seen[upper] {
			result = append(result, key+"="+value)
		}
	}
	return result
}
