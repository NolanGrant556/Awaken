package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CommitInstalledChanges validates and signs the current installed package
// tree after an authorized in-place edit. The expected digest is a required
// compare-and-swap token for the currently recorded plugin state.
func (s *Store) CommitInstalledChanges(name, expectedRecordedDigest string) (CommitInstalledChangesResult, error) {
	return s.commitInstalledChanges(name, expectedRecordedDigest, nil)
}

// commitInstalledChanges accepts an optional hook immediately before the
// final live-tree digest. It keeps concurrent-mutation coverage deterministic
// without widening the production Store surface.
func (s *Store) commitInstalledChanges(name, expectedRecordedDigest string, beforeFinalDigest func()) (CommitInstalledChangesResult, error) {
	if s == nil {
		return CommitInstalledChangesResult{}, ErrHomeRequired
	}
	name = strings.TrimSpace(name)
	expectedRecordedDigest = strings.TrimSpace(expectedRecordedDigest)
	if expectedRecordedDigest == "" {
		return CommitInstalledChangesResult{}, fmt.Errorf("%w: expected recorded digest is required", ErrRecordedDigestChanged)
	}

	var result CommitInstalledChangesResult
	err := s.withMutationLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if rec.Builtin {
			return fmt.Errorf("%w: %s", ErrBuiltinImmutable, name)
		}
		if rec.Digest != expectedRecordedDigest {
			return recordedDigestChangedError(expectedRecordedDigest, rec.Digest)
		}

		root := filepath.Join(s.packagesRoot, name)
		if err := s.rejectUncontrolledPath(root); err != nil {
			return err
		}
		info, err := os.Lstat(root)
		if err != nil {
			return fmt.Errorf("inspect installed plugin package: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: installed plugin package must be a regular directory", ErrManifestInvalid)
		}
		if err := resolvedStayWithin(s.packagesRoot, root); err != nil {
			return fmt.Errorf("%w: installed plugin package escapes the managed packages root: %v", ErrPathEscapes, err)
		}

		dataDir := filepath.Join(s.dataRoot, name)
		if err := validateManagedPluginDataDir(s.dataRoot, dataDir); err != nil {
			return err
		}

		liveDigestBefore, err := digestTree(root)
		if err != nil {
			return fmt.Errorf("%w: compute installed plugin digest before validation: %v", ErrManifestInvalid, err)
		}
		snapshotRoot, err := os.MkdirTemp(s.stagingRoot, "commit-*")
		if err != nil {
			return fmt.Errorf("create plugin commit snapshot: %w", err)
		}
		defer os.RemoveAll(snapshotRoot)
		if err := copyTree(root, snapshotRoot); err != nil {
			return fmt.Errorf("%w: snapshot installed plugin package: %v", ErrManifestInvalid, err)
		}

		validation, err := s.validatePackageTree(snapshotRoot, dataDir, rec)
		if err != nil {
			return err
		}
		// Manifestless Claude packages derive identity from their materialized
		// directory. The commit snapshot has a temporary directory name, while
		// the durable installation identity remains the Store key.
		if rec.ManifestPath == "" && rec.Dialect == "claude_code" {
			validation.Name = name
		}
		if validation.Name != name {
			return fmt.Errorf("%w: plugin.json name changed from %q to %q", ErrManifestInvalid, name, validation.Name)
		}
		if !validation.Valid {
			return fmt.Errorf("%w: installed plugin package failed validation", ErrManifestInvalid)
		}
		if validation.Digest != liveDigestBefore {
			return fmt.Errorf("%w: installed plugin changed while its validation snapshot was created", ErrSourceChanged)
		}

		if beforeFinalDigest != nil {
			beforeFinalDigest()
		}
		liveDigestAfter, err := digestTree(root)
		if err != nil {
			return fmt.Errorf("%w: compute installed plugin digest after validation: %v", ErrSourceChanged, err)
		}
		if liveDigestAfter != validation.Digest {
			return fmt.Errorf("%w: installed plugin changed during commit validation: validated %s, actual %s", ErrSourceChanged, validation.Digest, liveDigestAfter)
		}

		// Re-read immediately before writing so the expected recorded digest is
		// a real CAS even if another process ignored the advisory Store lock.
		currentState, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		current, ok := currentState.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if current.Digest != expectedRecordedDigest {
			return recordedDigestChangedError(expectedRecordedDigest, current.Digest)
		}

		result = CommitInstalledChangesResult{
			PreviousDigest: current.Digest,
			Digest:         liveDigestAfter,
			Changed:        current.Digest != liveDigestAfter || current.Version != validation.Version,
		}
		if !result.Changed {
			plugin := s.loadRecord(name, current)
			if !plugin.IntegrityValid {
				return fmt.Errorf("%w: installed plugin changed before commit completed", ErrSourceChanged)
			}
			result.Plugin = plugin
			result.Message = "installed plugin changes already committed"
			return nil
		}

		updated := current
		updated.Digest = liveDigestAfter
		updated.Version = validation.Version
		currentState.Plugins[name] = updated
		if err := s.writeStateUnlocked(currentState); err != nil {
			return err
		}

		plugin := s.loadRecord(name, updated)
		if !plugin.IntegrityValid {
			currentState.Plugins[name] = current
			if rollbackErr := s.writeStateUnlocked(currentState); rollbackErr != nil {
				return fmt.Errorf("%w: installed plugin changed before commit completed; restore recorded digest: %v", ErrSourceChanged, rollbackErr)
			}
			return fmt.Errorf("%w: installed plugin changed before commit completed", ErrSourceChanged)
		}
		result.Plugin = plugin
		result.Message = "installed plugin changes committed"
		return nil
	})
	if err == nil && result.Changed {
		s.notify()
	}
	return result, err
}

func recordedDigestChangedError(expected, actual string) error {
	return fmt.Errorf("%w: expected %s, recorded %s", ErrRecordedDigestChanged, expected, actual)
}
