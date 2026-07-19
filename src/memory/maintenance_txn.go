package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/filestore"
	"github.com/google/uuid"
)

const maintenanceMarkerFilename = ".maintenance-transaction.json"

var ErrMaintenanceInProgress = errors.New("memory: maintenance transaction in progress")

type maintenanceTransaction struct {
	TransactionID string                       `json:"transaction_id"`
	State         string                       `json:"state"`
	Entries       []maintenanceTransactionFile `json:"entries"`
	CreatedAt     time.Time                    `json:"created_at"`
}

type maintenanceTransactionFile struct {
	Target       string `json:"target"`
	BeforeExists bool   `json:"before_exists"`
	StageName    string `json:"stage_name"`
}

type maintenanceMarker struct {
	TransactionID string `json:"transaction_id"`
}

func (s *Store) commitMaintenanceTransaction(root string, files map[string][]byte) error {
	txn := maintenanceTransaction{
		TransactionID: "txn_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		State:         "prepared",
		CreatedAt:     s.Now().UTC(),
	}
	txnRoot := filepath.Join(root, ".maintenance-transactions", txn.TransactionID)
	beforeRoot := filepath.Join(txnRoot, "before")
	afterRoot := filepath.Join(txnRoot, "after")
	if err := filestore.EnsureDir(beforeRoot); err != nil {
		return err
	}
	if err := filestore.EnsureDir(afterRoot); err != nil {
		return err
	}
	targets := make([]string, 0, len(files))
	for target := range files {
		targets = append(targets, target)
	}
	sortStrings(targets)
	for index, target := range targets {
		relative, err := filepath.Rel(root, target)
		if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return ErrMaintenanceInProgress
		}
		stageName := fmt.Sprintf("%03d.bin", index)
		before, readErr := os.ReadFile(target)
		beforeExists := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}
		if beforeExists {
			if err := filestore.WriteFileAtomic(filepath.Join(beforeRoot, stageName), before); err != nil {
				return err
			}
		}
		if err := filestore.WriteFileAtomic(filepath.Join(afterRoot, stageName), files[target]); err != nil {
			return err
		}
		txn.Entries = append(txn.Entries, maintenanceTransactionFile{Target: filepath.ToSlash(relative), BeforeExists: beforeExists, StageName: stageName})
	}
	manifestPath := filepath.Join(txnRoot, "manifest.json")
	if err := writeMaintenanceManifest(manifestPath, txn); err != nil {
		return err
	}
	markerData, _ := json.Marshal(maintenanceMarker{TransactionID: txn.TransactionID})
	markerPath := filepath.Join(root, maintenanceMarkerFilename)
	if err := filestore.WriteFileAtomic(markerPath, append(markerData, '\n')); err != nil {
		return err
	}
	txn.State = "committing"
	if err := writeMaintenanceManifest(manifestPath, txn); err != nil {
		return s.rollbackMaintenanceTransaction(root, txnRoot, txn, err)
	}
	for _, entry := range txn.Entries {
		data, err := os.ReadFile(filepath.Join(afterRoot, entry.StageName))
		if err != nil {
			return s.rollbackMaintenanceTransaction(root, txnRoot, txn, err)
		}
		if err := atomicWrite(filepath.Join(root, filepath.FromSlash(entry.Target)), data); err != nil {
			return s.rollbackMaintenanceTransaction(root, txnRoot, txn, err)
		}
	}
	txn.State = "committed"
	if err := writeMaintenanceManifest(manifestPath, txn); err != nil {
		return s.rollbackMaintenanceTransaction(root, txnRoot, txn, err)
	}
	return cleanupMaintenanceTransaction(root, txnRoot)
}

