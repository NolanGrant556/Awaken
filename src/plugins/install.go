package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/filestore"
	"awaken/src/plugins/dialect"
	"awaken/src/plugins/ir"
	"github.com/google/uuid"
)

// Install copies a plugin package into the store. Same-name installs abort
// unless Replace is set. Installation never executes package code.
func (s *Store) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if err := validateSource(req.Source); err != nil {
		return InstallResult{}, err
	}
	var result InstallResult
	err := s.withMutationLock(func() error {
		installed, err := s.installLocked(ctx, req)
		if err != nil {
			return err
		}
		result = installed
		return nil
	})
	if err == nil {
		s.notify()
	}
	return result, err
}

// Uninstall moves the package and data directories to Trash, then drops the record.
func (s *Store) Uninstall(name string) (string, error) {
	var trashed string
	err := s.withMutationLock(func() (operationErr error) {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		record, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		if record.Builtin {
			return fmt.Errorf("%w: %s", ErrBuiltinImmutable, name)
		}
		pkg := filepath.Join(s.packagesRoot, name)
		data := filepath.Join(s.dataRoot, name)
		if err := s.rejectUncontrolledPath(pkg); err != nil {
			return err
		}
		if _, statErr := os.Lstat(data); statErr == nil {
			if err := s.rejectUncontrolledPath(data); err != nil {
				return err
			}
		}
		rollbackRoot, err := os.MkdirTemp(s.stagingRoot, "uninstall-*")
		if err != nil {
			return fmt.Errorf("create uninstall rollback directory: %w", err)
		}
		if err := writeRollback(rollbackRoot, pluginRollback{Name: name, Previous: record, PackageBackup: "package", DataBackup: "data", Uninstall: true}); err != nil {
			os.RemoveAll(rollbackRoot)
			return err
		}
		defer finishPluginRollback(rollbackRoot, &operationErr)
		packageBackup := filepath.Join(rollbackRoot, "package")
		dataBackup := filepath.Join(rollbackRoot, "data")
		packageStaged, dataStaged := false, false
		restore := func() error {
			var restoreErrors []string
			if packageStaged {
				if restoreErr := restorePluginPackage(pkg, packageBackup); restoreErr != nil {
					restoreErrors = append(restoreErrors, "package: "+restoreErr.Error())
				}
			}
			if dataStaged {
				if restoreErr := restorePluginPackage(data, dataBackup); restoreErr != nil {
					restoreErrors = append(restoreErrors, "data: "+restoreErr.Error())
				}
			}
			if len(restoreErrors) > 0 {
				return fmt.Errorf("restore uninstalled plugin: %s", strings.Join(restoreErrors, "; "))
			}
			return nil
		}
		if info, statErr := os.Lstat(pkg); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: package is a symlink", ErrReplaceTrashUnavailable)
			}
			if err := os.Rename(pkg, packageBackup); err != nil {
				return fmt.Errorf("stage plugin package for uninstall: %w", err)
			}
			packageStaged = true
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		if info, statErr := os.Lstat(data); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				_ = restore()
				return fmt.Errorf("%w: plugin data is not a regular directory", ErrReplaceTrashUnavailable)
			}
			if err := os.Rename(data, dataBackup); err != nil {
				restoreErr := restore()
				if restoreErr != nil {
					return fmt.Errorf("stage plugin data for uninstall: %v; %w", err, restoreErr)
				}
				return fmt.Errorf("stage plugin data for uninstall: %w", err)
			}
			dataStaged = true
		} else if !os.IsNotExist(statErr) {
			_ = restore()
			return statErr
		}
		delete(state.Plugins, name)
		if err := s.writeStateUnlocked(state); err != nil {
			if restoreErr := restore(); restoreErr != nil {
				return fmt.Errorf("write plugin state: %v; %w", err, restoreErr)
			}
			return err
		}
		moved, trashErr := s.trash.TrashPath(rollbackRoot)
		if trashErr != nil {
			restoreErr := restore()
			state.Plugins[name] = record
			stateErr := s.writeStateUnlocked(state)
			if restoreErr != nil || stateErr != nil {
				return fmt.Errorf("%w: %v; package rollback: %v; state rollback: %v", ErrReplaceTrashUnavailable, trashErr, restoreErr, stateErr)
			}
			return fmt.Errorf("%w: %v", ErrReplaceTrashUnavailable, trashErr)
		}
		trashed = moved.TrashedPath
		if s.secrets != nil {
			for _, value := range record.Variables {
				_ = s.secrets.Delete(value.SecretRef)
			}
		}
		return nil
	})
	if err == nil {
		s.notify()
	}
	return trashed, err
}

