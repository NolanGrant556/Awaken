package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	"awaken/src/desktopdata"

	_ "modernc.org/sqlite"
)

type ThreadStore interface {
	CreateThread(snapshot ThreadSnapshot, modelProfileKey, preview string) error
	AppendRunInput(snapshot ThreadSnapshot, run Run, sessionID string, input string, attachments []RunInputAttachment) error
	AppendConversation(snapshot ThreadSnapshot, run Run, messages []convcontext.Message) error
	AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error
	ListThreads() ([]ThreadSnapshot, error)
	RestoreThread(threadID string) (storedThreadRecord, restoredTranscript, error)
	GetTranscript(threadID string) (ThreadTranscript, error)
	RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error)
	UpdateThreadTitleMetadata(threadID, label string, source ThreadTitleSource, status ThreadTitleStatus, seedRunID string, updatedAt time.Time) (ThreadSnapshot, error)
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

// Prepare opens and reconciles the metadata store before a run creates its
// append-only journal.
func (s *sqliteThreadStore) Prepare() error {
	_, err := s.open()
	return err
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
	path := s.paths.ThreadTranscriptPath(run.ThreadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if schema == 1 {
		if err := appendUserTranscriptRecordAtPath(path, run, sessionID, input, attachments); err != nil {
			return err
		}
	}
	// v2: durable user_message is written by Agent/SemanticRecorder before the first LLM call.
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	preview := strings.TrimSpace(input)
	if preview == "" {
		preview = snapshot.Label
	}
	return s.upsertThread(next, "", preview)
}

func (s *sqliteThreadStore) AppendConversation(snapshot ThreadSnapshot, run Run, messages []convcontext.Message) error {
	path := s.paths.ThreadTranscriptPath(run.ThreadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if schema == agentcore.SemanticSchemaVersion {
		// v2 journal already stores conversation facts via SemanticRecorder.
		next := snapshot
		next.RunID = run.RunID
		next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
		return s.upsertThread(next, "", "")
	}
	if err := appendConversationTranscriptAtPath(path, run, messages); err != nil {
		return err
	}
	next := snapshot
	next.RunID = run.RunID
	next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
	return s.upsertThread(next, "", "")
}

func (s *sqliteThreadStore) AppendRunEvents(snapshot ThreadSnapshot, run Run, events []Event) error {
	path := s.paths.ThreadTranscriptPath(run.ThreadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if schema == agentcore.SemanticSchemaVersion {
		next := snapshot
		next.RunID = run.RunID
		next.UpdatedAt = maxTime(next.UpdatedAt, run.UpdatedAt, run.CreatedAt)
		return s.upsertThread(next, "", "")
	}
	if err := appendRunEventTranscriptAtPath(path, run, events); err != nil {
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
SELECT thread_id, title, title_source, title_status, title_updated_at, title_seed_run_id, workspace_root, project_id, scope_id, latest_run_id, created_at, updated_at, workspace_json, model_profile_key, preview
FROM threads
WHERE deleted = 0
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
	path := s.paths.ThreadTranscriptPath(threadID)
	schema, err := DetectJournalSchema(path)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrThreadNotFound) || errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return storedThreadRecord{}, restoredTranscript{}, ErrThreadNotFound
		}
		return storedThreadRecord{}, restoredTranscript{}, err
	}
	var restored restoredTranscript
	if schema == agentcore.SemanticSchemaVersion {
		records, loadErr := loadAndRecoverSemanticRecords(path, threadID)
		if loadErr != nil {
			return storedThreadRecord{}, restoredTranscript{}, loadErr
		}
		restored = restoredTranscriptFromSemantic(records)
	} else {
		restored, err = loadRestoredTranscriptAtPath(path)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrThreadNotFound) || errors.Is(err, os.ErrNotExist) {
				return storedThreadRecord{}, restoredTranscript{}, ErrThreadNotFound
			}
			return storedThreadRecord{}, restoredTranscript{}, err
		}
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
	path := s.paths.ThreadTranscriptPath(threadID)
	schema, err := DetectJournalSchema(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}

	if schema == agentcore.SemanticSchemaVersion {
		records, loadErr := loadAndRecoverSemanticRecords(path, threadID)
		if loadErr != nil {
			return ThreadTranscript{}, loadErr
		}
		restored := restoredTranscriptFromSemantic(records)
		runs := durableRunsFromSemanticRecords(threadID, records)
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
		ordered := make([]ThreadTranscriptRun, 0, len(restored.RunIDs))
		for _, runID := range restored.RunIDs {
			if durable, ok := runs[runID]; ok {
				ordered = append(ordered, durable)
			}
		}
		return ThreadTranscript{Thread: snapshot, Runs: ordered}, nil
	}

	restored, err := loadRestoredTranscriptAtPath(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ThreadTranscript{}, ErrThreadNotFound
		}
		return ThreadTranscript{}, err
	}
	// Project v1 message-layer tool facts into semantic events for replay.
	semanticRecords, _ := (LegacyTranscriptAdapter{}).ReadFile(path)
	runs := durableRunsFromSemanticRecords(threadID, semanticRecords)
	if len(runs) == 0 {
		durableRuns, derr := loadDurableTranscriptRunsAtPath(path, threadID)
		if derr != nil {
			if errors.Is(derr, os.ErrNotExist) {
				return ThreadTranscript{}, ErrThreadNotFound
			}
			return ThreadTranscript{}, derr
		}
		runs = durableRuns
	}
	ordered := make([]ThreadTranscriptRun, 0, len(restored.RunIDs))
	for _, runID := range restored.RunIDs {
		if durable, ok := runs[runID]; ok {
			ordered = append(ordered, durable)
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
		Runs:   ordered,
	}, nil
}

