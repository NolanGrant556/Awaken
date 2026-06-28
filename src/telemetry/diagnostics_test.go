package telemetry

import (
	"strings"
	"testing"
)

func diag(message string, severity Severity, line int) Diagnostic {
	return Diagnostic{
		Message:  message,
		Severity: severity,
		Range: Range{
			Start: Position{Line: line, Character: 2},
			End:   Position{Line: line, Character: 8},
		},
		Source: "gopls",
		Code:   "compile",
	}
}

func TestTrackerReturnsOnlyDiagnosticsAddedAfterBaseline(t *testing.T) {
	tracker := NewTracker()
	existing := diag("existing", SeverityWarning, 1)
	added := diag("added", SeverityError, 2)

	tracker.CaptureBaseline([]DiagnosticFile{{
		URI:         "file:///workspace/main.go",
		Diagnostics: []Diagnostic{existing},
	}})

	newDiagnostics := tracker.NewDiagnostics([]DiagnosticFile{{
		URI:         "file:///workspace/main.go",
		Diagnostics: []Diagnostic{existing, added},
	}})

	if len(newDiagnostics) != 1 {
		t.Fatalf("got %d diagnostic files, want 1: %#v", len(newDiagnostics), newDiagnostics)
	}
	if len(newDiagnostics[0].Diagnostics) != 1 || !DiagnosticEqual(newDiagnostics[0].Diagnostics[0], added) {
		t.Fatalf("new diagnostics mismatch: %#v", newDiagnostics[0].Diagnostics)
	}

	if again := tracker.NewDiagnostics([]DiagnosticFile{{
		URI:         "file:///workspace/main.go",
		Diagnostics: []Diagnostic{existing, added},
	}}); len(again) != 0 {
		t.Fatalf("baseline was not updated after diff: %#v", again)
	}
}

func TestNormalizeDiagnosticURI(t *testing.T) {
	tests := map[string]string{
		"file:///Users/no%20lan/project/main.go":                  "/Users/no lan/project/main.go",
		"_claude_fs_right:file:///Users/nolan/project/../main.go": "/Users/nolan/main.go",
		"_awaken_fs_left:/tmp/workspace\\pkg\\file.go":            "/tmp/workspace/pkg/file.go",
		"file:///C:/Users/Nolan/Project/File.go":                  "c:/users/nolan/project/file.go",
	}

	for input, want := range tests {
		if got := NormalizeDiagnosticURI(input); got != want {
			t.Fatalf("NormalizeDiagnosticURI(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTrackerUsesChangedRightFileDiagnostics(t *testing.T) {
	tracker := NewTracker()
	existing := diag("existing", SeverityWarning, 1)
	rightAdded := diag("right added", SeverityError, 5)
	staleFileAdded := diag("stale file added", SeverityError, 7)

	tracker.CaptureBaseline([]DiagnosticFile{{
		URI:         "file:///workspace/main.go",
		Diagnostics: []Diagnostic{existing},
	}})

	newDiagnostics := tracker.NewDiagnostics([]DiagnosticFile{
		{
			URI:         "file:///workspace/main.go",
			Diagnostics: []Diagnostic{existing, staleFileAdded},
		},
		{
			URI:         "_claude_fs_right:file:///workspace/main.go",
			Diagnostics: []Diagnostic{existing, rightAdded},
		},
	})

	if len(newDiagnostics) != 1 {
		t.Fatalf("got %d diagnostic files, want 1: %#v", len(newDiagnostics), newDiagnostics)
	}
	if newDiagnostics[0].URI != "file:///workspace/main.go" {
		t.Fatalf("output URI = %q, want primary file URI", newDiagnostics[0].URI)
	}
	if len(newDiagnostics[0].Diagnostics) != 1 || !DiagnosticEqual(newDiagnostics[0].Diagnostics[0], rightAdded) {
		t.Fatalf("right-file diagnostic was not used: %#v", newDiagnostics[0].Diagnostics)
	}
}

func TestDiagnosticsEqualIgnoresOrder(t *testing.T) {
	a := []Diagnostic{
		diag("one", SeverityError, 1),
		diag("two", SeverityWarning, 2),
	}
	b := []Diagnostic{
		diag("two", SeverityWarning, 2),
		diag("one", SeverityError, 1),
	}

	if !DiagnosticsEqual(a, b) {
		t.Fatal("DiagnosticsEqual returned false for same diagnostics in different order")
	}
}

func TestFormatSummaryTruncates(t *testing.T) {
	files := []DiagnosticFile{{
		URI: "file:///workspace/main.go",
		Diagnostics: []Diagnostic{{
			Message:  strings.Repeat("a", 100),
			Severity: SeverityError,
			Range:    Range{Start: Position{Line: 0, Character: 0}},
			Source:   "gopls",
			Code:     "compile",
		}},
	}}

	summary := FormatSummaryWithLimit(files, 60)
	if len(summary) != 60 {
		t.Fatalf("summary length = %d, want 60: %q", len(summary), summary)
	}
	if !strings.HasSuffix(summary, truncationMarker) {
		t.Fatalf("summary did not end with truncation marker: %q", summary)
	}
	if !strings.Contains(summary, "main.go") {
		t.Fatalf("summary should include filename before truncation: %q", summary)
	}
}
