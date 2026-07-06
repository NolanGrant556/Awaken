package readartifact

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/localcompute"
)

const (
	defaultArtifactMaxRows   = 40
	defaultArtifactMaxChars  = 12000
	defaultArtifactMaxSheets = 3
	maxArtifactRows          = 200
	maxArtifactChars         = 50000
	maxArtifactSheets        = 10
)

type ReadArtifactTool struct{}

type readArtifactParams struct {
	Path      string `json:"path"`
	Sheet     string `json:"sheet,omitempty"`
	MaxRows   int    `json:"max_rows,omitempty"`
	MaxChars  int    `json:"max_chars,omitempty"`
	MaxSheets int    `json:"max_sheets,omitempty"`
}

type artifactPreview struct {
	Kind    string
	Path    string
	Content string
}

type xlsxWorkbook struct {
	Sheets []xlsxWorkbookSheet `xml:"sheets>sheet"`
}

type xlsxWorkbookSheet struct {
	Name string `xml:"name,attr"`
	ID   string `xml:"id,attr"`
}

type xlsxRelationships struct {
	Items []xlsxRelationship `xml:"Relationship"`
}

type xlsxRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type xlsxSharedStrings struct {
	Items []xlsxSharedString `xml:"si"`
}

type xlsxSharedString struct {
	Text string    `xml:"t"`
	Runs []xlsxRun `xml:"r"`
}

type xlsxRun struct {
	Text string `xml:"t"`
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref    string     `xml:"r,attr"`
	Type   string     `xml:"t,attr"`
	Value  string     `xml:"v"`
	Inline xlsxInline `xml:"is"`
}

type xlsxInline struct {
	Text string    `xml:"t"`
	Runs []xlsxRun `xml:"r"`
}

func (t *ReadArtifactTool) Name() string {
	return "read_artifact"
}

func (t *ReadArtifactTool) Description() string {
	return "读取 xlsx/xls/csv/tsv/pdf/docx 等工作文件，返回结构化预览"
}

func (t *ReadArtifactTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "要读取的文件绝对路径或相对工作区路径", Type: "string", Required: true},
		{Name: "sheet", Description: "读取指定工作表（xlsx/xls 可选）", Type: "string", Required: false},
		{Name: "max_rows", Description: "表格类文件最多预览多少行，默认 40，最大 200", Type: "integer", Required: false},
		{Name: "max_chars", Description: "最多返回多少字符，默认 12000，最大 50000", Type: "integer", Required: false},
		{Name: "max_sheets", Description: "表格类文件最多展示多少个工作表，默认 3，最大 10", Type: "integer", Required: false},
	}
}

func (t *ReadArtifactTool) IsReadOnly() bool {
	return true
}

func (t *ReadArtifactTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        "read_artifact",
		DisplayName: "Read Artifact",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
}

func (t *ReadArtifactTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseReadArtifactParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if !permissionContext.IsWithinReadableRootsResolved(params.Path) {
		return tool.DenyPermission(fmt.Sprintf("path %s is outside readable roots or escapes via symlink", params.Path))
	}
	return tool.AllowPermission()
}

func (t *ReadArtifactTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseReadArtifactParams(input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(params.Path) == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	if tool.HasPermissionContext(ctx) {
		checkedPath, err := permissionContext.ResolveReadablePathChecked(params.Path)
		if err != nil {
			return "", err
		}
		params.Path = checkedPath
	} else {
		params.Path = permissionContext.ResolvePath(params.Path)
	}

	preview, err := readArtifact(ctx, permissionContext, params)
	if err != nil {
		return "", err
	}

	body := strings.TrimSpace(preview.Content)
	if body == "" {
		body = "[empty]"
	}
	result := fmt.Sprintf("类型: %s\n路径: %s\n\n%s", preview.Kind, preview.Path, body)
	return truncateArtifactString(result, normalizeArtifactMaxChars(params.MaxChars)), nil
}

func parseReadArtifactParams(input string) (readArtifactParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return readArtifactParams{}, nil
	}
	if !strings.HasPrefix(input, "{") {
		return readArtifactParams{Path: input}, nil
	}
	var params readArtifactParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return readArtifactParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象或文件路径: %w", err)
	}
	return params, nil
}

