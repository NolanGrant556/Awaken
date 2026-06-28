package subdirhints

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	DefaultMaxAncestorDirs = 5
	DefaultMaxHintChars    = 8000
)

var defaultHintFilenames = []string{
	"AGENTS.md",
	"agents.md",
	"CLAUDE.md",
	"claude.md",
	".cursorrules",
}

var pathArgKeys = []string{"path", "file_path", "workdir", "cwd", "dir"}
var commandArgKeys = []string{"command"}

type Options struct {
	MaxAncestorDirs int
	MaxHintChars    int
	HintFilenames   []string
}

type Tracker struct {
	workingDir      string
	maxAncestorDirs int
	maxHintChars    int
	hintFilenames   []string
	loadedDirs      map[string]struct{}
}

type Result struct {
	Text     string
	Hints    []Hint
	Warnings []string
}

type Hint struct {
	Dir       string
	Path      string
	RelPath   string
	Filename  string
	Content   string
	Truncated bool
}

type Snapshot struct {
	WorkingDir      string
	LoadedDirs      []string
	MaxAncestorDirs int
	MaxHintChars    int
	HintFilenames   []string
}

func NewTracker(workingDir string, options Options) (*Tracker, error) {
	resolvedWorkingDir, err := resolveWorkingDir(workingDir)
	if err != nil {
		return nil, err
	}

	options = normalizeOptions(options)
	tracker := &Tracker{
		workingDir:      resolvedWorkingDir,
		maxAncestorDirs: options.MaxAncestorDirs,
		maxHintChars:    options.MaxHintChars,
		hintFilenames:   append([]string(nil), options.HintFilenames...),
		loadedDirs:      make(map[string]struct{}),
	}
	tracker.Reset()
	return tracker, nil
}

func New(workingDir string) (*Tracker, error) {
	return NewTracker(workingDir, Options{})
}

func (t *Tracker) CheckToolCall(_ string, args map[string]any) Result {
	if t == nil || len(args) == 0 {
		return Result{}
	}
	dirs := t.extractDirectories(args)
	return t.loadHintsForDirs(dirs)
}

func (t *Tracker) CheckToolCallJSON(toolName string, arguments string) Result {
	if t == nil {
		return Result{}
	}
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return Result{}
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return Result{
			Warnings: []string{fmt.Sprintf("subdirectory hints skipped invalid tool arguments for %s: %v", toolName, err)},
		}
	}
	return t.CheckToolCall(toolName, args)
}

func (t *Tracker) Reset() {
	if t == nil {
		return
	}
	t.loadedDirs = map[string]struct{}{
		t.workingDir: {},
	}
}

func (t *Tracker) LoadedDirs() []string {
	if t == nil {
		return nil
	}
	dirs := make([]string, 0, len(t.loadedDirs))
	for dir := range t.loadedDirs {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

func (t *Tracker) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}
	return Snapshot{
		WorkingDir:      t.workingDir,
		LoadedDirs:      t.LoadedDirs(),
		MaxAncestorDirs: t.maxAncestorDirs,
		MaxHintChars:    t.maxHintChars,
		HintFilenames:   append([]string(nil), t.hintFilenames...),
	}
}

func (r Result) Empty() bool {
	return r.Text == "" && len(r.Hints) == 0 && len(r.Warnings) == 0
}

func normalizeOptions(options Options) Options {
	if options.MaxAncestorDirs <= 0 {
		options.MaxAncestorDirs = DefaultMaxAncestorDirs
	}
	if options.MaxHintChars <= 0 {
		options.MaxHintChars = DefaultMaxHintChars
	}
	if len(options.HintFilenames) == 0 {
		options.HintFilenames = defaultHintFilenames
	} else {
		options.HintFilenames = append([]string(nil), options.HintFilenames...)
	}
	return options
}

func resolveWorkingDir(workingDir string) (string, error) {
	if strings.TrimSpace(workingDir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		workingDir = wd
	}

	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve working dir: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working dir is not a directory: %s", workingDir)
	}
	return filepath.Clean(resolved), nil
}