func (s *Store) installLocked(ctx context.Context, req InstallRequest) (result InstallResult, operationErr error) {
	state, err := s.readStateUnlocked()
	if err != nil && !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	staging, err := os.MkdirTemp(s.stagingRoot, "pkg-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := materializeSource(req.Source, staging); err != nil {
		return InstallResult{}, err
	}
	resolved, err := resolvePackage(staging, ResolveOptions{
		DialectHint: req.Source.DialectHint, SelectedDialect: req.Source.SelectedDialect,
		AllowManifestless: req.Source.DialectHint == "claude_code" || req.Source.SelectedDialect == "claude_code",
	})
	if err != nil {
		return InstallResult{}, err
	}
	if resolved.RequiresSelection {
		return InstallResult{}, fmt.Errorf("%w: multiple plugin manifests require an explicit dialect selection", ErrManifestInvalid)
	}
	pluginRoot := resolved.Root
	digest, err := digestTree(pluginRoot)
	if err != nil {
		return InstallResult{}, fmt.Errorf("compute plugin digest: %w", err)
	}
	normalized, err := dialect.NewDefaultRegistry().Parse(pluginRoot, resolved.Candidate, dialect.ParseOptions{PluginDigest: digest})
	if err != nil {
		return InstallResult{}, err
	}
	name := normalized.Identity.Name
	if resolved.Candidate.Manifestless {
		name = strings.TrimSpace(req.InstallName)
		if name == "" {
			return InstallResult{}, fmt.Errorf("%w: manifestless plugin requires installName", ErrManifestInvalid)
		}
	}
	if !isValidPluginName(name) {
		return InstallResult{}, fmt.Errorf("%w: invalid install name %q", ErrManifestInvalid, name)
	}
	manifest := Manifest{
		Name: name, Version: normalized.Identity.Version, Description: normalized.Identity.Description,
		Homepage: normalized.Identity.Homepage, Repository: normalized.Identity.Repository, License: normalized.Identity.License,
		Keywords: append([]string(nil), normalized.Identity.Keywords...),
	}
	if normalized.Layout.Dialect == ir.DialectAgentPlugins {
		manifest.Schema = SchemaPluginV1
	}
	if normalized.Identity.Author != nil {
		manifest.Author = &Author{Name: normalized.Identity.Author.Name, Email: normalized.Identity.Author.Email, URL: normalized.Identity.Author.URL}
	}
	if expected := strings.TrimSpace(req.ExpectedDigest); expected != "" && expected != digest {
		return InstallResult{}, fmt.Errorf("%w: validated %s, materialized %s", ErrSourceChanged, expected, digest)
	}

	existing, exists := state.Plugins[name]
	if exists && existing.Builtin && req.Builtin == nil {
		return InstallResult{}, fmt.Errorf("%w: %s", ErrBuiltinImmutable, name)
	}
	if exists && !req.Replace {
		return InstallResult{}, fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}
	var existingComponents []ir.Component
	if exists {
		if current := s.loadRecord(name, existing); current != nil && current.IntegrityValid {
			existingComponents = append([]ir.Component(nil), current.Components...)
		}
	}
	dest := filepath.Join(s.packagesRoot, name)
	if exists && req.Replace && existing.Digest == digest {
		if _, matches, verifyErr := verifyTreeDigest(dest, existing.Digest); verifyErr == nil && matches {
			if req.Builtin != nil {
				existing.Builtin = true
				existing.BrandIcon = strings.TrimSpace(req.Builtin.BrandIcon)
				existing.BuiltinOrder = req.Builtin.Order
				if displayName := strings.TrimSpace(req.Builtin.DisplayName); displayName != "" {
					existing.DisplayName = displayName
				}
				existing.Source = req.Source
				state.Plugins[name] = existing
				if err := s.writeStateUnlocked(state); err != nil {
					return InstallResult{}, err
				}
			}
			plugin := s.loadRecord(name, existing)
			return InstallResult{
				Plugin:   plugin,
				Replaced: false,
				Message:  "plugin digest unchanged",
			}, nil
		}
	}

	var previousTrash string
	var previousBackup string
	var rollbackRoot string
	if exists {
		if err := s.rejectUncontrolledPath(dest); err != nil {
			return InstallResult{}, err
		}
		if info, statErr := os.Lstat(dest); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return InstallResult{}, fmt.Errorf("%w: existing package is a symlink", ErrReplaceTrashUnavailable)
			}
			rollbackRoot, err = os.MkdirTemp(s.stagingRoot, "replace-*")
			if err != nil {
				return InstallResult{}, fmt.Errorf("create replacement rollback directory: %w", err)
			}
			if err := writeRollback(rollbackRoot, pluginRollback{Name: name, Previous: existing, PackageBackup: name, NextDigest: digest}); err != nil {
				os.RemoveAll(rollbackRoot)
				return InstallResult{}, err
			}
			defer finishPluginRollback(rollbackRoot, &operationErr)
			previousBackup = filepath.Join(rollbackRoot, name)
			if err := os.Rename(dest, previousBackup); err != nil {
				return InstallResult{}, fmt.Errorf("stage existing plugin package for replacement: %w", err)
			}
		} else if !os.IsNotExist(statErr) {
			return InstallResult{}, statErr
		}
	} else if _, statErr := os.Lstat(dest); statErr == nil {
		return InstallResult{}, fmt.Errorf("%w: unrecorded package already exists at %s", ErrStateCorrupt, dest)
	} else if !os.IsNotExist(statErr) {
		return InstallResult{}, statErr
	}

	if err := relocateTree(pluginRoot, dest); err != nil {
		if restoreErr := restorePluginPackage(dest, previousBackup); restoreErr != nil {
			return InstallResult{}, fmt.Errorf("activate plugin package: %v; restore previous package: %w", err, restoreErr)
		}
		return InstallResult{}, fmt.Errorf("activate plugin package: %w", err)
	}

	dataDir := filepath.Join(s.dataRoot, name)
	dataInfo, dataStatErr := os.Lstat(dataDir)
	dataExisted := dataStatErr == nil
	if dataStatErr != nil && !os.IsNotExist(dataStatErr) {
		_ = restorePluginPackage(dest, previousBackup)
		return InstallResult{}, dataStatErr
	}
	if dataExisted && (dataInfo.Mode()&os.ModeSymlink != 0 || !dataInfo.IsDir()) {
		_ = restorePluginPackage(dest, previousBackup)
		return InstallResult{}, fmt.Errorf("plugin data path must be a regular directory")
	}
	if err := filestore.EnsureDir(dataDir); err != nil {
		_ = restorePluginPackage(dest, previousBackup)
		return InstallResult{}, err
	}
	if err := resolvedStayWithin(s.dataRoot, dataDir); err != nil {
		if !dataExisted {
			_ = os.RemoveAll(dataDir)
		}
		_ = restorePluginPackage(dest, previousBackup)
		return InstallResult{}, fmt.Errorf("plugin data path escapes the managed data root: %w", err)
	}

	now := time.Now().UTC()
	rec := Record{
		Enabled: req.Builtin == nil, Version: manifest.Version, Source: req.Source, InstalledAt: now, Digest: digest,
		Dialect: normalized.Layout.Dialect, ManifestPath: normalized.Layout.ManifestPath,
		InstallationID: uuid.NewString(), DisplayName: normalized.Identity.DisplayName,
	}
	if req.Builtin != nil {
		rec.Builtin = true
		rec.BrandIcon = strings.TrimSpace(req.Builtin.BrandIcon)
		rec.BuiltinOrder = req.Builtin.Order
		if displayName := strings.TrimSpace(req.Builtin.DisplayName); displayName != "" {
			rec.DisplayName = displayName
		}
	}
	for _, secondary := range resolved.Secondary {
		rec.SecondaryManifests = append(rec.SecondaryManifests, secondary.ManifestPath)
	}
	if exists {
		rec.Enabled = existing.Enabled
		rec.MCPServers = existing.MCPServers
		rec.InstallationID = existing.InstallationID
		rec.Components, rec.Variables, rec.MigrationConflicts = migrateOverrides(existing, existingComponents, normalized.Components)
		if existing.InstalledAt.IsZero() {
			rec.InstalledAt = now
		} else {
			rec.InstalledAt = existing.InstalledAt
		}
	}
	state.Plugins[name] = rec
	if err := s.writeStateUnlocked(state); err != nil {
		if !dataExisted {
			_ = os.RemoveAll(dataDir)
		}
		if restoreErr := restorePluginPackage(dest, previousBackup); restoreErr != nil {
			return InstallResult{}, fmt.Errorf("write plugin state: %v; restore previous package: %w", err, restoreErr)
		}
		return InstallResult{}, err
	}
	if previousBackup != "" {
		moved, trashErr := s.trash.TrashPath(previousBackup)
		if trashErr != nil {
			restoreErr := restorePluginPackage(dest, previousBackup)
			state.Plugins[name] = existing
			stateErr := s.writeStateUnlocked(state)
			if restoreErr != nil || stateErr != nil {
				return InstallResult{}, fmt.Errorf("%w: %v; package rollback: %v; state rollback: %v", ErrReplaceTrashUnavailable, trashErr, restoreErr, stateErr)
			}
			return InstallResult{}, fmt.Errorf("%w: %v", ErrReplaceTrashUnavailable, trashErr)
		}
		previousTrash = moved.TrashedPath
	}
	plugin := s.loadRecord(name, rec)
	message := "plugin installed"
	if exists {
		message = "plugin replaced"
	}
	return InstallResult{
		Plugin:              plugin,
		Replaced:            exists,
		PreviousTrashedPath: previousTrash,
		Message:             message,
	}, nil
}