func readArtifact(ctx context.Context, permCtx tool.PermissionContext, params readArtifactParams) (artifactPreview, error) {
	ext := strings.ToLower(filepath.Ext(params.Path))
	switch ext {
	case ".csv":
		return readDelimitedArtifact(params.Path, ',', normalizeArtifactMaxRows(params.MaxRows), normalizeArtifactMaxChars(params.MaxChars))
	case ".tsv":
		return readDelimitedArtifact(params.Path, '\t', normalizeArtifactMaxRows(params.MaxRows), normalizeArtifactMaxChars(params.MaxChars))
	case ".xlsx":
		return readXLSXArtifact(params.Path, params.Sheet, normalizeArtifactMaxRows(params.MaxRows), normalizeArtifactMaxSheets(params.MaxSheets), normalizeArtifactMaxChars(params.MaxChars))
	case ".xls":
		return readXLSArtifact(ctx, permCtx, params)
	case ".docx":
		return readDOCXArtifact(params.Path, normalizeArtifactMaxChars(params.MaxChars))
	case ".pdf":
		return readPDFArtifact(ctx, permCtx, params.Path, normalizeArtifactMaxChars(params.MaxChars))
	default:
		return artifactPreview{}, fmt.Errorf("暂不支持读取该文件类型: %s", ext)
	}
}

func readDelimitedArtifact(path string, delimiter rune, maxRows int, maxChars int) (artifactPreview, error) {
	file, err := os.Open(path)
	if err != nil {
		return artifactPreview{}, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1

	rows := make([][]string, 0, maxRows)
	for len(rows) < maxRows {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return artifactPreview{}, err
		}
		rows = append(rows, record)
	}
	content := formatTable(rows)
	if len(rows) == maxRows {
		content += fmt.Sprintf("\n\n[preview truncated at %d rows]", maxRows)
	}
	return artifactPreview{
		Kind:    strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		Path:    path,
		Content: truncateArtifactString(content, maxChars),
	}, nil
}

func readXLSArtifact(ctx context.Context, permCtx tool.PermissionContext, params readArtifactParams) (artifactPreview, error) {
	convertDir := filepath.Join(permCtx.TemporaryWorkRoot, "artifact-xls")
	if err := os.MkdirAll(convertDir, 0o755); err != nil {
		return artifactPreview{}, err
	}
	if _, err := localcompute.Run(ctx, permCtx, localcompute.CommandSpec{
		Backend:     "soffice",
		Args:        []string{"--headless", "--convert-to", "xlsx", "--outdir", convertDir, params.Path},
		InputPaths:  []string{params.Path},
		OutputPaths: []string{convertDir},
		Workdir:     permCtx.TemporaryWorkRoot,
		Timeout:     45 * time.Second,
	}); err != nil {
		return artifactPreview{}, err
	}
	converted := filepath.Join(convertDir, strings.TrimSuffix(filepath.Base(params.Path), filepath.Ext(params.Path))+".xlsx")
	return readXLSXArtifact(converted, params.Sheet, normalizeArtifactMaxRows(params.MaxRows), normalizeArtifactMaxSheets(params.MaxSheets), normalizeArtifactMaxChars(params.MaxChars))
}

