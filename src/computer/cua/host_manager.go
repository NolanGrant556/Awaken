package cua

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// HostManager owns a single shared CUA Host process. It lazily starts the host
// on first use, monitors its health, and restarts it on failure. The manager is
// intended to live for the lifetime of the awaken-core process and be shared
// across all Computer runs.
type HostManager struct {
	opts        HostManagerOptions
	mu          sync.RWMutex
	client      HostClient
	unavailable bool
	lastErr     error
	lastAttempt time.Time
	closed      bool
}

// HostManagerOptions configures host discovery, startup, and health monitoring.
type HostManagerOptions struct {
	// HostScriptPath is the path to the Node host.mjs script. When empty, the
	// manager attempts to discover the script (env var, bundled resources, PoC
	// workspace relative to cwd).
	HostScriptPath string

	// WorkingDir is the directory from which to run the host. When empty, the
	// manager discovers it alongside the script.
	WorkingDir string

	// NodePath is the path to the node binary. Empty uses "node" from PATH.
	NodePath string

	// ExtraEnv is appended to the host environment.
	ExtraEnv []string

	// StartupTimeout bounds the initial health check.
	StartupTimeout time.Duration

	// HealthInterval is the interval between background health checks. Zero
	// defaults to 30 seconds.
	HealthInterval time.Duration

	// RestartCooldown is the minimum time between restart attempts. Zero defaults
	// to 5 seconds.
	RestartCooldown time.Duration

	// OnStateChange is called whenever the manager transitions between
	// available/unavailable. It must not block.
	OnStateChange func(available bool, err error)
}

// NewHostManager creates a manager for the shared CUA Host. The returned
// manager is idle until GetClient is called or Start is invoked explicitly.
func NewHostManager(opts HostManagerOptions) *HostManager {
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = 30 * time.Second
	}
	if opts.HealthInterval <= 0 {
		opts.HealthInterval = 30 * time.Second
	}
	if opts.RestartCooldown <= 0 {
		opts.RestartCooldown = 5 * time.Second
	}
	return &HostManager{opts: opts}
}

// Start eagerly connects to the host and begins health monitoring. It is safe
// to call multiple times and from multiple goroutines.
func (m *HostManager) Start(ctx context.Context) error {
	_, err := m.GetClient(ctx)
	return err
}

// Close shuts down the host process and stops health monitoring.
func (m *HostManager) Close() error {
	m.mu.Lock()
	m.closed = true
	client := m.client
	m.client = nil
	m.mu.Unlock()
	if client != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Close(closeCtx); err != nil {
			log.Printf("[cua-host-manager] close error: %v", err)
		}
	}
	return nil
}

// Available reports whether the manager currently has a healthy host.
func (m *HostManager) Available() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client != nil && !m.unavailable
}

// GetClient returns the current healthy host client, starting or restarting the
// host if necessary. If the host cannot be started, it returns an error and the
// manager marks itself unavailable until the restart cooldown passes.
func (m *HostManager) GetClient(ctx context.Context) (HostClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("host manager is closed")
	}
	if m.client != nil {
		return m.client, nil
	}
	if m.unavailable && time.Since(m.lastAttempt) < m.opts.RestartCooldown {
		return nil, m.lastErr
	}
	m.lastAttempt = time.Now()
	client, err := m.startLocked(ctx)
	if err != nil {
		m.unavailable = true
		m.lastErr = err
		m.notifyStateChange(false, err)
		return nil, err
	}
	m.client = client
	m.unavailable = false
	m.lastErr = nil
	m.notifyStateChange(true, nil)
	go m.healthLoop(client)
	return client, nil
}

