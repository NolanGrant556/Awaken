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
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"
)

type typedArtifactTool struct {
	name            string
	displayName     string
	description     string
	readOnly        bool
	capability      tool.CapabilityClass
	requiresRuntime bool
	parameters      []tool.ToolParameter
	inputSchema     *tool.ToolParametersSchema
	execute         func(context.Context, string) (string, error)
}

type presentationTheme struct {
	Background string `json:"background,omitempty"`
	Surface    string `json:"surface,omitempty"`
	Primary    string `json:"primary,omitempty"`
	Accent     string `json:"accent,omitempty"`
	Text       string `json:"text,omitempty"`
	Muted      string `json:"muted,omitempty"`
	FontLatin  string `json:"font_latin,omitempty"`
	FontEA     string `json:"font_ea,omitempty"`
}

type presentationImageData struct {
	Path    string  `json:"path,omitempty"`
	Alt     string  `json:"alt,omitempty"`
	Caption string  `json:"caption,omitempty"`
	Fit     string  `json:"fit,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Width   float64 `json:"width,omitempty"`
	Height  float64 `json:"height,omitempty"`
}

type slideData struct {
	Title      string                  `json:"title,omitempty"`
	Subtitle   string                  `json:"subtitle,omitempty"`
	Body       string                  `json:"body,omitempty"`
	Layout     string                  `json:"layout,omitempty"`
	Notes      string                  `json:"notes,omitempty"`
	Background string                  `json:"background,omitempty"`
	Accent     string                  `json:"accent,omitempty"`
	Bullets    []string                `json:"bullets,omitempty"`
	Images     []presentationImageData `json:"images,omitempty"`
	Columns    []map[string]any        `json:"columns,omitempty"`
	Metrics    []map[string]any        `json:"metrics,omitempty"`
	Timeline   []map[string]any        `json:"timeline,omitempty"`
}

type presentationSpec struct {
	OutputPath    string            `json:"output_path"`
	Title         string            `json:"title,omitempty"`
	Theme         presentationTheme `json:"theme,omitempty"`
	Slides        []slideData       `json:"slides"`
	RenderPreview bool              `json:"render_preview"`
	PreviewDir    string            `json:"preview_dir,omitempty"`
}

type presentationEditSpec struct {
	SourcePath    string            `json:"source_path"`
	OutputPath    string            `json:"output_path"`
	Theme         presentationTheme `json:"theme,omitempty"`
	Edits         []map[string]any  `json:"edits"`
	RenderPreview bool              `json:"render_preview"`
	PreviewDir    string            `json:"preview_dir,omitempty"`
	SlideCount    int               `json:"-"`
}

type presentationRenderReport struct {
	Verified           bool     `json:"verified"`
	VerificationLevel  string   `json:"verification_level"`
	Engine             string   `json:"engine"`
	PDFPath            string   `json:"pdf_path,omitempty"`
	PreviewDir         string   `json:"preview_dir,omitempty"`
	PreviewImages      []string `json:"preview_images,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	NeedsRuntime       bool     `json:"needs_runtime,omitempty"`
	SlideCount         int      `json:"slide_count,omitempty"`
	PDFPageCount       int      `json:"pdf_page_count,omitempty"`
	PNGCount           int      `json:"png_count,omitempty"`
}

var supportedPresentationLayouts = map[string]struct{}{
	"title": {}, "section": {}, "bullets": {}, "two_column": {},
	"image_right": {}, "comparison": {}, "metrics": {}, "timeline": {}, "closing": {},
}

func (t typedArtifactTool) Name() string { return t.name }

func (t typedArtifactTool) Description() string { return t.description }

func (t typedArtifactTool) Parameters() []tool.ToolParameter {
	return append([]tool.ToolParameter(nil), t.parameters...)
}

