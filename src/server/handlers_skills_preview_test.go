package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/skills"
)

func TestSkillListAcceptsOrderedProjectFolderRoots(t *testing.T) {
	home := t.TempDir()
	primary := filepath.Join(t.TempDir(), "primary")
	secondary := filepath.Join(t.TempDir(), "secondary")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(skills.EnvManagedSkillDirs, filepath.Join(t.TempDir(), "managed"))
	t.Setenv(skills.EnvAdditionalSkillDirs, "")
	t.Cleanup(func() { skills.SetPluginSkillRoots(nil) })
	skills.SetPluginSkillRoots(nil)
	t.Setenv(skills.EnvBundledSkillDirs, "")
	t.Setenv(skills.EnvLegacyCommandDirs, "")
	for _, fixture := range []struct {
		root, id, description string
	}{
		{primary, "shared", "primary winner"},
		{secondary, "shared", "secondary shadow"},
		{secondary, "secondary-only", "secondary only"},
	} {
		packageRoot := filepath.Join(fixture.root, ".awaken", "skills", fixture.id)
		if err := os.MkdirAll(packageRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		raw := "---\nname: " + fixture.id + "\ndescription: " + fixture.description + "\n---\nBody"
		if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	handler := skillHandlers{registry: skills.NewSkillRegistry(nil)}
	query := url.Values{}
	query.Add("projectRoot", primary)
	query.Add("projectRoot", secondary)
	recorder := httptest.NewRecorder()
	handler.handleListSkills(recorder, httptest.NewRequest(http.MethodGet, "/v1/skills?"+query.Encode(), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	loaded := map[string]skillInfo{}
	secondaryShadowed := 0
	for _, info := range listed {
		if info.State == string(skills.StateLoaded) {
			loaded[info.ID] = info
		}
		if info.ID == "shared" && info.State == string(skills.StateShadowed) && info.ProjectPath == secondary {
			secondaryShadowed++
		}
	}
	if loaded["shared"].ProjectPath != primary || loaded["secondary-only"].ProjectPath != secondary {
		t.Fatalf("multi-folder project provenance is wrong: %+v", loaded)
	}
	if secondaryShadowed != 1 {
		t.Fatalf("expected one shadowed secondary duplicate, got %d", secondaryShadowed)
	}
}

func TestSkillPreviewHandlerReadsExactCatalogEntry(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(skills.EnvAdditionalSkillDirs, "")
	t.Cleanup(func() { skills.SetPluginSkillRoots(nil) })
	skills.SetPluginSkillRoots(nil)
	t.Setenv(skills.EnvBundledSkillDirs, "")
	t.Setenv(skills.EnvLegacyCommandDirs, "")

	writeSkill := func(root, body string) {
		t.Helper()
		packageRoot := filepath.Join(root, "shared")
		if err := os.MkdirAll(packageRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		raw := "---\nname: shared\ndescription: preview test\ncustom: preserved\n---\n\n# " + body + "\n"
		if err := os.WriteFile(filepath.Join(packageRoot, "SKILL.md"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill(filepath.Join(home, ".awaken", "skills"), "User body")
	writeSkill(filepath.Join(workspace, ".awaken", "skills"), "Project body")

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(skills.LocalSourceDirs(workspace))
	handler := skillHandlers{registry: registry, workspaceRoot: workspace}

	listRecorder := httptest.NewRecorder()
	handler.handleListSkills(listRecorder, httptest.NewRequest(http.MethodGet, "/v1/skills?projectRoot="+workspace, nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	entryIDs := map[string]string{}
	entryStates := map[string]string{}
	for _, info := range listed {
		if info.ID == "shared" {
			entryIDs[info.SourceOrigin] = info.CatalogEntryID
			entryStates[info.SourceOrigin] = info.State
		}
	}
	if entryIDs["project"] == "" || entryIDs["user"] == "" || entryIDs["project"] == entryIDs["user"] {
		t.Fatalf("expected unique project/user identities, got %#v", entryIDs)
	}
	if entryStates["project"] != string(skills.StateLoaded) || entryStates["user"] != string(skills.StateShadowed) {
		t.Fatalf("expected project winner and retained user shadow, got %#v", entryStates)
	}

	for source, expectedHeading := range map[string]string{"project": "\n# Project body\n", "user": "\n# User body\n"} {
		requestBody, err := json.Marshal(previewSkillRequest{CatalogEntryID: entryIDs[source], ProjectRoot: workspace})
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler.handlePreviewSkill(recorder, httptest.NewRequest(http.MethodPost, "/v1/skills/preview", bytes.NewReader(requestBody)))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s preview status=%d body=%s", source, recorder.Code, recorder.Body.String())
		}
		var preview previewSkillResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.FrontmatterRaw != "---\nname: shared\ndescription: preview test\ncustom: preserved\n---" {
			t.Fatalf("%s frontmatter=%q", source, preview.FrontmatterRaw)
		}
		if preview.BodyMarkdown != expectedHeading {
			t.Fatalf("%s body=%q", source, preview.BodyMarkdown)
		}
		if len(preview.Resources) != 1 || preview.Resources[0].Path != "SKILL.md" || preview.Resources[0].Kind != "file" || preview.ResourcesTruncated {
			t.Fatalf("%s package resources=%+v truncated=%v", source, preview.Resources, preview.ResourcesTruncated)
		}
	}
}

func TestSkillPreviewHandlerRejectsUnknownIdentity(t *testing.T) {
	registry := skills.NewSkillRegistry(nil)
	handler := skillHandlers{registry: registry, workspaceRoot: t.TempDir()}
	body, err := json.Marshal(previewSkillRequest{CatalogEntryID: "not-a-real-entry"})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.handlePreviewSkill(recorder, httptest.NewRequest(http.MethodPost, "/v1/skills/preview", bytes.NewReader(body)))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSkillListExposesBundledPreviewIdentity(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	bundledRoot := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(skills.EnvAdditionalSkillDirs, "")
	t.Cleanup(func() { skills.SetPluginSkillRoots(nil) })
	skills.SetPluginSkillRoots(nil)
	t.Setenv(skills.EnvBundledSkillDirs, bundledRoot)
	t.Setenv(skills.EnvLegacyCommandDirs, "")

	packageRoot := filepath.Join(bundledRoot, "bundled-preview-enabled")
	if err := os.MkdirAll(packageRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(packageRoot, "SKILL.md"),
		[]byte("---\nname: bundled-preview-enabled\ndescription: bundled test\n---\n# Bundled\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	registry := skills.NewSkillRegistry(nil)
	registry.LoadFromSources(skills.LocalSourceDirs(workspace))
	handler := skillHandlers{registry: registry, workspaceRoot: workspace}
	recorder := httptest.NewRecorder()
	handler.handleListSkills(recorder, httptest.NewRequest(http.MethodGet, "/v1/skills", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var listed []skillInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var entryID string
	for _, info := range listed {
		if info.ID != "bundled-preview-enabled" {
			continue
		}
		if info.SourceOrigin != string(skills.SourceBundled) || info.CatalogEntryID == "" {
			t.Fatalf("bundled Skill must expose a preview identity: %#v", info)
		}
		entryID = info.CatalogEntryID
		break
	}
	if entryID == "" {
		t.Fatal("expected bundled Skill in list")
	}

	requestBody, err := json.Marshal(previewSkillRequest{CatalogEntryID: entryID})
	if err != nil {
		t.Fatal(err)
	}
	previewRecorder := httptest.NewRecorder()
	handler.handlePreviewSkill(previewRecorder, httptest.NewRequest(http.MethodPost, "/v1/skills/preview", bytes.NewReader(requestBody)))
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var preview previewSkillResponse
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.BodyMarkdown != "# Bundled\n" {
		t.Fatalf("unexpected bundled preview body: %q", preview.BodyMarkdown)
	}
	if len(preview.Resources) != 1 || preview.Resources[0].Path != "SKILL.md" {
		t.Fatalf("unexpected bundled preview resources: %+v", preview.Resources)
	}
}
