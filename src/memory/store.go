package memory

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"awaken/src/filestore"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	SchemaVersion       = 1
	IndexFilename       = "MEMORY.md"
	maxIndexLines       = 200
	maxIndexBytes       = 25_000
	maxTopicBytes       = 64 * 1024
	maxCandidateHeaders = 200
)

var (
	ErrNotFound         = errors.New("memory: record not found")
	ErrAmbiguousRef     = errors.New("memory: reference is ambiguous")
	ErrRevisionConflict = errors.New("memory: revision conflict")
	ErrDuplicate        = errors.New("memory: possible duplicate record")
	ErrSensitive        = errors.New("memory: sensitive or disallowed content")
)

type Store struct {
	UserRoot    string
	ProjectRoot string
	Now         func() time.Time
	mu          *sync.Mutex
}

var globalStoreMu sync.Mutex

func NewStore(workspaceRoot string) (*Store, error) {
	home, err := filestore.ResolveAwakenHome("")
	if err != nil {
		return nil, err
	}
	workspaceRoot = filepath.Clean(strings.TrimSpace(workspaceRoot))
	if workspaceRoot != "." && workspaceRoot != "" && !filepath.IsAbs(workspaceRoot) {
		return nil, fmt.Errorf("memory: absolute workspace root required")
	}
	projectRoot := ""
	if workspaceRoot != "." && workspaceRoot != "" {
		projectRoot = filepath.Join(workspaceRoot, ".awaken", "memory")
	}
	store := &Store{
		UserRoot:    filepath.Join(home, "memory"),
		ProjectRoot: projectRoot,
		Now:         func() time.Time { return time.Now().UTC() },
	}
	store.mu = &globalStoreMu
	return store, nil
}

func NewStoreWithRoots(userRoot, projectRoot string) *Store {
	store := &Store{UserRoot: filepath.Clean(userRoot), ProjectRoot: filepath.Clean(projectRoot), Now: func() time.Time { return time.Now().UTC() }}
	store.mu = &globalStoreMu
	return store
}

func (s *Store) root(scope Scope) (string, error) {
	switch scope {
	case ScopeUser:
		return s.UserRoot, nil
	case ScopeProject:
		if strings.TrimSpace(s.ProjectRoot) == "" {
			return "", fmt.Errorf("memory: project scope requires a confirmed workspace")
		}
		return s.ProjectRoot, nil
	default:
		return "", fmt.Errorf("memory: invalid scope %q", scope)
	}
}

func (s *Store) Create(input CreateInput) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateType(input.Type); err != nil {
		return Record{}, err
	}
	if err := validateSource(input.Source); err != nil {
		return Record{}, err
	}
	if err := validateText(input.Name, input.Description, input.Content, input.Keywords); err != nil {
		return Record{}, err
	}
	root, err := s.prepareRoot(input.Scope)
	if err != nil {
		return Record{}, err
	}
	records, err := s.scanRoot(root, input.Scope, true)
	if err != nil {
		return Record{}, err
	}
	if duplicate(records, input.Description, input.Content) {
		return Record{}, ErrDuplicate
	}
	now := s.Now().UTC()
	id := "mem_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	record := Record{
		SchemaVersion: SchemaVersion, MemoryID: id, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description),
		Type: input.Type, Status: StatusActive, Revision: 1, UpdatedAt: now,
		SourceThreadID: strings.TrimSpace(input.Source.ThreadID), SourceRecordID: strings.TrimSpace(input.Source.RecordID),
		CreatedAt: &now, CapturedBy: strings.TrimSpace(input.Source.CapturedBy), Keywords: cleanStrings(input.Keywords), Content: strings.TrimSpace(input.Content), Scope: input.Scope,
	}
	filename := topicFilename(record)
	record.Path = filepath.Join(root, filename)
	if err := writeRecord(record); err != nil {
		return Record{}, err
	}
	if err := s.rebuildIndexLocked(root, input.Scope); err != nil {
		return Record{}, fmt.Errorf("memory topic committed but index rebuild failed: %w", err)
	}
	return record, nil
}

func (s *Store) Get(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(strings.TrimSpace(id))
}

