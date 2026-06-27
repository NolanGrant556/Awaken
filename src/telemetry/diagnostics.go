package telemetry

import (
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

const (
	MaxDiagnosticsSummaryChars = 4000
	truncationMarker           = "...[truncated]"
)

type Severity string

const (
	SeverityError   Severity = "Error"
	SeverityWarning Severity = "Warning"
	SeverityInfo    Severity = "Info"
	SeverityHint    Severity = "Hint"
)

type Position struct {
	Line      int
	Character int
}

type Range struct {
	Start Position
	End   Position
}

type Diagnostic struct {
	Message  string
	Severity Severity
	Range    Range
	Source   string
	Code     string
}

type DiagnosticFile struct {
	URI         string
	Diagnostics []Diagnostic
}

type Tracker struct {
	baseline                  map[string][]Diagnostic
	rightFileDiagnosticsState map[string][]Diagnostic
}

func NewTracker() *Tracker {
	return &Tracker{
		baseline:                  map[string][]Diagnostic{},
		rightFileDiagnosticsState: map[string][]Diagnostic{},
	}
}

func (t *Tracker) CaptureBaseline(files []DiagnosticFile) {
	if t == nil {
		return
	}
	if t.baseline == nil {
		t.baseline = map[string][]Diagnostic{}
	}
	if t.rightFileDiagnosticsState == nil {
		t.rightFileDiagnosticsState = map[string][]Diagnostic{}
	}

	for _, file := range files {
		normalized := NormalizeDiagnosticURI(file.URI)
		if normalized == "" {
			continue
		}
		t.baseline[normalized] = cloneDiagnostics(file.Diagnostics)
	}
}

func (t *Tracker) NewDiagnostics(files []DiagnosticFile) []DiagnosticFile {
	if t == nil || len(t.baseline) == 0 {
		return nil
	}
	if t.rightFileDiagnosticsState == nil {
		t.rightFileDiagnosticsState = map[string][]Diagnostic{}
	}

	primaryFiles := map[string]DiagnosticFile{}
	rightFiles := map[string]DiagnosticFile{}
	for _, file := range files {
		normalized := NormalizeDiagnosticURI(file.URI)
		if normalized == "" {
			continue
		}
		if isRightFileURI(file.URI) {
			rightFiles[normalized] = file
			continue
		}
		if _, exists := primaryFiles[normalized]; !exists || isFileURI(file.URI) {
			primaryFiles[normalized] = file
		}
	}

	keys := make([]string, 0, len(t.baseline))
	for key := range t.baseline {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var result []DiagnosticFile
	for _, key := range keys {
		baselineDiagnostics := t.baseline[key]
		primary, hasPrimary := primaryFiles[key]
		right, hasRight := rightFiles[key]
		if !hasPrimary && !hasRight {
			continue
		}

		fileToUse := primary
		if !hasPrimary {
			fileToUse = right
		}

		if hasRight {
			previousRightDiagnostics, seenRight := t.rightFileDiagnosticsState[key]
			if !seenRight || !DiagnosticsEqual(previousRightDiagnostics, right.Diagnostics) {
				fileToUse = right
			}
			t.rightFileDiagnosticsState[key] = cloneDiagnostics(right.Diagnostics)
		}

		newDiagnostics := diagnosticsDifference(fileToUse.Diagnostics, baselineDiagnostics)
		if len(newDiagnostics) > 0 {
			outputURI := fileToUse.URI
			if hasPrimary {
				outputURI = primary.URI
			}
			result = append(result, DiagnosticFile{
				URI:         outputURI,
				Diagnostics: newDiagnostics,
			})
		}

		t.baseline[key] = cloneDiagnostics(fileToUse.Diagnostics)
	}

	return result
}

func NormalizeDiagnosticURI(uri string) string {
	normalized := strings.TrimSpace(uri)
	if normalized == "" {
		return ""
	}

	for _, prefix := range []string{
		"_awaken_fs_right:",
		"_awaken_fs_left:",
		"_claude_fs_right:",
		"_claude_fs_left:",
	} {
		if strings.HasPrefix(normalized, prefix) {
			normalized = strings.TrimPrefix(normalized, prefix)
			break
		}
	}

	if strings.HasPrefix(normalized, "file://") {
		if parsed, err := url.Parse(normalized); err == nil && parsed.Path != "" {
			normalized = parsed.Path
		} else {
			normalized = strings.TrimPrefix(normalized, "file://")
		}
	}

	if unescaped, err := url.PathUnescape(normalized); err == nil {
		normalized = unescaped
	}

	normalized = strings.ReplaceAll(normalized, "\\", "/")
	normalized = path.Clean(normalized)
	if normalized == "." {
		return ""
	}
	if len(normalized) >= 3 && normalized[0] == '/' && hasWindowsDrive(normalized[1:]) {
		normalized = normalized[1:]
	}

	if hasWindowsDrive(normalized) {
		normalized = strings.ToLower(normalized)
	}

	return normalized
}

func DiagnosticEqual(a, b Diagnostic) bool {
	return a.Message == b.Message &&
		a.Severity == b.Severity &&
		a.Source == b.Source &&
		a.Code == b.Code &&
		a.Range.Start.Line == b.Range.Start.Line &&
		a.Range.Start.Character == b.Range.Start.Character &&
		a.Range.End.Line == b.Range.End.Line &&
		a.Range.End.Character == b.Range.End.Character
}

func DiagnosticsEqual(a, b []Diagnostic) bool {
	if len(a) != len(b) {
		return false
	}

	used := make([]bool, len(b))
	for _, left := range a {
		found := false
		for index, right := range b {
			if used[index] || !DiagnosticEqual(left, right) {
				continue
			}
			used[index] = true
			found = true
			break
		}
		if !found {
			return false
		}
	}

	return true
}

func FormatSummary(files []DiagnosticFile) string {
	return FormatSummaryWithLimit(files, MaxDiagnosticsSummaryChars)
}

func FormatSummaryWithLimit(files []DiagnosticFile, limit int) string {
	var builder strings.Builder
	for fileIndex, file := range files {
		if fileIndex > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(diagnosticFilename(file.URI))
		builder.WriteString(":\n")
		for diagnosticIndex, diagnostic := range file.Diagnostics {
			if diagnosticIndex > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString("  ")
			builder.WriteString(severitySymbol(diagnostic.Severity))
			builder.WriteString(" [Line ")
			builder.WriteString(strconv.Itoa(diagnostic.Range.Start.Line + 1))
			builder.WriteByte(':')
			builder.WriteString(strconv.Itoa(diagnostic.Range.Start.Character + 1))
			builder.WriteString("] ")
			builder.WriteString(diagnostic.Message)
			if diagnostic.Code != "" {
				builder.WriteString(" [")
				builder.WriteString(diagnostic.Code)
				builder.WriteByte(']')
			}
			if diagnostic.Source != "" {
				builder.WriteString(" (")
				builder.WriteString(diagnostic.Source)
				builder.WriteByte(')')
			}
		}
	}

	summary := builder.String()
	if limit <= 0 || len(summary) <= limit {
		return summary
	}
	if limit <= len(truncationMarker) {
		return truncationMarker[:limit]
	}
	return summary[:limit-len(truncationMarker)] + truncationMarker
}

func diagnosticsDifference(current, baseline []Diagnostic) []Diagnostic {
	var result []Diagnostic
	for _, diagnostic := range current {
		if containsDiagnostic(baseline, diagnostic) {
			continue
		}
		result = append(result, diagnostic)
	}
	return result
}

func containsDiagnostic(diagnostics []Diagnostic, target Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if DiagnosticEqual(diagnostic, target) {
			return true
		}
	}
	return false
}

func cloneDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	return append([]Diagnostic(nil), diagnostics...)
}

func isFileURI(uri string) bool {
	return strings.HasPrefix(uri, "file://")
}

func isRightFileURI(uri string) bool {
	return strings.HasPrefix(uri, "_awaken_fs_right:") ||
		strings.HasPrefix(uri, "_claude_fs_right:")
}

func diagnosticFilename(uri string) string {
	normalized := NormalizeDiagnosticURI(uri)
	if normalized == "" {
		return uri
	}
	return path.Base(normalized)
}

func severitySymbol(severity Severity) string {
	switch severity {
	case SeverityError:
		return "x"
	case SeverityWarning:
		return "!"
	case SeverityInfo:
		return "i"
	case SeverityHint:
		return "*"
	default:
		return "-"
	}
}

func hasWindowsDrive(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	first := value[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
}
