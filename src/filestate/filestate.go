package filestate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const DefaultMaxHashBytes int64 = 1024 * 1024

type DecisionCode string

const (
	DecisionAllow               DecisionCode = "allow"
	DecisionAllowNewFile        DecisionCode = "allow_new_file"
	DecisionAllowOverride       DecisionCode = "allow_override"
	DecisionAllowSameWriter     DecisionCode = "allow_same_writer"
	DecisionBlockDirectory      DecisionCode = "block_directory"
	DecisionBlockDiskChanged    DecisionCode = "block_disk_changed"
	DecisionBlockNewFile        DecisionCode = "block_new_file"
	DecisionBlockOtherTaskWrite DecisionCode = "block_other_task_write"
	DecisionBlockPartialRead    DecisionCode = "block_partial_read"
	DecisionBlockPathInvalid    DecisionCode = "block_path_invalid"
	DecisionBlockUnreadExisting DecisionCode = "block_unread_existing"
)

type Snapshot struct {
	Path          string
	Exists        bool
	IsDir         bool
	ModTime       time.Time
	Size          int64
	Hash          string
	HashAvailable bool
	HashSkipped   bool
	MaxHashBytes  int64
	CapturedAt    time.Time
}

type ReadRecord struct {
	TaskID   string
	Path     string
	Snapshot Snapshot
	Partial  bool
	At       time.Time
}

type WriteRecord struct {
	TaskID   string
	Path     string
	Snapshot Snapshot
	At       time.Time
}

type ExternalSnapshotRecord struct {
	Path     string
	Snapshot Snapshot
	At       time.Time
}

type SnapshotDiff struct {
	Changed bool
	Reasons []string
}

type Decision struct {
	Allowed    bool
	Code       DecisionCode
	Reason     string
	Path       string
	Current    Snapshot
	Read       *ReadRecord
	LastWriter *WriteRecord
	Override   bool
}

type ValidateOptions struct {
	AllowNewFile    bool
	AllowSameWriter bool
	Override        bool
}

type Option func(*Registry)

func WithMaxHashBytes(maxBytes int64) Option {
	return func(registry *Registry) {
		if maxBytes < 0 {
			maxBytes = 0
		}
		registry.maxHashBytes = maxBytes
	}
}

type Registry struct {
	root         string
	realRoot     string
	maxHashBytes int64

	mu        sync.Mutex
	reads     map[string]map[string]ReadRecord
	writes    map[string]WriteRecord
	externals map[string]ExternalSnapshotRecord

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex
}

func NewRegistry(workspaceRoot string, options ...Option) (*Registry, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	realRoot = filepath.Clean(realRoot)
	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect workspace root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace root %q is not a directory", workspaceRoot)
	}

	registry := &Registry{
		root:         absRoot,
		realRoot:     realRoot,
		maxHashBytes: DefaultMaxHashBytes,
		reads:        make(map[string]map[string]ReadRecord),
		writes:       make(map[string]WriteRecord),
		externals:    make(map[string]ExternalSnapshotRecord),
		locks:        make(map[string]*sync.Mutex),
	}
	for _, option := range options {
		option(registry)
	}
	return registry, nil
}

func (r *Registry) WorkspaceRoot() string {
	return r.root
}

func (r *Registry) RealWorkspaceRoot() string {
	return r.realRoot
}

func (r *Registry) MaxHashBytes() int64 {
	return r.maxHashBytes
}

func (r *Registry) NormalizePath(path string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("file state registry is nil")
	}
	raw := strings.TrimSpace(path)
	if raw == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("path contains NUL byte")
	}
	raw = expandTilde(raw)
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(r.root, raw)
	}
	absPath, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	absPath = filepath.Clean(absPath)
	if !pathWithinRoot(absPath, r.root) {
		return "", fmt.Errorf("path %q escapes workspace root %q", path, r.root)
	}

	normalized, err := r.canonicalPath(absPath)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(normalized, r.realRoot) {
		return "", fmt.Errorf("path %q resolves outside workspace root", path)
	}
	return normalized, nil
}

func (r *Registry) Snapshot(path string) (Snapshot, error) {
	normalized, err := r.NormalizePath(path)
	if err != nil {
		return Snapshot{}, err
	}
	return r.snapshotNormalized(normalized)
}

