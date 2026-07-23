package acp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ConfigFile is the top-level persisted ACP runner configuration.
type ConfigFile struct {
	SchemaVersion int            `json:"schema_version"`
	Runners       []RunnerConfig `json:"runners"`
}

const (
	DefaultStartupTimeoutSeconds = 15
	DefaultTurnTimeoutSeconds    = 600
	DefaultMaxFrameBytes         = 8 * 1024 * 1024
	MaxStartupTimeoutSeconds     = 300
	MaxTurnTimeoutSeconds        = 3600
	MaxMaxFrameBytes             = 64 * 1024 * 1024
)

var runnerIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Normalize validates and caps a runner configuration.
func (cfg *RunnerConfig) Normalize() error {
	cfg.ID = strings.TrimSpace(cfg.ID)
	if !runnerIDRe.MatchString(cfg.ID) {
		return fmt.Errorf("invalid runner id %q: must match %s", cfg.ID, runnerIDRe.String())
	}
	cfg.Command = strings.TrimSpace(cfg.Command)
	if cfg.Command == "" {
		return fmt.Errorf("runner %s: command is required", cfg.ID)
	}
	if cfg.StartupTimeoutSeconds <= 0 {
		cfg.StartupTimeoutSeconds = DefaultStartupTimeoutSeconds
	}
	if cfg.StartupTimeoutSeconds > MaxStartupTimeoutSeconds {
		cfg.StartupTimeoutSeconds = MaxStartupTimeoutSeconds
	}
	if cfg.TurnTimeoutSeconds <= 0 {
		cfg.TurnTimeoutSeconds = DefaultTurnTimeoutSeconds
	}
	if cfg.TurnTimeoutSeconds > MaxTurnTimeoutSeconds {
		cfg.TurnTimeoutSeconds = MaxTurnTimeoutSeconds
	}
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = DefaultMaxFrameBytes
	}
	if cfg.MaxFrameBytes > MaxMaxFrameBytes {
		cfg.MaxFrameBytes = MaxMaxFrameBytes
	}
	mode := strings.ToLower(string(cfg.PermissionMode))
	switch mode {
	case string(PermissionModeDeny):
		cfg.PermissionMode = PermissionModeDeny
	default:
		cfg.PermissionMode = PermissionModeAsk
	}
	return nil
}

// Snapshot returns an immutable runner snapshot for a live session.
func (cfg RunnerConfig) Snapshot() RunnerSnapshot {
	env := make(map[string]string, len(cfg.Env))
	for k, v := range cfg.Env {
		env[k] = v
	}
	args := make([]string, len(cfg.Args))
	copy(args, cfg.Args)
	return RunnerSnapshot{
		ID:                    cfg.ID,
		Command:               cfg.Command,
		Args:                  args,
		Env:                   env,
		PermissionMode:        cfg.PermissionMode,
		StartupTimeoutSeconds: cfg.StartupTimeoutSeconds,
		TurnTimeoutSeconds:    cfg.TurnTimeoutSeconds,
		MaxFrameBytes:         cfg.MaxFrameBytes,
	}
}

// ConfigPath returns the user-scoped ACP config path under the Awaken home root.
func ConfigPath(root string) string {
	return filepath.Join(filepath.Clean(root), "config", "acp-agents.json")
}

// ProjectConfigPath returns the project-scoped ACP config path.
//
// Deprecated: ACP runner configuration is user-scoped only. This path is no
// longer loaded or merged by the runtime and remains only for migration or
// historical inspection.
func ProjectConfigPath(workspaceRoot string) string {
	return filepath.Join(filepath.Clean(workspaceRoot), ".awaken", "acp-agents.json")
}

// LoadConfigFile reads a config file. A missing file returns an empty config.
func LoadConfigFile(path string) (ConfigFile, error) {
	var cfg ConfigFile
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// SaveConfigFile atomically writes a config file. It uses a unique temporary
// file in the same directory, fsyncs before replacing, and handles Windows
// existing-file replacement.
func SaveConfigFile(path string, cfg ConfigFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := replaceFile(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
