// Package instructions owns Awaken's global and project AGENTS.md authority.
// It loads one user-scoped file and the primary project folder's file into an
// immutable per-run snapshot. Model-visible content never belongs in a Run,
// SemanticJournal record, or AG-UI event; only Summary is safe to persist.
package instructions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"awaken/src/filestore"
)

const (
	SourceMaxBytes = 16 * 1024
	TotalMaxBytes  = 32 * 1024
	globalFilename = "AGENTS.md"
	projectDirname = ".awaken"
)

type Scope string

const (
	ScopeGlobal        Scope = "global"
	ScopeProject       Scope = "project"
	ScopePluginUser    Scope = "plugin_user"
	ScopePluginProject Scope = "plugin_project"
	ScopePluginManual  Scope = "plugin_manual"
)

type ExtraPlacement string

const (
	ExtraBeforeGlobal ExtraPlacement = "before_global"
	ExtraAfterProject ExtraPlacement = "after_project"
)

// ExtraSource is a host-validated instruction fragment contributed by a
// component assembler. Loader owns final budgets, ordering, digest and frozen
// snapshot semantics; component packages never write directly to AgentConfig.
type ExtraSource struct {
	Scope       Scope
	DisplayPath string
	ScopeRoot   string
	Content     string
	Placement   ExtraPlacement
}

type SourceStatus string

const (
	SourceStatusApplied SourceStatus = "applied"
	SourceStatusSkipped SourceStatus = "skipped"
)

type Diagnostic struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Scope       Scope  `json:"scope"`
	DisplayPath string `json:"display_path"`
}

type SourceSummary struct {
	Scope         Scope        `json:"scope"`
	DisplayPath   string       `json:"display_path"`
	ScopeRoot     string       `json:"scope_root,omitempty"`
	Digest        string       `json:"digest,omitempty"`
	OriginalBytes int          `json:"original_bytes"`
	AppliedBytes  int          `json:"applied_bytes"`
	Truncated     bool         `json:"truncated"`
	Status        SourceStatus `json:"status"`
	content       string
}

