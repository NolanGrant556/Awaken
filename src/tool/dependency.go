package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"awaken/src/tool/sandbox"
)

type DependencyKind string

const (
	DependencyOSExecutable     DependencyKind = "os_executable"
	DependencyLanguageRuntime  DependencyKind = "language_runtime"
	DependencyLanguagePackage  DependencyKind = "language_package"
	DependencyPackageManager   DependencyKind = "package_manager"
	DependencyDesktopApp       DependencyKind = "desktop_application"
	DependencyMCPConnector     DependencyKind = "mcp_connector"
	DependencyNetworkService   DependencyKind = "network_service"
	DependencyPackagedResource DependencyKind = "packaged_resource"
)

// DependencyRequirement is provider-authored, observable readiness metadata.
// It does not make a Skill or tool unavailable at catalog-discovery time.
type DependencyRequirement struct {
	Kind              DependencyKind `json:"kind"`
	Name              string         `json:"name"`
	VersionConstraint string         `json:"version_constraint,omitempty"`
	ProbeID           string         `json:"probe_id,omitempty"`
	Optional          bool           `json:"optional,omitempty"`
	Source            string         `json:"source,omitempty"`
}

type DependencyStatus string

const (
	DependencyUnknown  DependencyStatus = "unknown"
	DependencyReady    DependencyStatus = "ready"
	DependencyMissing  DependencyStatus = "missing"
	DependencyMismatch DependencyStatus = "version_mismatch"
	DependencyError    DependencyStatus = "error"
)

type DependencyObservation struct {
	Requirement           DependencyRequirement  `json:"requirement"`
	Status                DependencyStatus       `json:"status"`
	Observed              string                 `json:"observed,omitempty"`
	Diagnostic            string                 `json:"diagnostic,omitempty"`
	CandidateInstallPlans []CandidateInstallPlan `json:"candidate_install_plans,omitempty"`
}

type InstallScope string

const (
	InstallWorkspace      InstallScope = "workspace"
	InstallManagedRuntime InstallScope = "managed_runtime"
	InstallUser           InstallScope = "user"
	InstallSystem         InstallScope = "system"
)

// CandidateInstallPlan is resolver output, not permission to execute. It must
// be built from a provider manifest and a real environment probe, never copied
// from untrusted free text.
type CandidateInstallPlan struct {
	PlanID                 string            `json:"plan_id"`
	EnvironmentID          string            `json:"environment_snapshot_id"`
	RequirementFingerprint string            `json:"requirement_fingerprint"`
	Executable             string            `json:"executable"`
	ExecutableFingerprint  string            `json:"executable_fingerprint"`
	Args                   []string          `json:"args"`
	CWD                    string            `json:"cwd,omitempty"`
	EnvDelta               map[string]string `json:"env_delta,omitempty"`
	InstallScope           InstallScope      `json:"install_scope"`
	RequiresAdmin          bool              `json:"requires_admin"`
	ModifiesManifest       bool              `json:"modifies_manifest,omitempty"`
	Source                 string            `json:"source"`
	Provenance             string            `json:"provenance"`
}

