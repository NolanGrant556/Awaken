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
	"strings"
	"time"

	"awaken/src/runtime"

	"github.com/google/uuid"
)

const (
	runtimeLockTTL       = 30 * time.Second
	manifestPollInterval = 200 * time.Millisecond
	lockOwnerID          = "awaken-acp-launcher"
)

// ErrDesktopBinaryNotFound is returned when the Desktop executable cannot be
// resolved next to the awaken-acp binary.
var ErrDesktopBinaryNotFound = errors.New("desktop binary not found")

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
	OwnerID      string
	OwnerMode    string
	// Close releases the lease and any spawned headless Core. It is safe to
	// call multiple times.
	Close func() error
}

// Launcher discovers or starts a managed awaken-core.
type Launcher struct {
	UserDataDir      string
	Strategy         StartupStrategy
	CoreBinaryPath   string
	IdentityParam    string // Optional explicit identity for dev/tests.
	SpawnDesktopFunc func(ctx context.Context, userDataDir string) error
	SpawnCoreFunc    func(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error)
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
	lock, err := runtime.AcquireRuntimeLock(l.UserDataDir, lockOwnerID, runtimeLockTTL)
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
	client := NewCoreClient(manifest.BaseURL, manifest.ControlToken)
	if err := client.VerifyIdentity(ctx, manifest); err != nil {
		return nil, fmt.Errorf("verify identity: %w", err)
	}
	ownerID := uuid.NewString()
	leaseID, _, err := client.CreateLease(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("create lease: %w", err)
	}
	return &LaunchResult{
		Identity:     manifest,
		ControlToken: manifest.ControlToken,
		BaseURL:      manifest.BaseURL,
		LeaseID:      leaseID,
		OwnerID:      ownerID,
		OwnerMode:    manifest.OwnerMode,
		Close: func() error {
			return client.ReleaseLease(context.Background(), leaseID)
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

// DefaultUserDataDir returns the packaged identity userData directory. On
// Windows it uses %APPDATA%\@awaken\desktop; on macOS
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
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", errors.New("APPDATA not set")
		}
		return filepath.Join(append([]string{appData}, segments...)...), nil
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
	desktopPath := os.Getenv("AWAKEN_DESKTOP_PATH")
	if desktopPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate awaken-acp binary: %w", err)
		}
		desktopPath, err = findDesktopBinary(exe)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(desktopPath); err != nil {
		return fmt.Errorf("%w: %s", ErrDesktopBinaryNotFound, desktopPath)
	}

	program := desktopPath
	var args []string
	if goruntime.GOOS == "darwin" && strings.HasSuffix(desktopPath, ".app") {
		program = "open"
		args = []string{"-a", desktopPath}
	}

	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = append(os.Environ(), "AWAKEN_USER_DATA_DIR="+userDataDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start desktop: %w", err)
	}
	return nil
}

func findDesktopBinary(acpExecutable string) (string, error) {
	dir := filepath.Dir(acpExecutable)
	var candidates []string
	for i := 0; i < 6; i++ {
		switch goruntime.GOOS {
		case "windows":
			candidates = append(candidates,
				filepath.Join(dir, "Awaken.exe"),
				filepath.Join(dir, "Awaken", "Awaken.exe"),
			)
		case "darwin":
			if strings.HasSuffix(strings.ToLower(dir), ".app") {
				candidates = append(candidates, dir)
			}
			candidates = append(candidates, filepath.Join(dir, "Awaken.app"))
		default:
			candidates = append(candidates,
				filepath.Join(dir, "awaken"),
				filepath.Join(dir, "Awaken", "awaken"),
			)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && (info.Mode().IsRegular() || info.IsDir()) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: searched from %s", ErrDesktopBinaryNotFound, filepath.Dir(acpExecutable))
}

func defaultSpawnCore(ctx context.Context, userDataDir, coreBinaryPath string) (*exec.Cmd, error) {
	if coreBinaryPath == "" {
		return nil, errors.New("core binary path is required for --startup=core")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve Awaken home: %w", err)
	}
	cmd := exec.CommandContext(ctx, coreBinaryPath, "--owner-mode=headless")
	cmd.Env = replaceProcessEnv(os.Environ(), map[string]string{
		"AWAKEN_USER_DATA_DIR":     userDataDir,
		"AWAKEN_HOME":              filepath.Join(homeDir, ".awaken"),
		"AWAKEN_DEFAULT_WORKSPACE": "",
	})
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func replaceProcessEnv(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		matched := false
		for replacementKey := range replacements {
			if strings.EqualFold(key, replacementKey) {
				matched = true
				break
			}
		}
		if !matched {
			result = append(result, entry)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

// processAlive checks whether a process with the given PID exists. On Windows it
// uses os.FindProcess; on Unix it sends signal 0.
func processAlive(pid int) bool {
	return isProcessAlive(pid)
}
