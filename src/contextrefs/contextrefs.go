package contextrefs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	KindFile   = "file"
	KindFolder = "folder"
	KindDiff   = "diff"
	KindStaged = "staged"
	KindGit    = "git"
	KindURL    = "url"

	DefaultHardTokenLimit   = 12000
	DefaultFolderEntryLimit = 200
)

type Reference struct {
	Raw       string `json:"raw"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
}

type Block struct {
	Reference       Reference `json:"reference"`
	Title           string    `json:"title"`
	Language        string    `json:"language,omitempty"`
	Content         string    `json:"content"`
	EstimatedTokens int       `json:"estimated_tokens"`
}

type Result struct {
	OriginalMessage string      `json:"original_message"`
	StrippedMessage string      `json:"stripped_message"`
	References      []Reference `json:"references"`
	Warnings        []string    `json:"warnings"`
	Blocks          []Block     `json:"blocks"`
	EstimatedTokens int         `json:"estimated_tokens"`
	Expanded        bool        `json:"expanded"`
	Blocked         bool        `json:"blocked"`
}

type Options struct {
	CWD              string
	AllowedRoot      string
	HardTokenLimit   int
	FolderEntryLimit int
	URLFetcher       URLFetcher
	GitRunner        GitRunner
}

type URLFetcher interface {
	FetchURL(ctx context.Context, rawURL string) (string, error)
}

type URLFetcherFunc func(ctx context.Context, rawURL string) (string, error)

func (f URLFetcherFunc) FetchURL(ctx context.Context, rawURL string) (string, error) {
	return f(ctx, rawURL)
}

type GitRunner interface {
	RunGit(ctx context.Context, cwd string, args ...string) (string, error)
}

type GitRunnerFunc func(ctx context.Context, cwd string, args ...string) (string, error)

func (f GitRunnerFunc) RunGit(ctx context.Context, cwd string, args ...string) (string, error) {
	return f(ctx, cwd, args...)
}

func Parse(message string) []Reference {
	if message == "" {
		return nil
	}

	refs := []Reference{}
	for i := 0; i < len(message); i++ {
		if message[i] != '@' {
			continue
		}
		if i > 0 && blocksReferencePrefix(message[i-1]) {
			continue
		}
		ref, ok := parseAtReference(message, i)
		if !ok {
			continue
		}
		refs = append(refs, ref)
		i = ref.End - 1
	}
	return refs
}

func Resolve(ctx context.Context, message string, opts Options) (Result, error) {
	refs := Parse(message)
	result := Result{
		OriginalMessage: message,
		StrippedMessage: StripReferences(message, refs),
		References:      refs,
	}
	if len(refs) == 0 {
		result.StrippedMessage = message
		return result, nil
	}

	settings, err := normalizeOptions(opts)
	if err != nil {
		return result, err
	}

	blocks := []Block{}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key := expansionKey(ref)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		block, warnings := expandReference(ctx, ref, settings)
		result.Warnings = append(result.Warnings, warnings...)
		if block != nil {
			blocks = append(blocks, *block)
			result.EstimatedTokens += block.EstimatedTokens
		}
	}

	if result.EstimatedTokens > settings.hardTokenLimit {
		result.Blocked = true
		result.Warnings = append(result.Warnings, fmt.Sprintf("@ context injection refused: %d estimated tokens exceeds hard limit %d", result.EstimatedTokens, settings.hardTokenLimit))
		return result, nil
	}

	result.Blocks = blocks
	result.Expanded = len(blocks) > 0
	return result, nil
}

func StripReferences(message string, refs []Reference) string {
	if len(refs) == 0 {
		return message
	}

	var builder strings.Builder
	cursor := 0
	for _, ref := range refs {
		if ref.Start < cursor || ref.End > len(message) {
			continue
		}
		builder.WriteString(message[cursor:ref.Start])
		cursor = ref.End
	}
	builder.WriteString(message[cursor:])

	stripped := multiSpacePattern.ReplaceAllString(builder.String(), " ")
	stripped = spaceBeforePunctuationPattern.ReplaceAllString(stripped, "$1")
	return strings.TrimSpace(stripped)
}

func EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	tokens := (runes + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func (b Block) Render() string {
	if b.Language == "" {
		return fmt.Sprintf("%s\n%s", b.Title, b.Content)
	}
	return fmt.Sprintf("%s\n```%s\n%s\n```", b.Title, b.Language, b.Content)
}

func (r Result) ExpandedMessage() string {
	base := strings.TrimSpace(r.StrippedMessage)
	if r.Blocked || len(r.Blocks) == 0 {
		return base
	}

	rendered := make([]string, 0, len(r.Blocks))
	for _, block := range r.Blocks {
		rendered = append(rendered, block.Render())
	}
	if base == "" {
		return "--- Attached Context ---\n\n" + strings.Join(rendered, "\n\n")
	}
	return base + "\n\n--- Attached Context ---\n\n" + strings.Join(rendered, "\n\n")
}

type resolverSettings struct {
	cwd              string
	allowedRoot      string
	hardTokenLimit   int
	folderEntryLimit int
	urlFetcher       URLFetcher
	gitRunner        GitRunner
}

func normalizeOptions(opts Options) (resolverSettings, error) {
	cwd := strings.TrimSpace(opts.CWD)
	if cwd == "" {
		current, err := os.Getwd()
		if err != nil {
			return resolverSettings{}, err
		}
		cwd = current
	}
	cwd, err := cleanExistingRoot(cwd)
	if err != nil {
		return resolverSettings{}, fmt.Errorf("invalid cwd: %w", err)
	}

	allowedRoot := strings.TrimSpace(opts.AllowedRoot)
	if allowedRoot == "" {
		allowedRoot = cwd
	}
	allowedRoot, err = cleanExistingRoot(allowedRoot)
	if err != nil {
		return resolverSettings{}, fmt.Errorf("invalid allowed root: %w", err)
	}

	hardLimit := opts.HardTokenLimit
	if hardLimit <= 0 {
		hardLimit = DefaultHardTokenLimit
	}

	folderLimit := opts.FolderEntryLimit
	if folderLimit <= 0 {
		folderLimit = DefaultFolderEntryLimit
	}

	gitRunner := opts.GitRunner
	if gitRunner == nil {
		gitRunner = commandGitRunner{}
	}

	return resolverSettings{
		cwd:              cwd,
		allowedRoot:      allowedRoot,
		hardTokenLimit:   hardLimit,
		folderEntryLimit: folderLimit,
		urlFetcher:       opts.URLFetcher,
		gitRunner:        gitRunner,
	}, nil
}

func expandReference(ctx context.Context, ref Reference, settings resolverSettings) (*Block, []string) {
	switch ref.Kind {
	case KindFile:
		return expandFileReference(ref, settings)
	case KindFolder:
		return expandFolderReference(ref, settings)
	case KindDiff:
		return expandGitReference(ctx, ref, settings, []string{"diff"}, "git diff")
	case KindStaged:
		return expandGitReference(ctx, ref, settings, []string{"diff", "--staged"}, "git diff --staged")
	case KindGit:
		count, err := parseGitCount(ref.Target)
		if err != nil {
			return nil, []string{fmt.Sprintf("%s: %v", ref.Raw, err)}
		}
		return expandGitReference(ctx, ref, settings, []string{"log", fmt.Sprintf("-%d", count), "-p"}, fmt.Sprintf("git log -%d -p", count))
	case KindURL:
		return expandURLReference(ctx, ref, settings)
	default:
		return nil, []string{fmt.Sprintf("%s: unsupported reference kind %q", ref.Raw, ref.Kind)}
	}
}

func expandFileReference(ref Reference, settings resolverSettings) (*Block, []string) {
	path, err := resolveReferencePath(ref.Target, settings)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", ref.Raw, err)}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: file not found", ref.Raw)}
	}
	if info.IsDir() {
		return nil, []string{fmt.Sprintf("%s: path is not a file", ref.Raw)}
	}

	binary, err := looksBinary(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: could not inspect file: %v", ref.Raw, err)}
	}
	if binary {
		return nil, []string{fmt.Sprintf("%s: binary files are not supported", ref.Raw)}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: could not read file: %v", ref.Raw, err)}
	}
	content := string(data)
	if ref.LineStart > 0 {
		content = selectLineRange(content, ref.LineStart, ref.LineEnd)
	}
	content = strings.TrimSuffix(content, "\n")

	title := fmt.Sprintf("%s (%s)", ref.Raw, displayPath(path, settings.cwd))
	block := Block{
		Reference:       ref,
		Title:           title,
		Language:        codeFenceLanguage(path),
		Content:         content,
		EstimatedTokens: EstimateTokens(content),
	}
	return &block, nil
}

func expandFolderReference(ref Reference, settings resolverSettings) (*Block, []string) {
	path, err := resolveReferencePath(ref.Target, settings)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", ref.Raw, err)}
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: folder not found", ref.Raw)}
	}
	if !info.IsDir() {
		return nil, []string{fmt.Sprintf("%s: path is not a folder", ref.Raw)}
	}

	listing, err := buildFolderListing(path, settings.cwd, settings.folderEntryLimit)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: could not list folder: %v", ref.Raw, err)}
	}
	block := Block{
		Reference:       ref,
		Title:           fmt.Sprintf("%s (%s)", ref.Raw, displayPath(path, settings.cwd)),
		Language:        "text",
		Content:         listing,
		EstimatedTokens: EstimateTokens(listing),
	}
	return &block, nil
}

func expandGitReference(ctx context.Context, ref Reference, settings resolverSettings, args []string, title string) (*Block, []string) {
	output, err := settings.gitRunner.RunGit(ctx, settings.cwd, args...)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", ref.Raw, err)}
	}
	output = strings.TrimSpace(output)
	if output == "" {
		output = "(no output)"
	}
	block := Block{
		Reference:       ref,
		Title:           fmt.Sprintf("%s (%s)", ref.Raw, title),
		Language:        "diff",
		Content:         output,
		EstimatedTokens: EstimateTokens(output),
	}
	return &block, nil
}

func expandURLReference(ctx context.Context, ref Reference, settings resolverSettings) (*Block, []string) {
	if settings.urlFetcher == nil {
		return nil, []string{fmt.Sprintf("%s: URL fetcher is not configured", ref.Raw)}
	}
	parsed, err := url.Parse(ref.Target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, []string{fmt.Sprintf("%s: invalid URL", ref.Raw)}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, []string{fmt.Sprintf("%s: unsupported URL scheme %q", ref.Raw, parsed.Scheme)}
	}

	content, err := settings.urlFetcher.FetchURL(ctx, ref.Target)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", ref.Raw, err)}
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, []string{fmt.Sprintf("%s: no content returned", ref.Raw)}
	}
	block := Block{
		Reference:       ref,
		Title:           fmt.Sprintf("%s (%s)", ref.Raw, ref.Target),
		Language:        "markdown",
		Content:         content,
		EstimatedTokens: EstimateTokens(content),
	}
	return &block, nil
}

func parseAtReference(message string, start int) (Reference, bool) {
	afterAt := start + 1
	for _, kind := range []string{KindDiff, KindStaged} {
		end := afterAt + len(kind)
		if strings.HasPrefix(message[afterAt:], kind) && isSimpleReferenceBoundary(message, end) {
			return Reference{
				Raw:   message[start:end],
				Kind:  kind,
				Start: start,
				End:   end,
			}, true
		}
	}

	for _, kind := range []string{KindFile, KindFolder, KindGit, KindURL} {
		prefix := kind + ":"
		if !strings.HasPrefix(message[afterAt:], prefix) {
			continue
		}
		valueStart := afterAt + len(prefix)
		valueEnd, ok := scanReferenceValue(message, valueStart)
		if !ok {
			return Reference{}, false
		}
		valueEnd = trimTrailingPunctuationEnd(message, valueStart, valueEnd)
		if valueEnd <= valueStart {
			return Reference{}, false
		}

		value := message[valueStart:valueEnd]
		target := stripReferenceWrappers(value)
		lineStart := 0
		lineEnd := 0
		if kind == KindFile {
			target, lineStart, lineEnd = parseFileValue(value)
		}

		return Reference{
			Raw:       message[start:valueEnd],
			Kind:      kind,
			Target:    target,
			Start:     start,
			End:       valueEnd,
			LineStart: lineStart,
			LineEnd:   lineEnd,
		}, true
	}

	return Reference{}, false
}

func scanReferenceValue(message string, start int) (int, bool) {
	if start >= len(message) {
		return start, false
	}

	if isQuote(message[start]) {
		quote := message[start]
		cursor := start + 1
		for cursor < len(message) {
			if message[cursor] == '\n' || message[cursor] == '\r' {
				return start, false
			}
			if message[cursor] == quote {
				cursor++
				if cursor < len(message) && message[cursor] == ':' {
					if end, ok := scanLineRangeSuffix(message, cursor); ok {
						cursor = end
					}
				}
				return cursor, true
			}
			cursor++
		}
		return start, false
	}

	cursor := start
	for cursor < len(message) {
		r, size := utf8.DecodeRuneInString(message[cursor:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if unicode.IsSpace(r) {
			break
		}
		cursor += size
	}
	return cursor, cursor > start
}

func scanLineRangeSuffix(message string, colon int) (int, bool) {
	cursor := colon + 1
	startDigits := cursor
	for cursor < len(message) && isDigit(message[cursor]) {
		cursor++
	}
	if cursor == startDigits {
		return colon, false
	}
	if cursor < len(message) && message[cursor] == '-' {
		cursor++
		endDigits := cursor
		for cursor < len(message) && isDigit(message[cursor]) {
			cursor++
		}
		if cursor == endDigits {
			return colon, false
		}
	}
	return cursor, true
}

func parseFileValue(value string) (string, int, int) {
	if len(value) >= 2 && isQuote(value[0]) {
		quote := value[0]
		for i := 1; i < len(value); i++ {
			if value[i] != quote {
				continue
			}
			target := value[1:i]
			lineStart, lineEnd := parseLineRange(value[i+1:])
			return target, lineStart, lineEnd
		}
	}

	target, lineStart, lineEnd, ok := splitUnquotedFileRange(value)
	if ok {
		return target, lineStart, lineEnd
	}
	return stripReferenceWrappers(value), 0, 0
}

func splitUnquotedFileRange(value string) (string, int, int, bool) {
	colon := strings.LastIndex(value, ":")
	if colon <= 0 || colon == len(value)-1 {
		return "", 0, 0, false
	}
	lineStart, lineEnd := parseLineRange(value[colon:])
	if lineStart == 0 {
		return "", 0, 0, false
	}
	return value[:colon], lineStart, lineEnd, true
}

func parseLineRange(suffix string) (int, int) {
	if !strings.HasPrefix(suffix, ":") {
		return 0, 0
	}
	rangeText := suffix[1:]
	if rangeText == "" {
		return 0, 0
	}
	parts := strings.Split(rangeText, "-")
	if len(parts) > 2 || parts[0] == "" {
		return 0, 0
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil || start <= 0 {
		return 0, 0
	}
	end := start
	if len(parts) == 2 {
		if parts[1] == "" {
			return 0, 0
		}
		parsedEnd, err := strconv.Atoi(parts[1])
		if err != nil || parsedEnd <= 0 {
			return 0, 0
		}
		end = parsedEnd
	}
	if end < start {
		end = start
	}
	return start, end
}

func trimTrailingPunctuationEnd(message string, start, end int) int {
	for end > start {
		r, size := utf8.DecodeLastRuneInString(message[start:end])
		if strings.ContainsRune(",.;!?", r) {
			end -= size
			continue
		}
		if isUnmatchedCloser(message[start:end], r) {
			end -= size
			continue
		}
		break
	}
	return end
}

func isUnmatchedCloser(value string, r rune) bool {
	var opener rune
	switch r {
	case ')':
		opener = '('
	case ']':
		opener = '['
	case '}':
		opener = '{'
	default:
		return false
	}
	return strings.Count(value, string(r)) > strings.Count(value, string(opener))
}

func stripReferenceWrappers(value string) string {
	if len(value) >= 2 && isQuote(value[0]) && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
}

func isSimpleReferenceBoundary(message string, end int) bool {
	if end >= len(message) {
		return true
	}
	next := message[end]
	return !isASCIIWord(next)
}

func blocksReferencePrefix(b byte) bool {
	return isASCIIWord(b) || b == '/'
}

func isASCIIWord(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isQuote(b byte) bool {
	return b == '`' || b == '"' || b == '\''
}

