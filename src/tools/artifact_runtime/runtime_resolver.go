package artifactruntime

import (
	"context"
	"debug/macho"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
)

const runtimeProbeTTL = 30 * time.Second

type runtimeResolverCache struct {
	mu        sync.Mutex
	report    preflightReport
	expiresAt time.Time
}

var globalRuntimeProbeCache runtimeResolverCache

// presentationRuntimeTestOverrides lets package tests inject hermetic tools
// without reusing the prepare-only AWAKEN_PYTHON/AWAKEN_SOFFICE/
// AWAKEN_POPPLER_BIN source variables at execution time. It is deliberately
// unexported and has no environment-variable bridge, so production execution
// can only resolve components from the verified Awaken-managed runtime.
var presentationRuntimeTestOverrides struct {
	sync.RWMutex
	python     string
	soffice    string
	popplerBin string
}

func presentationRuntimeTestOverride(name string) string {
	presentationRuntimeTestOverrides.RLock()
	defer presentationRuntimeTestOverrides.RUnlock()
	switch name {
	case "python":
		return presentationRuntimeTestOverrides.python
	case "libreoffice":
		return presentationRuntimeTestOverrides.soffice
	case "poppler":
		return presentationRuntimeTestOverrides.popplerBin
	default:
		return ""
	}
}

type preflightReport struct {
	SchemaVersion int                      `json:"schemaVersion,omitempty"`
	RuntimeName   string                   `json:"runtimeName"`
	BundleVersion string                   `json:"bundleVersion,omitempty"`
	Ready         bool                     `json:"ready"`
	ManifestPath  string                   `json:"manifestPath,omitempty"`
	RuntimeRoot   string                   `json:"runtimeRoot,omitempty"`
	VersionRoot   string                   `json:"versionRoot,omitempty"`
	Active        string                   `json:"active,omitempty"`
	Previous      string                   `json:"previous,omitempty"`
	Missing       []string                 `json:"missing,omitempty"`
	Components    []runtimeComponentStatus `json:"components,omitempty"`
	Detail        string                   `json:"detail,omitempty"`
	Provenance    runtimeProvenanceReport  `json:"provenance"`
}

type runtimeProvenanceReport struct {
	OK     bool                     `json:"ok"`
	Issues []runtimeProvenanceIssue `json:"issues,omitempty"`
}

type runtimeProvenanceIssue struct {
	Path   string `json:"path"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type runtimeComponentStatus struct {
	Name         string `json:"name"`
	Kind         string `json:"kind,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Path         string `json:"path,omitempty"`
	ResolvedPath string `json:"resolvedPath,omitempty"`
	OK           bool   `json:"ok"`
	Detail       string `json:"detail,omitempty"`
}

// resolveAwakenUserDataDir mirrors Electron/@awaken/desktop userData resolution.
func resolveAwakenUserDataDir() (string, error) {
	if path := strings.TrimSpace(os.Getenv("AWAKEN_USER_DATA_DIR")); path != "" {
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("unable to resolve Awaken userData")
	}
	switch goruntime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "@awaken", "desktop"), nil
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			return filepath.Join(appData, "@awaken", "desktop"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "@awaken", "desktop"), nil
	default:
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			return filepath.Join(xdg, "@awaken", "desktop"), nil
		}
		return filepath.Join(home, ".config", "@awaken", "desktop"), nil
	}
}

// resolvePrimaryRuntimeRoot returns the Awaken-owned primary-runtime root.
// Only AWAKEN_PRIMARY_RUNTIME_ROOT may override for test/dev.
func resolvePrimaryRuntimeRoot() (string, error) {
	if path := strings.TrimSpace(os.Getenv("AWAKEN_PRIMARY_RUNTIME_ROOT")); path != "" {
		return filepath.Clean(path), nil
	}
	userData, err := resolveAwakenUserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userData, "runtimes", "primary-runtime"), nil
}

