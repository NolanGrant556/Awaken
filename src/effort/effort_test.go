package effort

import (
	"errors"
	"testing"
)

func TestParseValueAcceptsLevelsAndNumbers(t *testing.T) {
	value, err := ParseValue("HIGH")
	if err != nil {
		t.Fatalf("ParseValue level error: %v", err)
	}
	if level, ok := value.Level(); !ok || level != LevelHigh {
		t.Fatalf("expected high level, got %q ok=%v", level, ok)
	}

	value, err = ParseValue("75")
	if err != nil {
		t.Fatalf("ParseValue numeric string error: %v", err)
	}
	if n, ok := value.Numeric(); !ok || n != 75 {
		t.Fatalf("expected numeric 75, got %d ok=%v", n, ok)
	}

	value, err = ParseValue(42)
	if err != nil {
		t.Fatalf("ParseValue numeric error: %v", err)
	}
	if n, ok := value.Numeric(); !ok || n != 42 {
		t.Fatalf("expected numeric 42, got %d ok=%v", n, ok)
	}
}

func TestParseValueRejectsInvalidInput(t *testing.T) {
	if _, err := ParseValue("auto"); err == nil {
		t.Fatal("expected auto to be invalid for ParseValue")
	}
	if _, err := ParseValue("medium-ish"); err == nil {
		t.Fatal("expected invalid string to fail")
	}
	if _, err := ParseValue(1.5); err == nil {
		t.Fatal("expected non-integer number to fail")
	}
}

func TestEnvOverrideParsesValueAndSuppressesAutoUnset(t *testing.T) {
	value, suppressed, err := EnvOverride("medium")
	if err != nil {
		t.Fatalf("EnvOverride medium error: %v", err)
	}
	if suppressed || value == nil {
		t.Fatalf("expected medium value without suppression, got value=%v suppressed=%v", value, suppressed)
	}
	if level, ok := value.Level(); !ok || level != LevelMedium {
		t.Fatalf("expected medium level, got %q ok=%v", level, ok)
	}

	for _, raw := range []string{"auto", "unset", " AUTO "} {
		value, suppressed, err := EnvOverride(raw)
		if err != nil {
			t.Fatalf("EnvOverride %q error: %v", raw, err)
		}
		if value != nil || !suppressed {
			t.Fatalf("expected %q to suppress with nil value, got value=%v suppressed=%v", raw, value, suppressed)
		}
	}

	value, suppressed, err = EnvOverride("")
	if err != nil {
		t.Fatalf("EnvOverride empty error: %v", err)
	}
	if value != nil || suppressed {
		t.Fatalf("expected empty env to be absent, got value=%v suppressed=%v", value, suppressed)
	}
}

func TestEnvOverrideRejectsInvalidValue(t *testing.T) {
	if _, _, err := EnvOverride("turbo"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected ErrInvalidValue, got %v", err)
	}
}

func TestResolveAppliedEffortUsesEnvBeforeSessionAndDefault(t *testing.T) {
	session := mustValue(t, "low")
	modelDefault := mustValue(t, "medium")
	resolved, err := ResolveAppliedEffort(ResolutionInput{
		Provider:     ProviderAnthropic,
		Model:        "claude-opus-4-6",
		EnvRaw:       "high",
		SessionValue: &session,
		ModelDefault: &modelDefault,
	})
	if err != nil {
		t.Fatalf("ResolveAppliedEffort error: %v", err)
	}
	if resolved.Value == nil {
		t.Fatal("expected resolved effort")
	}
	if resolved.Source != SourceEnvironment {
		t.Fatalf("expected env source, got %q", resolved.Source)
	}
	if level, ok := resolved.Value.Level(); !ok || level != LevelHigh {
		t.Fatalf("expected high, got %q ok=%v", level, ok)
	}
}

