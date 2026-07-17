package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/skills"
)

func TestSkillPreviewHandlerReadsExactCatalogEntry(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(skills.EnvAdditionalSkillDirs, "")
	t.Setenv(skills.EnvPluginSkillDirs, "")
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
	t.Setenv(skills.EnvPluginSkillDirs, "")
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
}
