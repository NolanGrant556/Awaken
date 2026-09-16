package server

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"awaken/src/managedenv"
	awruntime "awaken/src/runtime"
)

func TestEnvironmentControlAuthenticationAndStreamingReuse(t *testing.T) {
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for name, value := range map[string]string{"font.otf": "OTTO controlled resource fixture", "LICENSE": "test fixture"} {
		f, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data.Bytes())
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data.Bytes()) }))
	defer assets.Close()
	catalog := managedenv.Catalog{SchemaVersion: managedenv.SchemaVersion, Components: map[string]managedenv.Component{"fonts": {Version: "1.0.0", Assets: map[string]managedenv.Asset{managedenv.Target(): {URL: assets.URL, SHA256: hex.EncodeToString(hash[:]), Format: "zip", Resources: map[string]string{"font_regular": "font.otf", "license": "LICENSE"}}}}}, Capabilities: map[string]managedenv.Spec{"fonts": {Components: []string{"fonts"}}}}
	manager, err := managedenv.New(managedenv.Options{Root: t.TempDir(), Catalog: catalog, Client: assets.Client(), AllowFileAssets: true})
	if err != nil {
		t.Fatal(err)
	}
	prior := managedenv.Default()
	managedenv.SetDefault(manager)
	defer func() { managedenv.SetDefault(prior); _ = manager.Close() }()
	router := newRouterInternal(routerConfig{control: &ControlConfig{Identity: &awruntime.RuntimeIdentity{ControlToken: "environment-control-test"}}})
	for _, authorized := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/v1/control/environments/components/lark-cli", nil)
		if authorized {
			req.Header.Set("Authorization", "Bearer environment-control-test")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if !authorized && rec.Code != 401 {
			t.Fatal("unauthenticated ownership inspection", rec.Code)
		}
		if authorized && (rec.Code != 200 || !strings.Contains(rec.Body.String(), `"installation":null`)) {
			t.Fatal("missing installation projection", rec.Code, rec.Body.String())
		}
	}
	request := func(body string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/control/environments/prepare", strings.NewReader(body))
		if authorized {
			req.Header.Set("Authorization", "Bearer environment-control-test")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := request(`{"capability":"fonts"}`, false); rec.Code != 401 {
		t.Fatalf("unauthorized: %d", rec.Code)
	}
	if len(manager.Operations()) != 0 {
		t.Fatal("unauthenticated preparation started")
	}
	for _, body := range []string{`{"capability":"fonts","directory":"/tmp/evil"}`, `{"capability":"fonts"} {}`} {
		if rec := request(body, true); rec.Code != 400 {
			t.Fatalf("unsafe body accepted: %d %s", rec.Code, rec.Body.String())
		}
	}
	instance := ""
	for i := 0; i < 2; i++ {
		rec := request(`{"capability":"fonts"}`, true)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/x-ndjson" {
			t.Fatalf("bad stream: %d %s", rec.Code, rec.Body.String())
		}
		lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
		var result struct {
			Type    string             `json:"type"`
			Binding managedenv.Binding `json:"binding"`
		}
		if err = json.Unmarshal([]byte(lines[len(lines)-1]), &result); err != nil {
			t.Fatal(err)
		}
		if result.Type != "ready" || result.Binding.Environment["AWAKEN_FONTS_DIR"] == "" {
			t.Fatalf("not ready: %s", rec.Body.String())
		}
		if i == 0 {
			instance = result.Binding.InstanceID
		} else if instance != result.Binding.InstanceID || !result.Binding.Reused {
			t.Fatal("control reinstalled instead of reusing")
		}
	}
	if len(manager.Operations()) != 1 {
		t.Fatal("inspection or repeated request started extra operations")
	}
}

func TestOfficeControlIsAuthenticatedStrictAndReadOnly(t *testing.T) {
	catalog, err := managedenv.BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	catalog.OfficeBundle.Assets = nil
	manager, err := managedenv.New(managedenv.Options{Root: t.TempDir(), Catalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	prior := managedenv.Default()
	managedenv.SetDefault(manager)
	defer func() { managedenv.SetDefault(prior); _ = manager.Close() }()
	router := newRouterInternal(routerConfig{control: &ControlConfig{Identity: &awruntime.RuntimeIdentity{ControlToken: "office-test"}}})
	send := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/v1/control/environments/office"+path, strings.NewReader(body))
		if auth {
			req.Header.Set("Authorization", "Bearer office-test")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"", "/prepare", "/repair"} {
		method := "POST"
		if path == "" {
			method = "GET"
		}
		if rec := send(method, path, `{}`, false); rec.Code != 401 {
			t.Fatal("unprotected office route", path, rec.Code)
		}
	}
	if rec := send("GET", "", "", true); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"state":"unavailable"`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
	for _, entry := range [][2]string{{"/prepare", `null`}, {"/prepare", `{"expected_instance_id":"old"}`}, {"/prepare", `{"directory":"/tmp"}`}, {"/prepare", `{} {}`}, {"/repair", `{}`}, {"/repair", `{"expected_instance_id":"old","command":"pip"}`}} {
		if rec := send("POST", entry[0], entry[1], true); rec.Code != 400 {
			t.Fatal("accepted unsupported request", entry, rec.Body.String())
		}
	}
	if rec := send("POST", "/prepare", `{}`, true); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"type":"error"`) || !strings.Contains(rec.Body.String(), "environment_source_unavailable") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if len(manager.Operations()) != 0 {
		t.Fatal("status/invalid request started installation")
	}
}
