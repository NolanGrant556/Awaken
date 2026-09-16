package managedenv

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/filestore"
)

type registry struct {
	SchemaVersion int                 `json:"schema_version"`
	Instances     map[string]Manifest `json:"instances"`
}

func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func writeJSON(path string, v any) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return writeAtomicRecord(path, append(b, '\n'))
}
func (m *Manager) persist(path string, value any) error {
	if m.persistRecord != nil {
		return m.persistRecord(path, value)
	}
	return writeJSON(path, value)
}
func inside(root, path string) bool {
	r, e := filepath.Rel(root, path)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}
func ownedPath(root, path string) error {
	if !inside(root, path) {
		return failure("environment_path_invalid", "path is outside managed root")
	}
	for p := path; inside(root, p); p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return failure("environment_path_invalid", "managed path contains a symbolic link")
		}
		if p == root {
			break
		}
	}
	return nil
}
func (m *Manager) locked(ctx context.Context, key string, fn func() error) error {
	return filestore.WithPathLockAt(ctx, filepath.Join(m.root, "locks"), filepath.Join(m.root, key), fn)
}

// Only mutable usage data and manifest locations belong in the index. Program
// versions, launch paths and file fingerprints have one source: manifest.json.
// Schema 4 prevents older Core versions from discarding imported CLI ownership.
const registrySchemaVersion = 4

type registryEntry struct {
	Root       string     `json:"root"`
	LastUsedAt time.Time  `json:"last_used_at"`
	RetiredAt  *time.Time `json:"retired_at,omitempty"`
}