func resolveReferencePath(target string, settings resolverSettings) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", errors.New("path is empty")
	}

	candidate := expandHome(target)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(settings.cwd, candidate)
	}

	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	cleanCandidate := filepath.Clean(absCandidate)
	resolvedCandidate := cleanCandidate
	if evaluated, err := filepath.EvalSymlinks(cleanCandidate); err == nil {
		resolvedCandidate = evaluated
	}

	if !pathWithin(settings.allowedRoot, resolvedCandidate) {
		return "", errors.New("path is outside the allowed workspace")
	}
	if isSensitivePath(resolvedCandidate, settings.allowedRoot) {
		return "", errors.New("path is a sensitive credential path and cannot be attached")
	}
	return resolvedCandidate, nil
}

func cleanExistingRoot(path string) (string, error) {
	path = expandHome(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
		return evaluated, nil
	}
	return filepath.Clean(abs), nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func isSensitivePath(path, allowedRoot string) bool {
	if rel, err := filepath.Rel(allowedRoot, path); err == nil && matchesSensitiveRel(rel) {
		return true
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && matchesSensitiveRel(rel) {
			return true
		}
	}
	return false
}

func matchesSensitiveRel(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		switch part {
		case ".ssh", ".aws", ".gnupg", ".kube", ".docker", ".azure":
			return true
		case ".netrc", ".npmrc", ".pypirc", ".pgpass", ".env":
			return true
		}
		if part == ".config" && i+1 < len(parts) && parts[i+1] == "gh" {
			return true
		}
	}
	return false
}

