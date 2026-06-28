package featureflags

import (
	"errors"
	"reflect"
	"testing"
)

func TestEnvOverridesOnlyParseForInternalUsers(t *testing.T) {
	evaluator, err := NewEvaluator(Inputs{
		UserType:         "external",
		EnvOverridesJSON: `{"beta": true}`,
		Defaults:         map[string]any{"beta": false},
	})
	if err != nil {
		t.Fatalf("expected non-ant env override to be ignored without error, got %v", err)
	}

	resolved := evaluator.Resolve("beta")
	if resolved.Source != SourceDefault || resolved.Value != false {
		t.Fatalf("expected default value for non-ant user, got %#v", resolved)
	}
	if evaluator.HasOverride("beta") {
		t.Fatalf("expected ignored env value not to count as override")
	}
}

func TestEnvOverrideInvalidJSONReturnsError(t *testing.T) {
	_, err := NewEvaluator(Inputs{
		UserType:         "ant",
		EnvOverridesJSON: `{"beta":`,
	})
	if !errors.Is(err, ErrInvalidEnvOverrides) {
		t.Fatalf("expected ErrInvalidEnvOverrides, got %v", err)
	}
}

func TestEnvOverrideMustBeJSONObject(t *testing.T) {
	for _, raw := range []string{`null`, `[["beta", true]]`, `"beta"`} {
		t.Run(raw, func(t *testing.T) {
			_, err := NewEvaluator(Inputs{
				UserType:         "ant",
				EnvOverridesJSON: raw,
			})
			if !errors.Is(err, ErrInvalidEnvOverrides) {
				t.Fatalf("expected ErrInvalidEnvOverrides, got %v", err)
			}
		})
	}
}

func TestResolveUsesExpectedPriority(t *testing.T) {
	evaluator, err := NewEvaluator(Inputs{
		UserType:         "ant",
		EnvOverridesJSON: `{"feature": "env"}`,
		ConfigOverrides:  map[string]any{"feature": "config"},
		RemoteValues:     map[string]any{"feature": "remote"},
		Defaults:         map[string]any{"feature": "default"},
	})
	if err != nil {
		t.Fatalf("expected evaluator, got %v", err)
	}

	resolved := evaluator.Resolve("feature")
	if resolved.Source != SourceEnvOverride || resolved.Value != "env" || !resolved.Found {
		t.Fatalf("expected env override to win, got %#v", resolved)
	}
}

func TestResolveFallsThroughConfigRemoteDefault(t *testing.T) {
	evaluator, err := NewEvaluator(Inputs{
		ConfigOverrides: map[string]any{"config_only": "config"},
		RemoteValues: map[string]any{
			"config_only":  "remote",
			"remote_only":  "remote",
			"default_only": "remote-shadow",
		},
		Defaults: map[string]any{
			"default_only": "default",
			"missing":      "fallback",
		},
	})
	if err != nil {
		t.Fatalf("expected evaluator, got %v", err)
	}

	assertResolved(t, evaluator.Resolve("config_only"), SourceConfigOverride, "config")
	assertResolved(t, evaluator.Resolve("remote_only"), SourceRemote, "remote")
	assertResolved(t, evaluator.Resolve("missing"), SourceDefault, "fallback")

	notFound := evaluator.Resolve("unknown")
	if notFound.Found || notFound.Source != SourceNone || notFound.Value != nil {
		t.Fatalf("expected unknown feature not to resolve, got %#v", notFound)
	}
}

func TestNilValuesAreExplicitValues(t *testing.T) {
	evaluator, err := NewEvaluator(Inputs{
		UserType:         "ant",
		EnvOverridesJSON: `{"env_null": null}`,
		ConfigOverrides:  map[string]any{"config_null": nil},
		RemoteValues:     map[string]any{"remote_null": nil},
		Defaults:         map[string]any{"default_null": nil},
	})
	if err != nil {
		t.Fatalf("expected evaluator, got %v", err)
	}

	for _, feature := range []string{"env_null", "config_null", "remote_null", "default_null"} {
		resolved := evaluator.Resolve(feature)
		if !resolved.Found || resolved.Value != nil {
			t.Fatalf("expected %q to resolve to explicit nil, got %#v", feature, resolved)
		}
	}
	if !evaluator.HasOverride("env_null") || !evaluator.HasOverride("config_null") {
		t.Fatalf("expected explicit nil env/config values to count as overrides")
	}
}

func TestListOverridesReturnsEffectiveOverridesSorted(t *testing.T) {
	evaluator, err := NewEvaluator(Inputs{
		UserType:         "ant",
		EnvOverridesJSON: `{"z_env": true, "shared": "env"}`,
		ConfigOverrides: map[string]any{
			"a_config": "config",
			"shared":   "config",
		},
	})
	if err != nil {
		t.Fatalf("expected evaluator, got %v", err)
	}

	got := evaluator.ListOverrides()
	want := []Override{
		{Feature: "a_config", Value: "config", Source: SourceConfigOverride},
		{Feature: "shared", Value: "env", Source: SourceEnvOverride},
		{Feature: "z_env", Value: true, Source: SourceEnvOverride},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected overrides %#v, got %#v", want, got)
	}
}

func TestConfigOverrideHelpersReturnNewState(t *testing.T) {
	current := map[string]any{
		"existing": true,
	}

	withOverride := SetConfigOverride(current, "new_flag", "enabled")
	if _, ok := current["new_flag"]; ok {
		t.Fatalf("expected original map not to be mutated")
	}
	if withOverride["existing"] != true || withOverride["new_flag"] != "enabled" {
		t.Fatalf("expected copied map with new override, got %#v", withOverride)
	}

	withoutExisting := ClearConfigOverride(withOverride, "existing")
	if _, ok := withOverride["existing"]; !ok {
		t.Fatalf("expected clear helper not to mutate input map")
	}
	if _, ok := withoutExisting["existing"]; ok {
		t.Fatalf("expected cleared override to be removed, got %#v", withoutExisting)
	}

	empty := ClearConfigOverrides()
	if len(empty) != 0 {
		t.Fatalf("expected empty override state, got %#v", empty)
	}
}

func assertResolved(t *testing.T, got ResolvedValue, source Source, value any) {
	t.Helper()
	if got.Source != source || got.Value != value || !got.Found {
		t.Fatalf("expected source %q value %#v, got %#v", source, value, got)
	}
}