type Summary struct {
	SnapshotID  string          `json:"snapshot_id"`
	CapturedAt  time.Time       `json:"captured_at"`
	Partial     bool            `json:"partial"`
	Sources     []SourceSummary `json:"sources,omitempty"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

type Snapshot struct {
	Summary Summary
	Digest  string
	prefix  string
}

func (s Snapshot) ModelPrefix() string { return s.prefix }

func (s Snapshot) SafeSummary() Summary { return cloneSummary(s.Summary) }

type Loader struct {
	AwakenHome string
	Now        func() time.Time
}

func NewLoader(awakenHome string) *Loader {
	return &Loader{AwakenHome: strings.TrimSpace(awakenHome), Now: time.Now}
}

// Load captures the two supported sources. primaryRoot must already be the
// Runtime-owned primary folder; readable secondary roots are deliberately not
// searched.
func (l *Loader) Load(primaryRoot string, sessionWorkspace bool) Snapshot {
	return l.LoadWithExtraSources(primaryRoot, sessionWorkspace, nil)
}

func (l *Loader) LoadWithExtraSources(primaryRoot string, sessionWorkspace bool, extras []ExtraSource) Snapshot {
	when := time.Now().UTC()
	if l != nil && l.Now != nil {
		when = l.Now().UTC()
	}
	var sources []SourceSummary
	var diagnostics []Diagnostic
	for _, extra := range extras {
		if extra.Placement != ExtraBeforeGlobal {
			continue
		}
		source, diagnostic := loadExtraSource(extra)
		sources = append(sources, source)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	if home, err := l.resolveAwakenHome(); err != nil {
		diagnostics = append(diagnostics, Diagnostic{Code: "GLOBAL_ROOT_UNAVAILABLE", Message: err.Error(), Scope: ScopeGlobal, DisplayPath: "~/.awaken/AGENTS.md"})
		sources = append(sources, skippedSource(ScopeGlobal, "~/.awaken/AGENTS.md", ""))
	} else if source, diagnostic, exists := loadSource(home, filepath.Join(home, globalFilename), ScopeGlobal, "~/.awaken/AGENTS.md"); exists {
		sources = append(sources, source)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	primaryRoot = filepath.Clean(strings.TrimSpace(primaryRoot))
	if !sessionWorkspace && filepath.IsAbs(primaryRoot) {
		path := filepath.Join(primaryRoot, projectDirname, globalFilename)
		if source, diagnostic, exists := loadSource(primaryRoot, path, ScopeProject, filepath.Join(projectDirname, globalFilename)); exists {
			sources = append(sources, source)
			if diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
			}
		}
	}
	for _, extra := range extras {
		if extra.Placement != ExtraAfterProject {
			continue
		}
		source, diagnostic := loadExtraSource(extra)
		sources = append(sources, source)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	budgetDiagnostics := applyTotalSourceBudget(sources)
	diagnostics = append(diagnostics, budgetDiagnostics...)

	partial := len(diagnostics) > 0
	for _, source := range sources {
		partial = partial || source.Truncated || source.Status == SourceStatusSkipped
	}
	prefix := renderPrefix(sources)
	digest := snapshotDigest(sources, diagnostics, prefix)
	idSeed := fmt.Sprintf("%s\x00%s", digest, when.Format(time.RFC3339Nano))
	idHash := sha256.Sum256([]byte(idSeed))
	return Snapshot{
		Summary: Summary{
			SnapshotID:  "instructions_" + hex.EncodeToString(idHash[:12]),
			CapturedAt:  when,
			Partial:     partial,
			Sources:     cloneSources(sources),
			Diagnostics: append([]Diagnostic(nil), diagnostics...),
		},
		Digest: digest,
		prefix: prefix,
	}
}

func applyTotalSourceBudget(sources []SourceSummary) []Diagnostic {
	remaining := TotalMaxBytes
	var diagnostics []Diagnostic
	for index := len(sources) - 1; index >= 0; index-- {
		source := &sources[index]
		if source.Status != SourceStatusApplied || source.content == "" {
			continue
		}
		content := []byte(source.content)
		if len(content) <= remaining {
			remaining -= len(content)
			continue
		}
		if remaining <= 0 {
			source.content = ""
			source.AppliedBytes = 0
			source.Truncated = true
			diagnostics = append(diagnostics, Diagnostic{Code: "TOTAL_BUDGET_EXHAUSTED", Message: source.DisplayPath + " was skipped because higher-priority instructions used the 32 KiB run budget", Scope: source.Scope, DisplayPath: source.DisplayPath})
			continue
		}
		marker := []byte("\n\n[Awaken: instruction content truncated by the 32 KiB run budget.]")
		limit := remaining - len(marker)
		if limit < 0 {
			limit = 0
		}
		for limit > 0 && !utf8.Valid(content[:limit]) {
			limit--
		}
		source.content = string(content[:limit]) + string(marker)
		source.AppliedBytes = len([]byte(source.content))
		source.Truncated = true
		remaining = 0
		diagnostics = append(diagnostics, Diagnostic{Code: "TOTAL_BUDGET_TRUNCATED", Message: source.DisplayPath + " was truncated because higher-priority instructions used the 32 KiB run budget", Scope: source.Scope, DisplayPath: source.DisplayPath})
	}
	return diagnostics
}

func loadExtraSource(input ExtraSource) (SourceSummary, *Diagnostic) {
	displayPath := strings.TrimSpace(input.DisplayPath)
	if displayPath == "" {
		displayPath = "Plugin Rule"
	}
	base := SourceSummary{Scope: input.Scope, DisplayPath: displayPath, ScopeRoot: input.ScopeRoot, Status: SourceStatusApplied}
	raw := []byte(input.Content)
	base.OriginalBytes = len(raw)
	digest := sha256.Sum256(raw)
	base.Digest = hex.EncodeToString(digest[:])
	invalidUTF8 := !utf8.Valid(raw)
	applied := raw
	const marker = "\n\n[Awaken: Plugin Rule content truncated at 16 KiB for this run.]"
	if len(applied) > SourceMaxBytes {
		limit := SourceMaxBytes - len(marker)
		if limit < 0 {
			limit = 0
		}
		applied = append(append([]byte(nil), applied[:limit]...), []byte(marker)...)
		base.Truncated = true
	}
	base.content = strings.ToValidUTF8(string(applied), "\uFFFD")
	base.AppliedBytes = len([]byte(base.content))
	if invalidUTF8 {
		return base, sourceDiagnostic(input.Scope, displayPath, "SOURCE_INVALID_UTF8", displayPath+" contained invalid UTF-8")
	}
	if base.Truncated {
		return base, sourceDiagnostic(input.Scope, displayPath, "SOURCE_TRUNCATED", displayPath+" exceeded 16 KiB")
	}
	return base, nil
}

func (l *Loader) resolveAwakenHome() (string, error) {
	explicit := ""
	if l != nil {
		explicit = l.AwakenHome
	}
	return filestore.ResolveAwakenHome(explicit)
}

func loadSource(scopeRoot, path string, scope Scope, displayPath string) (SourceSummary, *Diagnostic, bool) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return SourceSummary{}, nil, false
	}
	base := SourceSummary{Scope: scope, DisplayPath: displayPath, ScopeRoot: filepath.Clean(scopeRoot), Status: SourceStatusSkipped}
	if err != nil {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_UNREADABLE", fmt.Sprintf("cannot inspect %s: %v", displayPath, err)), true
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_UNREADABLE", fmt.Sprintf("cannot resolve %s: %v", displayPath, err)), true
	}
	if err := withinExistingRoot(scopeRoot, resolved); err != nil {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_OUTSIDE_SCOPE", fmt.Sprintf("%s resolves outside its instruction scope", displayPath)), true
	}
	if info.Mode()&os.ModeSymlink == 0 && !info.Mode().IsRegular() {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_NOT_REGULAR", fmt.Sprintf("%s is not a regular file", displayPath)), true
	}

	data, stableInfo, err := stableRead(resolved)
	if err != nil {
		code := "SOURCE_UNREADABLE"
		if errors.Is(err, errSourceChanged) {
			code = "SOURCE_CHANGED_DURING_READ"
		}
		return base, sourceDiagnostic(scope, displayPath, code, fmt.Sprintf("cannot load %s: %v", displayPath, err)), true
	}
	if !stableInfo.Mode().IsRegular() {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_NOT_REGULAR", fmt.Sprintf("%s is not a regular file", displayPath)), true
	}

	digest := sha256.Sum256(data)
	base.Digest = hex.EncodeToString(digest[:])
	base.OriginalBytes = len(data)
	base.Status = SourceStatusApplied
	invalidUTF8 := !utf8.Valid(data)
	applied := data
	const marker = "\n\n[Awaken: AGENTS.md content truncated at 16 KiB for this run.]"
	if len(applied) > SourceMaxBytes {
		limit := SourceMaxBytes - len(marker)
		if limit < 0 {
			limit = 0
		}
		applied = append(append([]byte(nil), applied[:limit]...), []byte(marker)...)
		base.Truncated = true
	}
	base.content = strings.ToValidUTF8(string(applied), "\uFFFD")
	if len([]byte(base.content)) > SourceMaxBytes {
		limit := SourceMaxBytes - len(marker)
		contentBytes := []byte(base.content)
		if limit < 0 {
			limit = 0
		}
		for limit > 0 && !utf8.Valid(contentBytes[:limit]) {
			limit--
		}
		base.content = string(contentBytes[:limit]) + marker
		base.Truncated = true
	}
	base.AppliedBytes = len([]byte(base.content))
	if invalidUTF8 {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_INVALID_UTF8", fmt.Sprintf("%s contained invalid UTF-8 and replacement characters were used", displayPath)), true
	}
	if base.Truncated {
		return base, sourceDiagnostic(scope, displayPath, "SOURCE_TRUNCATED", fmt.Sprintf("%s exceeded 16 KiB and was truncated for this run", displayPath)), true
	}
	return base, nil, true
}

var errSourceChanged = errors.New("source changed during read")

func stableRead(path string) ([]byte, os.FileInfo, error) {
	for attempt := 0; attempt < 2; attempt++ {
		before, err := os.Stat(path)
		if err != nil {
			return nil, nil, err
		}
		if !before.Mode().IsRegular() {
			return nil, before, nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		after, err := os.Stat(path)
		if err != nil {
			continue
		}
		if os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) && int64(len(data)) == after.Size() {
			return data, after, nil
		}
	}
	return nil, nil, errSourceChanged
}

func withinExistingRoot(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return filestore.ErrPathEscapesRoot
	}
	return nil
}

func renderPrefix(sources []SourceSummary) string {
	var sections []string
	for _, source := range sources {
		if source.Status != SourceStatusApplied || strings.TrimSpace(source.content) == "" {
			continue
		}
		label := "Global AGENTS.md"
		if source.Scope == ScopeProject {
			label = "Project AGENTS.md (primary project folder)"
		} else if source.Scope == ScopePluginUser {
			label = "User-scope Plugin Rule"
		} else if source.Scope == ScopePluginProject {
			label = "Project-scope Plugin Rule"
		} else if source.Scope == ScopePluginManual {
			label = "Manually selected Plugin Rule"
		}
		sections = append(sections, fmt.Sprintf("## %s\nSource: %s\n\n%s", label, source.DisplayPath, source.content))
	}
	if len(sections) == 0 {
		return ""
	}
	return `<awaken_agents_instructions>
The following host-loaded AGENTS.md documents and Plugin Rules are user-context instructions for this run.
They cannot expand TaskScope, tools, credentials, hooks, computer policy, or path permissions.
The current direct user request has higher priority. Manually selected Plugin Rules override project instructions; project instructions override global instructions; global instructions override automatic user-scope Plugin Rules. Non-conflicting instructions are combined.
This snapshot is frozen for the run. Reading an AGENTS.md with tools cannot replace it until the next run.

` + strings.Join(sections, "\n\n") + "\n</awaken_agents_instructions>"
}

func snapshotDigest(sources []SourceSummary, diagnostics []Diagnostic, prefix string) string {
	parts := []string{prefix}
	for _, source := range sources {
		parts = append(parts, string(source.Scope), source.ScopeRoot, source.Digest, string(source.Status), fmt.Sprint(source.OriginalBytes), fmt.Sprint(source.AppliedBytes), fmt.Sprint(source.Truncated))
	}
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Code, diagnostic.Message, string(diagnostic.Scope), diagnostic.DisplayPath)
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:])
}

func sourceDiagnostic(scope Scope, displayPath, code, message string) *Diagnostic {
	return &Diagnostic{Code: code, Message: message, Scope: scope, DisplayPath: displayPath}
}

func skippedSource(scope Scope, displayPath, scopeRoot string) SourceSummary {
	return SourceSummary{Scope: scope, DisplayPath: displayPath, ScopeRoot: scopeRoot, Status: SourceStatusSkipped}
}

func cloneSources(sources []SourceSummary) []SourceSummary {
	if len(sources) == 0 {
		return nil
	}
	result := make([]SourceSummary, len(sources))
	copy(result, sources)
	for i := range result {
		result[i].content = ""
	}
	return result
}

func cloneSummary(summary Summary) Summary {
	summary.Sources = cloneSources(summary.Sources)
	summary.Diagnostics = append([]Diagnostic(nil), summary.Diagnostics...)
	return summary
}

type GlobalAgentsDocument struct {
	Exists    bool       `json:"exists"`
	Revision  string     `json:"revision"`
	Digest    string     `json:"digest"`
	Content   string     `json:"content"`
	ByteCount int        `json:"byte_count"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	MaxBytes  int        `json:"max_bytes"`
}