func (t typedArtifactTool) InputSchema() tool.ToolParametersSchema {
	if t.inputSchema != nil {
		return *t.inputSchema
	}
	return tool.SchemaFromParameters(t.Parameters())
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

func (t typedArtifactTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	if t.readOnly {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}
	var artifacts []tool.ArtifactDescriptor
	if path := firstString(payload, "path"); path != "" && !strings.HasSuffix(t.name, "_render") {
		artifacts = append(artifacts, artifactDescriptorForPath(path, permissionContext, t.name))
	}
	if pdfPath := firstString(payload, "pdf_path"); pdfPath != "" {
		artifacts = append(artifacts, artifactDescriptorForPath(pdfPath, permissionContext, t.name))
	}
	for _, imagePath := range stringSliceParam(payload, "preview_images") {
		artifacts = append(artifacts, artifactDescriptorForPath(imagePath, permissionContext, t.name))
	}
	return artifacts
}

func (t typedArtifactTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if t.requiresRuntime {
		report := runtimePreflightCached(true)
		if !report.Ready {
			return tool.NeedsRuntimePermission(fmt.Sprintf("%s is not ready: missing %s", runtimeName, strings.Join(report.Missing, ", ")))
		}
	}
	params, _ := decodeParams(input, singleRawParamName(t.parameters))
	if t.readOnly {
		return tool.AllowPermission()
	}
	outputPath := firstString(params, "output_path", "target_path", "output_dir", "preview_dir")
	if outputPath == "" && t.name == "presentation_edit" {
		outputPath = firstString(params, "source_path")
	}
	if outputPath == "" && !toolRequiresInputPath(t.name) {
		outputPath = firstString(params, "path")
	}
	if outputPath != "" {
		if _, err := resolveArtifactOutputPathChecked(permissionContext, outputPath); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermission()
}

func toolRequiresInputPath(name string) bool {
	switch name {
	case "artifact_export", "artifact_render", "image_convert", "presentation_render":
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
		report := runtimePreflightCached(true)
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
		description: "返回 Awaken 打包/准备的本地 Work runtime 依赖清单，例如 Python、LibreOffice/soffice 等路径；需要确认本机文档/PPT/PDF 渲染依赖时用这个工具，不要用 list_files/grep_search 去扫描系统目录",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters:  nil,
		execute: func(context.Context, string) (string, error) {
			report := runtimePreflightCached(true)
			data, _ := json.MarshalIndent(report, "", "  ")
			return string(data), nil
		},
	}
}

func runtimePreflightTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "runtime_preflight",
		displayName: "Runtime Preflight",
		description: "检查 Office/PDF/媒体 typed artifact tools 所需本地 runtime 是否准备完成；文档、表格、PPT、PDF 渲染/转换前可先调用，缺失时返回具体 runtime 缺口",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters:  nil,
		execute: func(context.Context, string) (string, error) {
			report := runtimePreflightCached(true)
			data, _ := json.MarshalIndent(report, "", "  ")
			return string(data), nil
		},
	}
}

func artifactInspectTool(name, displayName, description string) typedArtifactTool {
	return typedArtifactTool{
		name:        name,
		displayName: displayName,
		description: description + "；只返回文件类型、大小、扩展名和小型文本内容等基础元数据，不抽取 PDF/Office 深层内容。PDF/Office/表格内容请用 read_artifact 或 pdf_extract，图片尺寸请用 image_inspect",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "本机文件路径，可用绝对路径、相对当前工作区路径或已上传附件文件名", Type: "string", Required: true},
		},
		execute: artifactReadOrInspect,
	}
}

func artifactExportTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "artifact_export",
		displayName: "Artifact Export",
		description: "将本机 artifact 复制/导出到 writable/output roots 内的指定路径；不解析文件内容，仅复制文件",
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
		description:     description + "；需要 primary-runtime 就绪，执行前可用 runtime_preflight 检查依赖",
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
	params, err := decodeParams(input, "path")
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
	checkedOutput, err := resolveArtifactOutputPathChecked(perm, output)
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

func artifactRender(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	source := firstString(params, "path", "input_path", "source_path")
	if source == "" {
		return "", fmt.Errorf("path is required")
	}
	checkedSource, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(source)
	if err != nil {
		return "", err
	}
	outputDir, err := presentationRenderOutputDir(ctx, params, checkedSource)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(checkedSource)) {
	case ".ppt", ".pptx":
		slideCount, _ := countPPTXSlides(checkedSource)
		report, err := renderPresentation(ctx, checkedSource, outputDir, slideCount)
		if err != nil {
			return "", err
		}
		return jsonResultWithFields("artifact_rendered", checkedSource, map[string]any{
			"kind":               "presentation",
			"verified":           report.Verified,
			"verification_level": report.VerificationLevel,
			"render_check":       report.Engine,
			"pdf_path":           report.PDFPath,
			"preview_dir":        report.PreviewDir,
			"preview_images":     report.PreviewImages,
			"warnings":           report.Warnings,
			"needs_runtime":      report.NeedsRuntime,
			"slide_count":        report.SlideCount,
			"pdf_page_count":     report.PDFPageCount,
			"png_count":          report.PNGCount,
		})
	case ".pdf":
		images, err := renderPDFPagesToPNG(ctx, checkedSource, outputDir)
		if err != nil {
			return "", err
		}
		return jsonResultWithFields("artifact_rendered", checkedSource, map[string]any{
			"kind":           "pdf",
			"verified":       true,
			"render_check":   "poppler_png",
			"pdf_path":       checkedSource,
			"preview_dir":    outputDir,
			"preview_images": images,
		})
	default:
		return "", fmt.Errorf("artifact_render 暂不支持渲染该文件类型: %s。PPTX 请用 presentation_render，PDF 请用 pdf_render；Office 文档深度渲染仍需补 executor", filepath.Ext(checkedSource))
	}
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

