package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"awaken/src/skills"
)

func TestCommitInstalledChangesAcceptsValidatedLiveTreeAndNotifies(t *testing.T) {
	store := testStore(t)
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "commit.plugin", true)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetServerEnabled("commit.plugin", "local", false); err != nil {
		t.Fatal(err)
	}
	original := installedPluginRecord(t, store, "commit.plugin")

	notifications := 0
	store.SetOnChange(func() { notifications++ })
	manifest := `{
		"$schema":"` + SchemaPluginV1 + `",
		"name":"commit.plugin",
		"version":"2.0.0",
		"description":"Edited in place"
	}`
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "skills", "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: updated lint\n---\nUpdated body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installed.Plugin.Root, "skills", "broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "skills", "broken", "SKILL.md"), []byte("---\nname: broken\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	brokenView, err := store.Get("commit.plugin")
	if err != nil {
		t.Fatal(err)
	}
	if brokenView.IntegrityValid || brokenView.SkillRoot != "" || len(brokenView.MCPServers) != 0 {
		t.Fatalf("digest drift did not fail closed before commit: %+v", brokenView)
	}

	committed, err := store.CommitInstalledChanges("commit.plugin", original.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if !committed.Changed || committed.PreviousDigest != original.Digest || committed.Digest == original.Digest {
		t.Fatalf("commit result=%+v", committed)
	}
	if committed.Plugin == nil || !committed.Plugin.IntegrityValid || committed.Plugin.Digest != committed.Digest || committed.Plugin.ActualDigest != committed.Digest {
		t.Fatalf("committed plugin=%+v", committed.Plugin)
	}
	if committed.Plugin.Manifest.Version != "2.0.0" || committed.Plugin.SkillRoot == "" || len(committed.Plugin.MCPServers) != 1 {
		t.Fatalf("components were not restored: %+v", committed.Plugin)
	}
	if committed.Plugin.MCPServers[0].Enabled {
		t.Fatal("commit lost the recorded MCP server toggle")
	}
	if notifications != 1 {
		t.Fatalf("notifications=%d want 1", notifications)
	}

	definitions := skills.LoadDir(committed.Plugin.SkillRoot, skills.SourcePlugin)
	lint := definitions["plugin__commit-plugin__lint"]
	if lint == nil || lint.State != skills.StateLoaded {
		t.Fatalf("valid Skill was not restored: %+v", lint)
	}
	broken := definitions["plugin__commit-plugin__broken"]
	if broken == nil || broken.State != skills.StateInvalid {
		t.Fatalf("invalid sibling Skill was not isolated: %+v", broken)
	}
	servers := store.PluginMCPServers()
	if len(servers) != 1 || servers[0].Config.Enabled == nil || *servers[0].Config.Enabled {
		t.Fatalf("MCP server/toggle was not restored: %+v", servers)
	}

	current := installedPluginRecord(t, store, "commit.plugin")
	if current.Digest != committed.Digest || current.Version != "2.0.0" {
		t.Fatalf("record was not updated: %+v", current)
	}
	if current.Enabled != original.Enabled || current.Source != original.Source || !current.InstalledAt.Equal(original.InstalledAt) {
		t.Fatalf("commit changed durable install metadata: before=%+v after=%+v", original, current)
	}
	if current.MCPServers["local"] != original.MCPServers["local"] {
		t.Fatalf("commit changed MCP toggles: before=%+v after=%+v", original.MCPServers, current.MCPServers)
	}

	noChange, err := store.CommitInstalledChanges("commit.plugin", committed.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if noChange.Changed || noChange.Plugin == nil || !noChange.Plugin.IntegrityValid {
		t.Fatalf("no-op commit=%+v", noChange)
	}
	if notifications != 1 {
		t.Fatalf("no-op commit notified: %d", notifications)
	}
}

func TestCommitInstalledChangesRequiresMatchingRecordedDigest(t *testing.T) {
	store := testStore(t)
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "commit.cas", false)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "changed.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}

	notifications := 0
	store.SetOnChange(func() { notifications++ })
	for _, expected := range []string{"", "sha256-v2:stale"} {
		if _, err := store.CommitInstalledChanges("commit.cas", expected); !errors.Is(err, ErrRecordedDigestChanged) {
			t.Fatalf("expected=%q err=%v", expected, err)
		}
	}
	if notifications != 0 {
		t.Fatalf("failed CAS notified: %d", notifications)
	}
	current := installedPluginRecord(t, store, "commit.cas")
	if current.Digest != installed.Plugin.Digest {
		t.Fatalf("failed CAS changed record: %+v", current)
	}
}

func TestCommitInstalledChangesRejectsInvalidManifestAndIdentityChange(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
	}{
		{name: "invalid manifest", manifest: `{not-json`},
		{name: "identity change", manifest: `{"$schema":"` + SchemaPluginV1 + `","name":"different.plugin","version":"2.0.0"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := testStore(t)
			sourceRoot := t.TempDir()
			writePluginFixture(t, sourceRoot, "commit.invalid", false)
			installed, err := store.Install(context.Background(), InstallRequest{
				Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "plugin.json"), []byte(test.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			notifications := 0
			store.SetOnChange(func() { notifications++ })
			if _, err := store.CommitInstalledChanges("commit.invalid", installed.Plugin.Digest); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("err=%v", err)
			}
			if notifications != 0 {
				t.Fatalf("invalid commit notified: %d", notifications)
			}
			current := installedPluginRecord(t, store, "commit.invalid")
			if current.Digest != installed.Plugin.Digest || current.Version != "1.0.0" {
				t.Fatalf("invalid commit changed record: %+v", current)
			}
		})
	}
}

func TestCommitInstalledChangesRejectsMutationDuringValidation(t *testing.T) {
	store := testStore(t)
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "commit.race", false)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed.Plugin.Root, "authorized.txt"), []byte("first edit"), 0o600); err != nil {
		t.Fatal(err)
	}

	notifications := 0
	store.SetOnChange(func() { notifications++ })
	_, err = store.commitInstalledChanges("commit.race", installed.Plugin.Digest, func() {
		if writeErr := os.WriteFile(filepath.Join(installed.Plugin.Root, "concurrent.txt"), []byte("second edit"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("err=%v", err)
	}
	if notifications != 0 {
		t.Fatalf("racing commit notified: %d", notifications)
	}
	current := installedPluginRecord(t, store, "commit.race")
	if current.Digest != installed.Plugin.Digest || current.Version != "1.0.0" {
		t.Fatalf("racing commit changed record: %+v", current)
	}
}

func TestCommitInstalledChangesRejectsPackageSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture is Unix-oriented")
	}
	store := testStore(t)
	sourceRoot := t.TempDir()
	writePluginFixture(t, sourceRoot, "commit.symlink", false)
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: sourceRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(installed.Plugin.Root, "plugin.json"), filepath.Join(installed.Plugin.Root, "manifest-link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitInstalledChanges("commit.symlink", installed.Plugin.Digest); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("err=%v", err)
	}
	current := installedPluginRecord(t, store, "commit.symlink")
	if current.Digest != installed.Plugin.Digest {
		t.Fatalf("symlink commit changed record: %+v", current)
	}
}

func installedPluginRecord(t *testing.T, store *Store, name string) Record {
	t.Helper()
	state, err := store.readStateUnlocked()
	if err != nil {
		t.Fatal(err)
	}
	record, ok := state.Plugins[name]
	if !ok {
		t.Fatalf("missing plugin record %q", name)
	}
	return record
}