// ResolveReference converts the canonical mem_* identity or one exact legacy
// topic filename from a model-visible MEMORY.md index into the canonical
// record identity. Legacy aliases are resolved only against records already
// admitted by this Store; arbitrary paths and ambiguous aliases fail closed.
func (s *Store) ResolveReference(reference string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Record{}, ErrNotFound
	}
	if strings.HasPrefix(reference, "mem_") {
		return s.getLocked(reference)
	}
	if filepath.IsAbs(reference) || strings.ContainsAny(reference, `/\\`) || reference == "." || reference == ".." {
		return Record{}, ErrNotFound
	}

	legacyFilename := filepath.Base(reference)
	legacyStem := strings.TrimSuffix(legacyFilename, filepath.Ext(legacyFilename))
	var matches []Record
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		root, rootErr := s.root(scope)
		if rootErr != nil {
			continue
		}
		records, err := s.scanRoot(root, scope, true)
		if err != nil {
			return Record{}, err
		}
		for _, record := range records {
			filename := filepath.Base(record.Path)
			if filename == legacyFilename || strings.TrimSuffix(filename, filepath.Ext(filename)) == legacyStem {
				matches = append(matches, record)
			}
		}
	}
	if len(matches) == 0 {
		return Record{}, ErrNotFound
	}
	if len(matches) > 1 {
		return Record{}, ErrAmbiguousRef
	}
	return matches[0], nil
}

func (s *Store) Search(query string, scope *Scope, types []Type, limit int) ([]Record, error) {
	return s.List(query, scope, types, []Status{StatusActive}, limit)
}

// List returns bounded metadata for Settings. Unlike Agent search it may
// include superseded records, while still withholding topic content.
func (s *Store) List(query string, scope *Scope, types []Type, statuses []Status, limit int) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	typeSet := map[Type]bool{}
	for _, typ := range types {
		if err := validateType(typ); err != nil {
			return nil, err
		}
		typeSet[typ] = true
	}
	statusSet := map[Status]bool{}
	for _, status := range statuses {
		if status != StatusActive && status != StatusSuperseded {
			return nil, fmt.Errorf("memory: invalid status %q", status)
		}
		statusSet[status] = true
	}
	scopes := []Scope{ScopeProject, ScopeUser}
	if scope != nil {
		scopes = []Scope{*scope}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	var found []Record
	for _, itemScope := range scopes {
		root, err := s.root(itemScope)
		if err != nil {
			return nil, err
		}
		records, err := s.scanRoot(root, itemScope, true)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if (len(statusSet) > 0 && !statusSet[record.Status]) || (len(typeSet) > 0 && !typeSet[record.Type]) {
				continue
			}
			haystack := strings.ToLower(record.Name + "\n" + record.Description + "\n" + strings.Join(record.Keywords, " ") + "\n" + record.Content)
			if query == "" || strings.Contains(haystack, query) {
				record.Content = ""
				found = append(found, record)
			}
		}
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].UpdatedAt.After(found[j].UpdatedAt) })
	if len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}

