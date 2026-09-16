package managedenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type removalReservation struct {
	Token           string `json:"token"`
	PID             int    `json:"pid"`
	ProcessIdentity string `json:"process_identity"`
}

func removableComponent(id string) bool { return id == "dingtalk-workspace" || id == "lark-cli" }

func (m *Manager) componentPreparing(id string) bool {
	entries, err := os.ReadDir(filepath.Join(m.root, "component-uses", id))
	if err != nil && !os.IsNotExist(err) {
		return true
	}
	for _, entry := range entries {
		var use lease
		if readJSON(filepath.Join(m.root, "component-uses", id, entry.Name()), &use) != nil || m.coreAlive(use.CoreID) {
			return true
		}
	}
	return false
}

func (m *Manager) holdComponentPreparation(id string) (func(), error) {
	path := filepath.Join(m.root, "component-uses", id, newID()+".json")
	err := m.registry(func(r *registry) error {
		if err := m.checkRemoval(Manifest{ComponentID: id}, r.Instances, ""); err != nil {
			return err
		}
		return writeJSON(path, lease{CoreID: m.coreID, CreatedAt: m.now(), Owner: Owner{Kind: "preparation"}})
	})
	return func() { _ = os.Remove(path) }, err
}

func (m *Manager) removal(id string) (removalReservation, error) {
	var reservation removalReservation
	if !removableComponent(id) {
		return reservation, nil
	}
	err := readJSON(filepath.Join(m.root, "removals", id+".json"), &reservation)
	if os.IsNotExist(err) {
		return removalReservation{}, nil
	}
	if err != nil {
		return reservation, failure("environment_removal_invalid", "卸载占用记录不可读")
	}
	if reservation.PID <= 0 || reservation.ProcessIdentity == "" || reservation.ProcessIdentity != processIdentity(reservation.PID) || !processAlive(reservation.PID) {
		return removalReservation{}, nil
	}
	return reservation, nil
}

// Called while holding the registry lock, so acquisition and host uninstall
// cannot pass each other between preflight and logout/deletion.
func (m *Manager) checkRemoval(f Manifest, instances map[string]Manifest, token string) error {
	ids := []string{f.ComponentID}
	ids = append(ids, f.Spec.Components...)
	for _, key := range f.Components {
		ids = append(ids, instances[key].ComponentID)
	}
	for _, id := range ids {
		reservation, err := m.removal(id)
		if err != nil {
			return err
		}
		if reservation.Token != "" && reservation.Token != token {
			return failure("environment_removing", "运行组件正在卸载，请稍后重试")
		}
	}
	return nil
}

func (m *Manager) ReserveRemoval(ctx context.Context, id string, pid int) (string, error) {
	if !removableComponent(id) || pid <= 0 || !processAlive(pid) {
		return "", failure("environment_request_invalid", "invalid component removal owner")
	}
	if err := m.migrateCLIOwnership(ctx); err != nil {
		return "", err
	}
	token := newID()
	err := m.registry(func(r *registry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.checkRemoval(Manifest{ComponentID: id}, r.Instances, ""); err != nil {
			return err
		}
		if err := m.checkComponentIdle(r, id); err != nil {
			return err
		}
		for _, f := range r.Instances {
			if f.ComponentID == id {
				for path, hash := range f.OwnedFiles {
					if err := m.validateOwnedCLIFile(id, path, hash); err != nil {
						return err
					}
				}
			}
		}
		return writeJSON(filepath.Join(m.root, "removals", id+".json"), removalReservation{Token: token, PID: pid, ProcessIdentity: processIdentity(pid)})
	})
	return token, err
}

func (m *Manager) ReleaseRemoval(id, token string) error {
	if !removableComponent(id) || len(token) != 32 || !safeID.MatchString(token) {
		return failure("environment_request_invalid", "invalid removal reference")
	}
	return m.registry(func(r *registry) error {
		var reservation removalReservation
		path := filepath.Join(m.root, "removals", id+".json")
		if err := readJSON(path, &reservation); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		if reservation.Token != token {
			return failure("environment_removal_invalid", "removal owner changed")
		}
		return os.Remove(path)
	})
}

// Main supplies its own PID before spawning a command. The lease survives Core
// restart while Main is alive, and is explicitly released on exit/start failure.
func (m *Manager) HoldHostExecutable(executable string, pid int, removalToken string) (string, error) {
	if pid <= 0 || !processAlive(pid) || !filepath.IsAbs(executable) {
		return "", failure("environment_request_invalid", "invalid host process")
	}
	if err := m.migrateCLIOwnership(m.ctx); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	ref := ""
	err = m.registry(func(r *registry) error {
		for _, f := range r.Instances {
			for _, launch := range f.Executables {
				candidate, err := filepath.EvalSymlinks(launch.Executable)
				if err != nil || !sameHostPath(candidate, resolved) {
					continue
				}
				if err := m.checkRemoval(f, r.Instances, removalToken); err != nil {
					return err
				}
				if !m.healthyTree(f, r.Instances) {
					return failure("environment_binding_stale", "host component needs repair")
				}
				ref = f.InstanceID + "." + newID()
				return writeJSON(filepath.Join(m.root, "leases", f.InstanceID, "host-"+strings.Split(ref, ".")[1]+".json"), lease{Owner: Owner{Kind: "process", ThreadID: "desktop-host", AgentID: "desktop"}, PID: pid, ProcessIdentity: processIdentity(pid), CreatedAt: m.now()})
			}
		}
		if inside(m.root, resolved) {
			return failure("environment_binding_stale", "managed executable is not registered")
		}
		return nil // External user installation retains its existing ownership.
	})
	return ref, err
}

func (m *Manager) ReleaseHostExecutable(ref string) error {
	parts := strings.Split(ref, ".")
	if len(parts) != 2 || len(parts[0]) != 32 || len(parts[1]) != 32 || !safeID.MatchString(parts[0]) || !safeID.MatchString(parts[1]) {
		return failure("environment_request_invalid", "invalid host usage reference")
	}
	path := filepath.Join(m.root, "leases", parts[0], "host-"+parts[1]+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