func cloneDependencyRequirements(values []DependencyRequirement) []DependencyRequirement {
	if values == nil {
		return nil
	}
	result := make([]DependencyRequirement, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		value.ProbeID = strings.TrimSpace(value.ProbeID)
		if value.Name == "" {
			continue
		}
		keyBytes, _ := json.Marshal(value)
		key := string(keyBytes)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneDependencyObservations(values []DependencyObservation) []DependencyObservation {
	if values == nil {
		return nil
	}
	result := make([]DependencyObservation, len(values))
	for index, value := range values {
		value.CandidateInstallPlans = cloneCandidateInstallPlans(value.CandidateInstallPlans)
		result[index] = value
	}
	return result
}

func cloneCandidateInstallPlans(values []CandidateInstallPlan) []CandidateInstallPlan {
	result := make([]CandidateInstallPlan, len(values))
	for index, value := range values {
		value.Args = append([]string(nil), value.Args...)
		value.EnvDelta = cloneStringMap(value.EnvDelta)
		result[index] = value
	}
	return result
}

func observeDependencyRequirements(requirements []DependencyRequirement, environment ExecutionEnvironmentSnapshot) []DependencyObservation {
	observations := make([]DependencyObservation, 0, len(requirements))
	for _, requirement := range requirements {
		observation := DependencyObservation{Requirement: requirement, Status: DependencyUnknown}
		switch requirement.Kind {
		case DependencyOSExecutable, DependencyLanguageRuntime, DependencyPackageManager:
			if executable, ok := environment.Executable(requirement.Name); ok {
				observation.Status = DependencyReady
				observation.Observed = strings.TrimSpace(executable.Path + " " + executable.Version)
				if requirement.VersionConstraint != "" && executable.Version == "" {
					observation.Status = DependencyUnknown
					observation.Diagnostic = "executable was found but its version was not observed"
				}
			} else {
				observation.Status = DependencyMissing
				observation.Diagnostic = "not found in the pinned execution environment snapshot"
			}
		case DependencyDesktopApp:
			if executable, ok := environment.Executable(requirement.Name); ok {
				observation.Status = DependencyReady
				observation.Observed = executable.Path
			} else {
				observation.Status = DependencyMissing
				observation.Diagnostic = "desktop application executable was not observed"
			}
		case DependencyLanguagePackage:
			observation = probeLanguagePackageDependency(requirement, environment)
		case DependencyMCPConnector, DependencyNetworkService:
			observation.Status = DependencyUnknown
			observation.Diagnostic = "connection and authentication readiness require the concrete provider probe"
		case DependencyPackagedResource:
			observation.Status = DependencyUnknown
			observation.Diagnostic = "packaged resources are resolved on demand by their owning provider"
		default:
			observation.Status = DependencyUnknown
			observation.Diagnostic = "dependency kind is not supported by the generic resolver"
		}
		if observation.Status != DependencyReady {
			observation.CandidateInstallPlans = candidateInstallPlans(requirement, environment)
		}
		observations = append(observations, observation)
	}
	return observations
}

// ObserveDependencyRequirement performs the same bounded, read-only probe used
// by run snapshot construction. Typed dependency installation uses it only on
// a freshly captured environment snapshot; it never mutates the current run's
// pinned availability.
func ObserveDependencyRequirement(requirement DependencyRequirement, environment ExecutionEnvironmentSnapshot) DependencyObservation {
	observations := observeDependencyRequirements([]DependencyRequirement{requirement}, environment)
	if len(observations) == 0 {
		return DependencyObservation{Requirement: requirement, Status: DependencyUnknown, Diagnostic: "dependency observation was unavailable"}
	}
	result := observations[0]
	result.CandidateInstallPlans = cloneCandidateInstallPlans(result.CandidateInstallPlans)
	return result
}

// ObserveDependencyRequirementWithSandbox is the typed-install verification
// path. It uses the exact already-selected full sandbox adapter and never
// falls back to an unsupervised language runtime process.
func ObserveDependencyRequirementWithSandbox(requirement DependencyRequirement, environment ExecutionEnvironmentSnapshot, supervisor sandbox.SandboxAdapter) DependencyObservation {
	if requirement.Kind != DependencyLanguagePackage {
		return ObserveDependencyRequirement(requirement, environment)
	}
	result := probeLanguagePackageDependencyWithSandbox(requirement, environment, supervisor)
	if result.Status != DependencyReady {
		result.CandidateInstallPlans = candidateInstallPlans(requirement, environment)
	}
	return result
}

func enrichDependencyObservations(values []DependencyObservation, environment ExecutionEnvironmentSnapshot) []DependencyObservation {
	result := cloneDependencyObservations(values)
	for index := range result {
		// Provider readiness may describe the requirement and observed state,
		// but executable install plans are always regenerated by the host. Never
		// preserve provider/Skill/MCP-authored command, argv, or provenance.
		result[index].CandidateInstallPlans = nil
		if result[index].Status != DependencyReady {
			result[index].CandidateInstallPlans = candidateInstallPlans(result[index].Requirement, environment)
		}
	}
	return result
}

var (
	safeHostDependencyName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	safePythonDistribution = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	safeNodePackage        = regexp.MustCompile(`^(?:[A-Za-z0-9][A-Za-z0-9._-]{0,127}|@[A-Za-z0-9][A-Za-z0-9._-]{0,62}/[A-Za-z0-9][A-Za-z0-9._-]{0,127})$`)
)

func candidateInstallPlans(requirement DependencyRequirement, environment ExecutionEnvironmentSnapshot) []CandidateInstallPlan {
	name := strings.TrimSpace(requirement.Name)
	source := strings.ToLower(strings.TrimSpace(requirement.Source))
	if name == "" || strings.Contains(name, "..") {
		return nil
	}
	if requirement.Kind == DependencyLanguagePackage {
		switch source {
		case "python", "pip", "pypi":
			if !safePythonDistribution.MatchString(name) {
				return nil
			}
		case "node", "npm", "npmjs":
			if !safeNodePackage.MatchString(name) {
				return nil
			}
		default:
			return nil
		}
	} else if !safeHostDependencyName.MatchString(name) {
		return nil
	}
	cwd := firstNonBlank(environment.WorkspaceRoot, environment.WorkingDirectory)
	managers := make(map[string]ExecutableObservation, len(environment.AvailablePackageManagers))
	for _, manager := range environment.AvailablePackageManagers {
		if executable, ok := environment.Executable(manager); ok {
			managers[strings.ToLower(manager)] = executable
		}
	}
	var plans []CandidateInstallPlan
	appendPlan := func(manager string, args []string, scope InstallScope, requiresAdmin bool, modifiesManifest bool) {
		executable, ok := managers[manager]
		if !ok || environment.IsTaskControlledPath(executable.Path) {
			return
		}
		fingerprint := strings.TrimSpace(executable.Fingerprint)
		if fingerprint == "" {
			fingerprint, _ = FingerprintExecutable(executable.Path)
		}
		if fingerprint == "" {
			return
		}
		plan := CandidateInstallPlan{
			EnvironmentID: environment.SnapshotID, RequirementFingerprint: dependencyRequirementFingerprint(requirement),
			Executable: executable.Path, ExecutableFingerprint: fingerprint,
			Args: append([]string(nil), args...), CWD: cwd,
			InstallScope: scope, RequiresAdmin: requiresAdmin, ModifiesManifest: modifiesManifest,
			Source: manager, Provenance: "host_dependency_resolver_v1",
		}
		plan.PlanID = candidateInstallPlanIdentity(plan)
		plans = append(plans, plan)
	}

	if requirement.Kind == DependencyLanguagePackage {
		if strings.TrimSpace(cwd) == "" {
			return nil
		}
		switch source {
		case "python", "pip", "pypi":
			packageSpec, ok := dependencyPackageSpec(name, requirement.VersionConstraint, "python")
			if !ok {
				return nil
			}
			// Keep workspace destinations relative so the typed installer can
			// execute the exact argv inside a private broker staging workspace.
			// An absolute live-workspace path would bypass that path remapping.
			target := filepath.Join(".awaken", "deps", "python")
			appendPlan("uv", []string{"pip", "install", "--no-cache", "--only-binary", ":all:", "--target", target, packageSpec}, InstallWorkspace, false, false)
			pipArgs := []string{"install", "--disable-pip-version-check", "--no-input", "--no-cache-dir", "--only-binary=:all:", "--target", target, packageSpec}
			appendPlan("pip", pipArgs, InstallWorkspace, false, false)
			appendPlan("pip3", pipArgs, InstallWorkspace, false, false)
		case "node", "npm", "npmjs":
			packageSpec, ok := dependencyPackageSpec(name, requirement.VersionConstraint, "node")
			if !ok {
				return nil
			}
			appendPlan("npm", []string{"install", "--save-exact", "--ignore-scripts", "--no-audit", "--no-fund", packageSpec}, InstallWorkspace, false, true)
			appendPlan("pnpm", []string{"add", "--save-exact", "--ignore-scripts", "--reporter=silent", packageSpec}, InstallWorkspace, false, true)
			yarnArgs := []string{"add", "--exact", "--ignore-scripts", "--non-interactive", packageSpec}
			appendPlan("yarn", yarnArgs, InstallWorkspace, false, true)
		}
		return plans
	}

	packageNames := hostPackageNames(environment.OS, name)
	if packageNames.brew != "" {
		appendPlan("brew", append([]string{"install"}, packageNames.brewArgs...), InstallUser, false, false)
	}
	if packageNames.winget != "" {
		appendPlan("winget", []string{"install", "--id", packageNames.winget, "--exact", "--silent"}, InstallUser, false, false)
	}
	if packageNames.choco != "" {
		appendPlan("choco", []string{"install", packageNames.choco, "--yes", "--no-progress"}, InstallSystem, true, false)
	}
	if packageNames.apt != "" {
		appendPlan("apt", []string{"install", "--yes", packageNames.apt}, InstallSystem, true, false)
	}
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Source < plans[j].Source })
	return plans
}

