package agentbridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
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

func testManifest(t *testing.T, userDataDir, ownerMode, baseURL string) *runtime.RuntimeIdentity {
	t.Helper()
	id, err := runtime.GenerateRuntimeIdentity(userDataDir, ownerMode, baseURL)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	id.PID = os.Getpid()
	return id
}

func testControlServer(t *testing.T, token, runtimeIdentity, instanceID, ownerMode string) (*httptest.Server, *bool, *bool) {
	t.Helper()
	created := false
	released := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/control/runtime/identity":
			writeJSON(w, http.StatusOK, map[string]any{
				"runtime_identity": runtimeIdentity,
				"instance_id":      instanceID,
				"owner_mode":       ownerMode,
				"base_url":         "http://" + r.Host,
			})
		case "POST /v1/control/leases":
			created = true
			writeJSON(w, http.StatusCreated, map[string]any{
				"lease_id":   "lease_test_123",
				"owner_kind": "bridge",
				"owner_id":   "owner-test",
				"expires_at": time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano),
				"ttl_ms":     30000,
			})
		case "DELETE /v1/control/leases/lease_test_123":
			released = true
			writeJSON(w, http.StatusOK, map[string]any{"status": "released"})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	return srv, &created, &released
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func TestLaunchAttachesToExistingManifest(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless", "http://127.0.0.1:18080")
	id.ControlToken = "test-token"
	srv, created, released := testControlServer(t, "test-token", id.RuntimeIdentity, id.InstanceID, "headless")
	defer srv.Close()
	id.BaseURL = srv.URL
	writeTestManifest(t, dir, id)

	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail}
	res, err := launcher.Launch(context.Background())
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if res.Identity.InstanceID != id.InstanceID {
		t.Fatalf("instance mismatch: got %q, want %q", res.Identity.InstanceID, id.InstanceID)
	}
	if res.LeaseID != "lease_test_123" {
		t.Fatalf("unexpected lease: %q", res.LeaseID)
	}
	if res.OwnerID == "" {
		t.Fatal("ownerID is empty")
	}
	if !*created {
		t.Fatal("lease create endpoint was not called")
	}
	if err := res.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !*released {
		t.Fatal("lease release endpoint was not called")
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
	id := testManifest(t, dir, "headless", "http://127.0.0.1:18080")
	id.ControlToken = "test-token"
	srv, created, _ := testControlServer(t, "test-token", id.RuntimeIdentity, id.InstanceID, "headless")
	defer srv.Close()
	id.BaseURL = srv.URL

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
	if !*created {
		t.Fatal("lease create endpoint was not called")
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
	id := testManifest(t, dir, "headless", "http://127.0.0.1:18080")
	writeTestManifest(t, dir, id)

	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail, IdentityParam: "other-identity"}
	if _, err := launcher.Launch(context.Background()); err == nil {
		t.Fatal("expected identity mismatch error")
	}
}

func TestLaunchCreateLeaseFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless", "http://127.0.0.1:18080")
	writeTestManifest(t, dir, id)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/control/leases" {
			http.Error(w, `{"error":"lease denied"}`, http.StatusBadRequest)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/v1/control/runtime/identity" {
			writeJSON(w, http.StatusOK, map[string]any{
				"runtime_identity": id.RuntimeIdentity,
				"instance_id":      id.InstanceID,
				"owner_mode":       id.OwnerMode,
				"base_url":         id.BaseURL,
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	id.BaseURL = srv.URL
	writeTestManifest(t, dir, id)

	launcher := Launcher{UserDataDir: dir, Strategy: StartupFail}
	if _, err := launcher.Launch(context.Background()); err == nil {
		t.Fatal("expected lease error")
	}
}

func TestLaunchDoubleCheckAfterLockAvoidsDuplicateSpawn(t *testing.T) {
	dir := t.TempDir()
	id := testManifest(t, dir, "headless", "http://127.0.0.1:18080")
	id.ControlToken = "test-token"
	srv, _, _ := testControlServer(t, "test-token", id.RuntimeIdentity, id.InstanceID, "headless")
	defer srv.Close()
	id.BaseURL = srv.URL

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
	appData := os.Getenv("APPDATA")
	if appData == "" {
		t.Skip("APPDATA not set")
	}
	dir, err := DefaultUserDataDir(false)
	if err != nil {
		t.Fatalf("default userData dir: %v", err)
	}
	expected := filepath.Join(appData, "@awaken", "desktop")
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

func TestDefaultSpawnDesktopReturnsTypedErrorWhenMissing(t *testing.T) {
	t.Setenv("AWAKEN_DESKTOP_PATH", "")
	// Exercise the default function directly; the real executable location is
	// unlikely to have a sibling Awaken directory in tests.
	tmp := t.TempDir()
	err := defaultSpawnDesktop(context.Background(), tmp)
	if !errors.Is(err, ErrDesktopBinaryNotFound) {
		t.Fatalf("expected ErrDesktopBinaryNotFound, got %v", err)
	}
}

func TestDefaultSpawnDesktopUsesEnvOverride(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nonexistent-desktop")
	t.Setenv("AWAKEN_DESKTOP_PATH", missing)
	err := defaultSpawnDesktop(context.Background(), dir)
	if !errors.Is(err, ErrDesktopBinaryNotFound) {
		t.Fatalf("expected ErrDesktopBinaryNotFound for missing env override, got %v", err)
	}
}

func TestReplaceProcessEnvPinsHeadlessAwakenRoots(t *testing.T) {
	got := replaceProcessEnv([]string{
		"PATH=C:\\tools",
		"AWAKEN_HOME=C:\\untrusted",
		"awaken_user_data_dir=C:\\old",
	}, map[string]string{
		"AWAKEN_HOME":          `C:\Users\tester\.awaken`,
		"AWAKEN_USER_DATA_DIR": `C:\Users\tester\AppData\Awaken`,
	})
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, `C:\untrusted`) || strings.Contains(joined, `C:\old`) {
		t.Fatalf("old Awaken roots must be removed: %v", got)
	}
	for _, expected := range []string{
		"PATH=C:\\tools",
		`AWAKEN_HOME=C:\Users\tester\.awaken`,
		`AWAKEN_USER_DATA_DIR=C:\Users\tester\AppData\Awaken`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %v", expected, got)
		}
	}
}

func TestFindDesktopBinaryFromPackagedResources(t *testing.T) {
	root := t.TempDir()
	acpDir := filepath.Join(root, "resources", "bin", goruntime.GOOS+"-test")
	if err := os.MkdirAll(acpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	acpName := "awaken-acp"
	desktopPath := ""
	switch goruntime.GOOS {
	case "windows":
		acpName += ".exe"
		desktopPath = filepath.Join(root, "Awaken.exe")
	case "darwin":
		appDir := filepath.Join(root, "Awaken.app")
		acpDir = filepath.Join(appDir, "Contents", "Resources", "bin", "darwin-test")
		if err := os.MkdirAll(acpDir, 0o755); err != nil {
			t.Fatal(err)
		}
		desktopPath = appDir
	default:
		desktopPath = filepath.Join(root, "awaken")
	}
	if goruntime.GOOS != "darwin" {
		if err := os.WriteFile(desktopPath, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := findDesktopBinary(filepath.Join(acpDir, acpName))
	if err != nil {
		t.Fatalf("find desktop binary: %v", err)
	}
	if got != desktopPath {
		t.Fatalf("desktop path mismatch: got %q, want %q", got, desktopPath)
	}
}
