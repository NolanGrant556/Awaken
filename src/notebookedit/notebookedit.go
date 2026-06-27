package notebookedit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

type EditMode string

const (
	EditModeReplace EditMode = "replace"
	EditModeInsert  EditMode = "insert"
	EditModeDelete  EditMode = "delete"
)

type CellType string

const (
	CellTypeCode     CellType = "code"
	CellTypeMarkdown CellType = "markdown"
)

type Request struct {
	NotebookPath string   `json:"notebook_path"`
	CellID       string   `json:"cell_id,omitempty"`
	NewSource    string   `json:"new_source"`
	CellType     CellType `json:"cell_type,omitempty"`
	EditMode     EditMode `json:"edit_mode,omitempty"`
}

type Result struct {
	NewSource    string   `json:"new_source"`
	CellID       string   `json:"cell_id,omitempty"`
	CellType     CellType `json:"cell_type"`
	Language     string   `json:"language"`
	EditMode     EditMode `json:"edit_mode"`
	NotebookPath string   `json:"notebook_path"`
	OriginalJSON string   `json:"original_file"`
	UpdatedJSON  string   `json:"updated_file"`
}

type notebook struct {
	data  map[string]any
	cells []map[string]any
}

type editPlan struct {
	mode     EditMode
	index    int
	cellID   string
	cellType CellType
}

func ValidateRequest(content []byte, req Request) error {
	_, err := validateRequestFields(req)
	if err != nil {
		return err
	}
	nb, err := parseNotebook(content)
	if err != nil {
		return err
	}
	_, err = planEdit(nb, req)
	return err
}

func Apply(content []byte, req Request, newID func() string) (Result, error) {
	_, err := validateRequestFields(req)
	if err != nil {
		return Result{}, err
	}
	nb, err := parseNotebook(content)
	if err != nil {
		return Result{}, err
	}
	plan, err := planEdit(nb, req)
	if err != nil {
		return Result{}, err
	}

	language := notebookLanguage(nb)
	result := Result{
		NewSource:    req.NewSource,
		CellID:       plan.cellID,
		CellType:     plan.cellType,
		Language:     language,
		EditMode:     plan.mode,
		NotebookPath: req.NotebookPath,
		OriginalJSON: string(content),
	}

	switch plan.mode {
	case EditModeDelete:
		nb.cells = append(nb.cells[:plan.index], nb.cells[plan.index+1:]...)
	case EditModeInsert:
		newCell, id, err := buildNewCell(nb, req.NewSource, plan.cellType, newID)
		if err != nil {
			return Result{}, err
		}
		if id != "" {
			result.CellID = id
		}
		nb.cells = append(nb.cells, nil)
		copy(nb.cells[plan.index+1:], nb.cells[plan.index:])
		nb.cells[plan.index] = newCell
	case EditModeReplace:
		cell := nb.cells[plan.index]
		wasCode := cellTypeOf(cell) == CellTypeCode
		cell["source"] = req.NewSource
		if req.CellType != "" {
			cell["cell_type"] = string(req.CellType)
		}
		finalType := cellTypeOf(cell)
		if wasCode || finalType == CellTypeCode {
			cell["execution_count"] = nil
			cell["outputs"] = []any{}
		}
		result.CellType = finalType
	default:
		return Result{}, fmt.Errorf("edit_mode must be replace, insert, or delete")
	}

	nb.data["cells"] = nb.cells
	updated, err := json.MarshalIndent(nb.data, "", " ")
	if err != nil {
		return Result{}, fmt.Errorf("marshal notebook: %w", err)
	}
	result.UpdatedJSON = string(updated)
	return result, nil
}

func parseNotebook(content []byte) (notebook, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()

	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return notebook{}, fmt.Errorf("notebook is not valid JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return notebook{}, fmt.Errorf("notebook is not valid JSON")
	}

	rawCells, ok := data["cells"].([]any)
	if !ok {
		return notebook{}, fmt.Errorf("notebook cells must be an array")
	}
	cells := make([]map[string]any, len(rawCells))
	for i, rawCell := range rawCells {
		cell, ok := rawCell.(map[string]any)
		if !ok {
			return notebook{}, fmt.Errorf("notebook cell %d must be an object", i)
		}
		cells[i] = cell
	}
	return notebook{data: data, cells: cells}, nil
}

func planEdit(nb notebook, req Request) (editPlan, error) {
	mode, err := normalizeEditMode(req.EditMode)
	if err != nil {
		return editPlan{}, err
	}

	switch mode {
	case EditModeInsert:
		cellType := req.CellType
		index := 0
		if req.CellID != "" {
			found, cellID, ok := findCell(nb.cells, req.CellID)
			if !ok {
				return editPlan{}, cellNotFoundError(req.CellID, len(nb.cells), false)
			}
			index = found + 1
			return editPlan{mode: mode, index: index, cellID: cellID, cellType: cellType}, nil
		}
		return editPlan{mode: mode, index: index, cellType: cellType}, nil
	case EditModeReplace, EditModeDelete:
		index, cellID, ok := findCell(nb.cells, req.CellID)
		if !ok {
			parsed, parsedOK := parseCellIndex(req.CellID)
			if mode == EditModeReplace && parsedOK && parsed == len(nb.cells) {
				cellType := req.CellType
				if cellType == "" {
					cellType = CellTypeCode
				}
				return editPlan{mode: EditModeInsert, index: parsed, cellType: cellType}, nil
			}
			return editPlan{}, cellNotFoundError(req.CellID, len(nb.cells), mode == EditModeReplace)
		}
		cellType := cellTypeOf(nb.cells[index])
		if req.CellType != "" {
			cellType = req.CellType
		}
		return editPlan{mode: mode, index: index, cellID: cellID, cellType: cellType}, nil
	default:
		return editPlan{}, fmt.Errorf("edit_mode must be replace, insert, or delete")
	}
}

