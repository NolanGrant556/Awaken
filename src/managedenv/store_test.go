package managedenv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompactRegistryMigratesLegacyManifests(t *testing.T) {
	for _, schema := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(schema), func(t *testing.T) {
			opts := Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)}
			m := managerForTest(t, opts)
			f := officeStoredFixture(t, m, "large-manifest", "")
			for i := 0; i < 5000; i++ {
				f.Fingerprints[filepath.Join(f.Root, fmt.Sprintf("library-%d", i))] = strings.Repeat("a", 64)
			}
			if err := m.save(f); err != nil {
				t.Fatal(err)
			}
			retired := time.Now().UTC().Add(-time.Hour)
			f.LastUsedAt, f.RetiredAt = retired, &retired
			path := filepath.Join(m.root, "registry.json")
			// Actual old on-disk shape, bypassing the compact registry marshaler.
			if err := writeJSON(path, map[string]any{"schema_version": schema, "instances": map[string]Manifest{f.InstanceID: f}}); err != nil {
				t.Fatal(err)
			}
			restarted := managerForTest(t, opts)
			all, err := restarted.instances()
			if err != nil || len(all) != 1 || len(all[0].Fingerprints) != 5001 || !all[0].LastUsedAt.Equal(retired) || all[0].RetiredAt == nil {
				t.Fatalf("lost metadata: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var index registry
			if err := json.Unmarshal(raw, &index); err != nil {
				t.Fatal(err)
			}
			if index.SchemaVersion != registrySchemaVersion || len(raw) > 2048 || strings.Contains(string(raw), "fingerprints") {
				t.Fatalf("index still duplicates manifest: %d bytes", len(raw))
			}
			t.Logf("5001 fingerprints retained in manifest; index %d bytes", len(raw))
		})
	}
}

func TestRegistryScansOnlyWhenRecoveryIsNeeded(t *testing.T) {
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	old := officeStoredFixture(t, m, "indexed", "")
	if err := m.activateOfficeBundle(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	m.catalog.OfficeBundle.Version = "next-version"
	next := officeStoredFixture(t, m, "unindexed", "")
	if err := m.activateOfficeBundle(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if err := m.registry(func(r *registry) error { delete(r.Instances, next.InstanceID); return nil }); err != nil {
		t.Fatal(err)
	}
	// Successful lookup must not glob the unindexed manifest into the registry.
	if _, err := m.resolveSpec(old.Spec); err != nil {
		t.Fatal(err)
	}
	all, err := m.instances()
	if err != nil || len(all) != 1 {
		t.Fatalf("ordinary read scanned disk: %d %v", len(all), err)
	}
	recovered, err := m.resolveSpec(next.Spec)
	if err != nil || recovered.InstanceID != next.InstanceID {
		t.Fatalf("miss failed to recover completed manifest: %v", err)
	}
	if err := os.Remove(filepath.Join(next.Root, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.resolveSpec(next.Spec); err == nil {
		t.Fatal("cached manifest concealed deletion")
	}
}

func TestHealthReuseIsBoundedAndLaunchFilesAlwaysChecked(t *testing.T) {
	var elapsed atomic.Int64
	base := time.Now()
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t), Now: func() time.Time { return base.Add(time.Duration(elapsed.Load())) }})
	f := officeStoredFixture(t, m, "health", "")
	source := filepath.Join(f.Root, "library.py")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	f.Fingerprints[source] = fileIdentity(info)
	if err := m.save(f); err != nil {
		t.Fatal(err)
	}
	if !m.reusable(f) {
		t.Fatal("healthy environment rejected")
	}
	if err := os.WriteFile(source, []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if !m.reusable(f) {
		t.Fatal("nearby lookup did not share completed scan")
	}
	if m.healthy(f) {
		t.Fatal("fresh diagnostic accepted changed source")
	}
	elapsed.Store(int64(healthReuseWindow))
	if m.reusable(f) {
		t.Fatal("expired scan concealed changed source")
	}
	info, _ = os.Stat(source)
	f.Fingerprints[source] = fileIdentity(info)
	if err := m.save(f); err != nil {
		t.Fatal(err)
	}
	if !m.reusable(f) {
		t.Fatal("replaced manifest did not invalidate parsed cache")
	}
	if err := os.Remove(f.Executables["python"].Executable); err != nil {
		t.Fatal(err)
	}
	if m.reusable(f) {
		t.Fatal("cached scan concealed missing interpreter")
	}
}

func TestMalformedIndexShapeIsRebuiltFromManifests(t *testing.T) {
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	f := officeStoredFixture(t, m, "recover-shape", "")
	path := filepath.Join(m.root, "registry.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":3,"instances":{"broken":{"root":42}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	all, err := m.instances()
	if err != nil || len(all) != 1 || all[0].InstanceID != f.InstanceID {
		t.Fatalf("partial malformed index was retained: %+v %v", all, err)
	}
	preserved, err := filepath.Glob(path + ".invalid-*")
	if err != nil || len(preserved) != 1 {
		t.Fatal("malformed index was not preserved", err)
	}
}
