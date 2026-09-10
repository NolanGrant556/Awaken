package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"awaken/src/agentcore"
	"awaken/src/filestore"
)

var (
	ErrJournalMiddleCorruption = errors.New("journal middle corruption")
	ErrJournalEncoding         = errors.New("journal encoding error")
	ErrJournalExternalChange   = errors.New("journal changed outside its active writer; reopen before writing")
)

const (
	journalRetryDelay1 = 50 * time.Millisecond
	journalRetryDelay2 = 200 * time.Millisecond
	journalMaxAttempts = 3
)

// ExecutionJournal is the per-thread SemanticRecorder implementation.
type ExecutionJournal struct {
	mu         sync.Mutex
	index      []journalIndexEntry
	indexInfo  os.FileInfo
	writerInfo os.FileInfo
	path       string
	threadID   string
	file       *os.File
	nextSeq    uint64
	pending    []agentcore.SemanticRecord
	seenIDs    map[string]struct{}
	closed     bool
	// syncHook is a test-only fault injection point. Production leaves it nil
	// and uses os.File.Sync directly.
	syncHook        func() error
	atomicWriteHook func(path string, data []byte) error
}

func NewExecutionJournal(path, threadID string) (*ExecutionJournal, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("journal path is required")
	}
	if strings.TrimSpace(threadID) == "" {
		return nil, fmt.Errorf("thread id is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create journal directory: %w", err)
	}

	j := &ExecutionJournal{
		path:     path,
		threadID: threadID,
		nextSeq:  1,
		seenIDs:  map[string]struct{}{},
	}
	if err := j.openAndScan(); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *ExecutionJournal) Path() string {
	return j.path
}

func (j *ExecutionJournal) ThreadID() string {
	return j.threadID
}

func (j *ExecutionJournal) LastAssignedSequence() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.nextSeq == 0 {
		return 0
	}
	return j.nextSeq - 1
}

func (j *ExecutionJournal) SemanticRecords(fromSeq, toSeq uint64) ([]agentcore.SemanticRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if fromSeq == 0 || toSeq < fromSeq {
		return nil, fmt.Errorf("invalid semantic record range %d..%d", fromSeq, toSeq)
	}
	return j.indexedRecords(fromSeq, toSeq, nil)
}

func (j *ExecutionJournal) Append(ctx context.Context, record agentcore.SemanticRecord) error {
	_, err := j.appendBatchAssigned(ctx, []agentcore.SemanticRecord{record})
	return err
}

func (j *ExecutionJournal) AppendBatch(ctx context.Context, records []agentcore.SemanticRecord) error {
	_, err := j.appendBatchAssigned(ctx, records)
	return err
}

// AppendBatchAssigned appends a batch and returns the exact durable records,
// including their journal-assigned sequence values.
func (j *ExecutionJournal) AppendBatchAssigned(ctx context.Context, records []agentcore.SemanticRecord) ([]agentcore.SemanticRecord, error) {
	return j.appendBatchAssigned(ctx, records)
}

// AppendAssigned appends one record and returns the exact durable record,
// including the seq assigned while holding the journal lock. Startup orphan
// recovery uses this instead of returning the caller's pre-append seq=0 copy.
func (j *ExecutionJournal) AppendAssigned(ctx context.Context, record agentcore.SemanticRecord) (agentcore.SemanticRecord, error) {
	prepared, err := j.appendBatchAssigned(ctx, []agentcore.SemanticRecord{record})
	if err != nil {
		return agentcore.SemanticRecord{}, err
	}
	if len(prepared) == 0 {
		return agentcore.SemanticRecord{}, fmt.Errorf("%w: record %q was not appended", ErrJournalEncoding, record.RecordID)
	}
	return prepared[0], nil
}

