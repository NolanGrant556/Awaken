package skills

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/tool/trash"
)

func TestSkillPackageRejectsPluginTreesAndArchives(t *testing.T) {
	pluginRoot := t.TempDir()
	writeFile(t, pluginRoot, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"example-plugin"}`)
	writeFile(t, pluginRoot, filepath.Join("skills", "example-command", "SKILL.md"), "---\nname: example-command\ndescription: Plugin component.\n---\nBody.\n")
	if _, err := PackageFromPath(pluginRoot); !errors.Is(err, ErrPluginPackageSelected) {
		t.Fatalf("plugin directory error=%v, want ErrPluginPackageSelected", err)
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range map[string]string{
		"example-plugin/.cursor-plugin/plugin.json": `{"name":"example-plugin"}`,
		"example-plugin/SKILL.md":                   "---\nname: example-command\ndescription: Plugin root Skill.\n---\nBody.\n",
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := PackageFromBytes("example-plugin.zip", archive.Bytes()); !errors.Is(err, ErrPluginPackageSelected) {
		t.Fatalf("plugin archive error=%v, want ErrPluginPackageSelected", err)
	}

	var mcpOnlyArchive bytes.Buffer
	mcpWriter := zip.NewWriter(&mcpOnlyArchive)
	manifest, err := mcpWriter.Create("mcp-only/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write([]byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"mcp-only"}`)); err != nil {
		t.Fatal(err)
	}
	if err := mcpWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := PackageFromBytes("mcp-only.zip", mcpOnlyArchive.Bytes()); !errors.Is(err, ErrPluginPackageSelected) {
		t.Fatalf("MCP-only plugin archive error=%v, want ErrPluginPackageSelected", err)
	}

	var ordinarySkill bytes.Buffer
	ordinaryWriter := zip.NewWriter(&ordinarySkill)
	for name, content := range map[string]string{
		"ordinary/SKILL.md":           "---\nname: ordinary\ndescription: Ordinary Skill.\n---\nBody.\n",
		"ordinary/assets/plugin.json": `{"example":true}`,
	} {
		entry, err := ordinaryWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := ordinaryWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := PackageFromBytes("ordinary.zip", ordinarySkill.Bytes()); err != nil {
		t.Fatalf("ordinary Skill resource was misclassified as Plugin: %v", err)
	}
}

type installTrashAdapter struct {
	dir  string
	fail bool
}

func (a installTrashAdapter) TrashPath(source string) (trash.TrashResult, error) {
	if a.fail {
		return trash.TrashResult{}, errors.New("Trash unavailable")
	}
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	destination := filepath.Join(a.dir, filepath.Base(source))
	if err := os.Rename(source, destination); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: source, TrashedPath: destination, Method: "test_rename"}, nil
}

func TestInstallSkillPackageFromDirectoryPreservesResources(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, sourceRoot, filepath.Join("nested", "daily-followup", "SKILL.md"), `---
name: daily-followup
description: Daily followup.
---
Use the references.
`)
	writeFile(t, sourceRoot, filepath.Join("nested", "daily-followup", "references", "guide.md"), "guide body")

	pkg, err := PackageFromPath(sourceRoot)
	if err != nil {
		t.Fatalf("package from path: %v", err)
	}
	workspace := t.TempDir()
	result, err := InstallSkillPackage(InstallRequest{
		Package:     pkg,
		Scope:       string(SourceProject),
		ProjectRoot: workspace,
	})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}
	if result.Skill == nil || result.Skill.ID != "daily-followup" {
		t.Fatalf("unexpected installed skill: %+v", result.Skill)
	}
	resourcePath := filepath.Join(workspace, ".awaken", "skills", "daily-followup", "references", "guide.md")
	data, err := os.ReadFile(resourcePath)
	if err != nil {
		t.Fatalf("read copied resource: %v", err)
	}
	if string(data) != "guide body" {
		t.Fatalf("unexpected resource body: %q", string(data))
	}
}