func presentationCreate(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	output, err := writableOutputPath(ctx, params, ".pptx")
	if err != nil {
		return "", err
	}
	spec, err := presentationSpecFromParams(ctx, params, output)
	if err != nil {
		return "", err
	}
	report, err := writeVerifiedPresentation(ctx, spec)
	if err != nil {
		return "", err
	}
	return presentationResultJSON("presentation_written", output, report, len(spec.Slides))
}

func presentationEdit(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	source := firstString(params, "source_path")
	if source == "" {
		return "", fmt.Errorf("source_path is required for presentation_edit")
	}
	editsRaw, ok := params["edits"]
	if !ok {
		return "", fmt.Errorf("edits is required for presentation_edit")
	}
	editsList, ok := editsRaw.([]any)
	if !ok || len(editsList) == 0 {
		return "", fmt.Errorf("edits must be a non-empty array")
	}
	edits := make([]map[string]any, 0, len(editsList))
	for i, item := range editsList {
		obj, ok := item.(map[string]any)
		if !ok {
			return "", fmt.Errorf("edits[%d] must be an object", i)
		}
		edits = append(edits, obj)
	}
	checkedSource, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(source)
	if err != nil {
		return "", err
	}
	output := firstString(params, "output_path", "path")
	if output == "" {
		output = checkedSource
	}
	checkedOutput, err := resolveArtifactOutputPathChecked(tool.PermissionContextFromContext(ctx), output)
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(checkedOutput); err != nil {
		return "", err
	}
	spec := presentationEditSpec{
		SourcePath:    checkedSource,
		OutputPath:    checkedOutput,
		Theme:         presentationThemeFromValue(params["theme"]),
		Edits:         edits,
		RenderPreview: true,
	}
	if value, ok := boolParam(params, "render_preview", "preview"); ok {
		spec.RenderPreview = value
	}
	if spec.RenderPreview {
		spec.PreviewDir = defaultPresentationPreviewDir(checkedOutput)
	}
	if err := editPPTXWithPython(ctx, spec); err != nil {
		return "", err
	}
	slideCount, _ := countPPTXSlides(checkedOutput)
	report, err := verifyPresentationPackage(ctx, checkedOutput, spec.PreviewDir, spec.RenderPreview, slideCount)
	if err != nil {
		return "", err
	}
	return presentationResultJSON("presentation_edited", checkedOutput, report, slideCount)
}

func presentationResultJSON(status, path string, report presentationRenderReport, slideCount int) (string, error) {
	fields := map[string]any{
		"verified":            report.Verified,
		"verification_level":  report.VerificationLevel,
		"render_check":        report.Engine,
		"pdf_path":            report.PDFPath,
		"preview_dir":         report.PreviewDir,
		"preview_images":      report.PreviewImages,
		"warnings":            report.Warnings,
		"slide_count":         slideCount,
		"pdf_page_count":      report.PDFPageCount,
		"png_count":           report.PNGCount,
	}
	if report.NeedsRuntime {
		fields["needs_runtime"] = true
	}
	return jsonResultWithFields(status, path, fields)
}

func presentationRender(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	source := firstString(params, "path", "input_path", "source_path")
	if source == "" {
		return "", fmt.Errorf("path is required")
	}
	checkedSource, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(source)
	if err != nil {
		return "", err
	}
	outputDir, err := presentationRenderOutputDir(ctx, params, checkedSource)
	if err != nil {
		return "", err
	}
	slideCount, _ := countPPTXSlides(checkedSource)
	report, err := renderPresentation(ctx, checkedSource, outputDir, slideCount)
	if err != nil {
		return "", err
	}
	return presentationResultJSON("presentation_rendered", checkedSource, report, slideCount)
}

