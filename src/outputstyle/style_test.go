package outputstyle

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMergeStylesAppliesSourcePriority(t *testing.T) {
	catalog, warnings := MergeStyles(
		StyleConfig{Name: "same", Description: "built-in", Prompt: "builtin", Source: SourceBuiltin},
		StyleConfig{Name: "same", Description: "plugin", Prompt: "plugin", Source: SourcePlugin},
		StyleConfig{Name: "same", Description: "user", Prompt: "user", Source: SourceUser},
		StyleConfig{Name: "same", Description: "project", Prompt: "project", Source: SourceProject},
		StyleConfig{Name: "same", Description: "managed", Prompt: "managed", Source: SourceManaged},
		StyleConfig{Name: "same", Description: "policy", Prompt: "policy", Source: SourcePolicy},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	style, ok := catalog.Get("same")
	if !ok {
		t.Fatal("expected merged style")
	}
	if style.Source != SourcePolicy || style.Prompt != "policy" {
		t.Fatalf("expected policy style to win, got %+v", style)
	}
}

func TestForcedPluginStyleWinsBeforeSelectedStyle(t *testing.T) {
	catalog, warnings := MergeStyles(
		StyleConfig{Name: "first", Prompt: "first forced", Source: SourcePlugin, ForceForPlugin: true},
		StyleConfig{Name: "selected", Prompt: "selected", Source: SourceUser},
		StyleConfig{Name: "second", Prompt: "second forced", Source: SourcePlugin, ForceForPlugin: true},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no merge warnings, got %v", warnings)
	}

	style, resolveWarnings := ResolveStyle(catalog, "selected")
	if style == nil {
		t.Fatal("expected forced style")
	}
	if style.Name != "first" {
		t.Fatalf("expected stable first forced style, got %q", style.Name)
	}
	if len(resolveWarnings) != 1 || !strings.Contains(resolveWarnings[0], "multiple forced plugin output styles") {
		t.Fatalf("expected multiple forced warning, got %v", resolveWarnings)
	}
}

func TestResolveStyleUsesDefaultAndWarnsForUnknown(t *testing.T) {
	catalog, warnings := MergeStyles(BuiltinStyles()...)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	style, resolveWarnings := ResolveStyle(catalog, "")
	if style == nil || style.Name != DefaultStyleName {
		t.Fatalf("expected default style, got %+v", style)
	}
	if len(resolveWarnings) != 0 {
		t.Fatalf("expected no default warnings, got %v", resolveWarnings)
	}

	style, resolveWarnings = ResolveStyle(catalog, "missing")
	if style != nil {
		t.Fatalf("expected nil style for unknown selection, got %+v", style)
	}
	if len(resolveWarnings) != 1 || !strings.Contains(resolveWarnings[0], "unknown output style") {
		t.Fatalf("expected unknown style warning, got %v", resolveWarnings)
	}
}

func TestRenderPromptSection(t *testing.T) {
	style := StyleConfig{
		Name:                   "brief",
		Prompt:                 "Keep answers tight.",
		Source:                 SourceUser,
		KeepCodingInstructions: false,
	}

	section := RenderPromptSection(&style)
	expected := "# Output Style: brief\nKeep answers tight."
	if section != expected {
		t.Fatalf("expected %q, got %q", expected, section)
	}
	if style.KeepCodingInstructions {
		t.Fatal("expected caller-visible keep coding instructions flag to remain false")
	}
}

func TestMergeStylesWarnsAndSkipsInvalidStyle(t *testing.T) {
	catalog, warnings := MergeStyles(
		StyleConfig{Name: "valid", Prompt: "ok", Source: SourceUser},
		StyleConfig{Name: "empty", Source: SourceUser},
		StyleConfig{Name: "bad-source", Prompt: "ok", Source: Source("workspace")},
	)

	if len(warnings) != 2 {
		t.Fatalf("expected two invalid style warnings, got %v", warnings)
	}
	if _, ok := catalog.Get("valid"); !ok {
		t.Fatal("expected valid style to remain")
	}
	if _, ok := catalog.Get("empty"); ok {
		t.Fatal("expected empty style to be skipped")
	}
	if _, ok := catalog.Get("bad-source"); ok {
		t.Fatal("expected invalid source style to be skipped")
	}
}

func TestCatalogListIsStable(t *testing.T) {
	catalog, warnings := MergeStyles(
		StyleConfig{Name: "project", Prompt: "project", Source: SourceProject},
		StyleConfig{Name: "builtin", Prompt: "builtin", Source: SourceBuiltin},
		StyleConfig{Name: "plugin", Prompt: "plugin", Source: SourcePlugin},
		StyleConfig{Name: "user", Prompt: "user", Source: SourceUser},
	)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	got := styleNamesForTest(catalog.List())
	want := []string{"builtin", "plugin", "user", "project"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected stable priority order %v, got %v", want, got)
	}
}

func TestValidateStyleRejectsEmptyStyle(t *testing.T) {
	if err := ValidateStyle(StyleConfig{}); !errors.Is(err, ErrInvalidStyle) {
		t.Fatalf("expected ErrInvalidStyle, got %v", err)
	}
	if section := RenderPromptSection(&StyleConfig{}); section != "" {
		t.Fatalf("expected empty section for invalid style, got %q", section)
	}
}

func styleNamesForTest(styles []StyleConfig) []string {
	names := make([]string, 0, len(styles))
	for _, style := range styles {
		names = append(names, style.Name)
	}
	return names
}
