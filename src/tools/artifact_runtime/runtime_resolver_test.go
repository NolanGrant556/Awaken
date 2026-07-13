package artifactruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeHostTool(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func setHostOverrides(t *testing.T, python, soffice, popplerBin, node, ffmpeg string) {
	t.Helper()
	t.Setenv("AWAKEN_PYTHON", python)
	t.Setenv("AWAKEN_SOFFICE", soffice)
	t.Setenv("AWAKEN_POPPLER_BIN", popplerBin)
	t.Setenv("AWAKEN_NODE", node)
	t.Setenv("AWAKEN_FFMPEG", ffmpeg)
}

func TestHostPreflightUsesExistingUserToolsWithoutManagedRuntime(t *testing.T) {
	dir := t.TempDir()
	python := writeHostTool(t, dir, "python3")
	soffice := writeHostTool(t, dir, "soffice")
	node := writeHostTool(t, dir, "node")
	ffmpeg := writeHostTool(t, dir, "ffmpeg")
	writeHostTool(t, dir, "pdftoppm")
	writeHostTool(t, dir, "pdfinfo")
	setHostOverrides(t, python, soffice, dir, node, ffmpeg)

	report := runtimePreflight()
	if !report.Ready || len(report.Missing) != 0 {
		t.Fatalf("expected host environment ready, got %#v", report)
	}
	if report.RuntimeName != "host-environment" {
		t.Fatalf("unexpected environment name %q", report.RuntimeName)
	}
}

func TestPresentationResolversAcceptExplicitExistingInstallations(t *testing.T) {
	dir := t.TempDir()
	python := writeHostTool(t, dir, "python3")
	soffice := writeHostTool(t, dir, "soffice")
	writeHostTool(t, dir, "pdftoppm")
	setHostOverrides(t, python, soffice, dir, "", "")

	if got, err := resolvePresentationPython(); err != nil || got != python {
		t.Fatalf("python got %q err=%v", got, err)
	}
	if got, err := resolveSoffice(); err != nil || got != soffice {
		t.Fatalf("soffice got %q err=%v", got, err)
	}
	if got, err := resolvePopplerTool("pdftoppm"); err != nil || got == "" {
		t.Fatalf("pdftoppm got %q err=%v", got, err)
	}
}

func TestHostPreflightDoesNotCacheMissingTools(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "")
	setHostOverrides(t, "", "", "", "", "")
	before := runtimePreflightCached(false)
	if len(before.Missing) == 0 {
		t.Fatal("expected missing host dependencies")
	}

	python := writeHostTool(t, dir, "python3")
	soffice := writeHostTool(t, dir, "soffice")
	node := writeHostTool(t, dir, "node")
	ffmpeg := writeHostTool(t, dir, "ffmpeg")
	writeHostTool(t, dir, "pdftoppm")
	writeHostTool(t, dir, "pdfinfo")
	setHostOverrides(t, python, soffice, dir, node, ffmpeg)
	after := runtimePreflightCached(false)
	if !after.Ready {
		t.Fatalf("expected install-and-retry refresh, got %#v", after)
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
}
