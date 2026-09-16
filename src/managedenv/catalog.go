package managedenv

import (
	_ "embed"
	"encoding/json"
	"maps"
	"os"
)

//go:embed assets/catalog.json
var catalogJSON []byte

//go:embed assets/package-locks.json
var packageLocksJSON []byte

type resolvedLocks struct {
	Python  string `json:"python"`
	Node    string `json:"node"`
	Package string `json:"package"`
}

// Curated capabilities ship their full dependency graphs. Arbitrary package
// requests are resolved once and retained under the user's managed root.
func baselineLocks(s Spec) (resolvedLocks, error) {
	var bundle struct {
		SchemaVersion int `json:"schema_version"`
		Entries       []struct {
			Spec Spec `json:"spec"`
			resolvedLocks
		} `json:"entries"`
	}
	if err := json.Unmarshal(packageLocksJSON, &bundle); err != nil {
		return resolvedLocks{}, err
	}
	if bundle.SchemaVersion != 1 {
		return resolvedLocks{}, failure("environment_lock_invalid", "unsupported bundled lock schema")
	}
	for _, entry := range bundle.Entries {
		if maps.Equal(canonicalPython(entry.Spec.Python), canonicalPython(s.Python)) && maps.Equal(entry.Spec.Node, s.Node) {
			return entry.resolvedLocks, nil
		}
	}
	return resolvedLocks{}, nil
}

func BuiltinCatalog() (Catalog, error) {
	var c Catalog
	e := json.Unmarshal(catalogJSON, &c)
	return c, e
}

// LoadCatalog reads only a host-selected resource. Model requests cannot pass
// this path. Production boot uses the embedded catalog unless Main supplies a
// product resource built by the asset packaging pipeline.
func LoadCatalog(path string) (Catalog, error) {
	if path == "" {
		return BuiltinCatalog()
	}
	var c Catalog
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
