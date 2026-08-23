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
	"strings"
	"syscall"
	"time"

	"awaken/src/filestore"
)

// Install copies a plugin package into the store. Same-name installs abort
// unless Replace is set. Installation never executes package code.
func (s *Store) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if err := validateSource(req.Source); err != nil {
		return InstallResult{}, err
	}
	var result InstallResult
	err := s.withLock(func() error {
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

// Upgrade re-fetches the recorded source. Identical digest is a no-op.
func (s *Store) Upgrade(ctx context.Context, name string) (InstallResult, error) {
	var result InstallResult
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		rec, ok := state.Plugins[name]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		installed, err := s.installLocked(ctx, InstallRequest{Source: rec.Source, Replace: true})
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
	err := s.withLock(func() error {
		state, err := s.readStateUnlocked()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if _, ok := state.Plugins[name]; !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
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
		if info, statErr := os.Lstat(pkg); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: package is a symlink", ErrReplaceTrashUnavailable)
			}
			moved, trashErr := s.trash.TrashPath(pkg)
			if trashErr != nil {
				return fmt.Errorf("%w: %v", ErrReplaceTrashUnavailable, trashErr)
			}
			trashed = moved.TrashedPath
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		if _, statErr := os.Lstat(data); statErr == nil {
			moved, trashErr := s.trash.TrashPath(data)
			if trashErr != nil {
				return fmt.Errorf("%w: %v", ErrReplaceTrashUnavailable, trashErr)
			}
			if trashed == "" {
				trashed = moved.TrashedPath
			}
		}
		delete(state.Plugins, name)
		return s.writeStateUnlocked(state)
	})
	if err == nil {
		s.notify()
	}
	return trashed, err
}

func (s *Store) installLocked(ctx context.Context, req InstallRequest) (InstallResult, error) {
	state, err := s.readStateUnlocked()
	if err != nil && !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	staging, err := os.MkdirTemp(s.stagingRoot, "pkg-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := materializeSource(ctx, req.Source, staging); err != nil {
		return InstallResult{}, err
	}
	pluginRoot, err := locatePluginRoot(staging)
	if err != nil {
		return InstallResult{}, err
	}
	parsed, err := loadManifestFile(pluginRoot)
	if err != nil {
		return InstallResult{}, err
	}
	name := parsed.Manifest.Name
	digest, err := digestTree(pluginRoot)
	if err != nil {
		return InstallResult{}, fmt.Errorf("compute plugin digest: %w", err)
	}

	existing, exists := state.Plugins[name]
	if exists && !req.Replace {
		return InstallResult{}, fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}
	if exists && req.Replace && existing.Digest == digest {
		plugin := s.loadRecord(name, existing)
		return InstallResult{
			Plugin:   plugin,
			Replaced: false,
			Message:  "plugin digest unchanged",
		}, nil
	}

	dest := filepath.Join(s.packagesRoot, name)
	var previousTrash string
	if exists {
		if err := s.rejectUncontrolledPath(dest); err != nil {
			return InstallResult{}, err
		}
		if info, statErr := os.Lstat(dest); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return InstallResult{}, fmt.Errorf("%w: existing package is a symlink", ErrReplaceTrashUnavailable)
			}
			moved, trashErr := s.trash.TrashPath(dest)
			if trashErr != nil {
				return InstallResult{}, fmt.Errorf("%w: %v", ErrReplaceTrashUnavailable, trashErr)
			}
			previousTrash = moved.TrashedPath
		} else if !os.IsNotExist(statErr) {
			return InstallResult{}, statErr
		}
	}

	if err := os.RemoveAll(dest); err != nil && !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	if err := relocateTree(pluginRoot, dest); err != nil {
		return InstallResult{}, fmt.Errorf("activate plugin package: %w", err)
	}

	dataDir := filepath.Join(s.dataRoot, name)
	if err := filestore.EnsureDir(dataDir); err != nil {
		return InstallResult{}, err
	}

	now := time.Now().UTC()
	rec := Record{
		Enabled:     true,
		Version:     parsed.Manifest.Version,
		Source:      req.Source,
		InstalledAt: now,
		Digest:      digest,
	}
	if exists {
		rec.Enabled = existing.Enabled
		rec.MCPServers = existing.MCPServers
		if existing.InstalledAt.IsZero() {
			rec.InstalledAt = now
		} else {
			rec.InstalledAt = existing.InstalledAt
		}
	}
	state.Plugins[name] = rec
	if err := s.writeStateUnlocked(state); err != nil {
		return InstallResult{}, err
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
	switch source.Kind {
	case SourceLocalPath:
		if strings.TrimSpace(source.Path) == "" {
			return fmt.Errorf("%w: local_path requires path", ErrManifestInvalid)
		}
		return nil
	case SourceRemoteURL:
		return validateHTTPSURL(source.URL, "remote_url")
	case SourceRemoteGit:
		if _, err := cleanRemoteGitURL(source.URL); err != nil {
			return err
		}
		if _, err := cleanGitRef(source.Ref); err != nil {
			return err
		}
		if _, err := cleanGitSubpath(source.Subpath); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported plugin source %q", source.Kind)
	}
}

func materializeSource(ctx context.Context, source Source, staging string) error {
	switch source.Kind {
	case SourceLocalPath:
		return materializeLocalPath(source.Path, staging)
	case SourceRemoteURL:
		return materializeRemoteURL(ctx, source.URL, staging)
	case SourceRemoteGit:
		return materializeRemoteGit(ctx, source, staging)
	default:
		return fmt.Errorf("unsupported plugin source %q", source.Kind)
	}
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
	if int64(len(data)) > MaxDownloadBytes {
		return fmt.Errorf("plugin archive exceeds %d bytes", MaxDownloadBytes)
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
		return os.WriteFile(target, data, 0o600)
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
		if err := os.WriteFile(target, contents, 0o600); err != nil {
			return err
		}
	}
	return nil
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
		if sys, ok := info.Sys().(*syscall.Stat_t); ok && sys.Nlink > 1 {
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
