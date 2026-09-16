package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func cliOwnershipFixture(t *testing.T) (*Manager, string, string, string) {
	t.Helper()
	home := t.TempDir()
	catalog, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	m := managerForTest(t, Options{Root: home, Catalog: catalog})
	name := "dws"
	if strings.HasPrefix(Target(), "windows-") {
		name += ".exe"
	}
	executable := filepath.Join(home, "runtime", "cli", "dingtalk-workspace", "1.0.32", "platform", name)
	alias := filepath.Join(home, "runtime", "cli", "bin", name)
	for _, p := range []string{executable, alias} {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("controlled CLI bytes"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	record := filepath.Join(home, "runtime", "cli", "installations.json")
	if err := writeJSON(record, map[string]any{"version": 1, "records": map[string]any{"dingtalk-workspace": map[string]string{"executablePath": executable, "version": "1.0.32", "ownership": "awaken-managed"}}}); err != nil {
		t.Fatal(err)
	}
	return m, executable, alias, record
}

func TestCLIOwnershipMigrationIsSharedDurableAndProtectsLegacyAlias(t *testing.T) {
	m, executable, alias, record := cliOwnershipFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.InspectCLIInstallation(ctx, "dingtalk-workspace"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	all, err := m.instances()
	if err != nil || len(all) != 1 || all[0].Source != "awaken-managed" || len(all[0].OwnedFiles) != 2 {
		t.Fatal(all, err)
	}
	original := all[0]
	manifest := filepath.Join(original.Root, "manifest.json")
	info, _ := os.Stat(manifest)
	got, err := m.InspectCLIInstallation(ctx, "dingtalk-workspace")
	if err != nil || got == nil || !got.Healthy || !sameHostPath(got.ExecutablePath, m.canonicalCLIPath(executable)) {
		t.Fatal(got, err)
	}
	after, _ := os.Stat(manifest)
	if !os.SameFile(info, after) {
		t.Fatal("repeated inspection rewrote ownership")
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(m.root, "registry.json")); err != nil {
		t.Fatal(err)
	}
	got, err = m.InspectCLIInstallation(ctx, "dingtalk-workspace")
	if err != nil || got == nil || !got.Healthy {
		t.Fatal("manifest-only recovery", got, err)
	}
	ref, err := m.HoldHostExecutable(alias, os.Getpid(), "")
	if err != nil || ref == "" {
		t.Fatal(ref, err)
	}
	_, err = m.ReserveRemoval(ctx, "dingtalk-workspace", os.Getpid())
	assertEnvironmentCode(t, err, "environment_in_use")
	if err := m.ReleaseHostExecutable(ref); err != nil {
		t.Fatal(err)
	}
	token, err := m.ReserveRemoval(ctx, "dingtalk-workspace", os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.HoldHostExecutable(executable, os.Getpid(), "")
	assertEnvironmentCode(t, err, "environment_removing")
	if err := m.RemoveComponent(ctx, "dingtalk-workspace", token); err != nil {
		t.Fatal(err)
	}
	if err := m.ReleaseRemoval("dingtalk-workspace", token); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{executable, alias, original.Root} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("owned path retained", path, err)
		}
	}
	if err := m.RemoveComponent(ctx, "dingtalk-workspace"); err != nil {
		t.Fatal("retry", err)
	}
}

func TestCLIOwnershipNormalizesLegacyInstanceWithoutChangingLeaseIdentity(t *testing.T) {
	m, executable, _, record := cliOwnershipFixture(t)
	f := Manifest{SchemaVersion: SchemaVersion, Kind: "component", InstanceID: strings.Repeat("c", 32), Key: "old-key", ComponentID: "dingtalk-workspace", Version: `{"version":"v1.0.32"}`, Target: m.target, Source: "awaken-legacy", State: "ready", Executables: map[string]Launch{"dws": {Executable: executable}}, CreatedAt: m.now()}
	f.Root = filepath.Join(m.root, "components", f.Key, f.InstanceID)
	f.Fingerprints = fingerprints(f.Executables)
	if err := m.save(f); err != nil {
		t.Fatal(err)
	}
	owner := testOwner("before-migration")
	if err := m.writeLease(f.InstanceID, owner); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	got, err := m.InspectCLIInstallation(context.Background(), "dingtalk-workspace")
	if err != nil || got == nil || got.Version != "1.0.32" {
		t.Fatal(got, err)
	}
	all, err := m.instances()
	if err != nil || len(all) != 1 || all[0].InstanceID != f.InstanceID || all[0].Source != "awaken-managed" {
		t.Fatal(all, err)
	}
	_, err = m.ReserveRemoval(context.Background(), "dingtalk-workspace", os.Getpid())
	assertEnvironmentCode(t, err, "environment_in_use")
}

func TestCLIOwnershipPreservesExternalAndModifiedFiles(t *testing.T) {
	t.Run("external record", func(t *testing.T) {
		m, executable, _, record := cliOwnershipFixture(t)
		external := filepath.Join(t.TempDir(), "dws")
		if err := os.Rename(executable, external); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(record, map[string]any{"version": 1, "records": map[string]any{"dingtalk-workspace": map[string]string{"executablePath": external, "version": "1.0.32", "ownership": "awaken-managed"}}}); err != nil {
			t.Fatal(err)
		}
		got, err := m.InspectCLIInstallation(context.Background(), "dingtalk-workspace")
		if err != nil || got != nil {
			t.Fatal(got, err)
		}
		if err := m.RemoveComponent(context.Background(), "dingtalk-workspace"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(external); err != nil {
			t.Fatal("external removed", err)
		}
	})
	t.Run("modified alias", func(t *testing.T) {
		m, executable, alias, _ := cliOwnershipFixture(t)
		if _, err := m.InspectCLIInstallation(context.Background(), "dingtalk-workspace"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(alias, []byte("user replacement"), 0700); err != nil {
			t.Fatal(err)
		}
		err := m.RemoveComponent(context.Background(), "dingtalk-workspace")
		assertEnvironmentCode(t, err, "environment_validation_failed")
		if _, err := os.Stat(executable); err != nil {
			t.Fatal("partial destructive uninstall", err)
		}
	})
	t.Run("redirected executable", func(t *testing.T) {
		m, executable, _, _ := cliOwnershipFixture(t)
		if err := os.Remove(executable); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external")
		if err := os.WriteFile(external, []byte("external"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, executable); err != nil {
			t.Skip("symlink unavailable", err)
		}
		got, err := m.InspectCLIInstallation(context.Background(), "dingtalk-workspace")
		if err != nil || got != nil {
			t.Fatal(got, err)
		}
	})
}