func runtimeTemplateManifestPath() string {
	if path := strings.TrimSpace(os.Getenv("AWAKEN_RUNTIME_MANIFEST")); path != "" {
		return path
	}
	candidates := []string{
		filepath.Join("apps", "desktop", "resources", "runtime-manifest", "primary-runtime.json"),
		filepath.Join("resources", "runtime-manifest", "primary-runtime.json"),
		filepath.Join("runtime-manifest", "primary-runtime.json"),
	}
	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates,
				filepath.Join(dir, "apps", "desktop", "resources", "runtime-manifest", "primary-runtime.json"),
			)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "..", "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "Resources", "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "..", "Resources", "runtime-manifest", "primary-runtime.json"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func pathInsideRoot(path, root string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(root) == "" {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(absRoot); err == nil {
		absRoot = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func readBashWrapperTarget(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "exec ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "exec "))
		if rest == "" {
			continue
		}
		if (strings.HasPrefix(rest, "'") && strings.Contains(rest[1:], "'")) ||
			(strings.HasPrefix(rest, `"`) && strings.Contains(rest[1:], `"`)) {
			quote := rest[0]
			end := strings.IndexByte(rest[1:], quote)
			if end >= 0 {
				return rest[1 : end+1]
			}
		}
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func resolveManagedPath(path, root string) (string, error) {
	current := filepath.Clean(path)
	seen := map[string]struct{}{}
	for i := 0; i < 8; i++ {
		if _, ok := seen[current]; ok {
			return "", fmt.Errorf("symlink cycle")
		}
		seen[current] = struct{}{}
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("path missing")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(current)
			if err != nil {
				return "", fmt.Errorf("broken symlink")
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			current = filepath.Clean(target)
			if !pathInsideRoot(current, root) {
				return "", fmt.Errorf("symlink escapes awaken root")
			}
			continue
		}
		if info.Mode().IsRegular() {
			if target := readBashWrapperTarget(current); target != "" {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(current), target)
				}
				target = filepath.Clean(target)
				if _, err := os.Stat(target); err != nil {
					return "", fmt.Errorf("wrapper target missing")
				}
				if !pathInsideRoot(target, root) {
					return "", fmt.Errorf("wrapper escapes awaken root")
				}
				current = target
				continue
			}
		}
		if !pathInsideRoot(current, root) {
			return "", fmt.Errorf("path outside awaken root")
		}
		return current, nil
	}
	return "", fmt.Errorf("resolve depth exceeded")
}

func probeAccepted(code int, accepted []int) bool {
	if len(accepted) == 0 {
		accepted = []int{0}
	}
	for _, value := range accepted {
		if value == code {
			return true
		}
	}
	return false
}

func runProbe(path string, args []string, accepted []int) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			return false, strings.TrimSpace(string(out))
		}
	}
	ok := probeAccepted(code, accepted)
	detail := strings.TrimSpace(string(out))
	return ok, detail
}

func pythonPrefixesManaged(path, root string) (bool, string) {
	ok, output := runProbe(path, []string{
		"-c",
		"import json,sys; print(json.dumps({'executable':sys.executable,'prefix':sys.prefix,'base_prefix':sys.base_prefix}))",
	}, []int{0})
	if !ok {
		return false, "python prefix probe failed"
	}
	var prefixes struct {
		Executable string `json:"executable"`
		Prefix     string `json:"prefix"`
		BasePrefix string `json:"base_prefix"`
	}
	if err := json.Unmarshal([]byte(output), &prefixes); err != nil {
		return false, "python prefix probe returned invalid JSON"
	}
	for _, candidate := range []string{prefixes.Executable, prefixes.Prefix, prefixes.BasePrefix} {
		if !pathInsideRoot(candidate, root) {
			return false, "python prefix/base_prefix escapes managed runtime"
		}
	}
	return true, ""
}

func isAllowedRuntimeSystemDependency(path string) bool {
	normalized := filepath.ToSlash(path)
	if goruntime.GOOS == "darwin" {
		return strings.HasPrefix(normalized, "/System/Library/") ||
			strings.HasPrefix(normalized, "/usr/lib/")
	}
	if goruntime.GOOS == "linux" {
		return strings.HasPrefix(normalized, "/lib/") ||
			strings.HasPrefix(normalized, "/lib64/") ||
			strings.HasPrefix(normalized, "/usr/lib/") ||
			strings.HasPrefix(normalized, "/usr/lib64/")
	}
	return false
}

