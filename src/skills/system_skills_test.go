package skills

import (
	"path/filepath"
	"runtime"
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
