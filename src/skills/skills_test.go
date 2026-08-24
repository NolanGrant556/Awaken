package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Frontmatter parsing tests ----

func TestParseFrontmatterValid(t *testing.T) {
	raw := `---
name: test-skill
description: A test skill
whenToUse: when testing
arguments:
  - name: input
    type: string
    description: input text
    required: true
context: inline
---
This is the body.
`
	def, err := ParseFrontmatter(raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if def.Name != "test-skill" {
		t.Errorf("expected name test-skill, got %q", def.Name)
	}
	if def.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got %q", def.Description)
	}
	if def.WhenToUse != "when testing" {
		t.Errorf("expected whenToUse 'when testing', got %q", def.WhenToUse)
	}
	if len(def.Arguments) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(def.Arguments))
	}
	if def.Arguments[0].Name != "input" {
		t.Errorf("expected arg name input, got %q", def.Arguments[0].Name)
	}
	if def.Body != "This is the body." {
		t.Errorf("expected body 'This is the body.', got %q", def.Body)
	}
}

func TestParseFrontmatterMissingClosing(t *testing.T) {
	raw := `---
name: broken
description: missing closing
`
	_, err := ParseFrontmatter(raw)
	if err == nil {
		t.Fatal("expected error for missing closing ---, got nil")
	}
}

func TestParseFrontmatterInvalidYAML(t *testing.T) {
	raw := `---
name: [invalid: yaml: structure:
---
Body.
`
	_, err := ParseFrontmatter(raw)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseFrontmatterNoFrontmatter(t *testing.T) {
	raw := `Just a markdown file without frontmatter.`
	_, err := ParseFrontmatter(raw)
	if err == nil {
		t.Fatal("expected error for missing frontmatter, got nil")
	}
}

// ---- Schema validation tests ----

func TestValidateSchemaRequiredFields(t *testing.T) {
	def := &SkillDefinition{ID: "test"}
	if err := ValidateSchema(def); err == nil {
		t.Fatal("expected error for missing name, got nil")
	}

	def.Name = "test-skill"
	if err := ValidateSchema(def); err == nil {
		t.Fatal("expected error for missing description, got nil")
	}

	def.Description = "A test"
	if err := ValidateSchema(def); err != nil {
		t.Errorf("expected no error for valid def, got %v", err)
	}
}

func TestValidateSchemaArgumentTypes(t *testing.T) {
	def := &SkillDefinition{
		ID:          "test",
		Name:        "test",
		Description: "test",
		Arguments: []SkillArgument{
			{Name: "bad", Type: "datetime"},
		},
	}
	if err := ValidateSchema(def); err == nil {
		t.Fatal("expected error for invalid argument type, got nil")
	}

	def.Arguments[0].Type = "string"
	if err := ValidateSchema(def); err != nil {
		t.Errorf("expected no error for valid type, got %v", err)
	}
}

func TestValidateSchemaContextMode(t *testing.T) {
	def := &SkillDefinition{
		ID:          "test",
		Name:        "test",
		Description: "test",
		Context:     "unknown_mode",
	}
	if err := ValidateSchema(def); err == nil {
		t.Fatal("expected error for invalid context mode, got nil")
	}
	def.Context = ContextFork
	if err := ValidateSchema(def); err != nil {
		t.Errorf("expected no error for valid context mode, got %v", err)
	}
}

// ---- LoadDir tests ----

func TestLoadDirValidSkills(t *testing.T) {
	dir := t.TempDir()

	writeTestSkillPackage(t, dir, "alpha", `---
name: Alpha
description: First skill
---
Alpha body.
`)
	writeTestSkillPackage(t, dir, "beta", `---
name: Beta
description: Second skill
whenToUse: when you need beta
context: fork
---
Beta body.
`)

	loaded := LoadDir(dir, SourceProject)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(loaded))
	}

	alpha := loaded["alpha"]
	if alpha == nil {
		t.Fatal("expected alpha skill")
	}
	if alpha.State != StateLoaded {
		t.Errorf("expected available, got %s", alpha.State)
	}
	if alpha.Source != SourceProject {
		t.Errorf("expected source project, got %s", alpha.Source)
	}
	if alpha.Context != ContextInline {
		t.Errorf("expected default context inline, got %s", alpha.Context)
	}
	if filepath.Base(alpha.RootDir) != "alpha" {
		t.Errorf("expected root dir for alpha package, got %s", alpha.RootDir)
	}
	if filepath.Base(alpha.SkillFilePath) != "SKILL.md" {
		t.Errorf("expected SKILL.md path, got %s", alpha.SkillFilePath)
	}

	beta := loaded["beta"]
	if beta == nil {
		t.Fatal("expected beta skill")
	}
	if beta.Context != ContextFork {
		t.Errorf("expected fork context, got %s", beta.Context)
	}
}

