package server

import (
	"os"
	"path/filepath"
	"testing"

	"awaken/src/skills"
)

func TestSkillInfoHidesDisabledByPolicyDiagnostic(t *testing.T) {
	dir := t.TempDir()
	packageRoot := filepath.Join(dir, "disabled-skill")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: disabled-skill\ndescription: test\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: dir})
	registry.SetDisabledByPolicy("disabled-skill", true, "disabled by user")

	def := registry.Get("disabled-skill")
	if def == nil || def.State != skills.StateDisabledByPolicy {
		t.Fatalf("expected disabled skill, got %+v", def)
	}

	info := skillInfoFromDefinition(def)
	for _, diagnostic := range info.Diagnostics {
		if diagnostic.Code == "skill_disabled_by_policy" {
			t.Fatalf("disabled-by-policy diagnostic should not be exposed: %#v", info.Diagnostics)
		}
	}
}
