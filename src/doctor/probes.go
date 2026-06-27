package doctor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// EnvProbe isolates process environment and user-path lookups.
type EnvProbe interface {
	LookupEnv(key string) (string, bool)
	HomeDir() (string, error)
	WorkingDir() (string, error)
}

// FileProbe isolates file-system reads used by diagnostics.
type FileProbe interface {
	Exists(path string) (bool, error)
	ReadFile(path string) (string, error)
}

// CommandProbe isolates external commands used by diagnostics.
type CommandProbe interface {
	Run(ctx context.Context, spec CommandSpec) CommandResult
}

// CommandSpec is a shell-free command invocation.
type CommandSpec struct {
	Name    string
	Args    []string
	Dir     string
	Timeout time.Duration
}

func (s CommandSpec) Display() string {
	if len(s.Args) == 0 {
		return s.Name
	}
	return s.Name + " " + strings.Join(s.Args, " ")
}

// CommandResult captures command output after the process exits.
type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
}

func (r CommandResult) OK() bool {
	return r.Err == nil && r.ExitCode == 0
}

type osEnvProbe struct{}

func (osEnvProbe) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

func (osEnvProbe) HomeDir() (string, error) {
	return os.UserHomeDir()
}

func (osEnvProbe) WorkingDir() (string, error) {
	return os.Getwd()
}

type osFileProbe struct{}

func (osFileProbe) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (osFileProbe) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// LocalCommandProbe runs commands without a shell.
type LocalCommandProbe struct {
	Timeout time.Duration
}

func (p LocalCommandProbe) Run(ctx context.Context, spec CommandSpec) CommandResult {
	timeout := spec.Timeout
	if timeout == 0 {
		timeout = p.Timeout
	}
	if timeout == 0 {
		timeout = 2 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, spec.Name, spec.Args...)
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if cmdCtx.Err() != nil {
		err = cmdCtx.Err()
		exitCode = -1
	}

	return CommandResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Err:      err,
	}
}
