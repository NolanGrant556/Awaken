package plugins

import (
	"testing"

	"awaken/src/subagentruntime"
)

func TestResolvePluginAgentRoleAcceptsUniqueVendorName(t *testing.T) {
	roles := map[string]subagentruntime.Role{
		"plugin__continual-learning__agents-memory-updater": {
			Type:       "plugin__continual-learning__agents-memory-updater",
			PluginName: "continual-learning",
		},
	}

	role, ok := resolvePluginAgentRole(roles, "agents-memory-updater")
	if !ok || role.Type != "plugin__continual-learning__agents-memory-updater" {
		t.Fatalf("expected unique vendor Agent name to resolve, got %#v, %v", role, ok)
	}
	role, ok = resolvePluginAgentRole(roles, "plugin__continual-learning__agents-memory-updater")
	if !ok || role.Type != "plugin__continual-learning__agents-memory-updater" {
		t.Fatalf("expected canonical Plugin Agent identity to keep resolving, got %#v, %v", role, ok)
	}
}

func TestResolvePluginAgentRoleRejectsAmbiguousVendorName(t *testing.T) {
	roles := map[string]subagentruntime.Role{
		"plugin__alpha__reviewer": {Type: "plugin__alpha__reviewer"},
		"plugin__beta__reviewer":  {Type: "plugin__beta__reviewer"},
	}

	if role, ok := resolvePluginAgentRole(roles, "reviewer"); ok {
		t.Fatalf("ambiguous vendor Agent name must not resolve: %#v", role)
	}
	if role, ok := resolvePluginAgentRole(roles, "plugin__missing__reviewer"); ok {
		t.Fatalf("unknown canonical identity must not fall back by suffix: %#v", role)
	}
}
