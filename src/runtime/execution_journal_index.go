package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"awaken/src/agentcore"
)

// The index contains only locations. JSONL remains the authority. File identity,
// length and modification time invalidate it before any indexed read or write.
type journalIndexEntry struct {
	seq     uint64
	start   int64
	end     int64
	agentID string
}

func (j *ExecutionJournal) ensureIndex() error {
	info, err := os.Stat(j.path)
	if err != nil {
		return err
	}
	if j.indexInfo != nil && os.SameFile(j.indexInfo, info) && j.indexInfo.Size() == info.Size() && j.indexInfo.ModTime() == info.ModTime() {
		return nil
	}
	data, err := os.ReadFile(j.path)
	if err != nil {
		return err
	}
	entries, err := indexJournalBytes(data, 0)
	if err != nil {
		return err
	}
	after, err := os.Stat(j.path)
	if err != nil {
		return err
	}
	if !os.SameFile(info, after) || info.Size() != after.Size() || info.ModTime() != after.ModTime() {
		return fmt.Errorf("journal changed during index rebuild")
	}
	sort.SliceStable(entries, func(a, b int) bool { return entries[a].seq < entries[b].seq })
	j.index, j.indexInfo = entries, after
	return nil
}

func indexJournalBytes(data []byte, offset int64) ([]journalIndexEntry, error) {
	var entries []journalIndexEntry
	for len(data) > 0 {
		end := bytes.IndexByte(data, '\n')
		if end < 0 {
			end = len(data)
		} else {
			end++
		}
		line := bytes.TrimSpace(data[:end])
		if len(line) != 0 {
			var record agentcore.SemanticRecord
			if err := json.Unmarshal(line, &record); err != nil {
				if len(bytes.TrimSpace(data[end:])) == 0 {
					break
				} // legacy incomplete tail
				return nil, fmt.Errorf("%w at offset %d: %v", ErrJournalMiddleCorruption, offset, err)
			}
			if record.SchemaVersion != 0 && record.SchemaVersion != agentcore.SemanticSchemaVersion {
				return nil, fmt.Errorf("%w: unsupported schema at offset %d", ErrJournalMiddleCorruption, offset)
			}
			agentID, _ := record.Payload["agent_id"].(string)
			entries = append(entries, journalIndexEntry{seq: record.Seq, start: offset, end: offset + int64(end), agentID: agentID})
		}
		offset += int64(end)
		data = data[end:]
	}
	return entries, nil
}

func (j *ExecutionJournal) indexedRecords(fromSeq, toSeq uint64, agentID *string) ([]agentcore.SemanticRecord, error) {
	if err := j.ensureIndex(); err != nil {
		return nil, err
	}
	start := sort.Search(len(j.index), func(i int) bool { return j.index[i].seq >= fromSeq })
	file, err := os.Open(j.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var result []agentcore.SemanticRecord
	for i := start; i < len(j.index) && j.index[i].seq <= toSeq; i++ {
		entry := j.index[i]
		if agentID != nil && entry.agentID != *agentID {
			continue
		}
		data := make([]byte, entry.end-entry.start)
		if _, err := file.ReadAt(data, entry.start); err != nil && err != io.EOF {
			return nil, err
		}
		var record agentcore.SemanticRecord
		if err := json.Unmarshal(data, &record); err != nil {
			j.indexInfo = nil
			return nil, fmt.Errorf("%w: %v", ErrJournalMiddleCorruption, err)
		}
		result = append(result, record)
	}
	return result, nil
}

func (j *ExecutionJournal) addIndexedBytes(data []byte, offset int64) {
	entries, err := indexJournalBytes(data, offset)
	if err != nil {
		j.indexInfo = nil
		return
	}
	j.index = append(j.index, entries...)
	j.indexInfo, _ = os.Stat(j.path)
}

// SemanticRecordsForAgent uses the same validated index and exact Agent scope.
func (j *ExecutionJournal) SemanticRecordsForAgent(agentID string, fromSeq, toSeq uint64) ([]agentcore.SemanticRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed || fromSeq == 0 || toSeq < fromSeq {
		return nil, fmt.Errorf("invalid journal range")
	}
	return j.indexedRecords(fromSeq, toSeq, &agentID)
}
