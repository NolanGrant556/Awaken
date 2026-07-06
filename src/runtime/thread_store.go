package runtime

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awaken/src/context"
	"awaken/src/desktopdata"

	_ "modernc.org/sqlite"
)

type ThreadStore interface {
	CreateThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error
	AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID string, input string, attachments []RunInputAttachment) error
	AppendConversation(snapshot ThreadSnapshot, run Run, messages []context.Message) error
	AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error
	ListThreads() ([]ThreadSnapshot, error)
	RestoreThread(threadID string) (storedThreadRecord, restoredTranscript, error)
	GetTranscript(threadID string) (ThreadTranscript, error)
	RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error)
	DeleteThread(threadID string) error
}

type storedThreadRecord struct {
	Snapshot        ThreadSnapshot
	ModelProfileKey string
	Preview         string
}

type sqliteThreadStore struct {
	paths desktopdata.Paths

	mu sync.Mutex
	db *sql.DB
}

func newThreadStore() ThreadStore {
	paths := desktopdata.Detect()
	if strings.TrimSpace(os.Getenv(runtimeUserDataDirEnv)) == "" {
		if dir, err := os.MkdirTemp("", "awaken-thread-store-"); err == nil {
			paths = desktopdata.FromRoot(dir)
		}
	}
	return &sqliteThreadStore{paths: paths}
}

func (s *sqliteThreadStore) CreateThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error {
	return s.upsertThread(snapshot, modelProfileKey, preview)
}

func (s *sqliteThreadStore) AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID string, input string, attachments []RunInputAttachment) error {
	if err := appendUserTranscriptRecordAtPath(s.paths.ThreadTranscriptPath(run.ThreadID), run, sessionID, input, attachments); err != nil {
		return err
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	preview := strings.TrimSpace(input)
	if preview == "" {
		preview = snapshot.Label
	}
	return s.upsertThread(next, "", preview)
}

func (s *sqliteThreadStore) AppendConversation(snapshot ThreadSnapshot, run Run, messages []context.Message) error {
	if err := appendConversationTranscriptAtPath(s.paths.ThreadTranscriptPath(run.ThreadID), run, messages); err != nil {
		return err
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	return s.upsertThread(next, "", "")
}

func (s *sqliteThreadStore) AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error {
	if err := appendRunEventTranscriptAtPath(s.paths.ThreadTranscriptPath(run.ThreadID), run, events); err != nil {
		return err
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	return s.upsertThread(next, "", "")
}

func (s *sqliteThreadStore) ListThreads() ([]ThreadSnapshot, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`
SELECT thread_id, title, workspace_root, scope_id, latest_run_id, created_at, updated_at, workspace_json, model_profile_key, preview
FROM threads
ORDER BY updated_at DESC, created_at DESC
`)
	if err != nil {
		return nil, fmt.Errorf("query threads: %w", err)
	}
	defer rows.Close()

	var out []ThreadSnapshot
	for rows.Next() {
		record, err := scanStoredThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record.Snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate threads: %w", err)
	}
	return out, nil
}

func (s *sqliteThreadStore) RestoreThread(threadID string) (storedThreadRecord, restoredTranscript, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return storedThreadRecord{}, restoredTranscript{}, err
	}
	restored, err := loadRestoredTranscriptAtPath(s.paths.ThreadTranscriptPath(threadID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrThreadNotFound) || errors.Is(err, os.ErrNotExist) {
			return storedThreadRecord{}, restoredTranscript{}, ErrThreadNotFound
		}
		return storedThreadRecord{}, restoredTranscript{}, err
	}
	if record.Snapshot.Label == "" {
		record.Snapshot.Label = restored.Label
	}
	if record.Snapshot.CreatedAt.IsZero() {
		record.Snapshot.CreatedAt = restored.CreatedAt
	}
	if record.Snapshot.UpdatedAt.IsZero() {
		record.Snapshot.UpdatedAt = restored.UpdatedAt
	}
	return record, restored, nil
}

func (s *sqliteThreadStore) GetTranscript(threadID string) (ThreadTranscript, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return ThreadTranscript{}, err
	}
	restored, err := loadRestoredTranscriptAtPath(s.paths.ThreadTranscriptPath(threadID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}
	durableRuns, err := loadDurableTranscriptRunsAtPath(s.paths.ThreadTranscriptPath(threadID), threadID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}
	runs := make([]ThreadTranscriptRun, 0, len(restored.RunIDs))
	for _, runID := range restored.RunIDs {
		if durable, ok := durableRuns[runID]; ok {
			runs = append(runs, durable)
		}
	}
	snapshot := record.Snapshot
	if snapshot.Label == "" {
		snapshot.Label = restored.Label
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = restored.CreatedAt
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = restored.UpdatedAt
	}
	if snapshot.RunID == "" && len(restored.RunIDs) > 0 {
		snapshot.RunID = restored.RunIDs[len(restored.RunIDs)-1]
	}
	return ThreadTranscript{
		Thread: snapshot,
		Runs:   runs,
	}, nil
}

func (s *sqliteThreadStore) RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return ThreadSnapshot{}, err
	}
	record.Snapshot.Label = trimThreadLabel(strings.TrimSpace(label))
	record.Snapshot.UpdatedAt = updatedAt.UTC()
	if record.Snapshot.Label == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}
	if err := appendThreadLabelTranscriptRecordAtPath(s.paths.ThreadTranscriptPath(threadID), threadID, record.Snapshot.Label, updatedAt); err != nil {
		return ThreadSnapshot{}, err
	}
	if err := s.upsertThread(record.Snapshot, record.ModelProfileKey, ""); err != nil {
		return ThreadSnapshot{}, err
	}
	return record.Snapshot, nil
}