func (s *Store) Update(id string, expectedRevision int, patch Patch, source Source) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.getLocked(strings.TrimSpace(id))
	if err != nil {
		return Record{}, err
	}
	if record.Revision != expectedRevision {
		return Record{}, ErrRevisionConflict
	}
	if patch.Name != nil {
		record.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Description != nil {
		record.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Content != nil {
		record.Content = strings.TrimSpace(*patch.Content)
	}
	if patch.Keywords != nil {
		record.Keywords = cleanStrings(patch.Keywords)
	}
	if err := validateText(record.Name, record.Description, record.Content, record.Keywords); err != nil {
		return Record{}, err
	}
	if err := validateSource(source); err != nil {
		return Record{}, err
	}
	record.Revision++
	record.UpdatedAt = s.Now().UTC()
	record.SourceThreadID, record.SourceRecordID = strings.TrimSpace(source.ThreadID), strings.TrimSpace(source.RecordID)
	if source.CapturedBy != "" {
		record.CapturedBy = strings.TrimSpace(source.CapturedBy)
	}
	if err := writeRecord(record); err != nil {
		return Record{}, err
	}
	root, _ := s.root(record.Scope)
	if err := s.rebuildIndexLocked(root, record.Scope); err != nil {
		return Record{}, fmt.Errorf("memory topic committed but index rebuild failed: %w", err)
	}
	return record, nil
}

func (s *Store) Supersede(id string, expectedRevision int, source Source) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.getLocked(strings.TrimSpace(id))
	if err != nil {
		return Record{}, err
	}
	if record.Revision != expectedRevision {
		return Record{}, ErrRevisionConflict
	}
	if err := validateSource(source); err != nil {
		return Record{}, err
	}
	record.Status, record.Revision, record.UpdatedAt = StatusSuperseded, record.Revision+1, s.Now().UTC()
	record.SourceThreadID, record.SourceRecordID = source.ThreadID, source.RecordID
	if err := writeRecord(record); err != nil {
		return Record{}, err
	}
	root, _ := s.root(record.Scope)
	if err := s.rebuildIndexLocked(root, record.Scope); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) Delete(id string, expectedRevision int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.getLocked(strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if record.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	root, _ := s.root(record.Scope)
	if err := filestore.WithinRoot(root, record.Path); err != nil {
		return err
	}
	if err := os.Remove(record.Path); err != nil {
		return err
	}
	if err := syncDirectory(root); err != nil {
		return err
	}
	return s.rebuildIndexLocked(root, record.Scope)
}

func (s *Store) Candidates(scope Scope) ([]Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root(scope)
	if err != nil {
		return nil, err
	}
	return s.candidatesLocked(root, scope)
}

// ReadSelected revalidates selector filenames against the candidate allowlist
// and returns at most five bounded topic records.
func (s *Store) ReadSelected(scope Scope, filenames []string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root(scope)
	if err != nil {
		return nil, err
	}
	allowedCandidates, err := s.candidatesLocked(root, scope)
	if err != nil {
		return nil, err
	}
	allowed := map[string]Candidate{}
	for _, candidate := range allowedCandidates {
		allowed[candidate.Filename] = candidate
	}
	if len(filenames) > 5 {
		filenames = filenames[:5]
	}
	result := make([]Record, 0, len(filenames))
	seen := map[string]bool{}
	for _, filename := range filenames {
		filename = filepath.ToSlash(filepath.Clean(strings.TrimSpace(filename)))
		if seen[filename] {
			continue
		}
		seen[filename] = true
		candidate, ok := allowed[filename]
		if !ok {
			continue
		}
		record, err := readRecord(filepath.Join(root, filepath.FromSlash(filename)), scope)
		if err != nil || record.Revision != candidate.Revision {
			continue
		}
		record.Content = boundTopicContent(record.Content, 200, 4096)
		result = append(result, record)
	}
	return result, nil
}

func (s *Store) candidatesLocked(root string, scope Scope) ([]Candidate, error) {
	if err := s.ensureMaintenanceReady(root); err != nil {
		return nil, err
	}
	type topicFile struct {
		filename string
		path     string
		modTime  time.Time
	}
	var files []topicFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) && path == root {
				return nil
			}
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".md" || entry.Name() == IndexFilename {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil
		}
		files = append(files, topicFile{filename: filepath.ToSlash(relative), path: path, modTime: info.ModTime()})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
	if len(files) > maxCandidateHeaders {
		files = files[:maxCandidateHeaders]
	}
	result := make([]Candidate, 0, len(files))
	for _, file := range files {
		record, err := readHeaderRecord(file.path, scope, 30)
		if err != nil || record.Status != StatusActive {
			continue
		}
		result = append(result, Candidate{Filename: file.filename, MemoryID: record.MemoryID, Name: record.Name, Description: record.Description, Type: record.Type, Scope: scope, Revision: record.Revision, UpdatedAt: record.UpdatedAt, ModTime: file.modTime})
	}
	return result, nil
}

func (s *Store) Index(scope Scope) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.root(scope)
	if err != nil {
		return "", err
	}
	if err := s.ensureMaintenanceReady(root); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, IndexFilename))
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Stat(root); errors.Is(statErr, os.ErrNotExist) {
			return "", nil
		}
		if rebuildErr := s.rebuildIndexLocked(root, scope); rebuildErr != nil {
			return "", rebuildErr
		}
		data, err = os.ReadFile(filepath.Join(root, IndexFilename))
	}
	return string(data), err
}

func (s *Store) getLocked(id string) (Record, error) {
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		root, rootErr := s.root(scope)
		if rootErr != nil {
			continue
		}
		records, err := s.scanRoot(root, scope, true)
		if err != nil {
			return Record{}, err
		}
		for _, record := range records {
			if record.MemoryID == id {
				return record, nil
			}
		}
	}
	return Record{}, ErrNotFound
}

func (s *Store) prepareRoot(scope Scope) (string, error) {
	root, err := s.root(scope)
	if err != nil {
		return "", err
	}
	if err := filestore.EnsureDir(root); err != nil {
		return "", err
	}
	if err := filestore.WithinRoot(root, root); err != nil {
		return "", err
	}
	if err := s.ensureMaintenanceReady(root); err != nil {
		return "", err
	}
	return root, nil
}

