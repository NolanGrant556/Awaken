package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"awaken/src/desktopdata"
	"awaken/src/mcpclient"
	"awaken/src/tool/trash"
)

type testTrashAdapter struct {
	dir  string
	fail bool
}

func (a testTrashAdapter) TrashPath(source string) (trash.TrashResult, error) {
	if a.fail {
		return trash.TrashResult{}, errors.New("Trash unavailable")
	}
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return trash.TrashResult{}, err
	}
	destination := filepath.Join(a.dir, filepath.Base(source)+"-"+strings.ReplaceAll(filepath.Base(filepath.Dir(source)), string(filepath.Separator), "_"))
	if _, err := os.Stat(destination); err == nil {
		destination = destination + "-2"
	}
	if err := os.Rename(source, destination); err != nil {
		return trash.TrashResult{}, err
	}
	return trash.TrashResult{OriginalPath: source, TrashedPath: destination, Method: "test_rename"}, nil
}

func testStore(t *testing.T) *Store {
	t.Helper()
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	return NewStoreAtWithTrash(home, testTrashAdapter{dir: filepath.Join(t.TempDir(), "Trash")})
}

func writePluginFixture(t *testing.T, root string, name string, extraMCP bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "skills", "lint"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.json"), []byte(`{
		"$schema":"`+SchemaPluginV1+`",
		"name":"`+name+`",
		"version":"1.0.0",
		"description":"Example plugin"
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: lint files\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if extraMCP {
		if err := os.WriteFile(filepath.Join(root, "mcp.json"), []byte(`{
			"$schema":"`+SchemaMCPV1+`",
			"mcpServers":{
				"local":{"type":"stdio","command":"npx","args":["--data","${PLUGIN_DATA}/cache"]}
			}
		}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewStoreRequiresAwakenHome(t *testing.T) {
	t.Setenv("AWAKEN_HOME", "")
	if _, err := NewStore(); !errors.Is(err, ErrHomeRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestInstallLocalPluginAndList(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "acme.tools", true)
	result, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugin.Name != "acme.tools" || result.Plugin.Manifest.Version != "1.0.0" {
		t.Fatalf("plugin=%+v", result.Plugin)
	}
	if result.Plugin.SkillRoot == "" || len(result.Plugin.MCPServers) != 1 {
		t.Fatalf("components: %+v", result.Plugin)
	}
	if result.Plugin.MCPServers[0].Identity != "plugin__acme.tools__local" {
		t.Fatalf("identity=%q", result.Plugin.MCPServers[0].Identity)
	}
	if !strings.Contains(result.Plugin.MCPServers[0].Config.Stdio.Args[1], filepath.Join("data", "acme.tools", "cache")) {
		t.Fatalf("PLUGIN_DATA not expanded: %v", result.Plugin.MCPServers[0].Config.Stdio.Args)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	roots := store.EnabledSkillRoots()
	if len(roots) != 1 || !strings.HasSuffix(roots[0], filepath.Join("acme.tools", "skills")) {
		t.Fatalf("roots=%v", roots)
	}
	servers := store.PluginMCPServers()
	if len(servers) != 1 || servers[0].Scope != mcpclient.ScopePlugin {
		t.Fatalf("mcp=%+v", servers)
	}
	statePath := desktopdata.PluginsConfigPath(store.home)
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"acme.tools"`) {
		t.Fatalf("state=%s", raw)
	}
}

func TestStoreChangeSubscribersDoNotReplaceRuntimeRefreshOwner(t *testing.T) {
	store := testStore(t)
	primaryCalls := 0
	listenerCalls := 0
	store.SetOnChange(func() { primaryCalls++ })
	unsubscribe := store.SubscribeChanges(func() { listenerCalls++ })
	src := t.TempDir()
	writePluginFixture(t, src, "subscriber.plugin", false)
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}
	if primaryCalls != 1 || listenerCalls != 1 {
		t.Fatalf("primary=%d listener=%d", primaryCalls, listenerCalls)
	}
	unsubscribe()
	if _, err := store.SetEnabled("subscriber.plugin", false); err != nil {
		t.Fatal(err)
	}
	if primaryCalls != 2 || listenerCalls != 1 {
		t.Fatalf("after unsubscribe primary=%d listener=%d", primaryCalls, listenerCalls)
	}
}

func TestResolveComposerSelectionRevalidatesEnabledDigestAndComponents(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "composer.plugin", true)
	installed, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := store.ResolveComposerSelection(installed.Plugin.Name, installed.Plugin.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.SkillIDs) != 1 || selection.SkillIDs[0] != "plugin__composer-plugin__lint" {
		t.Fatalf("skills=%v", selection.SkillIDs)
	}
	if len(selection.ServerIDs) != 1 || selection.ServerIDs[0] != "plugin__composer.plugin__local" {
		t.Fatalf("servers=%v", selection.ServerIDs)
	}
	if _, err := store.ResolveComposerSelection(installed.Plugin.Name, "stale"); !errors.Is(err, ErrDigestChanged) {
		t.Fatalf("stale digest err=%v", err)
	}
	if _, err := store.SetEnabled(installed.Plugin.Name, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveComposerSelection(installed.Plugin.Name, installed.Plugin.Digest); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled err=%v", err)
	}
}