func pdfExtract(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input, "path")
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
	if len(data) == 0 {
		return "", fmt.Errorf("pdf_extract 无法处理空 PDF 文件: %s。该文件大小为 0 字节，可能下载/复制失败或文件已损坏；请提供有效 PDF，普通文本请用 read_file，Office/表格/PDF 工作文件预览请用 read_artifact", checked)
	}
	if !looksLikePDF(data) {
		return "", fmt.Errorf("pdf_extract 只能处理有效 PDF 文件: %s。当前文件缺少 PDF 文件头，可能不是 PDF 或已损坏；普通文本请用 read_file，Office/表格/PDF 工作文件预览请用 read_artifact，图片请用 image_inspect", checked)
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
	params, err := decodeParams(input, "path")
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
		return "", fmt.Errorf("无法识别图片格式: %w。image_inspect 仅用于 PNG/JPEG/GIF 等可解码图片；PDF 请用 pdf_extract 或 read_artifact，Office/表格请用 read_artifact，音视频请用 media_probe", err)
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
		return "", fmt.Errorf("无法解码图片: %w。image_convert 仅转换 PNG/JPEG/GIF 等可解码图片；PDF 渲染请用 pdf_render 或 pdf_extract/read_artifact，Office 文件请用 read_artifact", err)
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
	params, err := decodeParams(input, "path")
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

func decodeParams(input string, rawSingleParamName ...string) (map[string]any, error) {
	params := map[string]any{}
	if err := json.Unmarshal([]byte(input), &params); err != nil && strings.TrimSpace(input) != "" {
		if len(rawSingleParamName) > 0 {
			name := strings.TrimSpace(rawSingleParamName[0])
			trimmed := strings.TrimSpace(input)
			if name != "" && !looksLikeJSONContainer(trimmed) {
				return map[string]any{name: trimmed}, nil
			}
		}
		return nil, err
	}
	return params, nil
}

func singleRawParamName(params []tool.ToolParameter) string {
	if len(params) != 1 {
		return ""
	}
	return strings.TrimSpace(params[0].Name)
}

func looksLikeJSONContainer(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[")
}

func looksLikePDF(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return bytes.HasPrefix(trimmed, []byte("%PDF-"))
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

func boolParam(params map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes", "y":
				return true, true
			case "false", "0", "no", "n":
				return false, true
			}
		}
	}
	return false, false
}

func floatParam(params map[string]any, keys ...string) float64 {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed
		case float32:
			return float64(typed)
		case int:
			return float64(typed)
		case json.Number:
			if v, err := typed.Float64(); err == nil {
				return v
			}
		case string:
			if v, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				return v
			}
		}
	}
	return 0
}

func presentationThemeFromValue(value any) presentationTheme {
	obj, ok := value.(map[string]any)
	if !ok {
		return presentationTheme{}
	}
	return presentationTheme{
		Background: firstString(obj, "background"),
		Surface:    firstString(obj, "surface"),
		Primary:    firstString(obj, "primary"),
		Accent:     firstString(obj, "accent"),
		Text:       firstString(obj, "text"),
		Muted:      firstString(obj, "muted"),
		FontLatin:  firstString(obj, "font_latin"),
		FontEA:     firstString(obj, "font_ea"),
	}
}

func presentationImagesFromValue(value any) []presentationImageData {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	images := make([]presentationImageData, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		images = append(images, presentationImageData{
			Path:    firstString(obj, "path", "input_path", "source_path"),
			Alt:     firstString(obj, "alt", "alt_text"),
			Caption: firstString(obj, "caption"),
			Fit:     firstString(obj, "fit"),
			X:       floatParam(obj, "x", "left"),
			Y:       floatParam(obj, "y", "top"),
			Width:   floatParam(obj, "width", "w"),
			Height:  floatParam(obj, "height", "h"),
		})
	}
	return images
}

func collectPresentationImagePaths(params map[string]any) []string {
	var paths []string
	slides, err := slidesFromParams(params)
	if err != nil {
		return nil
	}
	for _, slide := range slides {
		for _, image := range slide.Images {
			if strings.TrimSpace(image.Path) != "" {
				paths = append(paths, image.Path)
			}
		}
	}
	return paths
}

func writableOutputPath(ctx context.Context, params map[string]any, defaultExt string) (string, error) {
	output := firstString(params, "output_path", "path")
	if output == "" {
		output = "artifact" + defaultExt
	}
	if filepath.Ext(output) == "" && defaultExt != "" {
		output += defaultExt
	}
	checked, err := resolveArtifactOutputPathChecked(tool.PermissionContextFromContext(ctx), output)
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(checked); err != nil {
		return "", err
	}
	return checked, nil
}

func presentationRenderOutputDir(ctx context.Context, params map[string]any, sourcePath string) (string, error) {
	output := firstString(params, "output_dir", "preview_dir", "output_path", "target_path")
	if output == "" {
		output = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)) + "_preview"
	}
	checked, err := resolveArtifactOutputPathChecked(tool.PermissionContextFromContext(ctx), output)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(checked, 0o755); err != nil {
		return "", err
	}
	return checked, nil
}