func loadAndRecoverSemanticRecords(path, threadID string) ([]agentcore.SemanticRecord, error) {
	records, _, err := loadSemanticRecordsForThread(path)
	if err != nil {
		return nil, err
	}
	journal, err := NewExecutionJournal(path, threadID)
	if err != nil {
		return nil, err
	}
	recovered, recoverErr := recoverOrphanRuns(context.Background(), journal, records)
	closeErr := journal.Close()
	if recoverErr != nil {
		return nil, recoverErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return recovered, nil
}

func (s *sqliteThreadStore) RenameThread(threadID, label string, updatedAt time.Time) (ThreadSnapshot, error) {
	record, err := s.threadRecord(threadID)
	if err != nil {
		return ThreadSnapshot{}, err
	}
	record.Snapshot.Label = trimThreadLabel(strings.TrimSpace(label))
	record.Snapshot.TitleSource = ThreadTitleSourceManual
	record.Snapshot.TitleStatus = ThreadTitleStatusReady
	record.Snapshot.TitleUpdatedAt = updatedAt.UTC()
	record.Snapshot.UpdatedAt = updatedAt.UTC()
	if record.Snapshot.Label == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}
	path := s.paths.ThreadTranscriptPath(threadID)
	schema, err := DetectJournalSchema(path)
	if err != nil && !os.IsNotExist(err) {
		return ThreadSnapshot{}, err
	}
	if schema == 1 {
		if err := appendThreadLabelTranscriptRecordAtPath(path, threadID, record.Snapshot.Label, updatedAt); err != nil {
			return ThreadSnapshot{}, err
		}
	}
	// v2 label updates are reflected in SQLite metadata; journal thread records remain append-only.
	if err := s.upsertThread(record.Snapshot, record.ModelProfileKey, ""); err != nil {
		return ThreadSnapshot{}, err
	}
	return record.Snapshot, nil
}