func TestValidateLocalPluginDoesNotInstall(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "validate.only", true)

	validation, err := store.Validate(context.Background(), Source{Kind: SourceLocalPath, Path: src})
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || validation.Name != "validate.only" || validation.SkillCount != 1 || validation.ServerCount != 1 || validation.Digest == "" || validation.TargetRoot != filepath.Join(store.packagesRoot, "validate.only") {
		t.Fatalf("validation=%+v", validation)
	}
	installed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 0 {
		t.Fatalf("validation mutated installed state: %+v", installed)
	}
	if _, err := os.Stat(filepath.Join(store.packagesRoot, "validate.only")); !os.IsNotExist(err) {
		t.Fatalf("validation created a live package: %v", err)
	}
}

func TestValidatePluginReportsInvalidEmbeddedSkill(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "invalid.skill", false)
	if err := os.WriteFile(filepath.Join(src, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	validation, err := store.Validate(context.Background(), Source{Kind: SourceLocalPath, Path: src})
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatalf("invalid embedded Skill passed validation: %+v", validation)
	}
	found := false
	for _, diagnostic := range validation.Diagnostics {
		if diagnostic.Code == "plugin_skill_invalid" && diagnostic.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing embedded Skill diagnostic: %+v", validation.Diagnostics)
	}
}

func TestInstallSameNameAbortsWithoutReplace(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
}

func TestInstallRejectsSourceChangedAfterValidation(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "source.changed", false)
	validation, err := store.Validate(context.Background(), Source{Kind: SourceLocalPath, Path: src})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "changed.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}, ExpectedDigest: validation.Digest})
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("err=%v", err)
	}
	if listed, listErr := store.List(); listErr != nil || len(listed) != 0 {
		t.Fatalf("changed source mutated store: list=%v err=%v", listed, listErr)
	}
}

func TestReplaceUsesTrashAndKeepsData(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}
	dataFile := filepath.Join(store.dataRoot, "sample", "kept.txt")
	if err := os.WriteFile(dataFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extra.txt"), []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replaced || result.PreviousTrashedPath == "" {
		t.Fatalf("replace=%+v", result)
	}
	if _, err := os.Stat(dataFile); err != nil {
		t.Fatalf("data should survive replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.packagesRoot, "sample", "extra.txt")); err != nil {
		t.Fatalf("new file missing: %v", err)
	}
}

func TestReplaceFailsClosedWhenTrashUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	store := NewStoreAtWithTrash(home, testTrashAdapter{fail: true})
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extra.txt"), []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}, Replace: true}); !errors.Is(err, ErrReplaceTrashUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(store.packagesRoot, "sample", "plugin.json")); err != nil {
		t.Fatalf("original package should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.packagesRoot, "sample", "extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("failed replacement leaked the new package: %v", err)
	}
}

func TestUpgradeNoopOnSameDigest(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	first, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Upgrade(context.Background(), "sample")
	if err != nil {
		t.Fatal(err)
	}
	if second.Replaced || second.Plugin.Digest != first.Plugin.Digest {
		t.Fatalf("expected no-op upgrade: %+v", second)
	}
}

