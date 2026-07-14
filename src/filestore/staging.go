package filestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func marshalIndent(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("filestore: encode JSON: %w", err)
	}
	return append(encoded, '\n'), nil
}

// Staging implements the transaction-first two-phase directory creation
// protocol: build content in a private .staging directory, fsync it, record
// the commit in a lifecycle journal, then atomically rename into the public
// location and fsync the parent. Startup recovery rolls uncommitted staging
// back, rolls committed-but-unrenamed staging forward, and quarantines public
// directories without a commit.
type Staging struct {
	storeRoot string
	journal   *Journal
}

// NewStaging binds staging operations to a store root and lifecycle journal.
func NewStaging(storeRoot string, journal *Journal) *Staging {
	return &Staging{storeRoot: storeRoot, journal: journal}
}

type stagingCommit struct {
	Kind       string `json:"kind"`
	StagingDir string `json:"staging_dir"`
	TargetDir  string `json:"target_dir"`
}

const stagingCommitKind = "StagingCommitted"

func (s *Staging) stagingRoot() string {
	return filepath.Join(s.storeRoot, ".staging")
}

// Begin creates a private staging directory for target (a path relative to
// the store root).
func (s *Staging) Begin(target string) (string, error) {
	if err := EnsureDir(s.stagingRoot()); err != nil {
		return "", err
	}
	name := strings.ReplaceAll(filepath.ToSlash(target), "/", "__")
	stagingDir, err := os.MkdirTemp(s.stagingRoot(), name+"-txn-*")
	if err != nil {
		return "", fmt.Errorf("filestore: create staging dir: %w", err)
	}
	if err := os.Chmod(stagingDir, DirPerm); err != nil {
		os.RemoveAll(stagingDir)
		return "", err
	}
	return stagingDir, nil
}

// Commit fsyncs the staging directory, appends the commit record to the
// lifecycle journal, renames the staging directory to its public target and
// fsyncs the target's parent. A crash after the journal append is rolled
// forward by Recover.
func (s *Staging) Commit(stagingDir, target string) error {
	targetPath := filepath.Join(s.storeRoot, target)
	if err := WithinRoot(s.storeRoot, filepath.Dir(targetPath)); err != nil {
		return err
	}
	if err := SyncDir(stagingDir); err != nil {
		return err
	}
	if _, err := s.journal.Append(stagingCommit{
		Kind:       stagingCommitKind,
		StagingDir: stagingDir,
		TargetDir:  targetPath,
	}); err != nil {
		return err
	}
	if err := EnsureDir(filepath.Dir(targetPath)); err != nil {
		return err
	}
	if err := os.Rename(stagingDir, targetPath); err != nil {
		return fmt.Errorf("filestore: materialize staged dir: %w", err)
	}
	return SyncDir(filepath.Dir(targetPath))
}

// Recover replays the lifecycle journal: committed staging directories that
// were never renamed are rolled forward; staging directories without a commit
// are rolled back (removed). It returns the targets that were rolled forward.
func (s *Staging) Recover() ([]string, error) {
	records, err := s.journal.Records()
	if err != nil {
		return nil, err
	}
	committed := map[string]string{}
	for _, record := range records {
		var commit stagingCommit
		if err := json.Unmarshal(record.Payload, &commit); err != nil {
			continue
		}
		if commit.Kind == stagingCommitKind {
			committed[commit.StagingDir] = commit.TargetDir
		}
	}
	var rolledForward []string
	entries, err := os.ReadDir(s.stagingRoot())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		stagingDir := filepath.Join(s.stagingRoot(), entry.Name())
		target, wasCommitted := committed[stagingDir]
		if !wasCommitted {
			// No durable commit: roll back.
			if err := os.RemoveAll(stagingDir); err != nil {
				return rolledForward, fmt.Errorf("filestore: roll back staging %s: %w", stagingDir, err)
			}
			continue
		}
		if _, statErr := os.Stat(target); statErr == nil {
			// Target already materialized; leftover staging dir means the
			// rename happened for an identically named retry. Remove leftover.
			if err := os.RemoveAll(stagingDir); err != nil {
				return rolledForward, err
			}
			continue
		}
		if err := EnsureDir(filepath.Dir(target)); err != nil {
			return rolledForward, err
		}
		if err := os.Rename(stagingDir, target); err != nil {
			return rolledForward, fmt.Errorf("filestore: roll forward staging %s: %w", stagingDir, err)
		}
		if err := SyncDir(filepath.Dir(target)); err != nil {
			return rolledForward, err
		}
		rolledForward = append(rolledForward, target)
	}
	return rolledForward, nil
}
