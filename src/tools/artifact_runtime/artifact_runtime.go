package artifactruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"awaken/src/tool"
)

const runtimeName = "primary-runtime"

type runtimeComponent struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
	Path     string `json:"path,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
}

type runtimeManifest struct {
	Name       string             `json:"name"`
	Version    string             `json:"version"`
	Mode       string             `json:"mode"`
	Components []runtimeComponent `json:"components"`
}

type preflightReport struct {
	RuntimeName  string             `json:"runtimeName"`
	Ready        bool               `json:"ready"`
	ManifestPath string             `json:"manifestPath,omitempty"`
	Missing      []string           `json:"missing,omitempty"`
	Components   []runtimeComponent `json:"components"`
}

type typedArtifactTool struct {
	name            string
	displayName     string
	description     string
	readOnly        bool
	capability      tool.CapabilityClass
	requiresRuntime bool
	parameters      []tool.ToolParameter
	execute         func(context.Context, string) (string, error)
}

func (t typedArtifactTool) Name() string { return t.name }

func (t typedArtifactTool) Description() string { return t.description }

func (t typedArtifactTool) Parameters() []tool.ToolParameter {
	return append([]tool.ToolParameter(nil), t.parameters...)
}

func (t typedArtifactTool) IsReadOnly() bool { return t.readOnly }

func (t typedArtifactTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		DisplayName: t.displayName,
		Source:      tool.SourceBuiltin,
		Description: t.description,
		Annotations: tool.ToolAnnotations{
			ReadOnly:        t.readOnly,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: t.capability,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: fileBridgeScopeForTool(t.readOnly),
			NetworkScope:    tool.NetworkScopeNone,
		},
	}
}

func (t typedArtifactTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if t.requiresRuntime {
		report := runtimePreflight()
		if !report.Ready {
			return tool.NeedsRuntimePermission(fmt.Sprintf("%s is not ready: missing %s", runtimeName, strings.Join(report.Missing, ", ")))
		}
	}
	params := map[string]any{}
	_ = json.Unmarshal([]byte(input), &params)
	if t.readOnly {
		if path := firstString(params, "path", "input_path", "source_path"); path != "" {
			if !permissionContext.IsWithinReadableRootsResolved(path) {
				return tool.DenyPermission(fmt.Sprintf("path %s is outside readable roots or escapes via symlink", path))
			}
		}
		return tool.AllowPermission()
	}
	inputPath := firstString(params, "input_path", "source_path")
	if inputPath == "" && toolRequiresInputPath(t.name) {
		inputPath = firstString(params, "path")
	}
	if inputPath != "" {
		if !permissionContext.IsWithinReadableRootsResolved(inputPath) {
			return tool.DenyPermission(fmt.Sprintf("path %s is outside readable roots or escapes via symlink", inputPath))
		}
	}
	outputPath := firstString(params, "output_path", "target_path")
	if outputPath == "" && !toolRequiresInputPath(t.name) {
		outputPath = firstString(params, "path")
	}
	if outputPath != "" {
		if _, err := permissionContext.ResolveWritablePathChecked(outputPath); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermission()
}

func toolRequiresInputPath(name string) bool {
	switch name {
	case "artifact_export", "image_convert":
		return true
	default:
		return false
	}
}

func (t typedArtifactTool) Execute(ctx context.Context, input string) (string, error) {
	if t.execute != nil {
		return t.execute(ctx, input)
	}
	if t.requiresRuntime {
		report := runtimePreflight()
		if !report.Ready {
			data, _ := json.MarshalIndent(report, "", "  ")
			return "", fmt.Errorf("缺少 %s，无法执行 %s: %s", runtimeName, t.name, string(data))
		}
	}
	return "", fmt.Errorf("%s requires a prepared artifact runtime executor", t.name)
}

func workspaceDependenciesTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "workspace_dependencies",
		displayName: "Workspace Dependencies",
		description: "返回 Awaken 打包/准备的本地 Work runtime 依赖清单",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters:  nil,
		execute: func(context.Context, string) (string, error) {
			report := runtimePreflight()
			data, _ := json.MarshalIndent(report, "", "  ")
			return string(data), nil
		},
	}
}

func runtimePreflightTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "runtime_preflight",
		displayName: "Runtime Preflight",
		description: "检查 Office/PDF/媒体 typed artifact tools 所需本地 runtime 是否准备完成",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters:  nil,
		execute: func(context.Context, string) (string, error) {
			report := runtimePreflight()
			data, _ := json.MarshalIndent(report, "", "  ")
			return string(data), nil
		},
	}
}

func artifactInspectTool(name, displayName, description string) typedArtifactTool {
	return typedArtifactTool{
		name:        name,
		displayName: displayName,
		description: description,
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "工作区内或附件中的文件路径", Type: "string", Required: true},
		},
		execute: artifactReadOrInspect,
	}
}

func artifactExportTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "artifact_export",
		displayName: "Artifact Export",
		description: "将 artifact 复制/导出到 output roots 内的指定路径",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "输入文件路径", Type: "string", Required: true},
			{Name: "output_path", Description: "输出文件路径", Type: "string", Required: true},
		},
		execute: artifactExport,
	}
}

func runtimeBackedTool(name, displayName, description string, readOnly bool) typedArtifactTool {
	capability := tool.CapabilityArtifactWrite
	if readOnly {
		capability = tool.CapabilityArtifactRead
	}
	params := []tool.ToolParameter{
		{Name: "path", Description: "输入文件路径", Type: "string", Required: readOnly},
	}
	if !readOnly {
		params = append(params, tool.ToolParameter{Name: "output_path", Description: "输出文件路径，必须在 writable/output roots 内", Type: "string", Required: false})
	}
	return typedArtifactTool{
		name:            name,
		displayName:     displayName,
		description:     description,
		readOnly:        readOnly,
		capability:      capability,
		requiresRuntime: true,
		parameters:      params,
	}
}

func fileBridgeScopeForTool(readOnly bool) tool.FileBridgeScope {
	if readOnly {
		return tool.FileBridgeTaskScopeRead
	}
	return tool.FileBridgeTaskScopeWrite
}

func artifactReadOrInspect(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	path := firstString(params, "path", "input_path", "source_path")
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	perm := tool.PermissionContextFromContext(ctx)
	checked, err := perm.ResolveReadablePathChecked(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(checked)
	if err != nil {
		return "", err
	}
	content := ""
	if !info.IsDir() && isTextLike(checked) && info.Size() <= 256*1024 {
		if data, err := os.ReadFile(checked); err == nil {
			content = string(data)
		}
	}
	payload := map[string]any{
		"path":           checked,
		"name":           info.Name(),
		"size":           info.Size(),
		"isDirectory":    info.IsDir(),
		"extension":      strings.ToLower(filepath.Ext(info.Name())),
		"modifiedAtUnix": info.ModTime().Unix(),
	}
	if content != "" {
		payload["content"] = content
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data), nil
}

func artifactExport(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	source := firstString(params, "path", "input_path", "source_path")
	if source == "" {
		return "", fmt.Errorf("path is required")
	}
	output := firstString(params, "output_path", "target_path")
	if output == "" {
		return "", fmt.Errorf("output_path is required")
	}
	perm := tool.PermissionContextFromContext(ctx)
	checkedSource, err := perm.ResolveReadablePathChecked(source)
	if err != nil {
		return "", err
	}
	checkedOutput, err := perm.ResolveWritablePathChecked(output)
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(checkedOutput); err != nil {
		return "", err
	}
	in, err := os.Open(checkedSource)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(checkedOutput)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return jsonResult("exported", checkedOutput)
}

func spreadsheetCreateOrEdit(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	output, err := writableOutputPath(ctx, params, ".xlsx")
	if err != nil {
		return "", err
	}
	rows := rowsFromParams(params)
	if len(rows) == 0 {
		rows = [][]string{{"Column A", "Column B"}, {"", ""}}
	}
	switch strings.ToLower(filepath.Ext(output)) {
	case ".csv", ".tsv":
		sep := ","
		if strings.EqualFold(filepath.Ext(output), ".tsv") {
			sep = "\t"
		}
		if err := writeDelimited(output, rows, sep); err != nil {
			return "", err
		}
	default:
		if err := writeMinimalXLSX(output, rows); err != nil {
			return "", err
		}
	}
	return jsonResult("spreadsheet_written", output)
}

func documentCreateOrEdit(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	output, err := writableOutputPath(ctx, params, ".docx")
	if err != nil {
		return "", err
	}
	paragraphs := stringSliceParam(params, "paragraphs", "content")
	title := firstString(params, "title")
	if title != "" {
		paragraphs = append([]string{title}, paragraphs...)
	}
	if len(paragraphs) == 0 {
		paragraphs = []string{"New document"}
	}
	if strings.EqualFold(filepath.Ext(output), ".txt") || strings.EqualFold(filepath.Ext(output), ".md") {
		if err := os.WriteFile(output, []byte(strings.Join(paragraphs, "\n\n")), 0o644); err != nil {
			return "", err
		}
	} else if err := writeMinimalDOCX(output, paragraphs); err != nil {
		return "", err
	}
	return jsonResult("document_written", output)
}

func presentationCreateOrEdit(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	output, err := writableOutputPath(ctx, params, ".pptx")
	if err != nil {
		return "", err
	}
	slides := slidesFromParams(params)
	if len(slides) == 0 {
		slides = []slideData{{Title: firstNonEmpty(firstString(params, "title"), "New presentation"), Body: strings.Join(stringSliceParam(params, "bullets", "content"), "\n")}}
	}
	if err := writeMinimalPPTX(output, slides); err != nil {
		return "", err
	}
	return jsonResult("presentation_written", output)
}

func pdfExtract(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	path := firstString(params, "path", "input_path", "source_path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	checked, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(checked)
	if err != nil {
		return "", err
	}
	text := naivePDFText(data)
	payload := map[string]any{
		"path":       checked,
		"pagesHint":  strings.Count(string(data), "/Type /Page"),
		"text":       text,
		"ocr":        false,
		"extraction": "basic_pdf_text_no_ocr",
	}
	out, _ := json.MarshalIndent(payload, "", "  ")
	return string(out), nil
}

func imageInspect(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	path := firstString(params, "path", "input_path", "source_path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	checked, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(checked)
	if err != nil {
		return "", err
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"path":   checked,
		"format": format,
		"width":  cfg.Width,
		"height": cfg.Height,
		"ocr":    false,
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data), nil
}

func imageConvert(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	source := firstString(params, "path", "input_path", "source_path")
	if source == "" {
		return "", fmt.Errorf("path is required")
	}
	output, err := writableOutputPath(ctx, params, ".png")
	if err != nil {
		return "", err
	}
	perm := tool.PermissionContextFromContext(ctx)
	checkedSource, err := perm.ResolveReadablePathChecked(source)
	if err != nil {
		return "", err
	}
	in, err := os.Open(checkedSource)
	if err != nil {
		return "", err
	}
	img, _, err := image.Decode(in)
	_ = in.Close()
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(output); err != nil {
		return "", err
	}
	out, err := os.Create(output)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(output)) {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(out, img, &jpeg.Options{Quality: 92})
	default:
		err = png.Encode(out, img)
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return jsonResult("image_converted", output)
}

func mediaProbeBasic(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	path := firstString(params, "path", "input_path", "source_path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	checked, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(checked)
	if err != nil {
		return "", err
	}
	payload := map[string]any{
		"path":      checked,
		"size":      info.Size(),
		"extension": strings.ToLower(filepath.Ext(checked)),
		"probe":     "basic_file_metadata",
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data), nil
}

func decodeParams(input string) (map[string]any, error) {
	params := map[string]any{}
	if err := json.Unmarshal([]byte(input), &params); err != nil && strings.TrimSpace(input) != "" {
		return nil, err
	}
	return params, nil
}

func firstString(params map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func stringSliceParam(params map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return []string{strings.TrimSpace(typed)}
			}
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func writableOutputPath(ctx context.Context, params map[string]any, defaultExt string) (string, error) {
	output := firstString(params, "output_path", "path")
	if output == "" {
		output = "artifact" + defaultExt
	}
	if filepath.Ext(output) == "" && defaultExt != "" {
		output += defaultExt
	}
	checked, err := tool.PermissionContextFromContext(ctx).ResolveWritablePathChecked(output)
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(checked); err != nil {
		return "", err
	}
	return checked, nil
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func jsonResult(status, path string) (string, error) {
	payload := map[string]any{"status": status, "path": path}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data), nil
}

func isTextLike(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".csv", ".tsv", ".json", ".yaml", ".yml", ".xml", ".html", ".css", ".js", ".ts", ".go", ".py":
		return true
	default:
		return false
	}
}

func rowsFromParams(params map[string]any) [][]string {
	raw, ok := params["rows"].([]any)
	if !ok {
		return nil
	}
	rows := make([][]string, 0, len(raw))
	for _, row := range raw {
		rawCells, ok := row.([]any)
		if !ok {
			continue
		}
		cells := make([]string, 0, len(rawCells))
		for _, cell := range rawCells {
			cells = append(cells, fmt.Sprint(cell))
		}
		rows = append(rows, cells)
	}
	return rows
}

func writeDelimited(path string, rows [][]string, sep string) error {
	var builder strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				builder.WriteString(sep)
			}
			builder.WriteString(escapeDelimited(cell, sep))
		}
		builder.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func escapeDelimited(value, sep string) string {
	if strings.ContainsAny(value, "\"\n\r") || strings.Contains(value, sep) {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func writeMinimalXLSX(path string, rows [][]string) error {
	return writeZip(path, map[string]string{
		"[Content_Types].xml":        xlsxContentTypes(),
		"_rels/.rels":                officeRootRels("xl/workbook.xml"),
		"xl/workbook.xml":            xlsxWorkbook(),
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRels(),
		"xl/worksheets/sheet1.xml":   xlsxSheet(rows),
		"docProps/app.xml":           officeAppProps("Awaken Spreadsheet"),
		"docProps/core.xml":          officeCoreProps(),
	})
}

func writeMinimalDOCX(path string, paragraphs []string) error {
	return writeZip(path, map[string]string{
		"[Content_Types].xml": docxContentTypes(),
		"_rels/.rels":         officeRootRels("word/document.xml"),
		"word/document.xml":   docxDocument(paragraphs),
		"docProps/app.xml":    officeAppProps("Awaken Document"),
		"docProps/core.xml":   officeCoreProps(),
	})
}

type slideData struct {
	Title string
	Body  string
}

func slidesFromParams(params map[string]any) []slideData {
	raw, ok := params["slides"].([]any)
	if !ok {
		return nil
	}
	slides := make([]slideData, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		slides = append(slides, slideData{
			Title: firstString(obj, "title"),
			Body:  firstString(obj, "body", "content"),
		})
	}
	return slides
}

func writeMinimalPPTX(path string, slides []slideData) error {
	files := map[string]string{
		"[Content_Types].xml":             pptxContentTypes(len(slides)),
		"_rels/.rels":                     officeRootRels("ppt/presentation.xml"),
		"ppt/presentation.xml":            pptxPresentation(len(slides)),
		"ppt/_rels/presentation.xml.rels": pptxPresentationRels(len(slides)),
		"docProps/app.xml":                officeAppProps("Awaken Presentation"),
		"docProps/core.xml":               officeCoreProps(),
	}
	for i, slide := range slides {
		files[fmt.Sprintf("ppt/slides/slide%d.xml", i+1)] = pptxSlide(slide)
	}
	return writeZip(path, files)
}

func writeZip(path string, files map[string]string) error {
	if err := ensureParentDir(path); err != nil {
		return err
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	zipWriter := zip.NewWriter(out)
	for name, content := range files {
		entry, err := zipWriter.Create(name)
		if err != nil {
			_ = zipWriter.Close()
			_ = out.Close()
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			_ = zipWriter.Close()
			_ = out.Close()
			return err
		}
	}
	if err := zipWriter.Close(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func xmlText(value string) string {
	var buf bytes.Buffer
	_ = xmlEscapeText(&buf, value)
	return buf.String()
}

func xmlEscapeText(buf *bytes.Buffer, value string) error {
	for _, r := range value {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteRune(r)
		}
	}
	return nil
}

func officeRootRels(target string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="` + target + `"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
		`</Relationships>`
}

func officeAppProps(app string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>` + xmlText(app) + `</Application></Properties>`
}

