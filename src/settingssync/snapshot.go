package settingssync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type SnapshotOptions struct {
	MaxEntryBytes int
}

type SnapshotSkipReason string

const (
	SnapshotSkipInvalidKey SnapshotSkipReason = "invalid_key"
	SnapshotSkipTooLarge   SnapshotSkipReason = "too_large"
)

type SnapshotSkip struct {
	Key       string
	Reason    SnapshotSkipReason
	SizeBytes int
	MaxBytes  int
}

func (s SnapshotSkip) String() string {
	return fmt.Sprintf("settingssync.SnapshotSkip{key:%q reason:%q sizeBytes:%d maxBytes:%d}", s.Key, s.Reason, s.SizeBytes, s.MaxBytes)
}

type LocalSnapshot struct {
	Entries       map[string]Entry
	Skipped       []SnapshotSkip
	MaxEntryBytes int
}

func NewLocalSnapshot(entries []Entry, opts SnapshotOptions) LocalSnapshot {
	maxBytes := opts.MaxEntryBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxEntryBytes
	}

	snapshot := LocalSnapshot{
		Entries:       make(map[string]Entry, len(entries)),
		MaxEntryBytes: maxBytes,
	}

	for _, entry := range entries {
		entry.Key = strings.TrimSpace(entry.Key)
		if entry.Key == "" {
			snapshot.Skipped = append(snapshot.Skipped, SnapshotSkip{
				Key:    entry.Key,
				Reason: SnapshotSkipInvalidKey,
			})
			continue
		}
		size := entry.SizeBytes()
		if size > maxBytes {
			snapshot.Skipped = append(snapshot.Skipped, SnapshotSkip{
				Key:       entry.Key,
				Reason:    SnapshotSkipTooLarge,
				SizeBytes: size,
				MaxBytes:  maxBytes,
			})
			continue
		}
		snapshot.Entries[entry.Key] = entry
	}

	sort.Slice(snapshot.Skipped, func(i, j int) bool {
		return snapshot.Skipped[i].Key < snapshot.Skipped[j].Key
	})

	return snapshot
}

func (s LocalSnapshot) Entry(key string) (Entry, bool) {
	entry, ok := s.Entries[key]
	return entry, ok
}

func (s LocalSnapshot) SortedEntries() []Entry {
	keys := sortedEntryKeys(s.Entries)
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, s.Entries[key])
	}
	return entries
}

func (s LocalSnapshot) Manifest(version int) SyncManifest {
	entries := make(map[string]ManifestEntry, len(s.Entries))
	hashParts := make([]string, 0, len(s.Entries))

	for _, entry := range s.SortedEntries() {
		checksum := checksumString(entry.Value)
		entries[entry.Key] = ManifestEntry{
			Key:       entry.Key,
			Source:    entry.Source,
			Scope:     entry.Scope,
			ProjectID: entry.ProjectID,
			Profile:   entry.Profile,
			SizeBytes: entry.SizeBytes(),
			Checksum:  checksum,
			Sensitive: entry.IsSensitive(),
		}
		hashParts = append(hashParts, entry.Key+"="+checksum)
	}

	return SyncManifest{
		Version:  version,
		Checksum: checksumString(strings.Join(hashParts, "\n")),
		Entries:  entries,
	}
}

func checksumString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
