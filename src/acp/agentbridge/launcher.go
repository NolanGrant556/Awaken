// Package agentbridge implements the awaken-acp stdio bridge and Core launcher.
package agentbridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"time"

	"awaken/src/runtime"
)

const (
	runtimeLockTTL       = 30 * time.Second
	manifestPollInterval = 200 * time.Millisecond
	leaseOwnerID         = "awaken-acp-bridge"
)

// manifestWaitTimeout is the budget for waiting for a spawned Core to publish
// its manifest. Tests override it to avoid long sleeps.
var manifestWaitTimeout = 30 * time.Second

// StartupStrategy controls how the bridge behaves when no ready Core exists.
type StartupStrategy string

const (
	StartupDesktop StartupStrategy = "desktop"
	StartupCore    StartupStrategy = "core"
	StartupFail    StartupStrategy = "fail"
)

// LaunchResult is the outcome of a successful launcher attach.
type LaunchResult struct {
	Identity     *runtime.RuntimeIdentity
	ControlToken string
	BaseURL      string
	LeaseID      string
	OwnerMode    string
	// Close releases the lease and any spawned headless Core. It is safe to
	// call multiple times.
	Close func() error
}

// Launcher discovers or starts a managed awaken-core.
type Launcher struct {
	UserDataDir       string
	Strategy          StartupStrategy
	CoreBinaryPath    string
	IdentityParam     string // Optional explicit identity for dev/tests.
	SpawnDesktopFunc  func(ctx context.Context, userDataDir string) error
	SpawnCoreFunc     func(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error)
}

// Launch follows the documented state machine: validate manifest, attach if
// ready, otherwise acquire lock, spawn per strategy, wait for manifest, verify
// identity, create lease.
func (l *Launcher) Launch(ctx context.Context) (*LaunchResult, error) {
	if l.UserDataDir == "" {
		return nil, errors.New("userDataDir is required")
	}
	if err := os.MkdirAll(l.UserDataDir, 0o700); err != nil {
		return nil, fmt.Errorf("ensure userData dir: %w", err)
	}

	expectedRuntimeIdentity := l.expectedRuntimeIdentity()

	// Fast path: a valid manifest already exists.
	if manifest, err := l.readAndValidateManifest(expectedRuntimeIdentity); err == nil {
		return l.attach(ctx, manifest)
	}

	// No ready Core. Decide what to do based on strategy.
	switch l.Strategy {
	case StartupFail:
		return nil, errors.New("no ready Core and startup strategy is fail")
	case StartupDesktop, StartupCore:
		// proceed below
	default:
		return nil, fmt.Errorf("unknown startup strategy %q", l.Strategy)
	}

	// Acquire the runtime lock before spawning.
	lock, err := runtime.AcquireRuntimeLock(l.UserDataDir, leaseOwnerID, runtimeLockTTL)
	if err != nil {
		return nil, fmt.Errorf("acquire runtime lock: %w", err)
	}
	defer lock.Release()

	// Double-check manifest after acquiring lock.
	if manifest, err := l.readAndValidateManifest(expectedRuntimeIdentity); err == nil {
		return l.attach(ctx, manifest)
	}

	var spawned *exec.Cmd
	switch l.Strategy {
	case StartupDesktop:
		if err := l.spawnDesktop(ctx); err != nil {
			return nil, fmt.Errorf("spawn desktop: %w", err)
		}
	case StartupCore:
		spawned, err = l.spawnCore(ctx)
		if err != nil {
			return nil, fmt.Errorf("spawn core: %w", err)
		}
	}

	manifest, err := l.waitForManifest(ctx, expectedRuntimeIdentity)
	if err != nil {
		if spawned != nil {
			_ = spawned.Process.Kill()
		}
		return nil, fmt.Errorf("wait for manifest: %w", err)
	}

	res, err := l.attach(ctx, manifest)
	if err != nil {
		if spawned != nil {
			_ = spawned.Process.Kill()
		}
		return nil, err
	}

	// Wrap close to also terminate a headless Core we spawned.
	originalClose := res.Close
	res.Close = func() error {
		_ = originalClose()
		if spawned != nil && spawned.Process != nil {
			_ = spawned.Process.Kill()
			_ = spawned.Wait()
		}
		return nil
	}
	return res, nil
}

func (l *Launcher) readAndValidateManifest(expectedRuntimeIdentity string) (*runtime.RuntimeIdentity, error) {
	manifest, err := runtime.ReadManifest(l.UserDataDir)
	if err != nil {
		return nil, err
	}
	if err := runtime.ValidateManifest(manifest, expectedRuntimeIdentity); err != nil {
		return nil, err
	}
	return manifest, nil
}

