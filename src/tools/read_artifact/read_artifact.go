package readartifact

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"awaken/src/runtime/officerender"
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
	ID         string `xml:"Id,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
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
	return "读取本机工作文件并返回结构化预览；路径可用绝对路径、相对当前工作区路径或已上传附件文件名；支持 .xlsx/.xls/.csv/.tsv/.pdf/.docx。普通源码/Markdown/JSON 用 read_file，图片元数据用 image_inspect，PPTX 可用 artifact_inspect 或 presentation_render"
}

func (t *ReadArtifactTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "要读取的工作文件路径，可用绝对路径、相对当前工作区路径或已上传附件文件名；支持 xlsx/xls/csv/tsv/pdf/docx", Type: "string", Required: true},
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
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "read_artifact",
		DisplayName: "Read Artifact",
		Description: t.Description(),
		Purpose:     "把表格、PDF 和 Word 等本机工作文件转换成有界、可推理的结构化预览。",
		WhenToUse: []string{
			"需要预览 xlsx、xls、csv、tsv、pdf 或 docx 的内容和结构时",
			"附件是工作文件而不是普通源码或文本时",
		},
		WhenNotToUse: []string{
			"源码、Markdown、JSON 等普通文本使用 read_file",
			"需要修改工作文件时使用对应 spreadsheet/document/pdf typed artifact 工具",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"kind":       {Type: "string", Description: "识别出的 artifact 类型"},
			"path":       {Type: "string", Description: "实际预览路径"},
			"max_rows":   {Type: "integer", Description: "实际表格预览行数上限"},
			"max_chars":  {Type: "integer", Description: "实际字符上限"},
			"max_sheets": {Type: "integer", Description: "实际工作表数量上限"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"local work artifact"}, Description: "读取并解析工作文件，不修改源文件。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "预览工作簿指定工作表", Input: map[string]any{"path": "report.xlsx", "sheet": "Summary", "max_rows": 40}, Outcome: "返回工作表结构和有界单元格预览"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "read_artifact_path_required", Cause: "path 缺失", Retryable: false, Recovery: []string{"提供工作文件路径"}},
			{Code: "read_artifact_unsupported_type", Cause: "文件类型不受本工具支持", Retryable: false, Recovery: []string{"选择匹配的 artifact 工具"}},
			{Code: "read_artifact_empty_pdf", Cause: "PDF 没有可抽取内容", Retryable: false, Recovery: []string{"检查是否为扫描 PDF 或使用 OCR 能力"}},
			{Code: "read_artifact_invalid_or_corrupt", Cause: "工作文件损坏或格式无效", Retryable: false, Recovery: []string{"验证或重新生成文件"}},
			{Code: "read_artifact_specialized_tool_required", Cause: "文件完整但内置轻量预览不支持该变体", Retryable: false, Recovery: []string{"改用 spreadsheet Skill、openpyxl/pandas 等成熟工具读取"}},
			{Code: "read_artifact_runtime_failed", Cause: "解析 runtime 执行失败；原调用在状态核验前不可安全重试", Retryable: false, Recovery: []string{"准备或修复所需 runtime，再发起新调用"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func (t *ReadArtifactTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if _, err := parseReadArtifactParams(input); err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	return tool.AllowPermission()
}

func (t *ReadArtifactTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome reports the resolved artifact kind, preview bounds, and
// truncation state. Invalid or unreadable artifacts are semantic outcomes;
// this read-only tool never reports the preview as a filesystem mutation.
func (t *ReadArtifactTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseReadArtifactParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "read_artifact_invalid_input", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_json_parameters", "provide_file_path"}
		outcome.Diagnostic = map[string]any{"expected_input": "JSON object or file path"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if strings.TrimSpace(params.Path) == "" {
		outcome := tool.FailedOutcome("文件路径不能为空", "read_artifact_path_required", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_file_path"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	requestedPath := params.Path
	permissionContext := tool.PermissionContextFromContext(ctx)
	if tool.HasPermissionContext(ctx) {
		checkedPath, err := permissionContext.ResolveReadablePathChecked(params.Path)
		if err != nil {
			outcome := tool.FailedOutcome(err.Error(), "read_artifact_path_unavailable", tool.ErrorCategoryDomain, false)
			outcome.RecoveryOptions = []string{"provide_unambiguous_file_path", "list_files"}
			outcome.Diagnostic = map[string]any{"requested_path": requestedPath}
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
		params.Path = checkedPath
	} else {
		params.Path = permissionContext.ResolvePath(params.Path)
	}
	ext := strings.ToLower(filepath.Ext(params.Path))
	if !isSupportedArtifactExtension(ext) {
		message := fmt.Sprintf("暂不支持读取该文件类型: %s。read_artifact 支持 .csv/.tsv/.xlsx/.xls/.docx/.pdf；普通文本请用 read_file，图片请用 image_inspect，PPTX 请用 artifact_inspect 或 presentation_render", ext)
		outcome := tool.FailedOutcome(message, "read_artifact_unsupported_type", tool.ErrorCategoryDomain, false)
		outcome.RecoveryOptions = []string{"use_read_file_for_text", "use_image_inspect_for_images", "use_artifact_inspect_for_pptx"}
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": params.Path, "extension": ext}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	info, err := os.Stat(params.Path)
	if err != nil {
		outcome := readArtifactPathFailure(err, requestedPath, params.Path)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if !info.Mode().IsRegular() {
		message := fmt.Sprintf("read_artifact 只能读取普通文件: %s", params.Path)
		outcome := tool.FailedOutcome(message, "read_artifact_not_regular_file", tool.ErrorCategoryDomain, false)
		outcome.RecoveryOptions = []string{"choose_regular_file", "list_files"}
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": params.Path}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if ext == ".pdf" && info.Size() == 0 {
		message := fmt.Sprintf("read_artifact 无法读取空 PDF 文件: %s。该文件大小为 0 字节，可能下载/复制失败或文件已损坏；请提供有效 PDF", params.Path)
		outcome := tool.FailedOutcome(message, "read_artifact_empty_pdf", tool.ErrorCategoryDomain, false)
		outcome.RecoveryOptions = []string{"provide_valid_pdf", "verify_file_copy_or_download"}
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": params.Path, "size_bytes": info.Size()}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	preview, err := readArtifact(ctx, permissionContext, params)
	if err != nil {
		outcome := readArtifactFailure(err, ext, requestedPath, params.Path)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	body := strings.TrimSpace(preview.Content)
	if body == "" {
		body = "[empty]"
	}
	result := fmt.Sprintf("类型: %s\n路径: %s\n\n%s", preview.Kind, preview.Path, body)
	result = truncateArtifactString(result, normalizeArtifactMaxChars(params.MaxChars))
	truncated := strings.Contains(result, "[artifact truncated") || strings.Contains(result, "[preview truncated")
	outcome := tool.SucceededOutcome(result)
	outcome.Truncated = truncated
	outcome.Diagnostic = map[string]any{
		"requested_path": requestedPath, "resolved_path": params.Path, "preview_path": preview.Path,
		"kind": preview.Kind, "max_rows": normalizeArtifactMaxRows(params.MaxRows),
		"max_chars": normalizeArtifactMaxChars(params.MaxChars), "max_sheets": normalizeArtifactMaxSheets(params.MaxSheets),
		"truncated": truncated,
	}
	outcome.Details = map[string]any{
		"kind": preview.Kind, "path": preview.Path,
		"max_rows":   normalizeArtifactMaxRows(params.MaxRows),
		"max_chars":  normalizeArtifactMaxChars(params.MaxChars),
		"max_sheets": normalizeArtifactMaxSheets(params.MaxSheets),
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func isSupportedArtifactExtension(ext string) bool {
	switch ext {
	case ".csv", ".tsv", ".xlsx", ".xls", ".docx", ".pdf":
		return true
	default:
		return false
	}
}

func readArtifactPathFailure(err error, requestedPath, resolvedPath string) tool.ExecutionOutcome {
	code := "read_artifact_path_unavailable"
	category := tool.ErrorCategoryDomain
	recovery := []string{"correct_file_path", "list_files"}
	if errors.Is(err, os.ErrPermission) {
		code = "read_artifact_permission_denied"
		category = tool.ErrorCategoryAuthorization
		recovery = []string{"choose_readable_file", "adjust_file_permissions"}
	}
	outcome := tool.FailedOutcome(err.Error(), code, category, false)
	outcome.RecoveryOptions = recovery
	outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": resolvedPath}
	return outcome
}

func readArtifactFailure(err error, ext, requestedPath, resolvedPath string) tool.ExecutionOutcome {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		outcome := tool.FailedOutcome(err.Error(), "read_artifact_cancelled", tool.ErrorCategoryCancelled, true)
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.RecoveryOptions = []string{"retry_when_context_available"}
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": resolvedPath, "extension": ext}
		return outcome
	}
	if errors.Is(err, os.ErrPermission) {
		outcome := tool.FailedOutcome(err.Error(), "read_artifact_permission_denied", tool.ErrorCategoryAuthorization, false)
		outcome.RecoveryOptions = []string{"choose_readable_file", "adjust_file_permissions"}
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": resolvedPath, "extension": ext}
		return outcome
	}
	if errors.Is(err, errXLSXVariantUnsupported) {
		outcome := tool.FailedOutcome(err.Error(), "read_artifact_specialized_tool_required", tool.ErrorCategoryDomain, false)
		outcome.RecoveryOptions = []string{"use_specialized_artifact_tool", "use_spreadsheet_skill_or_library", "install_missing_dependency"}
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": resolvedPath, "extension": ext, "file_intact": true}
		return outcome
	}
	code := "read_artifact_invalid_or_corrupt"
	category := tool.ErrorCategoryDomain
	recovery := []string{"verify_artifact_integrity", "use_specialized_artifact_tool"}
	if ext == ".pdf" || ext == ".xls" {
		code = "read_artifact_runtime_failed"
		category = tool.ErrorCategoryRuntime
		recovery = []string{"install_missing_dependency", "verify_artifact_integrity", "retry_or_use_specialized_artifact_tool"}
	}
	outcome := tool.FailedOutcome(err.Error(), code, category, false)
	outcome.RecoveryOptions = recovery
	outcome.Diagnostic = map[string]any{"requested_path": requestedPath, "resolved_path": resolvedPath, "extension": ext}
	return outcome
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
		return artifactPreview{}, fmt.Errorf("暂不支持读取该文件类型: %s。read_artifact 支持 .csv/.tsv/.xlsx/.xls/.docx/.pdf；普通文本请用 read_file，图片请用 image_inspect，PPTX 请用 artifact_inspect 或 presentation_render", ext)
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
	converted := filepath.Join(convertDir, strings.TrimSuffix(filepath.Base(params.Path), filepath.Ext(params.Path))+".xlsx")
	if _, err := officerender.Render(ctx, params.Path, converted, "xlsx"); err != nil {
		return artifactPreview{}, err
	}
	return readXLSXArtifact(converted, params.Sheet, normalizeArtifactMaxRows(params.MaxRows), normalizeArtifactMaxSheets(params.MaxSheets), normalizeArtifactMaxChars(params.MaxChars))
}

// errXLSXVariantUnsupported marks failures where the archive passed ZIP
// integrity checks but the lightweight built-in reader cannot handle the
// workbook variant. These must never be reported as invalid_or_corrupt; the
// agent should route to a specialized spreadsheet tool or library instead.
var errXLSXVariantUnsupported = errors.New("read_artifact 内置 XLSX 预览不支持该工作簿变体")

func xlsxVariantUnsupported(err error) error {
	return fmt.Errorf("%w（文件 ZIP 完整性正常）: %v。请改用 spreadsheet Skill 或成熟库（如 openpyxl/pandas）读取", errXLSXVariantUnsupported, err)
}

func readXLSXArtifact(path string, sheetFilter string, maxRows int, maxSheets int, maxChars int) (artifactPreview, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return artifactPreview{}, err
	}
	defer zr.Close()

	workbookData, err := readZipEntry(&zr.Reader, "xl/workbook.xml")
	if err != nil {
		return artifactPreview{}, xlsxVariantUnsupported(err)
	}
	var workbook xlsxWorkbook
	if err := xml.Unmarshal(workbookData, &workbook); err != nil {
		return artifactPreview{}, xlsxVariantUnsupported(err)
	}
	relsData, err := readZipEntry(&zr.Reader, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return artifactPreview{}, xlsxVariantUnsupported(err)
	}
	var rels xlsxRelationships
	if err := xml.Unmarshal(relsData, &rels); err != nil {
		return artifactPreview{}, xlsxVariantUnsupported(err)
	}
	sharedStrings := readSharedStrings(&zr.Reader)
	targets := make(map[string]string, len(rels.Items))
	for _, rel := range rels.Items {
		resolved, ok := resolveOPCRelationshipTarget("xl/workbook.xml", rel)
		if !ok {
			continue
		}
		targets[rel.ID] = resolved
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
			return artifactPreview{}, xlsxVariantUnsupported(err)
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
		return artifactPreview{}, xlsxVariantUnsupported(fmt.Errorf("未找到可读取的工作表"))
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
	info, err := os.Stat(path)
	if err != nil {
		return artifactPreview{}, err
	}
	if info.Size() == 0 {
		return artifactPreview{}, fmt.Errorf("read_artifact 无法读取空 PDF 文件: %s。该文件大小为 0 字节，可能下载/复制失败或文件已损坏；请提供有效 PDF", path)
	}
	outputPath := filepath.Join(permCtx.TemporaryWorkRoot, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".txt")
	if _, err := localcompute.Run(ctx, permCtx, localcompute.CommandSpec{
		Backend:     "pdftotext",
		Args:        []string{"-enc", "UTF-8", "-nopgbrk", path, outputPath},
		InputPaths:  []string{path},
		OutputPaths: []string{outputPath},
		Workdir:     permCtx.TemporaryWorkRoot,
		Timeout:     30 * time.Second,
	}); err != nil {
		return artifactPreview{}, fmt.Errorf("read_artifact 无法提取 PDF 文本: %w。该 PDF 可能损坏、加密、为空或不是标准 PDF", err)
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

// resolveOPCRelationshipTarget resolves a relationship target following OPC
// (ECMA-376 Part 2) rules instead of naive string concatenation:
//   - external targets are not ZIP entries and are skipped;
//   - a target starting with "/" is a package absolute part name, used after
//     removing exactly one leading slash;
//   - other targets resolve relative to the directory of the source part
//     (the part owning the .rels file, e.g. xl/workbook.xml);
//   - the result is cleaned with path.Clean and must not escape the package
//     root through "..".
func resolveOPCRelationshipTarget(sourcePart string, rel xlsxRelationship) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(rel.TargetMode), "External") {
		return "", false
	}
	target := strings.TrimSpace(rel.Target)
	if target == "" {
		return "", false
	}
	target = strings.ReplaceAll(target, "\\", "/")
	var resolved string
	if strings.HasPrefix(target, "/") {
		resolved = strings.TrimPrefix(target, "/")
	} else {
		resolved = path.Join(path.Dir(sourcePart), target)
	}
	resolved = path.Clean(resolved)
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") || strings.HasPrefix(resolved, "/") {
		return "", false
	}
	return resolved, true
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
	tool.MustRegisterToolWithMode(&ReadArtifactTool{}, tool.DefinitionValidationStrictBuiltin)
}