func dependencyRequirementFingerprint(requirement DependencyRequirement) string {
	data, _ := json.Marshal(requirement)
	digest := sha256.Sum256(data)
	return "dependency_" + hex.EncodeToString(digest[:12])
}

var safeDependencyVersionConstraint = regexp.MustCompile(`^[0-9vV<>=!~^.*,+ -]{1,64}$`)

func dependencyPackageSpec(name string, constraint string, ecosystem string) (string, bool) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return name, true
	}
	if !safeDependencyVersionConstraint.MatchString(constraint) || strings.ContainsAny(constraint, `/\\`) {
		return "", false
	}
	if ecosystem == "node" {
		constraint = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(constraint, "=="), "="))
		if constraint == "" {
			return "", false
		}
		return name + "@" + constraint, true
	}
	if strings.HasPrefix(constraint, "=") && !strings.HasPrefix(constraint, "==") {
		constraint = "=" + constraint
	}
	return name + constraint, true
}

func candidateInstallPlanIdentity(plan CandidateInstallPlan) string {
	plan.PlanID = ""
	data, _ := json.Marshal(plan)
	digest := sha256.Sum256(data)
	return "install_plan_" + hex.EncodeToString(digest[:12])
}

type hostPackageMapping struct {
	brew, winget, choco, apt string
	brewArgs                 []string
}

