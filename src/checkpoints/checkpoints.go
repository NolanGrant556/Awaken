package checkpoints

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/tool"
)

type FileState string

const (
	FileStateExisting FileState = "existing"
	FileStateMissing  FileState = "missing"
)

type FileSnapshot struct {
	Path    string
	AbsPath string
	State   FileState
	Mode    fs.FileMode
	Size    int64
	SHA256  string
	Content []byte
}

type Checkpoint struct {
	ID            string
	WorkspaceRoot string
	CreatedAt     time.Time
	Snapshots     []FileSnapshot
}

type Summary struct {
	Existing int
	Missing  int
}

func (s Summary) String() string {
	return fmt.Sprintf("%d existing, %d missing", s.Existing, s.Missing)
}

func (c Checkpoint) Summary() Summary {
	var summary Summary
	for _, snapshot := range c.Snapshots {
		switch snapshot.State {
		case FileStateExisting:
			summary.Existing++
		case FileStateMissing:
			summary.Missing++
		}
	}
	return summary
}

func (c Checkpoint) SummaryString() string {
	return c.Summary().String()
}

type Store struct {
	mu          sync.RWMutex
	checkpoints map[string]Checkpoint
	counter     uint64
}

func NewStore() *Store {
	return &Store{
		checkpoints: make(map[string]Checkpoint),
	}
}

var defaultStore = NewStore()

func DefaultStore() *Store {
	return defaultStore
}

func Create(ctx context.Context, permissionContext tool.PermissionContext, affectedPaths []string) (Checkpoint, error) {
	return defaultStore.Create(ctx, permissionContext, affectedPaths)
}

func (s *Store) Create(ctx context.Context, permissionContext tool.PermissionContext, affectedPaths []string) (Checkpoint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	permissionContext = permissionContext.Normalize()
	paths := normalizeAffectedPaths(permissionContext, affectedPaths)
	if len(paths) == 0 {
		return Checkpoint{}, fmt.Errorf("checkpoint requires at least one affected path")
	}

	snapshots := make([]FileSnapshot, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return Checkpoint{}, err
		}
		snapshot, err := snapshotPath(permissionContext, path)
		if err != nil {
			return Checkpoint{}, err
		}
		snapshots = append(snapshots, snapshot)
	}

	checkpoint := Checkpoint{
		ID:            s.nextID(),
		WorkspaceRoot: permissionContext.WorkspaceRootOrCWD(),
		CreatedAt:     time.Now().UTC(),
		Snapshots:     snapshots,
	}

	s.mu.Lock()
	s.checkpoints[checkpoint.ID] = cloneCheckpoint(checkpoint)
	s.mu.Unlock()

	return cloneCheckpoint(checkpoint), nil
}

func (s *Store) Get(id string) (Checkpoint, bool) {
	s.mu.RLock()
	checkpoint, ok := s.checkpoints[id]
	s.mu.RUnlock()
	if !ok {
		return Checkpoint{}, false
	}
	return cloneCheckpoint(checkpoint), true
}

func (s *Store) nextID() string {
	seq := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("checkpoint_%d_%d", time.Now().UTC().UnixNano(), seq)
}

func normalizeAffectedPaths(permissionContext tool.PermissionContext, affectedPaths []string) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0, len(affectedPaths))
	for _, affectedPath := range affectedPaths {
		path := filepath.Clean(permissionContext.ResolvePath(affectedPath))
		if path == "." || path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func snapshotPath(permissionContext tool.PermissionContext, path string) (FileSnapshot, error) {
	path = filepath.Clean(permissionContext.ResolvePath(path))
	if path == "." || path == "" {
		return FileSnapshot{}, fmt.Errorf("path is empty")
	}
	if err := validateSnapshotPath(path, permissionContext); err != nil {
		return FileSnapshot{}, err
	}

	displayPath := workspaceRelativePath(path, permissionContext.WorkspaceRootOrCWD())
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileSnapshot{
				Path:    displayPath,
				AbsPath: path,
				State:   FileStateMissing,
			}, nil
		}
		return FileSnapshot{}, fmt.Errorf("failed to inspect file %s: %w", path, err)
	}
	if info.IsDir() {
		return FileSnapshot{}, fmt.Errorf("failed to snapshot file %s: path is a directory", path)
	}
	if !info.Mode().IsRegular() {
		return FileSnapshot{}, fmt.Errorf("failed to snapshot file %s: path is not a regular file", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return FileSnapshot{}, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	sum := sha256.Sum256(content)
	return FileSnapshot{
		Path:    displayPath,
		AbsPath: path,
		State:   FileStateExisting,
		Mode:    info.Mode(),
		Size:    info.Size(),
		SHA256:  hex.EncodeToString(sum[:]),
		Content: append([]byte(nil), content...),
	}, nil
}

func validateSnapshotPath(path string, permissionContext tool.PermissionContext) error {
	if !permissionContext.IsWithinWritableRoots(path) {
		return fmt.Errorf("path %q is outside writable roots", path)
	}

	current := path
	for {
		info, err := os.Lstat(current)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return fmt.Errorf("cannot resolve path %q: %w", current, resolveErr)
			}
			if !permissionContext.IsWithinWritableRootsResolved(resolved) {
				return fmt.Errorf("path %q resolves to %q which is outside writable roots", path, resolved)
			}
			if current != path && !info.IsDir() {
				return fmt.Errorf("path %q cannot be created under non-directory %q", path, current)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot inspect path %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("path %q has no existing parent within writable roots", path)
		}
		current = parent
	}
}

func workspaceRelativePath(path string, root string) string {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil || absRoot == "" {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func cloneCheckpoint(checkpoint Checkpoint) Checkpoint {
	checkpoint.Snapshots = append([]FileSnapshot(nil), checkpoint.Snapshots...)
	for i := range checkpoint.Snapshots {
		checkpoint.Snapshots[i].Content = append([]byte(nil), checkpoint.Snapshots[i].Content...)
	}
	return checkpoint
}
