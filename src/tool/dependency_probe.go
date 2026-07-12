package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool/sandbox"
)

const languagePackageProbeTimeout = time.Second

var (
	pythonPackageProbeName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	nodePackageProbeName      = regexp.MustCompile(`^[A-Za-z0-9_@][A-Za-z0-9._@/+:-]{0,127}$`)
	languageVersionPattern    = regexp.MustCompile(`^[vV]?([0-9]+(?:\.[0-9]+)*)(?:[-+].*)?$`)
	languagePackageProbeCache sync.Map
)

type languagePackageProbeResult struct {
	Found        bool   `json:"found"`
	Version      string `json:"version,omitempty"`
	Path         string `json:"path,omitempty"`
	ResolveError string `json:"resolve_error,omitempty"`
}

type cachedLanguagePackageProbe struct {
	result    DependencyObservation
	expiresAt time.Time
}

func probeLanguagePackageDependency(requirement DependencyRequirement, environment ExecutionEnvironmentSnapshot) DependencyObservation {
	return probeLanguagePackageDependencyWithSandbox(requirement, environment, sandbox.DefaultAdapter())
}

func probeLanguagePackageDependencyWithSandbox(requirement DependencyRequirement, environment ExecutionEnvironmentSnapshot, supervisor sandbox.SandboxAdapter) DependencyObservation {
	observation := DependencyObservation{Requirement: requirement, Status: DependencyUnknown}
	probeKind, packageName, ok := parseLanguagePackageProbeID(requirement.ProbeID)
	if !ok {
		observation.Diagnostic = "language package requires an explicit python_import:<module> or node_resolve:<package> probe_id"
		return observation
	}
	supervisorKey := fmt.Sprintf("%T", supervisor)
	if supervisor != nil {
		supervisorKey += ":" + string(supervisor.SupportLevel())
	}
	cacheKey := environment.SnapshotID + "\x00" + environment.WorkspaceRoot + "\x00" + string(requirement.Kind) + "\x00" + requirement.Name + "\x00" + requirement.VersionConstraint + "\x00" + requirement.ProbeID + "\x00" + supervisorKey
	if cached, found := languagePackageProbeCache.Load(cacheKey); found {
		entry := cached.(cachedLanguagePackageProbe)
		if time.Now().Before(entry.expiresAt) {
			entry.result.Requirement = requirement
			return entry.result
		}
		languagePackageProbeCache.Delete(cacheKey)
	}

	executable, found := languageProbeExecutable(probeKind, environment)
	if !found {
		observation.Status = DependencyMissing
		observation.Diagnostic = probeKind + " runtime was not found in the pinned execution environment snapshot"
		return observation
	}
	result, diagnostic, err := executeLanguagePackageProbe(probeKind, packageName, requirement.Name, executable.Path, environment, supervisor)
	switch {
	case err != nil:
		observation.Status = DependencyError
		observation.Diagnostic = diagnostic
	case !result.Found && strings.TrimSpace(result.ResolveError) != "" && result.ResolveError != "MODULE_NOT_FOUND":
		observation.Status = DependencyError
		observation.Diagnostic = fmt.Sprintf("%s %q resolution failed with %s", probeKind, packageName, result.ResolveError)
	case !result.Found:
		observation.Status = DependencyMissing
		observation.Diagnostic = fmt.Sprintf("%s %q was not resolved by %s", probeKind, packageName, executable.Path)
	default:
		observation.Status = DependencyReady
		observation.Observed = strings.TrimSpace(result.Path + " " + result.Version)
		if constraint := strings.TrimSpace(requirement.VersionConstraint); constraint != "" {
			satisfied, understood := languageVersionSatisfies(result.Version, constraint)
			switch {
			case strings.TrimSpace(result.Version) == "":
				observation.Status = DependencyUnknown
				observation.Diagnostic = "package was resolved but its distribution version was not observed"
			case !understood:
				observation.Status = DependencyUnknown
				observation.Diagnostic = "package version constraint is not supported by the bounded generic comparator"
			case !satisfied:
				observation.Status = DependencyMismatch
				observation.Diagnostic = fmt.Sprintf("observed version %s does not satisfy %s", result.Version, constraint)
			}
		}
	}
	languagePackageProbeCache.Store(cacheKey, cachedLanguagePackageProbe{result: observation, expiresAt: time.Now().Add(30 * time.Second)})
	return observation
}

