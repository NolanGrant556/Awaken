package tool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	maxEnvironmentSkillRoots      = 64
	defaultLoginShellMaxBytes     = 64 * 1024
	maxLoginShellMaxBytes         = 256 * 1024
	defaultLoginShellProbeTimeout = 500 * time.Millisecond
	maxLoginShellProbeTimeout     = 2 * time.Second
)

// LoginShellFileReader is injectable for deterministic timeout/cancellation
// tests. Implementations must return at most maxBytes and honor ctx.
type LoginShellFileReader func(ctx context.Context, path string, maxBytes int) ([]byte, error)

var strictPATHAssignment = regexp.MustCompile(`^\s*(?:export\s+)?PATH\s*=\s*(?:"([^"]*)"|'([^']*)'|([^#\s]+))\s*(?:#.*)?$`)
var possiblePATHAssignment = regexp.MustCompile(`(?i)(?:^|\s)(?:export\s+)?PATH\s*=`)

func resolveExecutionEnvironmentPATH(ctx context.Context, options ExecutionEnvironmentProbeOptions, shell string) (string, string, []string) {
	return ResolveExecutionEnvironmentPATH(ctx, options, shell, os.Getenv("PATH"))
}

// ResolveExecutionEnvironmentPATH resolves the exact PATH used by a host
// process surface from a caller-supplied base PATH. It retains the bounded,
// read-only shell-config parser: no login shell is started and no rc statement
// is executed. The Shell executor and ExecutionEnvironmentSnapshot builder use
// this same resolver so provider selection cannot observe a different PATH from
// the one used at transport time.
func ResolveExecutionEnvironmentPATH(ctx context.Context, options ExecutionEnvironmentProbeOptions, shell string, basePATH string) (string, string, []string) {
	if explicit := strings.TrimSpace(options.PATH); explicit != "" {
		source := strings.TrimSpace(options.PATHSource)
		if source == "" {
			source = "explicit_configuration"
		}
		return normalizeExecutionEnvironmentPATH(explicit), sanitizeProbeText(source, 128), nil
	}
	processPATH := normalizeExecutionEnvironmentPATH(basePATH)
	processSource := strings.TrimSpace(options.PATHSource)
	if processSource == "" {
		processSource = "process_environment"
	}
	if !options.EnableSafeLoginShellPATHProbe {
		return processPATH, sanitizeProbeText(processSource, 128), nil
	}
	if runtime.GOOS == "windows" {
		return processPATH, sanitizeProbeText(processSource, 128), []string{"safe shell-config PATH probing is unsupported on Windows; using process_environment without sourcing shell rc"}
	}

	timeout := options.LoginShellProbeTimeout
	if timeout <= 0 {
		timeout = defaultLoginShellProbeTimeout
	}
	if timeout > maxLoginShellProbeTimeout {
		timeout = maxLoginShellProbeTimeout
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	home := strings.TrimSpace(options.LoginShellHome)
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	home, ok := cleanAbsoluteMetadataRoot(home)
	if !ok {
		return processPATH, sanitizeProbeText(processSource, 128), []string{"safe shell-config PATH probe has no valid absolute home; using process_environment without sourcing shell rc"}
	}
	configPaths := append([]string(nil), options.LoginShellConfigPaths...)
	if len(configPaths) == 0 {
		configPaths = defaultLoginShellConfigPaths(shell, home)
	}
	reader := options.ReadLoginShellFile
	if reader == nil {
		reader = readBoundedLoginShellFile
	}
	maxBytes := options.LoginShellMaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultLoginShellMaxBytes
	}
	if maxBytes > maxLoginShellMaxBytes {
		maxBytes = maxLoginShellMaxBytes
	}

	pathEntries := boundedPathEntries(processPATH, 128)
	applied, rejected := 0, 0
	var warnings []string
	remaining := maxBytes
	for _, candidate := range configPaths {
		if err := probeCtx.Err(); err != nil {
			warnings = append(warnings, "safe shell-config PATH probe cancelled or timed out; using only already-proven literal assignments")
			break
		}
		candidate, valid := cleanAbsoluteMetadataRoot(candidate)
		if !valid || !PathWithinRoot(candidate, home) {
			warnings = append(warnings, "ignored shell-config path outside the bounded home root")
			continue
		}
		if remaining <= 0 {
			warnings = append(warnings, "safe shell-config PATH probe reached its total byte limit")
			break
		}
		content, err := reader(probeCtx, candidate, remaining)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			warnings = append(warnings, "could not safely read shell config "+filepath.Base(candidate)+": "+sanitizeProbeText(err.Error(), 160))
			continue
		}
		remaining -= len(content)
		parsed, acceptedCount, unsafeCount := parseLiteralPATHAssignments(content, pathEntries, home)
		if acceptedCount > 0 {
			pathEntries = parsed
			applied += acceptedCount
		}
		rejected += unsafeCount
	}

	if applied == 0 {
		warnings = append(warnings, "safe shell-config PATH probe found no strictly literal PATH assignment; using process_environment and never sourcing shell rc")
		return strings.Join(pathEntries, string(os.PathListSeparator)), sanitizeProbeText(processSource, 128), uniqueProviderStrings(warnings)
	}
	warnings = append(warnings, fmt.Sprintf("PATH includes %d statically parsed literal shell-config assignment(s); shell rc was read-only parsed and never executed", applied))
	if rejected > 0 {
		warnings = append(warnings, fmt.Sprintf("ignored %d non-literal or executable PATH assignment(s) from shell config", rejected))
	}
	return strings.Join(pathEntries, string(os.PathListSeparator)), "shell_config_static_literal", uniqueProviderStrings(warnings)
}

