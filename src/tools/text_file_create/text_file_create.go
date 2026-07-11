package textfilecreate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/tool"
)

const maxTextFileCreateBytes = 2 * 1024 * 1024

type TextFileCreateTool struct{}

type textFileCreateParams struct {
	Path                 string `json:"path"`
	Content              string `json:"content"`
	Append               bool   `json:"append"`
	PublishAsDeliverable bool   `json:"publish_as_deliverable"`
}

func (t *TextFileCreateTool) Name() string {
	return "text_file_create"
}

func (t *TextFileCreateTool) Description() string {
	return "创建或覆盖当前任务 writable roots 内的 Markdown/普通文本文件，适合先输出报告草稿、说明文档、JSON/CSV 等文本产物；不要用于 PDF、Office、图片、音视频。已有文件的精确局部修改请用 text_edit；多文件/移动/删除/高级 diff 才用 apply_patch"
}

func (t *TextFileCreateTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "要创建或覆盖的文本文件路径，必须位于 writable roots 内", Type: "string", Required: true},
		{Name: "content", Description: "完整文本内容，必须是 UTF-8；Markdown 报告可直接放在这里", Type: "string", Required: true},
		{Name: "append", Description: "为 true 时追加到文件末尾；默认 false 表示覆盖", Type: "boolean", Required: false},
		{Name: "publish_as_deliverable", Description: "为 true 时将创建的受支持文件显式发布为最终交付物；默认 false，仅记录为任务支持文件", Type: "boolean", Required: false},
	}
}

func (t *TextFileCreateTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "text_file_create",
		DisplayName: "Text File Create",
		Description: t.Description(),
		Purpose:     "创建、整体覆盖或追加 UTF-8 文本文件，并可显式发布受支持文本交付物。",
		WhenToUse: []string{
			"需要新建 Markdown、JSON、CSV、TSV 或普通文本文件时",
			"已明确要整体覆盖或追加完整文本内容时",
		},
		WhenNotToUse: []string{
			"修改现有文件中的局部字符串使用 text_edit",
			"生成 PDF、Office、图片或其他二进制产物使用对应 typed artifact 工具",
			"删除或移动文件使用 trash_file 或 apply_patch",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"path":                     {Type: "string", Description: "实际写入路径"},
			"action":                   {Type: "string", Enum: []any{"created", "overwritten", "appended"}, Description: "执行的写入动作"},
			"bytes_written":            {Type: "integer", Description: "本次写入字节数"},
			"published_as_deliverable": {Type: "boolean", Description: "是否同时显式发布为最终交付物"},
		}},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"one text file"}, Description: "目标不存在时创建 writable-root 内文本文件。", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectUpdate, Resources: []string{"one existing text file"}, Description: "目标存在时整体覆盖或追加内容。", Reversibility: tool.ReversibilityConditional},
		},
		Examples: []tool.ToolExample{{Scenario: "创建 Markdown 报告", Input: map[string]any{"path": "report.md", "content": "# Report\n", "append": false}, Outcome: "返回实际路径、写入动作和字节数"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeValidationError, Cause: "路径、UTF-8 内容、大小或发布类型无效", Retryable: false, Recovery: []string{"修正输入或选择 typed artifact 工具"}},
			{Code: tool.ErrorCodeToolDenied, Cause: "目标不在 writable/output roots 内", Retryable: false, Recovery: []string{"选择任务可写或输出路径"}},
			{Code: tool.ErrorCodeToolExecutionFailed, Cause: "目录创建或文件写入失败", Retryable: true, Recovery: []string{"检查文件系统状态后重试"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite,
		},
	})
}

func (t *TextFileCreateTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseTextFileCreateParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if strings.TrimSpace(params.Path) == "" {
		return tool.DenyPermission("text_file_create requires path")
	}
	if params.PublishAsDeliverable && !isPublishableTextFilePath(params.Path) {
		return tool.DenyPermission("text_file_create can publish only .md, .csv, and .tsv text files; use typed artifact tools or artifact_publish for Office/PDF files")
	}
	if _, err := permissionContext.ResolveMutationPathChecked(params.Path); err != nil {
		return tool.DenyPermission(err.Error())
	}
	if params.PublishAsDeliverable {
		if _, err := permissionContext.ResolveOutputPathChecked(params.Path); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "Create text file",
		Description:   params.Path,
		RiskTone:      "warning",
		AffectedPaths: []string{params.Path},
	})
}

func (t *TextFileCreateTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return outcome.Content, tool.ToolError{
			Code: outcome.ErrorCode, Category: outcome.ErrorCategory, Message: outcome.ErrorMessage,
			Retryable: outcome.Retryable, RecoveryOptions: outcome.RecoveryOptions, Diagnostic: outcome.Diagnostic,
		}
	}
	return outcome.Content, nil
}

