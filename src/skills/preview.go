package skills

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxSkillPreviewSize int64 = 2 << 20

var (
	ErrSkillPreviewUnavailable = errors.New("skill preview is unavailable")
	ErrSkillPreviewPathDenied  = errors.New("skill preview path is denied")
	ErrSkillPreviewTooLarge    = errors.New("skill preview is too large")
	ErrSkillPreviewNotUTF8     = errors.New("skill preview is not UTF-8")
)

// SkillPreview is the exact frontmatter block and Markdown body read from an
// installed local SKILL.md. FrontmatterRaw includes its --- delimiters so the
// Desktop can render the complete source without reconstructing metadata.
type SkillPreview struct {
	FrontmatterRaw string
	BodyMarkdown   string
}

// CatalogEntryID returns an opaque, deterministic identity for one concrete
// previewable local Catalog entry. Dynamic MCP Skills do not expose a local
// preview identity because they have no on-disk SKILL.md. The value deliberately
// hashes host-owned source/path facts so callers cannot use it as a filesystem
// capability.
func CatalogEntryID(def *SkillDefinition) string {
	if def == nil || def.Source == SourceMcp || strings.TrimSpace(def.SkillFilePath) == "" {
		return ""
	}
	identity := strings.Join([]string{
		string(def.Source),
		strings.TrimSpace(def.ID),
		filepath.Clean(strings.TrimSpace(def.SourceRoot)),
		filepath.Clean(strings.TrimSpace(def.ResolvedRootDir)),
		filepath.Clean(strings.TrimSpace(def.SkillFilePath)),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%x", sum)
}

// ReadSkillPreview revalidates the discovery anchors and reads one bounded,
// regular UTF-8 SKILL.md without executing package code or expanding runtime
// placeholders.
func ReadSkillPreview(def *SkillDefinition) (SkillPreview, error) {
	if def == nil || def.Source == SourceMcp {
		return SkillPreview{}, fmt.Errorf("%w: no local SKILL.md", ErrSkillPreviewUnavailable)
	}
	anchor := strings.TrimSpace(def.SourceRoot)
	rootDir := strings.TrimSpace(def.RootDir)
	skillPath := strings.TrimSpace(def.SkillFilePath)
	trustedRoot := strings.TrimSpace(def.ResolvedRootDir)
	if anchor == "" || rootDir == "" || skillPath == "" || trustedRoot == "" {
		return SkillPreview{}, fmt.Errorf("%w: incomplete discovery identity", ErrSkillPreviewUnavailable)
	}

	resolvedAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return SkillPreview{}, skillPreviewResolveError("source root", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return SkillPreview{}, skillPreviewResolveError("package root", err)
	}
	if filepath.Clean(resolvedRoot) != filepath.Clean(trustedRoot) {
		return SkillPreview{}, fmt.Errorf("%w: package root changed after discovery", ErrSkillPreviewPathDenied)
	}
	if !pathWithinRoot(resolvedRoot, resolvedAnchor) {
		return SkillPreview{}, fmt.Errorf("%w: package root escapes source root", ErrSkillPreviewPathDenied)
	}

	resolvedSkillPath, err := filepath.EvalSymlinks(skillPath)
	if err != nil {
		return SkillPreview{}, skillPreviewResolveError("SKILL.md", err)
	}
	if !pathWithinRoot(resolvedSkillPath, resolvedAnchor) {
		return SkillPreview{}, fmt.Errorf("%w: SKILL.md escapes source root", ErrSkillPreviewPathDenied)
	}
	flatLegacy := def.Source == SourceLegacy && filepath.Clean(resolvedRoot) == filepath.Clean(resolvedAnchor)
	if flatLegacy {
		rel, relErr := filepath.Rel(resolvedAnchor, resolvedSkillPath)
		if relErr != nil || filepath.IsAbs(rel) || rel == "." || strings.Contains(filepath.ToSlash(rel), "/") {
			return SkillPreview{}, fmt.Errorf("%w: legacy entrypoint is not a direct source child", ErrSkillPreviewPathDenied)
		}
	} else {
		if !pathWithinRoot(resolvedSkillPath, resolvedRoot) {
			return SkillPreview{}, fmt.Errorf("%w: SKILL.md escapes package root", ErrSkillPreviewPathDenied)
		}
		packageRel, relErr := filepath.Rel(resolvedAnchor, resolvedRoot)
		if relErr != nil || filepath.IsAbs(packageRel) || packageRel == "." || strings.Contains(filepath.ToSlash(packageRel), "/") {
			return SkillPreview{}, fmt.Errorf("%w: package is not one directory below its source root", ErrSkillPreviewPathDenied)
		}
	}

	entryRel, err := filepath.Rel(resolvedAnchor, resolvedSkillPath)
	if err != nil || filepath.IsAbs(entryRel) || entryRel == "." || entryRel == ".." || strings.HasPrefix(entryRel, ".."+string(filepath.Separator)) {
		return SkillPreview{}, fmt.Errorf("%w: invalid anchored SKILL.md path", ErrSkillPreviewPathDenied)
	}
	root, err := os.OpenRoot(resolvedAnchor)
	if err != nil {
		return SkillPreview{}, fmt.Errorf("%w: open source root: %v", ErrSkillPreviewPathDenied, err)
	}
	defer root.Close()
	file, err := root.Open(entryRel)
	if err != nil {
		return SkillPreview{}, fmt.Errorf("%w: open SKILL.md: %v", ErrSkillPreviewUnavailable, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return SkillPreview{}, fmt.Errorf("%w: stat SKILL.md: %v", ErrSkillPreviewUnavailable, err)
	}
	if !info.Mode().IsRegular() {
		return SkillPreview{}, fmt.Errorf("%w: SKILL.md is not a regular file", ErrSkillPreviewPathDenied)
	}
	if info.Size() > MaxSkillPreviewSize {
		return SkillPreview{}, fmt.Errorf("%w: SKILL.md exceeds %d bytes", ErrSkillPreviewTooLarge, MaxSkillPreviewSize)
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxSkillPreviewSize+1))
	if err != nil {
		return SkillPreview{}, fmt.Errorf("%w: read SKILL.md: %v", ErrSkillPreviewUnavailable, err)
	}
	if int64(len(raw)) > MaxSkillPreviewSize {
		return SkillPreview{}, fmt.Errorf("%w: SKILL.md exceeds %d bytes", ErrSkillPreviewTooLarge, MaxSkillPreviewSize)
	}
	if !utf8.Valid(raw) {
		return SkillPreview{}, ErrSkillPreviewNotUTF8
	}
	frontmatter, body := SplitSkillPreview(string(raw))
	return SkillPreview{FrontmatterRaw: frontmatter, BodyMarkdown: body}, nil
}

func skillPreviewResolveError(label string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: resolve %s: %v", ErrSkillPreviewUnavailable, label, err)
	}
	return fmt.Errorf("%w: resolve %s: %v", ErrSkillPreviewPathDenied, label, err)
}

// SplitSkillPreview preserves the original frontmatter bytes (including the
// delimiter lines) while returning the remaining Markdown body verbatim. Flat
// legacy Markdown without frontmatter is returned entirely as body content.
func SplitSkillPreview(raw string) (frontmatterRaw, bodyMarkdown string) {
	type sourceLine struct {
		start int
		end   int
		next  int
	}
	lines := make([]sourceLine, 0, 16)
	for start := 0; start < len(raw); {
		newline := strings.IndexByte(raw[start:], '\n')
		if newline < 0 {
			lines = append(lines, sourceLine{start: start, end: len(raw), next: len(raw)})
			break
		}
		end := start + newline
		lines = append(lines, sourceLine{start: start, end: end, next: end + 1})
		start = end + 1
	}
	if len(raw) == 0 {
		return "", ""
	}
	if len(lines) == 0 {
		lines = append(lines, sourceLine{start: 0, end: len(raw), next: len(raw)})
	}

	first := -1
	for index, line := range lines {
		text := strings.TrimSuffix(raw[line.start:line.end], "\r")
		if index == 0 {
			text = strings.TrimPrefix(text, "\ufeff")
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		if strings.TrimSpace(text) != "---" {
			return "", raw
		}
		first = index
		break
	}
	if first < 0 {
		return "", raw
	}
	for index := first + 1; index < len(lines); index++ {
		line := lines[index]
		text := strings.TrimSuffix(raw[line.start:line.end], "\r")
		if strings.TrimSpace(text) != "---" {
			continue
		}
		return raw[lines[first].start:line.end], raw[line.next:]
	}
	return "", raw
}