func isForbiddenRuntimeDependency(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(path))
	return strings.HasPrefix(normalized, "/opt/homebrew/") ||
		strings.HasPrefix(normalized, "/usr/local/") ||
		strings.Contains(normalized, "/.cache/codex-runtimes/") ||
		strings.Contains(normalized, "/codex-runtimes/")
}

func machoImportedLibraries(path string) ([]string, error) {
	if file, err := macho.Open(path); err == nil {
		defer file.Close()
		return file.ImportedLibraries()
	}
	fat, err := macho.OpenFat(path)
	if err != nil {
		return nil, err
	}
	defer fat.Close()
	seen := map[string]struct{}{}
	var libraries []string
	for _, arch := range fat.Arches {
		imports, err := arch.File.ImportedLibraries()
		if err != nil {
			return nil, err
		}
		for _, dependency := range imports {
			if _, ok := seen[dependency]; ok {
				continue
			}
			seen[dependency] = struct{}{}
			libraries = append(libraries, dependency)
		}
	}
	return libraries, nil
}

func machoInstallNamesFromFile(file *macho.File) []string {
	var names []string
	for _, load := range file.Loads {
		dylib, ok := load.(*macho.Dylib)
		if !ok {
			continue
		}
		raw := load.Raw()
		if len(raw) < 4 || file.ByteOrder.Uint32(raw[:4]) != 0xd { // LC_ID_DYLIB
			continue
		}
		names = append(names, dylib.Name)
	}
	return names
}

func machoInstallNames(path string) ([]string, error) {
	if file, err := macho.Open(path); err == nil {
		defer file.Close()
		return machoInstallNamesFromFile(file), nil
	}
	fat, err := macho.OpenFat(path)
	if err != nil {
		return nil, err
	}
	defer fat.Close()
	seen := map[string]struct{}{}
	var names []string
	for _, arch := range fat.Arches {
		for _, name := range machoInstallNamesFromFile(arch.File) {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	return names, nil
}

func inspectRuntimeProvenance(versionRoot, runtimeRoot string) runtimeProvenanceReport {
	report := runtimeProvenanceReport{OK: true}
	fileNames := map[string]struct{}{}
	if goruntime.GOOS == "darwin" {
		_ = filepath.WalkDir(versionRoot, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				fileNames[entry.Name()] = struct{}{}
			}
			return nil
		})
	}
	_ = filepath.WalkDir(versionRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			report.Issues = append(report.Issues, runtimeProvenanceIssue{
				Path: path, Code: "unreadable_runtime_entry", Detail: walkErr.Error(),
			})
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				report.Issues = append(report.Issues, runtimeProvenanceIssue{
					Path: path, Code: "broken_symlink", Detail: err.Error(),
				})
			} else if !pathInsideRoot(target, runtimeRoot) {
				report.Issues = append(report.Issues, runtimeProvenanceIssue{
					Path: path, Code: "external_symlink", Detail: target,
				})
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(entry.Name(), "pyvenv.cfg") {
			report.Issues = append(report.Issues, runtimeProvenanceIssue{
				Path: path, Code: "python_venv_not_relocatable", Detail: "pyvenv.cfg is forbidden",
			})
		}
		if goruntime.GOOS != "darwin" {
			return nil
		}
		dependencies, err := machoImportedLibraries(path)
		if err != nil {
			return nil // ordinary non-Mach-O file
		}
		if installNames, err := machoInstallNames(path); err == nil {
			for _, installName := range installNames {
				if filepath.IsAbs(installName) && !isAllowedRuntimeSystemDependency(installName) {
					code := "external_install_name"
					if isForbiddenRuntimeDependency(installName) {
						code = "forbidden_external_install_name"
					}
					report.Issues = append(report.Issues, runtimeProvenanceIssue{
						Path: path, Code: code, Detail: installName,
					})
				}
			}
		}
		for _, dependency := range dependencies {
			switch {
			case filepath.IsAbs(dependency):
				if isAllowedRuntimeSystemDependency(dependency) {
					continue
				}
				if _, err := os.Stat(dependency); err != nil {
					report.Issues = append(report.Issues, runtimeProvenanceIssue{
						Path: path, Code: "missing_absolute_dependency", Detail: dependency,
					})
					continue
				}
				if pathInsideRoot(dependency, runtimeRoot) {
					continue
				}
				code := "external_dependency"
				if isForbiddenRuntimeDependency(dependency) {
					code = "forbidden_external_dependency"
				}
				report.Issues = append(report.Issues, runtimeProvenanceIssue{
					Path: path, Code: code, Detail: dependency,
				})
			case strings.HasPrefix(dependency, "@loader_path/"):
				target := filepath.Join(filepath.Dir(path), filepath.FromSlash(strings.TrimPrefix(dependency, "@loader_path/")))
				if _, err := os.Stat(target); err != nil {
					report.Issues = append(report.Issues, runtimeProvenanceIssue{
						Path: path, Code: "missing_loader_dependency", Detail: dependency,
					})
				}
			case strings.HasPrefix(dependency, "@rpath/") || strings.HasPrefix(dependency, "@executable_path/"):
				if _, ok := fileNames[filepath.Base(filepath.ToSlash(dependency))]; !ok {
					report.Issues = append(report.Issues, runtimeProvenanceIssue{
						Path: path, Code: "missing_runtime_dependency", Detail: dependency,
					})
				}
			}
		}
		return nil
	})
	report.OK = len(report.Issues) == 0
	return report
}

