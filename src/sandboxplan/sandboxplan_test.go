package sandboxplan

import (
	"reflect"
	"testing"
)

func TestBuildNetworkAllowAndDenyFromSettingsAndRules(t *testing.T) {
	plan := Build([]Settings{
		{
			Source: SourceUser,
			Reason: "user network settings",
			Network: NetworkSettings{
				AllowDomains: []string{"Example.COM", "https://docs.example.com/path"},
				DenyDomains:  []string{"blocked.example.com"},
			},
			Rules: []Rule{
				{Tool: ToolWebFetch, Action: RuleAllow, Pattern: "WebFetch(domain:api.example.com)", Reason: "api docs"},
				{Tool: ToolWebFetch, Action: RuleDeny, Pattern: "WebFetch(domain:deny.example.com)", Reason: "deny rule"},
			},
		},
	}, Options{DisableDefaultProtectedDenyWritePaths: true})

	assertEntryValues(t, plan.Network.AllowDomains, []string{
		"api.example.com",
		"docs.example.com",
		"example.com",
	})
	assertEntryValues(t, plan.Network.DenyDomains, []string{
		"blocked.example.com",
		"deny.example.com",
	})

	api := findEntry(t, plan.Network.AllowDomains, "api.example.com")
	if got := api.Metadata[0]; got.Source != SourceUser || got.Reason != "api docs" {
		t.Fatalf("unexpected api metadata: %#v", got)
	}
}

func TestBuildManagedDomainsOnlyKeepsOnlyPolicyAllowsAndAllDenies(t *testing.T) {
	plan := Build([]Settings{
		{
			Source:                  SourcePolicy,
			AllowManagedDomainsOnly: true,
			Network: NetworkSettings{
				AllowDomains: []string{"managed.example.com"},
				DenyDomains:  []string{"policy-deny.example.com"},
			},
		},
		{
			Source: SourceUser,
			Network: NetworkSettings{
				AllowDomains: []string{"user-allow.example.com"},
				DenyDomains:  []string{"user-deny.example.com"},
			},
			Rules: []Rule{
				{Tool: ToolWebFetch, Action: RuleAllow, Domain: "rule-allow.example.com"},
				{Tool: ToolWebFetch, Action: RuleDeny, Domain: "rule-deny.example.com"},
			},
		},
	}, Options{DisableDefaultProtectedDenyWritePaths: true})

	assertEntryValues(t, plan.Network.AllowDomains, []string{"managed.example.com"})
	assertEntryValues(t, plan.Network.DenyDomains, []string{
		"policy-deny.example.com",
		"rule-deny.example.com",
		"user-deny.example.com",
	})
}

func TestBuildFilesystemReadAndWritePathsFromSettingsAndRules(t *testing.T) {
	plan := Build([]Settings{
		{
			Source: SourceProject,
			Filesystem: FilesystemSettings{
				AllowReadPaths:  []string{" ./src "},
				DenyReadPaths:   []string{"./secrets"},
				AllowWritePaths: []string{"./tmp"},
				DenyWritePaths:  []string{"./locked"},
			},
			Rules: []Rule{
				{Tool: ToolFileRead, Action: RuleAllow, Pattern: "FileRead(path:./docs)", Reason: "docs read"},
				{Tool: ToolFileRead, Action: RuleDeny, Path: "./private", Reason: "private read"},
				{Tool: ToolFileEdit, Action: RuleAllow, Pattern: "FileEdit(path:./generated)", Reason: "generated write"},
				{Tool: ToolFileEdit, Action: RuleDeny, Path: "./immutable", Reason: "immutable write"},
			},
		},
	}, Options{DisableDefaultProtectedDenyWritePaths: true})

	assertEntryValues(t, plan.Filesystem.AllowReadPaths, []string{"docs", "src"})
	assertEntryValues(t, plan.Filesystem.DenyReadPaths, []string{"private", "secrets"})
	assertEntryValues(t, plan.Filesystem.AllowWritePaths, []string{"generated", "tmp"})
	assertEntryValues(t, plan.Filesystem.DenyWritePaths, []string{"immutable", "locked"})
}

func TestBuildManagedReadPathsOnlyKeepsOnlyPolicyAllowsAndAllDenies(t *testing.T) {
	plan := Build([]Settings{
		{
			Source:                    SourcePolicy,
			AllowManagedReadPathsOnly: true,
			Filesystem: FilesystemSettings{
				AllowReadPaths: []string{"./managed"},
				DenyReadPaths:  []string{"./policy-deny"},
			},
		},
		{
			Source: SourceLocal,
			Filesystem: FilesystemSettings{
				AllowReadPaths:  []string{"./local-read"},
				DenyReadPaths:   []string{"./local-deny"},
				AllowWritePaths: []string{"./local-write"},
				DenyWritePaths:  []string{"./local-write-deny"},
			},
			Rules: []Rule{
				{Tool: ToolFileRead, Action: RuleAllow, Path: "./rule-read"},
				{Tool: ToolFileRead, Action: RuleDeny, Path: "./rule-deny"},
			},
		},
	}, Options{DisableDefaultProtectedDenyWritePaths: true})

	assertEntryValues(t, plan.Filesystem.AllowReadPaths, []string{"managed"})
	assertEntryValues(t, plan.Filesystem.DenyReadPaths, []string{"local-deny", "policy-deny", "rule-deny"})
	assertEntryValues(t, plan.Filesystem.AllowWritePaths, []string{"local-write"})
	assertEntryValues(t, plan.Filesystem.DenyWritePaths, []string{"local-write-deny"})
}