func resolveArtifactOutputPathChecked(permissionContext tool.PermissionContext, path string) (string, error) {
	if checked, err := permissionContext.ResolveWritablePathChecked(path); err == nil {
		return checked, nil
	}
	if checked, err := permissionContext.ResolveOutputPathChecked(path); err == nil {
		return checked, nil
	}
	resolved := permissionContext.ResolvePath(path)
	return "", fmt.Errorf("path %q resolves to %q which is outside writable/output roots", path, resolved)
}

func defaultPresentationPreviewDir(outputPath string) string {
	dir := filepath.Dir(outputPath)
	base := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))
	if strings.TrimSpace(base) == "" || base == "." {
		base = "presentation"
	}
	return filepath.Join(dir, base+"_preview")
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func jsonResult(status, path string) (string, error) {
	return jsonResultWithFields(status, path, nil)
}

func jsonResultWithFields(status, path string, fields map[string]any) (string, error) {
	payload := map[string]any{"status": status, "path": path}
	for key, value := range fields {
		payload[key] = value
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data), nil
}

func artifactDescriptorForPath(path string, permissionContext tool.PermissionContext, toolName string) tool.ArtifactDescriptor {
	displayPath := filepath.ToSlash(relativeArtifactPath(path, permissionContext.WorkspaceRootOrCWD()))
	return tool.ArtifactDescriptor{
		Action:      tool.ArtifactActionCreated,
		ID:          toolName + ":" + displayPath,
		Title:       filepath.Base(displayPath),
		Description: "Generated " + displayPath,
		Kind:        artifactKindForPath(path),
		Path:        displayPath,
		Status:      "ready",
	}
}

func relativeArtifactPath(path string, root string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return rel
	}
	return filepath.Clean(path)
}

func artifactKindForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ppt", ".pptx":
		return "presentation"
	case ".doc", ".docx":
		return "document"
	case ".xls", ".xlsx", ".csv", ".tsv":
		return "spreadsheet"
	case ".pdf":
		return "pdf"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return "image"
	case ".txt", ".md":
		return "text"
	default:
		return "file"
	}
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

func slidesFromParams(params map[string]any) ([]slideData, error) {
	raw, ok := params["slides"].([]any)
	if !ok {
		return nil, nil
	}
	slides := make([]slideData, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("$.slides[%d] expected object, got %T", i, item)
		}
		layout := firstString(obj, "layout")
		if layout != "" {
			if _, ok := supportedPresentationLayouts[layout]; !ok {
				return nil, fmt.Errorf("$.slides[%d].layout unsupported value %q; expected one of title, section, bullets, two_column, image_right, comparison, metrics, timeline, closing", i, layout)
			}
		}
		slides = append(slides, slideData{
			Title:      firstString(obj, "title"),
			Subtitle:   firstString(obj, "subtitle", "kicker"),
			Body:       firstString(obj, "body", "content"),
			Layout:     layout,
			Notes:      firstString(obj, "notes", "speaker_notes"),
			Background: firstString(obj, "background"),
			Accent:     firstString(obj, "accent"),
			Bullets:    stringSliceParam(obj, "bullets", "points"),
			Images:     presentationImagesFromValue(obj["images"]),
			Columns:    objectSliceParam(obj, "columns"),
			Metrics:    objectSliceParam(obj, "metrics"),
			Timeline:   objectSliceParam(obj, "timeline", "steps"),
		})
	}
	return slides, nil
}

func objectSliceParam(params map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		value, ok := params[key]
		if !ok {
			continue
		}
		raw, ok := value.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			if obj, ok := item.(map[string]any); ok {
				out = append(out, obj)
			}
		}
		return out
	}
	return nil
}

func presentationSpecFromParams(ctx context.Context, params map[string]any, output string) (presentationSpec, error) {
	slides, err := slidesFromParams(params)
	if err != nil {
		return presentationSpec{}, err
	}
	spec := presentationSpec{
		OutputPath:    output,
		Title:         firstString(params, "title"),
		Theme:         presentationThemeFromValue(params["theme"]),
		Slides:        slides,
		RenderPreview: true,
	}
	if value, ok := boolParam(params, "render_preview", "preview"); ok {
		spec.RenderPreview = value
	}
	if len(spec.Slides) == 0 {
		spec.Slides = []slideData{{
			Title:   firstNonEmpty(spec.Title, "New presentation"),
			Body:    strings.Join(stringSliceParam(params, "bullets", "content"), "\n"),
			Bullets: stringSliceParam(params, "bullets"),
			Layout:  "title",
		}}
	}
	perm := tool.PermissionContextFromContext(ctx)
	for slideIndex := range spec.Slides {
		for imageIndex := range spec.Slides[slideIndex].Images {
			image := &spec.Slides[slideIndex].Images[imageIndex]
			if strings.TrimSpace(image.Path) == "" {
				continue
			}
			checked, err := perm.ResolveReadablePathChecked(image.Path)
			if err != nil {
				return presentationSpec{}, err
			}
			image.Path = checked
		}
	}
	if spec.RenderPreview {
		spec.PreviewDir = defaultPresentationPreviewDir(output)
	}
	return spec, nil
}

