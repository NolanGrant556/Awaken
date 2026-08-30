package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/desktopdata"
	"awaken/src/pluginrules"
	"awaken/src/plugins/ir"
	pluginvariables "awaken/src/plugins/variables"
	"awaken/src/skills"
)

type compatExpectation struct {
	Dialect         ir.Dialect               `json:"dialect"`
	Name            string                   `json:"name"`
	Manifestless    bool                     `json:"manifestless"`
	Counts          map[ir.ComponentKind]int `json:"counts"`
	Mappings        map[ir.MappingState]int  `json:"mappings"`
	ForbiddenEvents []string                 `json:"forbiddenEvents"`
	HookEventOrder  []string                 `json:"hookEventOrder"`
}

type callbackSecretStore struct {
	*pluginvariables.MemorySecretStore
	afterSave func()
}

func (s *callbackSecretStore) Save(pluginName, key, currentRef, value string) (string, error) {
	ref, err := s.MemorySecretStore.Save(pluginName, key, currentRef, value)
	if s.afterSave != nil {
		callback := s.afterSave
		s.afterSave = nil
		callback()
	}
	return ref, err
}

func TestStateV1MigratesToV2OnNextWrite(t *testing.T) {
	home := t.TempDir()
	statePath := desktopdata.PluginsConfigPath(home)
	writeTestFile(t, statePath, `{"version":1,"plugins":{"legacy":{"enabled":true,"source":{"kind":"local_path","path":"/tmp/legacy"},"digest":"sha256-v2:abc"}}}`)
	store := NewStoreAt(home)
	state, err := store.readStateUnlocked()
	if err != nil {
		t.Fatal(err)
	}
	record := state.Plugins["legacy"]
	if state.Version != 2 || record.Dialect != ir.DialectAgentPlugins || record.ManifestPath != "plugin.json" || record.InstallationID == "" {
		t.Fatalf("migration incomplete: %#v", state)
	}
	if err := store.writeStateUnlocked(state); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || !containsBytes(raw, []byte(`"version": 2`)) {
		t.Fatalf("migrated state was not persisted: %s", raw)
	}
}

func TestUnknownStateVersionFailsClosed(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, desktopdata.PluginsConfigPath(home), `{"version":99,"plugins":{}}`)
	store := NewStoreAt(home)
	if err := store.Available(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("unknown state version did not fail closed: %v", err)
	}
}

