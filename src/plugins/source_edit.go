package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/filestore"
	"awaken/src/plugins/ir"
)

// UpdateMCPSource edits one MCP declaration file through an isolated package
// snapshot. The live package and recorded digest move together only after the
// candidate package passes the same parser and containment checks as install.
func (s *Store) UpdateMCPSource(name, logicalID, expectedRecordedDigest, source string) (UpdateComponentSourceResult, error) {
	if s == nil {
		return UpdateComponentSourceResult{}, ErrHomeRequired
	}
	name = strings.TrimSpace(name)
	logicalID = strings.TrimSpace(logicalID)
	expectedRecordedDigest = strings.TrimSpace(expectedRecordedDigest)
	if expectedRecordedDigest == "" {
		return UpdateComponentSourceResult{}, fmt.Errorf("%w: expected recorded digest is required", ErrRecordedDigestChanged)
	}
	if len(source) > MaxComponentSourceBytes {
		return UpdateComponentSourceResult{}, fmt.Errorf("%w: MCP source exceeds %d bytes", ErrMCPInvalid, MaxComponentSourceBytes)
	}

	var result UpdateComponentSourceResult
	changed := false
	err := s.withMutationLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if record.Builtin {
			return fmt.Errorf("%w: %s", ErrBuiltinImmutable, name)
		}
		if record.Digest != expectedRecordedDigest {
			return recordedDigestChangedError(expectedRecordedDigest, record.Digest)
		}

		plugin := s.loadRecord(name, record)
		if !plugin.IntegrityValid {
			return fmt.Errorf("%w: installed package must be intact before editing", ErrDigestChanged)
		}
		var targetComponent *ir.Component
		for index := range plugin.Components {
			if plugin.Components[index].LogicalID == logicalID {
				targetComponent = &plugin.Components[index]
				break
			}
		}
		if targetComponent == nil {
			return fmt.Errorf("%w: component %s", ErrNotFound, logicalID)
		}
		if targetComponent.Kind != ir.KindMCPServer {
			return fmt.Errorf("%w: only MCP source files are editable", ErrComponentNotSwitchable)
		}
		if targetComponent.MCP == nil || targetComponent.MCP.HostConnectorID != "" {
			return fmt.Errorf("%w: host connector MCP source is managed by Awaken", ErrComponentNotSwitchable)
		}

		relativeSourcePath := componentSourceRelativePath(plugin, *targetComponent)
		if relativeSourcePath == "" || relativeSourcePath == "." {
			return fmt.Errorf("%w: MCP component has no editable source file", ErrMCPInvalid)
		}
		cleanRelativePath := filepath.Clean(filepath.FromSlash(relativeSourcePath))
		liveRoot := filepath.Join(s.packagesRoot, name)
		liveSourcePath := filepath.Join(liveRoot, cleanRelativePath)
		if err := resolvedStayWithin(liveRoot, liveSourcePath); err != nil {
			return fmt.Errorf("%w: MCP source escapes the installed package", ErrPathEscapes)
		}
		info, err := os.Lstat(liveSourcePath)
		if err != nil {
			return fmt.Errorf("%w: inspect MCP source: %v", ErrMCPInvalid, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: MCP source must be a regular file", ErrMCPInvalid)
		}
		currentSource, err := os.ReadFile(liveSourcePath)
		if err != nil {
			return fmt.Errorf("%w: read MCP source: %v", ErrMCPInvalid, err)
		}
		if string(currentSource) == source {
			result = UpdateComponentSourceResult{
				Plugin: plugin, PreviousDigest: record.Digest, Digest: record.Digest, Changed: false,
			}
			return nil
		}

		transactionRoot, err := os.MkdirTemp(s.stagingRoot, "source-edit-*")
		if err != nil {
			return fmt.Errorf("create MCP source edit staging directory: %w", err)
		}
		defer os.RemoveAll(transactionRoot)
		candidateRoot := filepath.Join(transactionRoot, "candidate")
		if err := filestore.EnsureDir(candidateRoot); err != nil {
			return err
		}
		if err := copyTree(liveRoot, candidateRoot); err != nil {
			return fmt.Errorf("snapshot installed plugin for MCP source edit: %w", err)
		}
		candidateSourcePath := filepath.Join(candidateRoot, cleanRelativePath)
		if err := resolvedStayWithin(candidateRoot, candidateSourcePath); err != nil {
			return fmt.Errorf("%w: staged MCP source escapes the package", ErrPathEscapes)
		}
		if err := os.WriteFile(candidateSourcePath, []byte(source), safePluginFileMode(info.Mode())); err != nil {
			return fmt.Errorf("write staged MCP source: %w", err)
		}

		dataDir := filepath.Join(s.dataRoot, name)
		validation, err := s.validatePackageTree(candidateRoot, dataDir, record)
		if err != nil {
			return err
		}
		if record.ManifestPath == "" && record.Dialect == ir.DialectClaudeCode {
			validation.Name = name
		}
		if validation.Name != name {
			return fmt.Errorf("%w: plugin identity changed from %q to %q", ErrManifestInvalid, name, validation.Name)
		}
		if validation.Dialect != record.Dialect || validation.ManifestPath != record.ManifestPath {
			return fmt.Errorf("%w: MCP source edit cannot change plugin dialect or manifest ownership", ErrManifestInvalid)
		}
		if !validation.Valid {
			return fmt.Errorf("%w: edited package failed validation", ErrManifestInvalid)
		}
		matchingMCPComponents := 0
		for _, component := range validation.Components {
			if component.Kind != ir.KindMCPServer || componentSourceFilePath(component.SourcePath) != relativeSourcePath {
				continue
			}
			matchingMCPComponents++
			if component.Validation == "invalid" || component.Mapping == ir.MappingInvalid {
				return fmt.Errorf("%w: edited MCP source contains an invalid server %q", ErrMCPInvalid, component.DisplayName)
			}
		}
		if matchingMCPComponents == 0 {
			return fmt.Errorf("%w: edited source no longer declares an MCP server", ErrMCPInvalid)
		}

		if _, matches, verifyErr := verifyTreeDigest(liveRoot, record.Digest); verifyErr != nil || !matches {
			if verifyErr != nil {
				return fmt.Errorf("%w: verify installed package before activation: %v", ErrSourceChanged, verifyErr)
			}
			return fmt.Errorf("%w: installed package changed while MCP source was being validated", ErrSourceChanged)
		}
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

		previousRoot := filepath.Join(transactionRoot, "previous")
		if err := os.Rename(liveRoot, previousRoot); err != nil {
			return fmt.Errorf("stage previous plugin package: %w", err)
		}
		if err := os.Rename(candidateRoot, liveRoot); err != nil {
			_ = os.Rename(previousRoot, liveRoot)
			return fmt.Errorf("activate edited plugin package: %w", err)
		}

		updated := current
		updated.Digest = validation.Digest
		updated.Version = validation.Version
		updated.DisplayName = firstNonEmptyPlugin(validation.Normalized.Identity.DisplayName, current.DisplayName)
		updated.Components, updated.Variables, updated.MigrationConflicts = migrateOverrides(current, plugin.Components, validation.Components)
		currentState.Plugins[name] = updated
		rollback := func() error {
			if err := restorePluginPackage(liveRoot, previousRoot); err != nil {
				return err
			}
			currentState.Plugins[name] = current
			return s.writeStateUnlocked(currentState)
		}
		if err := s.writeStateUnlocked(currentState); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return fmt.Errorf("write plugin state: %v; rollback MCP source edit: %w", err, rollbackErr)
			}
			return err
		}
		updatedPlugin := s.loadRecord(name, updated)
		if !updatedPlugin.IntegrityValid {
			if rollbackErr := rollback(); rollbackErr != nil {
				return fmt.Errorf("%w: edited plugin package failed post-activation verification; rollback: %v", ErrSourceChanged, rollbackErr)
			}
			return fmt.Errorf("%w: edited plugin package failed post-activation integrity verification", ErrSourceChanged)
		}
		moved, trashErr := s.trash.TrashPath(previousRoot)
		if trashErr != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return fmt.Errorf("%w: %v; rollback MCP source edit: %v", ErrReplaceTrashUnavailable, trashErr, rollbackErr)
			}
			return fmt.Errorf("%w: %v", ErrReplaceTrashUnavailable, trashErr)
		}
		_ = moved
		result = UpdateComponentSourceResult{
			Plugin: updatedPlugin, PreviousDigest: current.Digest, Digest: updated.Digest, Changed: true,
		}
		changed = true
		return nil
	})
	if err != nil {
		return UpdateComponentSourceResult{}, err
	}
	if changed {
		s.notify()
	}
	return result, nil
}

func componentSourceFilePath(sourcePath string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.Split(sourcePath, "#")[0])))
}
