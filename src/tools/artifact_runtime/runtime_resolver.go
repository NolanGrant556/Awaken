package artifactruntime

import (
	"context"
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
			status.Detail = "python-pptx and Pillow available"
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
	report.Ready = len(report.Missing) == 0
	return report
}

func managedComponentPath(name string) (string, bool) {
	report := runtimePreflightCached(false)
	if !report.Ready {
		return "", false
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
	if path := strings.TrimSpace(os.Getenv("AWAKEN_PYTHON")); path != "" {
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
	if path := strings.TrimSpace(os.Getenv("AWAKEN_SOFFICE")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
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
	if bin := strings.TrimSpace(os.Getenv("AWAKEN_POPPLER_BIN")); bin != "" {
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