func (r *Registry) RecordRead(taskID string, path string, partial bool) (ReadRecord, error) {
	taskID, err := normalizeTaskID(taskID)
	if err != nil {
		return ReadRecord{}, err
	}
	snapshot, err := r.Snapshot(path)
	if err != nil {
		return ReadRecord{}, err
	}
	record := ReadRecord{
		TaskID:   taskID,
		Path:     snapshot.Path,
		Snapshot: snapshot,
		Partial:  partial,
		At:       time.Now(),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.reads[taskID]; !ok {
		r.reads[taskID] = make(map[string]ReadRecord)
	}
	r.reads[taskID][snapshot.Path] = record
	return record, nil
}

func (r *Registry) RecordWrite(taskID string, path string) (WriteRecord, error) {
	taskID, err := normalizeTaskID(taskID)
	if err != nil {
		return WriteRecord{}, err
	}
	snapshot, err := r.Snapshot(path)
	if err != nil {
		return WriteRecord{}, err
	}
	now := time.Now()
	write := WriteRecord{
		TaskID:   taskID,
		Path:     snapshot.Path,
		Snapshot: snapshot,
		At:       now,
	}
	read := ReadRecord{
		TaskID:   taskID,
		Path:     snapshot.Path,
		Snapshot: snapshot,
		Partial:  false,
		At:       now,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes[snapshot.Path] = write
	if _, ok := r.reads[taskID]; !ok {
		r.reads[taskID] = make(map[string]ReadRecord)
	}
	r.reads[taskID][snapshot.Path] = read
	return write, nil
}

func (r *Registry) RecordExternalSnapshot(path string) (ExternalSnapshotRecord, error) {
	snapshot, err := r.Snapshot(path)
	if err != nil {
		return ExternalSnapshotRecord{}, err
	}
	record := ExternalSnapshotRecord{
		Path:     snapshot.Path,
		Snapshot: snapshot,
		At:       time.Now(),
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.externals[snapshot.Path] = record
	return record, nil
}

func (r *Registry) ValidateWrite(taskID string, path string, options ValidateOptions) (Decision, error) {
	taskID, err := normalizeTaskID(taskID)
	if err != nil {
		return Decision{}, err
	}
	current, err := r.Snapshot(path)
	if err != nil {
		return Decision{
			Allowed: false,
			Code:    DecisionBlockPathInvalid,
			Reason:  err.Error(),
		}, err
	}

	read, hasRead, writer, hasWriter := r.recordsFor(taskID, current.Path)
	decision := Decision{
		Path:    current.Path,
		Current: current,
	}
	if hasRead {
		readCopy := read
		decision.Read = &readCopy
	}
	if hasWriter {
		writerCopy := writer
		decision.LastWriter = &writerCopy
	}

	if current.Exists && current.IsDir {
		return decision.block(DecisionBlockDirectory, fmt.Sprintf("%s is a directory", current.Path)), nil
	}
	if !current.Exists {
		if options.Override {
			return decision.allow(DecisionAllowOverride, "explicit override allowed write to missing path", true), nil
		}
		if options.AllowNewFile {
			return decision.allow(DecisionAllowNewFile, "new file creation is allowed", false), nil
		}
		return decision.block(DecisionBlockNewFile, fmt.Sprintf("%s does not exist and new file writes are not allowed", current.Path)), nil
	}
	if options.Override {
		return decision.allow(DecisionAllowOverride, "explicit override allowed write", true), nil
	}

	if hasWriter && writer.TaskID != taskID {
		if !hasRead {
			return decision.block(
				DecisionBlockOtherTaskWrite,
				fmt.Sprintf("%s was written by task %q, but task %q has not read it", current.Path, writer.TaskID, taskID),
			), nil
		}
		if writer.At.After(read.At) {
			return decision.block(
				DecisionBlockOtherTaskWrite,
				fmt.Sprintf("%s was written by task %q after task %q last read it", current.Path, writer.TaskID, taskID),
			), nil
		}
	}

	if !hasRead {
		if options.AllowSameWriter && hasWriter && writer.TaskID == taskID && snapshotsMatch(current, writer.Snapshot) {
			return decision.allow(DecisionAllowSameWriter, "same task wrote the current file state", false), nil
		}
		return decision.block(
			DecisionBlockUnreadExisting,
			fmt.Sprintf("%s exists but task %q has not read it", current.Path, taskID),
		), nil
	}

	if read.Partial {
		return decision.block(
			DecisionBlockPartialRead,
			fmt.Sprintf("%s was last read by task %q as a partial view", current.Path, taskID),
		), nil
	}

	diff := current.Diff(read.Snapshot)
	if diff.Changed {
		if options.AllowSameWriter && hasWriter && writer.TaskID == taskID && !writer.At.Before(read.At) && snapshotsMatch(current, writer.Snapshot) {
			return decision.allow(DecisionAllowSameWriter, "same task wrote the current file state after the read", false), nil
		}
		return decision.block(
			DecisionBlockDiskChanged,
			fmt.Sprintf("%s changed on disk since task %q last read it (%s)", current.Path, taskID, strings.Join(diff.Reasons, ", ")),
		), nil
	}

	return decision.allow(DecisionAllow, "file state matches last full read", false), nil
}

func (r *Registry) KnownReads(taskID string) []ReadRecord {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	reads := r.reads[taskID]
	out := make([]ReadRecord, 0, len(reads))
	for _, record := range reads {
		out = append(out, record)
	}
	return out
}

func (r *Registry) LastWriter(path string) (WriteRecord, bool, error) {
	normalized, err := r.NormalizePath(path)
	if err != nil {
		return WriteRecord{}, false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.writes[normalized]
	return record, ok, nil
}

func (r *Registry) ExternalSnapshot(path string) (ExternalSnapshotRecord, bool, error) {
	normalized, err := r.NormalizePath(path)
	if err != nil {
		return ExternalSnapshotRecord{}, false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.externals[normalized]
	return record, ok, nil
}

func (r *Registry) LockPath(path string) (func(), error) {
	normalized, err := r.NormalizePath(path)
	if err != nil {
		return nil, err
	}

	r.locksMu.Lock()
	lock, ok := r.locks[normalized]
	if !ok {
		lock = &sync.Mutex{}
		r.locks[normalized] = lock
	}
	r.locksMu.Unlock()

	lock.Lock()
	return lock.Unlock, nil
}

func (s Snapshot) Diff(previous Snapshot) SnapshotDiff {
	var reasons []string
	if s.Exists != previous.Exists {
		reasons = append(reasons, "existence")
	}
	if s.IsDir != previous.IsDir {
		reasons = append(reasons, "type")
	}
	if s.Size != previous.Size {
		reasons = append(reasons, "size")
	}
	if s.HashAvailable && previous.HashAvailable && s.Hash != previous.Hash {
		reasons = append(reasons, "hash")
	}
	if s.Exists && previous.Exists && !s.ModTime.Equal(previous.ModTime) {
		reasons = append(reasons, "mtime")
	}
	return SnapshotDiff{
		Changed: len(reasons) > 0,
		Reasons: reasons,
	}
}

func (d Decision) allow(code DecisionCode, reason string, override bool) Decision {
	d.Allowed = true
	d.Code = code
	d.Reason = reason
	d.Override = override
	return d
}

func (d Decision) block(code DecisionCode, reason string) Decision {
	d.Allowed = false
	d.Code = code
	d.Reason = reason
	return d
}

func (r *Registry) recordsFor(taskID string, path string) (ReadRecord, bool, WriteRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var read ReadRecord
	var hasRead bool
	if reads := r.reads[taskID]; reads != nil {
		read, hasRead = reads[path]
	}
	writer, hasWriter := r.writes[path]
	return read, hasRead, writer, hasWriter
}

func (r *Registry) snapshotNormalized(path string) (Snapshot, error) {
	snapshot := Snapshot{
		Path:         path,
		MaxHashBytes: r.maxHashBytes,
		CapturedAt:   time.Now(),
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return snapshot, nil
		}
		return Snapshot{}, fmt.Errorf("inspect path %q: %w", path, err)
	}

	snapshot.Exists = true
	snapshot.IsDir = info.IsDir()
	snapshot.ModTime = info.ModTime()
	snapshot.Size = info.Size()
	if info.IsDir() {
		return snapshot, nil
	}
	if info.Size() > r.maxHashBytes {
		snapshot.HashSkipped = true
		return snapshot, nil
	}

	hash, available, skipped, err := hashFile(path, r.maxHashBytes)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Hash = hash
	snapshot.HashAvailable = available
	snapshot.HashSkipped = skipped
	return snapshot, nil
}

func (r *Registry) canonicalPath(absPath string) (string, error) {
	existing := absPath
	for {
		info, err := os.Lstat(existing)
		if err == nil {
			if existing != absPath && !info.IsDir() {
				return "", fmt.Errorf("existing ancestor %q is not a directory", existing)
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect path ancestor %q: %w", existing, err)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("no existing ancestor for %q", absPath)
		}
		existing = parent
	}

	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks for %q: %w", existing, err)
	}
	realExisting = filepath.Clean(realExisting)
	if !pathWithinRoot(realExisting, r.realRoot) {
		return "", fmt.Errorf("path %q resolves outside workspace root", absPath)
	}
	if existing == absPath {
		return realExisting, nil
	}

	rel, err := filepath.Rel(existing, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path suffix for %q: %w", absPath, err)
	}
	normalized := filepath.Clean(filepath.Join(realExisting, rel))
	if !pathWithinRoot(normalized, r.realRoot) {
		return "", fmt.Errorf("path %q resolves outside workspace root", absPath)
	}
	return normalized, nil
}

func hashFile(path string, maxBytes int64) (string, bool, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, false, fmt.Errorf("open file for hash %q: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	copied, err := io.Copy(hash, io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", false, false, fmt.Errorf("hash file %q: %w", path, err)
	}
	if copied > maxBytes {
		return "", false, true, nil
	}
	return hex.EncodeToString(hash.Sum(nil)), true, false, nil
}

func snapshotsMatch(left Snapshot, right Snapshot) bool {
	return !left.Diff(right).Changed
}

func normalizeTaskID(taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task ID is empty")
	}
	return taskID, nil
}

func expandTilde(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func pathWithinRoot(path string, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(absRoot), filepath.Clean(absPath))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
