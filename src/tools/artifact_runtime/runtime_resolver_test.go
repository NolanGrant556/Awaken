package artifactruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveAwakenUserDataDirEnvFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWAKEN_USER_DATA_DIR", dir)
	got, err := resolveAwakenUserDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
}

func TestManagedComponentPathDoesNotRequireUnrelatedComponents(t *testing.T) {
	component := filepath.Join(t.TempDir(), "python")
	if err := os.WriteFile(component, []byte("python"), 0o755); err != nil {
		t.Fatal(err)
	}
	globalRuntimeProbeCache.mu.Lock()
	globalRuntimeProbeCache.report = preflightReport{
		RuntimeName: runtimeName,
		Ready:       false,
		Missing:     []string{"ffmpeg"},
		Components: []runtimeComponentStatus{
			{Name: "python", OK: true, ResolvedPath: component},
			{Name: "ffmpeg", OK: false},
		},
	}
	globalRuntimeProbeCache.expiresAt = time.Now().Add(time.Minute)
	globalRuntimeProbeCache.mu.Unlock()
	t.Cleanup(invalidateRuntimeProbeCache)

	got, ok := managedComponentPath("python")
	if !ok || got != component {
		t.Fatalf("expected ready python despite unrelated ffmpeg failure, got %q ok=%v", got, ok)
	}
}

func TestManagedComponentPathRejectsFailedProvenance(t *testing.T) {
	component := filepath.Join(t.TempDir(), "python")
	if err := os.WriteFile(component, []byte("python"), 0o755); err != nil {
		t.Fatal(err)
	}
	globalRuntimeProbeCache.mu.Lock()
	globalRuntimeProbeCache.report = preflightReport{
		RuntimeName: runtimeName,
		Ready:       false,
		Missing:     []string{"provenance"},
		Components:  []runtimeComponentStatus{{Name: "python", OK: true, ResolvedPath: component}},
	}
	globalRuntimeProbeCache.expiresAt = time.Now().Add(time.Minute)
	globalRuntimeProbeCache.mu.Unlock()
	t.Cleanup(invalidateRuntimeProbeCache)
	if got, ok := managedComponentPath("python"); ok || got != "" {
		t.Fatalf("expected provenance failure to block managed component, got %q ok=%v", got, ok)
	}
}

func TestPresentationRuntimeRequirementsIgnoreUnrelatedComponentFailure(t *testing.T) {
	report := preflightReport{
		Ready:   false,
		Missing: []string{"ffmpeg"},
		Components: []runtimeComponentStatus{
			{Name: "python", OK: true},
			{Name: "libreoffice", OK: true},
			{Name: "poppler", OK: true},
			{Name: "ffmpeg", OK: false},
		},
	}
	if missing := missingRuntimeRequirements(report, presentationAuthoringRuntimeRequirements); len(missing) != 0 {
		t.Fatalf("presentation authoring must not require unrelated ffmpeg: %v", missing)
	}
	if missing := missingRuntimeRequirements(report, []string{"python", "node"}); len(missing) != 1 || missing[0] != "node" {
		t.Fatalf("expected only node missing, got %v", missing)
	}
}

func TestResolvePrimaryRuntimeRootOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", dir)
	got, err := resolvePrimaryRuntimeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
}

func TestRuntimePreflightRejectsMissingCurrent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", root)
	t.Setenv("AWAKEN_PYTHON", "")
	t.Setenv("AWAKEN_SOFFICE", "")
	t.Setenv("AWAKEN_POPPLER_BIN", "")
	invalidateRuntimeProbeCache()
	report := runtimePreflight()
	if report.Ready {
		t.Fatal("expected not ready without current.json")
	}
	if len(report.Missing) == 0 || report.Missing[0] != "current" {
		t.Fatalf("expected missing current, got %#v", report.Missing)
	}
}

func TestPresentationResolversIgnorePrepareOnlySourceEnvironment(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", root)
	external := t.TempDir()
	python := filepath.Join(external, "python")
	soffice := filepath.Join(external, "soffice")
	poppler := filepath.Join(external, "poppler")
	if err := os.MkdirAll(poppler, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{python, soffice, filepath.Join(poppler, "pdftoppm")} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AWAKEN_PYTHON", python)
	t.Setenv("AWAKEN_SOFFICE", soffice)
	t.Setenv("AWAKEN_POPPLER_BIN", poppler)
	invalidateRuntimeProbeCache()
	t.Cleanup(invalidateRuntimeProbeCache)

	if got, err := resolvePresentationPython(); err == nil || got != "" {
		t.Fatalf("prepare-only AWAKEN_PYTHON must not bypass managed runtime, got %q err=%v", got, err)
	}
	if got, err := resolveSoffice(); err == nil || got != "" {
		t.Fatalf("prepare-only AWAKEN_SOFFICE must not bypass managed runtime, got %q err=%v", got, err)
	}
	if got, err := resolvePopplerTool("pdftoppm"); err == nil || got != "" {
		t.Fatalf("prepare-only AWAKEN_POPPLER_BIN must not bypass managed runtime, got %q err=%v", got, err)
	}
}

func TestRuntimePreflightAgainstPreparedIntegrationRoot(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("AWAKEN_TEST_PRIMARY_RUNTIME_ROOT"))
	if root == "" {
		t.Skip("set AWAKEN_TEST_PRIMARY_RUNTIME_ROOT to a prepared isolated runtime")
	}
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", root)
	t.Setenv("AWAKEN_PYTHON", "")
	t.Setenv("AWAKEN_SOFFICE", "")
	t.Setenv("AWAKEN_POPPLER_BIN", "")
	invalidateRuntimeProbeCache()
	report := runtimePreflight()
	if !report.Ready || !report.Provenance.OK {
		t.Fatalf("prepared runtime must be ready with clean provenance: %#v", report)
	}
}

func TestResolveManagedPathRejectsEscapingWrapper(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideBin := filepath.Join(outside, "tool")
	if err := os.WriteFile(outsideBin, []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "tool")
	body := "#!/usr/bin/env bash\nset -euo pipefail\nexec '" + outsideBin + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveManagedPath(wrapper, root); err == nil {
		t.Fatal("expected escaping wrapper to fail")
	}
}

func TestPathInsideRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if !pathInsideRoot(inside, root) {
		t.Fatal("expected inside")
	}
	if pathInsideRoot(t.TempDir(), root) {
		t.Fatal("expected outside")
	}
}

func TestInspectRuntimeProvenanceRejectsPyVenv(t *testing.T) {
	root := t.TempDir()
	pythonRoot := filepath.Join(root, "python")
	if err := os.MkdirAll(pythonRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pythonRoot, "pyvenv.cfg"), []byte("home = /opt/homebrew/bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := inspectRuntimeProvenance(root, root)
	if report.OK {
		t.Fatal("expected pyvenv.cfg to make provenance fail")
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Code == "python_venv_not_relocatable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected python_venv_not_relocatable, got %#v", report.Issues)
	}
}

func TestPythonPrefixesManagedRejectsExternalBasePrefix(t *testing.T) {
	root := t.TempDir()
	python := filepath.Join(root, "python")
	body := "#!/bin/sh\necho '{\"executable\":\"/outside/python\",\"prefix\":\"/outside\",\"base_prefix\":\"/outside\"}'\n"
	if err := os.WriteFile(python, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	ok, detail := pythonPrefixesManaged(python, root)
	if ok || !strings.Contains(detail, "escapes managed runtime") {
		t.Fatalf("expected external prefix rejection, ok=%v detail=%q", ok, detail)
	}
}
