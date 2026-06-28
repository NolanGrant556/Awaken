package hooktimeout

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultTimeoutsByHookType(t *testing.T) {
	hooks := []Hook{
		{ID: "prompt", Type: HookTypePrompt, Source: SourceManaged},
		{ID: "http", Type: HookTypeHTTP, Source: SourceManaged},
		{ID: "agent", Type: HookTypeAgent, Source: SourceManaged},
		{ID: "session", Type: HookTypeSession, Source: SourceManaged},
		{ID: "function", Type: HookTypeFunction, Source: SourceManaged},
	}

	plan, err := BuildPlan(hooks, Policy{})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	assertStepTimeout(t, plan.Steps[0], DefaultPromptTimeout)
	assertStepTimeout(t, plan.Steps[1], DefaultHTTPTimeout)
	assertStepTimeout(t, plan.Steps[2], DefaultAgentTimeout)
	assertStepTimeout(t, plan.Steps[3], DefaultSessionTimeout)
	assertStepTimeout(t, plan.Steps[4], DefaultFunctionTimeout)
	for _, step := range plan.Steps {
		if step.Status != StatusPlanned || !strings.Contains(step.Reason, "default") {
			t.Fatalf("expected planned default reason, got %+v", step)
		}
	}
}

func TestExplicitTimeoutSeconds(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "explicit", Type: HookTypePrompt, Source: SourceUser, TimeoutSeconds: seconds(12)},
	}, Policy{})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	assertStepTimeout(t, plan.Steps[0], 12*time.Second)
	if plan.Steps[0].TimeoutMS != 12000 {
		t.Fatalf("expected timeout_ms 12000, got %d", plan.Steps[0].TimeoutMS)
	}
	if !strings.Contains(plan.Steps[0].Reason, "explicit") {
		t.Fatalf("expected explicit reason, got %q", plan.Steps[0].Reason)
	}
}

func TestPolicyDefaultOverride(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "prompt", Type: HookTypePrompt, Source: SourceManaged},
	}, Policy{
		DefaultTimeouts: map[HookType]time.Duration{HookTypePrompt: 7 * time.Second},
	})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	assertStepTimeout(t, plan.Steps[0], 7*time.Second)
}

func TestExplicitTimeoutClampedToMax(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "slow", Type: HookTypeAgent, Source: SourceManaged, TimeoutSeconds: seconds(40)},
	}, Policy{MaxTimeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	assertStepTimeout(t, plan.Steps[0], 10*time.Second)
	if !strings.Contains(plan.Steps[0].Reason, "clamped") {
		t.Fatalf("expected clamp reason, got %q", plan.Steps[0].Reason)
	}
}

func TestExplicitTimeoutOverMaxCanError(t *testing.T) {
	_, err := BuildPlan([]Hook{
		{ID: "slow", Type: HookTypeAgent, Source: SourceManaged, TimeoutSeconds: seconds(40)},
	}, Policy{MaxTimeout: 10 * time.Second, RejectOverMax: true})
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("expected max timeout error, got %v", err)
	}
}

func TestExplicitTimeoutMustBePositive(t *testing.T) {
	_, err := BuildPlan([]Hook{
		{ID: "zero", Type: HookTypePrompt, Source: SourceManaged, TimeoutSeconds: seconds(0)},
	}, Policy{})
	if err == nil || !strings.Contains(err.Error(), "greater than 0") {
		t.Fatalf("expected positive timeout error, got %v", err)
	}
}

func TestDisableAllSkipsHooks(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "disabled", Type: HookTypePrompt, Source: SourceManaged},
	}, Policy{DisableAll: true})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	if len(plan.Steps) != 0 {
		t.Fatalf("expected no runnable steps, got %+v", plan.Steps)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Status != StatusSkipped {
		t.Fatalf("expected skipped decision, got %+v", plan.Skipped)
	}
	if !strings.Contains(plan.Skipped[0].Reason, "disabled") {
		t.Fatalf("expected disabled reason, got %q", plan.Skipped[0].Reason)
	}
}

func TestManagedOnlyFiltersUnmanagedHooks(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "managed", Type: HookTypePrompt, Source: SourceManaged},
		{ID: "project", Type: HookTypePrompt, Source: SourceProject},
		{ID: "user", Type: HookTypePrompt, Source: SourceUser},
	}, Policy{ManagedOnly: true})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	if len(plan.Steps) != 1 || plan.Steps[0].Hook.ID != "managed" {
		t.Fatalf("expected only managed runnable step, got %+v", plan.Steps)
	}
	if len(plan.Skipped) != 2 {
		t.Fatalf("expected unmanaged hooks skipped, got %+v", plan.Skipped)
	}
	for _, skipped := range plan.Skipped {
		if !strings.Contains(skipped.Reason, "managed hooks only") {
			t.Fatalf("expected managed-only reason, got %+v", skipped)
		}
	}
}

func TestPlanOrderingIsStable(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "third", Type: HookTypePrompt, Source: SourceManaged, Order: 2},
		{ID: "first", Type: HookTypePrompt, Source: SourceManaged, Order: 1},
		{ID: "second", Type: HookTypePrompt, Source: SourceManaged, Order: 1},
	}, Policy{})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	got := []string{plan.Steps[0].Hook.ID, plan.Steps[1].Hook.ID, plan.Steps[2].Hook.ID}
	want := []string{"first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected stable order %v, got %v", want, got)
		}
	}
}

func TestSkippedPlanReasonDoesNotValidateDisabledHookTimeout(t *testing.T) {
	plan, err := BuildPlan([]Hook{
		{ID: "disabled-invalid", Type: HookTypePrompt, Source: SourceUser, TimeoutSeconds: seconds(0)},
	}, Policy{ManagedOnly: true})
	if err != nil {
		t.Fatalf("disabled hook should be skipped before timeout validation, got error: %v", err)
	}
	if len(plan.Steps) != 0 || len(plan.Skipped) != 1 {
		t.Fatalf("expected one skipped decision, got %+v", plan)
	}
	if plan.Skipped[0].Reason == "" || plan.Skipped[0].Status != StatusSkipped {
		t.Fatalf("expected explanatory skipped decision, got %+v", plan.Skipped[0])
	}
}

func assertStepTimeout(t *testing.T, step Step, want time.Duration) {
	t.Helper()
	if step.Timeout != want {
		t.Fatalf("expected timeout %s, got %s in %+v", want, step.Timeout, step)
	}
}

func seconds(value int) *int {
	return &value
}