func (s *Store) ensureMaintenanceReady(root string) error {
	markerPath := filepath.Join(root, maintenanceMarkerFilename)
	var marker maintenanceMarker
	if err := filestore.ReadJSON(markerPath, &marker); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("memory: unreadable maintenance marker: %w", err)
	}
	if !strings.HasPrefix(marker.TransactionID, "txn_") || strings.ContainsAny(marker.TransactionID, "/\\") {
		return fmt.Errorf("memory: invalid maintenance transaction identity")
	}
	lock, err := filestore.AcquireLock(filepath.Join(root, ".maintenance-lock"), "recovery:"+marker.TransactionID, 30*time.Second)
	if errors.Is(err, filestore.ErrLockHeld) {
		return ErrMaintenanceInProgress
	}
	if err != nil {
		return err
	}
	defer lock.Release()
	txnRoot := filepath.Join(root, ".maintenance-transactions", marker.TransactionID)
	var txn maintenanceTransaction
	if err := filestore.ReadJSON(filepath.Join(txnRoot, "manifest.json"), &txn); err != nil {
		return fmt.Errorf("memory: maintenance recovery manifest unavailable: %w", err)
	}
	if txn.TransactionID != marker.TransactionID {
		return fmt.Errorf("memory: maintenance recovery identity mismatch")
	}
	if err := validateMaintenanceTransaction(root, txnRoot, txn); err != nil {
		return err
	}
	if txn.State == "committed" {
		for _, entry := range txn.Entries {
			data, err := os.ReadFile(filepath.Join(txnRoot, "after", entry.StageName))
			if err != nil {
				return err
			}
			if err := atomicWrite(filepath.Join(root, filepath.FromSlash(entry.Target)), data); err != nil {
				return err
			}
		}
		return cleanupMaintenanceTransaction(root, txnRoot)
	}
	return s.rollbackMaintenanceTransaction(root, txnRoot, txn, nil)
}

func validateMaintenanceTransaction(root, txnRoot string, txn maintenanceTransaction) error {
	if txn.State != "prepared" && txn.State != "committing" && txn.State != "committed" {
		return fmt.Errorf("memory: invalid maintenance transaction state %q", txn.State)
	}
	if err := filestore.WithinRoot(root, txnRoot); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, entry := range txn.Entries {
		targetRelative := filepath.Clean(filepath.FromSlash(entry.Target))
		if targetRelative == "." || filepath.IsAbs(targetRelative) || targetRelative == ".." || strings.HasPrefix(targetRelative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("memory: maintenance target escapes scope")
		}
		if filepath.Base(entry.StageName) != entry.StageName || filepath.Ext(entry.StageName) != ".bin" {
			return fmt.Errorf("memory: invalid maintenance stage identity")
		}
		if seen[targetRelative] {
			return fmt.Errorf("memory: duplicate maintenance target %q", entry.Target)
		}
		seen[targetRelative] = true
		for _, path := range []string{
			filepath.Join(root, targetRelative),
			filepath.Join(txnRoot, "after", entry.StageName),
			filepath.Join(txnRoot, "before", entry.StageName),
		} {
			if err := filestore.WithinRoot(root, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) rollbackMaintenanceTransaction(root, txnRoot string, txn maintenanceTransaction, cause error) error {
	var rollbackErr error
	for _, entry := range txn.Entries {
		target := filepath.Join(root, filepath.FromSlash(entry.Target))
		if entry.BeforeExists {
			data, err := os.ReadFile(filepath.Join(txnRoot, "before", entry.StageName))
			if err == nil {
				err = atomicWrite(target, data)
			}
			rollbackErr = errors.Join(rollbackErr, err)
		} else if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if rollbackErr == nil {
		rollbackErr = cleanupMaintenanceTransaction(root, txnRoot)
	}
	if cause != nil && rollbackErr != nil {
		return fmt.Errorf("%w (maintenance rollback failed: %v)", cause, rollbackErr)
	}
	if cause != nil {
		return cause
	}
	return rollbackErr
}

func cleanupMaintenanceTransaction(root, txnRoot string) error {
	if err := os.Remove(filepath.Join(root, maintenanceMarkerFilename)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := syncDirectory(root); err != nil {
		return err
	}
	return os.RemoveAll(txnRoot)
}

func writeMaintenanceManifest(path string, txn maintenanceTransaction) error {
	data, err := json.MarshalIndent(txn, "", "  ")
	if err != nil {
		return err
	}
	return filestore.WriteFileAtomic(path, append(data, '\n'))
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
