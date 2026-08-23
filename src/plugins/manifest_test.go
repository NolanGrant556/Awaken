package plugins

import (
	"errors"
	"strings"
	"testing"
)

func TestParseManifestMinimalAndFull(t *testing.T) {
	minimal, err := parseManifest([]byte(`{"$schema":"` + SchemaPluginV1 + `","name":"hello-plugin"}`))
	if err != nil {
		t.Fatal(err)
	}
	if minimal.Manifest.Name != "hello-plugin" {
		t.Fatalf("name=%q", minimal.Manifest.Name)
	}

	full, err := parseManifest([]byte(`{
		"$schema":"` + SchemaPluginV1 + `",
		"name":"acme.tools",
		"version":"1.2.0",
		"description":"Tools",
		"author":{"name":"Acme","email":"a@example.com","url":"https://acme.test"},
		"homepage":"https://docs.acme.test",
		"repository":"https://github.com/acme/tools",
		"license":"MIT",
		"keywords":["lint"],
		"extensions":{"io.awaken":{"ignored":true}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if full.Manifest.Version != "1.2.0" || full.Manifest.Author == nil || full.Manifest.Author.Name != "Acme" {
		t.Fatalf("full manifest: %+v", full.Manifest)
	}
}

func TestParseManifestNonFatalUnknownAndExtensions(t *testing.T) {
	result, err := parseManifest([]byte(`{
		"$schema":"` + SchemaPluginV1 + `",
		"name":"sample",
		"skills":"./skills",
		"extensions":["nope"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) < 2 {
		t.Fatalf("diagnostics=%v", result.Diagnostics)
	}
}

func TestParseManifestFatalCases(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"missing schema", `{"name":"sample"}`, ErrManifestInvalid},
		{"wrong schema", `{"$schema":"https://example.test/plugin.json","name":"sample"}`, ErrUnsupportedSchema},
		{"unsupported version", `{"$schema":"https://agent-plugins.org/schemas/2.0.0/plugin.schema.json","name":"sample"}`, ErrUnsupportedSchema},
		{"bad name", `{"$schema":"` + SchemaPluginV1 + `","name":"My-Plugin"}`, ErrManifestInvalid},
		{"null version", `{"$schema":"` + SchemaPluginV1 + `","name":"sample","version":null}`, ErrManifestInvalid},
		{"author extra", `{"$schema":"` + SchemaPluginV1 + `","name":"sample","author":{"org":"x"}}`, ErrManifestInvalid},
		{"not object", `[]`, ErrManifestInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseManifest([]byte(tc.raw))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestPluginNameConstraints(t *testing.T) {
	valid := []string{"my-plugin", "acme.tools", "lint3r", "a"}
	invalid := []string{"", "My-Plugin", "-start", "has--double", "too.many..dots", strings.Repeat("a", 65)}
	for _, name := range valid {
		if !isValidPluginName(name) {
			t.Fatalf("%q should be valid", name)
		}
	}
	for _, name := range invalid {
		if isValidPluginName(name) {
			t.Fatalf("%q should be invalid", name)
		}
	}
}