func (s *sqliteThreadStore) UpdateThreadTitleMetadata(threadID, label string, source ThreadTitleSource, status ThreadTitleStatus, seedRunID string, updatedAt time.Time) (ThreadSnapshot, error) {
	threadID = strings.TrimSpace(threadID)
	seedRunID = strings.TrimSpace(seedRunID)
	if threadID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	if seedRunID == "" {
		return ThreadSnapshot{}, ErrThreadNotFound
	}
	nextLabel := trimThreadLabel(strings.TrimSpace(label))
	if status == ThreadTitleStatusReady && nextLabel == "" {
		return ThreadSnapshot{}, ErrThreadLabelInvalid
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	} else {
		updatedAt = updatedAt.UTC()
	}

	db, err := s.open()
	if err != nil {
		return ThreadSnapshot{}, err
	}
	result, err := db.Exec(`
UPDATE threads
SET
	title = CASE WHEN ? = '' THEN title ELSE ? END,
	title_source = ?,
	title_status = ?,
	title_updated_at = ?,
	title_seed_run_id = ?,
	updated_at = ?
WHERE thread_id = ?
	AND title_source = ?
	AND title_status = ?
	AND title_seed_run_id = ?
`,
		nextLabel,
		nextLabel,
		string(source),
		string(status),
		updatedAt.Format(time.RFC3339Nano),
		seedRunID,
		updatedAt.Format(time.RFC3339Nano),
		threadID,
		string(ThreadTitleSourceFirstUser),
		string(ThreadTitleStatusPending),
		seedRunID,
	)
	if err != nil {
		return ThreadSnapshot{}, fmt.Errorf("update thread title metadata %q: %w", threadID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return ThreadSnapshot{}, fmt.Errorf("update thread title metadata %q rows affected: %w", threadID, err)
	}
	if rowsAffected == 0 {
		record, err := s.threadRecord(threadID)
		if err != nil {
			return ThreadSnapshot{}, err
		}
		return record.Snapshot, nil
	}
	record, err := s.threadRecord(threadID)
	if err != nil {
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
SELECT thread_id, title, title_source, title_status, title_updated_at, title_seed_run_id, workspace_root, project_id, scope_id, latest_run_id, created_at, updated_at, workspace_json, model_profile_key, preview
FROM threads
WHERE thread_id = ? AND deleted = 0
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
	projectID, err := ensureProjectForWorkspace(db, snapshot.Workspace.WorkspaceRoot, false)
	if err != nil {
		return err
	}
	title := trimThreadLabel(firstNonEmpty(snapshot.Label, snapshot.ThreadID))
	_, err = db.Exec(`
INSERT INTO threads (
	thread_id, title, title_source, title_status, title_updated_at, title_seed_run_id,
	workspace_root, project_id, scope_id, latest_run_id,
	created_at, updated_at, archived, deleted, preview, model_profile_key, workspace_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)
ON CONFLICT(thread_id) DO UPDATE SET
	title = CASE
		WHEN excluded.title_source = '' THEN threads.title
		WHEN excluded.title_source = 'first_user'
			AND excluded.title_status = 'pending'
			AND threads.title_source <> ''
			AND NOT (threads.title_source = 'first_user' AND threads.title_status = 'pending') THEN threads.title
		ELSE excluded.title
	END,
	title_source = CASE
		WHEN excluded.title_source = '' THEN threads.title_source
		WHEN excluded.title_source = 'first_user'
			AND excluded.title_status = 'pending'
			AND threads.title_source <> ''
			AND NOT (threads.title_source = 'first_user' AND threads.title_status = 'pending') THEN threads.title_source
		ELSE excluded.title_source
	END,
	title_status = CASE
		WHEN excluded.title_status = '' THEN threads.title_status
		WHEN excluded.title_source = 'first_user'
			AND excluded.title_status = 'pending'
			AND threads.title_source <> ''
			AND NOT (threads.title_source = 'first_user' AND threads.title_status = 'pending') THEN threads.title_status
		ELSE excluded.title_status
	END,
	title_updated_at = CASE
		WHEN excluded.title_updated_at = '' THEN threads.title_updated_at
		WHEN excluded.title_source = 'first_user'
			AND excluded.title_status = 'pending'
			AND threads.title_source <> ''
			AND NOT (threads.title_source = 'first_user' AND threads.title_status = 'pending') THEN threads.title_updated_at
		ELSE excluded.title_updated_at
	END,
	title_seed_run_id = CASE
		WHEN excluded.title_seed_run_id = '' THEN threads.title_seed_run_id
		WHEN excluded.title_source = 'first_user'
			AND excluded.title_status = 'pending'
			AND threads.title_source <> ''
			AND NOT (threads.title_source = 'first_user' AND threads.title_status = 'pending') THEN threads.title_seed_run_id
		ELSE excluded.title_seed_run_id
	END,
	workspace_root = excluded.workspace_root,
	project_id = excluded.project_id,
	scope_id = excluded.scope_id,
	latest_run_id = CASE WHEN excluded.latest_run_id = '' THEN threads.latest_run_id ELSE excluded.latest_run_id END,
	created_at = CASE WHEN threads.created_at = '' THEN excluded.created_at ELSE threads.created_at END,
	updated_at = excluded.updated_at,
	preview = CASE WHEN excluded.preview = '' THEN threads.preview ELSE excluded.preview END,
	model_profile_key = CASE WHEN excluded.model_profile_key = '' THEN threads.model_profile_key ELSE excluded.model_profile_key END,
	workspace_json = excluded.workspace_json
`,
		snapshot.ThreadID,
		title,
		string(snapshot.TitleSource),
		string(snapshot.TitleStatus),
		formatStoredTime(snapshot.TitleUpdatedAt),
		strings.TrimSpace(snapshot.TitleSeedRunID),
		snapshot.Workspace.WorkspaceRoot,
		projectID,
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
	dsn := &url.URL{Scheme: "file", Path: sqliteFilePath(s.paths.ThreadsDBPath())}
	query := dsn.Query()
	// modernc SQLite's _txlock setting makes every database/sql Begin use
	// BEGIN IMMEDIATE, so lifecycle mutations acquire the writer lock before
	// they inspect state and cannot race a concurrent run start.
	query.Set("_txlock", "immediate")
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open threads db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure threads db %q: %w", pragma, err)
		}
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS threads (
	thread_id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	title_source TEXT NOT NULL DEFAULT '',
	title_status TEXT NOT NULL DEFAULT '',
	title_updated_at TEXT NOT NULL DEFAULT '',
	title_seed_run_id TEXT NOT NULL DEFAULT '',
	workspace_root TEXT NOT NULL,
	project_id TEXT NOT NULL DEFAULT '',
	scope_id TEXT NOT NULL,
	latest_run_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	archived INTEGER NOT NULL DEFAULT 0,
	deleted INTEGER NOT NULL DEFAULT 0,
	deleted_at TEXT NOT NULL DEFAULT '',
	delete_operation_id TEXT NOT NULL DEFAULT '',
	preview TEXT NOT NULL DEFAULT '',
	model_profile_key TEXT NOT NULL DEFAULT '',
	workspace_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_threads_updated_at ON threads(updated_at DESC, created_at DESC);
`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init threads db: %w", err)
	}
	if err := ensureThreadStoreColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureProjectLifecycleSchema(db, s.paths); err != nil {
		_ = db.Close()
		return nil, err
	}
	s.db = db
	return s.db, nil
}

type requiredThreadStoreColumn struct {
	name string
	ddl  string
}

var requiredThreadStoreColumns = []requiredThreadStoreColumn{
	{name: "title_source", ddl: "title_source TEXT NOT NULL DEFAULT ''"},
	{name: "title_status", ddl: "title_status TEXT NOT NULL DEFAULT ''"},
	{name: "title_updated_at", ddl: "title_updated_at TEXT NOT NULL DEFAULT ''"},
	{name: "title_seed_run_id", ddl: "title_seed_run_id TEXT NOT NULL DEFAULT ''"},
	{name: "latest_run_id", ddl: "latest_run_id TEXT NOT NULL DEFAULT ''"},
	{name: "project_id", ddl: "project_id TEXT NOT NULL DEFAULT ''"},
	{name: "archived", ddl: "archived INTEGER NOT NULL DEFAULT 0"},
	{name: "deleted", ddl: "deleted INTEGER NOT NULL DEFAULT 0"},
	{name: "deleted_at", ddl: "deleted_at TEXT NOT NULL DEFAULT ''"},
	{name: "delete_operation_id", ddl: "delete_operation_id TEXT NOT NULL DEFAULT ''"},
	{name: "preview", ddl: "preview TEXT NOT NULL DEFAULT ''"},
	{name: "model_profile_key", ddl: "model_profile_key TEXT NOT NULL DEFAULT ''"},
	{name: "workspace_json", ddl: "workspace_json TEXT NOT NULL DEFAULT '{}'"},
}

func ensureThreadStoreColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(threads)`)
	if err != nil {
		return fmt.Errorf("inspect threads schema: %w", err)
	}
	defer rows.Close()

	existing := map[string]struct{}{}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return fmt.Errorf("scan threads schema: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate threads schema: %w", err)
	}

	for _, column := range requiredThreadStoreColumns {
		if _, ok := existing[column.name]; ok {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE threads ADD COLUMN ` + column.ddl); err != nil {
			return fmt.Errorf("migrate threads schema add %s: %w", column.name, err)
		}
	}
	return nil
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
		threadID       string
		title          string
		titleSource    string
		titleStatus    string
		titleUpdatedAt string
		titleSeedRunID string
		workspaceRoot  string
		projectID      string
		scopeID        string
		latestRunID    string
		createdAtRaw   string
		updatedAtRaw   string
		workspaceJSON  string
		modelProfile   string
		preview        string
	)
	if err := scanner.Scan(
		&threadID,
		&title,
		&titleSource,
		&titleStatus,
		&titleUpdatedAt,
		&titleSeedRunID,
		&workspaceRoot,
		&projectID,
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
	source := ThreadTitleSource(strings.TrimSpace(titleSource))
	status := ThreadTitleStatus(strings.TrimSpace(titleStatus))
	// Existing threads created before the title state machine default to
	// manual+ready so historical labels are not overwritten by later
	// automatic title generation.
	if source == "" && status == "" && strings.TrimSpace(title) != "" {
		source = ThreadTitleSourceManual
		status = ThreadTitleStatusReady
	}
	return storedThreadRecord{
		Snapshot: ThreadSnapshot{
			ThreadID:       threadID,
			ProjectID:      projectID,
			Label:          trimThreadLabel(title),
			TitleSource:    source,
			TitleStatus:    status,
			TitleUpdatedAt: parseStoredTime(titleUpdatedAt),
			TitleSeedRunID: strings.TrimSpace(titleSeedRunID),
			RunID:          latestRunID,
			Workspace:      workspace,
			CreatedAt:      parseStoredTime(createdAtRaw),
			UpdatedAt:      parseStoredTime(updatedAtRaw),
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

// sqliteFilePath converts a native filesystem path into a slash-separated,
// absolute path suitable for a SQLite URI. On Windows this turns a drive
// letter like "C:\foo" into "/C:/foo" so that url.URL produces a valid
// file:// URI instead of treating the drive letter as an authority.
func sqliteFilePath(native string) string {
	absolute, err := filepath.Abs(native)
	if err != nil {
		absolute = native
	}
	slashed := filepath.ToSlash(absolute)
	if runtime.GOOS == "windows" && len(slashed) >= 2 && slashed[1] == ':' {
		slashed = "/" + slashed
	}
	return slashed
}

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

func formatStoredTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
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
