package lifecyclehooks

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"awaken/src/internal/outputlimit"
)

const (
	defaultTimeoutMS      = 10000
	maxTimeoutMS          = 120000
	defaultMaxOutputChars = 12000
)

type Logger interface {
	Printf(format string, v ...interface{})
}

type Executor struct {
	config Config
	logger Logger
}

type Result struct {
	Event      Event
	ExitCode   int
	TimedOut   bool
	DurationMS int64
	Stdout     string
	Stderr     string
	Truncated  bool
	Error      string
}

func NewExecutor(config Config) *Executor {
	return NewExecutorWithLogger(config, log.Default())
}

func NewExecutorWithLogger(config Config, logger Logger) *Executor {
	return &Executor{
		config: config,
		logger: logger,
	}
}

func (e *Executor) HasHooks() bool {
	if e == nil {
		return false
	}
	return e.config.HasHooks()
}

func (e *Executor) Execute(ctx context.Context, event Event, metadata map[string]string) []Result {
	if e == nil || !isSupportedEvent(event) {
		return nil
	}

	hooks := e.config.CommandsFor(event)
	if len(hooks) == 0 {
		return nil
	}

	results := make([]Result, 0, len(hooks))
	for _, hook := range hooks {
		if !hook.isRunnable() {
			continue
		}
		result := e.executeOne(ctx, event, hook, metadata)
		results = append(results, result)
		e.record(result, hook)
	}
	return results
}

func (e *Executor) executeOne(ctx context.Context, event Event, hook CommandConfig, metadata map[string]string) Result {
	timeout := normalizeTimeoutMS(hook.TimeoutMS)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, strings.TrimSpace(hook.Command), hook.Args...)
	cmd.Env = hookEnvironment(event, metadata)
	if workdir := strings.TrimSpace(metadata["workspace_root"]); workdir != "" {
		cmd.Dir = workdir
	}

	stdout := outputlimit.NewCappedBuffer(defaultMaxOutputChars)
	stderr := outputlimit.NewCappedBuffer(defaultMaxOutputChars)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)
	timedOut := runCtx.Err() == context.DeadlineExceeded
	exitCode := 0
	errorText := ""
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		if timedOut {
			exitCode = -1
		}
		errorText = redactSecrets(err.Error())
	}

	stdoutResult := stdout.Result()
	stderrResult := stderr.Result()
	return Result{
		Event:      event,
		ExitCode:   exitCode,
		TimedOut:   timedOut,
		DurationMS: duration.Milliseconds(),
		Stdout:     stdoutResult.Content,
		Stderr:     stderrResult.Content,
		Truncated:  stdoutResult.Truncated || stderrResult.Truncated,
		Error:      errorText,
	}
}

func (e *Executor) record(result Result, hook CommandConfig) {
	if e.logger == nil {
		return
	}
	if result.Truncated {
		e.logger.Printf(
			"lifecycle hook output truncated event=%s command=%s stdout_chars=%d stderr_chars=%d duration_ms=%d",
			result.Event,
			redactedCommandSummary(hook),
			len([]rune(result.Stdout)),
			len([]rune(result.Stderr)),
			result.DurationMS,
		)
	}
	if result.ExitCode == 0 && !result.TimedOut && strings.TrimSpace(result.Error) == "" {
		return
	}
	e.logger.Printf(
		"lifecycle hook failed event=%s command=%s exit_code=%d timed_out=%t duration_ms=%d stdout_chars=%d stderr_chars=%d truncated=%t error=%s",
		result.Event,
		redactedCommandSummary(hook),
		result.ExitCode,
		result.TimedOut,
		result.DurationMS,
		len([]rune(result.Stdout)),
		len([]rune(result.Stderr)),
		result.Truncated,
		result.Error,
	)
}

func hookEnvironment(event Event, metadata map[string]string) []string {
	env := os.Environ()
	env = append(env, "AWAKEN_HOOK_EVENT="+string(event))

	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		envName := metadataEnvName(key)
		if envName == "" {
			continue
		}
		env = append(env, envName+"="+metadata[key])
	}
	return env
}

func metadataEnvName(key string) string {
	key = strings.TrimSpace(strings.ToUpper(key))
	if key == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range key {
		switch {
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return "AWAKEN_" + builder.String()
}

func normalizeTimeoutMS(value int) time.Duration {
	if value <= 0 {
		value = defaultTimeoutMS
	}
	if value > maxTimeoutMS {
		value = maxTimeoutMS
	}
	return time.Duration(value) * time.Millisecond
}

func redactedCommandSummary(hook CommandConfig) string {
	parts := make([]string, 0, len(hook.Args)+1)
	parts = append(parts, hook.Command)
	parts = append(parts, hook.Args...)
	return redactSecrets(fmt.Sprintf("%q", parts))
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|authorization)(\s*[=:]\s*)[^ \t,'"\]]+`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]+`),
}

func redactSecrets(value string) string {
	redacted := value
	redacted = secretPatterns[0].ReplaceAllString(redacted, `${1}${2}[REDACTED]`)
	redacted = secretPatterns[1].ReplaceAllString(redacted, `Bearer [REDACTED]`)
	redacted = secretPatterns[2].ReplaceAllString(redacted, `sk-[REDACTED]`)
	return redacted
}