func looksBinary(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	sample := buf[:n]
	if bytes.IndexByte(sample, 0) >= 0 {
		return true, nil
	}
	return n > 0 && !utf8.Valid(sample), nil
}

func selectLineRange(text string, start, end int) string {
	if start <= 0 {
		return text
	}
	if end <= 0 || end < start {
		end = start
	}

	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if start > len(lines) {
		return ""
	}
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n")
}

func buildFolderListing(path, cwd string, limit int) (string, error) {
	if limit <= 0 {
		limit = DefaultFolderEntryLimit
	}

	lines := []string{filepath.ToSlash(displayPath(path, cwd)) + "/"}
	count := 0
	errStop := errors.New("folder listing limit reached")
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if current == path {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if count >= limit {
			return errStop
		}

		rel, err := filepath.Rel(path, current)
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/")
		prefix := strings.Repeat("  ", depth)
		line := prefix + "- " + entry.Name()
		if entry.IsDir() {
			line += "/"
		} else {
			line += " (" + fileMetadata(current, entry) + ")"
		}
		lines = append(lines, line)
		count++
		return nil
	})
	if errors.Is(err, errStop) {
		lines = append(lines, "- ...")
		return strings.Join(lines, "\n"), nil
	}
	if err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

func fileMetadata(path string, entry fs.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return "symlink"
	}
	info, err := entry.Info()
	if err != nil {
		return "unknown"
	}
	binary, err := looksBinary(path)
	if err != nil || binary {
		return fmt.Sprintf("%d bytes", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%d bytes", info.Size())
	}
	text := string(data)
	if text == "" {
		return "0 lines"
	}
	lines := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		lines++
	}
	return fmt.Sprintf("%d lines", lines)
}

func displayPath(path, cwd string) string {
	rel, err := filepath.Rel(cwd, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		if rel == "." {
			return filepath.Base(path)
		}
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func codeFenceLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".sh":
		return "bash"
	case ".yml", ".yaml":
		return "yaml"
	case ".toml":
		return "toml"
	default:
		return ""
	}
}

func parseGitCount(target string) (int, error) {
	if strings.TrimSpace(target) == "" {
		return 1, nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(target))
	if err != nil {
		return 0, errors.New("git reference count must be a number")
	}
	if count < 1 {
		count = 1
	}
	if count > 10 {
		count = 10
	}
	return count, nil
}

func expansionKey(ref Reference) string {
	return strings.Join([]string{
		ref.Kind,
		ref.Target,
		strconv.Itoa(ref.LineStart),
		strconv.Itoa(ref.LineEnd),
	}, "\x00")
}

type commandGitRunner struct{}

func (commandGitRunner) RunGit(ctx context.Context, cwd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return stdout.String(), nil
}

var (
	multiSpacePattern             = regexp.MustCompile(`[ \t]{2,}`)
	spaceBeforePunctuationPattern = regexp.MustCompile(`[ \t]+([,.;:!?])`)
)