func (m *HostManager) startLocked(ctx context.Context) (HostClient, error) {
	script, workDir, err := m.resolveHostPaths()
	if err != nil {
		return nil, err
	}
	nodePath := m.opts.NodePath
	if nodePath == "" {
		nodePath = m.resolveNodePath()
	}
	client, err := NewHostClient(HostOptions{
		NodePath:       nodePath,
		HostScriptPath: script,
		WorkingDir:     workDir,
		ExtraEnv:       m.opts.ExtraEnv,
		StartupTimeout: m.opts.StartupTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("start cua host: %w", err)
	}
	return client, nil
}

// resolveNodePath discovers a bundled Node runtime relative to the running
// executable or the current working directory. It falls back to an empty
// string, which lets NewHostClient use "node" from PATH.
func (m *HostManager) resolveNodePath() string {
	binaryName := "node"
	if os.PathSeparator == '\\' {
		binaryName = "node.exe"
	}

	// Discover relative to the running executable.
	if exePath, exeErr := os.Executable(); exeErr == nil {
		exePath, _ = filepath.EvalSymlinks(exePath)
		exeDir := filepath.Dir(exePath)
		candidates := nodeRuntimeBinaryCandidates(exeDir, binaryName)
		for _, c := range candidates {
			for _, target := range nodeRuntimeTargets() {
				p := strings.ReplaceAll(c, "<target>", target)
				if s, sErr := filepath.Abs(p); sErr == nil {
					if _, statErr := os.Stat(s); statErr == nil {
						return s
					}
				}
			}
		}
	}

	// Final fallback to the current working directory.
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		candidates := nodeRuntimeBinaryCandidates(cwd, binaryName)
		for _, c := range candidates {
			for _, target := range nodeRuntimeTargets() {
				p := strings.ReplaceAll(c, "<target>", target)
				if s, sErr := filepath.Abs(p); sErr == nil {
					if _, statErr := os.Stat(s); statErr == nil {
						return s
					}
				}
			}
		}
	}

	return ""
}

func nodeRuntimeBinaryCandidates(baseDir, binaryName string) []string {
	nodeBinDir := "bin"
	if binaryName == "node.exe" {
		nodeBinDir = ""
	}
	return []string{
		// Development runtime layout:
		// resources/bin/<target>/awaken-core + resources/bin/<target>/node.
		filepath.Join(baseDir, "node", nodeBinDir, binaryName),
		// macOS app bundle layout.
		filepath.Join(baseDir, "..", "Resources", "bin", "<target>", "node", nodeBinDir, binaryName),
		// Windows/Linux alongside binary.
		filepath.Join(baseDir, "resources", "bin", "<target>", "node", nodeBinDir, binaryName),
		// Development: binary under apps/desktop/resources/bin/<target>.
		filepath.Join(baseDir, "..", "..", "..", "bin", "<target>", "node", nodeBinDir, binaryName),
		// Development: binary at repository root, resources under apps/desktop.
		filepath.Join(baseDir, "apps", "desktop", "resources", "bin", "<target>", "node", nodeBinDir, binaryName),
	}
}

func nodeRuntimeTargets() []string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goos == "windows" {
		goos = "win32"
	}
	return []string{
		fmt.Sprintf("%s-%s", goos, goarch),
		// Universal fallback order for the current OS.
		fmt.Sprintf("%s-x64", goos),
	}
}

func (m *HostManager) notifyStateChange(available bool, err error) {
	if m.opts.OnStateChange != nil {
		m.opts.OnStateChange(available, err)
	}
}

func (m *HostManager) healthLoop(client HostClient) {
	ticker := time.NewTicker(m.opts.HealthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			hr, err := client.Health(ctx)
			cancel()
			if err != nil || !hr.Healthy {
				log.Printf("[cua-host-manager] host unhealthy, scheduling restart: err=%v healthy=%v", err, hr.Healthy)
				m.handleFailure(client)
				return
			}
		case <-m.closeSignal():
			return
		}
	}
}

func (m *HostManager) closeSignal() <-chan struct{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return nil
}

func (m *HostManager) handleFailure(oldClient HostClient) {
	m.mu.Lock()
	if m.client != oldClient {
		m.mu.Unlock()
		return
	}
	m.client = nil
	m.unavailable = true
	m.lastErr = fmt.Errorf("host process failed health check")
	m.mu.Unlock()
	m.notifyStateChange(false, m.lastErr)

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = oldClient.Close(closeCtx)
}