func TestInstallSkillPackageDefaultsToAbortExisting(t *testing.T) {
	pkg, err := PackageFromBytes("SKILL.md", []byte(`---
name: duplicate-skill
description: Duplicate.
---
Body.
`))
	if err != nil {
		t.Fatalf("package bytes: %v", err)
	}
	workspace := t.TempDir()
	req := InstallRequest{
		Package:     pkg,
		Scope:       string(SourceProject),
		ProjectRoot: workspace,
	}
	if _, err := InstallSkillPackage(req); err != nil {
		t.Fatalf("first install: %v", err)
	}
	_, err = InstallSkillPackage(req)
	if !errors.Is(err, ErrSkillAlreadyExists) {
		t.Fatalf("expected ErrSkillAlreadyExists, got %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(workspace, ".awaken", "skills", "duplicate-skill")) {
		t.Fatalf("expected target path in error, got %v", err)
	}
}

func TestInstallSkillPackageReplaceMovesPreviousPackageToTrash(t *testing.T) {
	oldPackage, err := PackageFromBytes("SKILL.md", []byte("---\nname: replaceable\ndescription: Old.\n---\nOld body.\n"))
	if err != nil {
		t.Fatal(err)
	}
	newPackage, err := PackageFromBytes("SKILL.md", []byte("---\nname: replaceable\ndescription: New.\n---\nNew body.\n"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if _, err := InstallSkillPackage(InstallRequest{Package: oldPackage, Scope: string(SourceProject), ProjectRoot: workspace}); err != nil {
		t.Fatalf("initial install: %v", err)
	}
	result, err := InstallSkillPackage(InstallRequest{
		Package: newPackage, Scope: string(SourceProject), ProjectRoot: workspace,
		OverwritePolicy: "replace", TrashAdapter: installTrashAdapter{dir: filepath.Join(t.TempDir(), "Trash")},
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if result.PreviousPackageTrash == nil || result.PreviousPackageTrash.TrashedPath == "" {
		t.Fatalf("replacement did not report previous package Trash receipt: %+v", result)
	}
	current, err := os.ReadFile(filepath.Join(result.Path, "SKILL.md"))
	if err != nil || !strings.Contains(string(current), "New body.") {
		t.Fatalf("new package not installed: err=%v content=%q", err, current)
	}
	previous, err := os.ReadFile(filepath.Join(result.PreviousPackageTrash.TrashedPath, "SKILL.md"))
	if err != nil || !strings.Contains(string(previous), "Old body.") {
		t.Fatalf("old package not recoverable from Trash: err=%v content=%q", err, previous)
	}
}

func TestInstallSkillPackageReplaceRollsBackWhenTrashUnavailable(t *testing.T) {
	oldPackage, err := PackageFromBytes("SKILL.md", []byte("---\nname: rollback\ndescription: Old.\n---\nOld body.\n"))
	if err != nil {
		t.Fatal(err)
	}
	newPackage, err := PackageFromBytes("SKILL.md", []byte("---\nname: rollback\ndescription: New.\n---\nNew body.\n"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	initial, err := InstallSkillPackage(InstallRequest{Package: oldPackage, Scope: string(SourceProject), ProjectRoot: workspace})
	if err != nil {
		t.Fatal(err)
	}
	_, err = InstallSkillPackage(InstallRequest{
		Package: newPackage, Scope: string(SourceProject), ProjectRoot: workspace,
		OverwritePolicy: "replace", TrashAdapter: installTrashAdapter{fail: true},
	})
	if !errors.Is(err, ErrSkillReplaceTrashUnavailable) {
		t.Fatalf("expected typed Trash failure, got %v", err)
	}
	current, readErr := os.ReadFile(filepath.Join(initial.Path, "SKILL.md"))
	if readErr != nil || !strings.Contains(string(current), "Old body.") || strings.Contains(string(current), "New body.") {
		t.Fatalf("failed replacement did not restore old package: err=%v content=%q", readErr, current)
	}
	entries, readErr := os.ReadDir(filepath.Dir(initial.Path))
	if readErr != nil || len(entries) != 1 || entries[0].Name() != "rollback" {
		t.Fatalf("failed replacement left staging or backup state: entries=%v err=%v", entries, readErr)
	}
}

func TestDeleteSkillPackageMovesProjectPackageToTrash(t *testing.T) {
	workspace := t.TempDir()
	packageDir := filepath.Join(workspace, ".awaken", "skills", "folder-name")
	writeFile(t, packageDir, "SKILL.md", `---
name: deletable-skill
description: Delete this package.
---
Body.
`)
	trashDir := filepath.Join(t.TempDir(), "Trash")
	result, err := DeleteSkillPackage(DeleteRequest{
		ID:           "deletable-skill",
		Source:       SourceProject,
		ProjectRoot:  workspace,
		TrashAdapter: installTrashAdapter{dir: trashDir},
	})
	if err != nil {
		t.Fatalf("delete skill: %v", err)
	}
	if result.OriginalPath != packageDir || result.Trash.TrashedPath == "" {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	if _, err := os.Stat(packageDir); !os.IsNotExist(err) {
		t.Fatalf("package still exists after delete: %v", err)
	}
	trashedSkill, err := os.ReadFile(filepath.Join(result.Trash.TrashedPath, "SKILL.md"))
	if err != nil || !strings.Contains(string(trashedSkill), "deletable-skill") {
		t.Fatalf("trashed package is not recoverable: err=%v content=%q", err, trashedSkill)
	}
}

func TestDeleteSkillPackageTrashFailureLeavesPackageUntouched(t *testing.T) {
	workspace := t.TempDir()
	packageDir := filepath.Join(workspace, ".awaken", "skills", "kept-skill")
	writeFile(t, packageDir, "SKILL.md", `---
name: kept-skill
description: Keep this package when Trash fails.
---
Body.
`)
	_, err := DeleteSkillPackage(DeleteRequest{
		ID:           "kept-skill",
		Source:       SourceProject,
		ProjectRoot:  workspace,
		TrashAdapter: installTrashAdapter{fail: true},
	})
	if !errors.Is(err, ErrSkillDeleteTrashUnavailable) {
		t.Fatalf("expected Trash failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(packageDir, "SKILL.md")); statErr != nil {
		t.Fatalf("failed delete removed or damaged source package: %v", statErr)
	}
}

func TestDeleteSkillPackageRejectsNonOwnedSource(t *testing.T) {
	for _, source := range []SourceOrigin{SourceMcp, SourceBundled} {
		t.Run(string(source), func(t *testing.T) {
			_, err := DeleteSkillPackage(DeleteRequest{ID: "dynamic-skill", Source: source})
			if !errors.Is(err, ErrSkillDeleteSourceUnsupported) {
				t.Fatalf("expected unsupported source failure, got %v", err)
			}
		})
	}
}

func TestInstallSkillPackageRootedWriteRejectsSymlinkedInstallDirectory(t *testing.T) {
	pkg, err := PackageFromBytes("SKILL.md", []byte(`---
name: rooted-install
description: Rooted install.
---
Body.
`))
	if err != nil {
		t.Fatalf("package bytes: %v", err)
	}
	for _, linkRelative := range []string{".awaken", filepath.Join(".awaken", "skills")} {
		t.Run(filepath.ToSlash(linkRelative), func(t *testing.T) {
			workspace := t.TempDir()
			outside := t.TempDir()
			linkPath := filepath.Join(workspace, linkRelative)
			if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
				t.Fatalf("mkdir link parent: %v", err)
			}
			if err := os.Symlink(outside, linkPath); err != nil {
				t.Skipf("symlink unavailable on this platform: %v", err)
			}
			_, err := InstallSkillPackage(InstallRequest{
				Package: pkg, Scope: string(SourceProject), ProjectRoot: workspace,
			})
			if err == nil {
				t.Fatal("expected rooted installer to reject symlink escape")
			}
			if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
				t.Fatalf("rooted installer wrote outside project: entries=%v err=%v installErr=%v", entries, readErr, err)
			}
		})
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func writeTestSkillPackage(t *testing.T, root, id, content string) {
	t.Helper()
	writeFile(t, root, filepath.Join(id, "SKILL.md"), content)
}
