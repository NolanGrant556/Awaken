package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
