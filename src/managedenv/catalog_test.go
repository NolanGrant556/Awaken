package managedenv

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCuratedCapabilitiesHaveCompleteBaselineLocks(t *testing.T) {
	c, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for name := range c.Capabilities {
		t.Run(name, func(t *testing.T) {
			s, err := c.Resolve(Request{Capability: name})
			if err != nil {
				t.Fatal(err)
			}
			locks, err := baselineLocks(s)
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Python) > 0 && !strings.Contains(locks.Python, "--hash=sha256:") {
				t.Fatal("missing hashed Python graph")
			}
			if len(s.Node) > 0 {
				var lock struct {
					LockfileVersion int `json:"lockfileVersion"`
					Packages        map[string]struct {
						Integrity string `json:"integrity"`
					} `json:"packages"`
				}
				if err := json.Unmarshal([]byte(locks.Node), &lock); err != nil {
					t.Fatal(err)
				}
				if lock.LockfileVersion != 3 {
					t.Fatal("unsupported npm lock")
				}
				for pkg := range s.Node {
					if lock.Packages["node_modules/"+pkg].Integrity == "" {
						t.Fatalf("%s lacks integrity", pkg)
					}
				}
			}
		})
	}
}

func TestCustomDependencyDoesNotUseUnrelatedBaseline(t *testing.T) {
	lock, err := baselineLocks(Spec{Python: map[string]string{"openpyxl": "1.0.0"}})
	if err != nil || lock.Python != "" || lock.Node != "" {
		t.Fatalf("wrong baseline: %+v %v", lock, err)
	}
}

func TestPythonNameAndExtraOrderDoNotCauseReinstallation(t *testing.T) {
	have := Spec{Python: map[string]string{"markitdown[xlsx,pptx]": "0.1.5", "python_docx": "1.2.0"}}
	want := Spec{Python: map[string]string{"markitdown[pptx,xlsx]": "0.1.5", "python-docx": "1.2.0"}}
	if !specContains(have, want) {
		t.Fatal("equivalent installed package names fail reuse")
	}
}
