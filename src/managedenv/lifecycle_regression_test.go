package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOfficeRuntimeDataDamageRecovers(t *testing.T) {
	for _, relative := range []string{
		"dependencies/python/lib/site-packages/openpyxl/workbook.py",
		"dependencies/python/lib/site-packages/docx/templates/default.docx",
		"dependencies/python/lib/site-packages/pptx/templates/default.pptx",
		"dependencies/python/lib/site-packages/magika/models/model.onnx",
		"dependencies/node-packages/package.json",
		"dependencies/native/libreoffice/services.rdb",
		"dependencies/native/libreoffice/registry/calc.xcd",
		"dependencies/fonts/body.otf",
	} {
		t.Run(relative, func(t *testing.T) {
			m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
			old := officeStoredFixture(t, m, "old", "")
			path := filepath.Join(old.Root, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("required resource"), 0600); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(path)
			old.Fingerprints[path] = fileIdentity(info)
			if err := m.save(old); err != nil {
				t.Fatal(err)
			}
			owner := testOwner("task")
			binding, err := m.bind(old, owner, true)
			if err != nil {
				t.Fatal(err)
			}
			next := officeStoredFixture(t, m, "next", "")
			if err := m.activateOfficeBundle(context.Background(), next); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			got, err := m.EnsureOfficeBundle(context.Background(), owner, nil)
			if err != nil || got.InstanceID != next.InstanceID || got.EnvironmentRef == binding.EnvironmentRef || len(m.Operations()) != 0 {
				t.Fatalf("runtime data damage not recovered: %+v %v", got, err)
			}
		})
	}
}

func TestOfficeHealthExcludesOnlyKnownNonRuntimeDirectories(t *testing.T) {
	f := Manifest{Kind: "office", Root: t.TempDir()}
	for _, relative := range []string{"dependencies/python/include/Python.h", "dependencies/node/include/node/node.h", "dependencies/node/share/man/man1/node.1"} {
		if !officeUnneededHealthFile(f, filepath.Join(f.Root, filepath.FromSlash(relative))) {
			t.Error("development file still checked", relative)
		}
	}
	for _, relative := range []string{"dependencies/python/lib/templates/theme.xml", "dependencies/python/lib/data.json", "dependencies/python/lib/template.svg", "dependencies/native/libreoffice/ui/dialog.ui", "dependencies/native/libreoffice/help-required.html"} {
		if officeUnneededHealthFile(f, filepath.Join(f.Root, filepath.FromSlash(relative))) {
			t.Error("unknown runtime resource ignored", relative)
		}
	}
}

func TestPartialDownloadCollectionProtectsLiveOwnerAndAssetLock(t *testing.T) {
	options := Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)}
	m := managerForTest(t, options)
	other := managerForTest(t, options)
	hash := strings.Repeat("a", 64)
	file, err := os.CreateTemp(filepath.Join(m.root, "cache", "downloads"), ".download-"+hash+"-"+other.coreID+"-*")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := m.collectPartialDownload(context.Background(), file.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file.Name()); err != nil {
		t.Fatal("live partial removed", err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- m.locked(context.Background(), "asset-"+hash, func() error { close(locked); <-release; return nil })
	}()
	<-locked
	err = m.collectPartialDownload(context.Background(), file.Name())
	_, statErr := os.Stat(file.Name())
	close(release)
	if lockErr := <-done; lockErr != nil {
		t.Fatal(lockErr)
	}
	if err != nil || statErr != nil {
		t.Fatal("locked partial removed", err, statErr)
	}
	if err := m.collectPartialDownload(context.Background(), file.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file.Name()); !os.IsNotExist(err) {
		t.Fatal("inactive partial retained", err)
	}
}

