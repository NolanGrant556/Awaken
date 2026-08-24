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
