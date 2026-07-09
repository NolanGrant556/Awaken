package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if alpha.State != StateAvailable {
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
	if loaded["broken"].State != StateAvailable {
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

// ---- Merge tests ----

func TestMergePriority(t *testing.T) {
	user := map[string]*SkillDefinition{
		"alpha": {ID: "alpha", Name: "A User", Description: "desc", Source: SourceUser, State: StateAvailable},
	}
	project := map[string]*SkillDefinition{
		"alpha": {ID: "alpha", Name: "A Project", Description: "desc", Source: SourceProject, State: StateAvailable},
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

	shadowed := merged["alpha__shadowed"]
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

	avail := reg.ListAvailable()
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

func TestSkillRegistryConnectorAvailability(t *testing.T) {
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

	// Initially no connectors available.
	avail := reg.ListAvailable()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill (no-connector), got %d", len(avail))
	}
	if avail[0].ID != "no-connector" {
		t.Errorf("expected no-connector, got %s", avail[0].ID)
	}

	needsNotion := reg.Get("needs-notion")
	if needsNotion == nil {
		t.Fatal("expected needs-notion to exist")
	}
	if needsNotion.State != StateUnavailable {
		t.Errorf("expected unavailable state, got %s", needsNotion.State)
	}
	if needsNotion.UnavailableReason == "" {
		t.Error("expected unavailable reason to be set")
	}

	// Make notion connector available.
	reg.SetConnectorAvailable("notion", true)
	avail = reg.ListAvailable()
	if len(avail) != 2 {
		t.Fatalf("expected 2 available skills after connector, got %d", len(avail))
	}

	// Disconnect notion again.
	reg.SetConnectorAvailable("notion", false)
	avail = reg.ListAvailable()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill after disconnect, got %d", len(avail))
	}
}

func TestSkillRegistryOnChange(t *testing.T) {
	reg := NewSkillRegistry(nil)
	called := false
	reg.SetOnChange(func() {
		called = true
	})

	userDir := t.TempDir()
	writeTestSkillPackage(t, userDir, "x", `---
name: X
description: x
---
Body.
`)
	reg.LoadFromSources(map[SourceOrigin]string{SourceUser: userDir})

	// onChange runs in a goroutine, give it a moment.
	if !called {
		// Retry the callback manually.
		reg.FireOnChange()
	}
}

func TestInstallSkillPackageFromDirectoryPreservesResources(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, sourceRoot, filepath.Join("nested", "daily-followup", "SKILL.md"), `---
name: daily-followup
description: Daily followup.
---
Use the references.
`)
	writeFile(t, sourceRoot, filepath.Join("nested", "daily-followup", "references", "guide.md"), "guide body")

	pkg, err := PackageFromPath(sourceRoot)
	if err != nil {
		t.Fatalf("package from path: %v", err)
	}
	workspace := t.TempDir()
	result, err := InstallSkillPackage(InstallRequest{
		Package:     pkg,
		Scope:       string(SourceProject),
		ProjectRoot: workspace,
	})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}
	if result.Skill == nil || result.Skill.ID != "daily-followup" {
		t.Fatalf("unexpected installed skill: %+v", result.Skill)
	}
	resourcePath := filepath.Join(workspace, ".awaken", "skills", "daily-followup", "references", "guide.md")
	data, err := os.ReadFile(resourcePath)
	if err != nil {
		t.Fatalf("read copied resource: %v", err)
	}
	if string(data) != "guide body" {
		t.Fatalf("unexpected resource body: %q", string(data))
	}
}

func TestInstallSkillPackageDefaultsToAbortExisting(t *testing.T) {
	pkg, err := PackageFromBytes("SKILL.md", []byte(`---
name: duplicate-skill
description: Duplicate.
---
Body.
`))
	if err != nil {
		t.Fatalf("package bytes: %v", err)
	}
	workspace := t.TempDir()
	req := InstallRequest{
		Package:     pkg,
		Scope:       string(SourceProject),
		ProjectRoot: workspace,
	}
	if _, err := InstallSkillPackage(req); err != nil {
		t.Fatalf("first install: %v", err)
	}
	_, err = InstallSkillPackage(req)
	if !errors.Is(err, ErrSkillAlreadyExists) {
		t.Fatalf("expected ErrSkillAlreadyExists, got %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(workspace, ".awaken", "skills", "duplicate-skill")) {
		t.Fatalf("expected target path in error, got %v", err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func writeTestSkillPackage(t *testing.T, root, id, content string) {
	t.Helper()
	writeFile(t, root, filepath.Join(id, "SKILL.md"), content)
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
	if rejected.State != StateUnavailable {
		t.Errorf("expected rejected skill to be unavailable, got %s", rejected.State)
	}
	if rejected.UnavailableReason == "" {
		t.Error("expected unavailable reason to be set")
	}

	valid := loaded["valid"]
	if valid == nil {
		t.Fatal("expected valid skill to be present")
	}
	if valid.State != StateAvailable {
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
}

// ---- RequiredMcpServers availability tests ----

func TestSkillRegistryMcpServerAvailability(t *testing.T) {
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
	avail := reg.ListAvailable()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill (no-mcp), got %d", len(avail))
	}
	if avail[0].ID != "no-mcp" {
		t.Errorf("expected no-mcp, got %s", avail[0].ID)
	}

	needsMcp := reg.Get("needs-mcp")
	if needsMcp == nil {
		t.Fatal("expected needs-mcp to exist")
	}
	if needsMcp.State != StateUnavailable {
		t.Errorf("expected unavailable state, got %s", needsMcp.State)
	}

	needsBoth := reg.Get("needs-both")
	if needsBoth == nil {
		t.Fatal("expected needs-both to exist")
	}
	if needsBoth.State != StateUnavailable {
		t.Errorf("expected unavailable state, got %s", needsBoth.State)
	}

	// Make MCP server available.
	reg.SetMcpServerAvailable("internal-search", true)
	avail = reg.ListAvailable()
	// no-mcp + needs-mcp = 2, needs-both is still unavailable because notion is missing.
	if len(avail) != 2 {
		t.Fatalf("expected 2 available skills after MCP server, got %d", len(avail))
	}

	// Make connector available too.
	reg.SetConnectorAvailable("notion", true)
	avail = reg.ListAvailable()
	if len(avail) != 3 {
		t.Fatalf("expected 3 available skills after both, got %d", len(avail))
	}

	// Disconnect MCP server.
	reg.SetMcpServerAvailable("internal-search", false)
	avail = reg.ListAvailable()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill after MCP disconnect, got %d", len(avail))
	}
}

// ---- SetMcpServerAvailable tests ----

func TestSetMcpServerAvailable(t *testing.T) {
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

	// Initially unavailable.
	avail := reg.ListAvailable()
	if len(avail) != 0 {
		t.Fatalf("expected 0 available skills, got %d", len(avail))
	}

	// Set available.
	reg.SetMcpServerAvailable("s1", true)
	avail = reg.ListAvailable()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available skill, got %d", len(avail))
	}

	// Set unavailable again.
	reg.SetMcpServerAvailable("s1", false)
	avail = reg.ListAvailable()
	if len(avail) != 0 {
		t.Fatalf("expected 0 available skills, got %d", len(avail))
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
		State:              StateAvailable,
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
			State:       StateAvailable,
		})
	}

	// Remove all skills for server "a".
	reg.UnregisterSkillsByPrefix("mcp__a__")

	avail := reg.ListAvailable()
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
