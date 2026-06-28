package evalharness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMissingExecutor = errors.New("evalharness runner requires a step executor")
	ErrInvalidScenario = errors.New("invalid eval scenario")
)

type StepExecutor interface {
	ExecuteStep(ctx context.Context, scenario Scenario, step Step) (StepExecution, error)
}

type StepExecutorFunc func(ctx context.Context, scenario Scenario, step Step) (StepExecution, error)

func (f StepExecutorFunc) ExecuteStep(ctx context.Context, scenario Scenario, step Step) (StepExecution, error) {
	return f(ctx, scenario, step)
}

type Runner struct {
	executor             StepExecutor
	store                EvidenceStore
	now                  func() time.Time
	providerAvailability ProviderAvailability
}

type RunnerOption func(*Runner)

func WithClock(now func() time.Time) RunnerOption {
	return func(r *Runner) {
		if now != nil {
			r.now = now
		}
	}
}

func WithProviderAvailability(availability ProviderAvailability) RunnerOption {
	return func(r *Runner) {
		r.providerAvailability = availability
	}
}

func NewRunner(executor StepExecutor, store EvidenceStore, opts ...RunnerOption) (*Runner, error) {
	if executor == nil {
		return nil, ErrMissingExecutor
	}
	if store == nil {
		store = NewMemoryEvidenceStore()
	}

	runner := &Runner{
		executor: executor,
		store:    store,
		now:      func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(runner)
	}
	return runner, nil
}

func (r *Runner) Run(ctx context.Context, scenario Scenario) (Result, error) {
	if err := ValidateScenario(scenario); err != nil {
		return Result{}, err
	}

	started := r.timestamp()
	result := Result{
		ScenarioID:   scenario.ID,
		Name:         scenario.Name,
		Status:       StatusPassed,
		StartedAt:    started,
		Requirements: scenario.Requirements,
		Steps:        make([]StepResult, 0, len(scenario.Steps)),
	}

	if summary, blocked := r.providerBlockSummary(scenario.Requirements); blocked {
		if err := r.recordBlockedPreflight(ctx, &result, summary); err != nil {
			return result, err
		}
		r.finishResult(&result)
		return result, nil
	}

	for _, step := range scenario.Steps {
		stepResult, evidence, err := r.runStep(ctx, scenario, step)
		if err != nil {
			return result, err
		}
		result.Steps = append(result.Steps, stepResult)
		result.Evidence = append(result.Evidence, evidence)
		if stepResult.Status == StatusFailed || stepResult.Status == StatusBlocked {
			break
		}
	}

	r.finishResult(&result)
	return result, nil
}

func ValidateScenario(scenario Scenario) error {
	if strings.TrimSpace(scenario.ID) == "" {
		return fmt.Errorf("%w: scenario id is required", ErrInvalidScenario)
	}
	if len(scenario.Steps) == 0 {
		return fmt.Errorf("%w: scenario %q has no steps", ErrInvalidScenario, scenario.ID)
	}

	seen := make(map[string]struct{}, len(scenario.Steps))
	for _, step := range scenario.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("%w: scenario %q has a step without an id", ErrInvalidScenario, scenario.ID)
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("%w: scenario %q has duplicate step id %q", ErrInvalidScenario, scenario.ID, step.ID)
		}
		seen[step.ID] = struct{}{}
		if strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("%w: scenario %q step %q command is required", ErrInvalidScenario, scenario.ID, step.ID)
		}
	}
	return nil
}

func (r *Runner) runStep(ctx context.Context, scenario Scenario, step Step) (StepResult, Evidence, error) {
	if summary, blocked := r.providerBlockSummary(step.Requirements); blocked {
		return r.blockedStep(ctx, scenario, step, summary)
	}

	stepCtx := ctx
	var cancel context.CancelFunc
	if step.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	started := r.timestamp()
	execution, execErr := r.executor.ExecuteStep(stepCtx, scenario, step)
	finished := r.timestamp()
	status := execution.Status
	if execErr != nil {
		status = StatusFailed
	}
	if status == "" {
		status = StatusPassed
	}

	summary := firstNonEmpty(execution.Summary, string(status))
	errText := ""
	if execErr != nil {
		errText = RedactSecrets(execErr.Error())
		summary = firstNonEmpty(execution.Summary, errText)
	}
	summary = RedactSecrets(summary)

	evidence := Evidence{
		ScenarioID: scenario.ID,
		StepID:     step.ID,
		Command:    RedactSecrets(step.Command),
		Status:     status,
		StartedAt:  started,
		FinishedAt: finished,
		Duration:   nonNegativeDuration(finished.Sub(started)),
		Summary:    summary,
		Text:       RedactSecrets(execution.Text),
		Metadata:   redactMetadata(execution.Metadata),
	}
	stored, err := r.store.AddEvidence(ctx, evidence)
	if err != nil {
		return StepResult{}, Evidence{}, err
	}

	return StepResult{
		StepID:      step.ID,
		Name:        step.Name,
		Status:      status,
		StartedAt:   started,
		FinishedAt:  finished,
		Duration:    evidence.Duration,
		Summary:     summary,
		Error:       errText,
		EvidenceIDs: []string{stored.ID},
	}, stored, nil
}