func parseLanguagePackageProbeID(value string) (kind string, name string, ok bool) {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"python_import:", "node_resolve:"} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		name = strings.TrimSpace(strings.TrimPrefix(value, prefix))
		validName := nodePackageProbeName.MatchString(name)
		if prefix == "python_import:" {
			// importlib.util.find_spec may import a parent package for dotted
			// module names. Restrict the generic probe to one top-level module so
			// package readiness observation never executes package code.
			validName = pythonPackageProbeName.MatchString(name)
		}
		if name == "" || !validName || strings.Contains(name, "..") {
			return "", "", false
		}
		return strings.TrimSuffix(prefix, ":"), name, true
	}
	return "", "", false
}

func languageProbeExecutable(kind string, environment ExecutionEnvironmentSnapshot) (ExecutableObservation, bool) {
	if kind == "python_import" {
		if executable, ok := environment.Executable("python3"); ok {
			return executable, true
		}
		return environment.Executable("python")
	}
	return environment.Executable("node")
}

func executeLanguagePackageProbe(kind, moduleName, distributionName, executable string, environment ExecutionEnvironmentSnapshot, supervisor sandbox.SandboxAdapter) (languagePackageProbeResult, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), languagePackageProbeTimeout)
	defer cancel()
	var command *exec.Cmd
	if kind == "python_import" {
		const script = `import importlib.metadata, importlib.util, json, sys
module_name, distribution_name = sys.argv[1], sys.argv[2]
spec = importlib.util.find_spec(module_name)
version = ""
try:
    version = importlib.metadata.version(distribution_name)
except importlib.metadata.PackageNotFoundError:
    pass
print(json.dumps({"found": spec is not None, "version": version, "path": "" if spec is None else str(spec.origin or "namespace")}))
sys.exit(0 if spec is not None else 3)`
		command = exec.CommandContext(ctx, executable, "-c", script, moduleName, strings.TrimSpace(distributionName))
	} else {
		const script = `const fs = require("node:fs");
const path = require("node:path");
const moduleName = process.argv[1];
const distributionName = process.argv[2] || moduleName;
const paths = [process.cwd(), path.join(process.cwd(), "node_modules"), path.join(process.cwd(), ".awaken", "deps", "node", "node_modules")];
let resolved = "";
let resolveError = "";
try { resolved = require.resolve(moduleName, { paths }); } catch (error) { resolveError = String(error && error.code || "RESOLVE_ERROR"); }

// A package may intentionally hide package.json through exports. That error
// does not mean the package is absent: retry the declared distribution's main
// entry when the requested subpath was blocked.
if (!resolved && resolveError === "ERR_PACKAGE_PATH_NOT_EXPORTED" && distributionName !== moduleName) {
  try { resolved = require.resolve(distributionName, { paths }); } catch (error) { resolveError = String(error && error.code || resolveError); }
}

let version = "";
if (resolved) {
  // Never require() package code or package.json. Walk from the already
  // resolved entry and read the nearest manifest whose name matches the
  // declared distribution. This works even when package.json is not exported.
  if (!resolved.startsWith("node:")) {
    let current = path.dirname(resolved);
    for (let depth = 0; depth < 32; depth++) {
      const manifest = path.join(current, "package.json");
      if (fs.existsSync(manifest)) {
        try {
          const parsed = JSON.parse(fs.readFileSync(manifest, "utf8"));
          if (!parsed.name || parsed.name === distributionName) {
            version = String(parsed.version || "");
            break;
          }
        } catch (_) {}
      }
      const parent = path.dirname(current);
      if (parent === current) break;
      current = parent;
    }
  }
}
process.stdout.write(JSON.stringify({found: Boolean(resolved), version, path: resolved, resolve_error: resolveError}));
process.exit(resolved ? 0 : 3);`
		command = exec.CommandContext(ctx, executable, "-e", script, moduleName, strings.TrimSpace(distributionName))
	}
	command.Dir = firstNonBlank(environment.WorkingDirectory, environment.WorkspaceRoot)
	if command.Dir == "" {
		command.Dir = "."
	}
	command.Env = languagePackageProbeEnvironment(environment)
	stdout := outputlimit.NewCappedBuffer(4096)
	stderr := outputlimit.NewCappedBuffer(2048)
	command.Stdout = stdout
	command.Stderr = stderr
	if supervisor == nil || supervisor.SupportLevel() != sandbox.SandboxFull {
		return languagePackageProbeResult{}, "language package probe requires an applied read-only platform sandbox", ErrExecutionSandboxRequired
	}
	sandboxRoot := firstNonBlank(environment.WorkspaceRoot, environment.WorkingDirectory)
	resultSandbox, sandboxErr := supervisor.ApplySandbox(command, sandbox.TierInspectReadOnly, sandboxRoot)
	if sandboxErr != nil || !resultSandbox.Applied || resultSandbox.Support != sandbox.SandboxFull {
		if sandboxErr == nil {
			sandboxErr = ErrExecutionSandboxRequired
		}
		return languagePackageProbeResult{}, "language package probe read-only sandbox was not applied", sandboxErr
	}
	err := command.Run()
	stdoutText := strings.TrimSpace(stdout.Result().Content)
	stderrText := strings.TrimSpace(stderr.Result().Content)
	if ctx.Err() != nil {
		return languagePackageProbeResult{}, "language package probe timed out", ctx.Err()
	}
	var result languagePackageProbeResult
	if decodeErr := json.Unmarshal([]byte(stdoutText), &result); decodeErr != nil {
		diagnostic := strings.TrimSpace(strings.Join([]string{stdoutText, stderrText}, " "))
		if diagnostic == "" {
			diagnostic = decodeErr.Error()
		}
		return languagePackageProbeResult{}, sanitizeProbeText(diagnostic, 512), decodeErr
	}
	if err != nil && result.Found {
		return languagePackageProbeResult{}, sanitizeProbeText(stderrText, 512), err
	}
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 3 {
			return languagePackageProbeResult{}, sanitizeProbeText(stderrText, 512), err
		}
	}
	return result, "", nil
}

