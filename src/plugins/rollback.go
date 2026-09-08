package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"awaken/src/filestore"
)

const rollbackRecordName = "rollback.json"

// Paths are single directory names relative to the transaction root. The
// previous record contains secret references only, never credential values.
type pluginRollback struct {
	Name          string `json:"name"`
	Previous      Record `json:"previous"`
	PackageBackup string `json:"package_backup"`
	DataBackup    string `json:"data_backup,omitempty"`
	NextDigest    string `json:"next_digest,omitempty"`
	Uninstall     bool   `json:"uninstall,omitempty"`
}

func writeRollback(root string, record pluginRollback) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return filestore.WriteFileAtomic(filepath.Join(root, rollbackRecordName), data)
}

func finishPluginRollback(root string, operationErr *error) {
	if *operationErr != nil {
		if _, err := os.Lstat(filepath.Join(root, rollbackRecordName)); err == nil {
			*operationErr = fmt.Errorf("%w; plugin recovery retained at %s; retry after resolving the filesystem error", *operationErr, root)
			return
		}
	}
	_ = os.RemoveAll(root)
}

// RecoverPendingOperations retries interrupted rollbacks under the same store
// lock and runtime-lease boundary as install/uninstall. Startup and the next
// mutation use this path too; a failed recovery never deletes its backup.
func (s *Store) RecoverPendingOperations() error {
	return s.withMutationLock(func() error { return nil })
}

func (s *Store) recoverPendingOperationsLocked() error {
	entries, err := os.ReadDir(s.stagingRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		root := filepath.Join(s.stagingRoot, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, rollbackRecordName))
		if errors.Is(err, os.ErrNotExist) {
			// Old releases did not journal rollback directories. Keep those
			// backups for manual recovery rather than treating them as garbage.
			continue
		}
		if err != nil {
			return fmt.Errorf("read plugin recovery at %s: %w", root, err)
		}
		var record pluginRollback
		if err := json.Unmarshal(data, &record); err != nil {
			return fmt.Errorf("invalid plugin recovery at %s: %w", root, err)
		}
		if err := s.recoverPluginRollback(root, record); err != nil {
			return fmt.Errorf("plugin recovery pending at %s: %w", root, err)
		}
		if err := os.RemoveAll(root); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) recoverPluginRollback(root string, record pluginRollback) error {
	if !isValidPluginName(record.Name) || record.PackageBackup == "" || filepath.Base(record.PackageBackup) != record.PackageBackup || record.PackageBackup == "." || record.PackageBackup == ".." || (record.DataBackup != "" && record.DataBackup != "data") {
		return fmt.Errorf("invalid rollback identity or paths")
	}
	state, err := s.readStateUnlocked()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	live := filepath.Join(s.packagesRoot, record.Name)
	backup := filepath.Join(root, record.PackageBackup)
	if err := s.rejectUncontrolledPath(live); err != nil {
		return err
	}
	if err := s.rejectUncontrolledPath(backup); err != nil {
		return err
	}
	backupInfo, backupErr := os.Lstat(backup)
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		return backupErr
	}
	if backupErr == nil && !backupInfo.IsDir() {
		return fmt.Errorf("backup is not a directory")
	}
	if backupErr == nil {
		// Never replace an externally modified live package during recovery.
		if _, err := os.Lstat(live); err == nil {
			digest, err := digestTree(live)
			if err != nil {
				return err
			}
			if digest != record.Previous.Digest && digest != record.NextDigest {
				return fmt.Errorf("live package changed; manual reconciliation required")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := restorePluginPackage(live, backup); err != nil {
			return err
		}
	} else {
		// Trash may have completed just before the process stopped. In that
		// case the committed store and package prove there is no rollback left.
		current, exists := state.Plugins[record.Name]
		if record.Uninstall && !exists {
			if _, err := os.Lstat(live); errors.Is(err, os.ErrNotExist) {
				return nil
			}
		}
		if !record.Uninstall && exists && current.Digest == record.NextDigest {
			if _, matches, err := verifyTreeDigest(live, current.Digest); err == nil && matches {
				return nil
			}
		}
		if _, matches, err := verifyTreeDigest(live, record.Previous.Digest); err != nil || !matches {
			return fmt.Errorf("previous package cannot be verified; preserve recovery directory")
		}
	}
	if record.DataBackup != "" {
		backupData := filepath.Join(root, record.DataBackup)
		if _, err := os.Lstat(backupData); err == nil {
			liveData := filepath.Join(s.dataRoot, record.Name)
			if err := s.rejectUncontrolledPath(backupData); err != nil {
				return err
			}
			if _, err := os.Lstat(liveData); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("live plugin data exists or is inaccessible; preserve both copies")
			}
			if err := os.Rename(backupData, liveData); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	state.Plugins[record.Name] = record.Previous
	return s.writeStateUnlocked(state)
}