func TestVariablesUseSecretStoreAndMaskedDTO(t *testing.T) {
	home := t.TempDir()
	secrets := pluginvariables.NewMemorySecretStore()
	store := NewStoreAtWithTrashAndSecrets(home, nil, secrets)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "cursor", "variables"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectCursor}
	validated, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest}); err != nil {
		t.Fatal(err)
	}
	fields, err := store.UpdateConfiguration("cursor-variables", map[string]any{"API_TOKEN": "super-secret-value", "MODE": "safe"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields=%d", len(fields))
	}
	for _, field := range fields {
		if field.Key == "API_TOKEN" && (!field.Configured || !field.Masked || field.Value != nil) {
			t.Fatalf("secret field leaked or is not configured: %#v", field)
		}
	}
	raw, err := os.ReadFile(desktopdata.PluginsConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if containsBytes(raw, []byte("super-secret-value")) {
		t.Fatalf("secret leaked into plugins.json: %s", raw)
	}
	if len(secrets.Values) != 1 {
		t.Fatalf("secret store values=%d", len(secrets.Values))
	}
}

func TestSetComponentDesiredMatchesInventorySwitchable(t *testing.T) {
	home := t.TempDir()
	store := NewStoreAtWithTrashAndSecrets(home, nil, pluginvariables.NewMemorySecretStore())
	variablesFixture, err := filepath.Abs(filepath.Join("testdata", "compat", "cursor", "variables"))
	if err != nil {
		t.Fatal(err)
	}
	variablesSource := Source{Kind: SourceLocalPath, Path: variablesFixture, DialectHint: ir.DialectCursor}
	variablesValidation, err := store.Validate(context.Background(), variablesSource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: variablesSource, ExpectedDigest: variablesValidation.Digest}); err != nil {
		t.Fatal(err)
	}

	findVariable := func() ComponentState {
		t.Helper()
		inventory, inventoryErr := store.ComponentInventory("cursor-variables")
		if inventoryErr != nil {
			t.Fatal(inventoryErr)
		}
		for _, state := range inventory {
			if state.Component.Variable != nil && state.Component.Variable.Key == "API_TOKEN" {
				return state
			}
		}
		t.Fatal("API_TOKEN Variable component missing")
		return ComponentState{}
	}

	unconfigured := findVariable()
	if unconfigured.Configuration != "needs_configuration" || unconfigured.Switchable {
		t.Fatalf("unconfigured state=%#v", unconfigured)
	}
	if _, err := store.SetComponentDesired("cursor-variables", unconfigured.Component.LogicalID, false); !errors.Is(err, ErrComponentNotSwitchable) {
		t.Fatalf("unconfigured Variable mutation err=%v", err)
	}

	if _, err := store.UpdateConfiguration("cursor-variables", map[string]any{"API_TOKEN": "secret"}, nil); err != nil {
		t.Fatal(err)
	}
	configured := findVariable()
	if configured.Configuration != "ready" || configured.Switchable {
		t.Fatalf("configured state=%#v", configured)
	}
	if _, err := store.SetComponentDesired("cursor-variables", configured.Component.LogicalID, false); !errors.Is(err, ErrComponentNotSwitchable) {
		t.Fatalf("configured Variable mutation err=%v", err)
	}

	skillFixture, err := filepath.Abs(filepath.Join("testdata", "compat", "codex", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	skillSource := Source{Kind: SourceLocalPath, Path: skillFixture, DialectHint: ir.DialectCodex}
	skillValidation, err := store.Validate(context.Background(), skillSource)
	if err != nil {
		t.Fatal(err)
	}
	skillInstalled, err := store.Install(context.Background(), InstallRequest{Source: skillSource, ExpectedDigest: skillValidation.Digest})
	if err != nil {
		t.Fatal(err)
	}
	var skillState ComponentState
	for _, state := range skillInstalled.Plugin.ComponentState {
		if state.Component.Kind == ir.KindSkill {
			skillState = state
			break
		}
	}
	if skillState.Component.LogicalID == "" || !skillState.Switchable {
		t.Fatalf("skill state=%#v", skillState)
	}
	updated, err := store.SetComponentDesired(skillInstalled.Plugin.Name, skillState.Component.LogicalID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range updated.ComponentState {
		if state.Component.LogicalID == skillState.Component.LogicalID && (state.DesiredState != "disabled" || !state.Switchable) {
			t.Fatalf("updated skill state=%#v", state)
		}
	}
}

func TestVariableSecretMutationRollsBackWhenStateWriteFails(t *testing.T) {
	home := t.TempDir()
	secrets := &callbackSecretStore{MemorySecretStore: pluginvariables.NewMemorySecretStore()}
	store := NewStoreAtWithTrashAndSecrets(home, nil, secrets)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "cursor", "variables"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectCursor}
	validated, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateConfiguration("cursor-variables", map[string]any{"API_TOKEN": "old-secret"}, nil); err != nil {
		t.Fatal(err)
	}
	var secretRef string
	for ref := range secrets.Values {
		secretRef = ref
	}
	if secretRef == "" {
		t.Fatal("initial secret was not saved")
	}
	originalStatePath := store.statePath
	blockedStatePath := filepath.Join(home, "state-is-a-directory")
	if err := os.MkdirAll(blockedStatePath, 0o700); err != nil {
		t.Fatal(err)
	}
	secrets.afterSave = func() { store.statePath = blockedStatePath }
	if _, err := store.UpdateConfiguration("cursor-variables", map[string]any{"API_TOKEN": "new-secret"}, nil); err == nil {
		t.Fatal("state write failure was not reported")
	}
	store.statePath = originalStatePath
	if got := secrets.Values[secretRef]; got != "old-secret" {
		t.Fatalf("secret rollback value=%q", got)
	}
	fields, err := store.Configuration("cursor-variables")
	if err != nil || len(fields) == 0 {
		t.Fatalf("configuration after rollback=%#v err=%v", fields, err)
	}
}

func TestVariableResetAndSetSameSecretRetainsActiveReference(t *testing.T) {
	home := t.TempDir()
	secrets := pluginvariables.NewMemorySecretStore()
	store := NewStoreAtWithTrashAndSecrets(home, nil, secrets)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "cursor", "variables"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectCursor}
	validated, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validated.Digest}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateConfiguration("cursor-variables", map[string]any{"API_TOKEN": "first"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateConfiguration("cursor-variables", map[string]any{"API_TOKEN": "second"}, []string{"API_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	if len(secrets.Values) != 1 {
		t.Fatalf("active secret reference was retired: %#v", secrets.Values)
	}
	for _, value := range secrets.Values {
		if value != "second" {
			t.Fatalf("secret value=%q", value)
		}
	}
}

func TestVendorSkillSelectionOverrideAndComposerProjection(t *testing.T) {
	store := testStore(t)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "codex", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectCodex}
	validation, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validation.Digest})
	if err != nil {
		t.Fatal(err)
	}
	selections := store.EnabledSkillSelections()
	if len(selections) != 1 || selections[0].PluginName != "codex-minimal" {
		t.Fatalf("selections=%#v", selections)
	}
	skills.SetPluginSkillSelections(selections)
	t.Cleanup(func() { skills.SetPluginSkillSelections(nil) })
	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(skills.LocalSourceDirs(""))
	if got := registry.Get("plugin__codex-minimal__codex-skill"); got == nil || got.State != skills.StateLoaded {
		t.Fatalf("native Skill was not projected: %#v", got)
	}
	var logicalID string
	for _, component := range installed.Plugin.Components {
		if component.Kind == ir.KindSkill {
			logicalID = component.LogicalID
			break
		}
	}
	if logicalID == "" {
		t.Fatal("Skill component missing")
	}
	if _, err := store.SetComponentDesired("codex-minimal", logicalID, false); err != nil {
		t.Fatal(err)
	}
	if got := store.EnabledSkillSelections(); len(got) != 0 {
		t.Fatalf("disabled Skill remained selected: %#v", got)
	}
	if _, err := store.ResolveComposerSelection("codex-minimal", installed.Plugin.Digest); !errors.Is(err, ErrNoUsableComponents) {
		t.Fatalf("composer selection err=%v", err)
	}
}