func TestBuildAddsDefaultAndCustomProtectedDenyWritePaths(t *testing.T) {
	plan := Build(nil, Options{ProtectedDenyWritePaths: []string{"./awaken/settings.json"}})

	assertContainsEntry(t, plan.Filesystem.DenyWritePaths, ".claude/settings.json")
	assertContainsEntry(t, plan.Filesystem.DenyWritePaths, ".codex/skills")
	assertContainsEntry(t, plan.Filesystem.DenyWritePaths, "~/.claude/settings.json")
	assertContainsEntry(t, plan.Filesystem.DenyWritePaths, "awaken/settings.json")

	protected := findEntry(t, plan.Filesystem.DenyWritePaths, ".claude/settings.json")
	if got := protected.Metadata[0]; got.Source != SourcePolicy || got.Reason == "" {
		t.Fatalf("unexpected protected metadata: %#v", got)
	}
}

func TestBuildDecisionSupportsDisabledEnabledFailAndUnsandboxedFallback(t *testing.T) {
	enabled := true
	disabled := false
	fail := true
	allowUnsandboxed := true

	disabledPlan := Build([]Settings{{Enabled: &disabled}}, Options{DisableDefaultProtectedDenyWritePaths: true})
	if disabledPlan.Decision.Enabled {
		t.Fatalf("expected disabled plan")
	}

	enabledPlan := Build([]Settings{{
		Enabled:                  &enabled,
		FailIfUnavailable:        &fail,
		AllowUnsandboxedCommands: &allowUnsandboxed,
	}}, Options{DisableDefaultProtectedDenyWritePaths: true})
	if !enabledPlan.Decision.Enabled {
		t.Fatalf("expected enabled plan")
	}
	if !enabledPlan.Decision.FailIfUnavailable {
		t.Fatalf("expected fail-if-unavailable")
	}
	if !enabledPlan.Decision.AllowUnsandboxedCommands {
		t.Fatalf("expected unsandboxed fallback to be allowed")
	}
}

func TestBuildDedupeStableSortAndPreservesSourceMetadata(t *testing.T) {
	plan := Build([]Settings{
		{
			Source: SourceUser,
			Reason: "first",
			Network: NetworkSettings{
				AllowDomains: []string{"z.example.com", "a.example.com"},
			},
		},
		{
			Source: SourceProject,
			Reason: "second",
			Network: NetworkSettings{
				AllowDomains: []string{"A.EXAMPLE.COM"},
			},
		},
	}, Options{DisableDefaultProtectedDenyWritePaths: true})

	assertEntryValues(t, plan.Network.AllowDomains, []string{"a.example.com", "z.example.com"})
	entry := findEntry(t, plan.Network.AllowDomains, "a.example.com")
	want := []Metadata{
		{Source: SourceUser, Reason: "first"},
		{Source: SourceProject, Reason: "second"},
	}
	if !reflect.DeepEqual(entry.Metadata, want) {
		t.Fatalf("unexpected metadata: got %#v want %#v", entry.Metadata, want)
	}
}

func TestBuildDefensivelyCopiesInputSlices(t *testing.T) {
	domains := []string{"example.com"}
	paths := []string{"./src"}
	rules := []Rule{{Tool: ToolWebFetch, Action: RuleAllow, Domain: "api.example.com"}}
	inputs := []Settings{{
		Source: SourceUser,
		Network: NetworkSettings{
			AllowDomains: domains,
		},
		Filesystem: FilesystemSettings{
			AllowReadPaths: paths,
		},
		Rules: rules,
	}}

	plan := Build(inputs, Options{DisableDefaultProtectedDenyWritePaths: true})
	domains[0] = "changed.example.com"
	paths[0] = "./changed"
	rules[0].Domain = "changed-rule.example.com"
	inputs[0].Network.AllowDomains[0] = "mutated.example.com"

	assertEntryValues(t, plan.Network.AllowDomains, []string{"api.example.com", "example.com"})
	assertEntryValues(t, plan.Filesystem.AllowReadPaths, []string{"src"})
}

func assertEntryValues(t *testing.T, entries []Entry, want []string) {
	t.Helper()
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Value)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected entries: got %#v want %#v", got, want)
	}
}

func assertContainsEntry(t *testing.T, entries []Entry, value string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Value == value {
			return
		}
	}
	t.Fatalf("expected entries to contain %q, got %#v", value, entries)
}

func findEntry(t *testing.T, entries []Entry, value string) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Value == value {
			return entry
		}
	}
	t.Fatalf("expected entries to contain %q, got %#v", value, entries)
	return Entry{}
}
