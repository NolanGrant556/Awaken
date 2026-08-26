package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSkillPreviewPreservesFrontmatterAndBody(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "preview-skill")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nname: preview-skill\nunknown-field: keep-me\n---\n\n# Preview\n\n- one\n- two\n"
	path := filepath.Join(packageRoot, "SKILL.md")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(packageRoot, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "references", "guide.md"), []byte("guide"), 0o644); err != nil {
		t.Fatal(err)
	}
	defs := LoadDir(root, SourceUser)
	def := defs["preview-skill"]
	if def == nil {
		t.Fatal("expected discovered skill")
	}

	preview, err := ReadSkillPreview(def)
	if err != nil {
		t.Fatal(err)
	}
	if preview.FrontmatterRaw != "---\nname: preview-skill\nunknown-field: keep-me\n---" {
		t.Fatalf("unexpected frontmatter: %q", preview.FrontmatterRaw)
	}
	if preview.BodyMarkdown != "\n# Preview\n\n- one\n- two\n" {
		t.Fatalf("unexpected body: %q", preview.BodyMarkdown)
	}
	if len(preview.Resources) != 3 || preview.Resources[0].Path != "SKILL.md" || preview.Resources[1].Path != "references" || preview.Resources[1].Kind != "directory" || preview.Resources[2].Path != "references/guide.md" {
		t.Fatalf("unexpected package resources: %+v", preview.Resources)
	}
	if preview.ResourcesTruncated {
		t.Fatal("small package inventory should not be truncated")
	}
	if CatalogEntryID(def) == "" || strings.Contains(CatalogEntryID(def), packageRoot) {
		t.Fatalf("catalog entry identity must be non-empty and opaque: %q", CatalogEntryID(def))
	}
}

func TestCatalogEntryIDExcludesSourcesWithoutPreviewEntryPoints(t *testing.T) {
	for _, source := range []SourceOrigin{SourceMcp} {
		if id := CatalogEntryID(&SkillDefinition{
			ID: "not-previewable", Source: source, SkillFilePath: "/not/a/local/capability/SKILL.md",
		}); id != "" {
			t.Fatalf("%s skill must not expose a local preview identity: %q", source, id)
		}
	}
	if id := CatalogEntryID(&SkillDefinition{
		ID: "missing-entrypoint", Source: SourceBundled, SkillFilePath: "",
	}); id != "" {
		t.Fatalf("skill without a SKILL.md path must not expose a preview identity: %q", id)
	}
}

func TestReadSkillPreviewSupportsFlatLegacyMarkdown(t *testing.T) {
	root := t.TempDir()
	raw := "# Legacy command\n\nRead everything.\n"
	path := filepath.Join(root, "review.md")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	defs := LoadLegacyCommandsDir(root)
	def := defs["review"]
	if def == nil {
		t.Fatal("expected flat legacy definition")
	}

	preview, err := ReadSkillPreview(def)
	if err != nil {
		t.Fatal(err)
	}
	if preview.FrontmatterRaw != "" || preview.BodyMarkdown != raw {
		t.Fatalf("unexpected legacy preview: %#v", preview)
	}
	if len(preview.Resources) != 1 || preview.Resources[0].Path != "review.md" {
		t.Fatalf("legacy preview should expose its one entrypoint: %+v", preview.Resources)
	}
}

