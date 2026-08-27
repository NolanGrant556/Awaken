package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"awaken/src/plugins"
	"awaken/src/tool/trash"
)

type pluginTestTrash struct {
	dir string
}

func (a pluginTestTrash) TrashPath(source string) (trash.TrashResult, error) {
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	dest := filepath.Join(a.dir, filepath.Base(filepath.Dir(source))+"-"+filepath.Base(source))
	if err := os.Rename(source, dest); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: source, TrashedPath: dest, Method: "test"}, nil
}

func writeHTTPPluginFixture(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "skills", "lint"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(`{"$schema":"`+plugins.SchemaPluginV1+`","name":"`+name+`","version":"1.0.0","description":"http fixture"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: lint files\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPluginsUnavailableWithoutStore(t *testing.T) {
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPluginsInstallListEnableUninstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := plugins.NewStoreAtWithTrash(home, pluginTestTrash{dir: filepath.Join(t.TempDir(), "trash")})
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "http-plugin")

	body, _ := json.Marshal(map[string]any{
		"source": map[string]any{"kind": "local_path", "path": src},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed []pluginInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed) != 1 || listed[0].Name != "http-plugin" {
		t.Fatalf("list=%s err=%v", rec.Body.String(), err)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/plugins/http-plugin", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rec.Code, rec.Body.String())
	}
	var detail pluginInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.SkillCount != 1 || len(detail.Skills) != 1 || detail.Skills[0].ID != "plugin__http-plugin__lint" {
		t.Fatalf("detail skills=%+v", detail.Skills)
	}
	installed, err := store.Get("http-plugin")
	if err != nil {
		t.Fatal(err)
	}
	recordedDigest := detail.Digest
	if err := os.WriteFile(filepath.Join(installed.Root, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: updated lint files\n---\nUpdated body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commitBody, _ := json.Marshal(map[string]any{"expectedDigest": recordedDigest})
	req = httptest.NewRequest(http.MethodPost, "/v1/plugins/http-plugin/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", rec.Code, rec.Body.String())
	}
	var committed installPluginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &committed); err != nil {
		t.Fatal(err)
	}
	if committed.Plugin.Digest == recordedDigest || committed.Plugin.SkillCount != 1 || len(committed.Plugin.Skills) != 1 || committed.Plugin.Name != "http-plugin" || committed.Plugin.Version != "1.0.0" || committed.Plugin.Source.Path == "" || committed.Message == "" {
		t.Fatalf("commit response=%+v", committed)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/plugins/http-plugin/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale commit status=%d body=%s", rec.Code, rec.Body.String())
	}
	var conflict errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.Error != "PLUGIN_RECORDED_DIGEST_CHANGED" {
		t.Fatalf("stale commit error=%+v", conflict)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/plugins/http-plugin/commit", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing expectedDigest status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/plugins/http-plugin/disable", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/plugins/http-plugin", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPluginCommitRejectsInvalidInstalledPackage(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "invalid-commit-plugin")
	installed, err := store.Install(t.Context(), plugins.InstallRequest{Source: plugins.Source{Kind: plugins.SourceLocalPath, Path: src}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "plugin.json"), []byte(`{"name":"invalid-commit-plugin"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"expectedDigest": installed.Plugin.Digest})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/invalid-commit-plugin/commit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid commit status=%d body=%s", rec.Code, rec.Body.String())
	}
	var failure errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error != "PLUGIN_INVALID" {
		t.Fatalf("invalid commit error=%+v", failure)
	}
}

func TestPluginCommitMapsUnstableInstalledPackageToInvalid(t *testing.T) {
	rec := httptest.NewRecorder()
	pluginHandlers{}.writeStoreError(rec, plugins.ErrSourceChanged)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unstable commit status=%d body=%s", rec.Code, rec.Body.String())
	}
	var failure errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error != "PLUGIN_INVALID" {
		t.Fatalf("unstable commit error=%+v", failure)
	}
}
