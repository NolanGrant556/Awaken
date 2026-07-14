package filestore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Format is the FORMAT.json compatibility gate of a store generation.
type Format struct {
	FormatVersion    int       `json:"formatVersion"`
	MinReaderVersion int       `json:"minReaderVersion"`
	MinWriterVersion int       `json:"minWriterVersion"`
	CreatedAt        time.Time `json:"createdAt"`
	UpgradedAt       time.Time `json:"upgradedAt,omitempty"`
	FeatureFlags     []string  `json:"featureFlags,omitempty"`
}

// CurrentFormatVersion is the format this build reads and writes.
const CurrentFormatVersion = 1

// CheckWritable gates write access: a store demanding a newer writer than
// this build must be treated read-only or refused, never overwritten as an
// empty store.
func (f Format) CheckWritable(buildVersion int) error {
	if buildVersion < f.MinWriterVersion {
		return fmt.Errorf("%w: store requires writer >= %d, this build is %d", ErrFormatIncompatible, f.MinWriterVersion, buildVersion)
	}
	return nil
}

// CheckReadable gates read access.
func (f Format) CheckReadable(buildVersion int) error {
	if buildVersion < f.MinReaderVersion {
		return fmt.Errorf("%w: store requires reader >= %d, this build is %d", ErrFormatIncompatible, f.MinReaderVersion, buildVersion)
	}
	return nil
}

// ReadCurrentGeneration resolves the active generation from the CURRENT pointer file.
// A missing pointer, a pointer to a missing generation, or an unreadable
// format fails closed; callers must never guess a "latest" directory or
// create an empty store in response.
func ReadCurrentGeneration(root string) (string, Format, error) {
	pointer := filepath.Join(root, "CURRENT")
	raw, err := os.ReadFile(pointer)
	if err != nil {
		return "", Format{}, fmt.Errorf("filestore: CURRENT pointer unavailable (fail closed): %w", err)
	}
	generationID := strings.TrimSpace(string(raw))
	if generationID == "" || strings.ContainsAny(generationID, "/\\") || generationID == "." || generationID == ".." {
		return "", Format{}, fmt.Errorf("filestore: CURRENT pointer content %q is invalid (fail closed)", generationID)
	}
	generationDir := filepath.Join(root, "store-generations", generationID)
	var format Format
	if err := ReadJSON(filepath.Join(generationDir, "FORMAT.json"), &format); err != nil {
		return "", Format{}, fmt.Errorf("filestore: generation %s format unreadable (fail closed): %w", generationID, err)
	}
	return generationDir, format, nil
}

// PublishGeneration atomically switches CURRENT to generationID after the
// generation content has been fully built and fsynced.
func PublishGeneration(root, generationID string) error {
	if strings.TrimSpace(generationID) == "" || strings.ContainsAny(generationID, "/\\") {
		return fmt.Errorf("filestore: invalid generation id %q", generationID)
	}
	generationDir := filepath.Join(root, "store-generations", generationID)
	if _, err := os.Stat(filepath.Join(generationDir, "FORMAT.json")); err != nil {
		return fmt.Errorf("filestore: refuse to publish generation without FORMAT.json: %w", err)
	}
	return WriteFileAtomic(filepath.Join(root, "CURRENT"), []byte(generationID+"\n"))
}

// InitGeneration creates a new generation directory with its FORMAT.json but
// does not publish it.
func InitGeneration(root, generationID string, format Format) (string, error) {
	generationDir := filepath.Join(root, "store-generations", generationID)
	if err := EnsureDir(generationDir); err != nil {
		return "", err
	}
	if format.FormatVersion == 0 {
		format.FormatVersion = CurrentFormatVersion
		format.MinReaderVersion = CurrentFormatVersion
		format.MinWriterVersion = CurrentFormatVersion
	}
	if format.CreatedAt.IsZero() {
		format.CreatedAt = time.Now().UTC()
	}
	doc, err := marshalIndent(format)
	if err != nil {
		return "", err
	}
	if err := WriteFileAtomic(filepath.Join(generationDir, "FORMAT.json"), doc); err != nil {
		return "", err
	}
	return generationDir, nil
}
