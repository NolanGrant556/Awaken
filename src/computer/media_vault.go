package computer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MediaKind classifies the type of media stored in the session media store.
type MediaKind string

const (
	MediaKindScreenshot        MediaKind = "screenshot"
	MediaKindAccessibilityTree MediaKind = "accessibility_tree"
)

type MediaAccessAuditFunc func(ctx context.Context, sessionID SessionID, ref MediaRef)
type MediaDeleteAuditFunc func(ctx context.Context, sessionID SessionID, ref MediaRef, reason string)

// MediaVault is a session-scoped local media store. The historical name is
// retained to avoid a broad API migration; media is intentionally stored as
// ordinary 0600 files because model-facing tool media is also persisted by the
// existing ToolResultStore. Security comes from userData isolation, short
// retention, session ownership checks, access audit, and explicit deletion.
type MediaVault interface {
	Store(ctx context.Context, sessionID SessionID, kind MediaKind, data []byte) (MediaRef, error)
	Read(ctx context.Context, sessionID SessionID, ref MediaRef) ([]byte, error)
	PurgeSession(ctx context.Context, sessionID SessionID) error
	Close() error
}

type VaultOptions struct {
	RootDir   string
	OnAccess  MediaAccessAuditFunc
	OnDelete  MediaDeleteAuditFunc
	Retention RetentionOptions
}

type RetentionOptions struct {
	MaxAge          time.Duration
	MaxTotalBytes   int64
	CleanupInterval time.Duration
}

// NewFileMediaVault creates a private, session-scoped media store.
func NewFileMediaVault(opts VaultOptions) (MediaVault, error) {
	if opts.RootDir == "" {
		return nil, fmt.Errorf("media root directory is required")
	}
	if err := os.MkdirAll(opts.RootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create media root: %w", err)
	}
	if err := os.Chmod(opts.RootDir, 0o700); err != nil {
		return nil, fmt.Errorf("secure media root: %w", err)
	}
	retention := opts.Retention
	if retention.CleanupInterval <= 0 && (retention.MaxAge > 0 || retention.MaxTotalBytes > 0) {
		retention.CleanupInterval = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	store := &fileMediaVault{
		rootDir: opts.RootDir, onAccess: opts.OnAccess, onDelete: opts.OnDelete,
		retention: retention, closeCtx: ctx, closeCancel: cancel,
	}
	if retention.MaxAge > 0 || retention.MaxTotalBytes > 0 {
		store.wg.Add(1)
		go store.cleanupLoop()
	}
	return store, nil
}

type fileMediaVault struct {
	rootDir     string
	onAccess    MediaAccessAuditFunc
	onDelete    MediaDeleteAuditFunc
	retention   RetentionOptions
	mu          sync.Mutex
	closed      bool
	closeCtx    context.Context
	closeCancel context.CancelFunc
	wg          sync.WaitGroup
}

func (v *fileMediaVault) Close() error {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return nil
	}
	v.closed = true
	v.closeCancel()
	v.mu.Unlock()
	v.wg.Wait()
	return nil
}

