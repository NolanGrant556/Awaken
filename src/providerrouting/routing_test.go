package providerrouting

import (
	"reflect"
	"testing"
)

func TestPlanExcludesDeniedProviders(t *testing.T) {
	providers := []Provider{
		{Name: "anthropic"},
		{Name: "deepinfra"},
		{Name: "google"},
	}
	prefs := Preferences{
		AllowList:          []string{"anthropic", "deepinfra", "google"},
		DenyList:           []string{"deepinfra"},
		RequireOnlyAllowed: true,
	}

	plan := PlanProviders(providers, prefs)

	assertProviderNames(t, plan.Providers, []string{"anthropic", "google"})
	assertDecisionNames(t, plan.Excluded, []string{"deepinfra"})
	assertDecisionReasons(t, plan.Excluded[0], []string{ReasonDenied})
}

func TestPlanSupportsAllowOnlyMode(t *testing.T) {
	providers := []Provider{
		{Name: "anthropic"},
		{Name: "deepinfra"},
		{Name: "google"},
	}
	prefs := Preferences{
		AllowList:          []string{" google ", "ANTHROPIC"},
		RequireOnlyAllowed: true,
	}

	plan := PlanProviders(providers, prefs)

	assertProviderNames(t, plan.Providers, []string{"anthropic", "google"})
	assertDecisionNames(t, plan.Excluded, []string{"deepinfra"})
	assertDecisionReasons(t, plan.Excluded[0], []string{ReasonNotAllowed})
}

func TestPlanAppliesPriorityOrderBeforeOriginalOrder(t *testing.T) {
	providers := []Provider{
		{Name: "openai"},
		{Name: "anthropic"},
		{Name: "google"},
		{Name: "together"},
	}
	prefs := Preferences{
		PriorityOrder: []string{"google", "anthropic"},
	}

	plan := PlanProviders(providers, prefs)

	assertProviderNames(t, plan.Providers, []string{"google", "anthropic", "openai", "together"})
}

func TestPlanIgnoresUnknownPriorityProviders(t *testing.T) {
	providers := []Provider{
		{Name: "openai"},
		{Name: "anthropic"},
	}
	prefs := Preferences{
		PriorityOrder: []string{"missing"},
	}

	plan := PlanProviders(providers, prefs)

	assertProviderNames(t, plan.Providers, []string{"openai", "anthropic"})
}

func TestPlanKeepsStableOrderForPriorityTies(t *testing.T) {
	providers := []Provider{
		{Name: "openai"},
		{Name: "anthropic"},
		{Name: "google"},
		{Name: "together"},
	}
	prefs := Preferences{
		PriorityOrder: []string{"google"},
	}

	plan := prefs.Plan(providers)

	assertProviderNames(t, plan.Providers, []string{"google", "openai", "anthropic", "together"})
}

func TestPlanExplainsExcludedProviders(t *testing.T) {
	providers := []Provider{
		{Name: "deepinfra"},
		{Name: "openai"},
	}
	prefs := Preferences{
		AllowList:          []string{"anthropic"},
		DenyList:           []string{"deepinfra"},
		RequireOnlyAllowed: true,
	}

	plan := PlanProviders(providers, prefs)

	assertDecisionNames(t, plan.Excluded, []string{"deepinfra", "openai"})
	assertDecisionReasons(t, plan.Excluded[0], []string{ReasonDenied, ReasonNotAllowed})
	assertDecisionReasons(t, plan.Excluded[1], []string{ReasonNotAllowed})
}

func TestPlanLayersProviderStateBeforeUserPreferences(t *testing.T) {
	providers := []Provider{
		{Name: "deepinfra", Status: ProviderStatusDisabled},
	}
	prefs := Preferences{
		AllowList:          []string{"anthropic"},
		DenyList:           []string{"deepinfra"},
		RequireOnlyAllowed: true,
	}

	plan := PlanProviders(providers, prefs)

	assertDecisionNames(t, plan.Excluded, []string{"deepinfra"})
	assertDecisionReasons(t, plan.Excluded[0], []string{ReasonDisabled, ReasonDenied, ReasonNotAllowed})
}

func TestPlanExcludesUnavailableProviders(t *testing.T) {
	providers := []Provider{
		{Name: "anthropic"},
		{Name: "google", Status: ProviderStatusUnavailable},
	}

	plan := PlanProviders(providers, Preferences{})

	assertProviderNames(t, plan.Providers, []string{"anthropic"})
	assertDecisionNames(t, plan.Excluded, []string{"google"})
	assertDecisionReasons(t, plan.Excluded[0], []string{ReasonUnavailable})
}

func TestPlanRejectsBlankProviderNames(t *testing.T) {
	providers := []Provider{
		{Name: " "},
		{Name: "openai"},
	}

	plan := PlanProviders(providers, Preferences{})

	assertProviderNames(t, plan.Providers, []string{"openai"})
	assertDecisionNames(t, plan.Excluded, []string{" "})
	assertDecisionReasons(t, plan.Excluded[0], []string{ReasonUnknownProvider})
}

func TestNormalizePreferencesReturnsDefensiveCopies(t *testing.T) {
	prefs := Preferences{
		AllowList:     []string{" OpenAI ", "openai", "Anthropic"},
		DenyList:      []string{"DeepInfra"},
		PriorityOrder: []string{"Google"},
	}

	normalized := NormalizePreferences(prefs)
	prefs.AllowList[0] = "changed"
	prefs.DenyList[0] = "changed"
	prefs.PriorityOrder[0] = "changed"

	if !reflect.DeepEqual(normalized.AllowList, []string{"openai", "anthropic"}) {
		t.Fatalf("unexpected normalized allow list: %v", normalized.AllowList)
	}
	if !reflect.DeepEqual(normalized.DenyList, []string{"deepinfra"}) {
		t.Fatalf("unexpected normalized deny list: %v", normalized.DenyList)
	}
	if !reflect.DeepEqual(normalized.PriorityOrder, []string{"google"}) {
		t.Fatalf("unexpected normalized priority order: %v", normalized.PriorityOrder)
	}
}

func TestPlanDoesNotMutateInputs(t *testing.T) {
	providers := []Provider{
		{Name: "openai"},
		{Name: "anthropic"},
	}
	prefs := Preferences{
		PriorityOrder: []string{"anthropic"},
	}

	plan := PlanProviders(providers, prefs)
	providers[1].Name = "changed"
	prefs.PriorityOrder[0] = "changed"

	assertProviderNames(t, plan.Providers, []string{"anthropic", "openai"})
	if providers[0].Name != "openai" {
		t.Fatalf("PlanProviders mutated providers input: %v", providers)
	}
}

func assertProviderNames(t *testing.T, providers []Provider, want []string) {
	t.Helper()
	got := make([]string, 0, len(providers))
	for _, provider := range providers {
		got = append(got, provider.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider names = %v, want %v", got, want)
	}
}

func assertDecisionNames(t *testing.T, decisions []ProviderDecision, want []string) {
	t.Helper()
	got := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		got = append(got, decision.Provider.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decision names = %v, want %v", got, want)
	}
}

func assertDecisionReasons(t *testing.T, decision ProviderDecision, want []string) {
	t.Helper()
	if !reflect.DeepEqual(decision.Reasons, want) {
		t.Fatalf("reasons for %s = %v, want %v", decision.Provider.Name, decision.Reasons, want)
	}
}
