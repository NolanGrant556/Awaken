package artifactpublish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/tool"
)

type ArtifactPublishTool struct{}

type artifactPublishParams struct {
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
}

type artifactPublishResult struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	Title  string `json:"title"`
	Kind   string `json:"kind"`
}

type artifactPublishError struct {
	code       string
	category   tool.ErrorCategory
	retryable  bool
	recovery   []string
	message    string
	diagnostic map[string]any
}

func (e *artifactPublishError) Error() string { return e.message }

func (t *ArtifactPublishTool) Name() string { return "artifact_publish" }

func (t *ArtifactPublishTool) Description() string {
	return "将已经存在的 Word、Markdown、Excel/CSV、PDF 或 PowerPoint 普通文件显式发布为最终文档交付物；不支持图片或视频，image_generate/video_generate 会直接产生媒体交付物"
}

func (t *ArtifactPublishTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "要发布的现有文档路径；只支持 .md、.doc/.docx、.xls/.xlsx、.csv/.tsv、.pdf、.ppt/.pptx，不接受图片或视频", Type: "string", Required: true},
		{Name: "title", Description: "最终交付卡片标题；默认使用文件名", Type: "string", Required: false},
	}
}

func (t *ArtifactPublishTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        t.Name(),
		DisplayName: "Artifact Publish",
		Description: t.Description(),
		Purpose:     "把已存在且受支持的文档、表格、演示稿或 PDF 标记为最终文档交付物，不改写文件内容；output roots 是默认存放位置而非硬性要求。",
		WhenToUse: []string{
			"受支持的最终文档文件已经生成并通过校验，但尚未作为 deliverable 发布时",
			"需要为现有 Markdown、Office、PDF 或表格文件生成最终交付物事实时",
		},
		WhenNotToUse: []string{
			"image_generate 或 video_generate 成功时不得再次发布；它们已经产生 ready 媒体 Artifact 和专用前端交付卡",
			"任何图片或视频文件都不属于本工具支持范围",
			"文件尚未创建、修改或验证时先完成生成与校验",
			"扩展名不受支持时不得发布",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"publication_status": {Type: "string", Enum: []any{"published"}, Description: "交付物发布分类状态"},
			"path":               {Type: "string", Description: "发布的实际文件路径"},
			"title":              {Type: "string", Description: "交付卡片标题"},
			"kind":               {Type: "string", Description: "文件交付类型"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"existing output artifact"}, Description: "读取并校验现有文件，然后发布交付物元数据；不修改文件内容。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "发布已生成的 PDF", Input: map[string]any{"path": "output/report.pdf", "title": "Final Report"}, Outcome: "返回 published 状态并产生 deliverable artifact"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "artifact_publish_invalid_input", Cause: "输入不是有效 JSON 或 path 缺失", Retryable: false, Recovery: []string{"提供现有且受支持的文档文件"}},
			{Code: "artifact_publish_unsupported_type", Cause: "扩展名不属于可发布的文档交付物；图片/视频由专用媒体工具直接交付", Retryable: false, Recovery: []string{"选择受支持的文档类型", "媒体任务使用 image_generate 或 video_generate，且成功后不要二次发布"}},
			{Code: "artifact_publish_outside_output_root", Cause: "路径无法安全解析（例如断链的符号链接）", Retryable: false, Recovery: []string{"确认文件真实存在再发布"}},
			{Code: "artifact_publish_file_unavailable", Cause: "目标文件尚不存在", Retryable: true, Recovery: []string{"完成文件生成并验证实际路径"}},
			{Code: "artifact_publish_permission_denied", Cause: "现有文件不可读", Retryable: false, Recovery: []string{"选择可读文件或修正文件权限"}},
			{Code: "artifact_publish_not_regular_file", Cause: "目标是目录、符号链接或其他非普通文件", Retryable: false, Recovery: []string{"选择一个普通文件"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeRead,
			NetworkScope:    tool.NetworkScopeNone,
		},
	})
}

func (t *ArtifactPublishTool) IsReadOnly() bool { return true }

