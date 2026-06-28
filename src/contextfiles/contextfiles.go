package contextfiles

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	noFilesMessage      = "No files in context"
	filesHeader         = "Files in context:"
	outsideCWDPrefix    = "[outside cwd] "
	windowsDriveSepByte = ':'
)

// FileEntry describes a file already known to be in the model context.
// Path is the only required field; metadata is optional and only shown in
// verbose output.
type FileEntry struct {
	Path         string
	Source       string
	Tokens       int
	LastReadTime time.Time
}

// Options controls the rendered files-in-context output.
type Options struct {
	Verbose bool
}

// Format renders the files currently known to be in context.
func Format(files []FileEntry, cwd string, opts Options) string {
	lines := make([]formattedEntry, 0, len(files))
	for _, file := range files {
		display, key := displayPath(cwd, file.Path)
		if display == "" {
			continue
		}
		lines = append(lines, formattedEntry{
			display: display,
			key:     key,
			entry:   file,
		})
	}

	if len(lines) == 0 {
		return noFilesMessage
	}

	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].key == lines[j].key {
			return lines[i].display < lines[j].display
		}
		return lines[i].key < lines[j].key
	})

	var builder strings.Builder
	builder.WriteString(filesHeader)
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if _, ok := seen[line.key]; ok {
			continue
		}
		seen[line.key] = struct{}{}

		builder.WriteByte('\n')
		builder.WriteString(line.display)
		if opts.Verbose {
			builder.WriteString(formatMetadata(line.entry))
		}
	}

	return builder.String()
}

// RelativePath returns a cleaned display path relative to cwd when possible.
// Files outside cwd are marked explicitly. The function only inspects path
// strings and never reads from disk.
func RelativePath(cwd, file string) string {
	display, _ := displayPath(cwd, file)
	return display
}

// SortAndDedupe returns cleaned FileEntry values sorted by path and deduped by
// normalized path identity. Invalid empty or "." paths are omitted.
func SortAndDedupe(files []FileEntry) []FileEntry {
	cleaned := make([]FileEntry, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		info, ok := parsePath(file.Path)
		if !ok {
			continue
		}
		file.Path = info.clean
		key := info.key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, file)
	}

	sort.SliceStable(cleaned, func(i, j int) bool {
		left, _ := parsePath(cleaned[i].Path)
		right, _ := parsePath(cleaned[j].Path)
		return left.key() < right.key()
	})

	return cleaned
}

type formattedEntry struct {
	display string
	key     string
	entry   FileEntry
}

type pathInfo struct {
	clean    string
	volume   string
	parts    []string
	absolute bool
	windows  bool
}

func displayPath(cwd, file string) (string, string) {
	fileInfo, ok := parsePath(file)
	if !ok {
		return "", ""
	}

	cwdInfo, hasCWD := parsePath(cwd)
	windows := fileInfo.windows || cwdInfo.windows

	if fileInfo.absolute && hasCWD && cwdInfo.absolute && sameVolume(cwdInfo, fileInfo) {
		if rel, ok := relativeParts(cwdInfo, fileInfo); ok {
			return rel, displayKey(rel, windows)
		}
	}

	if fileInfo.absolute {
		display := fileInfo.clean
		if hasCWD && cwdInfo.absolute {
			display = outsideCWDPrefix + display
		}
		return display, displayKey(display, windows)
	}

	display := fileInfo.clean
	if escapesCWD(fileInfo) {
		display = outsideCWDPrefix + display
	}
	return display, displayKey(display, windows)
}

func parsePath(value string) (pathInfo, bool) {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if normalized == "" {
		return pathInfo{}, false
	}

	if hasWindowsDrive(normalized) {
		return parseWindowsPath(normalized)
	}

	absolute := strings.HasPrefix(normalized, "/")
	clean := path.Clean(normalized)
	if clean == "." {
		return pathInfo{}, false
	}

	parts := splitParts(clean, absolute)
	return pathInfo{
		clean:    clean,
		parts:    parts,
		absolute: absolute,
	}, true
}

func parseWindowsPath(value string) (pathInfo, bool) {
	volume := strings.ToUpper(value[:2])
	rest := value[2:]
	absolute := strings.HasPrefix(rest, "/")
	rest = strings.TrimLeft(rest, "/")
	cleanRest := path.Clean(rest)
	if cleanRest == "." {
		cleanRest = ""
	}

	clean := volume
	if cleanRest != "" {
		clean = volume + "/" + cleanRest
	} else if absolute {
		clean = volume + "/"
	}
	if clean == volume {
		return pathInfo{}, false
	}

	return pathInfo{
		clean:    clean,
		volume:   volume,
		parts:    splitCleanParts(cleanRest),
		absolute: absolute,
		windows:  true,
	}, true
}

func hasWindowsDrive(value string) bool {
	return len(value) >= 2 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) &&
		value[1] == windowsDriveSepByte
}

func sameVolume(left, right pathInfo) bool {
	if left.windows || right.windows {
		return strings.EqualFold(left.volume, right.volume)
	}
	return left.volume == right.volume
}

func relativeParts(cwd, file pathInfo) (string, bool) {
	if len(file.parts) < len(cwd.parts) {
		return "", false
	}
	for i := range cwd.parts {
		if cwd.windows || file.windows {
			if !strings.EqualFold(cwd.parts[i], file.parts[i]) {
				return "", false
			}
			continue
		}
		if cwd.parts[i] != file.parts[i] {
			return "", false
		}
	}

	relative := file.parts[len(cwd.parts):]
	if len(relative) == 0 {
		return "", false
	}
	return strings.Join(relative, "/"), true
}

func escapesCWD(info pathInfo) bool {
	return len(info.parts) > 0 && info.parts[0] == ".."
}

func splitParts(clean string, absolute bool) []string {
	if absolute {
		clean = strings.TrimPrefix(clean, "/")
	}
	return splitCleanParts(clean)
}

func splitCleanParts(clean string) []string {
	if clean == "" || clean == "." {
		return nil
	}
	return strings.Split(clean, "/")
}

func displayKey(display string, windows bool) string {
	key := display
	if strings.HasPrefix(key, outsideCWDPrefix) {
		key = strings.TrimPrefix(key, outsideCWDPrefix)
	}
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	key = path.Clean(key)
	if windows || hasWindowsDrive(key) {
		key = strings.ToLower(key)
	}
	return key
}

func (info pathInfo) key() string {
	if info.windows {
		return strings.ToLower(info.clean)
	}
	return info.clean
}

func formatMetadata(file FileEntry) string {
	fields := make([]string, 0, 3)
	if source := strings.TrimSpace(file.Source); source != "" {
		fields = append(fields, "source: "+source)
	}
	if file.Tokens > 0 {
		fields = append(fields, fmt.Sprintf("tokens: %d", file.Tokens))
	}
	if !file.LastReadTime.IsZero() {
		fields = append(fields, "last read: "+file.LastReadTime.UTC().Format(time.RFC3339))
	}
	if len(fields) == 0 {
		return ""
	}
	return " (" + strings.Join(fields, ", ") + ")"
}