func officeCoreProps() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:creator>Awaken</dc:creator></cp:coreProperties>`
}

func xlsxContentTypes() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		`</Types>`
}

func xlsxWorkbook() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`
}

func xlsxWorkbookRels() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
		`</Relationships>`
}

func xlsxSheet(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		rowIndex := r + 1
		b.WriteString(`<row r="` + strconv.Itoa(rowIndex) + `">`)
		for c, value := range row {
			cellRef := columnName(c+1) + strconv.Itoa(rowIndex)
			b.WriteString(`<c r="` + cellRef + `" t="inlineStr"><is><t>` + xmlText(value) + `</t></is></c>`)
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func columnName(index int) string {
	var out []byte
	for index > 0 {
		index--
		out = append([]byte{byte('A' + index%26)}, out...)
		index /= 26
	}
	return string(out)
}

func docxContentTypes() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		`</Types>`
}

func docxDocument(paragraphs []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paragraphs {
		b.WriteString(`<w:p><w:r><w:t xml:space="preserve">` + xmlText(p) + `</w:t></w:r></w:p>`)
	}
	b.WriteString(`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`)
	return b.String()
}

func pptxContentTypes(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`)
	for i := 1; i <= slideCount; i++ {
		b.WriteString(`<Override PartName="/ppt/slides/slide` + strconv.Itoa(i) + `.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`)
	}
	b.WriteString(`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/><Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/></Types>`)
	return b.String()
}

