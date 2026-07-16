package skillruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/skills"
)

func TestActivateLoadsOnlySkillBodyAndDoesNotPreloadResources(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "deck")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	content := "recipe body"
	if err := os.WriteFile(filepath.Join(skillRoot, "references", "recipes.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: deck
description: slides
version: "2"
toolPolicy:
  allow: [presentation_create, runtime_preflight]
requiredTools: [presentation_create]
requiredCapabilities: [artifact_write]
requiredResources:
  - path: references/recipes.md
    mode: preload
    required: true
    sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
---
Make a deck.
`), 0644); err != nil {
		t.Fatal(err)
	}

	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	act := NewActivator(reg, nil)
	plan, err := act.Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"Bash", "Read", "Write", "artifact_publish"},
		SkipArgRender:  true,
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if len(plan.ResourceReceipts) != 0 {
		t.Fatalf("receipts=%d", len(plan.ResourceReceipts))
	}
	if strings.Contains(plan.MainBody, "recipe body") {
		t.Fatalf("resource was eagerly preloaded: %s", plan.MainBody)
	}
	if !strings.Contains(plan.MainBody, "Make a deck") || !strings.Contains(plan.MainBody, "Resources are not preloaded") {
		t.Fatalf("lazy body context missing: %s", plan.MainBody)
	}
	if strings.Join(plan.EffectiveTools, ",") != "Bash,Read,Write,artifact_publish" {
		t.Fatalf("effective=%v", plan.EffectiveTools)
	}
}

func TestActivateDoesNotFailOnMissingDeclaredResource(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "deck")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: deck
description: slides
requiredResources:
  - path: references/missing.md
    required: true
---
Body
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	plan, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"Read"},
		SkipArgRender:  true,
	})
	if err != nil || plan == nil || !strings.Contains(plan.MainBody, "Body") {
		t.Fatalf("resource readiness gated activation: plan=%+v err=%v", plan, err)
	}
}

func TestActivateImplementationTextIsGuidanceNotConflict(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "pptx")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: pptx
description: legacy
---
Use pip install python-pptx and soffice.
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	def := reg.Get("pptx")
	if def == nil || def.State != skills.StateLoaded {
		t.Fatalf("expected loaded market skill, got %+v", def)
	}
	plan, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "pptx",
		ParentToolPool: []string{"Bash"},
		SkipArgRender:  true,
	})
	if err != nil || !strings.Contains(plan.MainBody, "pip install") || !strings.Contains(plan.MainBody, "soffice") {
		t.Fatalf("implementation guidance did not activate: plan=%+v err=%v", plan, err)
	}
}

func TestActivateDoesNotProbeRequiredRuntimeAtCatalogBoundary(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "deck")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: deck
description: slides
requiredRuntimeComponents: [python]
---
Body
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})

	plan, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"Read"},
		SkipArgRender:  true,
	})
	if err != nil {
		t.Fatalf("runtime declarations must not gate Skill activation: %v", err)
	}
	if plan == nil || plan.CompatibilityStatus != CompatibilityOK {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestActivateExplicitEmptyParentPoolDoesNotOpenGlobalRegistry(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "empty-parent")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: empty-parent
description: empty parent must stay empty
---
Body
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	plan, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "empty-parent",
		ParentToolPool: []string{},
		SkipArgRender:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.EffectiveTools == nil || len(plan.EffectiveTools) != 0 {
		t.Fatalf("explicit empty parent expanded: %v", plan.EffectiveTools)
	}
}