func readXLSXArtifact(path string, sheetFilter string, maxRows int, maxSheets int, maxChars int) (artifactPreview, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return artifactPreview{}, err
	}
	defer zr.Close()

	workbookData, err := readZipEntry(&zr.Reader, "xl/workbook.xml")
	if err != nil {
		return artifactPreview{}, err
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookData, &workbook); err != nil {
		return artifactPreview{}, err
	}
	relsData, err := readZipEntry(&zr.Reader, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return artifactPreview{}, err
	}
	var rels xlsxRelationships
	if err := xml.Unmarshal(relsData, &rels); err != nil {
		return artifactPreview{}, err
	}
	sharedStrings := readSharedStrings(&zr.Reader)
	targets := make(map[string]string, len(rels.Items))
	for _, rel := range rels.Items {
		targets[rel.ID] = "xl/" + strings.TrimPrefix(rel.Target, "/")
	}

	sections := make([]string, 0, maxSheets)
	for _, sheet := range workbook.Sheets {
		if strings.TrimSpace(sheetFilter) != "" && sheet.Name != strings.TrimSpace(sheetFilter) {
			continue
		}
		target := targets[sheet.ID]
		if strings.TrimSpace(target) == "" {
			continue
		}
		table, err := readWorksheetPreview(&zr.Reader, target, sharedStrings, maxRows)
		if err != nil {
			return artifactPreview{}, err
		}
		section := "[Sheet] " + sheet.Name
		if table != "" {
			section += "\n" + table
		}
		sections = append(sections, section)
		if len(sections) >= maxSheets {
			break
		}
	}
	if len(sections) == 0 {
		return artifactPreview{}, fmt.Errorf("未找到可读取的工作表")
	}
	content := strings.Join(sections, "\n\n")
	if len(sections) == maxSheets && len(workbook.Sheets) > maxSheets {
		content += fmt.Sprintf("\n\n[preview truncated at %d sheets]", maxSheets)
	}
	return artifactPreview{
		Kind:    "xlsx",
		Path:    path,
		Content: truncateArtifactString(content, maxChars),
	}, nil
}

func readWorksheetPreview(zr *zip.Reader, name string, sharedStrings []string, maxRows int) (string, error) {
	data, err := readZipEntry(zr, name)
	if err != nil {
		return "", err
	}
	var worksheet xlsxWorksheet
	if err := xml.Unmarshal(data, &worksheet); err != nil {
		return "", err
	}
	rows := make([][]string, 0, minInt(len(worksheet.Rows), maxRows))
	for _, row := range worksheet.Rows {
		if len(rows) >= maxRows {
			break
		}
		values := renderWorksheetRow(row, sharedStrings)
		if isEmptyRow(values) {
			continue
		}
		rows = append(rows, values)
	}
	if len(rows) == 0 {
		return "[empty sheet]", nil
	}
	table := formatTable(rows)
	if len(worksheet.Rows) > maxRows {
		table += fmt.Sprintf("\n\n[preview truncated at %d rows]", maxRows)
	}
	return table, nil
}

func renderWorksheetRow(row xlsxRow, sharedStrings []string) []string {
	maxIndex := 0
	indexed := make(map[int]string, len(row.Cells))
	sequentialIndex := 0
	for _, cell := range row.Cells {
		index := columnIndexFromCellRef(cell.Ref)
		if index < 0 {
			index = sequentialIndex
		}
		sequentialIndex = index + 1
		indexed[index] = extractCellValue(cell, sharedStrings)
		if index > maxIndex {
			maxIndex = index
		}
	}
	values := make([]string, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		values[i] = indexed[i]
	}
	return trimTrailingEmpty(values)
}

func extractCellValue(cell xlsxCell, sharedStrings []string) string {
	switch strings.TrimSpace(cell.Type) {
	case "s":
		index, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err == nil && index >= 0 && index < len(sharedStrings) {
			return sharedStrings[index]
		}
	case "inlineStr":
		return extractInlineString(cell.Inline)
	}
	if text := extractInlineString(cell.Inline); strings.TrimSpace(text) != "" {
		return text
	}
	return strings.TrimSpace(cell.Value)
}

