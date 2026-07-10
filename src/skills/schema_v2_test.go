package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatterToolPolicyV2(t *testing.T) {
	raw := `---
name: deck
description: make slides
version: "1.2.0"
toolPolicy:
  mode: intersect
  allow:
    - presentation_create
    - runtime_preflight
  deny:
    - shell
requiredTools:
  - presentation_create
requiredCapabilities:
  - artifact_write
requiredRuntimeComponents:
  - libreoffice
requiredResources:
  - path: references/recipes.md
    mode: preload
    required: true
    maxBytes: 1024
---
Body.
`
	def, err := ParseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if def.Version != "1.2.0" {
		t.Fatalf("version=%q", def.Version)
	}
	policy := def.EffectiveToolPolicy()
	if policy.Mode != ToolPolicyIntersect || len(policy.Allow) != 2 || policy.Deny[0] != "shell" {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	if len(def.RequiredTools) != 1 || def.RequiredCapabilities[0] != "artifact_write" {
		t.Fatalf("required fields missing: %+v", def)
	}
	if len(def.RequiredResources) != 1 || def.RequiredResources[0].Path != "references/recipes.md" {
		t.Fatalf("resources=%+v", def.RequiredResources)
	}
}

func TestAllowedToolsMapsToToolPolicyAllowWithWarning(t *testing.T) {
	raw := `---
name: legacy
description: legacy allow list
allowedTools:
  - file_read
  - shell
---
Body.
`
	def, err := ParseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	policy := def.EffectiveToolPolicy()
	if len(policy.Allow) != 2 || policy.Allow[0] != "file_read" {
		t.Fatalf("expected allowedTools mapped into allow, got %+v", policy.Allow)
	}
	if len(def.DeprecatedWarnings) == 0 {
		t.Fatal("expected deprecated warning for allowedTools")
	}
}

func TestLegacyPPTConflictMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "pptx-legacy")
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	skill := `---
name: pptx-legacy
description: legacy ppt skill
---
Run pip install python-pptx then generate.py and call soffice.
`
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "generate.py"), []byte("print('hi')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	loaded := LoadDir(dir, SourceUser)
	def := loaded["pptx-legacy"]
	if def == nil {
		t.Fatal("skill not loaded")
	}
	if def.State != StateUnavailable {
		t.Fatalf("expected unavailable, got %s", def.State)
	}
	if def.UnavailableCode != ErrorCodePlatformContractConflict {
		t.Fatalf("code=%q reason=%q", def.UnavailableCode, def.UnavailableReason)
	}
	if !strings.Contains(def.UnavailableReason, "pip") && !strings.Contains(def.UnavailableReason, "generate.py") {
		t.Fatalf("reason should mention conflict patterns: %q", def.UnavailableReason)
	}
	// Must not rewrite user files.
	raw, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "pip install") {
		t.Fatal("user skill file was rewritten")
	}
}