func migrateOverrides(existing Record, previousComponents, components []ir.Component) (map[string]ComponentOverride, map[string]VariableValueRef, []string) {
	componentByID := make(map[string]ir.Component, len(components))
	variableKeys := map[string]bool{}
	variableTypes := map[string]string{}
	previousVariableTypes := map[string]string{}
	for _, component := range previousComponents {
		if component.Variable != nil {
			previousVariableTypes[component.Variable.Key] = component.Variable.Type
		}
	}
	for _, component := range components {
		componentByID[component.LogicalID] = component
		if component.Variable != nil {
			variableKeys[component.Variable.Key] = true
			variableTypes[component.Variable.Key] = component.Variable.Type
		}
	}
	migratedComponents := map[string]ComponentOverride{}
	var conflicts []string
	for logicalID, override := range existing.Components {
		component, ok := componentByID[logicalID]
		if !ok {
			conflicts = append(conflicts, "component override no longer has a matching logicalId: "+logicalID)
			continue
		}
		override.BaseRevisionID = component.RevisionID
		migratedComponents[logicalID] = override
	}
	if len(migratedComponents) == 0 {
		migratedComponents = nil
	}
	migratedVariables := map[string]VariableValueRef{}
	for key, value := range existing.Variables {
		if variableKeys[key] && (previousVariableTypes[key] == "" || previousVariableTypes[key] == variableTypes[key]) {
			migratedVariables[key] = value
		} else if variableKeys[key] {
			conflicts = append(conflicts, "variable value type changed and was not migrated: "+key+" ("+previousVariableTypes[key]+" -> "+variableTypes[key]+")")
		} else {
			conflicts = append(conflicts, "variable value no longer has a matching schema: "+key)
		}
	}
	if len(migratedVariables) == 0 {
		migratedVariables = nil
	}
	sort.Strings(conflicts)
	return migratedComponents, migratedVariables, conflicts
}

