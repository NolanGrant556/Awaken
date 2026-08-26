package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBundledSystemSkillsIncludeLoadedCreators(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "apps", "desktop", "resources", "system-skills"))
	loaded := LoadDir(root, SourceBundled)
	if len(loaded) != 8 {
		t.Fatalf("bundled Skill count=%d, want 8: %#v", len(loaded), loaded)
	}
	for _, id := range []string{"skill-creator", "plugin-creator"} {
		def := loaded[id]
		if def == nil || def.State != StateLoaded || def.Source != SourceBundled {
			t.Fatalf("bundled creator %q not loaded: %+v", id, def)
		}
	}
}

func TestBundledCreatorsShipCompleteAuthoringPackages(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "apps", "desktop", "resources", "system-skills"))
	expected := map[string][]string{
		"skill-creator": {
			"SKILL.md",
			"references/package-design.md",
			"scripts/init_skill.py",
			"scripts/validate_skill.py",
		},
		"plugin-creator": {
			"SKILL.md",
			"references/package-authoring.md",
			"scripts/init_plugin.py",
			"scripts/validate_plugin.py",
		},
	}
	for creator, files := range expected {
		for _, rel := range files {
			path := filepath.Join(root, creator, filepath.FromSlash(rel))
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("%s missing authoring resource %s: info=%v err=%v", creator, rel, info, err)
			}
		}
		raw, err := os.ReadFile(filepath.Join(root, creator, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateAgentSkillEntrypoint(string(raw), creator); err != nil {
			t.Fatalf("%s entrypoint is not a strict Agent Skill: %v", creator, err)
		}
		text := string(raw)
		for _, required := range []string{"references/", "scripts/", "assets/", "skill_resource_list"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s does not teach the complete Skill package contract: missing %q", creator, required)
			}
		}
	}
}
