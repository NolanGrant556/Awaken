package toolsummary

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeEmptyBatch(t *testing.T) {
	got := Summarize(nil, Options{})
	if got.Label != "" {
		t.Fatalf("expected empty label, got %q", got.Label)
	}
	if got.Status != StatusUnknown || got.ToolCount != 0 {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestSummarizeSingleToolCases(t *testing.T) {
	tests := []struct {
		name string
		tool ToolUse
		want string
	}{
		{
			name: "search",
			tool: ToolUse{
				Name:  "grep_search",
				Input: map[string]any{"query": "login", "root": "src/auth"},
			},
			want: "Searched in auth/",
		},
		{
			name: "read",
			tool: ToolUse{
				Name:  "file_read",
				Input: `{"path":"config.json"}`,
			},
			want: "Read config.json",
		},
		{
			name: "failing tests",
			tool: ToolUse{
				Name:   "shell",
				Input:  map[string]string{"command": "go test ./..."},
				Output: map[string]any{"stderr": "FAIL awaken/src/toolsummary"},
				Status: StatusError,
			},
			want: "Ran failing tests",
		},
		{
			name: "patch",
			tool: ToolUse{
				Name: "apply_patch",
				Input: map[string]any{"patch": strings.Join([]string{
					"*** Begin Patch",
					"*** Update File: README.md",
					"@@",
					"-old",
					"+new",
					"*** End Patch",
				}, "\n")},
			},
			want: "Updated README",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize([]ToolUse{tt.tool}, Options{})
			if got.Label != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got.Label)
			}
			if got.Status != StatusSuccess && tt.tool.Status != StatusError {
				t.Fatalf("unexpected status: %+v", got)
			}
		})
	}
}

func TestSummarizeMultipleToolGrouping(t *testing.T) {
	readSummary := Summarize([]ToolUse{
		{Name: "file_read", Input: map[string]any{"path": "go.mod"}},
		{Name: "read_file", Input: map[string]any{"path": "src/tool/tool.go"}},
	}, Options{})
	if readSummary.Label != "Read 2 files" {
		t.Fatalf("expected grouped reads, got %+v", readSummary)
	}

	searchSummary := Summarize([]ToolUse{
		{Name: "grep_search", Input: map[string]any{"query": "token", "root": "src/auth"}},
		{Name: "grep_search", Input: map[string]any{"query": "token", "root": "src/llm"}},
	}, Options{})
	if searchSummary.Label != "Searched 2 locations" {
		t.Fatalf("expected grouped searches, got %+v", searchSummary)
	}

	mixedSummary := Summarize([]ToolUse{
		{Name: "file_read", Input: map[string]any{"path": "go.mod"}},
		{Name: "shell", Input: map[string]any{"command": "git status --short"}},
	}, Options{})
	if mixedSummary.Label != "Used 2 tools" {
		t.Fatalf("expected generic mixed label, got %+v", mixedSummary)
	}
}

func TestSummarizeErrorsAndPartialFailures(t *testing.T) {
	failedRead := Summarize([]ToolUse{{
		Name:   "file_read",
		Input:  map[string]any{"path": "config.json"},
		Status: StatusError,
		Error:  "permission denied",
	}}, Options{})
	if failedRead.Label != "Failed to read config.json" || failedRead.Status != StatusError || failedRead.ErrorCount != 1 {
		t.Fatalf("unexpected failed read summary: %+v", failedRead)
	}

	partialBatch := Summarize([]ToolUse{
		{Name: "file_read", Input: map[string]any{"path": "go.mod"}},
		{Name: "file_read", Input: map[string]any{"path": "missing.json"}, Status: StatusError, Error: "not found"},
	}, Options{})
	if partialBatch.Label != "Read 2 files with 1 error" || partialBatch.Status != StatusPartial || partialBatch.ErrorCount != 1 {
		t.Fatalf("unexpected partial batch summary: %+v", partialBatch)
	}

	partialPatch := Summarize([]ToolUse{{
		Name:   "apply_patch",
		Input:  map[string]any{"path": "README.md"},
		Status: StatusPartial,
	}}, Options{})
	if partialPatch.Label != "Partially updated README" || partialPatch.Status != StatusPartial || partialPatch.PartialCount != 1 {
		t.Fatalf("unexpected partial patch summary: %+v", partialPatch)
	}
}

func TestTruncationAndRedaction(t *testing.T) {
	summary := Summarize([]ToolUse{{
		Name:  "web_search",
		Input: map[string]any{"query": "sk-1234567890abcdef authorization token in a very long deployment configuration path"},
	}}, Options{MaxLabelChars: 36})
	if len(summary.Label) > 36 {
		t.Fatalf("expected truncated label, got %q (%d)", summary.Label, len(summary.Label))
	}
	if strings.Contains(summary.Label, "sk-") {
		t.Fatalf("expected secret redaction in label, got %q", summary.Label)
	}

	rendered := StringifyValue(map[string]any{
		"token": "sk-1234567890abcdef",
		"nested": map[string]any{
			"password": "hunter2",
			"safe":     "value",
		},
	}, Options{MaxValueChars: 200})
	if strings.Contains(rendered, "sk-") || strings.Contains(rendered, "hunter2") {
		t.Fatalf("expected redacted stringification, got %q", rendered)
	}
	if !strings.Contains(rendered, "[REDACTED]") || !strings.Contains(rendered, `"safe":"value"`) {
		t.Fatalf("expected JSON-ish redacted value, got %q", rendered)
	}

	raw := StringifyValue(json.RawMessage(`{"password":"hunter2","safe":"value"}`), Options{MaxValueChars: 200})
	if strings.Contains(raw, "hunter2") || !strings.Contains(raw, "[REDACTED]") {
		t.Fatalf("expected raw JSON secret redaction, got %q", raw)
	}
}

func TestPathAndNounExtraction(t *testing.T) {
	opts := Options{WorkspaceRoot: "/repo"}

	readSummary := Summarize([]ToolUse{{
		Name:  "file_read",
		Input: map[string]any{"path": "/repo/config/settings.json"},
	}}, opts)
	if readSummary.Label != "Read settings.json" {
		t.Fatalf("expected basename from workspace path, got %+v", readSummary)
	}

	createSummary := Summarize([]ToolUse{{
		Name: "apply_patch",
		Input: map[string]any{"patch": strings.Join([]string{
			"*** Begin Patch",
			"*** Add File: src/routes/routes.go",
			"+package routes",
			"*** End Patch",
		}, "\n")},
	}}, opts)
	if createSummary.Label != "Created routes.go" {
		t.Fatalf("expected created noun, got %+v", createSummary)
	}

	searchSummary := Summarize([]ToolUse{{
		Name:  "grep_search",
		Input: map[string]any{"query": "auth", "root": "/repo/src/auth"},
	}}, opts)
	if searchSummary.Label != "Searched in auth/" {
		t.Fatalf("expected directory noun, got %+v", searchSummary)
	}
}

func TestStringifyValueHandlesUnsupportedValues(t *testing.T) {
	rendered := StringifyValue(map[string]any{
		"fn": func() {},
	}, Options{MaxValueChars: 80})
	if rendered == "" {
		t.Fatal("expected fallback string for unsupported value")
	}
}