func restorePluginPackage(dest, previousBackup string) error {
	if err := os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if previousBackup == "" {
		return nil
	}
	if err := os.Rename(previousBackup, dest); err != nil {
		return err
	}
	return nil
}

func (s *Store) rejectUncontrolledPath(path string) error {
	if err := stayWithin(s.pluginsRoot, path); err != nil {
		return fmt.Errorf("%w: %s is outside the plugins root", ErrPathEscapes, path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is a symlink", ErrReplaceTrashUnavailable, path)
	}
	return nil
}

func validateSource(source Source) error {
	if source.Kind != SourceLocalPath {
		return fmt.Errorf("unsupported plugin source %q", source.Kind)
	}
	if strings.TrimSpace(source.Path) == "" {
		return fmt.Errorf("%w: local_path requires path", ErrManifestInvalid)
	}
	return nil
}

func materializeSource(source Source, staging string) error {
	if source.Kind != SourceLocalPath {
		return fmt.Errorf("unsupported plugin source %q", source.Kind)
	}
	return materializeLocalPath(source.Path, staging)
}

func materializeLocalPath(raw, staging string) error {
	clean := filepath.Clean(strings.TrimSpace(raw))
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat local plugin path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("local plugin path must not be a symlink")
	}
	if info.IsDir() {
		return copyTree(clean, staging)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("local plugin path must be a directory or zip file")
	}
	if !strings.EqualFold(filepath.Ext(clean), ".zip") {
		return fmt.Errorf("local plugin file must be a .zip archive")
	}
	data, err := os.ReadFile(clean)
	if err != nil {
		return err
	}
	if int64(len(data)) > MaxArchiveBytes {
		return fmt.Errorf("plugin archive exceeds %d bytes", MaxArchiveBytes)
	}
	return extractZip(data, staging)
}

