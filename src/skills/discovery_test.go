package skills

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalSourceDirsProductionContract(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	project := filepath.Join(base, "project")
	for _, dir := range []string{home, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	managedA := filepath.Join(base, "managed-a")
	managedB := filepath.Join(base, "managed-b")
	additionalA := filepath.Join(base, "additional-a")
	additionalB := filepath.Join(base, "additional-b")
	plugin := filepath.Join(base, "plugin-skills")
	legacy := filepath.Join(base, "legacy")
	t.Setenv("HOME", home)
	t.Setenv(EnvManagedSkillDirs, joinSourceDirList([]string{managedA, managedB}))
	t.Setenv(EnvAdditionalSkillDirs, joinSourceDirList([]string{additionalA, additionalB}))
	t.Setenv(EnvPluginSkillDirs, plugin)
	t.Setenv(EnvBundledSkillDirs, "")
	t.Setenv(EnvLegacyCommandDirs, legacy)

	dirs := LocalSourceDirs(project)
	assertPaths := func(source SourceOrigin, want ...string) {
		t.Helper()
		got := splitSourceDirList(dirs[source])
		if len(got) != len(want) {
			t.Fatalf("%s paths=%v want=%v", source, got, want)
		}
		for i := range want {
			if got[i] != filepath.Clean(want[i]) {
				t.Fatalf("%s path[%d]=%q want %q", source, i, got[i], want[i])
			}
		}
	}
	assertPaths(SourceManaged, managedA, managedB)
	assertPaths(SourceProject, filepath.Join(project, ".awaken", "skills"))
	assertPaths(SourceUser, filepath.Join(home, ".awaken", "skills"))
	assertPaths(SourceAdditional, additionalA, additionalB)
	assertPaths(SourcePlugin, plugin)
	assertPaths(SourceLegacy, filepath.Join(project, ".awaken", "commands"), filepath.Join(home, ".awaken", "commands"), legacy)
	if _, exists := dirs[SourceBundled]; exists {
		t.Fatal("bundled source appeared without explicit AWAKEN_BUNDLED_SKILL_DIRS")
	}
}

func TestProductionDiscoveryMultipleRootsPriorityAndRealpathDedupe(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	project := filepath.Join(base, "project")
	additionalA := filepath.Join(base, "additional-a")
	additionalB := filepath.Join(base, "additional-b")
	pluginA := filepath.Join(base, "plugin-a")
	pluginB := filepath.Join(base, "plugin-b")
	bundled := filepath.Join(base, "bundled")
	managed := filepath.Join(base, "managed")
	for _, dir := range []string{home, project, additionalA, additionalB, pluginA, pluginB, bundled, managed} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestSkillPackage(t, additionalA, "shared", "---\nname: shared\ndescription: additional first\n---\nBody")
	writeTestSkillPackage(t, additionalB, "shared", "---\nname: shared\ndescription: additional second\n---\nBody")
	writeTestSkillPackage(t, filepath.Join(project, ".awaken", "skills"), "shared", "---\nname: shared\ndescription: project winner\n---\nBody")
	writeTestSkillPackage(t, pluginA, "plugin-a", "---\nname: plugin-a\ndescription: plugin a\n---\nBody")
	writeTestSkillPackage(t, pluginB, "plugin-b", "---\nname: plugin-b\ndescription: plugin b\n---\nBody")
	writeTestSkillPackage(t, bundled, "bundled-one", "---\nname: bundled-one\ndescription: bundled\n---\nBody")
	writeTestSkillPackage(t, managed, "managed-one", "---\nname: managed-one\ndescription: managed\n---\nBody")
	duplicateLink := filepath.Join(base, "additional-link")
	if runtime.GOOS != "windows" {
		if err := os.Symlink(additionalA, duplicateLink); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv(EnvManagedSkillDirs, managed)
	additionalDirs := []string{additionalA}
	if runtime.GOOS != "windows" {
		additionalDirs = append(additionalDirs, duplicateLink)
	}
	additionalDirs = append(additionalDirs, additionalB)
	t.Setenv(EnvAdditionalSkillDirs, joinSourceDirList(additionalDirs))
	t.Setenv(EnvPluginSkillDirs, joinSourceDirList([]string{pluginA, pluginB}))
	t.Setenv(EnvBundledSkillDirs, bundled)
	t.Setenv(EnvLegacyCommandDirs, "")

	reg := NewSkillRegistry(nil)
	reg.LoadFromSources(LocalSourceDirs(project))
	for id, source := range map[string]SourceOrigin{
		"shared": SourceProject, "plugin-a": SourcePlugin, "plugin-b": SourcePlugin,
		"bundled-one": SourceBundled, "managed-one": SourceManaged,
	} {
		def := reg.Get(id)
		if def == nil || def.State != StateLoaded || def.Source != source {
			t.Fatalf("%s=%+v", id, def)
		}
	}
	if reg.Get("shared").Description != "project winner" {
		t.Fatalf("source priority failed: %+v", reg.Get("shared"))
	}
	loadedShared := 0
	for _, def := range reg.ListAll() {
		if def.ID == "shared" && def.State == StateLoaded {
			loadedShared++
		}
	}
	if loadedShared != 1 {
		t.Fatalf("realpath/source duplicates leaked into loaded catalog: %d", loadedShared)
	}
}

func TestLegacyFlatCommandDiscoveryWithoutFrontmatter(t *testing.T) {
	project := t.TempDir()
	commands := filepath.Join(project, ".awaken", "commands")
	if err := os.MkdirAll(commands, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commands, "review.md"), []byte("# Review changes\n\nInspect $ARGUMENTS carefully."), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvManagedSkillDirs, filepath.Join(t.TempDir(), "none"))
	t.Setenv(EnvAdditionalSkillDirs, "")
	t.Setenv(EnvPluginSkillDirs, "")
	t.Setenv(EnvBundledSkillDirs, "")
	t.Setenv(EnvLegacyCommandDirs, "")
	reg := NewSkillRegistry(nil)
	reg.LoadFromSources(LocalSourceDirs(project))
	def := reg.Get("review")
	if def == nil || def.State != StateLoaded || def.Source != SourceLegacy || def.Description != "Review changes" {
		t.Fatalf("legacy command=%+v", def)
	}
	if filepath.Base(def.SkillFilePath) != "review.md" || def.Body != "" {
		t.Fatalf("legacy command was not catalog-only: %+v", def)
	}
}

func TestLiveUserSkillDirectoryLoadsWhenRequested(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("AWAKEN_LIVE_SKILLS_ROOT"))
	if root == "" {
		t.Skip("set AWAKEN_LIVE_SKILLS_ROOT for read-only live user Skill acceptance")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read live Skill root: %v", err)
	}
	definitions := LoadDir(root, SourceUser)
	seenPackages := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "SKILL.md")); err != nil {
			continue
		}
		seenPackages++
		definition := definitions[entry.Name()]
		if definition == nil {
			t.Errorf("live package %q was not discovered", entry.Name())
			continue
		}
		if definition.State != StateLoaded {
			t.Errorf("live package %q state=%s reason=%s diagnostics=%+v", entry.Name(), definition.State, definition.UnavailableReason, definition.Diagnostics)
		}
		if definition.Body != "" {
			t.Errorf("live package %q eagerly cached SKILL.md body", entry.Name())
		}
	}
	if seenPackages == 0 || len(definitions) != seenPackages {
		t.Fatalf("live Skill discovery count mismatch: packages=%d definitions=%d", seenPackages, len(definitions))
	}
}