func findSofficeIn(path string) string {
	for _, candidate := range libreOfficeExecutableCandidates(path) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func probeResolvedComponent(component RuntimeResolvedComponent, versionRoot, runtimeRoot string) runtimeComponentStatus {
	absolute := filepath.Join(versionRoot, filepath.FromSlash(component.RelativePath))
	status := runtimeComponentStatus{
		Name:     component.Name,
		Kind:     component.Kind,
		Required: component.Required,
		Path:     absolute,
	}
	resolved, err := resolveManagedPath(absolute, runtimeRoot)
	if err != nil {
		status.Detail = err.Error()
		return status
	}
	status.ResolvedPath = resolved
	probe := component.Probe
	if probe == nil {
		probe = &RuntimeProbeSpec{}
	}
	switch component.Name {
	case "python":
		args := probe.Args
		if len(args) == 0 {
			args = []string{"-c", "import pptx, PIL"}
		}
		ok, detail := runProbe(resolved, args, probe.AcceptedExitCodes)
		status.OK = ok
		if ok {
			if prefixesOK, prefixDetail := pythonPrefixesManaged(resolved, runtimeRoot); !prefixesOK {
				status.OK = false
				status.Detail = prefixDetail
			} else {
				status.Detail = "python-pptx and Pillow available"
			}
		} else if detail != "" {
			status.Detail = detail
		} else {
			status.Detail = "python probe failed"
		}
	case "node":
		args := probe.Args
		if len(args) == 0 {
			args = []string{"--version"}
		}
		ok, detail := runProbe(resolved, args, probe.AcceptedExitCodes)
		status.OK = ok
		status.Detail = firstNonEmpty(detail, map[bool]string{true: "node ok", false: "node did not execute"}[ok])
	case "libreoffice":
		soffice := findSofficeIn(resolved)
		if soffice == "" && probe.ExecutableRelativePath != "" {
			soffice = filepath.Join(resolved, filepath.FromSlash(probe.ExecutableRelativePath))
		}
		if soffice == "" {
			status.Detail = "soffice not found"
			return status
		}
		managed, err := resolveManagedPath(soffice, runtimeRoot)
		if err != nil {
			status.Detail = err.Error()
			return status
		}
		args := probe.Args
		if len(args) == 0 {
			args = []string{"--version"}
		}
		ok, detail := runProbe(managed, args, probe.AcceptedExitCodes)
		status.OK = ok
		status.ResolvedPath = managed
		status.Detail = firstNonEmpty(detail, map[bool]string{true: "soffice ok", false: "soffice not executable"}[ok])
	case "poppler":
		tools := probe.Tools
		if len(tools) == 0 {
			tools = []string{"pdftoppm", "pdfinfo"}
		}
		ok := true
		for _, toolName := range tools {
			names := runtimeExecutableNames(toolName)
			found := false
			for _, name := range names {
				toolPath := filepath.Join(resolved, name)
				managed, err := resolveManagedPath(toolPath, runtimeRoot)
				if err != nil {
					continue
				}
				args := []string{"-h"}
				if toolName == "pdfinfo" {
					args = []string{"-v"}
				}
				probeOK, detail := runProbe(managed, args, []int{0, 1})
				if probeOK || strings.Contains(detail, "Usage") {
					found = true
					break
				}
			}
			if !found {
				ok = false
				break
			}
		}
		status.OK = ok
		if ok {
			status.Detail = "pdftoppm and pdfinfo available"
		} else {
			status.Detail = "missing pdftoppm or pdfinfo"
		}
	case "ffmpeg":
		args := probe.Args
		if len(args) == 0 {
			args = []string{"-version"}
		}
		ok, _ := runProbe(resolved, args, probe.AcceptedExitCodes)
		status.OK = ok
		if ok {
			status.Detail = "ffmpeg available"
		} else {
			status.Detail = "ffmpeg did not execute"
		}
	case "helper-packages":
		required := probe.RequireFile
		if required == "" {
			required = "runtime.json"
		}
		_, err := os.Stat(filepath.Join(resolved, required))
		status.OK = err == nil
		if status.OK {
			status.Detail = "helper package directory available"
		} else {
			status.Detail = "missing " + required
		}
	default:
		_, err := os.Stat(resolved)
		status.OK = err == nil
		if status.OK {
			status.Detail = "component path exists"
		} else {
			status.Detail = "component missing"
		}
	}
	return status
}

func runtimePreflightCached(force bool) preflightReport {
	globalRuntimeProbeCache.mu.Lock()
	defer globalRuntimeProbeCache.mu.Unlock()
	if !force && time.Now().Before(globalRuntimeProbeCache.expiresAt) && globalRuntimeProbeCache.report.RuntimeName != "" {
		return globalRuntimeProbeCache.report
	}
	report := runtimePreflight()
	globalRuntimeProbeCache.report = report
	globalRuntimeProbeCache.expiresAt = time.Now().Add(runtimeProbeTTL)
	return report
}

func invalidateRuntimeProbeCache() {
	globalRuntimeProbeCache.mu.Lock()
	defer globalRuntimeProbeCache.mu.Unlock()
	globalRuntimeProbeCache.expiresAt = time.Time{}
}

func runtimePreflight() preflightReport {
	manifestPath := runtimeTemplateManifestPath()
	report := preflightReport{
		SchemaVersion: runtimeSchemaVersion,
		RuntimeName:   runtimeName,
		ManifestPath:  manifestPath,
	}
	root, err := resolvePrimaryRuntimeRoot()
	if err != nil {
		report.Missing = []string{"runtime_root"}
		report.Detail = err.Error()
		return report
	}
	report.RuntimeRoot = root

	template, err := loadRuntimeManifestTemplate(manifestPath)
	if err != nil {
		// Template missing is non-fatal if resolved state exists; still useful for names.
		report.Detail = "template manifest unavailable: " + err.Error()
	} else {
		report.RuntimeName = firstNonEmpty(template.RuntimeName, runtimeName)
		report.BundleVersion = template.BundleVersion
	}

	currentPath := filepath.Join(root, "current.json")
	current, err := loadRuntimeCurrent(currentPath)
	if err != nil || strings.TrimSpace(current.Active) == "" {
		report.Missing = []string{"current"}
		report.Detail = "current.json missing or inactive"
		return report
	}
	if strings.Contains(current.Active, "..") {
		report.Missing = []string{"active_version"}
		report.Detail = "invalid active version key"
		return report
	}
	report.Active = current.Active
	report.Previous = current.Previous
	versionRoot := filepath.Join(append([]string{root, "versions"}, strings.Split(current.Active, "/")...)...)
	report.VersionRoot = versionRoot
	if !pathInsideRoot(versionRoot, root) {
		report.Missing = []string{"active_version"}
		report.Detail = "active version outside awaken root"
		return report
	}

	statePath := filepath.Join(versionRoot, "state.json")
	state, err := loadRuntimeResolvedState(statePath)
	if err != nil {
		// Fall back to template relative paths when state is absent.
		if template.SchemaVersion == 0 && len(template.Components) == 0 {
			report.Missing = []string{"resolved_state"}
			return report
		}
		state = RuntimeResolvedState{
			SchemaVersion: runtimeSchemaVersion,
			RuntimeName:   report.RuntimeName,
			BundleVersion: report.BundleVersion,
			Platform:      runtimePlatformName(),
			Arch:          goruntime.GOARCH,
			Root:          versionRoot,
		}
		for _, component := range template.Components {
			state.Components = append(state.Components, RuntimeResolvedComponent{
				Name:         component.Name,
				Kind:         component.Kind,
				Required:     component.Required,
				Version:      component.Version,
				Source:       component.Source,
				RelativePath: componentRelativePath(component),
				SHA256:       component.SHA256,
				Probe:        component.Probe,
			})
		}
	}
	report.BundleVersion = firstNonEmpty(state.BundleVersion, report.BundleVersion)
	report.RuntimeName = firstNonEmpty(state.RuntimeName, report.RuntimeName)

	for _, component := range state.Components {
		status := probeResolvedComponent(component, versionRoot, root)
		report.Components = append(report.Components, status)
		if component.Required && !status.OK {
			report.Missing = append(report.Missing, component.Name)
		}
	}
	report.Provenance = inspectRuntimeProvenance(versionRoot, versionRoot)
	if !report.Provenance.OK {
		report.Missing = append(report.Missing, "provenance")
		if report.Detail == "" && len(report.Provenance.Issues) > 0 {
			report.Detail = report.Provenance.Issues[0].Code + ": " + report.Provenance.Issues[0].Detail
		}
	}
	report.Ready = len(report.Missing) == 0
	return report
}

func managedComponentPath(name string) (string, bool) {
	report := runtimePreflightCached(false)
	for _, missing := range report.Missing {
		if missing == "provenance" {
			return "", false
		}
	}
	for _, component := range report.Components {
		if component.Name == name && component.OK {
			if component.ResolvedPath != "" {
				return component.ResolvedPath, true
			}
			return component.Path, true
		}
	}
	return "", false
}

func resolvePresentationPython() (string, error) {
	if path := strings.TrimSpace(presentationRuntimeTestOverride("python")); path != "" {
		if pythonCanImportPPTX(path) {
			return path, nil
		}
	}
	if path, ok := managedComponentPath("python"); ok {
		return path, nil
	}
	return "", fmt.Errorf("python runtime with python-pptx is not available")
}

func resolveSoffice() (string, error) {
	if path := strings.TrimSpace(presentationRuntimeTestOverride("libreoffice")); path != "" {
		if soffice := findSofficeIn(path); soffice != "" {
			return soffice, nil
		}
	}
	if path, ok := managedComponentPath("libreoffice"); ok {
		if soffice := findSofficeIn(path); soffice != "" {
			return soffice, nil
		}
		return path, nil
	}
	return "", fmt.Errorf("libreoffice runtime is not available")
}

func resolvePopplerTool(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("poppler tool name is required")
	}
	if bin := strings.TrimSpace(presentationRuntimeTestOverride("poppler")); bin != "" {
		for _, toolName := range runtimeExecutableNames(name) {
			candidate := filepath.Join(bin, toolName)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	if path, ok := managedComponentPath("poppler"); ok {
		for _, toolName := range runtimeExecutableNames(name) {
			candidate := filepath.Join(path, toolName)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("poppler %s runtime is not available", name)
}
