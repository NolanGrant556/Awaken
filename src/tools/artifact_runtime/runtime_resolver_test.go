package artifactruntime

import (
	"os"
	"path/filepath"
	"testing"
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
