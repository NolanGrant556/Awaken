package shellhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	DefaultTimeout = 60 * time.Second
	MaxTimeout     = 5 * time.Minute
)

type Options struct {
	Runner          CommandRunner
	ApprovalStore   ApprovalStore
	RequireApproval bool
	Redactor        *Redactor
	DefaultTimeout  time.Duration
	MaxTimeout      time.Duration
	CWD             string
}

type Executor struct {
	runner          CommandRunner
	approvalStore   ApprovalStore
	requireApproval bool
	redactor        *Redactor
	defaultTimeout  time.Duration
	maxTimeout      time.Duration
	cwd             string
}

func NewExecutor(options Options) *Executor {
	runner := options.Runner
	if runner == nil {
		runner = OSRunner{}
	}
	redactor := options.Redactor
	if redactor == nil {
		redactor = DefaultRedactor()
	}
	defaultTimeout := options.DefaultTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = DefaultTimeout
	}
	maxTimeout := options.MaxTimeout
	if maxTimeout <= 0 {
		maxTimeout = MaxTimeout
	}
	return &Executor{
		runner:          runner,
		approvalStore:   options.ApprovalStore,
		requireApproval: options.RequireApproval,
		redactor:        redactor,
		defaultTimeout:  defaultTimeout,
		maxTimeout:      maxTimeout,
		cwd:             options.CWD,
	}
}

func (e *Executor) Run(ctx context.Context, spec Spec, input Input) Result {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := ValidateSpec(spec); err != nil {
		return e.errorResult(StatusError, err)
	}
	if !MatchesInput(spec, input) {
		return Result{Status: StatusSkipped}
	}

	approvalState := ApprovalApproved
	if e.requireApproval {
		approvalState = ApprovalUnknown
		if e.approvalStore != nil {
			approvalState = e.approvalStore.State(spec)
		}
		if approvalState != ApprovalApproved {
			return Result{
				Status:        StatusSkipped,
				ApprovalState: approvalState,
				Error:         e.redactor.Redact("hook command is not approved"),
			}
		}
	}

	argv, err := ResolveArgv(spec)
	if err != nil {
		result := e.errorResult(StatusError, err)
		result.ApprovalState = approvalState
		return result
	}

	input = e.prepareInput(spec, input)
	stdin, err := json.Marshal(input)
	if err != nil {
		result := e.errorResult(StatusError, fmt.Errorf("marshal hook input: %w", err))
		result.ApprovalState = approvalState
		return result
	}

	timeout := e.normalizeTimeout(spec.Timeout)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, runErr := e.runner.Run(runCtx, RunRequest{
		Argv:  argv,
		Stdin: append(stdin, '\n'),
		CWD:   firstNonEmpty(spec.CWD, input.CWD),
		Env:   append([]string(nil), spec.Env...),
	})

	parsed := ParseOutput(output.Stdout).Redact(e.redactor)
	result := Result{
		Status:        StatusSuccess,
		ApprovalState: approvalState,
		ExitCode:      output.ExitCode,
		TimedOut:      output.TimedOut || errors.Is(runCtx.Err(), context.DeadlineExceeded),
		Duration:      output.Duration,
		Stdout:        e.redactor.Redact(output.Stdout),
		Stderr:        e.redactor.Redact(output.Stderr),
		InertOutput:   parsed.PlainText,
		Decision:      parsed.Decision,
		Parsed:        parsed,
	}

	switch {
	case result.TimedOut || errors.Is(runErr, context.DeadlineExceeded):
		result.Status = StatusTimeout
		result.TimedOut = true
		if runErr != nil {
			result.Error = e.redactor.Redact(runErr.Error())
		}
	case runErr != nil:
		result.Status = StatusError
		result.Error = e.redactor.Redact(runErr.Error())
	case parsed.Decision.Type == DecisionBlock:
		result.Status = StatusBlocked
	case output.ExitCode != 0:
		result.Status = StatusError
		result.Error = e.redactor.Redact(fmt.Sprintf("hook exited with status %d", output.ExitCode))
	default:
		result.Status = StatusSuccess
	}

	return result
}

func (e *Executor) prepareInput(spec Spec, input Input) Input {
	if input.HookEventName == "" {
		input.HookEventName = spec.Event
	}
	if input.CWD == "" {
		if e.cwd != "" {
			input.CWD = e.cwd
		} else if cwd, err := os.Getwd(); err == nil {
			input.CWD = cwd
		}
	}
	return input
}

func (e *Executor) normalizeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		timeout = e.defaultTimeout
	}
	if timeout > e.maxTimeout {
		timeout = e.maxTimeout
	}
	return timeout
}

func (e *Executor) errorResult(status Status, err error) Result {
	return Result{Status: status, Error: e.redactor.Redact(err.Error())}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