func TestLoadDirSkipsInvalid(t *testing.T) {
	dir := t.TempDir()

	// Missing name -> should still load but be unavailable.
	writeTestSkillPackage(t, dir, "broken", `---
description: No name
---
Body.
`)
	// Valid.
	writeTestSkillPackage(t, dir, "good", `---
name: Good
description: Valid skill
---
Body.
`)

	loaded := LoadDir(dir, SourceUser)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(loaded))
	}
	if loaded["broken"].State != StateLoaded {
		t.Errorf("expected missing-name package to use directory name and load, got %s", loaded["broken"].State)
	}
	if loaded["broken"].Name != "broken" {
		t.Errorf("expected fallback name broken, got %s", loaded["broken"].Name)
	}
}

func TestLoadDirSkipsNonMd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "just notes")
	writeTestSkillPackage(t, dir, "valid", `---
name: Valid
description: A valid skill
---
Body.
`)

	loaded := LoadDir(dir, SourceProject)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(loaded))
	}
}

func TestLoadDirIgnoresFlatMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "legacy.md", `---
name: legacy
description: Legacy flat skill.
---
Body.
`)
	writeTestSkillPackage(t, dir, "packaged", `---
name: packaged
description: Packaged skill.
---
Body.
`)

	loaded := LoadDir(dir, SourceProject)
	if len(loaded) != 1 {
		t.Fatalf("expected only packaged skill, got %d", len(loaded))
	}
	if loaded["packaged"] == nil {
		t.Fatalf("expected packaged skill, got %+v", loaded)
	}
}

func TestLoadDirEmptyDir(t *testing.T) {
	dir := t.TempDir()
	loaded := LoadDir(dir, SourceProject)
	if len(loaded) != 0 {
		t.Errorf("expected 0 skills from empty dir, got %d", len(loaded))
	}
}

func TestLoadDirSkipsReservedSystemSkillsDirectoryCaseInsensitive(t *testing.T) {
	for _, reservedName := range []string{"system-skills", "SYSTEM-SKILLS", "System-Skills"} {
		t.Run(reservedName, func(t *testing.T) {
			root := t.TempDir()
			bundledRoot := filepath.Join(root, reservedName)
			writeTestSkillPackage(t, bundledRoot, "word", `---
name: word
description: Word documents.
---
Body.
`)

			if loaded := LoadDir(root, SourceUser); len(loaded) != 0 {
				t.Fatalf("reserved system-skills directory leaked into user discovery: %+v", loaded)
			}
			loaded := LoadDir(bundledRoot, SourceBundled)
			if len(loaded) != 1 || loaded["word"] == nil || loaded["word"].Source != SourceBundled {
				t.Fatalf("bundled root did not load its direct child package: %+v", loaded)
			}
		})
	}
}

// ---- Merge tests ----

func TestMergePriority(t *testing.T) {
	user := map[string]*SkillDefinition{
		"alpha": {ID: "alpha", Name: "A User", Description: "desc", Source: SourceUser, State: StateLoaded},
	}
	project := map[string]*SkillDefinition{
		"alpha": {ID: "alpha", Name: "A Project", Description: "desc", Source: SourceProject, State: StateLoaded},
	}

	merged := MergeSkills(user, project)
	if len(merged) != 2 {
		t.Fatalf("expected 2 entries (one shadowed), got %d", len(merged))
	}

	winner := merged["alpha"]
	if winner == nil {
		t.Fatal("expected alpha to exist")
	}
	if winner.Source != SourceProject {
		t.Errorf("expected project source to win, got %s", winner.Source)
	}

	var shadowed *SkillDefinition
	for key, def := range merged {
		if isShadowedMapKey(key) {
			shadowed = def
		}
	}
	if shadowed == nil {
		t.Fatal("expected shadowed alpha")
	}
	if shadowed.State != StateShadowed {
		t.Errorf("expected shadowed state, got %s", shadowed.State)
	}
	if shadowed.ShadowedBy == "" {
		t.Error("expected ShadowedBy to be set")
	}
}

