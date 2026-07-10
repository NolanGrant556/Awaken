package skillruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/skills"
	"awaken/src/tool"
)

type runtimePreflightStub struct {
	payload string
	err     error
}

func (s runtimePreflightStub) Name() string        { return "runtime_preflight" }
func (s runtimePreflightStub) Description() string { return "test runtime readiness" }
func (s runtimePreflightStub) Parameters() []tool.ToolParameter {
	return nil
}
func (s runtimePreflightStub) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        s.Name(),
		DisplayName: "Runtime Preflight",
		Source:      tool.SourceBuiltin,
		Description: s.Description(),
	}
}
func (s runtimePreflightStub) Execute(context.Context, string) (string, error) {
	return s.payload, s.err
}

func installRuntimePreflightStub(t *testing.T, stub tool.Tool) {
	t.Helper()
	previous, previousErr := tool.GetTool("runtime_preflight")
	if err := tool.RegisterToolForce(stub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if previousErr == nil {
			_ = tool.RegisterToolForce(previous)
			return
		}
		tool.UnregisterTool("runtime_preflight")
	})
}

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

func TestActivateRequiredRuntimeUsesRegisteredPreflightAndFailsClosed(t *testing.T) {
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

	installRuntimePreflightStub(t, runtimePreflightStub{payload: `{
  "ready": false,
  "components": [
    {"name": "python", "ok": true},
    {"name": "libreoffice", "ok": false}
  ]
}`})
	plan, err := NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"file_read"},
		SkipArgRender:  true,
	})
	if err != nil {
		t.Fatalf("python should be ready even when another component is missing: %v", err)
	}
	if plan == nil || plan.CompatibilityStatus != CompatibilityOK {
		t.Fatalf("plan=%+v", plan)
	}

	installRuntimePreflightStub(t, runtimePreflightStub{payload: `{"ready":false,"components":[{"name":"python","ok":false}]}`})
	plan, err = NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"file_read"},
		SkipArgRender:  true,
	})
	if err == nil || !strings.Contains(err.Error(), "skill_required_runtime_missing") {
		t.Fatalf("expected runtime failure, plan=%+v err=%v", plan, err)
	}

	installRuntimePreflightStub(t, runtimePreflightStub{err: errors.New("probe unavailable")})
	_, err = NewActivator(reg, nil).Activate(ActivationRequest{
		SkillID:        "deck",
		ParentToolPool: []string{"file_read"},
		SkipArgRender:  true,
	})
	if err == nil || !strings.Contains(err.Error(), "skill_required_runtime_missing") {
		t.Fatalf("probe error must fail closed: %v", err)
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
