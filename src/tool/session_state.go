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

// ToolSessionState owns mutable thread-local tool state. Agent replacement,
// process restart and thread deletion therefore invalidate it safely.
type ToolSessionState struct {
	mu       sync.RWMutex
	files    map[string]FileReadState
	cwd      string
	pathMu   sync.Mutex
	pathLock map[string]*sync.Mutex
}

func NewToolSessionState(initialCWD string) *ToolSessionState {
	state := &ToolSessionState{
		files:    make(map[string]FileReadState),
		pathLock: make(map[string]*sync.Mutex),
	}
	if initialCWD != "" {
		if absolute, err := filepath.Abs(initialCWD); err == nil {
			state.cwd = filepath.Clean(absolute)
		}
	}
	return state
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