func writeVerifiedPresentation(ctx context.Context, spec presentationSpec) (presentationRenderReport, error) {
	if err := ensureParentDir(spec.OutputPath); err != nil {
		return presentationRenderReport{}, err
	}
	if err := writePPTXWithPython(ctx, spec); err != nil {
		return presentationRenderReport{}, err
	}
	return verifyPresentationPackage(ctx, spec.OutputPath, spec.PreviewDir, spec.RenderPreview, len(spec.Slides))
}

func verifyPresentationPackage(ctx context.Context, path, previewDir string, keepPreview bool, expectedSlides int) (presentationRenderReport, error) {
	slideCount := expectedSlides
	if counted, err := countPPTXSlides(path); err == nil && counted > 0 {
		slideCount = counted
	}
	report := presentationRenderReport{
		Verified:          false,
		VerificationLevel: "package",
		Engine:            "package",
		SlideCount:        slideCount,
	}
	outputDir := previewDir
	cleanup := false
	if strings.TrimSpace(outputDir) == "" {
		tempDir, err := os.MkdirTemp("", "awaken-presentation-render-*")
		if err != nil {
			return presentationRenderReport{}, err
		}
		outputDir = tempDir
		cleanup = true
	}
	if cleanup {
		defer os.RemoveAll(outputDir)
	}
	rendered, err := renderPresentation(ctx, path, outputDir, slideCount)
	if err != nil {
		// Technical verification ran; surface as unverified package-level result only when
		// the failure is a hard transport error before any level was established.
		return presentationRenderReport{}, fmt.Errorf("presentation render verification failed: %w", err)
	}
	report = rendered
	if !keepPreview {
		report.PDFPath = ""
		report.PreviewDir = ""
		report.PreviewImages = nil
	}
	return report, nil
}