func TestOfficeRollbackExpiresBeforeOrdinaryRetirement(t *testing.T) {
	base := time.Now()
	var days atomic.Int64
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t), Now: func() time.Time { return base.Add(time.Duration(days.Load()) * 24 * time.Hour) }})
	old := officeStoredFixture(t, m, "old", "")
	next := officeStoredFixture(t, m, "next", "")
	for _, f := range []Manifest{old, next} {
		if err := m.activateOfficeBundle(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if removed, err := m.Collect(context.Background()); err != nil || len(removed) != 0 {
		t.Fatal(removed, err)
	}
	days.Store(8)
	if removed, err := m.Collect(context.Background()); err != nil || len(removed) != 0 {
		t.Fatal(removed, err)
	}
	choice, err := m.officeChoice()
	if err != nil || choice.Previous != "" {
		t.Fatal("expired rollback selected", choice, err)
	}
	// A resumed task still protects its old instance beyond rollback expiry.
	owner := testOwner("resumed")
	b, err := m.bind(old, owner, true)
	if err != nil {
		t.Fatal(err)
	}
	days.Store(16)
	if removed, err := m.Collect(context.Background()); err != nil || len(removed) != 0 {
		t.Fatal("active old task collected", removed, err)
	}
	m.ReleaseOwner(owner)
	if _, err := m.Collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	days.Store(24)
	removed, err := m.Collect(context.Background())
	if err != nil || len(removed) != 1 || removed[0] != b.InstanceID {
		t.Fatal("retired Office copy not collected", removed, err)
	}
	if _, err := os.Stat(next.Root); err != nil {
		t.Fatal("current removed", err)
	}
}

func TestCoreReceiptPruningFollowsLeasePruning(t *testing.T) {
	options, _ := fixture(t)
	m := managerForTest(t, options)
	other := managerForTest(t, options)
	b, err := other.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("old-core"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.root, "cores", other.coreID+".json")
	if err := m.pruneCoreRecords(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("receipt removed before leases", err)
	}
	if err := m.pruneInactiveLeases(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.hasLeases(b.InstanceID) {
		t.Fatal("dead lease retained")
	}
	if err := m.pruneCoreRecords(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unused core receipt retained", err)
	}
}

func TestVersionWildcardPrefixComparison(t *testing.T) {
	for _, tc := range []struct {
		version, constraint string
		want                bool
	}{
		{"3.1.5", "!=3.1.*", false}, {"3.2.0", "!=3.1.*", true},
		{"1.2.3.4", "!=1.2.3.*", false}, {"1.2.4", "!=1.2.3.*", true},
		{"1.2.4", "==1.2.3.*", false}, {"1.2.3.4", "==1.2.3.*", true},
	} {
		if got := satisfies(tc.version, tc.constraint); got != tc.want {
			t.Errorf("%s %s = %v", tc.version, tc.constraint, got)
		}
	}
}

func TestOfficeReuseRejectsChangedIdentityAndHonorsExplicitInstaller(t *testing.T) {
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	f := officeStoredFixture(t, m, "installed", "")
	if err := m.activateOfficeBundle(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	_, err := m.Resolve(context.Background(), Request{Requirements: []Requirement{{Ecosystem: "python", Name: "openpyxl"}, {Ecosystem: "component", Name: "uv"}}})
	assertEnvironmentCode(t, err, "environment_missing")
	required, err := m.officeRequirements()
	if err != nil {
		t.Fatal(err)
	}
	required.OfficeBundle.SHA256 = strings.Repeat("b", 64)
	if officeCompatible(f, required) {
		t.Fatal("changed hash accepted")
	}
	required.OfficeBundle.SHA256 = ""
	required.OfficeBundle.Version = "incompatible"
	if officeCompatible(f, required) {
		t.Fatal("changed version accepted without source")
	}
	for _, name := range []string{"python-office", "node-office", "pdf_max", "word.visual_qa", "excel.recalc"} {
		delete(m.catalog.Capabilities, name)
	}
	if _, err := m.officeSpec(); err != nil {
		t.Fatal("Office contract still depends on legacy aliases", err)
	}
}
