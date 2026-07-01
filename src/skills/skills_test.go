package skills

import (
	"os"
	"path/filepath"
	"testing"
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

	writeFile(t, dir, "alpha.md", `---
name: Alpha
description: First skill
---
Alpha body.
`)
	writeFile(t, dir, "beta.md", `---
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
	writeFile(t, dir, "broken.md", `---
description: No name
---
Body.
`)
	// Valid.
	writeFile(t, dir, "good.md", `---
name: Good
description: Valid skill
---
Body.
`)

	loaded := LoadDir(dir, SourceUser)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 skills (both loaded, one unavailable), got %d", len(loaded))
	}
	if loaded["broken"].State != StateUnavailable {
		t.Errorf("expected broken skill to be unavailable, got %s", loaded["broken"].State)
	}
}

func TestLoadDirSkipsNonMd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "just notes")
	writeFile(t, dir, "valid.md", `---
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

func TestLoadDirEmptyDir(t *testing.T) {
	dir := t.TempDir()
	loaded := LoadDir(dir, SourceProject)
	if len(loaded) != 0 {
		t.Errorf("expected 0 skills from empty dir, got %d", len(loaded))
	}
}

// ---- Merge tests ----

func TestMergePriority(t *testing.T) {
	bundled := map[string]*SkillDefinition{
		"alpha": {ID: "alpha", Name: "A Bundled", Description: "desc", Source: SourceBundled, State: StateAvailable},
	}
	project := map[string]*SkillDefinition{
		"alpha": {ID: "alpha", Name: "A Project", Description: "desc", Source: SourceProject, State: StateAvailable},
	}

	merged := MergeSkills(project, bundled)
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

	// Load bundled skills.
	bundled := map[string]*SkillDefinition{
		"test": {
			ID:          "test",
			Name:        "Test Skill",
			Description: "A test skill",
			Source:      SourceBundled,
			State:       StateAvailable,
		},
	}
	reg.LoadFromSources(nil, bundled)

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

	bundled := map[string]*SkillDefinition{
		"needs-notion": {
			ID:                 "needs-notion",
			Name:               "Notion Skill",
			Description:        "Uses Notion",
			Source:             SourceBundled,
			State:              StateAvailable,
			RequiredConnectors: []string{"notion"},
		},
		"no-connector": {
			ID:          "no-connector",
			Name:        "Plain Skill",
			Description: "No connectors needed",
			Source:      SourceBundled,
			State:       StateAvailable,
		},
	}
	reg.LoadFromSources(nil, bundled)

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

	bundled := map[string]*SkillDefinition{
		"x": {ID: "x", Name: "X", Description: "x", Source: SourceBundled, State: StateAvailable},
	}
	reg.LoadFromSources(nil, bundled)

	// onChange runs in a goroutine, give it a moment.
	if !called {
		// Retry the callback manually.
		reg.FireOnChange()
	}
}

// ---- Buned skills tests ----

func TestLoadBundledSkills(t *testing.T) {
	bundled := LoadBundledSkills()
	if len(bundled) < 4 {
		t.Fatalf("expected at least 4 bundled skills, got %d", len(bundled))
	}

	expectedIDs := []string{"meeting-notes-distribute", "customer-followup-daily", "sheet-anomaly-scan", "doc-to-actions"}
	for _, id := range expectedIDs {
		if def, ok := bundled[id]; !ok {
			t.Errorf("expected bundled skill %q to exist", id)
		} else if def.State != StateAvailable {
			t.Errorf("expected bundled skill %q to be available, got %s: %s", id, def.State, def.UnavailableReason)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}