func (r *Runner) blockedStep(ctx context.Context, scenario Scenario, step Step, summary string) (StepResult, Evidence, error) {
	started := r.timestamp()
	finished := r.timestamp()
	summary = RedactSecrets(summary)
	evidence := Evidence{
		ScenarioID: scenario.ID,
		StepID:     step.ID,
		Command:    RedactSecrets(step.Command),
		Status:     StatusBlocked,
		StartedAt:  started,
		FinishedAt: finished,
		Duration:   nonNegativeDuration(finished.Sub(started)),
		Summary:    summary,
	}
	stored, err := r.store.AddEvidence(ctx, evidence)
	if err != nil {
		return StepResult{}, Evidence{}, err
	}
	return StepResult{
		StepID:      step.ID,
		Name:        step.Name,
		Status:      StatusBlocked,
		StartedAt:   started,
		FinishedAt:  finished,
		Duration:    evidence.Duration,
		Summary:     summary,
		EvidenceIDs: []string{stored.ID},
	}, stored, nil
}

func (r *Runner) recordBlockedPreflight(ctx context.Context, result *Result, summary string) error {
	started := r.timestamp()
	summary = RedactSecrets(summary)
	evidence := Evidence{
		ScenarioID: result.ScenarioID,
		Command:    "real-provider preflight",
		Status:     StatusBlocked,
		StartedAt:  started,
		FinishedAt: started,
		Summary:    summary,
	}
	stored, err := r.store.AddEvidence(ctx, evidence)
	if err != nil {
		return err
	}
	result.Status = StatusBlocked
	result.Summary = summary
	result.Evidence = append(result.Evidence, stored)
	return nil
}

func (r *Runner) finishResult(result *Result) {
	result.FinishedAt = r.timestamp()
	result.Duration = nonNegativeDuration(result.FinishedAt.Sub(result.StartedAt))
	if result.Status != StatusBlocked {
		result.Status = resultStatus(result.Steps)
		result.Summary = resultSummary(result.Status, len(result.Steps))
	}
}

func (r *Runner) providerBlockSummary(requirements Requirements) (string, bool) {
	if !requirements.RequiresRealProvider() || r.providerAvailability.Ready {
		return "", false
	}

	req := requirements.RealProvider
	parts := []string{firstNonEmpty(req.Reason, "real provider validation is required")}
	if req.Provider != "" {
		parts = append(parts, "provider="+req.Provider)
	}
	if req.Model != "" {
		parts = append(parts, "model="+req.Model)
	}
	if len(req.EnvVars) > 0 {
		parts = append(parts, "env="+strings.Join(req.EnvVars, ","))
	}
	if len(r.providerAvailability.Missing) > 0 {
		parts = append(parts, "missing="+strings.Join(r.providerAvailability.Missing, ","))
	} else {
		parts = append(parts, "provider readiness was not supplied")
	}
	if r.providerAvailability.Summary != "" {
		parts = append(parts, r.providerAvailability.Summary)
	}
	return strings.Join(parts, "; "), true
}

func (r *Runner) timestamp() time.Time {
	return r.now().UTC()
}

func resultStatus(steps []StepResult) Status {
	if len(steps) == 0 {
		return StatusSkipped
	}

	status := StatusPassed
	for _, step := range steps {
		switch step.Status {
		case StatusFailed:
			return StatusFailed
		case StatusBlocked:
			return StatusBlocked
		case StatusSkipped:
			status = StatusSkipped
		}
	}
	return status
}

func resultSummary(status Status, steps int) string {
	if steps == 1 {
		return fmt.Sprintf("1 step %s", status)
	}
	return fmt.Sprintf("%d steps %s", steps, status)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nonNegativeDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}
