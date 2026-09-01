package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writeBuiltinMetadataFixture(t *testing.T, packageRoot, displayName, brandIcon string, order int) {
	t.Helper()
	metadataDir := filepath.Join(packageRoot, ".awaken")
	if err := os.MkdirAll(metadataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"version":1,"displayName":"` + displayName + `","brandIcon":"` + brandIcon + `","order":` + strconv.Itoa(order) + `}`)
	if err := os.WriteFile(filepath.Join(metadataDir, "builtin.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSyncBuiltinPluginsDefaultsDisabledAndPreservesActivationAcrossUpdates(t *testing.T) {
	store := testStore(t)
	root := t.TempDir()
	packageRoot := filepath.Join(root, "outlook")
	writePluginFixture(t, packageRoot, "awaken-outlook", false)
	writeBuiltinMetadataFixture(t, packageRoot, "Outlook 邮箱", "outlook", 0)

	synced, err := store.SyncBuiltinPlugins(context.Background(), root)
	if err != nil || len(synced) != 1 {
		t.Fatalf("sync=%+v err=%v", synced, err)
	}
	plugin := synced[0]
	if !plugin.Builtin || plugin.Enabled || plugin.DisplayName != "Outlook 邮箱" || plugin.BrandIcon != "outlook" || plugin.BuiltinOrder != 0 {
		t.Fatalf("built-in projection=%+v", plugin)
	}
	if selections := store.EnabledSkillSelections(); len(selections) != 0 {
		t.Fatalf("disabled built-in plugin leaked Skills: %+v", selections)
	}

	if _, err := store.SetEnabled("awaken-outlook", true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageRoot, "release-marker.txt"), []byte("v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	synced, err = store.SyncBuiltinPlugins(context.Background(), root)
	if err != nil || len(synced) != 1 || !synced[0].Enabled || !synced[0].Builtin {
		t.Fatalf("updated sync=%+v err=%v", synced, err)
	}
	if selections := store.EnabledSkillSelections(); len(selections) != 1 {
		t.Fatalf("enabled built-in plugin did not restore Skills: %+v", selections)
	}
}

func TestBuiltinPluginRejectsUserReplacementAndUninstall(t *testing.T) {
	store := testStore(t)
	root := t.TempDir()
	packageRoot := filepath.Join(root, "outlook")
	writePluginFixture(t, packageRoot, "awaken-outlook", false)
	writeBuiltinMetadataFixture(t, packageRoot, "Outlook 邮箱", "outlook", 0)
	if _, err := store.SyncBuiltinPlugins(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Install(context.Background(), InstallRequest{
		Source:  Source{Kind: SourceLocalPath, Path: packageRoot},
		Replace: true,
	}); !errors.Is(err, ErrBuiltinImmutable) {
		t.Fatalf("replace err=%v", err)
	}
	if _, err := store.Uninstall("awaken-outlook"); !errors.Is(err, ErrBuiltinImmutable) {
		t.Fatalf("uninstall err=%v", err)
	}
}

func TestSyncBuiltinPluginsIsolatesInvalidCatalogEntries(t *testing.T) {
	store := testStore(t)
	root := t.TempDir()
	good := filepath.Join(root, "good")
	bad := filepath.Join(root, "bad")
	writePluginFixture(t, good, "good.plugin", false)
	writeBuiltinMetadataFixture(t, good, "内置插件", "outlook", 1)
	writePluginFixture(t, bad, "bad.plugin", false)
	writeBuiltinMetadataFixture(t, bad, "错误插件", "../../escape", 2)

	synced, err := store.SyncBuiltinPlugins(context.Background(), root)
	if err == nil || len(synced) != 1 || synced[0].Name != "good.plugin" {
		t.Fatalf("sync=%+v err=%v", synced, err)
	}
}