func TestResolveAppliedEffortAutoEnvOmitsEvenWithSession(t *testing.T) {
	session := mustValue(t, "high")
	resolved, err := ResolveAppliedEffort(ResolutionInput{
		Provider:     ProviderAnthropic,
		Model:        "claude-opus-4-6",
		EnvRaw:       "unset",
		SessionValue: &session,
	})
	if err != nil {
		t.Fatalf("ResolveAppliedEffort error: %v", err)
	}
	if !resolved.Omit || !resolved.Suppressed {
		t.Fatalf("expected omitted suppressed effort, got omit=%v suppressed=%v", resolved.Omit, resolved.Suppressed)
	}
	if resolved.Value != nil {
		t.Fatalf("expected nil value, got %v", resolved.Value)
	}
}

func TestResolveAppliedEffortRejectsNumericForNonAnt(t *testing.T) {
	numeric := NumericValue(80)
	_, err := ResolveAppliedEffort(ResolutionInput{
		Provider:     ProviderAnthropic,
		Model:        "claude-opus-4-6",
		SessionValue: &numeric,
		IsAnt:        false,
	})
	if !errors.Is(err, ErrNumericRequiresAnt) {
		t.Fatalf("expected ErrNumericRequiresAnt, got %v", err)
	}
}

func TestResolveAppliedEffortAllowsNumericForAntSessionOnly(t *testing.T) {
	numeric := NumericValue(80)
	resolved, err := ResolveAppliedEffort(ResolutionInput{
		Provider:     ProviderAnthropic,
		Model:        "claude-opus-4-6",
		SessionValue: &numeric,
		IsAnt:        true,
	})
	if err != nil {
		t.Fatalf("ResolveAppliedEffort error: %v", err)
	}
	if resolved.Value == nil {
		t.Fatal("expected resolved numeric effort")
	}
	if n, ok := resolved.Value.Numeric(); !ok || n != 80 {
		t.Fatalf("expected numeric 80, got %d ok=%v", n, ok)
	}
	if resolved.DisplayLevel != LevelMedium {
		t.Fatalf("expected numeric display medium, got %q", resolved.DisplayLevel)
	}
	if _, ok := ToPersistable(*resolved.Value, true); ok {
		t.Fatal("numeric effort should never be persistable")
	}
}

func TestResolveAppliedEffortDowngradesMaxWhenUnsupported(t *testing.T) {
	max := mustValue(t, "max")
	resolved, err := ResolveAppliedEffort(ResolutionInput{
		Provider:     ProviderAnthropic,
		Model:        "claude-sonnet-4-6",
		SessionValue: &max,
	})
	if err != nil {
		t.Fatalf("ResolveAppliedEffort error: %v", err)
	}
	if !resolved.Downgraded {
		t.Fatal("expected max to be downgraded")
	}
	if level, ok := resolved.Value.Level(); !ok || level != LevelHigh {
		t.Fatalf("expected high after downgrade, got %q ok=%v", level, ok)
	}
}

func TestToPersistableRules(t *testing.T) {
	low := mustValue(t, "low")
	if level, ok := ToPersistable(low, false); !ok || level != LevelLow {
		t.Fatalf("expected low persistable, got %q ok=%v", level, ok)
	}

	max := mustValue(t, "max")
	if _, ok := ToPersistable(max, false); ok {
		t.Fatal("max should not be persistable for non-ant users")
	}
	if level, ok := ToPersistable(max, true); !ok || level != LevelMax {
		t.Fatalf("expected ant max persistable, got %q ok=%v", level, ok)
	}

	numeric := NumericValue(101)
	if _, ok := ToPersistable(numeric, true); ok {
		t.Fatal("numeric effort should not be persistable")
	}
}