func (t *Tracker) extractDirectories(args map[string]any) []string {
	candidates := newOrderedDirs()
	commandBase := t.commandBaseDir(args)

	for _, key := range pathArgKeys {
		for _, raw := range stringValues(args[key]) {
			kind := pathKindAny
			base := commandBase
			if key == "file_path" {
				kind = pathKindFile
			}
			if key == "workdir" || key == "cwd" || key == "dir" {
				kind = pathKindDir
				base = t.workingDir
			}
			t.addCandidate(raw, base, kind, candidates)
		}
	}

	for _, key := range commandArgKeys {
		for _, command := range stringValues(args[key]) {
			t.extractCommandPaths(command, commandBase, candidates)
		}
	}

	return candidates.list
}

func (t *Tracker) commandBaseDir(args map[string]any) string {
	for _, key := range []string{"workdir", "cwd", "dir"} {
		for _, raw := range stringValues(args[key]) {
			if dir, ok := t.resolveCandidateDir(raw, t.workingDir, pathKindDir); ok {
				return dir
			}
		}
	}
	return t.workingDir
}

func (t *Tracker) extractCommandPaths(command string, baseDir string, candidates *orderedDirs) {
	expectCommand := true
	for _, token := range splitShellLike(command) {
		if isCommandSeparator(token) {
			expectCommand = true
			continue
		}
		if expectCommand {
			expectCommand = false
			if !looksLikeShellPath(token, baseDir) {
				continue
			}
		} else if !looksLikeShellPath(token, baseDir) {
			continue
		}
		t.addCandidate(token, baseDir, pathKindAny, candidates)
	}
}

func (t *Tracker) addCandidate(raw string, baseDir string, kind pathKind, candidates *orderedDirs) {
	dir, ok := t.resolveCandidateDir(raw, baseDir, kind)
	if !ok {
		return
	}

	chain := make([]string, 0, t.maxAncestorDirs)
	for current, depth := dir, 0; depth < t.maxAncestorDirs; depth++ {
		if current == t.workingDir || !t.isWithinWorkingDir(current) || t.isLoaded(current) {
			break
		}
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	for i := len(chain) - 1; i >= 0; i-- {
		candidates.add(chain[i])
	}
}

type pathKind int

const (
	pathKindAny pathKind = iota
	pathKindFile
	pathKindDir
)

func (t *Tracker) resolveCandidateDir(raw string, baseDir string, kind pathKind) (string, bool) {
	raw = cleanRawPath(raw)
	if raw == "" || shouldIgnoreRawPath(raw) {
		return "", false
	}

	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)

	dir := path
	switch kind {
	case pathKindDir:
		dir = path
	case pathKindFile:
		dir = filepath.Dir(path)
	default:
		if info, err := os.Stat(path); err == nil {
			if !info.IsDir() {
				dir = filepath.Dir(path)
			}
		} else if looksLikeFilePath(raw) {
			dir = filepath.Dir(path)
		}
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", false
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", false
	}
	if !t.isWithinWorkingDir(resolved) || t.hasHiddenDir(resolved) {
		return "", false
	}
	return resolved, true
}

func (t *Tracker) loadHintsForDirs(dirs []string) Result {
	var result Result
	for _, dir := range dirs {
		if t.isLoaded(dir) {
			continue
		}
		t.loadedDirs[dir] = struct{}{}
		hint, warnings, ok := t.loadHintForDir(dir)
		result.Warnings = append(result.Warnings, warnings...)
		if ok {
			result.Hints = append(result.Hints, hint)
		}
	}
	result.Text = formatHints(result.Hints)
	return result
}

func (t *Tracker) loadHintForDir(dir string) (Hint, []string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Hint{}, []string{fmt.Sprintf("subdirectory hint dir %s could not be read: %v", t.relPath(dir), err)}, false
	}
	files := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files[entry.Name()] = struct{}{}
		}
	}

	for _, filename := range t.hintFilenames {
		if _, ok := files[filename]; !ok {
			continue
		}

		path := filepath.Join(dir, filename)
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return Hint{}, []string{fmt.Sprintf("subdirectory hint %s could not be read: %v", t.relPath(path), err)}, false
		}
		resolvedPath = filepath.Clean(resolvedPath)
		info, err := os.Stat(resolvedPath)
		if err != nil || info.IsDir() || !t.isWithinWorkingDir(resolvedPath) {
			return Hint{}, nil, false
		}

		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return Hint{}, []string{fmt.Sprintf("subdirectory hint %s could not be read: %v", t.relPath(path), err)}, false
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return Hint{}, nil, false
		}

		truncatedContent, truncated, totalChars := truncateChars(content, t.maxHintChars)
		hint := Hint{
			Dir:       dir,
			Path:      path,
			RelPath:   t.relPath(path),
			Filename:  filename,
			Content:   truncatedContent,
			Truncated: truncated,
		}
		if truncated {
			warning := fmt.Sprintf("subdirectory hint %s truncated to %d chars (%d chars total)", hint.RelPath, t.maxHintChars, totalChars)
			return hint, []string{warning}, true
		}
		return hint, nil, true
	}
	return Hint{}, nil, false
}

