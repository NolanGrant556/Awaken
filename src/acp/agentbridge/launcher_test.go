package agentbridge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"awaken/src/runtime"
)

func writeTestManifest(t *testing.T, userDataDir string, id *runtime.RuntimeIdentity) {
	t.Helper()
	if err := runtime.WriteManifest(userDataDir, id); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func testManifest(t *testing.T, userDataDir, ownerMode string) *runtime.RuntimeIdentity {
	t.Helper()
	id, err := runtime.GenerateRuntimeIdentity(userDataDir, ownerMode, "http://127.0.0.1:18080")
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	id.PID = os.Getpid()
	return id
}

func TestLaunchAttachesToExistingManifest(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless")
	writeTestManifest(t, dir, id)

	var released bool
	releaseLeaseHook = func(baseURL, token, leaseID string) error {
		released = true
		return nil
	}
	defer func() { releaseLeaseHook = nil }()

	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail}
	res, err := launcher.Launch(context.Background())
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if res.Identity.InstanceID != id.InstanceID {
		t.Fatalf("instance mismatch: got %q, want %q", res.Identity.InstanceID, id.InstanceID)
	}
	if res.LeaseID != "lease_placeholder" {
		t.Fatalf("unexpected lease: %q", res.LeaseID)
	}
	if err := res.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !released {
		t.Fatal("lease was not released")
	}
}

func TestLaunchFailStrategyReturnsErrorWhenNoManifest(t *testing.T) {
	dir := t.TempDir()
	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail}
	if _, err := launcher.Launch(context.Background()); err == nil {
		t.Fatal("expected error for fail strategy with no manifest")
	}
}

func TestLaunchCoreSpawnStrategyWaitsForManifest(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless")

	spawned := false
	launcher := Launcher{
		UserDataDir:    dir,
		Strategy:       StartupCore,
		CoreBinaryPath: "fake-core-binary",
		SpawnCoreFunc: func(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error) {
			spawned = true
			if coreBinaryPath != "fake-core-binary" {
				t.Fatalf("unexpected core binary: %q", coreBinaryPath)
			}
			// Write manifest after a short delay to simulate Core startup.
			go func() {
				time.Sleep(50 * time.Millisecond)
				writeTestManifest(t, userDataDir, id)
			}()
			return nil, nil
		},
	}

	res, err := launcher.Launch(context.Background())
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if !spawned {
		t.Fatal("expected Core spawn function to be called")
	}
	if res.Identity.InstanceID != id.InstanceID {
		t.Fatalf("instance mismatch: got %q, want %q", res.Identity.InstanceID, id.InstanceID)
	}
	if err := res.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func startLongRunningProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	var cmd *exec.Cmd
	if goruntime.GOOS == "windows" {
		cmd = exec.Command("timeout", "/t", "30")
	} else {
		cmd = exec.Command("sleep", "30")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start long-running process: %v", err)
	}
	return cmd
}

func TestLaunchCoreSpawnStrategyCleansUpOnTimeout(t *testing.T) {
	origTimeout := manifestWaitTimeout
	manifestWaitTimeout = 300 * time.Millisecond
	defer func() { manifestWaitTimeout = origTimeout }()

	dir := t.TempDir()
	child := startLongRunningProcess(t)

	launcher := Launcher{
		UserDataDir:    dir,
		Strategy:       StartupCore,
		CoreBinaryPath: "fake-core-binary",
		SpawnCoreFunc: func(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error) {
			return child, nil
		},
	}

	releaseLeaseHook = func(baseURL, token, leaseID string) error { return nil }
	defer func() { releaseLeaseHook = nil }()

	start := time.Now()
	_, err := launcher.Launch(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
	// The launcher calls Kill and Wait on the spawned process. Wait for the
	// child to exit rather than relying on PID existence, which is unreliable
	// on Windows shortly after termination.
	waitCh := make(chan error, 1)
	go func() { waitCh <- child.Wait() }()
	select {
	case <-waitCh:
		// child exited
	case <-time.After(2 * time.Second):
		t.Fatal("spawned child process was not killed after timeout")
	}
}

func TestLaunchIdentityMismatchRejectsManifest(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless")
	writeTestManifest(t, dir, id)

	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail, IdentityParam: "other-identity"}
	if _, err := launcher.Launch(context.Background()); err == nil {
		t.Fatal("expected identity mismatch error")
	}
}

func TestLaunchCreateLeaseFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless")
	writeTestManifest(t, dir, id)

	createLeaseHook = func(ctx context.Context, baseURL, token, ownerID string) (*runtimeLease, error) {
		return nil, errors.New("lease denied")
	}
	defer func() { createLeaseHook = nil }()

	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail}
	if _, err := launcher.Launch(context.Background()); err == nil {
		t.Fatal("expected lease error")
	}
}

func TestLaunchDoubleCheckAfterLockAvoidsDuplicateSpawn(t *testing.T) {
	// This test verifies the locked double-check path by writing the manifest
	// inside SpawnCoreFunc. Because the fast path runs without the lock and sees
	// no manifest, Launch enters the locked section; the double-check then finds
	// the manifest written by the first SpawnCoreFunc call and skips a second
	// spawn.
	dir := t.TempDir()
	id := testManifest(t, dir, "headless")

	spawnCount := 0
	launcher := Launcher{
		UserDataDir:    dir,
		Strategy:       StartupCore,
		CoreBinaryPath: "fake-core-binary",
		SpawnCoreFunc: func(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error) {
			spawnCount++
			if spawnCount == 1 {
				writeTestManifest(t, userDataDir, id)
			}
			return nil, nil
		},
	}

	res, err := launcher.Launch(context.Background())
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if spawnCount != 1 {
		t.Fatalf("expected exactly one spawn attempt, got %d", spawnCount)
	}
	if res.Identity.InstanceID != id.InstanceID {
		t.Fatalf("instance mismatch: got %q, want %q", res.Identity.InstanceID, id.InstanceID)
	}
	if err := res.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
}

func TestDefaultUserDataDirPackaged(t *testing.T) {
	if goruntime.GOOS != "windows" {
		t.Skip("windows-only test")
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		t.Skip("LOCALAPPDATA not set")
	}
	dir, err := DefaultUserDataDir(false)
	if err != nil {
		t.Fatalf("default userData dir: %v", err)
	}
	expected := filepath.Join(localAppData, "@awaken", "desktop")
	if dir != expected {
		t.Fatalf("unexpected default dir: got %q, want %q", dir, expected)
	}
}

func TestComputeRuntimeIdentityIsStable(t *testing.T) {
	dir := t.TempDir()
	a := ComputeRuntimeIdentity(dir)
	b := ComputeRuntimeIdentity(dir)
	if a == "" {
		t.Fatal("identity is empty")
	}
	if a != b {
		t.Fatalf("identity not stable: %q vs %q", a, b)
	}
}