// AppendFlushed assigns and durably replaces the complete journal with one
// additional safety-sensitive record. A same-directory temp + fsync + rename
// means failure before commit leaves the old complete file intact; failure
// reported after rename is recognized as a committed success by comparing the
// target with the exact transaction bytes. No failed boundary can later be
// resurrected by a flush or restart.
func (j *ExecutionJournal) AppendFlushed(ctx context.Context, record agentcore.SemanticRecord) (agentcore.SemanticRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return agentcore.SemanticRecord{}, fmt.Errorf("journal closed")
	}
	if err := j.ensureWriterAuthority(); err != nil {
		return agentcore.SemanticRecord{}, err
	}
	if err := j.writePendingWithRetry(ctx, true); err != nil {
		return agentcore.SemanticRecord{}, err
	}
	prepared, err := j.prepareRecords([]agentcore.SemanticRecord{record})
	if err != nil {
		return agentcore.SemanticRecord{}, err
	}
	if len(prepared) != 1 {
		return agentcore.SemanticRecord{}, fmt.Errorf("%w: record %q was not prepared", ErrJournalEncoding, record.RecordID)
	}
	if err := j.ensureIndex(); err != nil {
		return agentcore.SemanticRecord{}, err
	}
	previous, err := os.ReadFile(j.path)
	if err != nil {
		return agentcore.SemanticRecord{}, err
	}
	added, err := encodeExecutionJournal(prepared)
	if err != nil {
		return agentcore.SemanticRecord{}, err
	}
	data := append(previous, added...)

	if j.file != nil {
		if err := j.file.Close(); err != nil {
			return agentcore.SemanticRecord{}, err
		}
		j.file = nil
	}
	writeErr := j.writeJournalAtomically(data)
	if writeErr != nil {
		committed, readErr := os.ReadFile(j.path)
		if readErr != nil || !bytes.Equal(committed, data) {
			_ = j.reopenForAppend()
			return agentcore.SemanticRecord{}, writeErr
		}
	}
	j.addIndexedBytes(added, int64(len(previous)))
	j.writerInfo = j.indexInfo
	j.seenIDs[prepared[0].RecordID] = struct{}{}
	if prepared[0].Seq+1 > j.nextSeq {
		j.nextSeq = prepared[0].Seq + 1
	}
	// The transaction is already committed. Reopening is an operational
	// optimization; a later append will retry it if this attempt fails.
	_ = j.reopenForAppend()
	return prepared[0], nil
}

func (j *ExecutionJournal) writeJournalAtomically(data []byte) error {
	if j.atomicWriteHook != nil {
		return j.atomicWriteHook(j.path, data)
	}
	return filestore.WriteFileAtomic(j.path, data)
}

func encodeExecutionJournal(records []agentcore.SemanticRecord) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrJournalEncoding, err)
		}
	}
	return buf.Bytes(), nil
}

func (j *ExecutionJournal) appendBatchAssigned(ctx context.Context, records []agentcore.SemanticRecord) ([]agentcore.SemanticRecord, error) {
	if len(records) == 0 {
		return nil, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, fmt.Errorf("journal closed")
	}
	if err := j.ensureWriterAuthority(); err != nil {
		return nil, err
	}
	prepared, err := j.prepareRecords(records)
	if err != nil {
		return nil, err
	}
	if len(prepared) == 0 {
		return nil, nil
	}
	// Invalid payloads must not reserve sequence numbers or poison dedup state.
	if _, err := encodeExecutionJournal(prepared); err != nil {
		return nil, err
	}

	j.pending = append(j.pending, prepared...)
	for _, record := range prepared {
		j.seenIDs[record.RecordID] = struct{}{}
		if record.Seq+1 > j.nextSeq {
			j.nextSeq = record.Seq + 1
		}
	}
	if err := j.writePendingWithRetry(ctx, false); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (j *ExecutionJournal) Flush(ctx context.Context) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return fmt.Errorf("journal closed")
	}
	return j.writePendingWithRetry(ctx, true)
}

func (j *ExecutionJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	return err
}

func (j *ExecutionJournal) PendingCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.pending)
}

func (j *ExecutionJournal) NextSeq() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.nextSeq
}

// SnapshotRecords returns a consistent read of the journal while preventing a
// concurrent append from exposing a partial JSONL line to active transcript
// readers.
func (j *ExecutionJournal) SnapshotRecords() ([]agentcore.SemanticRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, fmt.Errorf("journal closed")
	}
	return ReadExecutionJournal(j.path)
}