func TestVendorCommandExpansionIsDigestBound(t *testing.T) {
	store := testStore(t)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "claude-code", "manifest-minimal"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectClaudeCode}
	validation, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validation.Digest})
	if err != nil {
		t.Fatal(err)
	}
	var logicalID string
	for _, component := range installed.Plugin.Components {
		if component.Kind == ir.KindCommand {
			logicalID = component.LogicalID
		}
	}
	expansion, err := store.ExpandCommand(installed.Plugin.Name, installed.Plugin.Digest, logicalID, []string{"Nolan"})
	if err != nil {
		t.Fatal(err)
	}
	if expansion.Prompt != "Hello Nolan.\n" {
		t.Fatalf("prompt=%q", expansion.Prompt)
	}
	if _, err := store.ExpandCommand(installed.Plugin.Name, "stale", logicalID, []string{"Nolan"}); err == nil {
		t.Fatal("stale command digest was accepted")
	}
}

func TestVendorRuleActivationOverrideAndManualSelection(t *testing.T) {
	store := testStore(t)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "cursor", "rules"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectCursor}
	validation, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validation.Digest})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(store.RuleInstructionSources(nil, nil)); got != 1 {
		t.Fatalf("always sources=%d", got)
	}
	if got := len(store.RuleInstructionSources(nil, []string{"src/app.ts"})); got != 2 {
		t.Fatalf("typescript sources=%d", got)
	}
	var alwaysID string
	for _, component := range installed.Plugin.Components {
		if component.Kind == ir.KindRule && component.Rule != nil && component.Rule.AlwaysApply != nil && *component.Rule.AlwaysApply {
			alwaysID = component.LogicalID
		}
	}
	if _, err := store.SetRuleActivation(installed.Plugin.Name, alwaysID, "manual"); err != nil {
		t.Fatal(err)
	}
	if got := len(store.RuleInstructionSources(nil, nil)); got != 0 {
		t.Fatalf("manual rule auto-injected: %d", got)
	}
	manual := []pluginrules.Selection{{PluginName: installed.Plugin.Name, PluginDigest: installed.Plugin.Digest, LogicalID: alwaysID}}
	sources := store.RuleInstructionSources(manual, nil)
	if len(sources) != 1 || sources[0].Scope != "plugin_manual" {
		t.Fatalf("manual sources=%#v", sources)
	}
}

