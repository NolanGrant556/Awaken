package configlayers

import "testing"

func TestResolveUsesHigherPrecedenceAndTracksOverriddenSources(t *testing.T) {
	defaults := NewBehaviorLayer("defaults", 0, Source{Name: "bundled", Path: "/etc/awaken/config.yaml"}, map[string]string{
		"model":       "gpt-4.1",
		"temperature": "0.2",
	})
	project := NewBehaviorLayer("project", 20, Source{Name: "project", Path: "/repo/config.yaml"}, map[string]string{
		"temperature": "0.4",
	})
	flags := NewBehaviorLayer("flags", 100, Source{Name: "cli flag"}, map[string]string{
		"temperature": "0.1",
	})

	resolution := Resolve(flags, defaults, project)

	value, ok := resolution.Get("temperature")
	if !ok {
		t.Fatalf("expected temperature to resolve")
	}
	if value.Value != "0.1" {
		t.Fatalf("expected highest precedence value, got %q", value.Value)
	}
	if value.LayerName != "flags" || value.Source.Name != "cli flag" {
		t.Fatalf("expected flag source, got %+v", value)
	}
	if len(value.Overridden) != 2 {
		t.Fatalf("expected 2 overridden sources, got %+v", value.Overridden)
	}
	if value.Overridden[0].LayerName != "project" || value.Overridden[0].Value != "0.4" {
		t.Fatalf("expected project to be the immediate overridden source, got %+v", value.Overridden[0])
	}
	if value.Overridden[1].LayerName != "defaults" || value.Overridden[1].Value != "0.2" {
		t.Fatalf("expected defaults to be the oldest overridden source, got %+v", value.Overridden[1])
	}

	model, ok := resolution.Get("model")
	if !ok {
		t.Fatalf("expected model to resolve")
	}
	if model.Value != "gpt-4.1" || model.LayerName != "defaults" {
		t.Fatalf("expected default model source, got %+v", model)
	}
}

func TestResolveTracksSourcePath(t *testing.T) {
	layer := NewBehaviorLayer("project", 10, Source{Name: "project file", Path: "/repo/.awaken/config.yaml"}, map[string]string{
		"approval_mode": "ask",
	})

	resolution := Resolve(layer)

	value, ok := resolution.Get("approval_mode")
	if !ok {
		t.Fatalf("expected approval_mode to resolve")
	}
	if value.Source.Path != "/repo/.awaken/config.yaml" {
		t.Fatalf("expected source path to be tracked, got %+v", value.Source)
	}
}

func TestSecretValuesAreRedactedInSafeSnapshots(t *testing.T) {
	behavior := NewBehaviorLayer("config file", 10, Source{Name: "config.yaml", Path: "/repo/config.yaml"}, map[string]string{
		"base_url": "https://api.example.test",
	})
	secrets := NewSecretLayer("env", 20, Source{Name: ".env", Path: "/repo/.env"}, map[string]string{
		"api_key": "sk-test",
	})

	resolution := Resolve(behavior, secrets)

	raw, ok := resolution.Get("api_key")
	if !ok {
		t.Fatalf("expected api_key to resolve")
	}
	if raw.Value != "sk-test" {
		t.Fatalf("expected raw value for runtime access, got %q", raw.Value)
	}
	if !raw.IsSecret() {
		t.Fatalf("expected api_key to be secret")
	}

	redacted := resolution.RedactedSnapshot()
	if redacted["api_key"] != RedactedValue {
		t.Fatalf("expected api_key to be redacted, got %q", redacted["api_key"])
	}
	if redacted["base_url"] != "https://api.example.test" {
		t.Fatalf("expected behavior config to remain visible, got %q", redacted["base_url"])
	}

	safe := resolution.SafeSnapshot()
	if safe["api_key"].Value != RedactedValue {
		t.Fatalf("expected safe snapshot to redact api_key, got %+v", safe["api_key"])
	}
	if safe["api_key"].Kind != SecretConfig {
		t.Fatalf("expected secret kind in safe snapshot, got %+v", safe["api_key"])
	}
}

func TestSecretMarkingSurvivesOverride(t *testing.T) {
	file := NewSecretLayer("file secret", 10, Source{Name: "config.yaml"}, map[string]string{
		"api_key": "old-secret",
	})
	envOverride := NewBehaviorLayer("env override", 20, Source{Name: "environment"}, map[string]string{
		"api_key": "new-secret",
	})

	resolution := Resolve(file, envOverride)

	value, ok := resolution.Get("api_key")
	if !ok {
		t.Fatalf("expected api_key to resolve")
	}
	if value.Value != "new-secret" {
		t.Fatalf("expected override value, got %q", value.Value)
	}
	if !value.IsSecret() {
		t.Fatalf("expected secret marking to survive override")
	}
	if resolution.RedactedSnapshot()["api_key"] != RedactedValue {
		t.Fatalf("expected overridden secret key to be redacted")
	}
}

func TestNewLayerDefensivelyCopiesInputMap(t *testing.T) {
	values := map[string]Entry{
		"model": BehaviorValue("gpt-4.1"),
	}
	layer := NewLayer("project", 10, Source{Name: "project"}, values)
	values["model"] = BehaviorValue("mutated")
	values["new_key"] = BehaviorValue("unexpected")

	resolution := Resolve(layer)

	value, ok := resolution.Get("model")
	if !ok {
		t.Fatalf("expected model to resolve")
	}
	if value.Value != "gpt-4.1" {
		t.Fatalf("expected defensive copy to preserve original value, got %q", value.Value)
	}
	if _, ok := resolution.Get("new_key"); ok {
		t.Fatalf("did not expect later input map mutation to affect resolution")
	}

	layerValues := layer.Values()
	layerValues["model"] = BehaviorValue("changed copy")
	value, ok = Resolve(layer).Get("model")
	if !ok {
		t.Fatalf("expected model to resolve after layer Values mutation")
	}
	if value.Value != "gpt-4.1" {
		t.Fatalf("expected Values to return a copy, got %q", value.Value)
	}
}

func TestEmptyLayersAndSamePrecedenceUseStableInputOrder(t *testing.T) {
	empty := NewBehaviorLayer("empty", 0, Source{Name: "empty"}, nil)
	first := NewBehaviorLayer("first", 10, Source{Name: "first"}, map[string]string{
		"mode": "first",
	})
	second := NewBehaviorLayer("second", 10, Source{Name: "second"}, map[string]string{
		"mode": "second",
	})

	emptyResolution := Resolve()
	if len(emptyResolution.Keys()) != 0 {
		t.Fatalf("expected empty resolution, got keys %+v", emptyResolution.Keys())
	}

	resolution := Resolve(empty, first, second)

	value, ok := resolution.Get("mode")
	if !ok {
		t.Fatalf("expected mode to resolve")
	}
	if value.Value != "second" {
		t.Fatalf("expected later same-precedence layer to win, got %q", value.Value)
	}
	if len(value.Overridden) != 1 || value.Overridden[0].LayerName != "first" {
		t.Fatalf("expected stable same-precedence history, got %+v", value.Overridden)
	}
}
