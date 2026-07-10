package skillruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/skills"
	"awaken/src/tool"
)

func TestCompileEffectiveToolsIntersectAndDeny(t *testing.T) {
	parent := []string{"shell", "file_read", "presentation_create", "runtime_preflight"}
	effective, errs := CompileEffectiveTools(parent, []skills.ToolPolicy{{
		Mode:  skills.ToolPolicyIntersect,
		Allow: []string{"presentation_create", "runtime_preflight", "outside_tool"},
		Deny:  []string{"runtime_preflight"},
	}})
	if len(errs) == 0 {
		t.Fatal("expected outside_tool error")
	}
	if strings.Join(effective, ",") != "presentation_create" {
		t.Fatalf("effective=%v", effective)
	}
}

func TestCompileEffectiveToolsEmptyAllowInheritsParent(t *testing.T) {
	parent := []string{"shell", "file_read"}
	effective, errs := CompileEffectiveTools(parent, []skills.ToolPolicy{{
		Mode: skills.ToolPolicyIntersect,
		Deny: []string{"shell"},
	}})
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if strings.Join(effective, ",") != "file_read" {
		t.Fatalf("effective=%v", effective)
	}
}

func TestActivateRequiredResourceReceipt(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "deck")
	if err := os.MkdirAll(filepath.Join(skillRoot, "references"), 0755); err != nil {
		t.Fatal(err)
	}
	content := "recipe body"
	sum := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(sum[:])
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
    sha256: `+digest+`
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
		ParentToolPool: []string{"presentation_create", "runtime_preflight", "shell"},
		SkipArgRender:  true,
		ToolCapability: func(name string) (tool.CapabilityClass, bool) {
			if name == "presentation_create" {
				return tool.CapabilityArtifactWrite, true
			}
			return "", true
		},
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if len(plan.ResourceReceipts) != 1 {
		t.Fatalf("receipts=%d", len(plan.ResourceReceipts))
	}
	r := plan.ResourceReceipts[0]
	if r.SHA256 != digest || r.Resource != "references/recipes.md" || r.Source != "user" {
		t.Fatalf("bad receipt: %+v", r)
	}
	if !strings.Contains(plan.MainBody, "recipe body") {
		t.Fatalf("main body missing preload resource: %s", plan.MainBody)
	}
	if strings.Join(plan.EffectiveTools, ",") != "presentation_create,runtime_preflight" {
		t.Fatalf("effective=%v", plan.EffectiveTools)
	}
}

func TestActivateFailsClosedOnMissingRequiredResource(t *testing.T) {
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
	_, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"file_read"},
		SkipArgRender:  true,
	})
	if err == nil {
		t.Fatal("expected fail closed")
	}
}

func TestActivateLegacyConflict(t *testing.T) {
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
	if def == nil || def.State != skills.StateUnavailable {
		t.Fatalf("expected unavailable legacy skill, got %+v", def)
	}
	_, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "pptx",
		ParentToolPool: []string{"shell"},
		SkipArgRender:  true,
	})
	var compat *SkillCompatibilityError
	if err == nil {
		t.Fatal("expected conflict")
	}
	if !strings.Contains(err.Error(), skills.ErrorCodePlatformContractConflict) {
		t.Fatalf("err=%v", err)
	}
	_ = compat
}