func TestVendorMCPTranslationAndPerToolOverride(t *testing.T) {
	store := testStore(t)
	fixture, err := filepath.Abs(filepath.Join("testdata", "compat", "codex", "complex"))
	if err != nil {
		t.Fatal(err)
	}
	source := Source{Kind: SourceLocalPath, Path: fixture, DialectHint: ir.DialectCodex}
	validation, err := store.Validate(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{Source: source, ExpectedDigest: validation.Digest})
	if err != nil {
		t.Fatal(err)
	}
	if len(installed.Plugin.MCPServers) != 1 || installed.Plugin.MCPServers[0].Config.HTTP == nil {
		t.Fatalf("vendor MCP was not translated: %#v", installed.Plugin.MCPServers)
	}
	var logicalID string
	for _, component := range installed.Plugin.Components {
		if component.Kind == ir.KindMCPServer {
			logicalID = component.LogicalID
		}
	}
	if _, err := store.SetMCPToolEnabled("codex-complex", logicalID, "search", false); err != nil {
		t.Fatal(err)
	}
	servers := store.PluginMCPServers()
	if len(servers) != 1 || len(servers[0].Config.DisabledTools) != 1 || servers[0].Config.DisabledTools[0] != "search" {
		t.Fatalf("tool override not projected: %#v", servers)
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for index := 0; index+len(needle) <= len(haystack); index++ {
		match := true
		for offset := range needle {
			if haystack[index+offset] != needle[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestCompatibilityFixtures(t *testing.T) {
	root := filepath.Join("testdata", "compat")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "expect.json" {
			return nil
		}
		t.Run(filepath.ToSlash(filepath.Dir(path)), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var expected compatExpectation
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatalf("parse expectation: %v", err)
			}
			store := NewStoreAt(t.TempDir())
			result, err := store.Validate(context.Background(), Source{
				Kind: SourceLocalPath, Path: filepath.Dir(path), DialectHint: expected.Dialect,
			})
			if err != nil {
				t.Fatalf("validate fixture: %v", err)
			}
			if !result.Valid {
				t.Fatalf("fixture invalid: %#v", result.Diagnostics)
			}
			if result.Dialect != expected.Dialect || result.Name != expected.Name {
				t.Fatalf("identity=(%s,%s), want=(%s,%s)", result.Dialect, result.Name, expected.Dialect, expected.Name)
			}
			counts := map[ir.ComponentKind]int{}
			mappings := map[ir.MappingState]int{}
			events := map[string]bool{}
			var hookEventOrder []string
			for _, component := range result.Components {
				counts[component.Kind]++
				mappings[component.Mapping]++
				if decoded, err := ir.DecodeLogicalID(component.LogicalIDEncoded); err != nil || decoded != component.LogicalID {
					t.Fatalf("logical ID did not round trip: id=%q encoded=%q err=%v", component.LogicalID, component.LogicalIDEncoded, err)
				}
				if component.Hook != nil {
					events[component.Hook.SourceEvent] = true
					hookEventOrder = append(hookEventOrder, component.Hook.SourceEvent)
				}
			}
			for kind, want := range expected.Counts {
				if got := counts[kind]; got != want {
					t.Fatalf("%s count=%d, want=%d; all=%v", kind, got, want, counts)
				}
			}
			if expected.Mappings == nil {
				t.Fatal("fixture expectation must declare mapping-state counts")
			}
			expectedMappingTotal := 0
			for mapping, want := range expected.Mappings {
				expectedMappingTotal += want
				if got := mappings[mapping]; got != want {
					t.Fatalf("%s mapping count=%d, want=%d; all=%v", mapping, got, want, mappings)
				}
			}
			if expectedMappingTotal != len(result.Components) {
				t.Fatalf("mapping expectation covers %d components, fixture has %d", expectedMappingTotal, len(result.Components))
			}
			if expected.Mappings[ir.MappingInvalid] > 0 {
				componentErrors := 0
				for _, diagnostic := range result.Diagnostics {
					if diagnostic.Severity == "error" && diagnostic.ComponentID != "" {
						componentErrors++
					}
				}
				if componentErrors < expected.Mappings[ir.MappingInvalid] {
					t.Fatalf("invalid components were not surfaced in validation diagnostics: %#v", result.Diagnostics)
				}
			}
			for _, event := range expected.ForbiddenEvents {
				if events[event] {
					t.Fatalf("forbidden source event was fabricated: %s", event)
				}
			}
			if len(expected.HookEventOrder) > 0 && strings.Join(hookEventOrder, "\x00") != strings.Join(expected.HookEventOrder, "\x00") {
				t.Fatalf("hook event order=%v, want=%v", hookEventOrder, expected.HookEventOrder)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompatibilityLaneMatrix(t *testing.T) {
	root := filepath.Join("testdata", "compat")
	cases := []struct {
		name, relative string
		hint, want     ir.Dialect
	}{
		{name: "agent-plugins-direct", relative: "agent-plugins-v1/full", hint: ir.DialectAgentPlugins, want: ir.DialectAgentPlugins},
		{name: "codex-native", relative: "codex/minimal", hint: ir.DialectCodex, want: ir.DialectCodex},
		{name: "claude-manifest", relative: "claude-code/manifest-minimal", hint: ir.DialectClaudeCode, want: ir.DialectClaudeCode},
		{name: "claude-manifestless", relative: "claude-code/manifestless", hint: ir.DialectClaudeCode, want: ir.DialectClaudeCode},
		{name: "cursor-native", relative: "cursor/minimal", hint: ir.DialectCursor, want: ir.DialectCursor},
		{name: "cursor-loads-portable-agent-plugin", relative: "agent-plugins-v1/full", hint: ir.DialectCursor, want: ir.DialectAgentPlugins},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewStoreAt(t.TempDir()).Validate(t.Context(), Source{
				Kind: SourceLocalPath, Path: filepath.Join(root, test.relative), DialectHint: test.hint,
			})
			if err != nil || !result.Valid || result.Dialect != test.want {
				t.Fatalf("lane result=%#v err=%v", result, err)
			}
		})
	}
}

func TestLogicalIDsStayStableAcrossManifestReorderAndUpgrade(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".cursor-plugin", "plugin.json"), `{"name":"stable-ids","version":"1.0.0","rules":["./rules/a.mdc","./rules/b.mdc"]}`)
	writeTestFile(t, filepath.Join(root, "rules", "a.mdc"), "---\ndescription: A\nalwaysApply: true\n---\nA")
	writeTestFile(t, filepath.Join(root, "rules", "b.mdc"), "---\ndescription: B\nalwaysApply: true\n---\nB")
	store := NewStoreAt(t.TempDir())
	first, err := store.Validate(t.Context(), Source{Kind: SourceLocalPath, Path: root, DialectHint: ir.DialectCursor})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, ".cursor-plugin", "plugin.json"), `{"name":"stable-ids","version":"2.0.0","rules":["./rules/b.mdc","./rules/a.mdc"]}`)
	writeTestFile(t, filepath.Join(root, "rules", "a.mdc"), "---\ndescription: A\nalwaysApply: true\n---\nA upgraded")
	second, err := store.Validate(t.Context(), Source{Kind: SourceLocalPath, Path: root, DialectHint: ir.DialectCursor})
	if err != nil {
		t.Fatal(err)
	}
	ids := func(components []ir.Component) map[string]bool {
		out := map[string]bool{}
		for _, component := range components {
			out[component.LogicalID] = true
		}
		return out
	}
	firstIDs, secondIDs := ids(first.Components), ids(second.Components)
	if len(firstIDs) != 2 || len(secondIDs) != len(firstIDs) {
		t.Fatalf("logical ID counts changed: %v -> %v", firstIDs, secondIDs)
	}
	for id := range firstIDs {
		if !secondIDs[id] {
			t.Fatalf("logical ID changed after reorder/upgrade: %s; next=%v", id, secondIDs)
		}
	}
}

func TestResolverRequiresSelectionForConflictingDialects(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable"}`)
	writeTestFile(t, filepath.Join(root, ".cursor-plugin", "plugin.json"), `{"name":"cursor-native"}`)
	resolved, err := resolvePackage(root, ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resolved.RequiresSelection || len(resolved.Candidates) != 2 {
		t.Fatalf("expected explicit selection: %#v", resolved)
	}
	selected, err := resolvePackage(root, ResolveOptions{SelectedDialect: ir.DialectCursor})
	if err != nil {
		t.Fatalf("resolve selected: %v", err)
	}
	if selected.Candidate.Dialect != ir.DialectCursor {
		t.Fatalf("selected dialect=%s", selected.Candidate.Dialect)
	}
}

func TestResolverAutoDetectsUniqueClaudeManifestlessPackage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "commands", "hello.md"), "---\ndescription: Say hello\n---\nHello")

	resolved, err := resolvePackage(root, ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve manifestless: %v", err)
	}
	if resolved.RequiresSelection || resolved.Candidate.Dialect != ir.DialectClaudeCode || !resolved.Candidate.Manifestless {
		t.Fatalf("unexpected manifestless resolution: %#v", resolved)
	}
}