func formatHints(hints []Hint) string {
	if len(hints) == 0 {
		return ""
	}
	sections := make([]string, 0, len(hints))
	for _, hint := range hints {
		sections = append(sections, fmt.Sprintf("[Subdirectory context discovered: %s]\n%s", hint.RelPath, hint.Content))
	}
	return strings.Join(sections, "\n\n")
}

func (t *Tracker) isLoaded(dir string) bool {
	_, ok := t.loadedDirs[dir]
	return ok
}

func (t *Tracker) isWithinWorkingDir(path string) bool {
	rel, err := filepath.Rel(t.workingDir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func (t *Tracker) hasHiddenDir(dir string) bool {
	rel, err := filepath.Rel(t.workingDir, dir)
	if err != nil || rel == "." {
		return false
	}
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func (t *Tracker) relPath(path string) string {
	rel, err := filepath.Rel(t.workingDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				values = append(values, str)
			}
		}
		return values
	default:
		return nil
	}
}

func cleanRawPath(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), " \t\r\n;")
}

func shouldIgnoreRawPath(raw string) bool {
	if raw == "" || strings.ContainsRune(raw, 0) {
		return true
	}
	if strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "~") || strings.HasPrefix(raw, "$") {
		return true
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "://") || strings.HasPrefix(raw, "git@")
}

func looksLikeFilePath(raw string) bool {
	base := filepath.Base(raw)
	if base == "." || base == ".." {
		return false
	}
	return filepath.Ext(base) != ""
}

func looksLikeShellPath(token string, baseDir string) bool {
	token = cleanRawPath(token)
	if token == "" || shouldIgnoreRawPath(token) || isAssignment(token) {
		return false
	}
	if strings.Contains(token, "/") || strings.HasPrefix(token, ".") || looksLikeFilePath(token) {
		return true
	}
	_, err := os.Stat(filepath.Join(baseDir, token))
	return err == nil
}

func isAssignment(token string) bool {
	index := strings.IndexRune(token, '=')
	if index <= 0 {
		return false
	}
	name := token[:index]
	return !strings.Contains(name, "/")
}

func splitShellLike(command string) []string {
	var tokens []string
	var builder strings.Builder
	var quote rune
	escaped := false
	runes := []rune(command)

	flush := func() {
		if builder.Len() == 0 {
			return
		}
		tokens = append(tokens, builder.String())
		builder.Reset()
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if escaped {
			builder.WriteRune(r)
			escaped = false
			continue
		}
		if quote != '\'' && r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			builder.WriteRune(r)
			continue
		}

		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t', '\n', '\r':
			flush()
		case ';':
			flush()
			tokens = append(tokens, string(r))
		case '&', '|':
			flush()
			if i+1 < len(runes) && runes[i+1] == r {
				tokens = append(tokens, string([]rune{r, r}))
				i++
			} else {
				tokens = append(tokens, string(r))
			}
		default:
			builder.WriteRune(r)
		}
	}
	if escaped {
		builder.WriteRune('\\')
	}
	flush()
	return tokens
}

func isCommandSeparator(token string) bool {
	switch token {
	case ";", "&&", "||", "|":
		return true
	default:
		return false
	}
}

func truncateChars(value string, maxChars int) (string, bool, int) {
	totalChars := utf8.RuneCountInString(value)
	if maxChars <= 0 || totalChars <= maxChars {
		return value, false, totalChars
	}
	runes := []rune(value)
	return string(runes[:maxChars]), true, len(runes)
}

type orderedDirs struct {
	seen map[string]struct{}
	list []string
}

func newOrderedDirs() *orderedDirs {
	return &orderedDirs{seen: make(map[string]struct{})}
}

func (d *orderedDirs) add(dir string) {
	if _, ok := d.seen[dir]; ok {
		return
	}
	d.seen[dir] = struct{}{}
	d.list = append(d.list, dir)
}