func (j *ExecutionJournal) prepareRecords(records []agentcore.SemanticRecord) ([]agentcore.SemanticRecord, error) {
	out := make([]agentcore.SemanticRecord, 0, len(records))
	nextSeq := j.nextSeq
	// Include already-pending IDs so a retry of the same batch is skipped.
	seen := make(map[string]struct{}, len(records)+len(j.pending))
	for _, pending := range j.pending {
		seen[pending.RecordID] = struct{}{}
		if pending.Seq+1 > nextSeq {
			nextSeq = pending.Seq + 1
		}
	}
	for _, record := range records {
		if record.SchemaVersion == 0 {
			record.SchemaVersion = agentcore.SemanticSchemaVersion
		}
		if record.SchemaVersion != agentcore.SemanticSchemaVersion {
			return nil, fmt.Errorf("%w: unsupported schema_version %d", ErrJournalEncoding, record.SchemaVersion)
		}
		if strings.TrimSpace(string(record.Kind)) == "" {
			return nil, fmt.Errorf("%w: kind is required", ErrJournalEncoding)
		}
		if record.ThreadID == "" {
			record.ThreadID = j.threadID
		}
		if record.ThreadID != j.threadID {
			return nil, fmt.Errorf("%w: thread_id mismatch", ErrJournalEncoding)
		}
		if record.Timestamp.IsZero() {
			record.Timestamp = time.Now().UTC()
		} else {
			record.Timestamp = record.Timestamp.UTC()
		}
		if record.RecordID == "" {
			record.RecordID = agentcore.NewSemanticRecordID(string(record.Kind))
		}
		if _, already := j.seenIDs[record.RecordID]; already {
			continue
		}
		if _, already := seen[record.RecordID]; already {
			continue
		}
		if record.Seq == 0 {
			record.Seq = nextSeq
		} else if record.Seq < nextSeq {
			return nil, fmt.Errorf("%w: seq %d is behind next %d", ErrJournalEncoding, record.Seq, nextSeq)
		}
		if record.Payload == nil {
			record.Payload = map[string]any{}
		}
		out = append(out, record)
		seen[record.RecordID] = struct{}{}
		nextSeq = record.Seq + 1
	}
	return out, nil
}