// ResolvedHostPaths returns the discovered host script and working directory
// for diagnostics and settings. It does not start the host process.
func (m *HostManager) ResolvedHostPaths() (scriptPath, workingDir string, err error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolveHostPaths()
}

// ResolvedNodePath returns the discovered Node runtime binary for diagnostics
// and settings. It does not start the host process.
func (m *HostManager) ResolvedNodePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolveNodePath()
}

func (m *HostManager) resolveHostPaths() (scriptPath, workingDir string, err error) {
	scriptPath = strings.TrimSpace(m.opts.HostScriptPath)
	workingDir = strings.TrimSpace(m.opts.WorkingDir)

	// Explicit configuration wins.
	if scriptPath != "" {
		if workingDir == "" {
			workingDir = filepath.Dir(scriptPath)
		}
		return scriptPath, workingDir, nil
	}

	// Environment overrides.
	scriptPath = strings.TrimSpace(os.Getenv("AWAKEN_CUA_HOST_SCRIPT"))
	workingDir = strings.TrimSpace(os.Getenv("AWAKEN_CUA_HOST_WORKDIR"))
	if scriptPath != "" {
		if workingDir == "" {
			workingDir = filepath.Dir(scriptPath)
		}
		return scriptPath, workingDir, nil
	}

	// Discover a bundled host relative to the running executable.
	if exePath, exeErr := os.Executable(); exeErr == nil {
		exePath, _ = filepath.EvalSymlinks(exePath)
		exeDir := filepath.Dir(exePath)
		candidates := hostScriptCandidates(exeDir)
		for _, c := range candidates {
			if s, sErr := filepath.Abs(c.script); sErr == nil {
				if _, statErr := os.Stat(s); statErr == nil {
					if w, wErr := filepath.Abs(c.workDir); wErr == nil {
						return s, w, nil
					}
				}
			}
		}
	}

	// Final fallback to the current working directory. This supports development
	// launches from the repository root whether the host is in the packaged
	// resources path or the original PoC workspace.
	cwd, cwdErr := os.Getwd()
	if cwdErr == nil {
		candidates := []struct {
			script  string
			workDir string
		}{
			{filepath.Join(cwd, "apps", "desktop", "resources", "cua-driver", "host.mjs"), filepath.Join(cwd, "apps", "desktop", "resources", "cua-driver")},
			{filepath.Join(cwd, "scripts", "poc", "cua-driver", "host.mjs"), filepath.Join(cwd, "scripts", "poc", "cua-driver")},
		}
		for _, c := range candidates {
			if s, sErr := filepath.Abs(c.script); sErr == nil {
				if _, statErr := os.Stat(s); statErr == nil {
					if w, wErr := filepath.Abs(c.workDir); wErr == nil {
						return s, w, nil
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("cua host script not found; set AWAKEN_CUA_HOST_SCRIPT or install the host sidecar")
}

func hostScriptCandidates(baseDir string) []struct {
	script  string
	workDir string
} {
	return []struct {
		script  string
		workDir string
	}{
		// Development runtime layout:
		// resources/bin/<target>/awaken-core + resources/cua-driver.
		{filepath.Join(baseDir, "..", "..", "cua-driver", "host.mjs"), filepath.Join(baseDir, "..", "..", "cua-driver")},
		// macOS app bundle layout.
		{filepath.Join(baseDir, "..", "Resources", "cua-driver", "host.mjs"), filepath.Join(baseDir, "..", "Resources", "cua-driver")},
		// Windows/Linux alongside binary.
		{filepath.Join(baseDir, "cua-driver", "host.mjs"), filepath.Join(baseDir, "cua-driver")},
		// Legacy development layout.
		{filepath.Join(baseDir, "..", "..", "..", "cua-driver", "host.mjs"), filepath.Join(baseDir, "..", "..", "..", "cua-driver")},
		// Development binary launched from repository root.
		{filepath.Join(baseDir, "apps", "desktop", "resources", "cua-driver", "host.mjs"), filepath.Join(baseDir, "apps", "desktop", "resources", "cua-driver")},
	}
}
