package managedenv

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOfficePythonSubsetReuse(t *testing.T) {
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	f := officeStoredFixture(t, m, "review-bundle", "")
	if err := m.activateOfficeBundle(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	req := Request{Requirements: []Requirement{{Ecosystem: "python", Name: "openpyxl", Version: f.Spec.Python["openpyxl"]}}}
	spec, err := m.catalog.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("bundle components=%v; request components=%v; package versions bundle=%s request=%s", f.Spec.Components, spec.Components, f.Spec.Python["openpyxl"], spec.Python["openpyxl"])
	got, err := m.Resolve(context.Background(), req)
	if err != nil || got.InstanceID != f.InstanceID {
		t.Fatalf("healthy Office bundle already contains the exact requested Python package, but reuse failed: %v", err)
	}
	binding, err := m.Ensure(context.Background(), req, testOwner("python-subset"), nil)
	if err != nil || binding.InstanceID != f.InstanceID || !binding.Reused || len(m.Operations()) != 0 {
		t.Fatalf("covered package request started another preparation: %+v %v", binding, err)
	}
}

func TestOfficeDamagedSelectionRecovery(t *testing.T) {
	for _, damaged := range []string{"{broken", "null", `{"darwin-arm64":{"current":"../escape"}}`, `{"darwin-arm64":{"previous_until":123}}`} {
		t.Run(damaged, func(t *testing.T) {
			m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
			f := officeStoredFixture(t, m, "review-selection", "")
			if err := m.activateOfficeBundle(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(m.root, "office", "current.json"), []byte(damaged), 0600); err != nil {
				t.Fatal(err)
			}
			b, err := m.EnsureOfficeBundle(context.Background(), testOwner("new-task"), nil)
			if err != nil || b.InstanceID != f.InstanceID || !b.Reused {
				t.Fatalf("healthy bundle recovery: %+v %v", b, err)
			}
			choice, err := m.officeChoice()
			if err != nil || choice.Current != f.InstanceID {
				t.Fatalf("selection recovery: %+v %v", choice, err)
			}
			preserved, _ := filepath.Glob(filepath.Join(m.root, "office", "current.json.invalid-*"))
			if len(preserved) != 1 || len(m.Operations()) != 0 {
				t.Fatal("recovery must preserve corrupt record without starting installation")
			}
		})
	}
}

func TestAbandonedLegacyDownloadCleanup(t *testing.T) {
	base := time.Now()
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t), Now: func() time.Time { return base.Add(31 * 24 * time.Hour) }})
	// This is the exact file namespace left by CreateTemp when Core exits
	// without returning through download's explicit removal paths.
	file, err := os.CreateTemp(filepath.Join(m.root, "cache", "downloads"), ".download-*")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if _, err := file.WriteString("interrupted download"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := m.Maintain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("31-day-old partial download is retained by maintenance: %s", filepath.Base(path))
	}
}

func TestWildcardVersionExclusion(t *testing.T) {
	c, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Requirements: []Requirement{{Ecosystem: "python", Name: "openpyxl", Version: "!=3.1.*"}}}
	want, err := c.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	have := want
	have.Python = map[string]string{"openpyxl": "3.1.5"}
	if specContains(have, want) {
		t.Fatal("3.1.5 was accepted against !=3.1.*")
	}
}

func TestRepeatedBindingDoesNotRewriteRecords(t *testing.T) {
	m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
	f := officeStoredFixture(t, m, "review-writes", "")
	owner := testOwner("same-task")
	b, err := m.bind(f, owner, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Binding(context.Background(), b.EnvironmentRef, owner); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"registry":              filepath.Join(m.root, "registry.json"),
		"binding":               filepath.Join(m.root, "bindings", owner.key(), b.EnvironmentRef+".json"),
		"execution description": filepath.Join(m.root, "bindings", owner.key(), b.EnvironmentRef+".execution.json"),
		"run lease":             filepath.Join(m.root, "leases", b.InstanceID, digest([]any{m.coreID, owner})+".json"),
	}
	before := map[string]os.FileInfo{}
	for name, path := range paths {
		before[name], err = os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Binding(context.Background(), b.EnvironmentRef, owner); err != nil {
		t.Fatal(err)
	}
	for name, path := range paths {
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before[name], after) {
			t.Errorf("same run rewrote %s", name)
		}
	}
}

type diskFullReader struct{}

func (diskFullReader) Read([]byte) (int, error) {
	return 0, &os.PathError{Op: "write", Path: "injected-download-target", Err: syscall.ENOSPC}
}

func TestDownloadPreservesDiskError(t *testing.T) {
	options, _ := fixture(t)
	transfers := 0
	// Inject the same error that io.Copy returns when its file writer runs out
	// of disk; no real disk is filled and the HTTP transport never connects.
	options.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		transfers++
		return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(diskFullReader{}), Header: make(http.Header)}, nil
	})}
	m := managerForTest(t, options)
	asset := options.Catalog.Components["fixture"].Assets[Target()]
	op := Operation{ID: strings.Repeat("d", 64), Attempt: newID(), CoreID: m.coreID}
	_, err := m.download(context.Background(), asset, &op)
	err = classifyPreparationError(err)
	typed, ok := err.(*Error)
	if !ok || typed.Code != "environment_disk_full" || transfers != 1 {
		t.Fatalf("injected ENOSPC lost: error=%v, transfers=%d", err, transfers)
	}
}

func TestExistingOfficeReuseWithoutDownloadMetadata(t *testing.T) {
	for _, mode := range []string{"missing_url", "absent_asset"} {
		t.Run(mode, func(t *testing.T) {
			m := managerForTest(t, Options{Root: t.TempDir(), Catalog: officeCatalogForTest(t)})
			f := officeStoredFixture(t, m, "review-existing-office", "")
			if err := m.activateOfficeBundle(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			oldOwner := testOwner("old-session")
			if _, err := m.EnsureOfficeBundle(context.Background(), oldOwner, nil); err != nil {
				t.Fatal(err)
			}
			if mode == "missing_url" {
				asset := m.catalog.OfficeBundle.Assets[Target()]
				asset.URL = ""
				m.catalog.OfficeBundle.Assets[Target()] = asset
			} else {
				delete(m.catalog.OfficeBundle.Assets, Target())
			}
			old, err := m.EnsureOfficeBundle(context.Background(), oldOwner, nil)
			if err != nil || old.InstanceID != f.InstanceID {
				t.Fatalf("old binding failed: %v", err)
			}
			t.Log("old session still reuses healthy instance")
			next, err := m.EnsureOfficeBundle(context.Background(), testOwner("new-session"), nil)
			if err != nil || next.InstanceID != f.InstanceID {
				t.Fatalf("same version/layout healthy instance not reusable by new session: %v", err)
			}
		})
	}
}