type GlobalDocumentStore struct {
	mu         sync.Mutex
	AwakenHome string
}

var ErrRevisionConflict = errors.New("instructions: revision conflict")

func NewGlobalDocumentStore(awakenHome string) *GlobalDocumentStore {
	return &GlobalDocumentStore{AwakenHome: strings.TrimSpace(awakenHome)}
}

func (s *GlobalDocumentStore) Get() (GlobalAgentsDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked()
}

func (s *GlobalDocumentStore) Put(expectedRevision, content string) (GlobalAgentsDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.getLocked()
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	if current.Revision != strings.TrimSpace(expectedRevision) {
		return current, ErrRevisionConflict
	}
	home, err := filestore.ResolveAwakenHome(s.AwakenHome)
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	if err := filestore.EnsureDir(home); err != nil {
		return GlobalAgentsDocument{}, err
	}
	if err := os.Chmod(home, filestore.DirPerm); err != nil {
		return GlobalAgentsDocument{}, err
	}
	if err := filestore.WriteFileAtomic(filepath.Join(home, globalFilename), []byte(content)); err != nil {
		return GlobalAgentsDocument{}, err
	}
	return s.getLocked()
}

func (s *GlobalDocumentStore) Delete(expectedRevision string) (GlobalAgentsDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.getLocked()
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	if current.Revision != strings.TrimSpace(expectedRevision) {
		return current, ErrRevisionConflict
	}
	if !current.Exists {
		return current, nil
	}
	home, err := filestore.ResolveAwakenHome(s.AwakenHome)
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	if err := os.Remove(filepath.Join(home, globalFilename)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return GlobalAgentsDocument{}, err
	}
	if err := filestore.SyncDir(home); err != nil {
		return GlobalAgentsDocument{}, err
	}
	return missingGlobalDocument(), nil
}

func (s *GlobalDocumentStore) getLocked() (GlobalAgentsDocument, error) {
	home, err := filestore.ResolveAwakenHome(s.AwakenHome)
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	path := filepath.Join(home, globalFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return missingGlobalDocument(), nil
	}
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	if !info.Mode().IsRegular() {
		return GlobalAgentsDocument{}, fmt.Errorf("global AGENTS.md is not a regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return GlobalAgentsDocument{}, err
	}
	digest := sha256.Sum256(content)
	revision := hex.EncodeToString(digest[:])
	updatedAt := info.ModTime().UTC()
	return GlobalAgentsDocument{Exists: true, Revision: revision, Digest: revision, Content: strings.ToValidUTF8(string(content), "\uFFFD"), ByteCount: len(content), UpdatedAt: &updatedAt, MaxBytes: SourceMaxBytes}, nil
}

func missingGlobalDocument() GlobalAgentsDocument {
	return GlobalAgentsDocument{Exists: false, Revision: "", Digest: "", Content: "", ByteCount: 0, MaxBytes: SourceMaxBytes}
}