func (s *Store) scanRoot(root string, scope Scope, includeSuperseded bool) ([]Record, error) {
	if err := s.ensureMaintenanceReady(root); err != nil {
		return nil, err
	}
	var records []Record
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) && path == root {
				return nil
			}
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".md" || entry.Name() == IndexFilename {
			return nil
		}
		record, readErr := readRecord(path, scope)
		if readErr == nil && (includeSuperseded || record.Status == StatusActive) {
			records = append(records, record)
		}
		return nil
	})
	return records, err
}

func readRecord(path string, scope Scope) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	if len(data) > maxTopicBytes || !utf8.Valid(data) {
		return Record{}, fmt.Errorf("memory: invalid topic encoding or size")
	}
	header, body, err := splitFrontmatter(data)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := yaml.Unmarshal(header, &record); err != nil {
		return Record{}, err
	}
	record.Scope, record.Path, record.Content = scope, path, strings.TrimSpace(string(body))
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func readHeaderRecord(path string, scope Scope, maxLines int) (Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 16*1024)
	lines := make([]string, 0, maxLines)
	for scanner.Scan() && len(lines) < maxLines {
		lines = append(lines, scanner.Text())
		if len(lines) > 1 && scanner.Text() == "---" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Record{}, err
	}
	if len(lines) < 3 || lines[0] != "---" || lines[len(lines)-1] != "---" {
		return Record{}, fmt.Errorf("memory: frontmatter exceeds %d lines", maxLines)
	}
	var record Record
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:len(lines)-1], "\n")), &record); err != nil {
		return Record{}, err
	}
	record.Scope, record.Path = scope, path
	if err := validateHeader(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func validateHeader(record Record) error {
	if record.SchemaVersion != SchemaVersion || !strings.HasPrefix(record.MemoryID, "mem_") || record.Revision < 1 || record.UpdatedAt.IsZero() || strings.TrimSpace(record.Name) == "" || strings.TrimSpace(record.Description) == "" || strings.TrimSpace(record.SourceThreadID) == "" || strings.TrimSpace(record.SourceRecordID) == "" {
		return fmt.Errorf("memory: invalid required frontmatter")
	}
	if record.Status != StatusActive && record.Status != StatusSuperseded {
		return fmt.Errorf("memory: invalid status")
	}
	for _, forbidden := range []string{"scope", "source_run_id", "source_turn_id", "path", "content"} {
		if _, exists := record.Extra[forbidden]; exists {
			return fmt.Errorf("memory: forbidden derived frontmatter field %q", forbidden)
		}
	}
	return validateType(record.Type)
}

func boundTopicContent(content string, maxLines, maxBytes int) string {
	lines := strings.Split(content, "\n")
	truncated := false
	if len(lines) > maxLines {
		lines, truncated = lines[:maxLines], true
	}
	result := strings.Join(lines, "\n")
	if len(result) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(result[cut]) {
			cut--
		}
		result, truncated = result[:cut], true
	}
	if truncated {
		result += "\n[truncated by host; use memory get for the complete topic]"
	}
	return result
}

func writeRecord(record Record) error {
	data, err := encodeRecord(record)
	if err != nil {
		return err
	}
	return atomicWrite(record.Path, data)
}

func encodeRecord(record Record) ([]byte, error) {
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	header, err := yaml.Marshal(record)
	if err != nil {
		return nil, err
	}
	data := append([]byte("---\n"), header...)
	data = append(data, []byte("---\n\n")...)
	data = append(data, []byte(strings.TrimSpace(record.Content)+"\n")...)
	if len(data) > maxTopicBytes {
		return nil, fmt.Errorf("memory: topic exceeds %d bytes", maxTopicBytes)
	}
	return data, nil
}

func (s *Store) rebuildIndexLocked(root string, scope Scope) error {
	if err := filestore.EnsureDir(root); err != nil {
		return err
	}
	records, err := s.scanRoot(root, scope, false)
	if err != nil {
		return err
	}
	data := buildIndexData(records)
	return atomicWrite(filepath.Join(root, IndexFilename), data)
}

