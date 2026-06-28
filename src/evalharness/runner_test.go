package evalharness

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunnerRunsScenarioAndCapturesRedactedEvidence(t *testing.T) {
	store := NewMemoryEvidenceStore()
	calls := []string{}
	runner, err := NewRunner(StepExecutorFunc(func(ctx context.Context, scenario Scenario, step Step) (StepExecution, error) {
		calls = append(calls, scenario.ID+"/"+step.ID)
		return StepExecution{
			Status:  StatusPassed,
			Summary: "completed with sk-summarysecret",
			Text:    "Authorization: Bearer raw-token\nLLM_API_KEY=sk-outputsecret",
			Metadata: map[string]string{
				"headers": "x-api-key: sk-metasecret",
			},
		}, nil
	}), store, WithClock(tickingClock(time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC), time.Second)))
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	result, err := runner.Run(context.Background(), Scenario{
		ID:   "scenario-1",
		Name: "Scenario 1",
		Steps: []Step{
			{ID: "step-1", Name: "Step 1", Command: "echo first"},
			{ID: "step-2", Name: "Step 2", Command: "echo second"},
		},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Status != StatusPassed {
		t.Fatalf("expected passed result, got %s", result.Status)
	}
	if !reflect.DeepEqual(calls, []string{"scenario-1/step-1", "scenario-1/step-2"}) {
		t.Fatalf("unexpected executor calls: %#v", calls)
	}
	if len(result.Evidence) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(result.Evidence))
	}
	if result.Evidence[0].ID != "EV-0001" || result.Evidence[1].ID != "EV-0002" {
		t.Fatalf("unexpected evidence ids: %#v", result.Evidence)
	}
	if result.Steps[0].Duration != time.Second {
		t.Fatalf("expected deterministic one second duration, got %s", result.Steps[0].Duration)
	}

	joined := result.Evidence[0].Summary + "\n" + result.Evidence[0].Text + "\n" + result.Evidence[0].Metadata["headers"]
	for _, secret := range []string{"sk-summarysecret", "raw-token", "sk-outputsecret", "sk-metasecret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("evidence leaked secret %q in %q", secret, joined)
		}
	}
	for _, token := range []string{"Authorization: Bearer [REDACTED]", "LLM_API_KEY=[REDACTED]", "x-api-key: [REDACTED]"} {
		if !strings.Contains(joined, token) {
			t.Fatalf("expected redacted token %q in %q", token, joined)
		}
	}

	stored := store.ListEvidence()
	stored[0].Metadata["headers"] = "mutated"
	if store.ListEvidence()[0].Metadata["headers"] == "mutated" {
		t.Fatalf("memory evidence store exposed mutable metadata")
	}
}

func TestRunnerRecordsFailureAndStops(t *testing.T) {
	calls := []string{}
	runner, err := NewRunner(StepExecutorFunc(func(ctx context.Context, scenario Scenario, step Step) (StepExecution, error) {
		calls = append(calls, step.ID)
		return StepExecution{
			Text: "secret=should-not-leak",
		}, errors.New("provider failed with LLM_API_KEY=sk-failsecret")
	}), NewMemoryEvidenceStore())
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	result, err := runner.Run(context.Background(), Scenario{
		ID: "failure",
		Steps: []Step{
			{ID: "boom", Command: "run boom"},
			{ID: "after", Command: "run after"},
		},
	})
	if err != nil {
		t.Fatalf("Run returned infrastructure error: %v", err)
	}

	if result.Status != StatusFailed {
		t.Fatalf("expected failed result, got %s", result.Status)
	}
	if !reflect.DeepEqual(calls, []string{"boom"}) {
		t.Fatalf("expected fail-fast executor calls, got %#v", calls)
	}
	if len(result.Steps) != 1 || result.Steps[0].Status != StatusFailed {
		t.Fatalf("expected one failed step, got %#v", result.Steps)
	}
	if strings.Contains(result.Steps[0].Error, "sk-failsecret") {
		t.Fatalf("step error leaked secret: %q", result.Steps[0].Error)
	}
	if strings.Contains(result.Evidence[0].Text, "should-not-leak") {
		t.Fatalf("evidence text leaked secret: %q", result.Evidence[0].Text)
	}
}