func (s *sqliteThreadStore) DeleteThread(threadID string) error {
	if _, err := s.threadRecord(threadID); err != nil {
		return err
	}
	if err := s.deleteThreadRecord(threadID); err != nil {
		return err
	}
	if err := deleteThreadTranscriptAtPath(s.paths.ThreadTranscriptPath(threadID)); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	if err := deleteThreadInputAttachments(threadID); err != nil && !errors.Is(err, ErrThreadNotFound) {
		return err
	}
	return nil
}

func (s *sqliteThreadStore) threadRecord(threadID string) (storedThreadRecord, error) {
	db, err := s.open()
	if err != nil {
		return storedThreadRecord{}, err
	}
	row := db.QueryRow(`
SELECT thread_id, title, workspace_root, scope_id, latest_run_id, created_at, updated_at, workspace_json, model_profile_key, preview
FROM threads
WHERE thread_id = ?
`, threadID)
	record, err := scanStoredThread(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storedThreadRecord{}, ErrThreadNotFound
		}
		return storedThreadRecord{}, err
	}
	return record, nil
}

func (s *sqliteThreadStore) deleteThreadRecord(threadID string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	result, err := db.Exec(`DELETE FROM threads WHERE thread_id = ?`, threadID)
	if err != nil {
		return fmt.Errorf("delete thread %q: %w", threadID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete thread %q rows affected: %w", threadID, err)
	}
	if rowsAffected == 0 {
		return ErrThreadNotFound
	}
	return nil
}

func (s *sqliteThreadStore) upsertThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	workspaceJSON, err := marshalWorkspace(snapshot.Workspace)
	if err != nil {
		return err
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = snapshot.CreatedAt
	}
	_, err = db.Exec(`
INSERT INTO threads (
	thread_id, title, workspace_root, scope_id, latest_run_id,
	created_at, updated_at, archived, deleted, preview, model_profile_key, workspace_json
) VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)
ON CONFLICT(thread_id) DO UPDATE SET
	title = excluded.title,
	workspace_root = excluded.workspace_root,
	scope_id = excluded.scope_id,
	latest_run_id = CASE WHEN excluded.latest_run_id = '' THEN threads.latest_run_id ELSE excluded.latest_run_id END,
	created_at = CASE WHEN threads.created_at = '' THEN excluded.created_at ELSE threads.created_at END,
	updated_at = excluded.updated_at,
	preview = CASE WHEN excluded.preview = '' THEN threads.preview ELSE excluded.preview END,
	model_profile_key = CASE WHEN excluded.model_profile_key = '' THEN threads.model_profile_key ELSE excluded.model_profile_key END,
	workspace_json = excluded.workspace_json
`,
		snapshot.ThreadID,
		firstNonEmpty(snapshot.Label, snapshot.ThreadID),
		snapshot.Workspace.WorkspaceRoot,
		snapshot.Workspace.ScopeID,
		firstNonEmpty(snapshot.RunID, ""),
		snapshot.CreatedAt.UTC().Format(time.RFC3339Nano),
		snapshot.UpdatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(preview),
		strings.TrimSpace(modelProfileKey),
		workspaceJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert thread %q: %w", snapshot.ThreadID, err)
	}
	return nil
}