func TestReadSkillPreviewSupportsEveryLocalPackageSource(t *testing.T) {
	for _, source := range []SourceOrigin{
		SourceManaged,
		SourceUser,
		SourceProject,
		SourceAdditional,
		SourcePlugin,
		SourceBundled,
	} {
		t.Run(string(source), func(t *testing.T) {
			root := t.TempDir()
			packageRoot := filepath.Join(root, "source-preview")
			if err := os.MkdirAll(packageRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			raw := "---\nname: source-preview\ndescription: source test\n---\n# " + string(source) + "\n"
			if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			loaded := LoadDir(root, source)
			id := "source-preview"
			if source == SourcePlugin {
				id = namespacedSkillID(source, root, "source-preview")
			}
			def := loaded[id]
			if def == nil {
				t.Fatal("expected discovered skill")
			}
			preview, err := ReadSkillPreview(def)
			if err != nil {
				t.Fatal(err)
			}
			if preview.FrontmatterRaw != "---\nname: source-preview\ndescription: source test\n---" || preview.BodyMarkdown != "# "+string(source)+"\n" {
				t.Fatalf("unexpected preview: %#v", preview)
			}
		})
	}
}

func TestReadSkillPreviewRejectsRootReplacementAndEscapingEntrypoint(t *testing.T) {
	root := t.TempDir()
	packageRoot := filepath.Join(root, "replace-me")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(packageRoot, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: replace-me\ndescription: test\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	def := LoadDir(root, SourceUser)["replace-me"]
	if def == nil {
		t.Fatal("expected discovered skill")
	}
	original := packageRoot + "-original"
	if err := os.Rename(packageRoot, original); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(t.TempDir(), "replacement")
	if err := os.MkdirAll(replacement, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "SKILL.md"), []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, packageRoot); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if _, err := ReadSkillPreview(def); !errors.Is(err, ErrSkillPreviewPathDenied) {
		t.Fatalf("expected root replacement rejection, got %v", err)
	}

	escapeRoot := t.TempDir()
	escapePackage := filepath.Join(escapeRoot, "escape")
	if err := os.MkdirAll(escapePackage, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolvedEscapeRoot, _ := filepath.EvalSymlinks(escapeRoot)
	resolvedEscapePackage, _ := filepath.EvalSymlinks(escapePackage)
	escapeDef := &SkillDefinition{
		ID: "escape", Source: SourceUser, RootDir: escapePackage,
		SourceRoot: resolvedEscapeRoot, ResolvedRootDir: resolvedEscapePackage,
		SkillFilePath: outside,
	}
	if _, err := ReadSkillPreview(escapeDef); !errors.Is(err, ErrSkillPreviewPathDenied) {
		t.Fatalf("expected escaping entrypoint rejection, got %v", err)
	}
}

func TestReadSkillPreviewRejectsOversizeAndNonUTF8(t *testing.T) {
	writeAndLoad := func(t *testing.T, id string, data []byte) *SkillDefinition {
		t.Helper()
		root := t.TempDir()
		packageRoot := filepath.Join(root, id)
		if err := os.MkdirAll(packageRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(packageRoot, "SKILL.md")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		resolvedRoot, _ := filepath.EvalSymlinks(root)
		resolvedPackage, _ := filepath.EvalSymlinks(packageRoot)
		return &SkillDefinition{
			ID: id, Source: SourceUser, RootDir: packageRoot, SourceRoot: resolvedRoot,
			ResolvedRootDir: resolvedPackage, SkillFilePath: path,
		}
	}

	oversize := writeAndLoad(t, "oversize", []byte(strings.Repeat("x", int(MaxSkillPreviewSize)+1)))
	if _, err := ReadSkillPreview(oversize); !errors.Is(err, ErrSkillPreviewTooLarge) {
		t.Fatalf("expected oversize error, got %v", err)
	}
	binary := writeAndLoad(t, "binary", []byte{0xff, 0xfe, 0xfd})
	if _, err := ReadSkillPreview(binary); !errors.Is(err, ErrSkillPreviewNotUTF8) {
		t.Fatalf("expected UTF-8 error, got %v", err)
	}
}

func TestReadSkillPreviewRejectsMissingAndNonRegularEntrypoints(t *testing.T) {
	makeDefinition := func(t *testing.T) (*SkillDefinition, string) {
		t.Helper()
		root := t.TempDir()
		packageRoot := filepath.Join(root, "entrypoint")
		if err := os.MkdirAll(packageRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(packageRoot, "SKILL.md")
		if err := os.WriteFile(path, []byte("---\nname: entrypoint\ndescription: test\n---\nbody"), 0o644); err != nil {
			t.Fatal(err)
		}
		def := LoadDir(root, SourceUser)["entrypoint"]
		if def == nil {
			t.Fatal("expected discovered skill")
		}
		return def, path
	}

	t.Run("missing", func(t *testing.T) {
		def, path := makeDefinition(t)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadSkillPreview(def); !errors.Is(err, ErrSkillPreviewUnavailable) {
			t.Fatalf("expected missing entrypoint rejection, got %v", err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		def, path := makeDefinition(t)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadSkillPreview(def); !errors.Is(err, ErrSkillPreviewPathDenied) {
			t.Fatalf("expected non-regular entrypoint rejection, got %v", err)
		}
	})
}

func TestSplitSkillPreviewKeepsMalformedFrontmatterVisible(t *testing.T) {
	frontmatter, body := SplitSkillPreview("---\nname: [broken\n---\n# Still readable\n")
	if frontmatter != "---\nname: [broken\n---" || body != "# Still readable\n" {
		t.Fatalf("unexpected split: frontmatter=%q body=%q", frontmatter, body)
	}
	frontmatter, body = SplitSkillPreview("# Plain Markdown\n")
	if frontmatter != "" || body != "# Plain Markdown\n" {
		t.Fatalf("unexpected flat Markdown split: frontmatter=%q body=%q", frontmatter, body)
	}
}