func TestRunnerBlocksScenarioWhenRealProviderIsRequired(t *testing.T) {
	called := false
	runner, err := NewRunner(StepExecutorFunc(func(ctx context.Context, scenario Scenario, step Step) (StepExecution, error) {
		called = true
		return StepExecution{}, nil
	}), NewMemoryEvidenceStore(), WithProviderAvailability(ProviderAvailability{
		Ready:   false,
		Missing: []string{"LLM_API_KEY"},
		Summary: "local provider config is incomplete",
	}))
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	result, err := runner.Run(context.Background(), Scenario{
		ID: "real-provider",
		Requirements: Requirements{RealProvider: &RealProviderRequirement{
			Required: true,
			Provider: "deepseek",
			Model:    "deepseek-v4-pro",
			EnvVars:  []string{"LLM_API_KEY"},
			Reason:   "must validate the live Awaken path",
		}},
		Steps: []Step{{ID: "would-run", Command: "POST /v1/runs"}},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if called {
		t.Fatalf("executor must not run when scenario real-provider preflight is blocked")
	}
	if result.Status != StatusBlocked {
		t.Fatalf("expected blocked result, got %s", result.Status)
	}
	if len(result.Steps) != 0 {
		t.Fatalf("scenario-level block should not record step results, got %#v", result.Steps)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Command != "real-provider preflight" {
		t.Fatalf("expected preflight evidence, got %#v", result.Evidence)
	}
	if !strings.Contains(result.Summary, "LLM_API_KEY") || !strings.Contains(result.Summary, "provider=deepseek") {
		t.Fatalf("blocked summary should name missing requirement, got %q", result.Summary)
	}
}

func TestRunnerBlocksStepWhenRealProviderIsRequired(t *testing.T) {
	calls := []string{}
	runner, err := NewRunner(StepExecutorFunc(func(ctx context.Context, scenario Scenario, step Step) (StepExecution, error) {
		calls = append(calls, step.ID)
		return StepExecution{Summary: "ok"}, nil
	}), NewMemoryEvidenceStore(), WithProviderAvailability(ProviderAvailability{
		Ready:   false,
		Missing: []string{"LLM_API_KEY"},
	}))
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	result, err := runner.Run(context.Background(), Scenario{
		ID: "mixed",
		Steps: []Step{
			{ID: "local", Command: "GET /health"},
			{
				ID:      "real",
				Command: "POST /v1/runs",
				Requirements: Requirements{RealProvider: &RealProviderRequirement{
					Required: true,
					Reason:   "agent loop smoke requires provider",
				}},
			},
			{ID: "after", Command: "GET /v1/runs/{run_id}"},
		},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !reflect.DeepEqual(calls, []string{"local"}) {
		t.Fatalf("expected only local step to execute, got %#v", calls)
	}
	if result.Status != StatusBlocked {
		t.Fatalf("expected blocked result, got %s", result.Status)
	}
	if len(result.Steps) != 2 || result.Steps[1].StepID != "real" || result.Steps[1].Status != StatusBlocked {
		t.Fatalf("expected blocked provider step, got %#v", result.Steps)
	}
	if len(result.Evidence) != 2 || result.Evidence[1].StepID != "real" {
		t.Fatalf("expected step-level evidence, got %#v", result.Evidence)
	}
}

func TestBuiltinAcceptanceProfileIsDataDriven(t *testing.T) {
	profile := BuiltinAcceptanceProfile()
	if profile.ID == "" || len(profile.Scenarios) < 4 {
		t.Fatalf("profile should expose named built-in scenarios, got %#v", profile)
	}

	requiredTags := map[string]bool{
		"runtime":    false,
		"agent_loop": false,
		"tools":      false,
		"approval":   false,
		"sse":        false,
	}
	requiresProvider := false
	for _, scenario := range profile.Scenarios {
		if err := ValidateScenario(scenario); err != nil {
			t.Fatalf("built-in scenario %q is invalid: %v", scenario.ID, err)
		}
		if scenario.Requirements.RequiresRealProvider() {
			requiresProvider = true
		}
		for _, tag := range scenario.Tags {
			if _, ok := requiredTags[tag]; ok {
				requiredTags[tag] = true
			}
		}
		for _, step := range scenario.Steps {
			for _, tag := range step.Tags {
				if _, ok := requiredTags[tag]; ok {
					requiredTags[tag] = true
				}
			}
		}
	}

	if !requiresProvider {
		t.Fatalf("built-in acceptance profile should include real-provider scenarios")
	}
	for tag, seen := range requiredTags {
		if !seen {
			t.Fatalf("built-in acceptance profile is missing tag %q", tag)
		}
	}
}

func TestValidateScenarioRejectsInvalidShape(t *testing.T) {
	cases := []Scenario{
		{},
		{ID: "empty-steps"},
		{ID: "empty-step-id", Steps: []Step{{Command: "GET /health"}}},
		{ID: "empty-command", Steps: []Step{{ID: "step"}}},
		{ID: "duplicate", Steps: []Step{{ID: "step", Command: "one"}, {ID: "step", Command: "two"}}},
	}
	for _, scenario := range cases {
		if err := ValidateScenario(scenario); err == nil {
			t.Fatalf("expected invalid scenario error for %#v", scenario)
		}
	}
}

func TestRedactSecrets(t *testing.T) {
	redacted := RedactSecrets(`Authorization: Bearer bearer-secret
x-api-key: sk-headersecret
LLM_API_KEY=sk-envsecret
{"api_key":"sk-jsonsecret","password":"p@ssw0rd"}`)

	for _, secret := range []string{"bearer-secret", "sk-headersecret", "sk-envsecret", "sk-jsonsecret", "p@ssw0rd"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted text leaked %q: %s", secret, redacted)
		}
	}
	if got := strings.Count(redacted, "[REDACTED]"); got < 5 {
		t.Fatalf("expected at least five redactions, got %d in %s", got, redacted)
	}
}

func tickingClock(start time.Time, step time.Duration) func() time.Time {
	tick := 0
	return func() time.Time {
		current := start.Add(time.Duration(tick) * step)
		tick++
		return current
	}
}
