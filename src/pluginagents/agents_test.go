package pluginagents

import (
	"testing"
	"time"

	"awaken/src/plugins/ir"
)

func TestCompileProjectsVendorAgentIntoNativeRole(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindAgent, LogicalID: "agent:agents/reviewer.md", RevisionID: "revision", Mapping: ir.MappingAdapted, Validation: "valid",
		DisplayName: "Reviewer", Agent: &ir.AgentSpec{Name: "reviewer", Description: "Review changes", Prompt: "Review the assigned work.", Model: "fast", Tools: []string{"Read", "Grep"}, MaxTurns: 5, TimeoutSeconds: 30, Background: true},
	}
	definition, err := Compile("Acme.Tools", "digest", component)
	if err != nil {
		t.Fatal(err)
	}
	role := definition.Role
	if role.Type != "plugin__acme_tools__reviewer" || !role.ReadOnly || role.MaxTurns != 5 || role.Timeout != 30*time.Second || len(role.AllowedTools) != 2 {
		t.Fatalf("role=%#v", role)
	}
	if len(definition.Diagnostics) != 2 {
		t.Fatalf("diagnostics=%#v", definition.Diagnostics)
	}
}

func TestCompilePreservesDeclaredPluginAgentLimits(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindAgent, LogicalID: "agent:agents/worker.md", Mapping: ir.MappingAdapted, Validation: "valid",
		DisplayName: "Worker", Agent: &ir.AgentSpec{Name: "worker", Prompt: "Work", MaxTurns: 99, TimeoutSeconds: 3600},
	}
	definition, err := Compile("bounded", "digest", component)
	if err != nil {
		t.Fatal(err)
	}
	role := definition.Role
	if role.MaxTurns != 99 || role.Timeout != time.Hour {
		t.Fatalf("role budget=%#v", role)
	}
}

func TestCompileLeavesUndeclaredPluginAgentLimitsUnset(t *testing.T) {
	component := ir.Component{
		Kind: ir.KindAgent, LogicalID: "agent:agents/worker.md", Mapping: ir.MappingAdapted, Validation: "valid",
		DisplayName: "Worker", Agent: &ir.AgentSpec{Name: "worker", Prompt: "Work"},
	}
	definition, err := Compile("unbounded", "digest", component)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Role.MaxTurns != 0 || definition.Role.Timeout != 0 {
		t.Fatalf("undeclared limits must remain unset: %#v", definition.Role)
	}
}
