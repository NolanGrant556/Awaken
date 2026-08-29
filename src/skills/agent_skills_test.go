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
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nlicense: Apache-2.0\ncompatibility: Requires git and network access.\nmetadata:\n  author: awaken\nallowed-tools: Bash Read\n---\nBody\n",
		},
		{
			name:      "awaken extension remains compatible",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nsource_type: local_path\n---\nBody\n",
		},
		{
			name:      "missing declared name",
			directory: "order-flow",
			raw:       "---\ndescription: Run an order flow when requested.\n---\nBody\n",
			want:      "name is required",
		},
		{
			name:      "name must be string",
			directory: "order-flow",
			raw:       "---\nname: 3\ndescription: Run an order flow when requested.\n---\nBody\n",
			want:      "name must be a string",
		},
		{
			name:      "missing declared description",
			directory: "order-flow",
			raw:       "---\nname: order-flow\n---\nBody fallback must not count.\n",
			want:      "description is required",
		},
		{
			name:      "description must be string",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: 3\n---\nBody\n",
			want:      "description must be a string",
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
			want:      "lowercase letters",
		},
		{
			name:      "whitespace padded name is rejected",
			directory: "order-flow",
			raw:       "---\nname: \" order-flow \"\ndescription: Run an order flow when requested.\n---\nBody\n",
			want:      "surrounding whitespace",
		},
		{
			name:      "description too long",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: " + strings.Repeat("界", 1025) + "\n---\nBody\n",
			want:      "must not exceed 1024 characters",
		},
		{
			name:      "license must be string",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nlicense:\n  - Apache-2.0\n---\nBody\n",
			want:      "license must be a string",
		},
		{
			name:      "license null is not an omitted field",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nlicense: null\n---\nBody\n",
			want:      "license must be a string",
		},
		{
			name:      "compatibility must be string",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\ncompatibility: 3\n---\nBody\n",
			want:      "compatibility must be a string",
		},
		{
			name:      "compatibility must not be empty",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\ncompatibility: \"   \"\n---\nBody\n",
			want:      "must contain 1-500 characters",
		},
		{
			name:      "compatibility null is not an omitted field",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\ncompatibility: null\n---\nBody\n",
			want:      "compatibility must be a string",
		},
		{
			name:      "compatibility too long",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\ncompatibility: " + strings.Repeat("界", 501) + "\n---\nBody\n",
			want:      "must not exceed 500 characters",
		},
		{
			name:      "metadata must be map",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nmetadata:\n  - author\n---\nBody\n",
			want:      "metadata must be a map",
		},
		{
			name:      "metadata must be string map",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nmetadata:\n  attempts: 2\n---\nBody\n",
			want:      "metadata[\"attempts\"] must be a string",
		},
		{
			name:      "metadata keys must be strings",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nmetadata:\n  1: owner\n---\nBody\n",
			want:      "metadata keys must be strings",
		},
		{
			name:      "metadata null is not an omitted field",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nmetadata: null\n---\nBody\n",
			want:      "metadata must be a map",
		},
		{
			name:      "allowed-tools must be string",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nallowed-tools:\n  - Bash\n---\nBody\n",
			want:      "allowed-tools must be a string",
		},
		{
			name:      "allowed-tools null is not an omitted field",
			directory: "order-flow",
			raw:       "---\nname: order-flow\ndescription: Run an order flow when requested.\nallowed-tools: null\n---\nBody\n",
			want:      "allowed-tools must be a string",
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

func TestParseFrontmatterAllowedToolsSpaceSeparated(t *testing.T) {
	def, err := ParseFrontmatter(`---
name: order-flow
description: Run an order flow when requested.
allowed-tools: Bash Read
---
Body
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(def.AllowedTools) != 2 || def.AllowedTools[0] != "Bash" || def.AllowedTools[1] != "Read" {
		t.Fatalf("allowed-tools were not parsed as a space-separated hint list: %+v", def.AllowedTools)
	}
}

func TestPluginSourceSkipsNonConformingAgentSkillButProjectKeepsCompatibility(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "order-flow")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: order-flow\ndescription: Run an order flow when requested.\nmetadata:\n  attempts: 2\n---\nBody-derived description.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goodRoot := filepath.Join(root, "good-flow")
	if err := os.MkdirAll(goodRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goodRoot, "SKILL.md"), []byte("---\nname: good-flow\ndescription: Run a good flow when requested.\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded := LoadDir(root, SourcePlugin)
	var bad, good *SkillDefinition
	for _, def := range loaded {
		switch {
		case def.State == StateInvalid && strings.Contains(def.UnavailableReason, "metadata[\"attempts\"] must be a string"):
			bad = def
		case def.Name == "good-flow":
			good = def
		}
	}
	if bad == nil || bad.State != StateInvalid || !strings.Contains(bad.UnavailableReason, "metadata[\"attempts\"] must be a string") {
		t.Fatalf("plugin source did not reject nonconforming Skill: %+v", bad)
	}
	if good == nil || good.State != StateLoaded {
		t.Fatalf("plugin source stopped loading siblings after one bad Skill: %+v", good)
	}

	projectRoot := t.TempDir()
	projectSkillRoot := filepath.Join(projectRoot, "different-name")
	if err := os.MkdirAll(projectSkillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectSkillRoot, "SKILL.md"), []byte("---\nname: different-name\ndescription: Project-derived description.\nmetadata:\n  attempts: 2\n---\nBody.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := LoadDir(projectRoot, SourceProject)["different-name"]
	if project == nil || project.State != StateLoaded || project.Description == "" {
		t.Fatalf("project compatibility source should remain permissive: %+v", project)
	}
}

func TestValidateSkillPackageRequiresDeclaredPortableMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "order-flow")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: order-flow\ndescription: Run an order flow when requested.\nmetadata:\n  attempts: 2\n---\nBody fallback.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := PackageFromPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSkillPackage(pkg); err == nil || !strings.Contains(err.Error(), "metadata[\"attempts\"] must be a string") {
		t.Fatalf("portable metadata omission passed install validation: %v", err)
	}
}

func TestValidateSkillPackageRejectsWhitespacePaddedName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "order-flow")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: \" order-flow \"\ndescription: Run an order flow when requested.\n---\nBody.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pkg, err := PackageFromPath(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSkillPackage(pkg); err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("whitespace padded name passed install validation: %v", err)
	}
}
