package pluginrules

import (
	"reflect"
	"testing"

	"awaken/src/instructions"
	"awaken/src/plugins/ir"
)

func TestAssembleHonorsActivationAndManualPriority(t *testing.T) {
	entry := func(name, activation, body string, globs ...string) Entry {
		return Entry{
			PluginName: name, PluginDigest: "digest", PluginRoot: "/plugins/" + name, DesiredState: "enabled",
			Component: ir.Component{Kind: ir.KindRule, LogicalID: "rule:" + name, Mapping: ir.MappingAdapted, Validation: "valid", Rule: &ir.RuleSpec{Activation: activation, Globs: globs, Body: body}},
		}
	}
	projectEntry := entry("p-project", "always", "project plugin rule")
	projectEntry.Component.Rule.Scope = "project"
	sources := Assemble(Request{
		Entries: []Entry{
			entry("z-manual", "model_decides", "manual"),
			entry("a-always", "always", "always"),
			entry("b-glob", "file_globs", "glob", "**/*.ts"),
			entry("c-miss", "file_globs", "miss", "**/*.go"),
			projectEntry,
		},
		Manual:    []Selection{{PluginName: "z-manual", PluginDigest: "digest", LogicalID: "rule:z-manual"}},
		FilePaths: []string{"src/app.ts"},
	})
	var got []string
	for _, source := range sources {
		got = append(got, string(source.Scope)+":"+source.Content)
	}
	want := []string{
		string(instructions.ScopePluginUser) + ":always",
		string(instructions.ScopePluginUser) + ":glob",
		string(instructions.ScopePluginProject) + ":project plugin rule",
		string(instructions.ScopePluginManual) + ":manual",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sources=%#v", got)
	}
	if sources[2].Placement != instructions.ExtraAfterProject || sources[3].Placement != instructions.ExtraAfterProject {
		t.Fatalf("project/manual placements=%s/%s", sources[2].Placement, sources[3].Placement)
	}
}
