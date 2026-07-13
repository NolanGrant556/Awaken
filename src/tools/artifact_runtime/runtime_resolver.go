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

const runtimeName = "host-environment"

// presentationRuntimeTestOverrides lets package tests inject hermetic tools.
// Production resolution uses the current login environment and ordinary OS
// installation locations; it never copies tools into an Awaken runtime.
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
	SchemaVersion int                      `json:"schemaVersion"`
	RuntimeName   string                   `json:"runtimeName"`
	Ready         bool                     `json:"ready"`
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

func runProbe(path string, args []string, accepted ...int) (bool, string) {
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
	if len(accepted) == 0 {
		accepted = []int{0}
	}
	for _, allowed := range accepted {
		if code == allowed {
			return true, strings.TrimSpace(string(out))
		}
	}
	return false, strings.TrimSpace(string(out))
}

func executableFromPath(names ...string) string {
	for _, name := range names {
		for _, candidate := range runtimeExecutableNames(name) {
			if path, err := exec.LookPath(candidate); err == nil {
				if absolute, absErr := filepath.Abs(path); absErr == nil {
					return absolute
				}
				return path
			}
		}
	}
	return ""
}

func firstUsableFile(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if goruntime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			continue
		}
		if absolute, err := filepath.Abs(candidate); err == nil {
			return absolute
		}
		return candidate
	}
	return ""
}

func hostPythonCandidates() []string {
	candidates := []string{
		strings.TrimSpace(os.Getenv("AWAKEN_PYTHON")),
		presentationRuntimeTestOverride("python"),
		executableFromPath("python3", "python"),
	}
	if goruntime.GOOS == "windows" {
		candidates = append(candidates, executableFromPath("py"))
	}
	return candidates
}

func resolveHostPython(requirePPTX bool) string {
	for _, candidate := range hostPythonCandidates() {
		candidate = firstUsableFile(candidate)
		if candidate == "" {
			continue
		}
		if requirePPTX && !pythonCanImportPPTX(candidate) {
			continue
		}
		return candidate
	}
	return ""
}

func hostSofficeCandidates() []string {
	candidates := []string{
		strings.TrimSpace(os.Getenv("AWAKEN_SOFFICE")),
		presentationRuntimeTestOverride("libreoffice"),
		executableFromPath("soffice", "libreoffice"),
	}
	home, _ := os.UserHomeDir()
	switch goruntime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/LibreOffice.app/Contents/MacOS/soffice",
			filepath.Join(home, "Applications", "LibreOffice.app", "Contents", "MacOS", "soffice"),
		)
	case "windows":
		for _, root := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			if strings.TrimSpace(root) != "" {
				candidates = append(candidates, filepath.Join(root, "LibreOffice", "program", "soffice.exe"))
			}
		}
	default:
		candidates = append(candidates, "/usr/bin/libreoffice", "/usr/bin/soffice", "/usr/local/bin/libreoffice")
	}
	return candidates
}

func resolveHostSoffice() string {
	for _, candidate := range hostSofficeCandidates() {
		if direct := firstUsableFile(candidate); direct != "" {
			return direct
		}
		if soffice := findSofficeIn(candidate); soffice != "" {
			return firstUsableFile(soffice)
		}
	}
	return ""
}

func findSofficeIn(path string) string {
	for _, candidate := range libreOfficeExecutableCandidates(path) {
		if executable := firstUsableFile(candidate); executable != "" {
			return executable
		}
	}
	return ""
}

func resolveHostPopplerTool(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var candidates []string
	if bin := strings.TrimSpace(os.Getenv("AWAKEN_POPPLER_BIN")); bin != "" {
		for _, executable := range runtimeExecutableNames(name) {
			candidates = append(candidates, filepath.Join(bin, executable))
		}
	}
	if bin := strings.TrimSpace(presentationRuntimeTestOverride("poppler")); bin != "" {
		for _, executable := range runtimeExecutableNames(name) {
			candidates = append(candidates, filepath.Join(bin, executable))
		}
	}
	candidates = append(candidates, executableFromPath(name))
	return firstUsableFile(candidates...)
}