func TestResolverRoutesStandaloneRootSkillAwayFromPluginInstaller(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "SKILL.md"), "---\nname: standalone\ndescription: Standalone Skill.\n---\nBody")
	writeTestFile(t, filepath.Join(root, "references", "guide.md"), "guide")

	_, err := resolvePackage(root, ResolveOptions{})
	if !errors.Is(err, ErrSkillPackageSelected) {
		t.Fatalf("resolve standalone Skill error=%v, want ErrSkillPackageSelected", err)
	}
}

func TestResolverDoesNotMergeSameIdentityWithConflictingVersions(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "plugin.json"), `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"portable","version":"1.0.0"}`)
	writeTestFile(t, filepath.Join(root, ".cursor-plugin", "plugin.json"), `{"name":"portable","version":"2.0.0"}`)

	resolved, err := resolvePackage(root, ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve conflicting versions: %v", err)
	}
	if !resolved.RequiresSelection || len(resolved.Candidates) != 2 {
		t.Fatalf("version conflict was silently merged: %#v", resolved)
	}
}

func TestPluginVariablesRejectCommandInterpolation(t *testing.T) {
	context := pluginvariables.InterpolationContext{PluginRoot: "/plugin", Values: map[string]string{"TOKEN": "secret"}}
	for dialectName, command := range map[ir.Dialect]string{
		ir.DialectClaudeCode: "python ${user_config.TOKEN}",
		ir.DialectCursor:     "python ${TOKEN}",
	} {
		if _, err := pluginvariables.Expand(command, dialectName, "hook.command", context); err == nil || !strings.Contains(err.Error(), "VARIABLE_FORBIDDEN_POSITION") {
			t.Fatalf("%s hook.command interpolation err=%v", dialectName, err)
		}
	}
}

func TestVendorMCPPreservesOAuthResource(t *testing.T) {
	component := ir.Component{
		DisplayName: "remote", SourceDialect: ir.DialectCodex,
		MCP: &ir.MCPSpec{ShortName: "remote", Transport: "streamable-http", URL: "https://mcp.example.test/rpc", OAuthResource: "https://resource.example.test/mcp"},
	}
	loaded, diagnostic := translateVendorMCP(component, &InstalledPlugin{Name: "oauth", Root: t.TempDir(), DataDir: t.TempDir(), Dialect: ir.DialectCodex}, pluginvariables.InterpolationContext{})
	if diagnostic != nil || loaded.Config.HTTP == nil || loaded.Config.OAuthResource != "https://resource.example.test/mcp" {
		t.Fatalf("loaded=%#v diagnostic=%#v", loaded, diagnostic)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