func extractInlineString(inline xlsxInline) string {
	if strings.TrimSpace(inline.Text) != "" {
		return strings.TrimSpace(inline.Text)
	}
	parts := make([]string, 0, len(inline.Runs))
	for _, run := range inline.Runs {
		if text := strings.TrimSpace(run.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

func readSharedStrings(zr *zip.Reader) []string {
	data, err := readZipEntry(zr, "xl/sharedStrings.xml")
	if err != nil {
		return nil
	}
	var shared xlsxSharedStrings
	if err := xml.Unmarshal(data, &shared); err != nil {
		return nil
	}
	result := make([]string, 0, len(shared.Items))
	for _, item := range shared.Items {
		if strings.TrimSpace(item.Text) != "" {
			result = append(result, item.Text)
			continue
		}
		parts := make([]string, 0, len(item.Runs))
		for _, run := range item.Runs {
			if text := strings.TrimSpace(run.Text); text != "" {
				parts = append(parts, text)
			}
		}
		result = append(result, strings.Join(parts, ""))
	}
	return result
}

func readDOCXArtifact(path string, maxChars int) (artifactPreview, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return artifactPreview{}, err
	}
	defer zr.Close()
	data, err := readZipEntry(&zr.Reader, "word/document.xml")
	if err != nil {
		return artifactPreview{}, err
	}
	text, err := extractDOCXText(data)
	if err != nil {
		return artifactPreview{}, err
	}
	return artifactPreview{
		Kind:    "docx",
		Path:    path,
		Content: truncateArtifactString(text, maxChars),
	}, nil
}

func extractDOCXText(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var builder strings.Builder
	inText := false
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "t":
				inText = true
			case "p":
				if builder.Len() > 0 {
					builder.WriteString("\n")
				}
			}
		case xml.EndElement:
			if value.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				builder.WriteString(string(value))
			}
		}
	}
	return strings.TrimSpace(builder.String()), nil
}

func readPDFArtifact(ctx context.Context, permCtx tool.PermissionContext, path string, maxChars int) (artifactPreview, error) {
	outputPath := filepath.Join(permCtx.TemporaryWorkRoot, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".txt")
	if _, err := localcompute.Run(ctx, permCtx, localcompute.CommandSpec{
		Backend:     "pdftotext",
		Args:        []string{"-enc", "UTF-8", "-nopgbrk", path, outputPath},
		InputPaths:  []string{path},
		OutputPaths: []string{outputPath},
		Workdir:     permCtx.TemporaryWorkRoot,
		Timeout:     30 * time.Second,
	}); err != nil {
		return artifactPreview{}, err
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return artifactPreview{}, err
	}
	return artifactPreview{
		Kind:    "pdf",
		Path:    path,
		Content: truncateArtifactString(strings.TrimSpace(string(data)), maxChars),
	}, nil
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, file := range zr.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	return nil, fmt.Errorf("zip entry not found: %s", name)
}

func columnIndexFromCellRef(ref string) int {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return -1
	}
	index := 0
	found := false
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			if r < 'a' || r > 'z' {
				break
			}
			r = r - 'a' + 'A'
		}
		found = true
		index = index*26 + int(r-'A'+1)
	}
	if !found {
		return -1
	}
	return index - 1
}

func formatTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var buffer bytes.Buffer
	writer := tabwriter.NewWriter(&buffer, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		line := make([]string, len(row))
		for i, cell := range row {
			line[i] = strings.TrimSpace(cell)
		}
		_, _ = fmt.Fprintln(writer, strings.Join(line, "\t"))
	}
	_ = writer.Flush()
	return strings.TrimSpace(buffer.String())
}

func truncateArtifactString(value string, maxChars int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxChars {
		return string(runes)
	}
	return string(runes[:maxChars]) + fmt.Sprintf("\n\n[artifact truncated to %d chars]", maxChars)
}

func normalizeArtifactMaxRows(value int) int {
	if value <= 0 {
		return defaultArtifactMaxRows
	}
	if value > maxArtifactRows {
		return maxArtifactRows
	}
	return value
}

func normalizeArtifactMaxChars(value int) int {
	if value <= 0 {
		return defaultArtifactMaxChars
	}
	if value > maxArtifactChars {
		return maxArtifactChars
	}
	return value
}

func normalizeArtifactMaxSheets(value int) int {
	if value <= 0 {
		return defaultArtifactMaxSheets
	}
	if value > maxArtifactSheets {
		return maxArtifactSheets
	}
	return value
}

func trimTrailingEmpty(values []string) []string {
	end := len(values)
	for end > 0 && strings.TrimSpace(values[end-1]) == "" {
		end--
	}
	return values[:end]
}

func isEmptyRow(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	tool.MustRegisterTool(&ReadArtifactTool{})
}