func validateRequestFields(req Request) (EditMode, error) {
	mode, err := normalizeEditMode(req.EditMode)
	if err != nil {
		return "", err
	}
	if filepath.Ext(req.NotebookPath) != ".ipynb" {
		return "", fmt.Errorf("file must be a Jupyter notebook (.ipynb file)")
	}
	if req.CellType != "" && req.CellType != CellTypeCode && req.CellType != CellTypeMarkdown {
		return "", fmt.Errorf("cell_type must be code or markdown")
	}
	if mode == EditModeInsert && req.CellType == "" {
		return "", fmt.Errorf("cell_type is required when using edit_mode=insert")
	}
	if mode != EditModeInsert && req.CellID == "" {
		return "", fmt.Errorf("cell_id must be specified when not inserting a new cell")
	}
	return mode, nil
}

func normalizeEditMode(mode EditMode) (EditMode, error) {
	if mode == "" {
		return EditModeReplace, nil
	}
	switch mode {
	case EditModeReplace, EditModeInsert, EditModeDelete:
		return mode, nil
	default:
		return "", fmt.Errorf("edit_mode must be replace, insert, or delete")
	}
}

func findCell(cells []map[string]any, cellID string) (int, string, bool) {
	for i, cell := range cells {
		if id, ok := cell["id"].(string); ok && id == cellID {
			return i, id, true
		}
	}
	index, ok := parseCellIndex(cellID)
	if !ok || index < 0 || index >= len(cells) {
		return 0, "", false
	}
	return index, displayCellID(cells[index], index), true
}

func parseCellIndex(cellID string) (int, bool) {
	if !strings.HasPrefix(cellID, "cell-") {
		return 0, false
	}
	rawIndex := strings.TrimPrefix(cellID, "cell-")
	if rawIndex == "" {
		return 0, false
	}
	for _, r := range rawIndex {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	index, err := strconv.Atoi(rawIndex)
	if err != nil {
		return 0, false
	}
	return index, true
}

func cellNotFoundError(cellID string, cellCount int, allowAppend bool) error {
	index, ok := parseCellIndex(cellID)
	if ok {
		if allowAppend && index == cellCount {
			return nil
		}
		return fmt.Errorf("cell with index %d does not exist in notebook", index)
	}
	return fmt.Errorf("cell with ID %q not found in notebook", cellID)
}

func displayCellID(cell map[string]any, index int) string {
	if id, ok := cell["id"].(string); ok && id != "" {
		return id
	}
	return fmt.Sprintf("cell-%d", index)
}

func cellTypeOf(cell map[string]any) CellType {
	if cellType, ok := cell["cell_type"].(string); ok {
		switch CellType(cellType) {
		case CellTypeCode:
			return CellTypeCode
		case CellTypeMarkdown:
			return CellTypeMarkdown
		}
	}
	return CellTypeCode
}

func buildNewCell(nb notebook, source string, cellType CellType, newID func() string) (map[string]any, string, error) {
	cell := map[string]any{
		"cell_type": string(cellType),
		"metadata":  map[string]any{},
		"source":    source,
	}
	if cellType == CellTypeCode {
		cell["execution_count"] = nil
		cell["outputs"] = []any{}
	}
	if notebookRequiresCellID(nb) {
		if newID == nil {
			return nil, "", fmt.Errorf("newID is required when inserting into nbformat 4.5+ notebooks")
		}
		id := newID()
		if id == "" {
			return nil, "", fmt.Errorf("newID returned an empty cell id")
		}
		cell["id"] = id
		return cell, id, nil
	}
	return cell, "", nil
}

func notebookRequiresCellID(nb notebook) bool {
	major := intField(nb.data["nbformat"])
	minor := intField(nb.data["nbformat_minor"])
	return major > 4 || (major == 4 && minor >= 5)
}

func intField(value any) int {
	switch n := value.(type) {
	case json.Number:
		i, err := strconv.Atoi(n.String())
		if err == nil {
			return i
		}
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func notebookLanguage(nb notebook) string {
	metadata, ok := nb.data["metadata"].(map[string]any)
	if !ok {
		return "python"
	}
	languageInfo, ok := metadata["language_info"].(map[string]any)
	if !ok {
		return "python"
	}
	name, ok := languageInfo["name"].(string)
	if !ok || name == "" {
		return "python"
	}
	return name
}
