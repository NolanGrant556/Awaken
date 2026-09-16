package managedenv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type bindingRecord struct {
	Owner        Owner     `json:"owner"`
	InstanceID   string    `json:"instance_id"`
	Ref          string    `json:"ref"`
	BoundAt      time.Time `json:"bound_at,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	SupersededBy string    `json:"superseded_by,omitempty"`
}
type lease struct {
	CoreID          string    `json:"core_id"`
	Owner           Owner     `json:"owner"`
	CreatedAt       time.Time `json:"created_at"`
	PID             int       `json:"pid,omitempty"`
	ProcessIdentity string    `json:"process_identity,omitempty"`
}

func clone[T any](v T) T {
	b, _ := json.Marshal(v)
	var result T
	_ = json.Unmarshal(b, &result)
	return result
}
func (m *Manager) bindingValue(f Manifest, owner Owner, reused bool) Binding {
	b := Binding{State: "ready", EnvironmentRef: digest([]string{owner.key(), f.InstanceID}), InstanceID: f.InstanceID, Reused: reused, Spec: clone(f.Spec), Executables: clone(f.Executables), Resources: clone(f.Resources), Environment: clone(f.Environment), Owner: owner}
	if f.Spec.OfficeBundle != nil {
		b.BundleVersion = f.Spec.OfficeBundle.Version
	}
	if b.Environment == nil {
		b.Environment = map[string]string{}
	}
	for name, launch := range b.Executables {
		b.Environment["AWAKEN_"+strings.ToUpper(strings.ReplaceAll(name, "-", "_"))] = launch.Executable
	}
	if launch, ok := b.Executables["soffice"]; ok {
		b.Environment["AWAKEN_LIBREOFFICE"] = launch.Executable
	}
	b.Environment["AWAKEN_ENVIRONMENT_FILE"] = filepath.Join(m.root, "bindings", owner.key(), b.EnvironmentRef+".execution.json")
	return b
}

func (m *Manager) bind(f Manifest, owner Owner, reused bool) (Binding, error) {
	var b Binding
	err := m.registry(func(r *registry) error {
		var err error
		b, err = m.bindLocked(r, f.InstanceID, owner, reused)
		return err
	})
	return b, err
}

// The registry lock covers validation, lease acquisition and collection. An
// existing binding is immutable; refreshing the same run is a read-only hit.
func (m *Manager) bindLocked(r *registry, instance string, owner Owner, reused bool) (Binding, error) {
	ref := digest([]string{owner.key(), instance})
	path := filepath.Join(m.root, "bindings", owner.key(), ref+".json")
	var previous bindingRecord
	if err := readJSON(path, &previous); err != nil && !os.IsNotExist(err) {
		return Binding{}, err
	}
	if previous.SupersededBy != "" {
		return Binding{}, failure("environment_binding_stale", "call load_workspace_dependencies to obtain the recovered Office environment")
	}
	f, ok := r.Instances[instance]
	if !ok || !m.healthyTree(f, r.Instances) {
		return Binding{}, failure("environment_binding_stale", "environment is no longer available")
	}
	if err := m.checkRemoval(f, r.Instances, ""); err != nil {
		return Binding{}, err
	}
	created, err := m.ensureLease(instance, owner)
	if err != nil {
		return Binding{}, err
	}
	if created || f.RetiredAt != nil {
		f.LastUsedAt = m.now()
		f.RetiredAt = nil
		r.Instances[instance] = f
	}
	if previous.Ref != ref || previous.InstanceID != instance || previous.Owner.key() != owner.key() || previous.Kind != f.Kind {
		boundAt := previous.BoundAt
		if boundAt.IsZero() {
			boundAt = m.now()
		}
		if err := writeJSON(path, bindingRecord{Owner: owner, InstanceID: instance, Ref: ref, BoundAt: boundAt, Kind: f.Kind}); err != nil {
			return Binding{}, err
		}
	}
	b := m.bindingValue(f, owner, reused)
	// Old callers can still read the saved execution descriptor. Instance
	// paths/spec never change, so create it once instead of rewriting per Bash.
	executionFile := b.Environment["AWAKEN_ENVIRONMENT_FILE"]
	if info, err := os.Stat(executionFile); os.IsNotExist(err) {
		if err := writeJSON(executionFile, b); err != nil {
			return Binding{}, err
		}
	} else if err != nil {
		return Binding{}, err
	} else if !info.Mode().IsRegular() {
		return Binding{}, failure("environment_path_invalid", "invalid execution descriptor")
	}
	return b, nil
}

func (m *Manager) Binding(ctx context.Context, ref string, owner Owner) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	if !owner.valid() || len(ref) != 64 || !safeID.MatchString(ref) {
		return Binding{}, failure("environment_binding_invalid", "invalid environment reference")
	}
	var b Binding
	err := m.registry(func(r *registry) error {
		var record bindingRecord
		if err := readJSON(filepath.Join(m.root, "bindings", owner.key(), ref+".json"), &record); err != nil || record.Ref != ref || record.Owner.key() != owner.key() {
			return failure("environment_binding_invalid", "environment reference does not belong to this Agent")
		}
		var err error
		b, err = m.bindLocked(r, record.InstanceID, owner, true)
		return err
	})
	return b, err
}

// RestoreBinding accepts a host-validated durable context reference. It only
// rebinds a healthy instance; replay and compaction never start installation.
func (m *Manager) RestoreBinding(ctx context.Context, ref, instance string, owner Owner) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	if !owner.valid() || !safeID.MatchString(instance) || ref != digest([]string{owner.key(), instance}) {
		return Binding{}, failure("environment_binding_invalid", "durable reference does not belong to this Agent")
	}
	var b Binding
	err := m.registry(func(r *registry) error { var err error; b, err = m.bindLocked(r, instance, owner, true); return err })
	return b, err
}

// Select the task's recorded Office binding before testing its health. Skipping
// unhealthy bindings here would silently move a task to the current release.
func (m *Manager) boundOfficeReference(owner Owner) (string, error) {
	dir := filepath.Join(m.root, "bindings", owner.key())
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var all []Manifest // Loaded only for pre-kind/pre-descriptor legacy records.
	var selected bindingRecord
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".execution.json") {
			continue
		}
		var record bindingRecord
		if readJSON(filepath.Join(dir, entry.Name()), &record) != nil || record.Ref == "" || record.SupersededBy != "" || record.Owner.key() != owner.key() {
			continue
		}
		isOffice := record.Kind == "office"
		if record.Kind == "" {
			var old Binding
			if readJSON(filepath.Join(dir, record.Ref+".execution.json"), &old) == nil && old.EnvironmentRef == record.Ref && old.InstanceID == record.InstanceID && old.Spec.OfficeBundle != nil {
				isOffice = true
			}
			if !isOffice {
				if all == nil {
					all, err = m.instances()
					if err != nil {
						return "", err
					}
				}
				for _, f := range all {
					if f.InstanceID == record.InstanceID && f.Kind == "office" {
						isOffice = true
						break
					}
				}
			}
		}
		if isOffice && (selected.Ref == "" || record.BoundAt.After(selected.BoundAt) || (record.BoundAt.Equal(selected.BoundAt) && record.Ref < selected.Ref)) {
			selected = record
		}
	}
	return selected.Ref, nil
}

func (m *Manager) Bindings(owner Owner) []Binding {
	if !owner.valid() {
		return nil
	}
	entries, _ := os.ReadDir(filepath.Join(m.root, "bindings", owner.key()))
	all, e := m.instances()
	if e != nil {
		return nil
	}
	out := []Binding{}
	boundAt := map[string]time.Time{}
	instances := map[string]Manifest{}
	for _, f := range all {
		instances[f.InstanceID] = f
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".execution.json") {
			continue
		}
		var r bindingRecord
		if readJSON(filepath.Join(m.root, "bindings", owner.key(), entry.Name()), &r) != nil || r.Ref == "" {
			continue
		}
		f, ok := instances[r.InstanceID]
		if ok && r.SupersededBy == "" && r.Owner.key() == owner.key() && r.Ref == digest([]string{owner.key(), f.InstanceID}) && m.healthyTree(f, instances) {
			boundAt[r.Ref] = r.BoundAt
			out = append(out, m.bindingValue(f, owner, true))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := boundAt[out[i].EnvironmentRef], boundAt[out[j].EnvironmentRef]
		if !left.Equal(right) {
			return left.After(right)
		}
		return out[i].EnvironmentRef < out[j].EnvironmentRef
	})
	return out
}

// Replacement applies only after an explicit prepare call succeeds. The old
// execution file and its process leases remain unchanged for history/in-flight work.
func (m *Manager) supersedeBinding(owner Owner, oldRef, newRef string) error {
	return m.registry(func(r *registry) error {
		dir := filepath.Join(m.root, "bindings", owner.key())
		var old, next bindingRecord
		if err := readJSON(filepath.Join(dir, oldRef+".json"), &old); err != nil {
			return err
		}
		if err := readJSON(filepath.Join(dir, newRef+".json"), &next); err != nil {
			return err
		}
		if old.Owner.key() != owner.key() || next.Owner.key() != owner.key() || old.Ref != oldRef || next.Ref != newRef || next.SupersededBy != "" {
			return failure("environment_binding_invalid", "invalid recovery binding")
		}
		f, ok := r.Instances[next.InstanceID]
		if !ok || !m.healthyTree(f, r.Instances) {
			return failure("environment_binding_stale", "recovery environment is no longer available")
		}
		old.SupersededBy = newRef
		return writeJSON(filepath.Join(dir, oldRef+".json"), old)
	})
}

func (m *Manager) Inherit(parent, child Owner) error {
	bindings := m.Bindings(parent)
	if len(bindings) == 0 {
		return nil
	}
	return m.registry(func(r *registry) error {
		for _, b := range bindings {
			if _, err := m.bindLocked(r, b.InstanceID, child, true); err != nil {
				return err
			}
		}
		return nil
	})
}
func (m *Manager) ensureLease(instance string, owner Owner) (bool, error) {
	path := filepath.Join(m.root, "leases", instance, digest([]any{m.coreID, owner})+".json")
	var existing lease
	if err := readJSON(path, &existing); err == nil && existing.CoreID == m.coreID && existing.Owner == owner && existing.PID == 0 {
		return false, nil
	}
	return true, writeJSON(path, lease{CoreID: m.coreID, Owner: owner, CreatedAt: m.now()})
}
func (m *Manager) writeLease(instance string, owner Owner) error {
	_, err := m.ensureLease(instance, owner)
	return err
}
func (m *Manager) hasLeases(instance string) bool {
	entries, err := os.ReadDir(filepath.Join(m.root, "leases", instance))
	if err != nil && !os.IsNotExist(err) {
		return true
	}
	for _, e := range entries {
		var l lease
		if readJSON(filepath.Join(m.root, "leases", instance, e.Name()), &l) != nil {
			return true
		}
		if !m.leaseInactive(l) {
			return true
		}
	}
	return false
}

// Unknown/corrupt owner metadata cannot prove a lease has expired. A service
// PID can outlive its Core, so both identities must have ended before pruning.
func (m *Manager) leaseInactive(l lease) bool {
	if (l.CoreID == "" && l.PID <= 0) || (l.PID > 0 && l.ProcessIdentity == "") {
		return false
	}
	if l.PID > 0 && l.ProcessIdentity != "" && l.ProcessIdentity == processIdentity(l.PID) && processAlive(l.PID) {
		return false
	}
	if l.CoreID != "" {
		var owner coreRecord
		if !safeID.MatchString(l.CoreID) || readJSON(filepath.Join(m.root, "cores", l.CoreID+".json"), &owner) != nil || owner.ID != l.CoreID {
			return false
		}
		if !owner.Closed && (owner.PID <= 0 || owner.ProcessIdentity == "") {
			return false
		}
		if !owner.Closed && owner.ProcessIdentity != "" && owner.ProcessIdentity == processIdentity(owner.PID) && processAlive(owner.PID) {
			return false
		}
	}
	return true
}
func (m *Manager) ReleaseRun(run string)    { m.releaseLeases(run, "") }
func (m *Manager) ReleaseOwner(owner Owner) { m.releaseLeases(owner.RunID, owner.key()) }

// ForgetThread removes resumable bindings after the runtime has stopped and
// deleted a conversation. Process leases remain independent: a live process
// must still protect its installation even after its conversation is gone.
func (m *Manager) ForgetThread(threadID string) error {
	if strings.TrimSpace(threadID) == "" {
		return nil
	}
	return m.registry(func(_ *registry) error {
		paths, err := filepath.Glob(filepath.Join(m.root, "bindings", "*", "*.json"))
		if err != nil {
			return err
		}
		for _, path := range paths {
			var b bindingRecord
			if readJSON(path, &b) != nil || b.Owner.ThreadID != threadID || filepath.Base(filepath.Dir(path)) != b.Owner.key() {
				continue
			}
			if err := ownedPath(m.root, path); err != nil {
				return err
			}
			if err := os.RemoveAll(filepath.Dir(path)); err != nil {
				return err
			}
		}
		return nil
	})
}
func (m *Manager) releaseLeases(run, ownerKey string) {
	if run == "" {
		return
	}
	_ = m.registry(func(r *registry) error {
		for id := range r.Instances {
			entries, _ := os.ReadDir(filepath.Join(m.root, "leases", id))
			for _, e := range entries {
				p := filepath.Join(m.root, "leases", id, e.Name())
				var l lease
				if readJSON(p, &l) == nil && l.CoreID == m.coreID && l.Owner.RunID == run && (ownerKey == "" || l.Owner.key() == ownerKey) && l.Owner.Kind != "process" {
					_ = os.Remove(p)
				}
			}
		}
		return nil
	})
}
func (m *Manager) HoldProcess(b Binding, pid int) (func(), error) {
	path := filepath.Join(m.root, "leases", b.InstanceID, "process-"+newID()+".json")
	owner := b.Owner
	owner.Kind = "process"
	e := m.registry(func(r *registry) error {
		if f, ok := r.Instances[b.InstanceID]; !ok || !m.healthyTree(f, r.Instances) {
			return failure("environment_binding_stale", "environment is no longer ready")
		}
		if err := m.checkRemoval(r.Instances[b.InstanceID], r.Instances, ""); err != nil {
			return err
		}
		return writeJSON(path, lease{CoreID: m.coreID, Owner: owner, PID: pid, ProcessIdentity: processIdentity(pid), CreatedAt: m.now()})
	})
	return func() { _ = os.Remove(path) }, e
}
func (m *Manager) Executable(name string) (Launch, bool) {
	all, e := m.instances()
	if e != nil {
		return Launch{}, false
	}
	for _, f := range all {
		if m.healthy(f) && (f.Kind != "office" || m.officeSelected(f)) {
			if l, ok := f.Executables[name]; ok {
				return l, true
			}
		}
	}
	return Launch{}, false
}

// ExecutionEnvironment changes only the selected child, never process globals.
func (b Binding) ExecutionEnvironment(base []string) []string {
	values := map[string]string{}
	for _, entry := range base {
		if i := strings.IndexByte(entry, '='); i > 0 {
			values[environmentKey(entry[:i])] = entry[i+1:]
		}
	}
	for _, key := range []string{"PYTHONHOME", "PYTHONPATH", "VIRTUAL_ENV", "NODE_PATH", "NODE_OPTIONS", "AWAKEN_LIBREOFFICE", "AWAKEN_PDFTOPPM", "AWAKEN_OFFICE_BUNDLE_ROOT", "AWAKEN_OFFICE_BUNDLE_VERSION", "AWAKEN_OVERRIDE_BIN", "AWAKEN_FALLBACK_BIN"} {
		delete(values, key)
	}
	if b.Spec.OfficeBundle != nil {
		// A caller's bytecode directory must not redirect imports outside the bundle.
		delete(values, "PYTHONPYCACHEPREFIX")
	}
	paths := []string{}
	for _, l := range b.Executables {
		paths = append(paths, filepath.Dir(l.Executable))
	}
	sort.Strings(paths)
	paths = unique(paths)
	values["PATH"] = strings.Join(append(paths, values["PATH"]), string(os.PathListSeparator))
	if b.Spec.OfficeBundle != nil {
		values["PATH"] = strings.Join([]string{b.Environment["AWAKEN_OVERRIDE_BIN"], values["PATH"], b.Environment["AWAKEN_FALLBACK_BIN"]}, string(os.PathListSeparator))
	}
	for k, v := range b.Environment {
		values[environmentKey(k)] = v
	}
	values["PYTHONNOUSERSITE"] = "1"
	values["PYTHONUTF8"] = "1"
	values["PYTHONIOENCODING"] = "utf-8"
	values["AWAKEN_ENVIRONMENT_REF"] = b.EnvironmentRef
	out := []string{}
	for k, v := range values {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// AcquireExecutable keeps a read-only tool's component alive for its owning run.
func (m *Manager) AcquireExecutable(name string, owner Owner) (Launch, bool) {
	if !owner.valid() {
		return Launch{}, false
	}
	var found Launch
	err := m.registry(func(r *registry) error {
		var selected Manifest
		var boundAt time.Time
		hasOwnedExecutable := false
		entries, _ := os.ReadDir(filepath.Join(m.root, "bindings", owner.key()))
		for _, entry := range entries {
			var record bindingRecord
			if readJSON(filepath.Join(m.root, "bindings", owner.key(), entry.Name()), &record) != nil || record.Owner.key() != owner.key() || record.Ref == "" || record.SupersededBy != "" {
				continue
			}
			f, ok := r.Instances[record.InstanceID]
			if !ok {
				continue
			}
			if _, ok := f.Executables[name]; !ok {
				continue
			}
			hasOwnedExecutable = true
			if m.healthyTree(f, r.Instances) && (selected.InstanceID == "" || record.BoundAt.After(boundAt)) {
				selected, boundAt = f, record.BoundAt
			}
		}
		if !hasOwnedExecutable {
			for _, f := range r.Instances {
				if f.Kind == "office" && !m.officeSelected(f) {
					continue
				}
				if _, ok := f.Executables[name]; ok && m.healthyTree(f, r.Instances) && (selected.InstanceID == "" || f.CreatedAt.After(selected.CreatedAt)) {
					selected = f
				}
			}
		}
		if selected.InstanceID == "" {
			return failure("environment_missing", name)
		}
		if err := m.checkRemoval(selected, r.Instances, ""); err != nil {
			return err
		}
		if err := m.writeLease(selected.InstanceID, owner); err != nil {
			return err
		}
		if !hasOwnedExecutable {
			ref := digest([]string{owner.key(), selected.InstanceID})
			if err := writeJSON(filepath.Join(m.root, "bindings", owner.key(), ref+".json"), bindingRecord{Owner: owner, InstanceID: selected.InstanceID, Ref: ref, BoundAt: m.now(), Kind: selected.Kind}); err != nil {
				return err
			}
		}
		found = selected.Executables[name]
		return nil
	})
	return found, err == nil
}

func (m *Manager) ManagedGitFiles(executable string) (string, string, bool) {
	all, err := m.instances()
	if err != nil {
		return "", "", false
	}
	for _, f := range all {
		git, ok := f.Executables["git"]
		helper, hasHelper := f.Executables["git-remote-https"]
		if ok && hasHelper && git.Executable == executable && f.Kind == "component" && f.ComponentID == "git" && f.Source == "awaken-managed" && m.healthy(f) && inside(f.Root, helper.Executable) {
			return filepath.Dir(helper.Executable), helper.Executable, true
		}
	}
	return "", "", false
}

func environmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

// Settings preparation has no resumable conversation. Drop only this temporary
// host identity after the request; ordinary task bindings remain durable.
func (m *Manager) ReleaseControlOwner(owner Owner) {
	if owner.Kind != "control" || !owner.valid() {
		return
	}
	m.ReleaseOwner(owner)
	_ = os.RemoveAll(filepath.Join(m.root, "bindings", owner.key()))
}