func componentStatus(name string) runtimeComponentStatus {
	status := runtimeComponentStatus{Name: name, Required: true}
	switch name {
	case "python":
		status.Kind = "executable"
		path := resolveHostPython(true)
		status.Path, status.ResolvedPath = path, path
		status.OK = path != ""
		if status.OK {
			status.Detail = "python-pptx and Pillow available"
		} else if resolveHostPython(false) != "" {
			status.Detail = "python found but python-pptx or Pillow is missing"
		} else {
			status.Detail = "python3/python not found in the current user environment"
		}
	case "node":
		status.Kind = "executable"
		path := firstUsableFile(strings.TrimSpace(os.Getenv("AWAKEN_NODE")), executableFromPath("node"))
		status.Path, status.ResolvedPath = path, path
		status.OK = path != ""
		status.Detail = map[bool]string{true: "node available", false: "node not found in PATH"}[status.OK]
	case "libreoffice":
		status.Kind = "application"
		path := resolveHostSoffice()
		status.Path, status.ResolvedPath = path, path
		if path != "" {
			status.OK, status.Detail = runProbe(path, []string{"--version"}, 0)
		}
		if !status.OK && status.Detail == "" {
			status.Detail = "LibreOffice/soffice not found in PATH or standard application locations"
		}
	case "poppler":
		status.Kind = "tool-group"
		pdftoppm := resolveHostPopplerTool("pdftoppm")
		pdfinfo := resolveHostPopplerTool("pdfinfo")
		status.Path, status.ResolvedPath = filepath.Dir(pdftoppm), filepath.Dir(pdftoppm)
		status.OK = pdftoppm != "" && pdfinfo != ""
		if status.OK {
			status.Detail = "pdftoppm and pdfinfo available"
		} else {
			status.Detail = "pdftoppm or pdfinfo not found in PATH"
			status.Path, status.ResolvedPath = "", ""
		}
	case "ffmpeg":
		status.Kind = "executable"
		path := firstUsableFile(strings.TrimSpace(os.Getenv("AWAKEN_FFMPEG")), executableFromPath("ffmpeg"))
		status.Path, status.ResolvedPath = path, path
		if path != "" {
			status.OK, status.Detail = runProbe(path, []string{"-version"}, 0)
		}
		if !status.OK && status.Detail == "" {
			status.Detail = "ffmpeg not found in PATH"
		}
	}
	return status
}

// runtimePreflight is retained as the public tool contract name, but it now
// reports the current host environment. It has no active/previous revisions.
func runtimePreflight() preflightReport {
	report := preflightReport{SchemaVersion: 1, RuntimeName: runtimeName}
	for _, name := range []string{"python", "node", "libreoffice", "poppler", "ffmpeg"} {
		status := componentStatus(name)
		report.Components = append(report.Components, status)
		if !status.OK {
			report.Missing = append(report.Missing, name)
		}
	}
	report.Ready = len(report.Missing) == 0
	if !report.Ready {
		report.Detail = "missing dependencies can be installed with the current user Shell and retried"
	}
	return report
}

// Host tools may be installed between two calls in the same run, so preflight
// intentionally does not cache a negative result.
func runtimePreflightCached(bool) preflightReport { return runtimePreflight() }
func invalidateRuntimeProbeCache()                {}

func resolvePresentationPython() (string, error) {
	if path := resolveHostPython(true); path != "" {
		return path, nil
	}
	if path := resolveHostPython(false); path != "" {
		return "", fmt.Errorf("python dependency missing: %s cannot import python-pptx and Pillow", path)
	}
	return "", fmt.Errorf("python dependency missing: python3/python was not found in the current user environment")
}

func resolveSoffice() (string, error) {
	if path := resolveHostSoffice(); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("LibreOffice dependency missing: soffice was not found in PATH or standard application locations")
}

func resolvePopplerTool(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("poppler tool name is required")
	}
	if path := resolveHostPopplerTool(name); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("Poppler dependency missing: %s was not found in PATH", name)
}