func pptxPresentation(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst>`)
	for i := 1; i <= slideCount; i++ {
		b.WriteString(`<p:sldId id="` + strconv.Itoa(255+i) + `" r:id="rId` + strconv.Itoa(i) + `"/>`)
	}
	b.WriteString(`</p:sldIdLst></p:presentation>`)
	return b.String()
}

func pptxPresentationRels(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= slideCount; i++ {
		b.WriteString(`<Relationship Id="rId` + strconv.Itoa(i) + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide` + strconv.Itoa(i) + `.xml"/>`)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func pptxSlide(slide slideData) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>` + xmlText(slide.Title) + `</a:t></a:r></a:p></p:txBody></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Body"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>` + xmlText(slide.Body) + `</a:t></a:r></a:p></p:txBody></p:sp>` +
		`</p:spTree></p:cSld></p:sld>`
}

func naivePDFText(data []byte) string {
	raw := string(data)
	var parts []string
	for {
		start := strings.Index(raw, "(")
		if start < 0 {
			break
		}
		raw = raw[start+1:]
		end := strings.Index(raw, ")")
		if end < 0 {
			break
		}
		part := strings.ReplaceAll(raw[:end], `\)`, `)`)
		part = strings.ReplaceAll(part, `\(`, `(`)
		if strings.TrimSpace(part) != "" {
			parts = append(parts, part)
		}
		raw = raw[end+1:]
		if len(strings.Join(parts, "\n")) > 20000 {
			break
		}
	}
	return strings.Join(parts, "\n")
}