func (r registry) MarshalJSON() ([]byte, error) {
	entries := make(map[string]registryEntry, len(r.Instances))
	for id, f := range r.Instances {
		entries[id] = registryEntry{f.Root, f.LastUsedAt, f.RetiredAt}
	}
	return json.Marshal(struct {
		SchemaVersion int                      `json:"schema_version"`
		Instances     map[string]registryEntry `json:"instances"`
	}{registrySchemaVersion, entries})
}
func (m *Manager) registry(fn func(*registry) error) error {
	return m.withRegistry(false, fn)
}
func (m *Manager) recoverRegistry() error {
	return m.withRegistry(true, func(*registry) error { return nil })
}
func (m *Manager) withRegistry(scan bool, fn func(*registry) error) error {
	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
	defer cancel()
	return m.locked(ctx, "registry.json", func() error {
		path := filepath.Join(m.root, "registry.json")
		if err := ownedPath(m.root, path); err != nil {
			return err
		}
		var r registry
		err := readJSON(path, &r)
		migrate := err == nil && r.SchemaVersion != registrySchemaVersion
		if err == nil && r.SchemaVersion != SchemaVersion && r.SchemaVersion != officeRegistrySchema && r.SchemaVersion != 3 && r.SchemaVersion != registrySchemaVersion {
			return failure("environment_schema_unsupported", "installed registry has an unsupported schema")
		}
		if err != nil {
			var syntax *json.SyntaxError
			var shape *json.UnmarshalTypeError
			if !errors.Is(err, fs.ErrNotExist) && !errors.As(err, &syntax) && !errors.As(err, &shape) {
				return err
			}
			if !errors.Is(err, fs.ErrNotExist) {
				if err := os.Rename(path, path+".invalid-"+newID()); err != nil {
					return err
				}
			}
			r = registry{}
			scan = true
		}
		if r.Instances == nil {
			r.Instances = map[string]Manifest{}
			scan = true
		}
		original := digest(r)
		for id, entry := range r.Instances {
			if !safeID.MatchString(id) {
				return failure("environment_path_invalid", "invalid indexed instance")
			}
			if err := ownedPath(m.root, entry.Root); err != nil {
				return err
			}
			stored, err := m.readManifest(entry.Root)
			if err != nil || stored.InstanceID != id {
				// Retain the location so an unavailable instance stays known;
				// it cannot satisfy a lookup and will be repaired on demand.
				r.Instances[id] = Manifest{InstanceID: id, Root: entry.Root, LastUsedAt: entry.LastUsedAt, RetiredAt: entry.RetiredAt}
				continue
			}
			stored.LastUsedAt, stored.RetiredAt = entry.LastUsedAt, entry.RetiredAt
			r.Instances[id] = stored
		}
		if scan {
			for _, pattern := range []string{"components/*/*/manifest.json", "environments/*/*/manifest.json", "office/instances/*/*/*/manifest.json"} {
				paths, err := filepath.Glob(filepath.Join(m.root, filepath.FromSlash(pattern)))
				if err != nil {
					return err
				}
				for _, path := range paths {
					if err := ownedPath(m.root, path); err != nil {
						return err
					}
					f, err := m.readManifest(filepath.Dir(path))
					if err != nil || !safeID.MatchString(f.InstanceID) {
						continue
					}
					if previous, exists := r.Instances[f.InstanceID]; !exists || previous.State != "ready" {
						r.Instances[f.InstanceID] = f
					}
				}
			}
		}
		r.SchemaVersion = registrySchemaVersion
		if err := fn(&r); err != nil {
			return err
		}
		if scan || migrate || digest(r) != original {
			return m.persist(path, r)
		}
		return nil
	})
}
func (m *Manager) save(f Manifest) error {
	if err := ownedPath(m.root, f.Root); err != nil {
		return err
	}
	if err := m.persist(filepath.Join(f.Root, "manifest.json"), f); err != nil {
		return err
	}
	err := m.registry(func(r *registry) error { r.Instances[f.InstanceID] = f; return nil })
	if err == nil {
		_ = os.Remove(filepath.Join(f.Root, "preparation.json"))
	}
	return err
}
func (m *Manager) instances() ([]Manifest, error) {
	out := []Manifest{}
	e := m.registry(func(r *registry) error {
		for _, f := range r.Instances {
			out = append(out, f)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, e
}
func (m *Manager) healthy(f Manifest) bool {
	for path := range f.OwnedFiles {
		if !m.ownedCLIFileAllowed(f, path) {
			return false
		}
	}
	if f.Kind == "office" && (f.Spec.OfficeBundle == nil || f.Spec.OfficeBundle.LayoutVersion != 1) {
		return false
	}
	if !officeBackgroundReady(f) {
		return false
	}
	if len(f.Spec.Node) > 0 && f.NodeLauncherVersion != 3 {
		return false
	}
	if f.State != "ready" || f.Target != m.target || f.SchemaVersion != SchemaVersion {
		return false
	}
	if f.Source == "awaken-managed" {
		if ownedPath(m.root, f.Root) != nil {
			return false
		}
		var stored Manifest
		var err error
		stored, err = m.readManifest(f.Root)
		if err != nil || stored.State != "ready" || stored.InstanceID != f.InstanceID {
			return false
		}
	}
	for path, want := range f.Fingerprints {
		if officeUnneededHealthFile(f, path) {
			continue
		}
		info, e := os.Stat(path)
		if e != nil || !info.Mode().IsRegular() || fileIdentity(info) != want {
			return false
		}
	}
	for _, launch := range f.Executables {
		info, e := os.Stat(launch.Executable)
		if e != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return len(f.Executables) > 0 || len(f.Fingerprints) > 0
}

// Python may refresh bytecode even during a read-only business task. Existing
// Office manifests included these generated caches, so ignore their metadata
// during health checks as well as excluding them from newly recorded files.
// Source modules, native libraries and launchers remain fingerprinted.
func officePythonBytecodeCache(f Manifest, path string) bool {
	if f.Kind != "office" || filepath.Ext(path) != ".pyc" {
		return false
	}
	root := filepath.Join(f.Root, "dependencies", "python")
	rel, err := filepath.Rel(root, path)
	return err == nil && inside(root, path) && contains(strings.Split(filepath.Dir(rel), string(filepath.Separator)), "__pycache__")
}

// Exclude only known development/documentation directories. Runtime data such
// as JSON, XML, Office templates, ONNX models, RDB/XCD and fonts stay tracked.
// The same rule applies to old manifests; no installed manifest is rewritten.
func officeUnneededHealthFile(f Manifest, path string) bool {
	if officePythonBytecodeCache(f, path) {
		return true
	}
	prefix := f.Root + string(filepath.Separator)
	if f.Kind != "office" || !strings.HasPrefix(path, prefix) {
		return false
	}
	rel := filepath.ToSlash(strings.TrimPrefix(path, prefix))
	for _, prefix := range []string{"dependencies/python/include/", "dependencies/node/include/", "dependencies/python/share/man/", "dependencies/node/share/man/", "dependencies/native/poppler/share/man/"} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}
func fileIdentity(f os.FileInfo) string {
	return digest([]any{f.Size(), f.ModTime().UnixNano(), f.Mode().String()})
}
func fingerprints(exes map[string]Launch) map[string]string {
	out := map[string]string{}
	for _, v := range exes {
		for _, p := range append([]string{v.Executable}, v.Args...) {
			if filepath.IsAbs(p) {
				if info, e := os.Stat(p); e == nil && info.Mode().IsRegular() {
					out[p] = fileIdentity(info)
				}
			}
		}
	}
	return out
}

// Collect marks retired instances under the same lock used for lease creation.
// No external resources or incomplete/unrecognised directories are deleted.
func (m *Manager) Collect(ctx context.Context) ([]string, error) {
	removed := []string{}
	err := m.registry(func(r *registry) error {
		keep := map[string]bool{}
		groups := map[string][]Manifest{}
		selected := map[string]officeSelection{}
		if err := readJSON(filepath.Join(m.root, "office", "current.json"), &selected); err != nil && !os.IsNotExist(err) {
			return err // Do not collect while the explicit selection is unreadable.
		}
		if selected == nil {
			return failure("environment_validation_failed", "office selection is not an object")
		}
		selectionChanged := false
		for target, s := range selected {
			if s.Previous != "" && s.PreviousUntil == nil {
				s = m.newOfficeChoice(s.Current, s.Previous)
				selected[target] = s
				selectionChanged = true
			}
			if s.Previous != "" && s.PreviousUntil != nil && !m.now().Before(*s.PreviousUntil) {
				s.Previous = ""
				s.PreviousUntil = nil
				selected[target] = s
				selectionChanged = true
			}
			keep[s.Current], keep[s.Previous] = true, true
		}
		if selectionChanged {
			if err := m.persist(filepath.Join(m.root, "office", "current.json"), selected); err != nil {
				return err
			}
		}
		for id, f := range r.Instances {
			if len(f.OwnedFiles) > 0 || f.Source != "awaken-managed" || m.hasLeases(id) || m.checkRemoval(f, r.Instances, "") != nil || (f.Kind == "component" && m.componentPreparing(f.ComponentID)) {
				keep[id] = true
			}
			key := f.Kind + ":" + f.ComponentID + ":" + familyKey(f.Spec) + ":" + f.Target
			if f.Kind != "office" {
				groups[key] = append(groups[key], f)
			}
		}
		for _, group := range groups {
			sort.Slice(group, func(i, j int) bool { return group[i].CreatedAt.After(group[j].CreatedAt) })
			n := 0
			for _, f := range group {
				if m.healthy(f) && n < 2 {
					keep[f.InstanceID] = true
					n++
				}
			}
		}
		var protect func(string)
		protect = func(id string) {
			for _, child := range r.Instances[id].Components {
				if !keep[child] {
					keep[child] = true
					protect(child)
				}
			}
		}
		for id := range keep {
			protect(id)
		}
		for id, f := range r.Instances {
			if e := ctx.Err(); e != nil {
				return e
			}
			if keep[id] {
				if f.RetiredAt != nil {
					f.RetiredAt = nil
					r.Instances[id] = f
				}
				continue
			}
			if f.RetiredAt == nil {
				now := m.now()
				f.RetiredAt = &now
				r.Instances[id] = f
				continue
			}
			if m.now().Sub(*f.RetiredAt) < retiredInstanceGrace {
				continue
			}
			if ownedPath(m.root, f.Root) != nil {
				continue
			}
			if e := os.RemoveAll(f.Root); e != nil {
				continue
			}
			delete(r.Instances, id)
			removed = append(removed, id)
		}
		return nil
	})
	return removed, err
}

// Dependencies are checked while the registry lock protects binding and collection.
func (m *Manager) healthyTree(f Manifest, instances map[string]Manifest) bool {
	if !m.reusable(f) {
		return false
	}
	for _, id := range f.Components {
		c, ok := instances[id]
		if !ok || c.Kind != "component" || !m.reusable(c) {
			return false
		}
	}
	return true
}

func familyKey(spec Spec) string {
	names := Spec{Components: spec.Components, Python: map[string]string{}, Node: map[string]string{}}
	for name := range spec.Python {
		names.Python[name] = ""
	}
	for name := range spec.Node {
		names.Node[name] = ""
	}
	return digest(names)
}
