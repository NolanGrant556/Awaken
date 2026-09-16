package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Optional timing with a real installed manifest. Only stat reads touch its
// files; all manager records, bindings and locks live in t.TempDir(). No
// installed executable is launched and the real registry is never opened.
func TestOfficeReuseProfile(t *testing.T) {
	path := os.Getenv("AWAKEN_REUSE_PROFILE_MANIFEST")
	if path == "" {
		t.Skip("set an explicit manifest for read-only reuse timing")
	}
	var f Manifest
	if err := readJSON(path, &f); err != nil {
		t.Fatal(err)
	}
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	f.Root = filepath.Join(m.root, "office", "instances", m.target, f.Spec.OfficeBundle.Version, f.InstanceID)
	if err := m.save(f); err != nil {
		t.Fatal(err)
	}
	owner := testOwner("profile")
	b, err := m.bind(f, owner, true)
	if err != nil {
		t.Fatal(err)
	}
	measure := func(name string, fn func() error) {
		t.Helper()
		start := time.Now()
		for i := 0; i < 5; i++ {
			if err := fn(); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("%s: mean %s", name, time.Since(start)/5)
	}
	measure("registry read", func() error { _, err := m.instances(); return err })
	measure("full health incl metadata fingerprints", func() error {
		if !m.healthy(f) {
			t.Fatal("source files changed during profile")
		}
		return nil
	})
	measure("atomic execution-description write", func() error { return writeJSON(filepath.Join(m.root, "profile.json"), b) })
	measure("existing Binding", func() error { _, err := m.Binding(context.Background(), b.EnvironmentRef, owner); return err })
	measure("EnsureOfficeBundle", func() error { _, err := m.EnsureOfficeBundle(context.Background(), owner, nil); return err })
}