func runtimePreflight() preflightReport {
	manifestPath := runtimeManifestPath()
	report := preflightReport{RuntimeName: runtimeName, ManifestPath: manifestPath}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		report.Missing = []string{"runtime_manifest"}
		return report
	}
	var manifest runtimeManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		report.Missing = []string{"valid_runtime_manifest"}
		return report
	}
	report.RuntimeName = firstNonEmpty(manifest.Name, runtimeName)
	report.Components = manifest.Components
	for _, component := range manifest.Components {
		if !component.Required {
			continue
		}
		if strings.TrimSpace(component.Path) == "" {
			report.Missing = append(report.Missing, component.Name)
			continue
		}
		path := component.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(manifestPath), path)
		}
		if _, err := os.Stat(path); err != nil {
			report.Missing = append(report.Missing, component.Name)
			continue
		}
		if expected := strings.TrimSpace(component.SHA256); expected != "" {
			actual, err := fileSHA256(path)
			if err != nil || !strings.EqualFold(actual, expected) {
				report.Missing = append(report.Missing, component.Name+"_checksum")
			}
		}
	}
	report.Ready = len(report.Missing) == 0
	return report
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func runtimeManifestPath() string {
	if path := strings.TrimSpace(os.Getenv("AWAKEN_RUNTIME_MANIFEST")); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("AWAKEN_RUNTIME_ROOT")); path != "" {
		return filepath.Join(path, "primary-runtime.json")
	}
	candidates := []string{
		filepath.Join("apps", "desktop", "resources", "runtime-manifest", "primary-runtime.json"),
		filepath.Join("resources", "runtime-manifest", "primary-runtime.json"),
		filepath.Join("runtime-manifest", "primary-runtime.json"),
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "..", "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "Resources", "runtime-manifest", "primary-runtime.json"),
			filepath.Join(dir, "..", "..", "Resources", "runtime-manifest", "primary-runtime.json"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func init() {
	tool.MustRegisterTool(workspaceDependenciesTool())
	tool.MustRegisterTool(runtimePreflightTool())
	tool.MustRegisterTool(artifactInspectTool("artifact_read", "Artifact Read", "读取工作区 artifact 的基础文件信息"))
	tool.MustRegisterTool(artifactInspectTool("artifact_inspect", "Artifact Inspect", "检查工作区 artifact 类型、大小与基础元数据"))
	tool.MustRegisterTool(artifactExportTool())
	tool.MustRegisterTool(typedArtifactTool{
		name:        "spreadsheet_create",
		displayName: "Spreadsheet Create",
		description: "创建 xlsx/csv 表格文件",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 xlsx/csv 路径", Type: "string", Required: true},
			{Name: "rows", Description: "二维数组数据", Type: "array", Required: false},
		},
		execute: spreadsheetCreateOrEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "spreadsheet_edit",
		displayName: "Spreadsheet Edit",
		description: "写入或重写 xlsx/csv 表格文件",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 xlsx/csv 路径", Type: "string", Required: true},
			{Name: "rows", Description: "二维数组数据", Type: "array", Required: false},
		},
		execute: spreadsheetCreateOrEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "document_create",
		displayName: "Document Create",
		description: "创建 docx/txt/md 文档",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 docx/txt/md 路径", Type: "string", Required: true},
			{Name: "paragraphs", Description: "段落数组", Type: "array", Required: false},
			{Name: "title", Description: "标题", Type: "string", Required: false},
		},
		execute: documentCreateOrEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "document_edit",
		displayName: "Document Edit",
		description: "写入或重写 docx/txt/md 文档",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 docx/txt/md 路径", Type: "string", Required: true},
			{Name: "paragraphs", Description: "段落数组", Type: "array", Required: false},
			{Name: "title", Description: "标题", Type: "string", Required: false},
		},
		execute: documentCreateOrEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "presentation_create",
		displayName: "Presentation Create",
		description: "创建 pptx 演示文稿",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 pptx 路径", Type: "string", Required: true},
			{Name: "slides", Description: "幻灯片数组，每项包含 title/body", Type: "array", Required: false},
		},
		execute: presentationCreateOrEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "presentation_edit",
		displayName: "Presentation Edit",
		description: "写入或重写 pptx 演示文稿",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 pptx 路径", Type: "string", Required: true},
			{Name: "slides", Description: "幻灯片数组，每项包含 title/body", Type: "array", Required: false},
		},
		execute: presentationCreateOrEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "pdf_extract",
		displayName: "PDF Extract",
		description: "抽取 PDF 文本和页面信息，不执行 OCR",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "PDF 文件路径", Type: "string", Required: true},
		},
		execute: pdfExtract,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "image_inspect",
		displayName: "Image Inspect",
		description: "读取图片尺寸和格式信息，不执行 OCR",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "图片文件路径", Type: "string", Required: true},
		},
		execute: imageInspect,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "image_convert",
		displayName: "Image Convert",
		description: "转换图片为 png/jpg",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "输入图片路径", Type: "string", Required: true},
			{Name: "output_path", Description: "输出 png/jpg 路径", Type: "string", Required: true},
		},
		execute: imageConvert,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "media_probe",
		displayName: "Media Probe",
		description: "读取音视频文件基础元数据；深度 probe 由 ffmpeg runtime 提供",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "音视频文件路径", Type: "string", Required: true},
		},
		execute: mediaProbeBasic,
	})

	runtimeReady := runtimePreflight().Ready
	for _, spec := range []struct {
		name        string
		displayName string
		description string
		readOnly    bool
	}{
		{"artifact_render", "Artifact Render", "将 artifact 渲染为预览产物", true},
		{"spreadsheet_chart", "Spreadsheet Chart", "为表格生成图表产物", false},
		{"document_render", "Document Render", "渲染 docx 文档预览", true},
		{"presentation_render", "Presentation Render", "渲染 pptx 演示文稿预览", true},
		{"pdf_render", "PDF Render", "渲染 PDF 页面预览", true},
	} {
		if runtimeReady {
			tool.MustRegisterTool(runtimeBackedTool(spec.name, spec.displayName, spec.description, spec.readOnly))
		}
	}
}