// ---- Registry tests ----

func TestSkillRegistryLifecycle(t *testing.T) {
	reg := NewSkillRegistry(nil)

	userDir := t.TempDir()
	writeTestSkillPackage(t, userDir, "test", `---
name: test
description: A test skill
---
Body.
`)
	reg.LoadFromSources(map[SourceOrigin]string{SourceUser: userDir})

	avail := reg.ListLoaded()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill, got %d", len(avail))
	}
	if avail[0].ID != "test" {
		t.Errorf("expected test skill, got %s", avail[0].ID)
	}

	all := reg.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 skill in list all, got %d", len(all))
	}

	got := reg.Get("test")
	if got == nil {
		t.Fatal("expected test skill in registry")
	}
}

func TestBundledSkillIsShadowedAndRestoredAfterOverrideRemoval(t *testing.T) {
	for _, source := range []SourceOrigin{SourceUser, SourceProject} {
		t.Run(string(source), func(t *testing.T) {
			bundledDir := t.TempDir()
			overrideDir := t.TempDir()
			writeTestSkillPackage(t, bundledDir, "documents", `---
name: documents
description: Bundled documents.
---
Bundled.
`)
			writeTestSkillPackage(t, overrideDir, "documents", `---
name: documents
description: Override documents.
---
Override.
`)

			registry := NewSkillRegistry(nil)
			dirs := map[SourceOrigin]string{
				SourceBundled: bundledDir,
				source:        overrideDir,
			}
			registry.LoadFromSources(dirs)
			if winner := registry.Get("documents"); winner == nil || winner.Source != source {
				t.Fatalf("override source did not win: %+v", winner)
			}
			shadowedBundled := false
			for _, definition := range registry.ListAll() {
				if definition.ID == "documents" && definition.Source == SourceBundled && definition.State == StateShadowed {
					shadowedBundled = true
				}
			}
			if !shadowedBundled {
				t.Fatal("bundled definition was not retained as shadowed")
			}

			if err := os.RemoveAll(filepath.Join(overrideDir, "documents")); err != nil {
				t.Fatal(err)
			}
			registry.ReloadLocalSources(dirs)
			if restored := registry.Get("documents"); restored == nil || restored.Source != SourceBundled || restored.State != StateLoaded {
				t.Fatalf("bundled definition was not restored: %+v", restored)
			}
		})
	}
}

func TestSkillRegistryConnectorRequirementsAreExecutionHints(t *testing.T) {
	reg := NewSkillRegistry(nil)

	projectDir := t.TempDir()
	writeTestSkillPackage(t, projectDir, "needs-notion", `---
name: needs-notion
description: Uses Notion
requiredConnectors:
  - notion
---
Body.
`)
	writeTestSkillPackage(t, projectDir, "no-connector", `---
name: no-connector
description: No connectors needed
---
Body.
`)
	reg.LoadFromSources(map[SourceOrigin]string{SourceProject: projectDir})

	// Catalog loading is independent of connector readiness.
	avail := reg.ListLoaded()
	if len(avail) != 2 {
		t.Fatalf("expected both packages loaded, got %d", len(avail))
	}

	needsNotion := reg.Get("needs-notion")
	if needsNotion == nil {
		t.Fatal("expected needs-notion to exist")
	}
	if needsNotion.State != StateLoaded || needsNotion.UnavailableReason != "" {
		t.Errorf("dependency incorrectly gated package: %+v", needsNotion)
	}

	// Make notion connector available.
	reg.SetConnectorAvailable("notion", true)
	avail = reg.ListLoaded()
	if len(avail) != 2 {
		t.Fatalf("expected 2 available skills after connector, got %d", len(avail))
	}

	// Disconnect notion again.
	reg.SetConnectorAvailable("notion", false)
	avail = reg.ListLoaded()
	if len(avail) != 2 {
		t.Fatalf("connector disconnect must not unload package, got %d", len(avail))
	}
}

