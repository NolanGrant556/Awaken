package managedenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
)

func TestStorageFailurePreservesHealthyEnvironmentAndRecoversManifest(t *testing.T) {
	for _, stage := range []string{"manifest", "index"} {
		for _, cause := range []error{syscall.ENOSPC, os.ErrPermission} {
			t.Run(stage+"/"+cause.Error(), func(t *testing.T) {
				o, downloads := fixture(t)
				o.Catalog.Components["extra"] = o.Catalog.Components["fixture"]
				o.Catalog.Capabilities["extended"] = Spec{Components: []string{"fixture", "extra"}}
				m := managerForTest(t, o)
				base, err := m.Ensure(context.Background(), Request{Capability: "fixture"}, testOwner("base"), nil)
				if err != nil {
					t.Fatal(err)
				}
				var enabled atomic.Bool
				enabled.Store(true)
				m.persistRecord = func(path string, value any) error {
					if enabled.Load() {
						shouldFail := false
						if f, ok := value.(Manifest); ok && stage == "manifest" && f.Kind == "environment" && len(f.Spec.Components) == 2 {
							shouldFail = true
						}
						if r, ok := value.(registry); ok && stage == "index" {
							for _, f := range r.Instances {
								if f.Kind == "environment" && len(f.Spec.Components) == 2 {
									shouldFail = true
								}
							}
						}
						if shouldFail {
							return &os.PathError{Op: "write", Path: path, Err: cause}
						}
					}
					return writeJSON(path, value)
				}
				_, err = m.Ensure(context.Background(), Request{Capability: "extended"}, testOwner("extended"), nil)
				var typed *Error
				if !errors.As(err, &typed) || (typed.Code != "environment_disk_full" && typed.Code != "environment_permission_denied") {
					t.Fatalf("unclassified storage failure: %v", err)
				}
				enabled.Store(false)
				if _, err = m.Binding(context.Background(), base.EnvironmentRef, testOwner("base")); err != nil {
					t.Fatal("healthy instance was damaged", err)
				}
				before := downloads.Load()
				again, err := m.Ensure(context.Background(), Request{Capability: "extended"}, testOwner("retry"), nil)
				if err != nil {
					t.Fatal(err)
				}
				if stage == "index" && !again.Reused {
					t.Fatal("committed ready manifest was reinstalled")
				}
				if downloads.Load() != before {
					t.Fatal("retry discarded a healthy component cache")
				}
				if _, err = os.Stat(filepath.Join(m.root, "registry.json")); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