func (v *fileMediaVault) Store(_ context.Context, sessionID SessionID, kind MediaKind, data []byte) (MediaRef, error) {
	if err := v.ensureOpen(); err != nil {
		return MediaRef{}, err
	}
	if sessionID == "" {
		return MediaRef{}, fmt.Errorf("sessionID is required")
	}
	if len(data) == 0 {
		return MediaRef{}, fmt.Errorf("empty media cannot be stored")
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	rel := filepath.Join("sessions", string(sessionID), string(kind), sha+".bin")
	path, err := v.ownedPath(rel)
	if err != nil {
		return MediaRef{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return MediaRef{}, fmt.Errorf("create media dir: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return MediaRef{}, fmt.Errorf("secure media dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return MediaRef{}, fmt.Errorf("write media file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return MediaRef{}, fmt.Errorf("secure media file: %w", err)
	}
	return MediaRef{
		StorageKey: rel,
		ByteSize:   int64(len(data)),
		SHA256:     sha,
		SessionID:  sessionID,
	}, nil
}

func (v *fileMediaVault) Read(ctx context.Context, sessionID SessionID, ref MediaRef) ([]byte, error) {
	if err := v.ensureOpen(); err != nil {
		return nil, err
	}
	if ref.SessionID == "" || ref.SessionID != sessionID {
		return nil, fmt.Errorf("media session mismatch")
	}
	path, err := v.ownedPath(ref.StorageKey)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read media file: %w", err)
	}
	sum := sha256.Sum256(data)
	if ref.SHA256 != "" && hex.EncodeToString(sum[:]) != ref.SHA256 {
		return nil, fmt.Errorf("media digest mismatch")
	}
	if v.onAccess != nil {
		v.onAccess(ctx, sessionID, ref)
	}
	return data, nil
}

func (v *fileMediaVault) PurgeSession(ctx context.Context, sessionID SessionID) error {
	if err := v.ensureOpen(); err != nil {
		return err
	}
	if sessionID == "" {
		return nil
	}
	path, err := v.ownedPath(filepath.Join("sessions", string(sessionID)))
	if err != nil {
		return err
	}
	files, _, err := v.listVaultFilesUnder(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, file := range files {
		if err := v.deleteFile(ctx, file, "session_purge"); err != nil {
			return fmt.Errorf("purge session media: %w", err)
		}
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("purge session media: %w", err)
	}
	return nil
}

func (v *fileMediaVault) ensureOpen() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return fmt.Errorf("media store is closed")
	}
	return nil
}

func (v *fileMediaVault) ownedPath(storageKey string) (string, error) {
	root, err := filepath.Abs(v.rootDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(storageKey)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("media storage key escapes store root")
	}
	return target, nil
}

func (v *fileMediaVault) cleanupLoop() {
	defer v.wg.Done()
	ticker := time.NewTicker(v.retention.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-v.closeCtx.Done():
			return
		case <-ticker.C:
			if err := v.cleanupOnce(v.closeCtx); err != nil {
				log.Printf("[computer-media] cleanup error: %v", err)
			}
		}
	}
}

type vaultFile struct {
	path       string
	storageKey string
	sessionID  SessionID
	sha        string
	size       int64
	modTime    time.Time
}

func (v *fileMediaVault) cleanupOnce(ctx context.Context) error {
	const safetyGrace = time.Minute
	now := time.Now().UTC()
	files, totalBytes, err := v.listVaultFiles()
	if err != nil {
		return err
	}
	removedPaths := make(map[string]struct{})
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if v.retention.MaxAge > 0 && now.Sub(file.modTime) > v.retention.MaxAge {
			if err := v.deleteFile(ctx, file, "age"); err != nil {
				log.Printf("[computer-media] failed to remove aged file %s: %v", file.path, err)
				continue
			}
			removedPaths[file.path] = struct{}{}
			totalBytes -= file.size
		}
	}
	if v.retention.MaxTotalBytes > 0 && totalBytes > v.retention.MaxTotalBytes {
		sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
		for _, file := range files {
			if totalBytes <= v.retention.MaxTotalBytes {
				break
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, removed := removedPaths[file.path]; removed || now.Sub(file.modTime) < safetyGrace {
				continue
			}
			if err := v.deleteFile(ctx, file, "capacity"); err != nil {
				log.Printf("[computer-media] failed to remove capacity-evicted file %s: %v", file.path, err)
				continue
			}
			totalBytes -= file.size
		}
	}
	return nil
}

func (v *fileMediaVault) listVaultFiles() ([]vaultFile, int64, error) {
	return v.listVaultFilesUnder(v.rootDir)
}

func (v *fileMediaVault) listVaultFilesUnder(walkRoot string) ([]vaultFile, int64, error) {
	var files []vaultFile
	var total int64
	err := filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".bin" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(v.rootDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(rel, string(filepath.Separator))
		var sessionID SessionID
		if len(parts) >= 4 && parts[0] == "sessions" {
			sessionID = SessionID(parts[1])
		}
		sha := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		files = append(files, vaultFile{
			path: path, storageKey: rel, sessionID: sessionID, sha: sha,
			size: info.Size(), modTime: info.ModTime().UTC(),
		})
		total += info.Size()
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list media files: %w", err)
	}
	return files, total, nil
}

func (v *fileMediaVault) deleteFile(ctx context.Context, file vaultFile, reason string) error {
	if err := os.Remove(file.path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if v.onDelete != nil {
		v.onDelete(ctx, file.sessionID, MediaRef{
			StorageKey: file.storageKey,
			SHA256:     file.sha,
			SessionID:  file.sessionID,
			ByteSize:   file.size,
		}, reason)
	}
	return nil
}