func (l *Launcher) waitForManifest(ctx context.Context, expectedRuntimeIdentity string) (*runtime.RuntimeIdentity, error) {
	ctx, cancel := context.WithTimeout(ctx, manifestWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(manifestPollInterval)
	defer ticker.Stop()
	for {
		if manifest, err := l.readAndValidateManifest(expectedRuntimeIdentity); err == nil {
			return manifest, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (l *Launcher) attach(ctx context.Context, manifest *runtime.RuntimeIdentity) (*LaunchResult, error) {
	lease, err := createLease(ctx, manifest.BaseURL, manifest.ControlToken, leaseOwnerID)
	if err != nil {
		return nil, fmt.Errorf("create lease: %w", err)
	}
	return &LaunchResult{
		Identity:     manifest,
		ControlToken: manifest.ControlToken,
		BaseURL:      manifest.BaseURL,
		LeaseID:      lease.LeaseID,
		OwnerMode:    manifest.OwnerMode,
		Close: func() error {
			return releaseLease(manifest.BaseURL, manifest.ControlToken, lease.LeaseID)
		},
	}, nil
}

func (l *Launcher) expectedRuntimeIdentity() string {
	if l.IdentityParam != "" {
		return l.IdentityParam
	}
	return runtime.ComputeRuntimeIdentity(l.UserDataDir)
}

func (l *Launcher) spawnDesktop(ctx context.Context) error {
	if l.SpawnDesktopFunc != nil {
		return l.SpawnDesktopFunc(ctx, l.UserDataDir)
	}
	return defaultSpawnDesktop(ctx, l.UserDataDir)
}

func (l *Launcher) spawnCore(ctx context.Context) (*exec.Cmd, error) {
	if l.SpawnCoreFunc != nil {
		return l.SpawnCoreFunc(ctx, l.UserDataDir, l.CoreBinaryPath)
	}
	return defaultSpawnCore(ctx, l.UserDataDir, l.CoreBinaryPath)
}

// ComputeRuntimeIdentity exposes the canonical identity digest for a userData
// directory. It is used by the launcher and tests.
func ComputeRuntimeIdentity(userDataDir string) string {
	return runtime.ComputeRuntimeIdentity(userDataDir)
}

var (
	// createLeaseHook and releaseLeaseHook are swappable for tests. Production
	// code leaves them nil; nil means use the default placeholder behavior.
	createLeaseHook  func(ctx context.Context, baseURL, token, ownerID string) (*runtimeLease, error)
	releaseLeaseHook func(baseURL, token, leaseID string) error
)

func createLease(ctx context.Context, baseURL, token, ownerID string) (*runtimeLease, error) {
	if createLeaseHook != nil {
		return createLeaseHook(ctx, baseURL, token, ownerID)
	}
	// Placeholder: real HTTP call is implemented once the control client is
	// wired in Stage 5. For Stage 4 tests use a fake transport.
	_ = ctx
	_ = baseURL
	_ = token
	return &runtimeLease{LeaseID: "lease_placeholder", OwnerID: ownerID}, nil
}

func releaseLease(baseURL, token, leaseID string) error {
	if releaseLeaseHook != nil {
		return releaseLeaseHook(baseURL, token, leaseID)
	}
	_ = baseURL
	_ = token
	_ = leaseID
	return nil
}

type runtimeLease struct {
	LeaseID string
	OwnerID string
}

// DefaultUserDataDir returns the packaged identity userData directory. On
// Windows it uses %LOCALAPPDATA%\@awaken\desktop; on macOS
// ~/Library/Application Support/@awaken/desktop; on Linux
// ~/.config/@awaken/desktop. The dev variant uses @awaken-dev.
func DefaultUserDataDir(dev bool) (string, error) {
	var segments []string
	if dev {
		segments = []string{"@awaken-dev", "desktop"}
	} else {
		segments = []string{"@awaken", "desktop"}
	}
	switch goruntime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", errors.New("LOCALAPPDATA not set")
		}
		return filepath.Join(append([]string{localAppData}, segments...)...), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(append([]string{home, "Library", "Application Support"}, segments...)...), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(append([]string{home, ".config"}, segments...)...), nil
	}
}

func defaultSpawnDesktop(ctx context.Context, userDataDir string) error {
	_ = ctx
	return fmt.Errorf("desktop spawn not implemented on %s", goruntime.GOOS)
}

func defaultSpawnCore(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error) {
	if coreBinaryPath == "" {
		return nil, errors.New("core binary path is required for --startup=core")
	}
	cmd := exec.CommandContext(ctx, coreBinaryPath)
	cmd.Env = append(os.Environ(),
		"AWAKEN_USER_DATA_DIR="+userDataDir,
		"AWAKEN_OWNER_MODE=headless",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// processAlive checks whether a process with the given PID exists. On Windows it
// uses os.FindProcess; on Unix it sends signal 0.
func processAlive(pid int) bool {
	return isProcessAlive(pid)
}