func buildIndexData(records []Record) []byte {
	sort.SliceStable(records, func(i, j int) bool { return records[i].UpdatedAt.After(records[j].UpdatedAt) })
	var buf bytes.Buffer
	for _, record := range records {
		line := fmt.Sprintf(
			"- [%s](%s) — memory_id: `%s`; scope: `%s`; type: `%s`; revision: `%d` — %s\n",
			markdownLabel(record.Name), filepath.Base(record.Path), record.MemoryID, record.Scope, record.Type, record.Revision, oneLine(record.Description),
		)
		if buf.Len()+len(line) > maxIndexBytes || bytes.Count(buf.Bytes(), []byte("\n")) >= maxIndexLines {
			break
		}
		buf.WriteString(line)
	}
	return buf.Bytes()
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, nil, fmt.Errorf("memory: missing YAML frontmatter")
	}
	rest := data[4:]
	index := bytes.Index(rest, []byte("\n---\n"))
	if index < 0 {
		return nil, nil, fmt.Errorf("memory: unterminated YAML frontmatter")
	}
	return rest[:index], rest[index+5:], nil
}

func validateRecord(record Record) error {
	if record.SchemaVersion != SchemaVersion || !strings.HasPrefix(record.MemoryID, "mem_") || record.Revision < 1 || record.UpdatedAt.IsZero() {
		return fmt.Errorf("memory: invalid required frontmatter")
	}
	if record.Status != StatusActive && record.Status != StatusSuperseded {
		return fmt.Errorf("memory: invalid status")
	}
	if err := validateType(record.Type); err != nil {
		return err
	}
	if err := validateSource(Source{ThreadID: record.SourceThreadID, RecordID: record.SourceRecordID}); err != nil {
		return err
	}
	for _, forbidden := range []string{"scope", "source_run_id", "source_turn_id", "path", "content"} {
		if _, exists := record.Extra[forbidden]; exists {
			return fmt.Errorf("memory: forbidden derived frontmatter field %q", forbidden)
		}
	}
	return validateText(record.Name, record.Description, record.Content, record.Keywords)
}

func validateType(value Type) error {
	switch value {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
		return nil
	}
	return fmt.Errorf("memory: invalid type %q", value)
}

func validateSource(source Source) error {
	if strings.TrimSpace(source.ThreadID) == "" || strings.TrimSpace(source.RecordID) == "" {
		return fmt.Errorf("memory: host source identity required")
	}
	return nil
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)authorization\s*:`), regexp.MustCompile(`(?i)\b(api[_ -]?key|access[_ -]?token|refresh[_ -]?token|password|passwd|cookie|secret)\b\s*[:=]`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`), regexp.MustCompile(`(?i)\b(one[- ]?time code|verification code|验证码)\b\s*[:：]?\s*\d{4,8}\b`),
	regexp.MustCompile(`(?i)<system>|system prompt|完整对话|raw transcript`),
}

func validateText(name, description, content string, keywords []string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(description) == "" || strings.TrimSpace(content) == "" {
		return fmt.Errorf("memory: name, description and content are required")
	}
	combined := strings.Join(append([]string{name, description, content}, keywords...), "\n")
	if !utf8.ValidString(combined) {
		return fmt.Errorf("memory: invalid UTF-8")
	}
	for _, pattern := range sensitivePatterns {
		if pattern.MatchString(combined) {
			return ErrSensitive
		}
	}
	return nil
}

func duplicate(records []Record, description, content string) bool {
	want := normalizedFingerprint(description + "\n" + content)
	for _, record := range records {
		if normalizedFingerprint(record.Description+"\n"+record.Content) == want {
			return true
		}
	}
	return false
}

func normalizedFingerprint(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func topicFilename(record Record) string {
	short := strings.TrimPrefix(record.MemoryID, "mem_")
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("%s-%s-%s.md", record.Type, slug(record.Name), short)
}

func slug(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = "topic"
	}
	if len(result) > 32 {
		result = strings.TrimRight(result[:32], "-")
	}
	return result
}

func oneLine(value string) string { return strings.Join(strings.Fields(value), " ") }
func markdownLabel(value string) string {
	value = oneLine(value)
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "[", "\\[")
	return strings.ReplaceAll(value, "]", "\\]")
}
func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := filestore.EnsureDir(dir); err != nil {
		return err
	}
	if err := filestore.WithinRoot(dir, path); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".memory-stage-*")
	if err != nil {
		return err
	}
	tempName := temporary.Name()
	defer os.Remove(tempName)
	if err := temporary.Chmod(filestore.FilePerm); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