func locatePluginRoot(staging string) (string, error) {
	if isRegularFile(filepath.Join(staging, "plugin.json")) {
		return staging, nil
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.Name() == "__MACOSX" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(staging, entry.Name()))
		}
	}
	if len(dirs) == 1 && isRegularFile(filepath.Join(dirs[0], "plugin.json")) {
		return dirs[0], nil
	}
	return "", fmt.Errorf("%w: plugin.json was not found at the package root", ErrManifestInvalid)
}

func copyTree(src, dest string) error {
	var files int
	var total int64
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if err := stayWithin(src, path); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := rejectUnsafeFile(path, info); err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if err := stayWithin(dest, target); err != nil {
			return err
		}
		if entry.IsDir() {
			return filestore.EnsureDir(target)
		}
		files++
		if files > MaxPackageFiles {
			return fmt.Errorf("plugin package contains too many files")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > MaxExtractBytes {
			return fmt.Errorf("plugin package extracted content exceeds %d bytes", MaxExtractBytes)
		}
		if err := filestore.EnsureDir(filepath.Dir(target)); err != nil {
			return err
		}
		return os.WriteFile(target, data, safePluginFileMode(info.Mode()))
	})
}

func extractZip(data []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	if len(reader.File) > MaxPackageFiles {
		return fmt.Errorf("plugin package contains too many files")
	}
	var total int64
	var files int
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if name == "" || strings.HasSuffix(name, "/") {
			dir := filepath.Join(dest, filepath.FromSlash(strings.TrimSuffix(name, "/")))
			if name != "" {
				if err := rejectZipName(name[:len(name)-1]); err != nil {
					return err
				}
				if err := stayWithin(dest, dir); err != nil {
					return err
				}
			}
			if err := filestore.EnsureDir(dir); err != nil {
				return err
			}
			continue
		}
		if err := rejectZipName(name); err != nil {
			return err
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin archive cannot contain symlinks: %s", name)
		}
		if !mode.IsRegular() {
			return fmt.Errorf("plugin archive cannot contain special files: %s", name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if err := stayWithin(dest, target); err != nil {
			return fmt.Errorf("zip entry escapes destination: %s", name)
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		limited := io.LimitReader(rc, MaxExtractBytes-total+1)
		contents, err := io.ReadAll(limited)
		rc.Close()
		if err != nil {
			return err
		}
		total += int64(len(contents))
		if total > MaxExtractBytes {
			return fmt.Errorf("plugin package extracted content exceeds %d bytes", MaxExtractBytes)
		}
		files++
		if files > MaxPackageFiles {
			return fmt.Errorf("plugin package contains too many files")
		}
		if err := filestore.EnsureDir(filepath.Dir(target)); err != nil {
			return err
		}
		if err := os.WriteFile(target, contents, safePluginFileMode(mode)); err != nil {
			return err
		}
	}
	return nil
}

func safePluginFileMode(source os.FileMode) os.FileMode {
	if source.Perm()&0o111 != 0 {
		return 0o700
	}
	return 0o600
}

func rejectZipName(name string) error {
	if hasParentComponent(name) || isAbsoluteLike(name) {
		return fmt.Errorf("zip entry escapes destination: %s", name)
	}
	return nil
}

func rejectUnsafeFile(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("plugin package cannot contain symlinks: %s", path)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("plugin package cannot contain special files: %s", path)
	}
	if info.Mode().IsRegular() {
		if hasMultipleHardlinks(info) {
			return fmt.Errorf("plugin package cannot contain hardlinks: %s", path)
		}
	}
	return nil
}

func relocateTree(src, dest string) error {
	if err := filestore.EnsureDir(filepath.Dir(dest)); err != nil {
		return err
	}
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	if err := copyTree(src, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	return nil
}
