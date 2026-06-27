package notebookedit

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateRequestRejectsInvalidJSON(t *testing.T) {
	req := Request{NotebookPath: "bad.ipynb", CellID: "cell-0"}
	err := ValidateRequest([]byte(`{"cells": [`), req)
	if err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
}

func TestApplyReplacesByCellIndex(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		codeCell("alpha", "print(1)", 1, []any{"old"}),
		markdownCell("beta", "old text"),
	}, 5)
	req := Request{
		NotebookPath: "book.ipynb",
		CellID:       "cell-1",
		NewSource:    "new text",
	}

	result, err := Apply(content, req, fixedID("unused"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.CellID != "beta" {
		t.Fatalf("expected actual cell id beta, got %q", result.CellID)
	}
	if result.EditMode != EditModeReplace {
		t.Fatalf("expected replace mode, got %q", result.EditMode)
	}

	updated := decodeNotebook(t, result.UpdatedJSON)
	cells := updated["cells"].([]any)
	cell := cells[1].(map[string]any)
	if cell["source"] != "new text" {
		t.Fatalf("expected source replacement, got %#v", cell["source"])
	}
}

func TestApplyReplaceCodeResetsOutputs(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		codeCell("alpha", "print(1)", 7, []any{map[string]any{"output_type": "stream", "text": "old"}}),
	}, 5)
	req := Request{
		NotebookPath: "book.ipynb",
		CellID:       "alpha",
		NewSource:    "print(2)",
	}

	result, err := Apply(content, req, fixedID("unused"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	updated := decodeNotebook(t, result.UpdatedJSON)
	cell := updated["cells"].([]any)[0].(map[string]any)
	if cell["execution_count"] != nil {
		t.Fatalf("expected execution_count reset to null, got %#v", cell["execution_count"])
	}
	outputs := cell["outputs"].([]any)
	if len(outputs) != 0 {
		t.Fatalf("expected outputs reset, got %#v", outputs)
	}
}

func TestApplyInsertsAfterID(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		codeCell("alpha", "print(1)", nil, []any{}),
		markdownCell("beta", "tail"),
	}, 5)
	req := Request{
		NotebookPath: "book.ipynb",
		CellID:       "alpha",
		NewSource:    "# inserted",
		CellType:     CellTypeMarkdown,
		EditMode:     EditModeInsert,
	}

	result, err := Apply(content, req, fixedID("generated"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.CellID != "generated" {
		t.Fatalf("expected generated cell id, got %q", result.CellID)
	}
	if result.EditMode != EditModeInsert {
		t.Fatalf("expected insert mode, got %q", result.EditMode)
	}

	updated := decodeNotebook(t, result.UpdatedJSON)
	cells := updated["cells"].([]any)
	if len(cells) != 3 {
		t.Fatalf("expected 3 cells, got %d", len(cells))
	}
	inserted := cells[1].(map[string]any)
	if inserted["id"] != "generated" || inserted["source"] != "# inserted" || inserted["cell_type"] != "markdown" {
		t.Fatalf("unexpected inserted cell: %#v", inserted)
	}
}

func TestApplyInsertsAtBeginningWithoutCellID(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		markdownCell("tail", "tail"),
	}, 5)
	req := Request{
		NotebookPath: "book.ipynb",
		NewSource:    "print('first')",
		CellType:     CellTypeCode,
		EditMode:     EditModeInsert,
	}

	result, err := Apply(content, req, fixedID("first"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	updated := decodeNotebook(t, result.UpdatedJSON)
	first := updated["cells"].([]any)[0].(map[string]any)
	if first["id"] != "first" || first["cell_type"] != "code" || first["source"] != "print('first')" {
		t.Fatalf("unexpected first cell: %#v", first)
	}
}

func TestApplyDelete(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		codeCell("alpha", "print(1)", nil, []any{}),
		markdownCell("beta", "remove me"),
	}, 5)
	req := Request{
		NotebookPath: "book.ipynb",
		CellID:       "beta",
		EditMode:     EditModeDelete,
	}

	result, err := Apply(content, req, fixedID("unused"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.CellID != "beta" || result.CellType != CellTypeMarkdown {
		t.Fatalf("unexpected result target: %#v", result)
	}
	updated := decodeNotebook(t, result.UpdatedJSON)
	cells := updated["cells"].([]any)
	if len(cells) != 1 {
		t.Fatalf("expected one remaining cell, got %d", len(cells))
	}
	if cells[0].(map[string]any)["id"] != "alpha" {
		t.Fatalf("unexpected remaining cell: %#v", cells[0])
	}
}

func TestReplaceOnePastEndTurnsIntoCodeInsert(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		markdownCell("alpha", "first"),
	}, 5)
	req := Request{
		NotebookPath: "book.ipynb",
		CellID:       "cell-1",
		NewSource:    "print('append')",
	}

	result, err := Apply(content, req, fixedID("append-id"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.EditMode != EditModeInsert || result.CellType != CellTypeCode || result.CellID != "append-id" {
		t.Fatalf("unexpected result: %#v", result)
	}
	updated := decodeNotebook(t, result.UpdatedJSON)
	inserted := updated["cells"].([]any)[1].(map[string]any)
	if inserted["cell_type"] != "code" || inserted["source"] != "print('append')" {
		t.Fatalf("unexpected inserted cell: %#v", inserted)
	}
}

func TestOldNbformatInsertDoesNotForceID(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{}, 4)
	req := Request{
		NotebookPath: "book.ipynb",
		NewSource:    "old",
		CellType:     CellTypeMarkdown,
		EditMode:     EditModeInsert,
	}

	result, err := Apply(content, req, fixedID("ignored"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.CellID != "" {
		t.Fatalf("expected no generated cell id, got %q", result.CellID)
	}
	updated := decodeNotebook(t, result.UpdatedJSON)
	cell := updated["cells"].([]any)[0].(map[string]any)
	if _, ok := cell["id"]; ok {
		t.Fatalf("did not expect id in old nbformat cell: %#v", cell)
	}
}

func TestLanguageDefaultsToPython(t *testing.T) {
	content := []byte(`{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`)
	req := Request{
		NotebookPath: "book.ipynb",
		NewSource:    "x",
		CellType:     CellTypeMarkdown,
		EditMode:     EditModeInsert,
	}

	result, err := Apply(content, req, fixedID("id"))
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if result.Language != "python" {
		t.Fatalf("expected default python language, got %q", result.Language)
	}
	if result.OriginalJSON != string(content) || result.UpdatedJSON == "" {
		t.Fatalf("expected original and updated JSON in result")
	}
}

func TestValidateRequestRejectsRequestErrors(t *testing.T) {
	content := sampleNotebook(t, []map[string]any{
		markdownCell("alpha", "first"),
	}, 5)
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{
			name: "extension",
			req:  Request{NotebookPath: "book.txt", CellID: "alpha"},
			want: ".ipynb",
		},
		{
			name: "mode",
			req:  Request{NotebookPath: "book.ipynb", CellID: "alpha", EditMode: "move"},
			want: "edit_mode",
		},
		{
			name: "insert cell type",
			req:  Request{NotebookPath: "book.ipynb", EditMode: EditModeInsert},
			want: "cell_type",
		},
		{
			name: "missing cell id",
			req:  Request{NotebookPath: "book.ipynb"},
			want: "cell_id",
		},
		{
			name: "not found",
			req:  Request{NotebookPath: "book.ipynb", CellID: "missing"},
			want: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(content, tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func sampleNotebook(t *testing.T, cells []map[string]any, minor int) []byte {
	t.Helper()
	data := map[string]any{
		"cells":          cells,
		"metadata":       map[string]any{"language_info": map[string]any{"name": "python"}},
		"nbformat":       4,
		"nbformat_minor": minor,
	}
	content, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal notebook: %v", err)
	}
	return content
}

func codeCell(id, source string, executionCount any, outputs []any) map[string]any {
	return map[string]any{
		"cell_type":       "code",
		"execution_count": executionCount,
		"id":              id,
		"metadata":        map[string]any{},
		"outputs":         outputs,
		"source":          source,
	}
}

func markdownCell(id, source string) map[string]any {
	return map[string]any{
		"cell_type": "markdown",
		"id":        id,
		"metadata":  map[string]any{},
		"source":    source,
	}
}

func fixedID(id string) func() string {
	return func() string {
		return id
	}
}

func decodeNotebook(t *testing.T, content string) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		t.Fatalf("failed to decode notebook: %v\n%s", err, content)
	}
	return data
}
