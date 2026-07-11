package officerender

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeManagedRuntimeFixture(t *testing.T, root string) (string, string) {
	t.Helper()
	active := "0.3.0/" + managedPlatformName() + "-" + runtime.GOARCH + "/build-test"
	versionRoot := filepath.Join(root, "versions", filepath.FromSlash(active))
	componentRelative := "libreoffice"
	sofficeRelative := filepath.Join("program", "soffice")
	if runtime.GOOS == "darwin" {
		componentRelative = "libreoffice/LibreOffice.app"
		sofficeRelative = filepath.Join("Contents", "MacOS", "soffice")
	} else if runtime.GOOS == "windows" {
		sofficeRelative = filepath.Join("program", "soffice.com")
	}
	componentRoot := filepath.Join(versionRoot, filepath.FromSlash(componentRelative))
	soffice := filepath.Join(componentRoot, sofficeRelative)
	if err := os.MkdirAll(filepath.Dir(soffice), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(soffice, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"bundleVersion": "0.3.0",
		"platform":      managedPlatformName(),
		"arch":          runtime.GOARCH,
		"components": []map[string]any{{
			"name":         "libreoffice",
			"relativePath": componentRelative,
		}},
		"officeRender": map[string]any{
			"component":                 "libreoffice",
			"macOSActivationPolicy":     "background-only",
			"macOSBundleIdentifier":     managedMacOSBundleIdentifier,
			"macOSLaunchServicesClaims": "none",
		},
	}
	writeJSONFixture(t, filepath.Join(root, "current.json"), map[string]any{"active": active})
	writeJSONFixture(t, filepath.Join(versionRoot, "state.json"), state)
	return versionRoot, soffice
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestManagedRuntimeResolverDerivesRuntimeFromActiveState(t *testing.T) {
	root := t.TempDir()
	wantRoot, wantSoffice := writeManagedRuntimeFixture(t, root)
	wantRoot, _ = filepath.EvalSymlinks(wantRoot)
	wantSoffice, _ = filepath.EvalSymlinks(wantSoffice)
	resolver, err := NewManagedRuntimeResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Root != wantRoot || resolved.SOfficePath != wantSoffice {
		t.Fatalf("unexpected managed runtime: %#v", resolved)
	}
	if resolved.Revision == "" || resolved.Attestation == "" {
		t.Fatalf("managed runtime must carry revision and state attestation: %#v", resolved)
	}
}

func TestManagedRuntimeResolverRejectsEscapingComponent(t *testing.T) {
	root := t.TempDir()
	versionRoot, _ := writeManagedRuntimeFixture(t, root)
	outside := t.TempDir()
	component := filepath.Join(versionRoot, "libreoffice")
	if runtime.GOOS == "darwin" {
		component = filepath.Join(versionRoot, "libreoffice", "LibreOffice.app")
	}
	if err := os.RemoveAll(component); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, component); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewManagedRuntimeResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background()); err == nil {
		t.Fatal("expected escaping component to be rejected")
	}
}

func TestManagedRuntimeResolverRejectsUnsafeActiveKey(t *testing.T) {
	root := t.TempDir()
	writeJSONFixture(t, filepath.Join(root, "current.json"), map[string]any{"active": "../../outside"})
	resolver, err := NewManagedRuntimeResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background()); err == nil {
		t.Fatal("expected unsafe active key to be rejected")
	}
}

func TestDefaultManagedRuntimeRootUsesExplicitOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWAKEN_PRIMARY_RUNTIME_ROOT", root)
	resolved, err := DefaultManagedRuntimeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != root {
		t.Fatalf("got %q want %q", resolved, root)
	}
}