func TestSkillRegistryOnChangeTracksCatalogAndRuntimeBindingChanges(t *testing.T) {
	reg := NewSkillRegistry(nil)
	changes := make(chan struct{}, 4)
	reg.SetOnChange(func() {
		changes <- struct{}{}
	})

	userDir := t.TempDir()
	initial := `---
name: X
description: x
---
Body.
`
	writeTestSkillPackage(t, userDir, "x", initial)
	dirs := map[SourceOrigin]string{SourceUser: userDir}
	reg.LoadFromSources(dirs)
	expectSkillRegistryChange(t, changes)
	initialFingerprint := reg.ToolStateFingerprint()

	reg.ReloadLocalSources(dirs)
	expectNoSkillRegistryChange(t, changes)
	if reg.ToolStateFingerprint() != initialFingerprint {
		t.Fatal("same-source same-content reload changed the published fingerprint")
	}

	reg.MarkInvoked("x", time.Now())
	expectNoSkillRegistryChange(t, changes)
	if reg.ToolStateFingerprint() != initialFingerprint {
		t.Fatal("recent-use metadata changed the model/runtime fingerprint")
	}
	reg.SetConnectorAvailable("unused-connector", true)
	expectNoSkillRegistryChange(t, changes)
	if reg.ToolStateFingerprint() != initialFingerprint {
		t.Fatal("readiness diagnostics changed the model/runtime fingerprint")
	}

	catalogChanged := `---
name: X
description: changed catalog description
---
Body.
`
	if err := os.WriteFile(filepath.Join(userDir, "x", "SKILL.md"), []byte(catalogChanged), 0o600); err != nil {
		t.Fatal(err)
	}
	reg.ReloadLocalSources(dirs)
	expectSkillRegistryChange(t, changes)
	catalogFingerprint := reg.ToolStateFingerprint()
	if catalogFingerprint == initialFingerprint {
		t.Fatal("catalog change retained the old published fingerprint")
	}

	bodyChanged := `---
name: X
description: changed catalog description
---
Different runtime instructions.
`
	if err := os.WriteFile(filepath.Join(userDir, "x", "SKILL.md"), []byte(bodyChanged), 0o600); err != nil {
		t.Fatal(err)
	}
	reg.ReloadLocalSources(dirs)
	expectSkillRegistryChange(t, changes)
	if reg.ToolStateFingerprint() == catalogFingerprint {
		t.Fatal("SKILL.md body change did not update the invocation binding fingerprint")
	}
}

func expectSkillRegistryChange(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Skill registry change")
	}
}

func expectNoSkillRegistryChange(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case <-changes:
		t.Fatal("unexpected Skill registry change")
	case <-time.After(50 * time.Millisecond):
	}
}

// ---- Mcp prefix validation tests ----

func TestLoadDirRejectsMcpPrefix(t *testing.T) {
	dir := t.TempDir()

	writeTestSkillPackage(t, dir, "mcp__foo__bar", `---
name: mcp__foo__bar
description: This should be rejected
---
Body.
`)
	writeTestSkillPackage(t, dir, "valid", `---
name: valid
description: A valid skill
---
Body.
`)

	loaded := LoadDir(dir, SourceProject)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills (one rejected as unavailable), got %d", len(loaded))
	}

	rejected := loaded["mcp-foo-bar"]
	if rejected == nil {
		t.Fatal("expected rejected skill to be present (as unavailable)")
	}
	if rejected.State != StateInvalid {
		t.Errorf("expected rejected skill to be unavailable, got %s", rejected.State)
	}
	if rejected.UnavailableReason == "" {
		t.Error("expected unavailable reason to be set")
	}

	valid := loaded["valid"]
	if valid == nil {
		t.Fatal("expected valid skill to be present")
	}
	if valid.State != StateLoaded {
		t.Errorf("expected valid skill to be available, got %s", valid.State)
	}
}