func (t *TextFileCreateTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		return cancelledTextFileOutcome(err).WithTiming(started, time.Now().UTC()), nil
	}
	params, err := parseTextFileCreateParams(input)
	if err != nil {
		return textFileFailure(started, err, tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false, []string{"correct_input"}), nil
	}
	if strings.TrimSpace(params.Path) == "" {
		return textFileFailure(started, errors.New("path is required"), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false, []string{"provide_path"}), nil
	}
	if params.PublishAsDeliverable && !isPublishableTextFilePath(params.Path) {
		err := errors.New("text_file_create can publish only .md, .csv, and .tsv text files; use typed artifact tools or artifact_publish for Office/PDF files")
		return textFileFailure(started, err, tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false, []string{"use_typed_artifact_tool"}), nil
	}
	if !utf8.ValidString(params.Content) {
		return textFileFailure(started, errors.New("content must be valid UTF-8 text"), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false, []string{"provide_utf8_text"}), nil
	}
	if len([]byte(params.Content)) > maxTextFileCreateBytes {
		err := fmt.Errorf("content is too large for text_file_create: %d bytes > %d bytes", len([]byte(params.Content)), maxTextFileCreateBytes)
		return textFileFailure(started, err, tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false, []string{"reduce_content_size"}), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	outputPath, err := permissionContext.ResolveMutationPathChecked(params.Path)
	if err != nil {
		outcome := tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied)
		outcome.RecoveryOptions = []string{"choose_path_within_writable_roots"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if params.PublishAsDeliverable {
		if _, err := permissionContext.ResolveOutputPathChecked(params.Path); err != nil {
			outcome := tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied)
			outcome.RecoveryOptions = []string{"choose_path_within_output_roots"}
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return textFileFailure(started, err, tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, true, []string{"inspect_filesystem", "retry_after_fix"}), nil
	}

	action := "created"
	if params.Append {
		if _, statErr := os.Stat(outputPath); statErr == nil {
			action = "appended"
		}
		file, openErr := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr != nil {
			return textFileFailure(started, openErr, tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, true, []string{"inspect_filesystem"}), nil
		}
		if _, writeErr := file.WriteString(params.Content); writeErr != nil {
			_ = file.Close()
			return textFileFailure(started, writeErr, tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, true, []string{"inspect_filesystem"}), nil
		}
		if closeErr := file.Close(); closeErr != nil {
			return textFileFailure(started, closeErr, tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, true, []string{"verify_file_state_before_retry"}), nil
		}
	} else {
		if _, statErr := os.Stat(outputPath); statErr == nil {
			action = "overwritten"
		}
		if err := os.WriteFile(outputPath, []byte(params.Content), 0o644); err != nil {
			return textFileFailure(started, err, tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, true, []string{"inspect_filesystem", "verify_file_state_before_retry"}), nil
		}
	}

	bytesWritten := len([]byte(params.Content))
	rel := filepath.ToSlash(relativeTextFilePath(outputPath, permissionContext.WorkspaceRootOrCWD()))
	content := fmt.Sprintf("%s %s (%d bytes)", action, outputPath, bytesWritten)
	outcome := tool.SucceededOutcome(content).WithTiming(started, time.Now().UTC())
	outcome.AffectedPaths = []string{rel}
	outcome.Summary = fmt.Sprintf("%s text file %s", action, rel)
	outcome.Details = map[string]any{"path": rel, "action": action, "bytes_written": bytesWritten, "published_as_deliverable": params.PublishAsDeliverable}
	outcome.Changes = []tool.ResourceChange{{
		Kind: "file", Action: action, Resource: rel, Summary: outcome.Summary,
		Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
	}}
	return outcome, nil
}

func textFileFailure(started time.Time, err error, code string, category tool.ErrorCategory, retryable bool, recovery []string) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(err.Error(), code, category, retryable)
	if category == tool.ErrorCategoryValidation || category == tool.ErrorCategoryPolicy || category == tool.ErrorCategoryAuthorization {
		outcome.TransportStatus = tool.TransportNotStarted
	}
	outcome.RecoveryOptions = append([]string(nil), recovery...)
	return outcome.WithTiming(started, time.Now().UTC())
}

func cancelledTextFileOutcome(err error) tool.ExecutionOutcome {
	now := time.Now().UTC()
	return tool.ExecutionOutcome{
		Content: err.Error(), Status: tool.OutcomeCancelled, TransportStatus: tool.TransportCancelled,
		SemanticStatus: tool.SemanticFailure, ProcessStatus: tool.ProcessNotApplicable,
		ErrorCode: tool.ErrorCodeToolCancelled, ErrorCategory: tool.ErrorCategoryCancelled,
		ErrorMessage: err.Error(), Retryable: false, StartedAt: now, FinishedAt: now,
	}
}

func (t *TextFileCreateTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	params, err := parseTextFileCreateParams(input)
	if err != nil || strings.TrimSpace(params.Path) == "" {
		return nil
	}
	path := filepath.ToSlash(relativeTextFilePath(permissionContext.ResolvePath(params.Path), permissionContext.WorkspaceRootOrCWD()))
	role := tool.ArtifactRoleSupport
	if params.PublishAsDeliverable && isPublishableTextFilePath(path) {
		role = tool.ArtifactRoleDeliverable
	}
	artifactAction := tool.ArtifactActionCreated
	description := "Created " + path
	switch {
	case strings.HasPrefix(strings.TrimSpace(output), "appended "):
		artifactAction = tool.ArtifactActionUpdated
		description = "Appended to " + path
	case strings.HasPrefix(strings.TrimSpace(output), "overwritten "):
		artifactAction = tool.ArtifactActionUpdated
		description = "Overwrote " + path
	}
	return []tool.ArtifactDescriptor{{
		ContractVersion: tool.ArtifactContractVersion,
		Role:            role,
		Producer:        "text_file_create",
		Action:          artifactAction,
		ID:              "text_file_create:" + path,
		Title:           path,
		Description:     description,
		Kind:            "text",
		Path:            path,
		Content:         params.Content,
		Status:          "ready",
	}}
}

func isPublishableTextFilePath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".md", ".csv", ".tsv":
		return true
	default:
		return false
	}
}

func parseTextFileCreateParams(input string) (textFileCreateParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return textFileCreateParams{}, nil
	}
	var params textFileCreateParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return textFileCreateParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	return params, nil
}

func relativeTextFilePath(path string, root string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return rel
	}
	return filepath.Clean(path)
}

func init() {
	tool.MustRegisterToolWithMode(&TextFileCreateTool{}, tool.DefinitionValidationStrictBuiltin)
}