// normalizeExecutionEnvironmentPATH de-duplicates and cleans the complete
// process PATH without imposing the snapshot's presentation bound. The full
// value is retained in-memory for process transport; the serialized snapshot
// carries a digest/count/truncation marker plus a bounded preview.
func normalizeExecutionEnvironmentPATH(value string) string {
	return strings.Join(boundedPathEntries(value, 0), string(os.PathListSeparator))
}

func defaultLoginShellConfigPaths(shell, home string) []string {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(shell))) {
	case "zsh":
		return []string{filepath.Join(home, ".zprofile"), filepath.Join(home, ".zshrc")}
	case "bash":
		return []string{filepath.Join(home, ".bash_profile"), filepath.Join(home, ".bash_login"), filepath.Join(home, ".profile"), filepath.Join(home, ".bashrc")}
	case "sh", "dash", "ksh":
		return []string{filepath.Join(home, ".profile")}
	default:
		return nil
	}
}

func readBoundedLoginShellFile(ctx context.Context, path string, maxBytes int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("config is not a non-symlink regular file")
	}
	if info.Size() < 0 || info.Size() > int64(maxBytes) {
		return nil, fmt.Errorf("config exceeds bounded byte limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make([]byte, 0, info.Size())
	buffer := make([]byte, 4096)
	for len(result) <= maxBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			if len(result)+count > maxBytes {
				return nil, fmt.Errorf("config exceeded bounded byte limit while reading")
			}
			result = append(result, buffer[:count]...)
		}
		if errors.Is(readErr, io.EOF) {
			return result, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return nil, fmt.Errorf("config exceeded bounded byte limit")
}

func parseLiteralPATHAssignments(content []byte, initial []string, home string) ([]string, int, int) {
	current := append([]string(nil), initial...)
	accepted, rejected := 0, 0
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	scanner.Buffer(make([]byte, 4096), 4096)
	for scanner.Scan() {
		line := scanner.Text()
		match := strictPATHAssignment.FindStringSubmatch(line)
		if len(match) != 4 {
			if possiblePATHAssignment.MatchString(line) {
				rejected++
			}
			continue
		}
		value := firstNonBlank(match[1], match[2], match[3])
		if match[2] != "" && strings.ContainsAny(value, "$~") {
			// Shell single quotes suppress variable and tilde expansion. Treating
			// these as expandable would fabricate a PATH the shell never produced.
			rejected++
			continue
		}
		next, ok := expandStrictPATHValue(value, current, home)
		if !ok {
			rejected++
			continue
		}
		current = next
		accepted++
	}
	if scanner.Err() != nil {
		rejected++
	}
	return current, accepted, rejected
}

func expandStrictPATHValue(value string, current []string, home string) ([]string, bool) {
	if value == "" || strings.ContainsAny(value, "`;&|<>()\\") || strings.Contains(value, "${") {
		return nil, false
	}
	var result []string
	for _, segment := range strings.Split(value, ":") {
		segment = strings.TrimSpace(segment)
		switch {
		case segment == "$PATH":
			result = append(result, current...)
			continue
		case segment == "":
			return nil, false // empty PATH segment means current working directory
		case segment == "$HOME":
			segment = home
		case strings.HasPrefix(segment, "$HOME/"):
			segment = filepath.Join(home, strings.TrimPrefix(segment, "$HOME/"))
		case segment == "~":
			segment = home
		case strings.HasPrefix(segment, "~/"):
			segment = filepath.Join(home, strings.TrimPrefix(segment, "~/"))
		case strings.Contains(segment, "$"):
			return nil, false
		}
		if !filepath.IsAbs(segment) || strings.ContainsRune(segment, '\x00') {
			return nil, false
		}
		result = append(result, filepath.Clean(segment))
	}
	result = boundedUniquePaths(result, 128)
	return result, len(result) > 0
}

func normalizeEnvironmentSkillRoots(values []SkillRootObservation) ([]SkillRootObservation, []string) {
	result := make([]SkillRootObservation, 0, minInt(len(values), maxEnvironmentSkillRoots))
	seen := map[string]bool{}
	var warnings []string
	for _, value := range values {
		if len(result) >= maxEnvironmentSkillRoots {
			warnings = append(warnings, fmt.Sprintf("skill root metadata was capped at %d entries", maxEnvironmentSkillRoots))
			break
		}
		path, ok := cleanAbsoluteMetadataRoot(value.Path)
		if !ok {
			warnings = append(warnings, "ignored non-absolute or invalid Skill root metadata")
			continue
		}
		source := sanitizeProbeText(value.Source, 64)
		if source == "" {
			source = "unknown"
		}
		key := source + "\x00" + path
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, SkillRootObservation{Source: source, Path: path})
	}
	return result, warnings
}

func normalizeEnvironmentAbsoluteRoot(value, label string) (string, []string) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	root, ok := cleanAbsoluteMetadataRoot(value)
	if !ok {
		return "", []string{"ignored non-absolute or invalid " + label + " metadata"}
	}
	return root, nil
}

func cleanAbsoluteMetadataRoot(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') || !filepath.IsAbs(value) {
		return "", false
	}
	return filepath.Clean(value), true
}

func boundedUniquePaths(values []string, limit int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, minInt(len(values), limit))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