func hostPackageNames(goos string, executable string) hostPackageMapping {
	key := strings.ToLower(filepath.Base(executable))
	mappings := map[string]hostPackageMapping{
		"soffice":     {brew: "libreoffice", brewArgs: []string{"--cask", "libreoffice"}, winget: "TheDocumentFoundation.LibreOffice", choco: "libreoffice-fresh", apt: "libreoffice"},
		"libreoffice": {brew: "libreoffice", brewArgs: []string{"--cask", "libreoffice"}, winget: "TheDocumentFoundation.LibreOffice", choco: "libreoffice-fresh", apt: "libreoffice"},
		"pdftoppm":    {brew: "poppler", brewArgs: []string{"poppler"}, winget: "oschwartz10612.Poppler", choco: "poppler", apt: "poppler-utils"},
		"ffmpeg":      {brew: "ffmpeg", brewArgs: []string{"ffmpeg"}, winget: "Gyan.FFmpeg", choco: "ffmpeg", apt: "ffmpeg"},
		"git":         {brew: "git", brewArgs: []string{"git"}, winget: "Git.Git", choco: "git", apt: "git"},
		"node":        {brew: "node", brewArgs: []string{"node"}, winget: "OpenJS.NodeJS.LTS", choco: "nodejs-lts", apt: "nodejs"},
		"python":      {brew: "python", brewArgs: []string{"python"}, winget: "Python.Python.3.12", choco: "python312", apt: "python3"},
		"python3":     {brew: "python", brewArgs: []string{"python"}, winget: "Python.Python.3.12", choco: "python312", apt: "python3"},
	}
	mapping := mappings[key]
	switch strings.ToLower(goos) {
	case "darwin":
		mapping.winget, mapping.choco, mapping.apt = "", "", ""
	case "windows":
		mapping.brew, mapping.brewArgs, mapping.apt = "", nil, ""
	case "linux":
		mapping.brew, mapping.brewArgs, mapping.winget, mapping.choco = "", nil, "", ""
	}
	return mapping
}