func TestSupportsEffortDefaultsAndOverrides(t *testing.T) {
	if !SupportsEffort(ProviderAnthropic, "claude-opus-4-6", ModelCapabilities{}) {
		t.Fatal("expected opus 4.6 to support effort")
	}
	if SupportsEffort(ProviderAnthropic, "claude-3-haiku", ModelCapabilities{}) {
		t.Fatal("expected haiku to not support effort by default")
	}
	if SupportsEffort(ProviderThirdParty, "anthropic/claude-opus-4-6", ModelCapabilities{}) {
		t.Fatal("expected third-party provider to not support effort by default")
	}
	if !SupportsEffort(ProviderThirdParty, "anthropic/claude-opus-4-6", ModelCapabilities{SupportsEffort: boolPtr(true)}) {
		t.Fatal("expected capability override to enable third-party effort")
	}
}

func TestResolveProviderPayloadAnthropicAdaptive(t *testing.T) {
	high := mustValue(t, "high")
	plan, err := ResolveProviderPayload(ResolutionInput{
		Provider:     ProviderAnthropic,
		Model:        "claude-opus-4-6",
		SessionValue: &high,
	})
	if err != nil {
		t.Fatalf("ResolveProviderPayload error: %v", err)
	}
	if plan.Omit || plan.Anthropic == nil {
		t.Fatalf("expected Anthropic payload, got omit=%v payload=%v", plan.Omit, plan.Anthropic)
	}
	if plan.Anthropic.ThinkingType != "adaptive" || plan.Anthropic.ThinkingDisplay != "summarized" {
		t.Fatalf("unexpected thinking payload: %+v", plan.Anthropic)
	}
	if plan.Anthropic.OutputConfigEffort != "high" {
		t.Fatalf("expected high output_config effort, got %q", plan.Anthropic.OutputConfigEffort)
	}
}

func TestResolveProviderPayloadOpenAIReasoningEffort(t *testing.T) {
	max := mustValue(t, "max")
	plan, err := ResolveProviderPayload(ResolutionInput{
		Provider:     ProviderOpenAI,
		Model:        "o3",
		SessionValue: &max,
	})
	if err != nil {
		t.Fatalf("ResolveProviderPayload error: %v", err)
	}
	if plan.Omit || plan.OpenAI == nil {
		t.Fatalf("expected OpenAI payload, got omit=%v payload=%v", plan.Omit, plan.OpenAI)
	}
	if plan.OpenAI.ReasoningEffort != "high" {
		t.Fatalf("expected max to downgrade to high for OpenAI, got %q", plan.OpenAI.ReasoningEffort)
	}
}

func TestResolveProviderPayloadLMStudioAllowedOptionsClamp(t *testing.T) {
	medium := mustValue(t, "medium")
	plan, err := ResolveProviderPayload(ResolutionInput{
		Provider:     ProviderLMStudio,
		Model:        "local-reasoner",
		SessionValue: &medium,
		Capabilities: ModelCapabilities{
			LMStudioAllowed: []string{"off", "on"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveProviderPayload error: %v", err)
	}
	if plan.Omit || plan.LMStudio == nil {
		t.Fatalf("expected LM Studio payload, got omit=%v payload=%v", plan.Omit, plan.LMStudio)
	}
	if plan.LMStudio.ReasoningEffort != "medium" {
		t.Fatalf("expected medium effort, got %q", plan.LMStudio.ReasoningEffort)
	}

	high := mustValue(t, "high")
	plan, err = ResolveProviderPayload(ResolutionInput{
		Provider:     ProviderLMStudio,
		Model:        "local-reasoner",
		SessionValue: &high,
		Capabilities: ModelCapabilities{
			LMStudioAllowed: []string{"off", "minimal", "low"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveProviderPayload error: %v", err)
	}
	if !plan.Omit || plan.Reason == "" {
		t.Fatalf("expected disallowed LM Studio effort to omit with reason, got omit=%v reason=%q", plan.Omit, plan.Reason)
	}
}

func mustValue(t *testing.T, raw string) Value {
	t.Helper()
	value, err := ParseValue(raw)
	if err != nil {
		t.Fatalf("ParseValue(%q): %v", raw, err)
	}
	return value
}

func boolPtr(value bool) *bool {
	return &value
}
