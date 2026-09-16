package managedenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type preparationMarker struct {
	CoreID    string    `json:"core_id"`
	CreatedAt time.Time `json:"created_at"`
}

const retiredInstanceGrace = 7 * 24 * time.Hour

func (m *Manager) markPreparation(root string) error {
	return writeJSON(filepath.Join(root, "preparation.json"), preparationMarker{CoreID: m.coreID, CreatedAt: m.now()})
}

func (m *Manager) discardUnpublished(root string) {
	var f Manifest
	if readJSON(filepath.Join(root, "manifest.json"), &f) == nil && f.State == "ready" {
		return
	}
	if ownedPath(m.root, root) == nil {
		_ = os.RemoveAll(root)
	}
}

// Maintenance never starts preparation and never follows unknown directories.
func (m *Manager) Maintain(ctx context.Context) error {
	if err := m.pruneInactiveLeases(ctx); err != nil {
		return err
	}
	if _, err := m.Collect(ctx); err != nil {
		return err
	}
	for _, pattern := range []string{"components/*/*/preparation.json", "environments/*/*/preparation.json", "office/instances/*/*/*/preparation.json"} {
		paths, _ := filepath.Glob(filepath.Join(m.root, filepath.FromSlash(pattern)))
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return err
			}
			var marker preparationMarker
			if readJSON(path, &marker) != nil || m.coreAlive(marker.CoreID) || m.now().Sub(marker.CreatedAt) < 24*time.Hour {
				continue
			}
			m.discardUnpublished(filepath.Dir(path))
		}
	}
	// Language caches belong to uv/npm and are not recursively deleted here.
	paths, _ := filepath.Glob(filepath.Join(m.root, "cache", "downloads", "*"))
	for _, path := range paths {
		name := filepath.Base(path)
		if strings.HasPrefix(name, ".download-") {
			if err := m.collectPartialDownload(ctx, path); err != nil {
				return err
			}
			continue
		}
		if len(name) != 64 || !safeID.MatchString(name) {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || m.now().Sub(info.ModTime()) < 30*24*time.Hour {
			continue
		}
		if err := m.locked(ctx, "asset-"+name, func() error {
			if ownedPath(m.root, path) != nil {
				return nil
			}
			info, err := os.Stat(path)
			if err != nil || m.now().Sub(info.ModTime()) < 30*24*time.Hour {
				return nil
			}
			return os.Remove(path)
		}); err != nil {
			return err
		}
	}
	return m.pruneCoreRecords(ctx)
}

// New partial files identify their asset and Core. Collection takes the same
// asset lock as the downloader; active transfers are never removed.
func (m *Manager) collectPartialDownload(ctx context.Context, path string) error {
	if err := ownedPath(m.root, path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return err
	}
	name := strings.TrimPrefix(filepath.Base(path), ".download-")
	parts := strings.Split(name, "-")
	if len(parts) == 3 && len(parts[0]) == 64 && len(parts[1]) == 32 && safeID.MatchString(parts[0]) && safeID.MatchString(parts[1]) {
		if _, err := strconv.ParseUint(parts[2], 10, 64); err != nil {
			return nil
		}
		if m.coreAlive(parts[1]) {
			return nil
		}
		lockCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		err := m.locked(lockCtx, "asset-"+parts[0], func() error {
			if m.coreAlive(parts[1]) {
				return nil
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		})
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil
		}
		return err
	}
	// Compatibility for old random-only names. No old download can remain
	// active here without a live preparation record; retain recent/unknown files.
	if _, err := strconv.ParseUint(name, 10, 64); err != nil || m.now().Sub(info.ModTime()) < 30*24*time.Hour {
		return nil
	}
	operations, _ := filepath.Glob(filepath.Join(m.root, "operations", "*.json"))
	for _, file := range operations {
		var op Operation
		if readJSON(file, &op) != nil {
			return nil
		}
		if !operationTerminal(op.State) && m.coreAlive(op.CoreID) {
			return nil
		}
	}
	return os.Remove(path)
}

func operationTerminal(state string) bool {
	return state == "ready" || state == "failed" || state == "interrupted" || state == "cancelled"
}

// Remove Core receipts only after all their leases/other ownership records
// have gone. Missing receipts deliberately keep unknown leases protected.
func (m *Manager) pruneCoreRecords(ctx context.Context) error {
	return m.registry(func(_ *registry) error {
		referenced := map[string]bool{}
		for _, pattern := range []string{"leases/*/*.json", "component-uses/*/*.json", "waiters/*/*.json", "components/*/*/preparation.json", "environments/*/*/preparation.json", "office/instances/*/*/*/preparation.json"} {
			paths, _ := filepath.Glob(filepath.Join(m.root, filepath.FromSlash(pattern)))
			for _, path := range paths {
				var record map[string]any
				if readJSON(path, &record) != nil {
					return nil
				}
				id, _ := record["core_id"].(string)
				if id == "" {
					id, _ = record["CoreID"].(string)
				}
				if id != "" {
					referenced[id] = true
				}
			}
		}
		paths, _ := filepath.Glob(filepath.Join(m.root, "cores", "*.json"))
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return err
			}
			var core coreRecord
			if readJSON(path, &core) != nil || core.ID == m.coreID || referenced[core.ID] || filepath.Base(path) != core.ID+".json" || !m.leaseInactive(lease{CoreID: core.ID}) {
				continue
			}
			if err := ownedPath(m.root, path); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

func (m *Manager) pruneInactiveLeases(ctx context.Context) error {
	return m.registry(func(_ *registry) error {
		paths, err := filepath.Glob(filepath.Join(m.root, "leases", "*", "*.json"))
		if err != nil {
			return err
		}
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := ownedPath(m.root, path); err != nil {
				return err
			}
			name := strings.TrimSuffix(filepath.Base(path), ".json")
			known := len(name) == 64 && safeID.MatchString(name)
			for _, prefix := range []string{"process-", "host-"} {
				id, ok := strings.CutPrefix(name, prefix)
				known = known || (ok && len(id) == 32 && safeID.MatchString(id))
			}
			if !known {
				continue
			}
			var use lease
			if readJSON(path, &use) != nil || !m.leaseInactive(use) {
				continue
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

func (m *Manager) maintainLoop() {
	defer m.wg.Done()
	timer := time.NewTicker(time.Hour)
	defer timer.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-timer.C:
			_ = m.Maintain(m.ctx)
		}
	}
}
