package acp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Registry loads user-scoped ACP runner configurations.
type Registry struct {
	mu      sync.RWMutex
	fileMu  sync.Mutex
	userPath string
	runners  map[string]RunnerConfig
}

// NewRegistry creates a registry. userPath may be empty to use a process-local temp.
func NewRegistry(userPath string) *Registry {
	if userPath == "" {
		userPath = filepath.Join(os.TempDir(), fmt.Sprintf("awaken-acp-config-%d", os.Getpid()), "acp-agents.json")
	}
	return &Registry{
		userPath: userPath,
		runners:  make(map[string]RunnerConfig),
	}
}

// UserPath returns the user-scoped config path.
func (r *Registry) UserPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.userPath
}

// Load refreshes the in-memory registry from disk. Active sessions keep their snapshots.
func (r *Registry) Load() error {
	userCfg, err := LoadConfigFile(r.userPath)
	if err != nil {
		return fmt.Errorf("load user acp config: %w", err)
	}

	merged := make(map[string]RunnerConfig)
	for _, rc := range userCfg.Runners {
		if err := rc.Normalize(); err != nil {
			continue
		}
		merged[rc.ID] = rc
	}

	r.mu.Lock()
	r.runners = merged
	r.mu.Unlock()
	return nil
}

// Get returns a runner config by ID. The returned config is a copy.
func (r *Registry) Get(id string) (RunnerConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.runners[id]
	return cfg, ok
}

// Snapshot returns the immutable runner snapshot for an enabled runner.
func (r *Registry) Snapshot(id string) (RunnerSnapshot, bool) {
	cfg, ok := r.Get(id)
	if !ok || !cfg.Enabled {
		return RunnerSnapshot{}, false
	}
	return cfg.Snapshot(), true
}

// ListEnabled returns enabled runner snapshots.
func (r *Registry) ListEnabled() []RunnerSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RunnerSnapshot, 0, len(r.runners))
	for _, cfg := range r.runners {
		if !cfg.Enabled {
			continue
		}
		out = append(out, cfg.Snapshot())
	}
	return out
}

// All returns all runner configs (including disabled).
func (r *Registry) All() []RunnerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RunnerConfig, 0, len(r.runners))
	for _, cfg := range r.runners {
		out = append(out, cfg)
	}
	return out
}

// UpsertUser updates or inserts a runner in the user config file.
func (r *Registry) UpsertUser(cfg RunnerConfig) error {
	if err := cfg.Normalize(); err != nil {
		return err
	}
	r.fileMu.Lock()
	defer r.fileMu.Unlock()
	fileCfg, err := LoadConfigFile(r.userPath)
	if err != nil {
		return err
	}
	var found bool
	for i, rc := range fileCfg.Runners {
		if strings.EqualFold(rc.ID, cfg.ID) {
			fileCfg.Runners[i] = cfg
			found = true
			break
		}
	}
	if !found {
		fileCfg.Runners = append(fileCfg.Runners, cfg)
	}
	fileCfg.SchemaVersion = 1
	if err := SaveConfigFile(r.userPath, fileCfg); err != nil {
		return err
	}
	return r.Load()
}

// DeleteUser removes a runner from the user config file.
func (r *Registry) DeleteUser(id string) error {
	r.fileMu.Lock()
	defer r.fileMu.Unlock()
	fileCfg, err := LoadConfigFile(r.userPath)
	if err != nil {
		return err
	}
	filtered := make([]RunnerConfig, 0, len(fileCfg.Runners))
	for _, rc := range fileCfg.Runners {
		if rc.ID != id {
			filtered = append(filtered, rc)
		}
	}
	fileCfg.Runners = filtered
	if err := SaveConfigFile(r.userPath, fileCfg); err != nil {
		return err
	}
	return r.Load()
}

// Probe checks whether a runner command is executable, regardless of whether
// the runner is currently enabled.
func (r *Registry) Probe(id string) error {
	cfg, ok := r.Get(id)
	if !ok {
		return fmt.Errorf("%s: %s", ErrCodeRunnerNotConfigured, id)
	}
	if _, err := execLookPath(cfg.Command); err != nil {
		return fmt.Errorf("%s: %w", ErrCodeRunnerNotReady, err)
	}
	return nil
}

var execLookPath = func(name string) (string, error) {
	return execLookPathImpl(name)
}

func execLookPathImpl(name string) (string, error) {
	return exec.LookPath(name)
}
