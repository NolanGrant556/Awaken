package shellhooks

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"
)

type CommandRunner interface {
	Run(ctx context.Context, request RunRequest) (RunOutput, error)
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, request RunRequest) (RunOutput, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	started := time.Now()
	cmd := exec.CommandContext(ctx, request.Argv[0], request.Argv[1:]...)
	cmd.Stdin = bytes.NewReader(request.Stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if request.CWD != "" {
		cmd.Dir = request.CWD
	}
	if len(request.Env) > 0 {
		cmd.Env = append(os.Environ(), request.Env...)
	}

	err := cmd.Run()
	output := RunOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(started),
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		output.ExitCode = -1
		output.TimedOut = true
		return output, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			output.ExitCode = exitErr.ExitCode()
			return output, nil
		}
		return output, err
	}
	return output, nil
}