func (j *ExecutionJournal) writePendingWithRetry(ctx context.Context, syncFile bool) error {
	if len(j.pending) == 0 {
		if syncFile {
			return j.syncFile(ctx)
		}
		return nil
	}

	var lastErr error
	written := false
	for attempt := 1; attempt <= journalMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !written {
			if err := j.appendPendingOnce(); err != nil {
				lastErr = err
				if !isRetryableJournalError(err) {
					return err
				}
			} else {
				written = true
				j.pending = j.pending[:0]
				if !syncFile {
					return nil
				}
			}
		}
		if written && syncFile {
			if err := j.syncCurrentFile(); err != nil {
				lastErr = err
				if !isRetryableJournalError(err) {
					return err
				}
			} else {
				return nil
			}
		}
		if attempt == journalMaxAttempts {
			break
		}
		delay := journalRetryDelay1
		if attempt >= 2 {
			delay = journalRetryDelay2
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (j *ExecutionJournal) appendPendingOnce() error {
	if err := j.ensureWriterAuthority(); err != nil {
		return err
	}
	if j.file == nil {
		if err := j.reopenForAppend(); err != nil {
			return err
		}
	}
	if err := j.ensureIndex(); err != nil {
		return err
	}
	offset := j.indexInfo.Size()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, record := range j.pending {
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("%w: %v", ErrJournalEncoding, err)
		}
	}
	if n, err := j.file.Write(buf.Bytes()); err != nil || n != buf.Len() {
		if n > 0 {
			if rollbackErr := j.file.Truncate(offset); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			_, _ = j.file.Seek(0, io.SeekEnd)
			j.writerInfo, _ = os.Stat(j.path)
		}
		if err != nil {
			return err
		}
		return io.ErrShortWrite
	}
	j.addIndexedBytes(buf.Bytes(), offset)
	j.writerInfo = j.indexInfo
	return nil
}

func (j *ExecutionJournal) syncFile(ctx context.Context) error {
	if j.file == nil {
		return nil
	}
	var lastErr error
	for attempt := 1; attempt <= journalMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := j.syncCurrentFile(); err == nil {
			return nil
		} else {
			lastErr = err
			if !isRetryableJournalError(err) {
				return err
			}
		}
		if attempt == journalMaxAttempts {
			break
		}
		delay := journalRetryDelay1
		if attempt >= 2 {
			delay = journalRetryDelay2
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (j *ExecutionJournal) syncCurrentFile() error {
	if j.syncHook != nil {
		return j.syncHook()
	}
	return j.file.Sync()
}

var _ agentcore.FlushedSemanticRecorder = (*ExecutionJournal)(nil)
var _ agentcore.SemanticRecordRangeReader = (*ExecutionJournal)(nil)

func (j *ExecutionJournal) openAndScan() error {
	file, err := os.OpenFile(j.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	j.file = file

	records, nextSeq, err := readJournalRecordsFromReader(file, true)
	if err != nil {
		_ = file.Close()
		j.file = nil
		return err
	}
	for _, record := range records {
		j.seenIDs[record.RecordID] = struct{}{}
	}
	j.nextSeq = nextSeq
	data, err := os.ReadFile(j.path)
	if err != nil {
		return err
	}
	entries, err := indexJournalBytes(data, 0)
	if err != nil {
		return err
	}
	validEnd := int64(0)
	if len(entries) > 0 {
		validEnd = entries[len(entries)-1].end
	}
	if validEnd < int64(len(data)) && len(bytes.TrimSpace(data[validEnd:])) > 0 {
		if err := file.Truncate(validEnd); err != nil {
			return err
		}
		data = data[:validEnd]
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		if _, err := file.WriteAt([]byte{'\n'}, int64(len(data))); err != nil {
			return err
		}
	}
	j.writerInfo, err = os.Stat(j.path)
	if err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		j.file = nil
		return err
	}
	return nil
}

func (j *ExecutionJournal) ensureWriterAuthority() error {
	current, err := os.Stat(j.path)
	if err != nil {
		return err
	}
	if j.writerInfo != nil && (!os.SameFile(j.writerInfo, current) || j.writerInfo.Size() != current.Size() || j.writerInfo.ModTime() != current.ModTime()) {
		return ErrJournalExternalChange
	}
	return nil
}

func (j *ExecutionJournal) reopenForAppend() error {
	file, err := os.OpenFile(j.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	j.file = file
	return nil
}

func ReadExecutionJournal(path string) ([]agentcore.SemanticRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	records, _, err := readJournalRecordsFromReader(file, true)
	return records, err
}

func readJournalRecordsFromReader(r io.Reader, tolerateIncompleteTail bool) ([]agentcore.SemanticRecord, uint64, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan journal: %w", err)
	}

	var (
		records []agentcore.SemanticRecord
		nextSeq uint64 = 1
	)
	for i, line := range lines {
		var record agentcore.SemanticRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			isLast := i == len(lines)-1
			if tolerateIncompleteTail && isLast {
				break
			}
			return nil, 0, fmt.Errorf("%w at line %d: %v", ErrJournalMiddleCorruption, i+1, err)
		}
		if record.SchemaVersion != 0 && record.SchemaVersion != agentcore.SemanticSchemaVersion {
			return nil, 0, fmt.Errorf("%w: unsupported schema_version %d at line %d", ErrJournalMiddleCorruption, record.SchemaVersion, i+1)
		}
		records = append(records, record)
		if record.Seq >= nextSeq {
			nextSeq = record.Seq + 1
		}
	}
	return records, nextSeq, nil
}

func isRetryableJournalError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrJournalEncoding) {
		return false
	}
	if errors.Is(err, syscall.ENOSPC) {
		return false
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if errors.Is(pathErr.Err, syscall.EROFS) || errors.Is(pathErr.Err, syscall.ENOSPC) {
			return false
		}
	}
	if errors.Is(err, syscall.EROFS) {
		return false
	}
	return true
}

// DetectJournalSchema peeks the first non-empty JSONL line to decide v1 vs v2.
func DetectJournalSchema(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var probe struct {
			SchemaVersion int    `json:"schema_version"`
			RecordType    string `json:"record_type"`
			Kind          string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrJournalMiddleCorruption, err)
		}
		if probe.SchemaVersion == agentcore.SemanticSchemaVersion || probe.Kind != "" {
			return agentcore.SemanticSchemaVersion, nil
		}
		if probe.RecordType != "" {
			return 1, nil
		}
		return 0, fmt.Errorf("%w: unrecognized journal line", ErrJournalMiddleCorruption)
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	// Empty file: treat as v2 destination for new writes.
	return agentcore.SemanticSchemaVersion, nil
}
