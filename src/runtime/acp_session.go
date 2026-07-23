package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ACPThreadSource        = "acp_bridge"
	ACPMetadataClientName  = "acp_client_name"
	ACPMetadataClientVersion = "acp_client_version"
	ACPMetadataSessionID   = "acp_session_id"
	ACPMetadataEphemeral   = "awaken.ephemeral"
)

var (
	ErrACPSessionNotFound = errors.New("acp session not found")
	ErrACPSessionExists   = errors.New("acp session already exists")
	ErrACPSessionBusy     = errors.New("acp session has an active run")
	ErrInvalidCWD         = errors.New("invalid cwd")
)

// ACPSessionMapping tracks the Core-side state for one inbound ACP session.
type ACPSessionMapping struct {
	ACPSessionID  string
	ProjectID     string
	ThreadID      string
	CurrentRunID  string
	CanonicalCWD  string
	Visibility    string // "visible" or "ephemeral"
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ACPProjectService is the narrow project surface the ACP session manager
// needs. It allows tests to inject a fake without satisfying the full
// ProjectService/ProjectLifecycleRepository contract.
type ACPProjectService interface {
	UpsertProject(workspaceRoot, displayName string) (Project, error)
}

// ACPSessionManager owns inbound ACP session mappings. It lives in the Core
// process and is consulted by control routes and run lifecycle hooks.
type ACPSessionManager struct {
	runService      *RunService
	projectService  ACPProjectService
	mu              sync.RWMutex
	sessions        map[string]*ACPSessionMapping
	threadToSession map[string]string
}

// NewACPSessionManager creates a session manager. Both services may be nil in
// tests, but create/new/prompt will fail if they are not wired.
func NewACPSessionManager(runService *RunService, projectService ACPProjectService) *ACPSessionManager {
	return &ACPSessionManager{
		runService:      runService,
		projectService:  projectService,
		sessions:        make(map[string]*ACPSessionMapping),
		threadToSession: make(map[string]string),
	}
}

// CreateSession establishes a new ACP session mapped to a normal project/thread.
func (m *ACPSessionManager) CreateSession(ctx context.Context, req ACPSessionCreateRequest) (*ACPSessionMapping, error) {
	if req.ACPSessionID == "" {
		return nil, errors.New("acp_session_id is required")
	}
	cwd, err := CanonicalizeCWD(req.CWD)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCWD, err)
	}

	m.mu.Lock()
	if _, exists := m.sessions[req.ACPSessionID]; exists {
		m.mu.Unlock()
		return nil, ErrACPSessionExists
	}
	m.mu.Unlock()

	projectID, threadID, err := m.ensureProjectAndThread(ctx, cwd, req)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	mapping := &ACPSessionMapping{
		ACPSessionID: req.ACPSessionID,
		ProjectID:    projectID,
		ThreadID:     threadID,
		CanonicalCWD: cwd,
		Visibility:   req.Visibility,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	m.mu.Lock()
	m.sessions[req.ACPSessionID] = mapping
	m.threadToSession[threadID] = req.ACPSessionID
	m.mu.Unlock()

	return mapping, nil
}

// GetSession returns an existing session mapping.
func (m *ACPSessionManager) GetSession(acpSessionID string) (*ACPSessionMapping, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mapping, ok := m.sessions[acpSessionID]
	if !ok {
		return nil, ErrACPSessionNotFound
	}
	return mapping, nil
}

// CloseSession cancels the active run and removes the mapping. It does not
// delete the visible thread; ephemeral thread deletion is handled by the caller
// or cleanup worker.
func (m *ACPSessionManager) CloseSession(ctx context.Context, acpSessionID string) error {
	m.mu.Lock()
	mapping, ok := m.sessions[acpSessionID]
	if !ok {
		m.mu.Unlock()
		return ErrACPSessionNotFound
	}
	delete(m.sessions, acpSessionID)
	delete(m.threadToSession, mapping.ThreadID)
	m.mu.Unlock()

	if mapping.CurrentRunID != "" && m.runService != nil {
		_, _ = m.runService.CancelRun(mapping.CurrentRunID)
	}
	return nil
}

// SetCurrentRun records the active run for a session. The run must belong to
// the session's thread.
func (m *ACPSessionManager) SetCurrentRun(acpSessionID, runID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mapping, ok := m.sessions[acpSessionID]
	if !ok {
		return ErrACPSessionNotFound
	}
	mapping.CurrentRunID = runID
	mapping.UpdatedAt = time.Now().UTC()
	return nil
}

// ClearCurrentRun removes the active run association when it finishes.
func (m *ACPSessionManager) ClearCurrentRun(acpSessionID, runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mapping, ok := m.sessions[acpSessionID]
	if !ok {
		return
	}
	if mapping.CurrentRunID == runID {
		mapping.CurrentRunID = ""
		mapping.UpdatedAt = time.Now().UTC()
	}
}

func (m *ACPSessionManager) ensureProjectAndThread(ctx context.Context, cwd string, req ACPSessionCreateRequest) (string, string, error) {
	if m.projectService == nil {
		return "", "", errors.New("project service not available")
	}
	project, err := m.projectService.UpsertProject(cwd, projectDisplayName(cwd))
	if err != nil {
		return "", "", fmt.Errorf("upsert project: %w", err)
	}

	threadID := newRuntimeID("thread")
	// Threads are created lazily on first prompt via StartRun. We only reserve
	// the thread ID here and record metadata through the first run's thread
	// store write.
	_ = ctx
	_ = req
	return project.ProjectID, threadID, nil
}

// ACPSessionCreateRequest is the Core-side input for session/new.
type ACPSessionCreateRequest struct {
	ACPSessionID string `json:"acp_session_id"`
	CWD          string `json:"cwd"`
	Visibility   string `json:"visibility"`
	ClientName   string `json:"client_name"`
	ClientVersion string `json:"client_version"`
}

// CanonicalizeCWD validates and canonicalizes an inbound ACP cwd. It returns
// ErrInvalidCWD for empty, relative, non-existent, non-directory, or otherwise
// invalid paths.
func CanonicalizeCWD(cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", errors.New("cwd is empty")
	}
	if !filepath.IsAbs(cwd) {
		return "", errors.New("cwd must be absolute")
	}
	// Reject NUL and other illegal characters at the URI/path level.
	if strings.ContainsRune(cwd, '\x00') {
		return "", errors.New("cwd contains NUL")
	}
	if _, err := url.ParseRequestURI(filepath.ToSlash(cwd)); err != nil {
		return "", fmt.Errorf("cwd is not a valid path: %w", err)
	}

	cleaned := filepath.Clean(cwd)
	var err error
	if runtime.GOOS != "windows" {
		cleaned, err = filepath.EvalSymlinks(cleaned)
		if err != nil {
			return "", fmt.Errorf("cwd symlinks cannot be resolved: %w", err)
		}
	} else {
		// On Windows, EvalSymlinks may fail for non-existent path components;
		// existence is checked next.
		cleaned, _ = filepath.EvalSymlinks(cleaned)
	}

	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("cwd cannot be accessed: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("cwd is not a directory")
	}

	// On Windows, canonicalize to lowercase for stable comparison, matching the
	// Go project_store behavior.
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, nil
}

// StableCWDHash returns a stable digest of a canonical cwd. It is used when a
// deterministic short identifier is needed (e.g. ephemeral deletion queue keys).
func StableCWDHash(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:12])
}
