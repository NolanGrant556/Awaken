package tool

import (
	"context"
	"path/filepath"
	"sync"
	"time"
)

// FileReadState is a host-computed snapshot proving what a thread observed.
// It is deliberately runtime-only and never enters SemanticJournal.
type FileReadState struct {
	CanonicalPath string
	SHA256        string
	Size          int64
	ModTime       time.Time
	Mode          uint32
	MimeType      string
}

// ReadProjectionState records the exact model-visible projection produced by
// Read for one file version. It is intentionally separate from FileReadState:
// Edit/Write only need proof that the thread observed the current file, while
// Read deduplication must also match the requested range/route.
type ReadProjectionState struct {
	Snapshot      FileReadState
	ProjectionKey string
}

// MediaGroupState tracks one image generation group's durable presentation
// identity during a thread process. It lets a follow-up tool call fill failed
// positions without turning the retry into a newly numbered group.
type MediaGroupState struct {
	ID             string
	Kind           string
	RequestedCount int
	ItemStatus     map[int]string
}

// ToolSessionState owns mutable thread-local tool state. Agent replacement,
// process restart and thread deletion therefore invalidate it safely.
type ToolSessionState struct {
	mu              sync.RWMutex
	files           map[string]FileReadState
	readProjections map[string]ReadProjectionState
	mediaGroups     map[string]MediaGroupState
	cwd             string
	pathMu          sync.Mutex
	pathLock        map[string]*sync.Mutex
}

func NewToolSessionState(initialCWD string) *ToolSessionState {
	state := &ToolSessionState{
		files:           make(map[string]FileReadState),
		readProjections: make(map[string]ReadProjectionState),
		mediaGroups:     make(map[string]MediaGroupState),
		pathLock:        make(map[string]*sync.Mutex),
	}
	if initialCWD != "" {
		if absolute, err := filepath.Abs(initialCWD); err == nil {
			state.cwd = filepath.Clean(absolute)
		}
	}
	return state
}

func cloneMediaGroupState(group MediaGroupState) MediaGroupState {
	cloned := group
	cloned.ItemStatus = make(map[int]string, len(group.ItemStatus))
	for index, status := range group.ItemStatus {
		cloned.ItemStatus[index] = status
	}
	return cloned
}

func (s *ToolSessionState) RecordMediaGroup(group MediaGroupState) {
	if s == nil || group.ID == "" || group.Kind == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mediaGroups == nil {
		s.mediaGroups = make(map[string]MediaGroupState)
	}
	if current, ok := s.mediaGroups[group.ID]; ok {
		if group.RequestedCount < current.RequestedCount {
			group.RequestedCount = current.RequestedCount
		}
		merged := cloneMediaGroupState(current)
		for index, status := range group.ItemStatus {
			merged.ItemStatus[index] = status
		}
		merged.RequestedCount = group.RequestedCount
		merged.Kind = group.Kind
		s.mediaGroups[group.ID] = merged
		return
	}
	s.mediaGroups[group.ID] = cloneMediaGroupState(group)
}

func (s *ToolSessionState) MediaGroup(id string) (MediaGroupState, bool) {
	if s == nil || id == "" {
		return MediaGroupState{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	group, ok := s.mediaGroups[id]
	if !ok {
		return MediaGroupState{}, false
	}
	return cloneMediaGroupState(group), true
}

func (s *ToolSessionState) RecordReadProjection(projection ReadProjectionState) {
	if s == nil || projection.Snapshot.CanonicalPath == "" || projection.ProjectionKey == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readProjections == nil {
		s.readProjections = make(map[string]ReadProjectionState)
	}
	s.readProjections[filepath.Clean(projection.Snapshot.CanonicalPath)] = projection
}

func (s *ToolSessionState) ReadProjection(path string) (ReadProjectionState, bool) {
	if s == nil {
		return ReadProjectionState{}, false
	}
	canonical, err := canonicalSessionPath(path)
	if err != nil {
		return ReadProjectionState{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	projection, ok := s.readProjections[canonical]
	return projection, ok
}

func canonicalSessionPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func (s *ToolSessionState) RecordFile(snapshot FileReadState) {
	if s == nil || snapshot.CanonicalPath == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = make(map[string]FileReadState)
	}
	s.files[filepath.Clean(snapshot.CanonicalPath)] = snapshot
}

func (s *ToolSessionState) File(path string) (FileReadState, bool) {
	if s == nil {
		return FileReadState{}, false
	}
	canonical, err := canonicalSessionPath(path)
	if err != nil {
		return FileReadState{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.files[canonical]
	return snapshot, ok
}

func (s *ToolSessionState) CWD(fallback string) string {
	if s == nil {
		return fallback
	}
	s.mu.RLock()
	cwd := s.cwd
	s.mu.RUnlock()
	if cwd != "" {
		return cwd
	}
	return fallback
}

func (s *ToolSessionState) SetCWD(path string) error {
	if s == nil {
		return nil
	}
	canonical, err := canonicalSessionPath(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cwd = canonical
	s.mu.Unlock()
	return nil
}

// WithPathLock serializes compare-and-write for one canonical path without
// blocking edits to unrelated files.
func (s *ToolSessionState) WithPathLock(path string, fn func(string) error) error {
	canonical, err := canonicalSessionPath(path)
	if err != nil {
		return err
	}
	if s == nil {
		return fn(canonical)
	}
	s.pathMu.Lock()
	lock := s.pathLock[canonical]
	if lock == nil {
		lock = &sync.Mutex{}
		s.pathLock[canonical] = lock
	}
	s.pathMu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	return fn(canonical)
}

type toolSessionStateContextKey struct{}

func ContextWithToolSessionState(ctx context.Context, state *ToolSessionState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, toolSessionStateContextKey{}, state)
}

func ToolSessionStateFromContext(ctx context.Context) (*ToolSessionState, bool) {
	if ctx == nil {
		return nil, false
	}
	state, ok := ctx.Value(toolSessionStateContextKey{}).(*ToolSessionState)
	return state, ok && state != nil
}
