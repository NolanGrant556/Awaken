package filestore

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// JournalRecord is one committed line of an append-only journal. Seq is
// assigned monotonically by the journal; Checksum covers seq + payload so a
// middle-of-file corruption is detected and fails stop instead of being
// silently skipped. Only an incomplete final line (crash tail) may be
// truncated back to the last committed offset.
type JournalRecord struct {
	Seq      uint64          `json:"seq"`
	Payload  json.RawMessage `json:"payload"`
	Checksum string          `json:"checksum"`
}

func journalChecksum(seq uint64, payload []byte) string {
	sum := sha256.Sum256(append([]byte(fmt.Sprintf("%d|", seq)), payload...))
	return hex.EncodeToString(sum[:8])
}

// Journal is a single-writer append-only JSONL file. Every append is flushed
// and fsynced before it is reported committed.
type Journal struct {
	path    string
	file    *os.File
	lastSeq uint64
}

// OpenJournal opens (or creates) the journal, verifies every committed
// record, truncates an incomplete crash tail back to the last committed
// offset, and fails stop on any mid-file corruption.
func OpenJournal(path string) (*Journal, RecoveryReport, error) {
	records, report, err := loadJournalRecords(path)
	if err != nil {
		return nil, report, err
	}
	if report.TruncatedTailBytes > 0 {
		if err := os.Truncate(path, report.CommittedBytes); err != nil {
			return nil, report, fmt.Errorf("filestore: truncate crash tail: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, FilePerm)
	if err != nil {
		return nil, report, fmt.Errorf("filestore: open journal: %w", err)
	}
	var lastSeq uint64
	if len(records) > 0 {
		lastSeq = records[len(records)-1].Seq
	}
	return &Journal{path: path, file: file, lastSeq: lastSeq}, report, nil
}

// RecoveryReport describes what journal recovery observed.
type RecoveryReport struct {
	CommittedRecords   int
	CommittedBytes     int64
	TruncatedTailBytes int64
}

func loadJournalRecords(path string) ([]JournalRecord, RecoveryReport, error) {
	report := RecoveryReport{}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, report, nil
	}
	if err != nil {
		return nil, report, fmt.Errorf("filestore: open journal for recovery: %w", err)
	}
	defer file.Close()

	var records []JournalRecord
	reader := bufio.NewReaderSize(file, 256*1024)
	var offset int64
	var expected uint64 = 1
	for {
		line, err := reader.ReadString('\n')
		complete := err == nil && strings.HasSuffix(line, "\n")
		if !complete {
			// Incomplete final line: recoverable crash tail. Anything else on
			// the read path is a hard failure.
			if len(line) > 0 {
				report.TruncatedTailBytes = int64(len(line))
			}
			break
		}
		var record JournalRecord
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return nil, report, fmt.Errorf("%w: blank line at offset %d in %s", ErrCorruptJournal, offset, path)
		}
		if unmarshalErr := json.Unmarshal([]byte(trimmed), &record); unmarshalErr != nil {
			// A corrupt record that is not the final line is a mid-file
			// corruption: fail stop. If it is the final line without newline
			// we never get here (handled above); a final complete-but-corrupt
			// line is still corruption because it was fsynced as committed.
			return nil, report, fmt.Errorf("%w: undecodable record at offset %d in %s: %v", ErrCorruptJournal, offset, path, unmarshalErr)
		}
		if record.Seq != expected {
			return nil, report, fmt.Errorf("%w: seq %d at offset %d, expected %d", ErrCorruptJournal, record.Seq, offset, expected)
		}
		if record.Checksum != journalChecksum(record.Seq, record.Payload) {
			return nil, report, fmt.Errorf("%w: checksum mismatch at seq %d", ErrCorruptJournal, record.Seq)
		}
		records = append(records, record)
		offset += int64(len(line))
		expected++
	}
	report.CommittedRecords = len(records)
	report.CommittedBytes = offset
	return records, report, nil
}

// Records re-reads and verifies all committed records.
func (j *Journal) Records() ([]JournalRecord, error) {
	records, _, err := loadJournalRecords(j.path)
	return records, err
}

// LastSeq returns the highest committed sequence number.
func (j *Journal) LastSeq() uint64 {
	return j.lastSeq
}

// Append durably commits payload as the next record: encode -> write ->
// fsync, before any side effect may proceed.
func (j *Journal) Append(payload any) (JournalRecord, error) {
	records, report, err := loadJournalRecords(j.path)
	if err != nil {
		return JournalRecord{}, err
	}
	if report.TruncatedTailBytes > 0 {
		return JournalRecord{}, fmt.Errorf(
			"filestore: journal has an incomplete tail before append: %s",
			j.path,
		)
	}
	if len(records) == 0 {
		j.lastSeq = 0
	} else {
		// Another process may have appended while this handle was idle. The
		// authority owner serializes writers with an OS-backed store lock, but
		// each process still has its own cached lastSeq. Refreshing under that
		// lock prevents a later owner from reusing the previous owner's seq.
		j.lastSeq = records[len(records)-1].Seq
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return JournalRecord{}, fmt.Errorf("filestore: encode journal payload: %w", err)
	}
	record := JournalRecord{
		Seq:      j.lastSeq + 1,
		Payload:  encoded,
		Checksum: journalChecksum(j.lastSeq+1, encoded),
	}
	line, err := json.Marshal(record)
	if err != nil {
		return JournalRecord{}, fmt.Errorf("filestore: encode journal record: %w", err)
	}
	if _, err := j.file.Write(append(line, '\n')); err != nil {
		return JournalRecord{}, fmt.Errorf("filestore: append journal record: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		return JournalRecord{}, fmt.Errorf("filestore: fsync journal: %w", err)
	}
	j.lastSeq = record.Seq
	return record, nil
}

// Close closes the journal file handle.
func (j *Journal) Close() error {
	if j == nil || j.file == nil {
		return nil
	}
	return j.file.Close()
}
