package lspdiagnostics

import (
	"strconv"
	"testing"
	"time"
)

func TestMapLSPSeverity(t *testing.T) {
	cases := []struct {
		name     string
		severity *int
		want     Severity
	}{
		{name: "missing defaults to error", severity: nil, want: SeverityError},
		{name: "error", severity: intPtr(1), want: SeverityError},
		{name: "warning", severity: intPtr(2), want: SeverityWarning},
		{name: "info", severity: intPtr(3), want: SeverityInfo},
		{name: "hint", severity: intPtr(4), want: SeverityHint},
		{name: "unknown defaults to error", severity: intPtr(99), want: SeverityError},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapLSPSeverity(tt.severity); got != tt.want {
				t.Fatalf("MapLSPSeverity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestURIToPath(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{name: "file URI", uri: "file:///tmp/example.go", want: "/tmp/example.go"},
		{name: "file URI with escaped space", uri: "file:///tmp/hello%20world.go", want: "/tmp/hello world.go"},
		{name: "localhost file URI", uri: "file://localhost/tmp/example.go", want: "/tmp/example.go"},
		{name: "plain path preserved", uri: "/tmp/example.go", want: "/tmp/example.go"},
		{name: "non local file URI preserved", uri: "file://server/share/example.go", want: "file://server/share/example.go"},
		{name: "invalid URI preserved", uri: "file://%zz", want: "file://%zz"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := URIToPath(tt.uri); got != tt.want {
				t.Fatalf("URIToPath(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestRegistryDrainDedupeWithinBatch(t *testing.T) {
	now := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	nextID := 0
	registry := NewRegistry(
		WithClockFunc(func() time.Time { return now }),
		WithIDGenerator(func() string {
			nextID++
			return "id-" + strconv.Itoa(nextID)
		}),
	)

	id, ok := registry.Register("gopls", PublishDiagnostics{
		URI: "file:///tmp/a.go",
		Diagnostics: []LSPDiagnostic{
			lspDiagnostic("same", 1, 1, "go", "E1"),
			lspDiagnostic("same", 1, 1, "go", "E1"),
			lspDiagnostic("same", 1, 1, "go", "E2"),
		},
	})
	if !ok || id != "id-1" {
		t.Fatalf("Register() = %q, %v; want id-1, true", id, ok)
	}
	if _, ok := registry.Register("tsserver", PublishDiagnostics{
		URI: "file:///tmp/a.go",
		Diagnostics: []LSPDiagnostic{
			lspDiagnostic("same", 1, 1, "go", "E1"),
			lspDiagnostic("new warning", 2, 2, "go", "W1"),
		},
	}); !ok {
		t.Fatal("expected second registration")
	}
	if !registry.CheckPending() {
		t.Fatal("expected pending diagnostics before drain")
	}

	batches := registry.DrainPending()
	if len(batches) != 1 {
		t.Fatalf("expected one batch, got %+v", batches)
	}
	if batches[0].ServerName != "gopls, tsserver" {
		t.Fatalf("unexpected server name %q", batches[0].ServerName)
	}
	if !batches[0].Timestamp.Equal(now) {
		t.Fatalf("unexpected timestamp %s", batches[0].Timestamp)
	}
	if len(batches[0].Files) != 1 {
		t.Fatalf("expected one file, got %+v", batches[0].Files)
	}
	file := batches[0].Files[0]
	if file.URI != "/tmp/a.go" {
		t.Fatalf("expected normalized file URI, got %q", file.URI)
	}
	if len(file.Diagnostics) != 3 {
		t.Fatalf("expected 3 diagnostics after dedupe, got %+v", file.Diagnostics)
	}
	assertDiagnosticPresent(t, file.Diagnostics, "same", "E1")
	assertDiagnosticPresent(t, file.Diagnostics, "same", "E2")
	assertDiagnosticPresent(t, file.Diagnostics, "new warning", "W1")
	if registry.CheckPending() {
		t.Fatal("expected pending diagnostics to be drained")
	}
	if batches := registry.DrainPending(); len(batches) != 0 {
		t.Fatalf("second drain should be empty, got %+v", batches)
	}
}

func TestRegistryCrossTurnDedupe(t *testing.T) {
	registry := NewRegistry()
	params := PublishDiagnostics{
		URI:         "file:///tmp/a.go",
		Diagnostics: []LSPDiagnostic{lspDiagnostic("same", 1, 1, "go", "E1")},
	}

	registry.Register("gopls", params)
	if got := diagnosticCount(registry.DrainPending()); got != 1 {
		t.Fatalf("first drain count = %d, want 1", got)
	}

	registry.Register("gopls", params)
	if got := diagnosticCount(registry.DrainPending()); got != 0 {
		t.Fatalf("same diagnostic should be filtered across turns, got %d", got)
	}

	registry.Register("gopls", PublishDiagnostics{
		URI:         "file:///tmp/a.go",
		Diagnostics: []LSPDiagnostic{lspDiagnostic("changed", 1, 1, "go", "E1")},
	})
	if got := diagnosticCount(registry.DrainPending()); got != 1 {
		t.Fatalf("changed diagnostic count = %d, want 1", got)
	}
}

func TestRegistryDeliveredLRUEvictsOldFiles(t *testing.T) {
	registry := NewRegistry(WithMaxDeliveredFiles(1))
	fileA := PublishDiagnostics{
		URI:         "file:///tmp/a.go",
		Diagnostics: []LSPDiagnostic{lspDiagnostic("same", 1, 1, "go", "E1")},
	}
	fileB := PublishDiagnostics{
		URI:         "file:///tmp/b.go",
		Diagnostics: []LSPDiagnostic{lspDiagnostic("other", 1, 1, "go", "E1")},
	}

	registry.Register("gopls", fileA)
	if got := diagnosticCount(registry.DrainPending()); got != 1 {
		t.Fatalf("file A first drain count = %d, want 1", got)
	}
	registry.Register("gopls", fileB)
	if got := diagnosticCount(registry.DrainPending()); got != 1 {
		t.Fatalf("file B drain count = %d, want 1", got)
	}
	registry.Register("gopls", fileA)
	if got := diagnosticCount(registry.DrainPending()); got != 1 {
		t.Fatalf("file A should be delivered after LRU eviction, got %d", got)
	}
}

func TestRegistryCapsDiagnosticsAndPrioritizesSeverity(t *testing.T) {
	registry := NewRegistry(
		WithMaxDiagnosticsPerFile(2),
		WithMaxTotalDiagnostics(3),
	)

	registry.Register("gopls", PublishDiagnostics{
		URI: "file:///tmp/a.go",
		Diagnostics: []LSPDiagnostic{
			lspDiagnostic("hint a", 4, 4, "go", "H"),
			lspDiagnostic("warning a", 2, 2, "go", "W"),
			lspDiagnostic("error a", 1, 1, "go", "E"),
		},
	})
	registry.Register("gopls", PublishDiagnostics{
		URI: "file:///tmp/b.go",
		Diagnostics: []LSPDiagnostic{
			lspDiagnostic("hint b", 4, 4, "go", "H"),
			lspDiagnostic("error b", 1, 1, "go", "E"),
		},
	})

	batches := registry.DrainPending()
	if len(batches) != 1 || len(batches[0].Files) != 2 {
		t.Fatalf("unexpected batches: %+v", batches)
	}

	fileA := batches[0].Files[0]
	if fileA.URI != "/tmp/a.go" {
		t.Fatalf("unexpected first file %q", fileA.URI)
	}
	if gotMessages(fileA.Diagnostics) != "error a,warning a" {
		t.Fatalf("unexpected file A diagnostics: %+v", fileA.Diagnostics)
	}

	fileB := batches[0].Files[1]
	if fileB.URI != "/tmp/b.go" {
		t.Fatalf("unexpected second file %q", fileB.URI)
	}
	if gotMessages(fileB.Diagnostics) != "error b" {
		t.Fatalf("unexpected file B diagnostics: %+v", fileB.Diagnostics)
	}
}

func TestRegisterPayloadRejectsInvalidPayloadsWithoutPanic(t *testing.T) {
	registry := NewRegistry()
	payloads := []any{
		nil,
		"{",
		map[string]any{"uri": "/tmp/a.go"},
		map[string]any{"uri": "/tmp/a.go", "diagnostics": "not an array"},
		map[string]any{"diagnostics": []any{}},
	}

	for _, payload := range payloads {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("RegisterPayload panicked for %#v: %v", payload, recovered)
				}
			}()
			if id, ok := registry.RegisterPayload("bad-server", payload); ok || id != "" {
				t.Fatalf("RegisterPayload(%#v) = %q, %v; want empty, false", payload, id, ok)
			}
		}()
	}
	if registry.CheckPending() {
		t.Fatal("invalid payloads should not leave pending diagnostics")
	}

	id, ok := registry.RegisterPayload("gopls", map[string]any{
		"uri": "file:///tmp/a.go",
		"diagnostics": []any{map[string]any{
			"message":  "from raw payload",
			"severity": 2,
			"source":   "go",
			"code":     123,
			"range": map[string]any{
				"start": map[string]any{"line": 1, "character": 2},
				"end":   map[string]any{"line": 1, "character": 5},
			},
		}},
	})
	if !ok || id == "" {
		t.Fatalf("expected valid raw payload to register, got %q, %v", id, ok)
	}

	batches := registry.DrainPending()
	if len(batches) != 1 || len(batches[0].Files) != 1 || len(batches[0].Files[0].Diagnostics) != 1 {
		t.Fatalf("unexpected batches from raw payload: %+v", batches)
	}
	diagnostic := batches[0].Files[0].Diagnostics[0]
	if diagnostic.Severity != SeverityWarning || diagnostic.Code != "123" || diagnostic.Message != "from raw payload" {
		t.Fatalf("unexpected diagnostic from raw payload: %+v", diagnostic)
	}
}

func intPtr(value int) *int {
	return &value
}

func lspDiagnostic(message string, severity int, line int, source string, code any) LSPDiagnostic {
	return LSPDiagnostic{
		Message:  message,
		Severity: intPtr(severity),
		Source:   source,
		Code:     code,
		Range: Range{
			Start: Position{Line: line, Character: 1},
			End:   Position{Line: line, Character: 5},
		},
	}
}

func diagnosticCount(batches []DiagnosticBatch) int {
	count := 0
	for _, batch := range batches {
		for _, file := range batch.Files {
			count += len(file.Diagnostics)
		}
	}
	return count
}

func assertDiagnosticPresent(t *testing.T, diagnostics []Diagnostic, message, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Message == message && diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("expected diagnostic %q/%q in %+v", message, code, diagnostics)
}

func gotMessages(diagnostics []Diagnostic) string {
	messages := ""
	for i, diagnostic := range diagnostics {
		if i > 0 {
			messages += ","
		}
		messages += diagnostic.Message
	}
	return messages
}
