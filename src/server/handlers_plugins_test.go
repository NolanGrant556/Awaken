package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/plugins"
	"awaken/src/plugins/ir"
	awruntime "awaken/src/runtime"
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

func writePluginTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
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

func TestPluginHookReceiptsUsesEmptyArrayWhenNoReceiptsExist(t *testing.T) {
	runService := awruntime.NewRunService(nil, nil)
	router := NewRouterWithScheduledTasks(runService, nil, nil, nil, nil, nil, "", nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/hookify/hooks/"+ir.EncodeLogicalID("hook/hooks.json#fixture")+"/receipts", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Items     []json.RawMessage `json:"items"`
		Retention string            `json:"retention"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Items == nil || len(response.Items) != 0 || response.Retention != "memory_only" {
		t.Fatalf("response=%s", rec.Body.String())
	}
}

func TestPluginValidateDoesNotInstall(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "validate-plugin")

	body, _ := json.Marshal(map[string]any{
		"source": map[string]any{"kind": "local_path", "path": src},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", rec.Code, rec.Body.String())
	}
	var validation plugins.ValidationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &validation); err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || validation.Name != "validate-plugin" || validation.SkillCount != 1 || validation.Digest == "" {
		t.Fatalf("validation=%+v", validation)
	}
	if validation.Diagnostics == nil {
		t.Fatalf("diagnostics must be a non-null array: %s", rec.Body.String())
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("validation installed plugins: %+v", listed)
	}
	if _, err := os.Stat(validation.TargetRoot); !os.IsNotExist(err) {
		t.Fatalf("validation created live package: %v", err)
	}
}

func TestPluginValidateReportsInvalidAgentSkillOptionalField(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "invalid-skill-field")
	if err := os.WriteFile(filepath.Join(src, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: lint files\nmetadata:\n  version: 2\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	validation := validateHTTPPlugin(t, router, src)
	if !validation.Valid || validation.SkillCount != 0 {
		t.Fatalf("invalid Skill must be isolated from the installable plugin: %+v", validation)
	}
	found := false
	for _, diagnostic := range validation.Diagnostics {
		if diagnostic.Code == "plugin_skill_invalid" && diagnostic.Severity == "warning" && strings.Contains(diagnostic.Message, "metadata[\"version\"] must be a string") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing optional-field diagnostic: %+v", validation.Diagnostics)
	}
}

func TestPluginInstallAcceptsValidatedDigest(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "digest-pinned-plugin")

	validation := validateHTTPPlugin(t, router, src)
	body, _ := json.Marshal(map[string]any{
		"source":         map[string]any{"kind": "local_path", "path": src},
		"expectedDigest": validation.Digest,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("install status=%d body=%s", rec.Code, rec.Body.String())
	}
	var installed installPluginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	if installed.Plugin.Name != "digest-pinned-plugin" || installed.Plugin.Digest != validation.Digest {
		t.Fatalf("installed=%+v validation=%+v", installed, validation)
	}
}

func TestPluginComponentListOmitsBodiesAndExecutionSpecs(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "safe-component-list")
	if _, err := store.Install(t.Context(), plugins.InstallRequest{
		Source: plugins.Source{Kind: plugins.SourceLocalPath, Path: src},
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/safe-component-list/components", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("components status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"Body", "promptTemplate", "rawFrontmatter", "allowedTools", `"command"`, `"args"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("component list leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"logicalId"`) || !strings.Contains(body, `"mappingState"`) {
		t.Fatalf("component list omitted safe summary fields: %s", body)
	}
	if !strings.Contains(body, `"catalogId":"plugin__safe-component-list__lint"`) {
		t.Fatalf("component list omitted exact Plugin Skill catalog identity: %s", body)
	}
}

func TestPluginComponentDetailOmitsUndeclaredAgentLimits(t *testing.T) {
	properties := map[string]any{
		"agent": map[string]any{"maxTurns": 0, "timeoutSeconds": 0, "model": "fast"},
	}
	applyEffectiveComponentBudget(ir.Component{Agent: &ir.AgentSpec{}}, properties)
	agentProperties := properties["agent"].(map[string]any)
	if _, exists := agentProperties["maxTurns"]; exists {
		t.Fatalf("raw maxTurns remained visible: %#v", properties)
	}
	if _, exists := agentProperties["timeoutSeconds"]; exists {
		t.Fatalf("raw timeoutSeconds remained visible: %#v", properties)
	}
	if _, exists := properties["runtimeBudget"]; exists {
		t.Fatalf("Plugin Agent must not expose a host-only runtime budget: %#v", properties)
	}
}

func TestPluginComponentDetailPreservesDeclaredAgentLimits(t *testing.T) {
	properties := map[string]any{
		"agent": map[string]any{"maxTurns": 99, "timeoutSeconds": 3600, "model": "fast"},
	}
	applyEffectiveComponentBudget(ir.Component{Agent: &ir.AgentSpec{MaxTurns: 99, TimeoutSeconds: 3600}}, properties)
	agentProperties := properties["agent"].(map[string]any)
	if agentProperties["declaredMaxTurns"] != 99 || agentProperties["declaredTimeoutSeconds"] != 3600 {
		t.Fatalf("declared limits were not preserved: %#v", properties)
	}
	if _, exists := properties["runtimeBudget"]; exists {
		t.Fatalf("unexpected Plugin Agent runtime budget: %#v", properties)
	}
}

func TestPluginComponentDetailShowsVendorCompatibleHookTimeout(t *testing.T) {
	properties := map[string]any{
		"hook": map[string]any{"timeoutSeconds": 180},
	}
	applyEffectiveComponentBudget(ir.Component{Hook: &ir.HookSpec{SourceEvent: "SessionStart", TimeoutSeconds: 180}}, properties)
	if _, exists := properties["hook"].(map[string]any)["timeoutSeconds"]; exists {
		t.Fatalf("raw Hook timeout remained duplicated: %#v", properties)
	}
	budget := properties["runtimeBudget"].(map[string]any)
	if budget["timeoutSeconds"] != float64(180) {
		t.Fatalf("effective Hook timeout=%#v", budget)
	}
}

func TestPluginMarketplaceRoutesAreNotRegistered(t *testing.T) {
	store := plugins.NewStoreAt(t.TempDir())
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	req := httptest.NewRequest(http.MethodGet, "/v1/plugin-marketplaces", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removed Marketplace route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPluginSkillForkRouteIsNotRegistered(t *testing.T) {
	store := plugins.NewStoreAt(t.TempDir())
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/example/components/skill/example/fork", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removed Plugin Skill fork route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPluginInstallRejectsSourceChangedAfterValidation(t *testing.T) {
	home := t.TempDir()
	store := plugins.NewStoreAt(home)
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "changed-source-plugin")

	validation := validateHTTPPlugin(t, router, src)
	if err := os.WriteFile(filepath.Join(src, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: changed files\n---\nChanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"source":         map[string]any{"kind": "local_path", "path": src},
		"expectedDigest": validation.Digest,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/install", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("changed source status=%d body=%s", rec.Code, rec.Body.String())
	}
	var failure errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error != "PLUGIN_SOURCE_CHANGED" {
		t.Fatalf("changed source error=%+v", failure)
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("changed source installed plugins: %+v", listed)
	}
}

func validateHTTPPlugin(t *testing.T, router http.Handler, sourcePath string) plugins.ValidationResult {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"source": map[string]any{"kind": "local_path", "path": sourcePath},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result plugins.ValidationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
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

func TestPluginHandlerMapsStandaloneSkillPackageForDesktopRouting(t *testing.T) {
	rec := httptest.NewRecorder()
	pluginHandlers{}.writeStoreError(rec, plugins.ErrSkillPackageSelected)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("standalone Skill status=%d body=%s", rec.Code, rec.Body.String())
	}
	var failure errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error != "SKILL_PACKAGE_SELECTED" {
		t.Fatalf("standalone Skill error=%+v", failure)
	}
}

func TestPluginVariableErrorsKeepTheirDomainCode(t *testing.T) {
	for _, code := range []string{
		"VARIABLE_FORBIDDEN_POSITION",
		"VARIABLE_MISSING",
		"VARIABLE_REQUIRED",
		"VARIABLE_TYPE_INVALID",
		"VARIABLE_RANGE_INVALID",
		"VARIABLE_VALUE_NOT_ALLOWED",
		"VARIABLE_SECRET_SAVE_FAILED",
	} {
		rec := httptest.NewRecorder()
		pluginHandlers{}.writeStoreError(rec, errors.New(code+": fixture"))
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error":"`+code+`"`) {
			t.Fatalf("%s response=%d %s", code, rec.Code, rec.Body.String())
		}
	}
}

func TestPluginConfigurationUsesEmptyArrayForPluginWithoutVariables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := plugins.NewStoreAtWithTrash(home, pluginTestTrash{dir: filepath.Join(t.TempDir(), "trash")})
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "no-variables")
	if _, err := store.Install(t.Context(), plugins.InstallRequest{Source: plugins.Source{Kind: plugins.SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/plugins/no-variables/configuration", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("configuration status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Fields json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if string(response.Fields) != "[]" {
		t.Fatalf("fields must be an array, body=%s", rec.Body.String())
	}
}

func TestPluginMCPSourceRouteReturnsRawTextAndSavesThroughDigestCAS(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := plugins.NewStoreAtWithTrash(home, pluginTestTrash{dir: filepath.Join(t.TempDir(), "trash")})
	router := NewRouterWithScheduledTasks(nil, nil, nil, nil, nil, nil, "", nil, nil, nil, store)
	src := t.TempDir()
	writeHTTPPluginFixture(t, src, "source-route")
	originalSource := `{
  "$schema": "` + plugins.SchemaMCPV1 + `",
  "mcpServers": {
    "local": {"type": "stdio", "command": "npx", "args": ["--token", "literal-value"]}
  }
}`
	writePluginTestFile(t, filepath.Join(src, "mcp.json"), originalSource)
	installed, err := store.Install(t.Context(), plugins.InstallRequest{Source: plugins.Source{Kind: plugins.SourceLocalPath, Path: src}})
	if err != nil {
		t.Fatal(err)
	}
	var component plugins.ComponentState
	for _, state := range installed.Plugin.ComponentState {
		if state.Component.Kind == ir.KindMCPServer {
			component = state
			break
		}
	}
	if component.Component.LogicalID == "" {
		t.Fatal("installed fixture has no MCP component")
	}
	componentPath := "/v1/plugins/source-route/components/mcp_server/" + component.Component.LogicalIDEncoded

	get := httptest.NewRequest(http.MethodGet, componentPath, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get source status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	var detail struct {
		Source         string `json:"source"`
		SourceEditable bool   `json:"sourceEditable"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.SourceEditable || detail.Source != originalSource || strings.Contains(detail.Source, "<configured>") {
		t.Fatalf("detail=%+v body=%s", detail, getRec.Body.String())
	}

	unchangedBody, _ := json.Marshal(map[string]any{"expectedDigest": installed.Plugin.Digest, "source": originalSource})
	unchangedPut := httptest.NewRequest(http.MethodPut, componentPath+"/source", bytes.NewReader(unchangedBody))
	unchangedPut.Header.Set("Content-Type", "application/json")
	unchangedRec := httptest.NewRecorder()
	router.ServeHTTP(unchangedRec, unchangedPut)
	if unchangedRec.Code != http.StatusOK {
		t.Fatalf("unchanged source status=%d body=%s", unchangedRec.Code, unchangedRec.Body.String())
	}
	var unchangedResponse struct {
		Plugin struct {
			Digest string `json:"digest"`
		} `json:"plugin"`
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(unchangedRec.Body.Bytes(), &unchangedResponse); err != nil {
		t.Fatal(err)
	}
	if unchangedResponse.Changed || unchangedResponse.Plugin.Digest != installed.Plugin.Digest {
		t.Fatalf("unchanged response=%s", unchangedRec.Body.String())
	}

	editedSource := strings.Replace(originalSource, `"command": "npx"`, `"command": "node"`, 1)
	body, _ := json.Marshal(map[string]any{"expectedDigest": installed.Plugin.Digest, "source": editedSource})
	put := httptest.NewRequest(http.MethodPut, componentPath+"/source", bytes.NewReader(body))
	put.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, put)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put source status=%d body=%s", putRec.Code, putRec.Body.String())
	}
	var response struct {
		Plugin struct {
			Digest string `json:"digest"`
		} `json:"plugin"`
		Revision uint64 `json:"revision"`
		Changed  bool   `json:"changed"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Changed || response.Plugin.Digest == "" || response.Plugin.Digest == installed.Plugin.Digest || response.Revision == 0 {
		t.Fatalf("response=%s", putRec.Body.String())
	}
	written, err := os.ReadFile(filepath.Join(installed.Plugin.Root, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != editedSource {
		t.Fatalf("written source=%s", written)
	}

	invalidBody, _ := json.Marshal(map[string]any{"expectedDigest": response.Plugin.Digest, "source": `{not-json`})
	invalidPut := httptest.NewRequest(http.MethodPut, componentPath+"/source", bytes.NewReader(invalidBody))
	invalidPut.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	router.ServeHTTP(invalidRec, invalidPut)
	if invalidRec.Code != http.StatusBadRequest || !strings.Contains(invalidRec.Body.String(), `"error":"MCP_SOURCE_INVALID"`) {
		t.Fatalf("invalid source status=%d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
	stillWritten, err := os.ReadFile(filepath.Join(installed.Plugin.Root, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stillWritten) != editedSource {
		t.Fatalf("invalid source changed live file=%s", stillWritten)
	}
}
