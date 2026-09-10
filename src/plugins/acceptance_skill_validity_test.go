package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/plugins/ir"
)

func TestAcceptancePluginSkillValidityMatchesRuntime(t *testing.T) {
	for _, tc := range []struct{ name, manifest, content string }{
		{"claude", ".claude-plugin/plugin.json", `{"name":"sa-invalid-skill"}`},
		{"codex", ".codex-plugin/plugin.json", `{"name":"sa-invalid-skill","skills":"./skills/"}`},
		{"cursor", ".cursor-plugin/plugin.json", `{"name":"sa-invalid-skill","skills":"./skills/"}`},
		{"portable", "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"sa-invalid-skill","version":"1.0.0"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range map[string]string{
				tc.manifest:              tc.content,
				"skills/broken/SKILL.md": "---\nname: different-directory\ndescription: Invalid fixture must remain unavailable.\n---\nRead only the assigned fixture.\n",
				"skills/valid/SKILL.md":  "---\nname: valid\ndescription: Valid sibling stays usable.\n---\nRead only the assigned fixture.\n",
			} {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			validation, err := NewStoreAt(t.TempDir()).Validate(context.Background(), Source{Kind: SourceLocalPath, Path: root})
			if err != nil || !validation.Valid {
				t.Fatalf("invalid Skill should not reject valid sibling package: %+v %v", validation, err)
			}
			var broken, valid *ir.Component
			for i := range validation.Components {
				component := &validation.Components[i]
				if component.SourcePath == "skills/broken/SKILL.md" {
					broken = component
				}
				if component.SourcePath == "skills/valid/SKILL.md" {
					valid = component
				}
			}
			if broken == nil || broken.Validation != "invalid" || broken.Mapping != ir.MappingInvalid || len(broken.Diagnostics) == 0 {
				t.Errorf("runtime-invalid Skill advertised as runnable: %+v", broken)
			}
			if valid == nil || valid.Validation != "valid" || valid.Mapping == ir.MappingInvalid {
				t.Errorf("valid sibling was disabled: %+v", valid)
			}
			if validation.SkillCount != 1 {
				t.Errorf("loadable Skill count=%d, want 1", validation.SkillCount)
			}
		})
	}
}

func TestAcceptanceRootSkillUsesInstalledPluginIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		valid        bool
	}{
		{"valid", "name: sa-root-skill\ndescription: Root Skill.\ncontext: fork\n", true},
		{"directory_mismatch", "name: another-skill\ndescription: Root Skill.\n", false},
		{"invalid_metadata", "name: sa-root-skill\ndescription: Root Skill.\nmetadata:\n  version: 1\n", false},
		{"invalid_runtime_schema", "name: sa-root-skill\ndescription: Root Skill.\ncontext: unknown\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Materialization uses a temporary staging basename. Root Skills
			// must be checked against the final package directory, not staging.
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".cursor-plugin"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".cursor-plugin/plugin.json"), []byte(`{"name":"sa-root-skill"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\n"+tc.fields+"---\nRead the assigned fixture.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			validation, err := NewStoreAt(t.TempDir()).Validate(context.Background(), Source{Kind: SourceLocalPath, Path: root})
			if err != nil || !validation.Valid || len(validation.Components) != 1 {
				t.Fatalf("validate package: %+v %v", validation, err)
			}
			component := validation.Components[0]
			if got := component.Validation == "valid" && component.Mapping != ir.MappingInvalid; got != tc.valid {
				t.Fatalf("valid=%v, want %v: %+v", got, tc.valid, component)
			}
		})
	}
}