func languagePackageProbeEnvironment(environment ExecutionEnvironmentSnapshot) []string {
	values := []string{
		"PATH=" + environment.PinnedTransportPATH(),
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "PYTHONDONTWRITEBYTECODE=1", "NO_COLOR=1",
	}
	workspace := firstNonBlank(environment.WorkspaceRoot, environment.WorkingDirectory)
	if workspace != "" {
		values = append(values,
			"PYTHONPATH="+filepath.Join(workspace, ".awaken", "deps", "python"),
			"NODE_PATH="+strings.Join([]string{filepath.Join(workspace, "node_modules"), filepath.Join(workspace, ".awaken", "deps", "node", "node_modules")}, string(os.PathListSeparator)),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		values = append(values, "HOME="+home)
	}
	if runtime.GOOS == "windows" {
		for _, key := range []string{"SYSTEMROOT", "WINDIR", "PATHEXT"} {
			if value := os.Getenv(key); value != "" {
				values = append(values, key+"="+value)
			}
		}
	}
	return values
}

func languageVersionSatisfies(observed, constraint string) (bool, bool) {
	observedParts, ok := parseLanguageVersion(observed)
	if !ok {
		return false, false
	}
	clauses := strings.Split(constraint, ",")
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		operator := "=="
		for _, candidate := range []string{"<=", ">=", "==", "=", "<", ">"} {
			if strings.HasPrefix(clause, candidate) {
				operator = candidate
				clause = strings.TrimSpace(strings.TrimPrefix(clause, candidate))
				break
			}
		}
		wantParts, valid := parseLanguageVersion(clause)
		if !valid {
			return false, false
		}
		comparison := compareLanguageVersions(observedParts, wantParts)
		satisfied := map[string]bool{
			"==": comparison == 0, "=": comparison == 0,
			"<": comparison < 0, "<=": comparison <= 0,
			">": comparison > 0, ">=": comparison >= 0,
		}[operator]
		if !satisfied {
			return false, true
		}
	}
	return true, true
}

func parseLanguageVersion(value string) ([]int, bool) {
	match := languageVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 {
		return nil, false
	}
	segments := strings.Split(match[1], ".")
	result := make([]int, len(segments))
	for index, segment := range segments {
		parsed, err := strconv.Atoi(segment)
		if err != nil {
			return nil, false
		}
		result[index] = parsed
	}
	return result, true
}

func compareLanguageVersions(left, right []int) int {
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	for index := 0; index < length; index++ {
		var leftValue, rightValue int
		if index < len(left) {
			leftValue = left[index]
		}
		if index < len(right) {
			rightValue = right[index]
		}
		switch {
		case leftValue < rightValue:
			return -1
		case leftValue > rightValue:
			return 1
		}
	}
	return 0
}
