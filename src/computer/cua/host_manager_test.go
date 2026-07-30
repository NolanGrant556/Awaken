package cua

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestHostManagerStartsHostOnFirstUse(t *testing.T) {
	scriptPath, workingDir := hostWorkspace(t)
	manager := NewHostManager(HostManagerOptions{
		HostScriptPath: scriptPath,
		WorkingDir:     workingDir,
		StartupTimeout: 30 * time.Second,
		HealthInterval: time.Minute, // Avoid rapid health checks in test.
	})
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := manager.GetClient(ctx)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if !manager.Available() {
		t.Fatal("expected manager to be available")
	}

	hr, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !hr.Healthy {
		t.Fatalf("expected healthy host: %+v", hr)
	}
}

func TestHostManagerUnavailableWhenScriptMissing(t *testing.T) {
	manager := NewHostManager(HostManagerOptions{
		HostScriptPath: "/nonexistent/cua/host.mjs",
		WorkingDir:     "/nonexistent/cua",
		StartupTimeout: 5 * time.Second,
	})
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := manager.GetClient(ctx)
	if err == nil {
		t.Fatal("expected error for missing host script")
	}
	if manager.Available() {
		t.Fatal("expected manager to be unavailable")
	}
}

func TestHostManagerCloseReleasesHost(t *testing.T) {
	scriptPath, workingDir := hostWorkspace(t)
	manager := NewHostManager(HostManagerOptions{
		HostScriptPath: scriptPath,
		WorkingDir:     workingDir,
		StartupTimeout: 30 * time.Second,
		HealthInterval: time.Minute,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := manager.GetClient(ctx)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// After close, the manager should reject further use and the original client
	// should be unusable.
	if manager.Available() {
		t.Fatal("expected manager to be unavailable after close")
	}
	_, err = client.Health(context.Background())
	if err == nil {
		t.Fatal("expected health to fail after close")
	}
}

func TestHostManagerRestartCooldown(t *testing.T) {
	manager := NewHostManager(HostManagerOptions{
		HostScriptPath:  "/nonexistent/cua/host.mjs",
		WorkingDir:      "/nonexistent/cua",
		StartupTimeout:  1 * time.Second,
		RestartCooldown: 500 * time.Millisecond,
	})
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = manager.GetClient(ctx)

	// Immediate retry should be throttled by cooldown.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	start := time.Now()
	_, err := manager.GetClient(ctx2)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error while in cooldown")
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected fast cooldown rejection, took %v", elapsed)
	}
}

func TestHostManagerDiscoversHostWorkspace(t *testing.T) {
	pocScriptPath, pocWorkingDir := hostWorkspace(t)

	// hostWorkspace returns .../apps/desktop/resources/cua-driver.
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(pocWorkingDir))))

	// The packaged product path takes priority when present; otherwise the PoC
	// workspace is accepted.
	packagedScriptPath := filepath.Join(repoRoot, "apps", "desktop", "resources", "cua-driver", "host.mjs")
	packagedWorkingDir := filepath.Join(repoRoot, "apps", "desktop", "resources", "cua-driver")

	expectedScriptPath := pocScriptPath
	expectedWorkingDir := pocWorkingDir
	if _, err := os.Stat(packagedScriptPath); err == nil {
		expectedScriptPath = packagedScriptPath
		expectedWorkingDir = packagedWorkingDir
	} else if _, err := os.Stat(pocScriptPath); err != nil {
		t.Skipf("cua host not installed at %s or %s: %v", packagedScriptPath, pocScriptPath, err)
	}

	// Change to the repository root so the cwd-relative fallback can discover
	// the host. Restore cwd afterwards.
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}

	manager := NewHostManager(HostManagerOptions{
		StartupTimeout: 30 * time.Second,
		HealthInterval: time.Minute,
	})
	defer manager.Close()

	script, workDir, err := manager.resolveHostPaths()
	if err != nil {
		t.Fatalf("resolve host paths: %v", err)
	}
	if script != expectedScriptPath {
		t.Fatalf("expected script %s, got %s", expectedScriptPath, script)
	}
	if workDir != expectedWorkingDir {
		t.Fatalf("expected working dir %s, got %s", expectedWorkingDir, workDir)
	}
}

func TestHostManagerDiscoversBundledNodeRuntime(t *testing.T) {
	_, workingDir := hostWorkspace(t)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(workingDir))))

	// The bundled Node runtime is installed by scripts/prepare-node-runtime.mjs.
	expected := filepath.Join(repoRoot, "apps", "desktop", "resources", "bin", nodeRuntimeTarget(), "node", "bin", "node")
	if _, err := os.Stat(expected); err != nil {
		t.Skipf("bundled Node runtime not installed at %s: %v", expected, err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}

	manager := NewHostManager(HostManagerOptions{})
	defer manager.Close()

	nodePath := manager.resolveNodePath()
	if nodePath == "" {
		t.Fatal("expected bundled Node runtime to be discovered")
	}
	if nodePath != expected {
		t.Fatalf("expected node path %s, got %s", expected, nodePath)
	}
}

func TestDevelopmentRuntimeLayoutCandidates(t *testing.T) {
	root := t.TempDir()
	executableDir := filepath.Join(root, "apps", "desktop", "resources", "bin", nodeRuntimeTarget())
	wantNode := filepath.Join(executableDir, "node", "bin", "node")
	foundNode := false
	for _, candidate := range nodeRuntimeBinaryCandidates(executableDir, "node") {
		if candidate == wantNode {
			foundNode = true
			break
		}
	}
	if !foundNode {
		t.Fatalf("development Node candidate missing: %s", wantNode)
	}
	wantHost := filepath.Join(root, "apps", "desktop", "resources", "cua-driver", "host.mjs")
	foundHost := false
	for _, candidate := range hostScriptCandidates(executableDir) {
		if filepath.Clean(candidate.script) == filepath.Clean(wantHost) {
			foundHost = true
			break
		}
	}
	if !foundHost {
		t.Fatalf("development Host candidate missing: %s", wantHost)
	}
}

func nodeRuntimeTarget() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goos == "windows" {
		goos = "win32"
	}
	return fmt.Sprintf("%s-%s", goos, goarch)
}