func (t *ArtifactPublishTool) CheckPermission(_ context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseArtifactPublishParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	_, err = validatePublishableArtifact(params.Path, permissionContext)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermission()
}

func (t *ArtifactPublishTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome records publication as a deliverable classification without
// claiming that the existing file was created or modified by this tool.
func (t *ArtifactPublishTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		outcome := tool.FailedOutcome(err.Error(), "artifact_publish_cancelled", tool.ErrorCategoryCancelled, true)
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.RecoveryOptions = []string{"retry_when_context_available"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	params, err := parseArtifactPublishParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "artifact_publish_invalid_input", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_json_parameters", "provide_output_artifact_path"}
		outcome.Diagnostic = map[string]any{"expected_input": "JSON object"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	resolved, err := validatePublishableArtifact(params.Path, permissionContext)
	if err != nil {
		outcome := artifactPublishFailureOutcome(err, params.Path)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = filepath.Base(resolved)
	}
	result, err := json.Marshal(artifactPublishResult{
		Status: "published",
		Path:   resolved,
		Title:  title,
		Kind:   tool.ArtifactKindForPath(resolved),
	})
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "artifact_publish_result_encode_failed", tool.ErrorCategoryTool, false)
		outcome.RecoveryOptions = []string{"report_tool_implementation_failure"}
		outcome.Diagnostic = map[string]any{"resolved_path": resolved}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	content := string(result)
	outcome := tool.SucceededOutcome(content)
	outcome.Artifacts = t.Artifacts(input, content, permissionContext)
	outcome.Diagnostic = map[string]any{
		"publication_status": "published", "path": resolved, "title": title,
		"kind": tool.ArtifactKindForPath(resolved), "artifact_count": len(outcome.Artifacts),
		"file_modified": false,
	}
	outcome.Details = map[string]any{
		"publication_status": "published", "path": resolved,
		"title": title, "kind": tool.ArtifactKindForPath(resolved),
	}
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func artifactPublishFailureOutcome(err error, requestedPath string) tool.ExecutionOutcome {
	var publishErr *artifactPublishError
	if errors.As(err, &publishErr) {
		outcome := tool.FailedOutcome(publishErr.message, publishErr.code, publishErr.category, publishErr.retryable)
		if publishErr.category == tool.ErrorCategoryPolicy {
			outcome.Status = tool.OutcomeDenied
			outcome.TransportStatus = tool.TransportNotStarted
		}
		outcome.RecoveryOptions = append([]string(nil), publishErr.recovery...)
		outcome.Diagnostic = map[string]any{"requested_path": requestedPath}
		for key, value := range publishErr.diagnostic {
			outcome.Diagnostic[key] = value
		}
		return outcome
	}
	outcome := tool.FailedOutcome(err.Error(), "artifact_publish_validation_failed", tool.ErrorCategoryDomain, false)
	outcome.RecoveryOptions = []string{"verify_output_artifact", "choose_supported_output_file"}
	outcome.Diagnostic = map[string]any{"requested_path": requestedPath}
	return outcome
}

func (t *ArtifactPublishTool) Artifacts(input string, _ string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	params, err := parseArtifactPublishParams(input)
	if err != nil {
		return nil
	}
	resolved, err := validatePublishableArtifact(params.Path, permissionContext)
	if err != nil {
		return nil
	}
	displayPath := filepath.ToSlash(relativeArtifactPath(resolved, permissionContext.WorkspaceRootOrCWD()))
	title := strings.TrimSpace(params.Title)
	if title == "" {
		title = filepath.Base(resolved)
	}
	return []tool.ArtifactDescriptor{{
		ContractVersion: tool.ArtifactContractVersion,
		Role:            tool.ArtifactRoleDeliverable,
		Producer:        t.Name(),
		Action:          tool.ArtifactActionPublished,
		ID:              t.Name() + ":" + displayPath,
		Title:           title,
		Description:     "Published " + displayPath + " as a final deliverable",
		Kind:            tool.ArtifactKindForPath(resolved),
		Path:            displayPath,
		Status:          "ready",
	}}
}

func parseArtifactPublishParams(input string) (artifactPublishParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return artifactPublishParams{}, fmt.Errorf("path is required")
	}
	var params artifactPublishParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return artifactPublishParams{}, fmt.Errorf("parameters must be a JSON object: %w", err)
	}
	if strings.TrimSpace(params.Path) == "" {
		return artifactPublishParams{}, fmt.Errorf("path is required")
	}
	return params, nil
}

func validatePublishableArtifact(path string, permissionContext tool.PermissionContext) (string, error) {
	if !tool.IsPublishableArtifactPath(path) {
		return "", &artifactPublishError{
			code: "artifact_publish_unsupported_type", category: tool.ErrorCategoryDomain,
			recovery: []string{"choose_supported_document_file", "use_image_generate_or_video_generate_for_media"},
			message:  "artifact_publish only supports document deliverables: .md, .doc/.docx, .xls/.xlsx, .csv/.tsv, .pdf, and .ppt/.pptx; image_generate/video_generate already publish ready media artifacts and must not be followed by artifact_publish",
		}
	}
	resolved, err := permissionContext.ResolveOutputPathChecked(path)
	if err != nil {
		return "", &artifactPublishError{
			code: "artifact_publish_outside_output_root", category: tool.ErrorCategoryPolicy,
			recovery: []string{"choose_file_in_output_root", "create_final_artifact_in_output_root"},
			message:  err.Error(), diagnostic: map[string]any{"boundary": "output_roots"},
		}
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		code := "artifact_publish_file_unavailable"
		category := tool.ErrorCategoryDomain
		retryable := errors.Is(err, os.ErrNotExist)
		recovery := []string{"verify_artifact_path", "complete_artifact_generation"}
		if errors.Is(err, os.ErrPermission) {
			code = "artifact_publish_permission_denied"
			category = tool.ErrorCategoryAuthorization
			retryable = false
			recovery = []string{"choose_readable_output_file", "adjust_file_permissions"}
		}
		return "", &artifactPublishError{
			code: code, category: category, retryable: retryable, recovery: recovery,
			message:    fmt.Sprintf("cannot publish %q: %v", path, err),
			diagnostic: map[string]any{"resolved_path": resolved},
		}
	}
	if !info.Mode().IsRegular() {
		return "", &artifactPublishError{
			code: "artifact_publish_not_regular_file", category: tool.ErrorCategoryDomain,
			recovery:   []string{"choose_regular_output_file"},
			message:    fmt.Sprintf("cannot publish %q: path is not a regular file", path),
			diagnostic: map[string]any{"resolved_path": resolved, "mode": info.Mode().String()},
		}
	}
	if strings.EqualFold(filepath.Ext(resolved), ".pdf") {
		file, openErr := os.Open(resolved)
		if openErr != nil {
			return "", &artifactPublishError{
				code: "artifact_publish_file_unavailable", category: tool.ErrorCategoryDomain,
				retryable: true, recovery: []string{"verify_artifact_path", "complete_artifact_generation"},
				message: fmt.Sprintf("cannot validate PDF %q: %v", path, openErr),
			}
		}
		header := make([]byte, 5)
		count, readErr := file.Read(header)
		_ = file.Close()
		if readErr != nil || count != len(header) || string(header) != "%PDF-" {
			return "", &artifactPublishError{
				code: "artifact_publish_content_mismatch", category: tool.ErrorCategoryDomain,
				recovery:   []string{"regenerate_valid_pdf", "choose_supported_output_file"},
				message:    fmt.Sprintf("cannot publish %q: file extension is .pdf but content is not a valid PDF", path),
				diagnostic: map[string]any{"resolved_path": resolved, "expected_signature": "%PDF-"},
			}
		}
	}
	return resolved, nil
}

func relativeArtifactPath(path, root string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	rel, err := filepath.Rel(root, path)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return filepath.Clean(path)
}

func init() {
	tool.MustRegisterToolWithMode(&ArtifactPublishTool{}, tool.DefinitionValidationStrictBuiltin)
}
