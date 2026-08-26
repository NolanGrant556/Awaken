package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAgentSkillEntrypoint(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		directory string
		want      string
	}{
		{
			name:      "valid",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\n---\nBody\n",
		},
		{
			name:      "missing declared name",
			directory: "order-flow",
			raw:       "---\ndescription: Run an order flow when requested.\n---\nBody\n",
			want:      "name is required",
		},
		{
			name:      "missing declared description",
			directory: "order-flow",
			raw:       "---\nname: order-flow\n---\nBody fallback must not count.\n",
			want:      "description is required",
		},
		{
			name:      "directory mismatch",
			directory: "order-flow",
			raw:       "---\nname: another-flow\ndescription: Run another flow when requested.\n---\nBody\n",
			want:      "must match parent directory",
		},
		{
			name:      "invalid name",
			directory: "order_flow",
			raw:       "---\nname: order_flow\ndescription: Run an order flow when requested.\n---\nBody\n",
			want:      "lowercase ASCII",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAgentSkillEntrypoint(tc.raw, tc.directory)
			if tc.want == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("err=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestPluginSourceSkipsNonConformingAgentSkillButProjectKeepsCompatibility(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "order-flow")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: different-name\n---\nBody-derived description.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var plugin *SkillDefinition
	// A plain temporary root has no plugin package identity; locate the only
	// returned entry without depending on its namespaced map key.
	for _, def := range LoadDir(root, SourcePlugin) {
		plugin = def
		break
	}
	if plugin == nil || plugin.State != StateInvalid || !strings.Contains(plugin.UnavailableReason, "must match parent directory") {
		t.Fatalf("plugin source did not reject nonconforming Skill: %+v", plugin)
	}

	project := LoadDir(root, SourceProject)["different-name"]
	if project == nil || project.State != StateLoaded || project.Description == "" {
		t.Fatalf("project compatibility source should remain permissive: %+v", project)
	}
}

func TestValidateSkillPackageRequiresDeclaredPortableMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "order-flow")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: order-flow\n---\nBody fallback.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := PackageFromPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSkillPackage(pkg); err == nil || !strings.Contains(err.Error(), "description is required") {
		t.Fatalf("portable metadata omission passed install validation: %v", err)
	}
}