func TestUninstallMovesPackageAndData(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.dataRoot, "sample", "x"), []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}
	trashed, err := store.Uninstall("sample")
	if err != nil {
		t.Fatal(err)
	}
	if trashed == "" {
		t.Fatal("expected trash path")
	}
	if _, err := os.Stat(filepath.Join(store.packagesRoot, "sample")); !os.IsNotExist(err) {
		t.Fatalf("package still present: %v", err)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 0 {
		t.Fatalf("list after uninstall: %v %v", listed, err)
	}
}

func TestCorruptStateFailsClosed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AWAKEN_HOME", home)
	path := desktopdata.PluginsConfigPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(home)
	if err := store.Available(); !errors.Is(err, ErrStateCorrupt) {
		t.Fatalf("available=%v", err)
	}
	if _, err := store.List(); err == nil {
		t.Fatal("list should fail closed")
	}
}

func TestEnableDisableAndServerToggle(t *testing.T) {
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "sample", true)
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err != nil {
		t.Fatal(err)
	}
	plugin, err := store.SetServerEnabled("sample", "local", false)
	if err != nil {
		t.Fatal(err)
	}
	if plugin.MCPServers[0].Enabled {
		t.Fatal("server should be disabled")
	}
	servers := store.PluginMCPServers()
	if len(servers) != 1 || servers[0].Config.Enabled == nil || *servers[0].Config.Enabled {
		t.Fatalf("disabled server should remain visible: %+v", servers)
	}
	if _, err := store.SetEnabled("sample", false); err != nil {
		t.Fatal(err)
	}
	if roots := store.EnabledSkillRoots(); len(roots) != 0 {
		t.Fatalf("disabled plugin still exported skills: %v", roots)
	}
	if servers := store.PluginMCPServers(); len(servers) != 0 {
		t.Fatalf("disabled plugin still exported MCP: %+v", servers)
	}
}

func TestInstallZipAndRejectZipSlip(t *testing.T) {
	store := testStore(t)
	good := t.TempDir()
	writePluginFixture(t, good, "zippy", false)
	zipPath := filepath.Join(t.TempDir(), "plugin.zip")
	if err := zipDir(good, zipPath); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: zipPath}}); err != nil {
		t.Fatal(err)
	}

	var slip bytes.Buffer
	writer := zip.NewWriter(&slip)
	file, err := writer.Create("../escape/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(`{"$schema":"` + SchemaPluginV1 + `","name":"evil"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(slip.Bytes(), t.TempDir()); err == nil {
		t.Fatal("expected zip-slip rejection")
	}
}

func TestInstallRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture is unix-oriented")
	}
	store := testStore(t)
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	if err := os.Symlink(filepath.Join(src, "plugin.json"), filepath.Join(src, "link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), InstallRequest{Source: Source{Kind: SourceLocalPath, Path: src}}); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestRemoteURLConstraints(t *testing.T) {
	if err := validateHTTPSURL("http://example.com/p.zip", "remote_url"); err == nil {
		t.Fatal("http remote_url should be rejected")
	}
	if err := validateHTTPSURL("https://user:pass@example.com/p.zip", "remote_url"); err == nil {
		t.Fatal("credential URL should be rejected")
	}
	if _, err := cleanRemoteGitURL("git@github.com:acme/tools.git"); err == nil {
		t.Fatal("ssh git URL should be rejected")
	}
}

func TestDigestStableAcrossCopy(t *testing.T) {
	src := t.TempDir()
	writePluginFixture(t, src, "sample", false)
	first, err := digestTree(src)
	if err != nil {
		t.Fatal(err)
	}
	second, err := digestTree(src)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("digest unstable: %q %q", first, second)
	}
}

func zipDir(src, dest string) error {
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	err = filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			_, err := writer.Create(rel + "/")
			return err
		}
		header, err := writer.Create(rel)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = header.Write(data)
		return err
	})
	if err != nil {
		writer.Close()
		return err
	}
	return writer.Close()
}