func verifyPresentationRender(ctx context.Context, path string) error {
	outDir, err := os.MkdirTemp("", "awaken-presentation-render-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(outDir)
	slideCount, _ := countPPTXSlides(path)
	report, err := renderPresentation(ctx, path, outDir, slideCount)
	if err != nil {
		return err
	}
	if !report.Verified {
		if len(report.Warnings) > 0 {
			return fmt.Errorf("presentation verification incomplete (%s): %s", report.VerificationLevel, strings.Join(report.Warnings, "; "))
		}
		return fmt.Errorf("presentation verification incomplete (%s)", report.VerificationLevel)
	}
	return nil
}

func renderPresentation(ctx context.Context, path string, outputDir string, expectedSlides int) (presentationRenderReport, error) {
	if strings.TrimSpace(outputDir) == "" {
		return presentationRenderReport{}, fmt.Errorf("output_dir is required")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return presentationRenderReport{}, err
	}
	slideCount := expectedSlides
	if counted, err := countPPTXSlides(path); err == nil && counted > 0 {
		slideCount = counted
	}
	report := presentationRenderReport{
		Verified:          false,
		VerificationLevel: "package",
		Engine:            "package",
		PreviewDir:        outputDir,
		SlideCount:        slideCount,
	}
	pdfPath, err := renderPresentationPDF(ctx, path, outputDir)
	if err != nil {
		report.Warnings = append(report.Warnings, err.Error())
		report.NeedsRuntime = true
		return report, nil
	}
	report.PDFPath = pdfPath
	report.VerificationLevel = "pdf"
	report.Engine = "libreoffice_pdf"
	report.PDFPageCount = slideCount // page count confirmed via PNG parity below

	pngs, err := renderPDFPagesToPNG(ctx, pdfPath, outputDir)
	if err != nil {
		report.Warnings = append(report.Warnings, err.Error())
		report.NeedsRuntime = true
		return report, nil
	}
	report.PreviewImages = pngs
	report.PNGCount = len(pngs)
	report.PDFPageCount = len(pngs)
	report.VerificationLevel = "png"
	report.Engine = "libreoffice_pdf_poppler_png"
	if slideCount > 0 && len(pngs) != slideCount {
		report.Warnings = append(report.Warnings, fmt.Sprintf("slide/PDF/PNG count mismatch: slides=%d png=%d", slideCount, len(pngs)))
		report.NeedsRuntime = false
		return report, nil
	}
	report.Verified = true
	report.VerificationLevel = "png"
	return report, nil
}

func renderPresentationPDF(ctx context.Context, path string, outDir string) (string, error) {
	soffice, err := resolveSoffice()
	if err != nil {
		return "", err
	}
	profileDir, err := os.MkdirTemp("", "awaken-libreoffice-profile-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(profileDir)

	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	args := []string{
		"--headless",
		"--nologo",
		"--nofirststartwizard",
		"--norestore",
		"-env:UserInstallation=" + fileURL(profileDir),
		"--convert-to",
		"pdf",
		"--outdir",
		outDir,
		path,
	}
	cmd := exec.CommandContext(runCtx, soffice, args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return "", fmt.Errorf("libreoffice render timed out: %w", runCtx.Err())
	}
	if err != nil {
		return "", fmt.Errorf("libreoffice render failed: %w: %s", err, excerptOutput(output))
	}
	pdfs, err := filepath.Glob(filepath.Join(outDir, "*.pdf"))
	if err != nil {
		return "", err
	}
	for _, pdf := range pdfs {
		if info, statErr := os.Stat(pdf); statErr == nil && info.Size() > 0 {
			return pdf, nil
		}
	}
	return "", fmt.Errorf("libreoffice render produced no non-empty PDF: %s", excerptOutput(output))
}

func renderPDFPagesToPNG(ctx context.Context, pdfPath string, outputDir string) ([]string, error) {
	pdftoppm, err := resolvePopplerTool("pdftoppm")
	if err != nil {
		return nil, err
	}
	prefix := filepath.Join(outputDir, "page")
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, pdftoppm, "-png", "-r", "144", pdfPath, prefix)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return nil, fmt.Errorf("poppler preview render timed out: %w", runCtx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("poppler preview render failed: %w: %s", err, excerptOutput(output))
	}
	pngs, err := filepath.Glob(filepath.Join(outputDir, "page-*.png"))
	if err != nil {
		return nil, err
	}
	sort.Strings(pngs)
	if len(pngs) == 0 {
		return nil, fmt.Errorf("poppler preview render produced no PNG pages")
	}
	return pngs, nil
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


func runtimeExecutableNames(names ...string) []string {
	seen := make(map[string]struct{}, len(names)*2)
	var out []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		candidates := []string{name}
		if goruntime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
			candidates = []string{name + ".exe", name}
		}
		for _, candidate := range candidates {
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}

func libreOfficeExecutableCandidates(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return []string{
		path,
		filepath.Join(path, "Contents", "MacOS", "soffice"),
		filepath.Join(path, "program", "soffice.bin"),
		filepath.Join(path, "program", "soffice.bin.exe"),
		filepath.Join(path, "program", "soffice"),
		filepath.Join(path, "program", "soffice.exe"),
		filepath.Join(path, "soffice.bin"),
		filepath.Join(path, "soffice.bin.exe"),
		filepath.Join(path, "soffice"),
		filepath.Join(path, "soffice.exe"),
	}
}

func firstExistingFile(candidates []string) (string, bool) {
	files := existingFiles(candidates)
	if len(files) == 0 {
		return "", false
	}
	return files[0], true
}

func existingFiles(candidates []string) []string {
	seen := make(map[string]struct{}, len(candidates))
	var files []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			files = append(files, candidate)
		}
	}
	return files
}

func pythonCanImportPPTX(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-c", "import pptx")
	return cmd.Run() == nil
}

func fileURL(path string) string {
	return "file://" + filepath.ToSlash(path)
}

func excerptOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 1200 {
		return text[:1200] + "...[truncated]"
	}
	return text
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
	tool.MustRegisterTool(artifactInspectTool("artifact_read", "Artifact Read", "读取本机 artifact 的基础文件信息"))
	tool.MustRegisterTool(artifactInspectTool("artifact_inspect", "Artifact Inspect", "检查本机 artifact 类型、大小与基础元数据"))
	tool.MustRegisterTool(artifactExportTool())
	tool.MustRegisterTool(typedArtifactTool{
		name:        "artifact_render",
		displayName: "Artifact Render",
		description: "将本机 artifact 渲染为预览产物；PPTX 自动走 presentation_render，PDF 自动走 Poppler PNG 预览，不要用 shell 调 LibreOffice/Poppler",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "输入 artifact 路径", Type: "string", Required: true},
			{Name: "output_dir", Description: "预览输出目录，必须在 writable/output roots 内；不传则使用同名 preview 目录", Type: "string", Required: false},
		},
		execute: artifactRender,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "spreadsheet_create",
		displayName: "Spreadsheet Create",
		description: "创建 xlsx/csv 表格文件；用于生成表格产物，不要用 apply_patch 或 read_file 手写二进制表格",
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
		description: "写入或重写 xlsx/csv 表格文件；用于生成表格产物，不要用 apply_patch 或 read_file 手写二进制表格",
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
		description: "创建 docx/txt/md 文档；用于生成文档产物，不要用 apply_patch 拼 docx 二进制包",
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
		description: "写入或重写 docx/txt/md 文档；用于生成文档产物，不要用 apply_patch 拼 docx 二进制包",
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
		description: "创建图文混排 pptx 演示文稿，支持 theme、slides、bullets、images、speaker notes 与固定 layout enum，并用 LibreOffice/Poppler 生成预览验证；模型只提交 semantic slide spec，不要用 apply_patch 手写 PPTX ZIP/XML 或 CJK helper",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "output_path", Description: "输出 pptx 路径", Type: "string", Required: true},
			{Name: "title", Description: "演示文稿标题", Type: "string", Required: false},
			{Name: "theme", Description: "主题颜色对象，可包含 background/surface/primary/accent/text/muted", Type: "object", Required: false},
			{Name: "slides", Description: "幻灯片数组，每项可包含 title/subtitle/body/bullets/images/notes/layout", Type: "array", Required: false},
			{Name: "render_preview", Description: "是否保留 PDF/PNG 预览文件，默认 true；false 仍执行技术验证", Type: "boolean", Required: false},
		},
		inputSchema: presentationCreateInputSchema(),
		execute:     presentationCreate,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "presentation_edit",
		displayName: "Presentation Edit",
		description: "对已有 pptx 做增量编辑：必填 source_path 与 edits，保留未修改幻灯片；不支持把全量重写伪装成 edit",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "source_path", Description: "源 pptx 路径", Type: "string", Required: true},
			{Name: "edits", Description: "增量编辑数组，每项含 slide_index 与 op/update 字段", Type: "array", Required: true},
			{Name: "output_path", Description: "输出 pptx 路径，默认覆盖 source_path", Type: "string", Required: false},
			{Name: "theme", Description: "主题颜色对象", Type: "object", Required: false},
			{Name: "render_preview", Description: "是否保留 PDF/PNG 预览文件，默认 true；false 仍执行技术验证", Type: "boolean", Required: false},
		},
		inputSchema: presentationEditInputSchema(),
		execute:     presentationEdit,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "presentation_render",
		displayName: "Presentation Render",
		description: "将本机 pptx 演示文稿渲染成 PDF 与 PNG 页面预览；用于检查 PPT 版式和图片是否可见，不要用 shell 调 LibreOffice/Poppler",
		readOnly:    false,
		capability:  tool.CapabilityArtifactWrite,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "输入 pptx 路径", Type: "string", Required: true},
			{Name: "output_dir", Description: "预览输出目录，必须在 writable/output roots 内；不传则使用同名 preview 目录", Type: "string", Required: false},
		},
		execute: presentationRender,
	})
	tool.MustRegisterTool(typedArtifactTool{
		name:        "pdf_extract",
		displayName: "PDF Extract",
		description: "抽取真实 PDF 文件的文本和页面信息，不执行 OCR；普通文本用 read_file，Office/表格预览用 read_artifact，图片不要传给 pdf_extract",
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
		description: "读取 PNG/JPEG/GIF 等真实图片的尺寸和格式信息，不执行 OCR；PDF 请用 pdf_extract/read_artifact，Office/表格请用 read_artifact",
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
		description: "将 PNG/JPEG/GIF 等真实图片转换为 png/jpg；不用于 PDF/Office 渲染",
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
		description: "读取音视频文件基础元数据；深度 probe 由 ffmpeg runtime 提供。图片用 image_inspect，PDF/Office 用 read_artifact/pdf_extract",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "音视频文件路径", Type: "string", Required: true},
		},
		execute: mediaProbeBasic,
	})

	for _, spec := range []struct {
		name        string
		displayName string
		description string
		readOnly    bool
	}{
		{"spreadsheet_chart", "Spreadsheet Chart", "为表格生成图表产物", false},
		{"document_render", "Document Render", "渲染 docx 文档预览", true},
		{"pdf_render", "PDF Render", "渲染 PDF 页面预览", true},
	} {
		tool.MustRegisterTool(runtimeBackedTool(spec.name, spec.displayName, spec.description, spec.readOnly))
	}
}
