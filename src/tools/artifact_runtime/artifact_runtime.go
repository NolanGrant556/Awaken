package artifactruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"sync"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/runtime/officerender"
	"awaken/src/tool"
)

type typedArtifactTool struct {
	name                string
	displayName         string
	description         string
	readOnly            bool
	capability          tool.CapabilityClass
	requiresRuntime     bool
	runtimeRequirements []string
	parameters          []tool.ToolParameter
	inputSchema         *tool.ToolParametersSchema
	execute             func(context.Context, string) (string, error)
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
	Verified          bool     `json:"verified"`
	VerificationLevel string   `json:"verification_level"`
	Engine            string   `json:"engine"`
	PDFPath           string   `json:"pdf_path,omitempty"`
	PreviewDir        string   `json:"preview_dir,omitempty"`
	PreviewImages     []string `json:"preview_images,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	NeedsRuntime      bool     `json:"needs_runtime,omitempty"`
	SlideCount        int      `json:"slide_count,omitempty"`
	PDFPageCount      int      `json:"pdf_page_count,omitempty"`
	PNGCount          int      `json:"png_count,omitempty"`
}

const errorCodePresentationVerificationFailed = "presentation_verification_failed"

var (
	presentationAuthoringRuntimeRequirements = []string{"python", "libreoffice", "poppler"}
	presentationRenderRuntimeRequirements    = []string{"libreoffice", "poppler"}
	presentationTestOfficeWorkers            = struct {
		sync.Mutex
		bySoffice map[string]*officerender.OfficeRenderWorker
	}{bySoffice: make(map[string]*officerender.OfficeRenderWorker)}
)

type artifactExecutionFailure struct {
	Content    string
	Code       string
	Category   tool.ErrorCategory
	Status     tool.OutcomeStatus
	Retryable  bool
	Diagnostic map[string]any
	Cause      error
}

func (e *artifactExecutionFailure) Error() string {
	if e == nil {
		return "artifact execution failed"
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if strings.TrimSpace(e.Content) != "" {
		return e.Content
	}
	return firstNonEmpty(e.Code, "artifact execution failed")
}

type presentationInputError struct {
	Path   string
	Reason string
}

func (e *presentationInputError) Error() string {
	return fmt.Sprintf("%s %s", e.Path, e.Reason)
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
	var schema tool.ToolParametersSchema
	if t.inputSchema != nil {
		schema = *t.inputSchema
	} else {
		schema = tool.SchemaFromParameters(t.Parameters())
	}
	return completeTypedArtifactInputSchema(schema)
}

func (t typedArtifactTool) IsReadOnly() bool { return t.readOnly }

func (t typedArtifactTool) Definition() tool.ToolDefinition {
	return typedArtifactDefinition(t)
}

func (t typedArtifactTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	if t.readOnly {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return nil
	}
	artifactStatus := firstString(payload, "artifact_status")
	if artifactStatus == "" {
		artifactStatus = "ready"
	}
	var artifacts []tool.ArtifactDescriptor
	primaryPath := firstString(payload, "path")
	if primaryPath != "" && !strings.HasSuffix(t.name, "_render") {
		artifact := artifactDescriptorForPath(primaryPath, permissionContext, t.name, typedArtifactAction(t.name))
		artifact.Status = artifactStatus
		artifacts = append(artifacts, artifact)
	}
	if pdfPath := firstString(payload, "pdf_path"); pdfPath != "" && !sameArtifactPath(pdfPath, primaryPath) {
		artifact := artifactDescriptorForPath(pdfPath, permissionContext, t.name, tool.ArtifactActionCreated)
		artifact.Status = artifactStatus
		artifacts = append(artifacts, artifact)
	}
	for _, imagePath := range stringSliceParam(payload, "preview_images") {
		artifact := artifactDescriptorForPath(imagePath, permissionContext, t.name, tool.ArtifactActionCreated)
		artifact.Status = artifactStatus
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

func sameArtifactPath(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func (t typedArtifactTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
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
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return outcome.Content, fmt.Errorf("%s", firstNonEmpty(outcome.ErrorMessage, outcome.ErrorCode, "artifact execution failed"))
	}
	return outcome.Content, nil
}

func (t typedArtifactTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	normalizedInput, err := normalizeTypedArtifactInput(input, t.parameters)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if err := tool.ValidateInputAgainstSchema(normalizedInput, t.InputSchema()); err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if t.name == "presentation_edit" {
		if err := validatePresentationEditOperationPayloads(normalizedInput); err != nil {
			outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
	}
	if t.requiresRuntime {
		report, missing := checkRuntimeRequirements(t.runtimeRequirements)
		if len(missing) > 0 {
			outcome := needsRuntimeExecutionOutcome(t.name, report, t.runtimeRequirements, missing)
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
	}
	if t.execute == nil {
		outcome := tool.FailedOutcome(
			fmt.Sprintf("%s requires a prepared artifact runtime executor", t.name),
			tool.ErrorCodeToolExecutionFailed,
			tool.ErrorCategoryTool,
			false,
		)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	content, err := t.execute(ctx, normalizedInput)
	finished := time.Now().UTC()
	if err != nil {
		var executionFailure *artifactExecutionFailure
		if errors.As(err, &executionFailure) {
			outcome := tool.FailedOutcome(
				firstNonEmpty(executionFailure.Content, err.Error()),
				firstNonEmpty(executionFailure.Code, tool.ErrorCodeToolExecutionFailed),
				executionFailure.Category,
				executionFailure.Retryable,
			)
			if executionFailure.Status == tool.OutcomeNeedsRuntime {
				outcome.Status = tool.OutcomeNeedsRuntime
				outcome.TransportStatus = tool.TransportCompleted
				outcome.SemanticStatus = tool.SemanticUnknown
				outcome.RecoveryOptions = []string{"install_missing_dependency", "retry_tool"}
			}
			outcome.Diagnostic = executionFailure.Diagnostic
			if executionFailure.Cause != nil {
				outcome.ErrorMessage = executionFailure.Cause.Error()
			}
			outcome.Artifacts = t.Artifacts(normalizedInput, outcome.Content, tool.PermissionContextFromContext(ctx))
			return outcome.WithTiming(started, finished), nil
		}
		var inputFailure *presentationInputError
		if errors.As(err, &inputFailure) {
			outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
			outcome.Diagnostic = map[string]any{"path": inputFailure.Path, "reason": inputFailure.Reason}
			return outcome.WithTiming(started, finished), nil
		}
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryTool, false)
		return outcome.WithTiming(started, finished), nil
	}

	outcome := tool.SucceededOutcome(content)
	outcome.Artifacts = t.Artifacts(normalizedInput, content, tool.PermissionContextFromContext(ctx))
	outcome.Details = typedArtifactOutcomeDetails(content, outcome.Artifacts)
	return outcome.WithTiming(started, finished), nil
}

func typedArtifactOutcomeDetails(content string, artifacts []tool.ArtifactDescriptor) map[string]any {
	details := make(map[string]any)
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err == nil && result != nil {
		details["result"] = result
		if path, ok := result["path"].(string); ok && strings.TrimSpace(path) != "" {
			details["path"] = path
		}
		if status, ok := result["status"].(string); ok && strings.TrimSpace(status) != "" {
			details["artifact_status"] = status
		}
		if path, ok := result["pdf_path"].(string); ok && strings.TrimSpace(path) != "" {
			details["pdf_path"] = path
		}
		if previews, ok := result["preview_images"].([]any); ok {
			details["preview_images"] = previews
		}
		if missing, ok := result["missing_components"].([]any); ok {
			details["runtime_missing"] = missing
		}
	} else if text := strings.TrimSpace(content); text != "" {
		details["result_text"] = text
	}
	if len(artifacts) > 0 {
		if _, exists := details["path"]; !exists && strings.TrimSpace(artifacts[0].Path) != "" {
			details["path"] = artifacts[0].Path
		}
		if _, exists := details["artifact_status"]; !exists && strings.TrimSpace(artifacts[0].Status) != "" {
			details["artifact_status"] = artifacts[0].Status
		}
	}
	if len(details) == 0 {
		return nil
	}
	return details
}

func normalizeTypedArtifactInput(input string, parameters []tool.ToolParameter) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "{}", nil
	}
	if looksLikeJSONContainer(trimmed) {
		return trimmed, nil
	}
	name := singleRawParamName(parameters)
	if name == "" {
		return trimmed, nil
	}
	data, err := json.Marshal(map[string]any{name: trimmed})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func checkRuntimeRequirements(requirements []string) (preflightReport, []string) {
	report := runtimePreflightCached(false)
	return report, missingRuntimeRequirements(report, requirements)
}

func missingRuntimeRequirements(report preflightReport, requirements []string) []string {
	if len(requirements) == 0 {
		if report.Ready {
			return nil
		}
		return append([]string(nil), report.Missing...)
	}
	components := make(map[string]runtimeComponentStatus, len(report.Components))
	for _, component := range report.Components {
		components[component.Name] = component
	}
	missing := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		component, ok := components[requirement]
		if !ok || !component.OK {
			missing = append(missing, requirement)
		}
	}
	return missing
}

func needsRuntimeExecutionOutcome(toolName string, report preflightReport, requirements, missing []string) tool.ExecutionOutcome {
	payload := map[string]any{
		"status":              "dependency_missing",
		"runtime":             firstNonEmpty(report.RuntimeName, runtimeName),
		"tool":                toolName,
		"required_components": append([]string(nil), requirements...),
		"missing_components":  append([]string(nil), missing...),
		"detail":              report.Detail,
	}
	content, _ := json.MarshalIndent(payload, "", "  ")
	now := time.Now().UTC()
	return tool.ExecutionOutcome{
		Content:         string(content),
		Status:          tool.OutcomeNeedsRuntime,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticUnknown,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       tool.ErrorCodeNeedsRuntime,
		ErrorCategory:   tool.ErrorCategoryRuntime,
		ErrorMessage:    fmt.Sprintf("%s is missing host dependencies: %s", toolName, strings.Join(missing, ", ")),
		Retryable:       true,
		RecoveryOptions: []string{"install_missing_dependency", "retry_tool"},
		Diagnostic: map[string]any{
			"runtime":             report.RuntimeName,
			"required_components": append([]string(nil), requirements...),
			"missing_components":  append([]string(nil), missing...),
			"detail":              report.Detail,
		},
		StartedAt:  now,
		FinishedAt: now,
	}
}

func workspaceDependenciesTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "workspace_dependencies",
		displayName: "Workspace Dependencies",
		description: "返回当前用户环境中可复用的 Work 依赖，例如 Python、LibreOffice/soffice、Poppler 和 FFmpeg 的实际路径；缺失时可由 Agent 使用 Shell 按正常包管理方式安装",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters:  nil,
		execute: func(context.Context, string) (string, error) {
			report := runtimePreflightCached(false)
			data, _ := json.MarshalIndent(report, "", "  ")
			return string(data), nil
		},
	}
}

func runtimePreflightTool() typedArtifactTool {
	return typedArtifactTool{
		name:        "runtime_preflight",
		displayName: "Runtime Preflight",
		description: "检查 Office/PDF/媒体 typed artifact tools 所需的当前用户环境依赖；缺失时返回具体命令或包缺口，安装后可直接重试",
		readOnly:    true,
		capability:  tool.CapabilityArtifactRead,
		parameters:  nil,
		execute: func(context.Context, string) (string, error) {
			report := runtimePreflightCached(false)
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
		description:     description + "；复用当前用户环境，缺少依赖时先按错误提示用 Shell 安装，再重试",
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
		report, err := renderPresentationPreviewAtomically(ctx, checkedSource, outputDir, slideCount)
		if err != nil {
			return "", err
		}
		if !report.Verified {
			return presentationFailureResult("artifact_render_failed", checkedSource, report, slideCount)
		}
		result, err := presentationResultJSON("artifact_rendered", checkedSource, report, slideCount)
		if err != nil {
			return "", err
		}
		var payload map[string]any
		if json.Unmarshal([]byte(result), &payload) == nil {
			payload["kind"] = "presentation"
			data, _ := json.MarshalIndent(payload, "", "  ")
			return string(data), nil
		}
		return result, nil
	case ".pdf":
		images, err := renderPDFPreviewAtomically(ctx, checkedSource, outputDir)
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
		return "", fmt.Errorf("artifact_render 暂不支持渲染该文件类型: %s。当前只支持 PPT/PPTX 与 PDF；Office 文档深度渲染尚无可执行工具", filepath.Ext(checkedSource))
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
	extension := strings.ToLower(filepath.Ext(output))
	if extension != ".docx" && extension != ".txt" && extension != ".md" {
		return "", fmt.Errorf("document_create/document_edit 只支持 .docx、.txt 或 .md 输出，不能把 %s 内容写入 %s 文件；如需 PDF，请先创建 DOCX，再使用 Office PDF 转换工具", extension, extension)
	}
	paragraphs := stringSliceParam(params, "paragraphs", "content")
	title := firstString(params, "title")
	if title != "" {
		paragraphs = append([]string{title}, paragraphs...)
	}
	if len(paragraphs) == 0 {
		paragraphs = []string{"New document"}
	}
	if extension == ".txt" || extension == ".md" {
		if err := os.WriteFile(output, []byte(strings.Join(paragraphs, "\n\n")), 0o644); err != nil {
			return "", err
		}
	} else if err := writeMinimalDOCX(output, paragraphs); err != nil {
		return "", err
	}
	return jsonResult("document_written", output)
}

func officeExportPDF(ctx context.Context, input string) (string, error) {
	params, err := decodeParams(input)
	if err != nil {
		return "", err
	}
	source := firstString(params, "path", "input_path", "source_path")
	if source == "" {
		return "", fmt.Errorf("path is required")
	}
	perm := tool.PermissionContextFromContext(ctx)
	checkedSource, err := perm.ResolveReadablePathChecked(source)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(checkedSource)) {
	case ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
	default:
		return "", fmt.Errorf("office_export_pdf 只支持 Word、Excel 或 PowerPoint 文件，收到 %s", filepath.Ext(checkedSource))
	}
	output := firstString(params, "output_path")
	if output == "" {
		output = strings.TrimSuffix(filepath.Base(checkedSource), filepath.Ext(checkedSource)) + ".pdf"
	}
	if !strings.EqualFold(filepath.Ext(output), ".pdf") {
		return "", fmt.Errorf("office_export_pdf 的 output_path 必须以 .pdf 结尾")
	}
	checkedOutput, err := resolveArtifactOutputPathChecked(perm, output)
	if err != nil {
		return "", err
	}
	if err := ensureParentDir(checkedOutput); err != nil {
		return "", err
	}
	result, err := officerender.Render(ctx, checkedSource, checkedOutput, "pdf")
	if err != nil {
		return "", fmt.Errorf("Office PDF 转换失败: %w", err)
	}
	return jsonResultWithFields("pdf_exported", result.OutputPath, map[string]any{
		"source_path": checkedSource,
		"pdf_path":    result.OutputPath,
		"kind":        "pdf",
		"verified":    true,
	})
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
	if !report.Verified {
		return presentationFailureResult("presentation_failed", output, report, len(spec.Slides))
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
	if err := resolvePresentationEditImages(ctx, edits); err != nil {
		return "", err
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
	report, err := writeVerifiedPresentationEdit(ctx, spec)
	if err != nil {
		return "", err
	}
	slideCount := report.SlideCount
	if !report.Verified {
		return presentationFailureResult("presentation_failed", checkedOutput, report, slideCount)
	}
	return presentationResultJSON("presentation_edited", checkedOutput, report, slideCount)
}

func validatePresentationEditOperationPayloads(input string) error {
	params, err := decodeParams(input)
	if err != nil {
		return err
	}
	edits, _ := params["edits"].([]any)
	for index, raw := range edits {
		edit, _ := raw.(map[string]any)
		op := strings.ToLower(strings.TrimSpace(firstString(edit, "op")))
		if op == "" {
			op = "update"
		}
		switch op {
		case "update":
			hasPayload := false
			for key, value := range edit {
				if key == "op" || key == "slide_index" {
					continue
				}
				if key == "slide" {
					slide, _ := value.(map[string]any)
					if len(slide) == 0 {
						continue
					}
				}
				hasPayload = true
				break
			}
			if !hasPayload {
				return fmt.Errorf("$.edits[%d] update requires at least one editable field", index)
			}
		case "insert", "replace":
			slide, _ := edit["slide"].(map[string]any)
			if len(slide) == 0 {
				return fmt.Errorf("$.edits[%d].slide must be a non-empty slide object for %s", index, op)
			}
		}
	}
	return nil
}

func presentationResultJSON(status, path string, report presentationRenderReport, slideCount int) (string, error) {
	artifactStatus := "error"
	if report.Verified {
		artifactStatus = "ready"
	}
	return jsonResultWithFields(status, path, presentationResultFields(report, slideCount, artifactStatus))
}

func presentationResultFields(report presentationRenderReport, slideCount int, artifactStatus string) map[string]any {
	fields := map[string]any{
		"artifact_status":    artifactStatus,
		"verified":           report.Verified,
		"verification_level": report.VerificationLevel,
		"render_check":       report.Engine,
		"pdf_path":           report.PDFPath,
		"preview_dir":        report.PreviewDir,
		"preview_images":     report.PreviewImages,
		"warnings":           report.Warnings,
		"slide_count":        slideCount,
		"pdf_page_count":     report.PDFPageCount,
		"png_count":          report.PNGCount,
	}
	if report.NeedsRuntime {
		fields["needs_runtime"] = true
	}
	return fields
}

func presentationFailureResult(status, path string, report presentationRenderReport, slideCount int) (string, error) {
	fields := presentationResultFields(report, slideCount, "error")
	// A failed atomic publication deliberately leaves no artifact at the final
	// path. Keep the requested destination as diagnostic context without using
	// the canonical `path` field, otherwise typedArtifactTool.Artifacts would
	// publish a card that points at a non-existent (or unchanged old) file.
	fields["attempted_path"] = path
	payload := map[string]any{"status": status}
	for key, value := range fields {
		payload[key] = value
	}
	data, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		return "", marshalErr
	}
	content := string(data)
	code := errorCodePresentationVerificationFailed
	category := tool.ErrorCategoryDomain
	outcomeStatus := tool.OutcomeFailed
	if report.NeedsRuntime {
		code = tool.ErrorCodeNeedsRuntime
		category = tool.ErrorCategoryRuntime
		outcomeStatus = tool.OutcomeNeedsRuntime
	}
	reason := fmt.Sprintf("presentation verification stopped at %s", firstNonEmpty(report.VerificationLevel, "package"))
	if len(report.Warnings) > 0 {
		reason += ": " + strings.Join(report.Warnings, "; ")
	}
	return content, &artifactExecutionFailure{
		Content:   content,
		Code:      code,
		Category:  category,
		Status:    outcomeStatus,
		Retryable: false,
		Diagnostic: map[string]any{
			"verified":           false,
			"attempted_path":     path,
			"verification_level": report.VerificationLevel,
			"render_check":       report.Engine,
			"warnings":           append([]string(nil), report.Warnings...),
			"artifact_status":    "error",
		},
		Cause: errors.New(reason),
	}
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
	report, err := renderPresentationPreviewAtomically(ctx, checkedSource, outputDir, slideCount)
	if err != nil {
		return "", err
	}
	if !report.Verified {
		return presentationFailureResult("presentation_render_failed", checkedSource, report, slideCount)
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
		return "", fmt.Errorf("无法解码图片: %w。image_convert 仅转换 PNG/JPEG/GIF 等可解码图片；PDF 页面预览请用 artifact_render，文本抽取请用 pdf_extract/read_artifact，Office 文件请用 read_artifact", err)
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
	if err := ensureParentDir(checked); err != nil {
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

func artifactDescriptorForPath(path string, permissionContext tool.PermissionContext, toolName string, action tool.ArtifactAction) tool.ArtifactDescriptor {
	displayPath := filepath.ToSlash(relativeArtifactPath(path, permissionContext.WorkspaceRootOrCWD()))
	kind := artifactKindForPath(path)
	return tool.ArtifactDescriptor{
		ContractVersion: tool.ArtifactContractVersion,
		Role:            tool.ResolveArtifactRole("", toolName, kind, displayPath),
		Producer:        toolName,
		Action:          action,
		ID:              toolName + ":" + displayPath,
		Title:           filepath.Base(displayPath),
		Description:     typedArtifactDescription(action, displayPath),
		Kind:            kind,
		Path:            displayPath,
		Status:          "ready",
	}
}

func typedArtifactDescription(action tool.ArtifactAction, displayPath string) string {
	if action == tool.ArtifactActionUpdated {
		return "Updated " + displayPath
	}
	return "Generated " + displayPath
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
				return presentationSpec{}, &presentationInputError{
					Path:   fmt.Sprintf("$.slides[%d].images[%d].path", slideIndex, imageIndex),
					Reason: "is required",
				}
			}
			checked, err := perm.ResolveReadablePathChecked(image.Path)
			if err != nil {
				return presentationSpec{}, &presentationInputError{
					Path:   fmt.Sprintf("$.slides[%d].images[%d].path", slideIndex, imageIndex),
					Reason: err.Error(),
				}
			}
			if err := validatePresentationImageFile(checked); err != nil {
				return presentationSpec{}, &presentationInputError{
					Path:   fmt.Sprintf("$.slides[%d].images[%d].path", slideIndex, imageIndex),
					Reason: err.Error(),
				}
			}
			image.Path = checked
		}
	}
	if spec.RenderPreview {
		spec.PreviewDir = defaultPresentationPreviewDir(output)
	}
	return spec, nil
}

func resolvePresentationEditImages(ctx context.Context, edits []map[string]any) error {
	for editIndex, edit := range edits {
		if err := resolvePresentationImageList(
			ctx,
			edit["images"],
			fmt.Sprintf("$.edits[%d].images", editIndex),
		); err != nil {
			return err
		}
		slide, _ := edit["slide"].(map[string]any)
		if slide == nil {
			continue
		}
		if err := resolvePresentationImageList(
			ctx,
			slide["images"],
			fmt.Sprintf("$.edits[%d].slide.images", editIndex),
		); err != nil {
			return err
		}
	}
	return nil
}

func resolvePresentationImageList(ctx context.Context, value any, basePath string) error {
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return &presentationInputError{Path: basePath, Reason: "expected array"}
	}
	perm := tool.PermissionContextFromContext(ctx)
	for imageIndex, item := range items {
		image, ok := item.(map[string]any)
		if !ok {
			return &presentationInputError{Path: fmt.Sprintf("%s[%d]", basePath, imageIndex), Reason: "expected object"}
		}
		path, _ := image["path"].(string)
		path = strings.TrimSpace(path)
		if path == "" {
			return &presentationInputError{Path: fmt.Sprintf("%s[%d].path", basePath, imageIndex), Reason: "is required"}
		}
		checked, err := perm.ResolveReadablePathChecked(path)
		if err != nil {
			return &presentationInputError{Path: fmt.Sprintf("%s[%d].path", basePath, imageIndex), Reason: err.Error()}
		}
		if err := validatePresentationImageFile(checked); err != nil {
			return &presentationInputError{Path: fmt.Sprintf("%s[%d].path", basePath, imageIndex), Reason: err.Error()}
		}
		image["path"] = checked
	}
	return nil
}

func validatePresentationImageFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("image file does not exist: %s", path)
		}
		return fmt.Errorf("image file is not readable: %v", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("image path is not a regular file: %s", path)
	}
	if info.Size() <= 0 {
		return fmt.Errorf("image file is empty: %s", path)
	}
	return nil
}

func writeVerifiedPresentation(ctx context.Context, spec presentationSpec) (presentationRenderReport, error) {
	if err := ensureParentDir(spec.OutputPath); err != nil {
		return presentationRenderReport{}, err
	}
	staging, err := newPresentationStaging(spec.OutputPath, spec.PreviewDir, spec.RenderPreview)
	if err != nil {
		return presentationRenderReport{}, err
	}
	defer staging.cleanup()

	stagedSpec := spec
	stagedSpec.OutputPath = staging.stagedPath
	stagedSpec.PreviewDir = staging.stagedPreviewDir
	engineReport, err := writePPTXWithPython(ctx, stagedSpec)
	if err != nil {
		return presentationRenderReport{}, err
	}
	return staging.verifyAndPublish(ctx, engineReport.Warnings, len(spec.Slides))
}

func writeVerifiedPresentationEdit(ctx context.Context, spec presentationEditSpec) (presentationRenderReport, error) {
	if err := ensureParentDir(spec.OutputPath); err != nil {
		return presentationRenderReport{}, err
	}
	staging, err := newPresentationStaging(spec.OutputPath, spec.PreviewDir, spec.RenderPreview)
	if err != nil {
		return presentationRenderReport{}, err
	}
	defer staging.cleanup()

	// SourcePath always remains the caller's original deck. In-place edits are
	// rendered into the sibling stagedPath and cannot mutate the source before
	// the full render/visual gate succeeds.
	stagedSpec := spec
	stagedSpec.OutputPath = staging.stagedPath
	stagedSpec.PreviewDir = staging.stagedPreviewDir
	engineReport, err := editPPTXWithPython(ctx, stagedSpec)
	if err != nil {
		return presentationRenderReport{}, err
	}
	slideCount, err := countPPTXSlides(staging.stagedPath)
	if err != nil {
		return presentationRenderReport{}, fmt.Errorf("count staged presentation slides: %w", err)
	}
	return staging.verifyAndPublish(ctx, engineReport.Warnings, slideCount)
}

type presentationStaging struct {
	finalPath        string
	stagedPath       string
	finalPreviewDir  string
	stagedPreviewDir string
	keepPreview      bool
}

func newPresentationStaging(finalPath, finalPreviewDir string, keepPreview bool) (*presentationStaging, error) {
	finalPath = filepath.Clean(finalPath)
	parent := filepath.Dir(finalPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create presentation output directory: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(finalPath), filepath.Ext(finalPath))
	if strings.TrimSpace(base) == "" || base == "." {
		base = "presentation"
	}
	stagedFile, err := os.CreateTemp(parent, "."+base+".awaken-stage-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("create presentation staging file: %w", err)
	}
	stagedPath := stagedFile.Name()
	if err := stagedFile.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return nil, fmt.Errorf("close presentation staging file: %w", err)
	}
	stagedPreviewDir, err := os.MkdirTemp(parent, "."+base+"_preview.awaken-stage-*")
	if err != nil {
		_ = os.Remove(stagedPath)
		return nil, fmt.Errorf("create presentation preview staging directory: %w", err)
	}
	if keepPreview && strings.TrimSpace(finalPreviewDir) == "" {
		finalPreviewDir = defaultPresentationPreviewDir(finalPath)
	}
	return &presentationStaging{
		finalPath:        finalPath,
		stagedPath:       stagedPath,
		finalPreviewDir:  filepath.Clean(finalPreviewDir),
		stagedPreviewDir: stagedPreviewDir,
		keepPreview:      keepPreview,
	}, nil
}

func (s *presentationStaging) cleanup() {
	if s == nil {
		return
	}
	_ = os.Remove(s.stagedPath)
	_ = os.RemoveAll(s.stagedPreviewDir)
}

func (s *presentationStaging) verifyAndPublish(ctx context.Context, engineWarnings []string, expectedSlides int) (presentationRenderReport, error) {
	report, err := verifyPresentationPackage(ctx, s.stagedPath, s.stagedPreviewDir, true, expectedSlides)
	if err != nil {
		return presentationRenderReport{}, err
	}
	report = gatePresentationVerificationWarnings(report, engineWarnings...)
	if !report.Verified {
		return clearPresentationPreviewReferences(report), nil
	}

	var previewSwap *presentationPreviewSwap
	if s.keepPreview {
		if err := normalizeStagedPresentationPreview(&report, s.stagedPath, s.finalPath, s.stagedPreviewDir); err != nil {
			return presentationRenderReport{}, err
		}
		policy := presentationManagedPreviewPolicy(s.finalPath, s.finalPreviewDir)
		if err := prepareManagedPreviewDirectory(s.stagedPreviewDir, policy); err != nil {
			return presentationRenderReport{}, err
		}
		previewSwap, err = beginManagedPreviewSwap(s.stagedPreviewDir, s.finalPreviewDir, policy)
		if err != nil {
			return presentationRenderReport{}, err
		}
	} else {
		report = clearPresentationPreviewReferences(report)
	}

	if err := publishPresentationFile(s.stagedPath, s.finalPath); err != nil {
		if previewSwap != nil {
			if rollbackErr := previewSwap.rollback(); rollbackErr != nil {
				return presentationRenderReport{}, errors.Join(err, fmt.Errorf("rollback presentation preview publication: %w", rollbackErr))
			}
		}
		return presentationRenderReport{}, err
	}
	if previewSwap != nil {
		if err := previewSwap.commit(); err != nil {
			return presentationRenderReport{}, err
		}
		report = rebasePresentationPreviewReport(report, s.stagedPreviewDir, s.finalPreviewDir)
	}
	return report, nil
}

func clearPresentationPreviewReferences(report presentationRenderReport) presentationRenderReport {
	report.PDFPath = ""
	report.PreviewDir = ""
	report.PreviewImages = nil
	return report
}

func normalizeStagedPresentationPreview(report *presentationRenderReport, stagedPath, finalPath, stagedPreviewDir string) error {
	if report == nil || strings.TrimSpace(report.PDFPath) == "" {
		return nil
	}
	expectedPDF := presentationPDFPath(finalPath, stagedPreviewDir)
	if filepath.Clean(report.PDFPath) != filepath.Clean(expectedPDF) {
		if err := os.Rename(report.PDFPath, expectedPDF); err != nil {
			return fmt.Errorf("normalize staged presentation PDF name: %w", err)
		}
		oldMetadata := report.PDFPath + ".pagecount"
		if _, err := os.Stat(oldMetadata); err == nil {
			if err := os.Rename(oldMetadata, expectedPDF+".pagecount"); err != nil {
				return fmt.Errorf("normalize staged presentation PDF metadata: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect staged presentation PDF metadata: %w", err)
		}
		report.PDFPath = expectedPDF
	}
	report.PreviewDir = stagedPreviewDir
	_ = stagedPath // kept in the signature to make the source/final basename boundary explicit.
	return nil
}

func syncPresentationPreview(report presentationRenderReport) error {
	paths := append([]string{report.PDFPath, report.PDFPath + ".pagecount"}, report.PreviewImages...)
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := syncRegularFile(path); err != nil {
			return fmt.Errorf("sync staged presentation preview %s: %w", path, err)
		}
	}
	return syncDirectory(report.PreviewDir)
}

func preparePresentationPreviewPermissions(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if err := os.Chmod(path, 0o755); err != nil {
				return fmt.Errorf("set presentation preview directory permissions: %w", err)
			}
			return nil
		}
		if info.Mode().IsRegular() {
			if err := os.Chmod(path, 0o644); err != nil {
				return fmt.Errorf("set presentation preview file permissions: %w", err)
			}
		}
		return nil
	})
}

type presentationPreviewSwap struct {
	finalDir  string
	backupDir string
}

func beginPresentationPreviewSwap(stagedDir, finalDir, finalPath string) (*presentationPreviewSwap, error) {
	return beginManagedPreviewSwap(stagedDir, finalDir, presentationManagedPreviewPolicy(finalPath, finalDir))
}

type managedPreviewPolicy struct {
	pdfName string
	marker  previewOwnershipMarker
}

func presentationManagedPreviewPolicy(presentationPath, previewDir string) managedPreviewPolicy {
	return managedPreviewPolicy{
		pdfName: filepath.Base(presentationPDFPath(presentationPath, previewDir)),
		marker:  newPreviewOwnershipMarker("presentation_preview", presentationPath, previewDir),
	}
}

func pdfManagedPreviewPolicy(pdfPath, previewDir string) managedPreviewPolicy {
	return managedPreviewPolicy{
		marker: newPreviewOwnershipMarker("pdf_preview", pdfPath, previewDir),
	}
}

func (policy managedPreviewPolicy) allows(name string) bool {
	if isManagedPresentationPreviewPage(name) {
		return true
	}
	return policy.pdfName != "" && (name == policy.pdfName || name == policy.pdfName+".pagecount")
}

const (
	previewOwnershipMarkerName = ".awaken-preview.json"
	previewOwnershipSchema     = 1
	previewOwnershipOwner      = "awaken.artifact_runtime"
	previewOwnershipMaxBytes   = 16 * 1024
)

type previewOwnershipMarker struct {
	SchemaVersion  int    `json:"schema_version"`
	Owner          string `json:"owner"`
	Tool           string `json:"tool"`
	SourceIdentity string `json:"source_identity"`
	OutputIdentity string `json:"output_identity"`
}

func newPreviewOwnershipMarker(toolName, sourcePath, outputDir string) previewOwnershipMarker {
	return previewOwnershipMarker{
		SchemaVersion:  previewOwnershipSchema,
		Owner:          previewOwnershipOwner,
		Tool:           toolName,
		SourceIdentity: managedPreviewIdentity(sourcePath),
		OutputIdentity: managedPreviewIdentity(outputDir),
	}
}

func managedPreviewIdentity(path string) string {
	path = filepath.Clean(path)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

func (marker previewOwnershipMarker) validate() error {
	if marker.SchemaVersion != previewOwnershipSchema || marker.Owner != previewOwnershipOwner {
		return fmt.Errorf("unsupported preview ownership marker schema or owner")
	}
	if strings.TrimSpace(marker.Tool) == "" || strings.TrimSpace(marker.SourceIdentity) == "" || strings.TrimSpace(marker.OutputIdentity) == "" {
		return fmt.Errorf("preview ownership marker is incomplete")
	}
	return nil
}

func beginManagedPreviewSwap(stagedDir, finalDir string, policy managedPreviewPolicy) (*presentationPreviewSwap, error) {
	if strings.TrimSpace(finalDir) == "" {
		return nil, fmt.Errorf("final preview directory is required")
	}
	parent := filepath.Dir(finalDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	swap := &presentationPreviewSwap{finalDir: finalDir}
	if _, err := os.Lstat(finalDir); err == nil {
		if err := validateManagedPreviewDir(finalDir, policy); err != nil {
			return nil, err
		}
		placeholder, tempErr := os.MkdirTemp(parent, "."+filepath.Base(finalDir)+".awaken-backup-*")
		if tempErr != nil {
			return nil, tempErr
		}
		if removeErr := os.Remove(placeholder); removeErr != nil {
			return nil, removeErr
		}
		swap.backupDir = placeholder
		if err := os.Rename(finalDir, swap.backupDir); err != nil {
			return nil, fmt.Errorf("backup existing presentation preview: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(stagedDir, finalDir); err != nil {
		publishErr := fmt.Errorf("publish presentation preview: %w", err)
		if swap.backupDir != "" {
			restoreErr := swap.restoreBackup()
			syncErr := syncDirectory(parent)
			return nil, errors.Join(publishErr, restoreErr, syncErr)
		}
		return nil, publishErr
	}
	if err := syncDirectory(parent); err != nil {
		syncErr := fmt.Errorf("sync presentation preview directory: %w", err)
		if rollbackErr := swap.rollback(); rollbackErr != nil {
			return nil, errors.Join(syncErr, fmt.Errorf("rollback preview after sync failure: %w", rollbackErr))
		}
		return nil, syncErr
	}
	return swap, nil
}

func validateManagedPresentationPreviewDir(previewDir, presentationPath string) error {
	return validateManagedPreviewDir(previewDir, presentationManagedPreviewPolicy(presentationPath, previewDir))
}

func validateManagedPreviewDir(previewDir string, policy managedPreviewPolicy) error {
	if err := policy.marker.validate(); err != nil {
		return fmt.Errorf("invalid expected preview ownership: %w", err)
	}
	info, err := os.Lstat(previewDir)
	if err != nil {
		return fmt.Errorf("inspect preview directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("refusing to replace unmanaged preview path %s", previewDir)
	}

	entries, err := os.ReadDir(previewDir)
	if err != nil {
		return fmt.Errorf("inspect preview entries: %w", err)
	}
	markerFound := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to replace preview directory containing symlink %s", filepath.Join(previewDir, name))
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect preview entry %s: %w", filepath.Join(previewDir, name), err)
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace preview directory containing non-regular entry %s", filepath.Join(previewDir, name))
		}
		if name == previewOwnershipMarkerName {
			if entryInfo.Size() > previewOwnershipMaxBytes {
				return fmt.Errorf("refusing to replace preview directory with oversized ownership marker %s", filepath.Join(previewDir, name))
			}
			markerFound = true
			continue
		}
		if policy.allows(name) {
			continue
		}
		return fmt.Errorf("refusing to replace preview directory containing unrelated file %s", filepath.Join(previewDir, name))
	}
	if !markerFound {
		return fmt.Errorf("refusing to replace preview directory without ownership marker %s", filepath.Join(previewDir, previewOwnershipMarkerName))
	}
	actualMarker, err := readPreviewOwnershipMarker(filepath.Join(previewDir, previewOwnershipMarkerName))
	if err != nil {
		return fmt.Errorf("refusing to replace preview directory with invalid ownership marker: %w", err)
	}
	if actualMarker != policy.marker {
		return fmt.Errorf("refusing to replace preview directory with mismatched ownership marker %s", filepath.Join(previewDir, previewOwnershipMarkerName))
	}
	return nil
}

func readPreviewOwnershipMarker(path string) (previewOwnershipMarker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return previewOwnershipMarker{}, err
	}
	if len(data) > previewOwnershipMaxBytes {
		return previewOwnershipMarker{}, fmt.Errorf("ownership marker exceeds %d bytes", previewOwnershipMaxBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var marker previewOwnershipMarker
	if err := decoder.Decode(&marker); err != nil {
		return previewOwnershipMarker{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return previewOwnershipMarker{}, fmt.Errorf("ownership marker contains trailing JSON")
		}
		return previewOwnershipMarker{}, err
	}
	if err := marker.validate(); err != nil {
		return previewOwnershipMarker{}, err
	}
	return marker, nil
}

func isManagedPresentationPreviewPage(name string) bool {
	if !strings.HasPrefix(name, "page-") || !strings.HasSuffix(name, ".png") {
		return false
	}
	page := strings.TrimSuffix(strings.TrimPrefix(name, "page-"), ".png")
	if page == "" {
		return false
	}
	for _, digit := range page {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(page)
	return err == nil && number > 0
}

func (s *presentationPreviewSwap) rollback() error {
	if s == nil {
		return nil
	}
	var errs []error
	if err := os.RemoveAll(s.finalDir); err != nil {
		errs = append(errs, fmt.Errorf("remove newly published preview %s: %w", s.finalDir, err))
	}
	if s.backupDir != "" {
		if err := s.restoreBackup(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := syncDirectory(filepath.Dir(s.finalDir)); err != nil {
		errs = append(errs, fmt.Errorf("sync preview parent after rollback: %w", err))
	}
	return errors.Join(errs...)
}

func (s *presentationPreviewSwap) restoreBackup() error {
	if s == nil || s.backupDir == "" {
		return nil
	}
	backupDir := s.backupDir
	if err := os.Rename(backupDir, s.finalDir); err != nil {
		return fmt.Errorf("restore preview backup from %s to %s failed; backup retained at %s: %w", backupDir, s.finalDir, backupDir, err)
	}
	s.backupDir = ""
	return nil
}

func (s *presentationPreviewSwap) commit() error {
	if s == nil || s.backupDir == "" {
		return nil
	}
	backupDir := s.backupDir
	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("remove committed preview backup %s: %w", backupDir, err)
	}
	s.backupDir = ""
	if err := syncDirectory(filepath.Dir(s.finalDir)); err != nil {
		return fmt.Errorf("sync preview parent after commit: %w", err)
	}
	return nil
}

func rebasePresentationPreviewReport(report presentationRenderReport, fromDir, toDir string) presentationRenderReport {
	rebase := func(path string) string {
		if strings.TrimSpace(path) == "" {
			return ""
		}
		rel, err := filepath.Rel(fromDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return path
		}
		return filepath.Join(toDir, rel)
	}
	report.PDFPath = rebase(report.PDFPath)
	for index := range report.PreviewImages {
		report.PreviewImages[index] = rebase(report.PreviewImages[index])
	}
	report.PreviewDir = toDir
	return report
}

func publishPresentationFile(stagedPath, finalPath string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(finalPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("presentation output path is not a regular file: %s", finalPath)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Chmod(stagedPath, mode); err != nil {
		return fmt.Errorf("set published presentation permissions: %w", err)
	}
	if err := syncRegularFile(stagedPath); err != nil {
		return fmt.Errorf("sync staged presentation: %w", err)
	}
	backupPath, err := backupPresentationFile(finalPath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed && backupPath != "" {
			_ = os.Remove(backupPath)
		}
	}()
	if err := os.Rename(stagedPath, finalPath); err != nil {
		if backupPath != "" {
			_ = os.Remove(backupPath)
		}
		return fmt.Errorf("atomically publish presentation: %w", err)
	}
	if err := syncDirectory(filepath.Dir(finalPath)); err != nil {
		rollbackErr := rollbackPresentationFile(finalPath, backupPath)
		if rollbackErr != nil {
			return fmt.Errorf("sync published presentation: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("sync published presentation: %w", err)
	}
	committed = true
	return nil
}

func backupPresentationFile(finalPath string) (string, error) {
	info, err := os.Stat(finalPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("presentation output path is not a regular file: %s", finalPath)
	}
	backup, err := os.CreateTemp(filepath.Dir(finalPath), "."+filepath.Base(finalPath)+".awaken-backup-*")
	if err != nil {
		return "", err
	}
	backupPath := backup.Name()
	source, err := os.Open(finalPath)
	if err != nil {
		_ = backup.Close()
		_ = os.Remove(backupPath)
		return "", err
	}
	_, copyErr := io.Copy(backup, source)
	closeSourceErr := source.Close()
	syncErr := backup.Sync()
	closeBackupErr := backup.Close()
	if err := errors.Join(copyErr, closeSourceErr, syncErr, closeBackupErr); err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("backup existing presentation: %w", err)
	}
	if err := os.Chmod(backupPath, info.Mode().Perm()); err != nil {
		_ = os.Remove(backupPath)
		return "", err
	}
	return backupPath, nil
}

func rollbackPresentationFile(finalPath, backupPath string) error {
	if backupPath == "" {
		if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return syncDirectory(filepath.Dir(finalPath))
	}
	if err := os.Rename(backupPath, finalPath); err != nil {
		// Windows may refuse replacement. The fallback is only for rollback;
		// the success path above still publishes with one atomic rename.
		if removeErr := os.Remove(finalPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return errors.Join(err, removeErr)
		}
		if retryErr := os.Rename(backupPath, finalPath); retryErr != nil {
			return errors.Join(err, retryErr)
		}
	}
	return syncDirectory(filepath.Dir(finalPath))
}

func syncRegularFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func syncDirectory(path string) error {
	if goruntime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}

// gatePresentationVerificationWarnings makes the publishing contract stricter
// than the transport/render contract. LibreOffice and Poppler can successfully
// produce PDF/PNG files while the semantic engine has already detected a blank,
// clipped, overflowing, or otherwise visually unsafe slide. Such an artifact is
// evidence for recovery, not a ready deliverable.
func gatePresentationVerificationWarnings(report presentationRenderReport, warnings ...string) presentationRenderReport {
	report.Warnings = appendUniqueStrings(warnings, report.Warnings...)
	if len(report.Warnings) > 0 {
		report.Verified = false
	}
	return report
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(append([]string(nil), base...), values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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

func renderPresentationPreviewAtomically(ctx context.Context, path, outputDir string, expectedSlides int) (presentationRenderReport, error) {
	stagedDir, err := newManagedPreviewStagingDir(outputDir)
	if err != nil {
		return presentationRenderReport{}, err
	}
	defer os.RemoveAll(stagedDir)

	report, err := renderPresentation(ctx, path, stagedDir, expectedSlides)
	if err != nil {
		return presentationRenderReport{}, err
	}
	if !report.Verified {
		return clearPresentationPreviewReferences(report), nil
	}
	policy := presentationManagedPreviewPolicy(path, outputDir)
	if err := publishManagedPreviewDirectory(stagedDir, outputDir, policy); err != nil {
		return presentationRenderReport{}, err
	}
	return rebasePresentationPreviewReport(report, stagedDir, outputDir), nil
}

func renderPDFPreviewAtomically(ctx context.Context, pdfPath, outputDir string) ([]string, error) {
	stagedDir, err := newManagedPreviewStagingDir(outputDir)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stagedDir)

	images, err := renderPDFPagesToPNG(ctx, pdfPath, stagedDir)
	if err != nil {
		return nil, err
	}
	if err := publishManagedPreviewDirectory(stagedDir, outputDir, pdfManagedPreviewPolicy(pdfPath, outputDir)); err != nil {
		return nil, err
	}
	published := make([]string, len(images))
	for index, image := range images {
		published[index] = filepath.Join(outputDir, filepath.Base(image))
	}
	return published, nil
}

func newManagedPreviewStagingDir(finalDir string) (string, error) {
	finalDir = filepath.Clean(finalDir)
	parent := filepath.Dir(finalDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create preview output parent: %w", err)
	}
	base := filepath.Base(finalDir)
	if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
		base = "preview"
	}
	stagedDir, err := os.MkdirTemp(parent, "."+base+".awaken-stage-*")
	if err != nil {
		return "", fmt.Errorf("create preview staging directory: %w", err)
	}
	return stagedDir, nil
}

func publishManagedPreviewDirectory(stagedDir, finalDir string, policy managedPreviewPolicy) error {
	if err := prepareManagedPreviewDirectory(stagedDir, policy); err != nil {
		return err
	}
	swap, err := beginManagedPreviewSwap(stagedDir, finalDir, policy)
	if err != nil {
		return err
	}
	return swap.commit()
}

func prepareManagedPreviewDirectory(stagedDir string, policy managedPreviewPolicy) error {
	if err := writePreviewOwnershipMarker(stagedDir, policy.marker); err != nil {
		return err
	}
	if err := validateManagedPreviewDir(stagedDir, policy); err != nil {
		return fmt.Errorf("validate staged preview: %w", err)
	}
	if err := preparePresentationPreviewPermissions(stagedDir); err != nil {
		return err
	}
	return syncManagedPreviewDirectory(stagedDir)
}

func writePreviewOwnershipMarker(previewDir string, marker previewOwnershipMarker) error {
	if err := marker.validate(); err != nil {
		return fmt.Errorf("invalid preview ownership marker: %w", err)
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preview ownership marker: %w", err)
	}
	data = append(data, '\n')
	markerPath := filepath.Join(previewDir, previewOwnershipMarkerName)
	file, err := os.OpenFile(markerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create reserved preview ownership marker %s: %w", markerPath, err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("write preview ownership marker %s: %w", markerPath, err)
	}
	return nil
}

func syncManagedPreviewDirectory(previewDir string) error {
	entries, err := os.ReadDir(previewDir)
	if err != nil {
		return fmt.Errorf("inspect staged preview for sync: %w", err)
	}
	for _, entry := range entries {
		path := filepath.Join(previewDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect staged preview entry %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to sync non-regular staged preview entry %s", path)
		}
		if err := syncRegularFile(path); err != nil {
			return fmt.Errorf("sync staged preview entry %s: %w", path, err)
		}
	}
	if err := syncDirectory(previewDir); err != nil {
		return fmt.Errorf("sync staged preview directory: %w", err)
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
	if err := clearPresentationRenderOutputs(path, outputDir); err != nil {
		return presentationRenderReport{}, fmt.Errorf("clear stale presentation preview: %w", err)
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
		report.NeedsRuntime = isRuntimeDependencyUnavailable(err)
		return report, nil
	}
	report.PDFPath = pdfPath
	report.VerificationLevel = "pdf"
	report.Engine = "libreoffice_pdf"
	report.PDFPageCount = slideCount // page count confirmed via PNG parity below

	pngs, err := renderPDFPagesToPNG(ctx, pdfPath, outputDir)
	if err != nil {
		report.Warnings = append(report.Warnings, err.Error())
		report.NeedsRuntime = isRuntimeDependencyUnavailable(err)
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

func isRuntimeDependencyUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "runtime is not available") ||
		strings.Contains(message, "executable file not found") ||
		strings.Contains(message, "no such file or directory")
}

func renderPresentationPDF(ctx context.Context, path string, outDir string) (string, error) {
	expectedPDF := presentationPDFPath(path, outDir)
	if err := removeIfExists(expectedPDF + ".pagecount"); err != nil {
		return "", fmt.Errorf("remove stale presentation PDF metadata: %w", err)
	}
	result, err := renderManagedPresentationPDF(ctx, path, expectedPDF)
	if err != nil {
		return "", fmt.Errorf("managed office render failed: %w", err)
	}
	return result.OutputPath, nil
}

func renderManagedPresentationPDF(ctx context.Context, inputPath, outputPath string) (officerender.Result, error) {
	testOverride := strings.TrimSpace(presentationRuntimeTestOverride("libreoffice"))
	if testOverride == "" {
		return officerender.Render(ctx, inputPath, outputPath, "pdf")
	}
	soffice := findSofficeIn(testOverride)
	if soffice == "" {
		return officerender.Result{}, fmt.Errorf("test LibreOffice runtime is not available")
	}
	soffice, err := filepath.Abs(soffice)
	if err != nil {
		return officerender.Result{}, err
	}
	root := filepath.Dir(soffice)
	presentationTestOfficeWorkers.Lock()
	worker := presentationTestOfficeWorkers.bySoffice[soffice]
	if worker == nil {
		worker = officerender.NewOfficeRenderWorker(officerender.Config{})
		presentationTestOfficeWorkers.bySoffice[soffice] = worker
	}
	presentationTestOfficeWorkers.Unlock()
	return worker.Render(ctx, officerender.Job{
		Runtime: officerender.Runtime{
			Revision:    "test:" + soffice,
			Root:        root,
			SOfficePath: soffice,
		},
		InputPath:  inputPath,
		OutputPath: outputPath,
		Format:     "pdf",
	})
}

func renderPDFPagesToPNG(ctx context.Context, pdfPath string, outputDir string) ([]string, error) {
	pdftoppm, err := resolvePopplerTool("pdftoppm")
	if err != nil {
		return nil, err
	}
	prefix := filepath.Join(outputDir, "page")
	if err := removeGlob(filepath.Join(outputDir, "page-*.png")); err != nil {
		return nil, fmt.Errorf("remove stale PDF preview pages: %w", err)
	}
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

func presentationPDFPath(path, outputDir string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return filepath.Join(outputDir, base+".pdf")
}

func clearPresentationRenderOutputs(path, outputDir string) error {
	pdfPath := presentationPDFPath(path, outputDir)
	for _, candidate := range []string{pdfPath, pdfPath + ".pagecount"} {
		if err := removeIfExists(candidate); err != nil {
			return err
		}
	}
	return removeGlob(filepath.Join(outputDir, "page-*.png"))
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func removeGlob(pattern string) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := removeIfExists(match); err != nil {
			return err
		}
	}
	return nil
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
	cmd := exec.CommandContext(ctx, path, "-c", "import pptx, PIL")
	return cmd.Run() == nil
}

func fileURL(path string) string {
	return "file://" + filepath.ToSlash(path)
}

func excerptOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	prefix, truncated := outputlimit.Prefix(text, 1200)
	if truncated {
		return prefix + "...[truncated]"
	}
	return prefix
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
	mustRegisterTypedArtifactTool(workspaceDependenciesTool())
	mustRegisterTypedArtifactTool(runtimePreflightTool())
	mustRegisterTypedArtifactTool(artifactInspectTool("artifact_read", "Artifact Read", "读取本机 artifact 的基础文件信息"))
	mustRegisterTypedArtifactTool(artifactInspectTool("artifact_inspect", "Artifact Inspect", "检查本机 artifact 类型、大小与基础元数据"))
	mustRegisterTypedArtifactTool(artifactExportTool())
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
		name:                "office_export_pdf",
		displayName:         "Office Export PDF",
		description:         "将已存在的 Word、Excel 或 PowerPoint 文件通过 Awaken managed Office 后台渲染为真实 PDF；先用对应 typed artifact 工具创建源文件，再用本工具转换，不要把 DOCX 内容直接写进 .pdf 路径",
		readOnly:            false,
		capability:          tool.CapabilityArtifactWrite,
		requiresRuntime:     true,
		runtimeRequirements: []string{"libreoffice"},
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "输入的 doc/docx/xls/xlsx/ppt/pptx 文件", Type: "string", Required: true},
			{Name: "output_path", Description: "输出 PDF 路径，必须位于 writable/output roots 内；默认与源文件同名", Type: "string", Required: false},
		},
		execute: officeExportPDF,
	})
	mustRegisterTypedArtifactTool(typedArtifactTool{
		name:                "presentation_create",
		displayName:         "Presentation Create",
		description:         "创建图文混排 pptx 演示文稿，支持 theme、slides、bullets、images、speaker notes 与固定 layout enum，并用 LibreOffice/Poppler 生成预览验证；需要 Awaken managed Python/python-pptx、LibreOffice、Poppler；模型只提交 semantic slide spec，不要用 apply_patch 手写 PPTX ZIP/XML 或 CJK helper",
		readOnly:            false,
		capability:          tool.CapabilityArtifactWrite,
		requiresRuntime:     true,
		runtimeRequirements: append([]string(nil), presentationAuthoringRuntimeRequirements...),
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
		name:                "presentation_edit",
		displayName:         "Presentation Edit",
		description:         "对已有 pptx 做增量编辑：必填 source_path 与 edits，按请求顺序执行且 slide_index 基于前一项执行后的当前页序；update 保留未指定正文和图片，replace 必须提供完整 slide；需要 Awaken managed Python/python-pptx、LibreOffice、Poppler",
		readOnly:            false,
		capability:          tool.CapabilityArtifactWrite,
		requiresRuntime:     true,
		runtimeRequirements: append([]string(nil), presentationAuthoringRuntimeRequirements...),
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
		name:                "presentation_render",
		displayName:         "Presentation Render",
		description:         "将本机 pptx 演示文稿渲染成 PDF 与 PNG 页面预览；需要 Awaken managed LibreOffice、Poppler；用于检查 PPT 版式和图片是否可见，不要用 shell 调 LibreOffice/Poppler",
		readOnly:            false,
		capability:          tool.CapabilityArtifactWrite,
		requiresRuntime:     true,
		runtimeRequirements: append([]string(nil), presentationRenderRuntimeRequirements...),
		parameters: []tool.ToolParameter{
			{Name: "path", Description: "输入 pptx 路径", Type: "string", Required: true},
			{Name: "output_dir", Description: "预览输出目录，必须在 writable/output roots 内；不传则使用同名 preview 目录", Type: "string", Required: false},
		},
		inputSchema: presentationRenderInputSchema(),
		execute:     presentationRender,
	})
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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
	mustRegisterTypedArtifactTool(typedArtifactTool{
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

}

func mustRegisterTypedArtifactTool(runtime tool.Tool) {
	tool.MustRegisterToolWithMode(runtime, tool.DefinitionValidationStrictBuiltin)
}