func TestValidateNameReserved(t *testing.T) {
	// Normal names should pass.
	if err := ValidateNameReserved("my-skill"); err != nil {
		t.Errorf("normal name should pass: %v", err)
	}
	if err := ValidateNameReserved("code-review"); err != nil {
		t.Errorf("normal name should pass: %v", err)
	}

	// mcp__ prefix should be rejected.
	if err := ValidateNameReserved("mcp__foo__bar"); err == nil {
		t.Error("mcp__ prefix should be rejected")
	}
	if err := ValidateNameReserved("mcp__test"); err == nil {
		t.Error("mcp__ prefix should be rejected")
	}
	if err := ValidateNameReserved("plugin__acme-tools__lint"); err == nil {
		t.Error("plugin__ prefix should be rejected")
	}
}

func TestPluginSourceSkillIDsAreNamespaced(t *testing.T) {
	pkg := filepath.Join(t.TempDir(), "packages", "acme.tools")
	skillsRoot := filepath.Join(pkg, "skills")
	writeTestSkillPackage(t, skillsRoot, "lint", "---\nname: lint\ndescription: lint files\n---\nBody")
	loaded := LoadDir(skillsRoot, SourcePlugin)
	def := loaded["plugin__acme-tools__lint"]
	if def == nil || def.Name != "lint" || def.Source != SourcePlugin {
		t.Fatalf("namespaced plugin skill missing: %+v keys=%v", def, loaded)
	}
	if _, exists := loaded["lint"]; exists {
		t.Fatal("bare skill id should not be used for plugin source")
	}
}

// ---- RequiredMcpServers availability tests ----

func TestSkillRegistryMcpRequirementsAreExecutionHints(t *testing.T) {
	reg := NewSkillRegistry(nil)

	projectDir := t.TempDir()
	writeTestSkillPackage(t, projectDir, "needs-mcp", `---
name: needs-mcp
description: Uses MCP server
requiredMcpServers:
  - internal-search
---
Body.
`)
	writeTestSkillPackage(t, projectDir, "no-mcp", `---
name: no-mcp
description: No MCP needed
---
Body.
`)
	writeTestSkillPackage(t, projectDir, "needs-both", `---
name: needs-both
description: Needs both
requiredConnectors:
  - notion
requiredMcpServers:
  - internal-search
---
Body.
`)
	reg.LoadFromSources(map[SourceOrigin]string{SourceProject: projectDir})

	// Initially no MCP servers available.
	avail := reg.ListLoaded()
	if len(avail) != 3 {
		t.Fatalf("expected all packages loaded, got %d", len(avail))
	}

	needsMcp := reg.Get("needs-mcp")
	if needsMcp == nil {
		t.Fatal("expected needs-mcp to exist")
	}
	if needsMcp.State != StateLoaded {
		t.Errorf("expected loaded state, got %s", needsMcp.State)
	}

	needsBoth := reg.Get("needs-both")
	if needsBoth == nil {
		t.Fatal("expected needs-both to exist")
	}
	if needsBoth.State != StateLoaded {
		t.Errorf("expected loaded state, got %s", needsBoth.State)
	}

	// Make MCP server available.
	reg.SetMcpServerAvailable("internal-search", true)
	avail = reg.ListLoaded()
	if len(avail) != 3 {
		t.Fatalf("MCP state must not change package catalog, got %d", len(avail))
	}

	// Make connector available too.
	reg.SetConnectorAvailable("notion", true)
	avail = reg.ListLoaded()
	if len(avail) != 3 {
		t.Fatalf("expected 3 available skills after both, got %d", len(avail))
	}

	// Disconnect MCP server.
	reg.SetMcpServerAvailable("internal-search", false)
	avail = reg.ListLoaded()
	if len(avail) != 3 {
		t.Fatalf("MCP disconnect must not unload local packages, got %d", len(avail))
	}
}

// ---- SetMcpServerAvailable tests ----

