package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"awaken/src/plugins/ir"
)

func installedMCPComponent(t *testing.T, plugin *InstalledPlugin) ir.Component {
	t.Helper()
	for _, component := range plugin.Components {
		if component.Kind == ir.KindMCPServer {
			return component
		}
	}
	t.Fatal("installed plugin has no MCP component")
	return ir.Component{}
}

func TestMCPSourceIsRawAndAtomicEditRefreshesRuntime(t *testing.T) {
	store := testStore(t)
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "source.edit", true)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	component := installedMCPComponent(t, installed.Plugin)
	detail, err := store.ComponentDetail("source.edit", component.LogicalID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.SourceEditable {
		t.Fatal("MCP source must be marked editable")
	}
	for _, expected := range []string{`"command":"npx"`, `"${PLUGIN_DATA}/cache"`} {
		if !strings.Contains(detail.Source, expected) {
			t.Fatalf("raw MCP source is missing %q: %s", expected, detail.Source)
		}
	}
	if strings.Contains(detail.Source, "<configured>") {
		t.Fatalf("MCP source was redacted: %s", detail.Source)
	}

	notifications := 0
	store.SetOnChange(func() { notifications++ })
	unchanged, err := store.UpdateMCPSource("source.edit", component.LogicalID, installed.Plugin.Digest, detail.Source)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Changed || unchanged.Digest != installed.Plugin.Digest || unchanged.PreviousDigest != installed.Plugin.Digest {
		t.Fatalf("unchanged result=%+v", unchanged)
	}
	if notifications != 0 {
		t.Fatalf("unchanged save notified listeners: %d", notifications)
	}
	editedSource := `{
  "$schema": "` + SchemaMCPV1 + `",
  "mcpServers": {
    "local": {
      "type": "stdio",
      "command": "node",
      "args": ["server.js", "${PLUGIN_ROOT}/config.json"]
    }
  }
}`
	updated, err := store.UpdateMCPSource("source.edit", component.LogicalID, installed.Plugin.Digest, editedSource)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Changed || updated.PreviousDigest != installed.Plugin.Digest || updated.Digest == installed.Plugin.Digest {
		t.Fatalf("update result=%+v", updated)
	}
	if updated.Plugin == nil || !updated.Plugin.IntegrityValid || updated.Plugin.Digest != updated.Digest {
		t.Fatalf("updated plugin=%+v", updated.Plugin)
	}
	if notifications != 1 {
		t.Fatalf("notifications=%d want 1", notifications)
	}
	written, err := os.ReadFile(filepath.Join(updated.Plugin.Root, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != editedSource {
		t.Fatalf("written source changed formatting:\n%s", written)
	}
	servers := store.PluginMCPServers()
	if len(servers) != 1 || servers[0].Config.Stdio == nil || servers[0].Config.Stdio.Command != "node" {
		t.Fatalf("runtime MCP projection was not refreshed: %+v", servers)
	}
	if got := servers[0].Config.Stdio.Args; len(got) != 2 || got[0] != "server.js" || !strings.HasSuffix(got[1], "/config.json") {
		t.Fatalf("runtime MCP args=%v", got)
	}

	updatedDetail, err := store.ComponentDetail("source.edit", component.LogicalID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDetail.Source != editedSource {
		t.Fatalf("detail source=%q", updatedDetail.Source)
	}
}

func TestMCPSourceEditRejectsInvalidStaleAndLeasedMutationsWithoutChangingLiveTree(t *testing.T) {
	store := testStore(t)
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "source.reject", true)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	component := installedMCPComponent(t, installed.Plugin)
	livePath := filepath.Join(installed.Plugin.Root, "mcp.json")
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.UpdateMCPSource("source.reject", component.LogicalID, "sha256-v2:stale", string(original)); !errors.Is(err, ErrRecordedDigestChanged) {
		t.Fatalf("stale digest err=%v", err)
	}
	if _, err := store.UpdateMCPSource("source.reject", component.LogicalID, installed.Plugin.Digest, `{not-json`); !errors.Is(err, ErrMCPInvalid) && !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("invalid source err=%v", err)
	}
	assertSourceUnchanged := func() {
		t.Helper()
		current, readErr := os.ReadFile(livePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(current) != string(original) {
			t.Fatalf("rejected edit changed live source: %s", current)
		}
		plugin, getErr := store.Get("source.reject")
		if getErr != nil || !plugin.IntegrityValid || plugin.Digest != installed.Plugin.Digest {
			t.Fatalf("rejected edit changed plugin: plugin=%+v err=%v", plugin, getErr)
		}
	}
	assertSourceUnchanged()

	lease, err := store.AcquireRuntimeLease()
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseRuntimeLease(lease.ID)
	if _, err := store.UpdateMCPSource("source.reject", component.LogicalID, installed.Plugin.Digest, string(original)+"\n"); !errors.Is(err, ErrRuntimeLeaseActive) {
		t.Fatalf("leased edit err=%v", err)
	}
	assertSourceUnchanged()
}

func TestMCPSourceEditRollsBackPackageAndStateWhenTrashIsUnavailable(t *testing.T) {
	home := t.TempDir()
	store := NewStoreAtWithTrash(home, testTrashAdapter{dir: filepath.Join(t.TempDir(), "Trash"), fail: true})
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "source.rollback", true)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	component := installedMCPComponent(t, installed.Plugin)
	livePath := filepath.Join(installed.Plugin.Root, "mcp.json")
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(original), `"command":"npx"`, `"command":"node"`, 1)
	notifications := 0
	store.SetOnChange(func() { notifications++ })
	if _, err := store.UpdateMCPSource("source.rollback", component.LogicalID, installed.Plugin.Digest, edited); !errors.Is(err, ErrReplaceTrashUnavailable) {
		t.Fatalf("err=%v", err)
	}
	currentSource, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentSource) != string(original) {
		t.Fatalf("failed edit was not rolled back: %s", currentSource)
	}
	plugin, err := store.Get("source.rollback")
	if err != nil || !plugin.IntegrityValid || plugin.Digest != installed.Plugin.Digest {
		t.Fatalf("plugin=%+v err=%v", plugin, err)
	}
	if notifications != 0 {
		t.Fatalf("failed edit notified listeners: %d", notifications)
	}
}