func (s *sqliteThreadStore) open() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	if err := s.paths.Ensure(); err != nil {
		return nil, fmt.Errorf("ensure desktop data dirs: %w", err)
	}
	db, err := sql.Open("sqlite", s.paths.ThreadsDBPath())
	if err != nil {
		return nil, fmt.Errorf("open threads db: %w", err)
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS threads (
	thread_id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	workspace_root TEXT NOT NULL,
	scope_id TEXT NOT NULL,
	latest_run_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	archived INTEGER NOT NULL DEFAULT 0,
	deleted INTEGER NOT NULL DEFAULT 0,
	preview TEXT NOT NULL DEFAULT '',
	model_profile_key TEXT NOT NULL DEFAULT '',
	workspace_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_threads_updated_at ON threads(updated_at DESC, created_at DESC);
`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init threads db: %w", err)
	}
	if _, err := db.Exec(`DELETE FROM threads WHERE deleted <> 0`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("purge deleted thread residue: %w", err)
	}
	if err := s.purgeOrphanThreadTranscripts(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("purge orphan thread transcripts: %w", err)
	}
	s.db = db
	return s.db, nil
}

func (s *sqliteThreadStore) purgeOrphanThreadTranscripts(db *sql.DB) error {
	rows, err := db.Query(`SELECT thread_id FROM threads`)
	if err != nil {
		return fmt.Errorf("query live thread ids: %w", err)
	}
	defer rows.Close()

	expectedFiles := map[string]struct{}{}
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			return fmt.Errorf("scan live thread id: %w", err)
		}
		expectedFiles[filepath.Base(s.paths.ThreadTranscriptPath(threadID))] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate live thread ids: %w", err)
	}

	entries, err := os.ReadDir(s.paths.ThreadsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "thread_") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		if _, ok := expectedFiles[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(s.paths.ThreadsDir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove orphan transcript %q: %w", name, err)
		}
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStoredThread(scanner rowScanner) (storedThreadRecord, error) {
	var (
		threadID      string
		title         string
		workspaceRoot string
		scopeID       string
		latestRunID   string
		createdAtRaw  string
		updatedAtRaw  string
		workspaceJSON string
		modelProfile  string
		preview       string
	)
	if err := scanner.Scan(
		&threadID,
		&title,
		&workspaceRoot,
		&scopeID,
		&latestRunID,
		&createdAtRaw,
		&updatedAtRaw,
		&workspaceJSON,
		&modelProfile,
		&preview,
	); err != nil {
		return storedThreadRecord{}, err
	}
	workspace, err := unmarshalWorkspace(workspaceJSON)
	if err != nil {
		return storedThreadRecord{}, err
	}
	if strings.TrimSpace(workspace.WorkspaceRoot) == "" {
		workspace.WorkspaceRoot = workspaceRoot
	}
	if strings.TrimSpace(workspace.ScopeID) == "" {
		workspace.ScopeID = scopeID
	}
	return storedThreadRecord{
		Snapshot: ThreadSnapshot{
			ThreadID:  threadID,
			Label:     title,
			RunID:     latestRunID,
			Workspace: workspace,
			CreatedAt: parseStoredTime(createdAtRaw),
			UpdatedAt: parseStoredTime(updatedAtRaw),
		},
		ModelProfileKey: modelProfile,
		Preview:         preview,
	}, nil
}

func marshalWorkspace(workspace WorkspaceConfig) (string, error) {
	data, err := jsonMarshalWorkspace(workspace)
	if err != nil {
		return "", fmt.Errorf("marshal workspace: %w", err)
	}
	return string(data), nil
}

func unmarshalWorkspace(raw string) (WorkspaceConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return WorkspaceConfig{}, nil
	}
	var workspace WorkspaceConfig
	if err := jsonUnmarshalWorkspace([]byte(raw), &workspace); err != nil {
		return WorkspaceConfig{}, fmt.Errorf("unmarshal workspace: %w", err)
	}
	return workspace, nil
}

var (
	jsonMarshalWorkspace   = json.Marshal
	jsonUnmarshalWorkspace = json.Unmarshal
)

func parseStoredTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func maxTime(values ...time.Time) time.Time {
	var latest time.Time
	for _, value := range values {
		if value.After(latest) {
			latest = value
		}
	}
	return latest
}