func TestSetMcpServerAvailableDoesNotGateLocalSkill(t *testing.T) {
	reg := NewSkillRegistry(nil)

	projectDir := t.TempDir()
	writeTestSkillPackage(t, projectDir, "test", `---
name: Test
description: test
requiredMcpServers:
  - s1
---
Body.
`)
	reg.LoadFromSources(map[SourceOrigin]string{SourceProject: projectDir})

	// The package is loaded even before the provider is ready.
	avail := reg.ListLoaded()
	if len(avail) != 1 {
		t.Fatalf("expected 1 loaded skill, got %d", len(avail))
	}

	// Set available.
	reg.SetMcpServerAvailable("s1", true)
	avail = reg.ListLoaded()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill, got %d", len(avail))
	}

	// Set unavailable again.
	reg.SetMcpServerAvailable("s1", false)
	avail = reg.ListLoaded()
	if len(avail) != 1 {
		t.Fatalf("MCP disconnect must not unload package, got %d", len(avail))
	}
}

func TestSkillRegistryDisabledByPolicyIsDistinctFromInvalid(t *testing.T) {
	reg := NewSkillRegistry(nil)
	dir := t.TempDir()
	writeTestSkillPackage(t, dir, "policy-skill", "---\nname: policy-skill\ndescription: policy\n---\nBody")
	reg.LoadFromSources(map[SourceOrigin]string{SourceUser: dir})
	reg.SetDisabledByPolicy("policy-skill", true, "organization policy")
	got := reg.Get("policy-skill")
	if got == nil || got.State != StateDisabledByPolicy || got.UnavailableReason != "organization policy" {
		t.Fatalf("policy state=%+v", got)
	}
	if len(reg.ListLoaded()) != 0 {
		t.Fatal("disabled package leaked into loaded catalog")
	}
}

// ---- RegisterSkill / UnregisterSkill / UnregisterSkillsByPrefix tests ----

func TestRegisterAndUnregisterSkill(t *testing.T) {
	reg := NewSkillRegistry(nil)

	def := &SkillDefinition{
		ID:                 "mcp__test__my-prompt",
		Name:               "my-prompt",
		Description:        "A test prompt",
		Source:             SourceMcp,
		State:              StateLoaded,
		RequiredMcpServers: []string{"test"},
		McpBinding: &McpBinding{
			ServerName: "test",
			PromptName: "my-prompt",
			Kind:       "prompt",
		},
	}
	reg.SetMcpServerAvailable("test", true)
	reg.RegisterSkill(def)

	got := reg.Get("mcp__test__my-prompt")
	if got == nil {
		t.Fatal("expected skill to be registered")
	}
	if got.Source != SourceMcp {
		t.Errorf("expected source mcp, got %s", got.Source)
	}
	if got.McpBinding == nil {
		t.Fatal("expected McpBinding to be set")
	}
	if got.McpBinding.ServerName != "test" {
		t.Errorf("expected serverName test, got %s", got.McpBinding.ServerName)
	}

	// Unregister.
	reg.UnregisterSkill("mcp__test__my-prompt")
	if reg.Get("mcp__test__my-prompt") != nil {
		t.Fatal("expected skill to be removed")
	}
}

func TestUnregisterSkillsByPrefix(t *testing.T) {
	reg := NewSkillRegistry(nil)

	for _, id := range []string{"mcp__a__p1", "mcp__a__p2", "mcp__b__p1", "other-skill"} {
		reg.RegisterSkill(&SkillDefinition{
			ID:          id,
			Name:        id,
			Description: "test",
			Source:      SourceMcp,
			State:       StateLoaded,
		})
	}

	// Remove all skills for server "a".
	reg.UnregisterSkillsByPrefix("mcp__a__")

	avail := reg.ListLoaded()
	// Should have mcp__b__p1 and other-skill left.
	if len(avail) != 2 {
		t.Fatalf("expected 2 skills remaining, got %d", len(avail))
	}

	ids := make(map[string]bool)
	for _, s := range avail {
		ids[s.ID] = true
	}
	if !ids["mcp__b__p1"] {
		t.Error("expected mcp__b__p1 to remain")
	}
	if !ids["other-skill"] {
		t.Error("expected other-skill to remain")
	}
}

// ---- Prompt to Skill conversion tests ----

