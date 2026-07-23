package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ProcessHandle abstracts an ACP runner child process.
type ProcessHandle interface {
	PID() int
	Wait() error
	Kill() error
	KillTree() error
}

// StdioPipes holds the stdin/stdout/stderr of a spawned process.
type StdioPipes struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// SpawnConfig holds parameters for spawning an ACP runner.
type SpawnConfig struct {
	Command string
	Args    []string
	Env     map[string]string
	Dir     string
}

// Spawn starts a local stdio process for an ACP runner.
func Spawn(ctx context.Context, cfg SpawnConfig) (*exec.Cmd, StdioPipes, error) {
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	cmd.Env = MergeProcessEnv(os.Environ(), cfg.Env)

	ConfigureProcessTree(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, StdioPipes{}, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, StdioPipes{}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, StdioPipes{}, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, StdioPipes{}, fmt.Errorf("start %s: %w", cfg.Command, err)
	}
	return cmd, StdioPipes{Stdin: stdin, Stdout: stdout, Stderr: stderr}, nil
}

// MergeProcessEnv merges explicit overrides on top of a base environment. On
// Windows keys are matched case-insensitively so that "PATH" overrides "Path".
func MergeProcessEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	merged := make(map[string]string, len(base))
	for _, e := range base {
		if i := indexEnvSep(e); i >= 0 {
			key := e[:i]
			if existing, ok := merged[key]; !ok || shouldOverrideKey(key, existing) {
				merged[key] = e[i+1:]
			}
		}
	}
	for k, v := range overrides {
		if envKeyIsCaseInsensitive() {
			for baseKey := range merged {
				if strings.EqualFold(baseKey, k) {
					delete(merged, baseKey)
					break
				}
			}
		}
		merged[k] = v
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

func indexEnvSep(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return i
		}
	}
	return -1
}

var osEnviron = func() []string {
	return os.Environ()
}