func TestPromptToSkill(t *testing.T) {
	prompt := &mcp.Prompt{
		Name:        "client-followup",
		Description: "Generate a client follow-up email",
		Title:       "Client Follow-up",
		Arguments: []*mcp.PromptArgument{
			{Name: "client_id", Description: "The client ID", Required: true},
			{Name: "tone", Description: "Email tone", Required: false},
		},
	}

	skill := NewBridge(nil, nil).promptToSkill("crm", prompt, "notion")

	if skill.ID != "mcp__crm__client-followup" {
		t.Errorf("expected ID mcp__crm__client-followup, got %s", skill.ID)
	}
	if skill.Source != SourceMcp {
		t.Errorf("expected source mcp, got %s", skill.Source)
	}
	if skill.McpBinding == nil {
		t.Fatal("expected McpBinding")
	}
	if skill.McpBinding.Kind != "prompt" {
		t.Errorf("expected kind prompt, got %s", skill.McpBinding.Kind)
	}
	if len(skill.RequiredMcpServers) != 1 || skill.RequiredMcpServers[0] != "crm" {
		t.Errorf("expected requiredMcpServers [crm], got %v", skill.RequiredMcpServers)
	}
	if len(skill.RequiredConnectors) != 1 || skill.RequiredConnectors[0] != "notion" {
		t.Errorf("expected requiredConnectors [notion], got %v", skill.RequiredConnectors)
	}
	if len(skill.Arguments) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(skill.Arguments))
	}
	if skill.Arguments[0].Name != "client_id" {
		t.Errorf("expected arg name client_id, got %s", skill.Arguments[0].Name)
	}
}

func TestPromptToSkillNoConnector(t *testing.T) {
	prompt := &mcp.Prompt{
		Name:        "echo",
		Description: "Echo prompt",
	}

	skill := NewBridge(nil, nil).promptToSkill("builtin", prompt, "")
	if len(skill.RequiredConnectors) != 0 {
		t.Errorf("expected no requiredConnectors for connector-less server, got %v", skill.RequiredConnectors)
	}
}

func TestToKebab(t *testing.T) {
	cases := map[string]string{
		"hello":           "hello",
		"Hello World":     "hello-world",
		"client_followup": "client-followup",
		"weekly-summary":  "weekly-summary",
		"camelCase":       "camel-case",
		"UPPER":           "u-p-p-e-r",
		"  spaces  ":      "spaces",
		"":                "unnamed",
		"____":            "unnamed",
		"a--b":            "a-b",
	}
	for input, want := range cases {
		got := toKebab(input)
		if got != want {
			t.Errorf("toKebab(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResourceSlug(t *testing.T) {
	cases := map[string]string{
		"wiki://prompts/sop-runbook":   "sop-runbook",
		"file:///home/user/docs/guide": "guide",
		"http://example.com/api":       "api",
		"no-scheme-here":               "no-scheme-here",
	}
	for input, want := range cases {
		got := resourceSlug(input)
		if got != want {
			t.Errorf("resourceSlug(%q) = %q, want %q", input, got, want)
		}
	}
}

// ---- McpBinding shallow copy test ----

func TestShallowCopyPreservesMcpBinding(t *testing.T) {
	def := &SkillDefinition{
		ID:          "test",
		Name:        "test",
		Description: "test",
		McpBinding: &McpBinding{
			ServerName: "srv",
			PromptName: "prompt",
			Kind:       "prompt",
		},
		RequiredMcpServers: []string{"srv"},
	}

	copied := shallowCopy(def)
	if copied.McpBinding == nil {
		t.Fatal("expected McpBinding to be copied")
	}
	if copied.McpBinding.ServerName != "srv" {
		t.Errorf("expected serverName srv, got %s", copied.McpBinding.ServerName)
	}
	if len(copied.RequiredMcpServers) != 1 {
		t.Errorf("expected 1 RequiredMcpServers, got %d", len(copied.RequiredMcpServers))
	}

	// Mutate original to ensure deep copy.
	def.McpBinding.ServerName = "changed"
	if copied.McpBinding.ServerName == "changed" {
		t.Error("McpBinding should be deep-copied, not shared")
	}
}
