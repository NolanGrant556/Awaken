package textedit

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

const (
	ErrorCodeTextNotFound        = "text_not_found"
	ErrorCodeTextNotUnique       = "text_not_unique"
	ErrorCodeStaleFileRevision   = "stale_file_revision"
	ErrorCodeValidationError     = "validation_error"
	ErrorCodeToolExecutionFailed = "tool_execution_failed"
)

var (
	errTextNotFound  = errors.New(ErrorCodeTextNotFound)
	errTextNotUnique = errors.New(ErrorCodeTextNotUnique)
)

type TextEditTool struct{}

type textEditParams struct {
	Path           string `json:"path"`
	OldString      string `json:"old_string"`
	NewString      string `json:"new_string"`
	ReplaceAll     bool   `json:"replace_all"`
	ExpectedSHA256 string `json:"expected_sha256"`
}

type textEditResult struct {
	Path             string `json:"path"`
	ReplacementCount int    `json:"replacement_count"`
	BeforeSHA256     string `json:"before_sha256"`
	AfterSHA256      string `json:"after_sha256"`
	UnifiedDiff      string `json:"unified_diff"`
	ExpectedSHA256   string `json:"expected_sha256,omitempty"`
}

func (t *TextEditTool) Name() string {
	return "text_edit"
}

func (t *TextEditTool) Description() string {
	return "对单个文本文件做精确字符串替换；参数为 path、old_string、new_string、可选 replace_all 与 expected_sha256。普通单文件精确修改优先使用本工具；多文件/移动/删除/高级 diff 才用 apply_patch；新建普通文本用 text_file_create；删除用 trash_file"
}

func (t *TextEditTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "要修改的文本文件路径，必须位于 writable roots 内", Type: "string", Required: true},
		{Name: "old_string", Description: "要替换的原文，必须在文件中精确匹配", Type: "string", Required: true},
		{Name: "new_string", Description: "替换后的新文本，不得与 old_string 完全相同", Type: "string", Required: true},
		{Name: "replace_all", Description: "为 true 时替换全部匹配；默认 false，多处匹配会失败", Type: "boolean", Required: false},
		{Name: "expected_sha256", Description: "可选；来自 read_file 的文件 digest，不匹配则拒绝写入", Type: "string", Required: false},
	}
}

func (t *TextEditTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "text_edit",
		DisplayName: "Text Edit",
		Description: t.Description(),
		Purpose:     "在一个现有文本文件中执行精确、可校验的字符串替换。",
		WhenToUse: []string{
			"已知一个现有文件中的精确 old_string，并要做局部替换时",
			"需要用 read_file 返回的 expected_sha256 防止覆盖并发修改时",
		},
		WhenNotToUse: []string{
			"新建或整体覆盖文本文件使用 text_file_create",
			"多文件移动、删除或高级 patch 使用 apply_patch；删除单个资源使用 trash_file",
			"old_string 尚未从当前文件事实中确认时先使用 read_file",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"path":              {Type: "string", Description: "实际修改的文件"},
			"replacement_count": {Type: "integer", Description: "完成的替换次数"},
			"before_sha256":     {Type: "string", Description: "写入前摘要"},
			"after_sha256":      {Type: "string", Description: "写入后摘要"},
			"unified_diff":      {Type: "string", Description: "本次文本变化 diff"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectUpdate, Resources: []string{"one existing text file"}, Description: "原子更新一个 writable-root 内文本文件。", Reversibility: tool.ReversibilityConditional}},
		Examples:    []tool.ToolExample{{Scenario: "基于已读取摘要修改唯一字符串", Input: map[string]any{"path": "docs/note.md", "old_string": "旧标题", "new_string": "新标题", "expected_sha256": "<read_file sha256>"}, Outcome: "返回替换数、前后摘要和 unified diff"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: ErrorCodeStaleFileRevision, Cause: "文件摘要与 expected_sha256 不一致", Retryable: true, Recovery: []string{"重新 read_file", "基于新内容重建编辑"}},
			{Code: ErrorCodeTextNotFound, Cause: "old_string 在当前文件中不存在", Retryable: true, Recovery: []string{"重新读取文件", "修正 old_string"}},
			{Code: ErrorCodeTextNotUnique, Cause: "old_string 多处匹配且 replace_all=false", Retryable: true, Recovery: []string{"扩大上下文", "确认后使用 replace_all"}},
			{Code: tool.ErrorCodeToolDenied, Cause: "目标不在 writable roots 内", Retryable: false, Recovery: []string{"选择任务可写路径"}},
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

func (t *TextEditTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseTextEditParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if strings.TrimSpace(params.Path) == "" {
		return tool.DenyPermission("text_edit requires path")
	}
	if _, err := permissionContext.ResolveMutationPathChecked(params.Path); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "Edit text file",
		Description:   params.Path,
		RiskTone:      "warning",
		AffectedPaths: []string{params.Path},
	})
}

func (t *TextEditTool) DescribeActivity(input string) tool.ToolActivity {
	params, err := parseTextEditParams(input)
	if err != nil || strings.TrimSpace(params.Path) == "" {
		return tool.ToolActivity{Stage: "started", Title: "正在修改文件"}
	}
	return tool.ToolActivity{
		Stage:  "started",
		Title:  "正在修改文件",
		Detail: params.Path,
	}
}

func (t *TextEditTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return outcome.Content, errors.New(outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *TextEditTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		return tool.ExecutionOutcome{}, err
	}

	params, err := parseTextEditParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_json_parameters"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if err := validateTextEditParams(params); err != nil {
		outcome := tool.FailedOutcome(err.Error(), ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_parameters", "ensure_old_and_new_differ"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	resolvedPath, err := permissionContext.ResolveMutationPathChecked(params.Path)
	if err != nil {
		outcome := tool.DeniedOutcome(err.Error(), tool.ErrorCodeToolDenied)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	before, beforeDigest, mode, err := readFileForEdit(resolvedPath)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), ErrorCodeToolExecutionFailed, tool.ErrorCategoryTool, true)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	expected := strings.TrimSpace(params.ExpectedSHA256)
	if expected != "" && !strings.EqualFold(expected, beforeDigest) {
		msg := fmt.Sprintf("%s: file %s digest is %s, expected %s; re-read with read_file then retry text_edit",
			ErrorCodeStaleFileRevision, params.Path, beforeDigest, expected)
		outcome := tool.FailedOutcome(msg, ErrorCodeStaleFileRevision, tool.ErrorCategoryDomain, true)
		outcome.RecoveryOptions = []string{"read_file", "retry_text_edit_with_fresh_digest"}
		outcome.Diagnostic = map[string]any{
			"path":            params.Path,
			"before_sha256":   beforeDigest,
			"expected_sha256": expected,
		}
		outcome.AffectedPaths = []string{relativeEditPath(resolvedPath, permissionContext.WorkspaceRootOrCWD())}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	after, count, err := applyReplacements(before, params.OldString, params.NewString, params.ReplaceAll)
	if err != nil {
		return textEditMatchFailure(started, params, resolvedPath, permissionContext, beforeDigest, count, err), nil
	}

	if err := atomicWriteFile(resolvedPath, []byte(after), mode); err != nil {
		msg := fmt.Sprintf("atomic write failed: %v", err)
		outcome := tool.FailedOutcome(msg, ErrorCodeToolExecutionFailed, tool.ErrorCategoryPersistence, true)
		outcome.RecoveryOptions = []string{"retry_text_edit", "inspect_filesystem"}
		outcome.Diagnostic = map[string]any{
			"path":          params.Path,
			"before_sha256": beforeDigest,
		}
		// Confirm original still intact.
		if current, curErr := os.ReadFile(resolvedPath); curErr == nil && string(current) == before {
			outcome.Diagnostic["original_preserved"] = true
		}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	afterDigest := fileDigest([]byte(after))
	rel := filepath.ToSlash(relativeEditPath(resolvedPath, permissionContext.WorkspaceRootOrCWD()))
	diff := buildUnifiedDiff(rel, before, after)
	result := textEditResult{
		Path:             rel,
		ReplacementCount: count,
		BeforeSHA256:     beforeDigest,
		AfterSHA256:      afterDigest,
		UnifiedDiff:      diff,
		ExpectedSHA256:   expected,
	}
	payload, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return tool.ExecutionOutcome{}, marshalErr
	}

	artifacts := buildTextEditArtifacts(rel, count)
	outcome := tool.SucceededOutcome(string(payload)).WithTiming(started, time.Now().UTC())
	outcome.AffectedPaths = []string{rel}
	outcome.PatchSummary = fmt.Sprintf("修改 1 个文件，%d 处替换", count)
	outcome.Artifacts = artifacts
	outcome.Diagnostic = map[string]any{
		"path":              rel,
		"replacement_count": count,
		"before_sha256":     beforeDigest,
		"after_sha256":      afterDigest,
		"sha256":            afterDigest,
	}
	outcome.Details = map[string]any{
		"path": rel, "replacement_count": count,
		"before_sha256": beforeDigest, "after_sha256": afterDigest,
		"unified_diff": diff,
	}
	return outcome, nil
}

func (t *TextEditTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	params, err := parseTextEditParams(input)
	if err != nil || strings.TrimSpace(params.Path) == "" {
		return nil
	}
	rel := filepath.ToSlash(relativeEditPath(permissionContext.ResolvePath(params.Path), permissionContext.WorkspaceRootOrCWD()))
	count := 1
	var parsed textEditResult
	if json.Unmarshal([]byte(output), &parsed) == nil && parsed.ReplacementCount > 0 {
		count = parsed.ReplacementCount
		if parsed.Path != "" {
			rel = parsed.Path
		}
	}
	return buildTextEditArtifacts(rel, count)
}

func (t *TextEditTool) PatchPreview(input string, permissionContext tool.PermissionContext) (tool.PatchPreview, bool) {
	params, err := parseTextEditParams(input)
	if err != nil || strings.TrimSpace(params.Path) == "" {
		return tool.PatchPreview{}, false
	}
	rel := filepath.ToSlash(relativeEditPath(permissionContext.ResolvePath(params.Path), permissionContext.WorkspaceRootOrCWD()))
	summary := "修改 1 个文件"
	preview := fmt.Sprintf("text_edit %s\n-old_string (%d chars)\n+new_string (%d chars)",
		rel, len(params.OldString), len(params.NewString))
	if params.ReplaceAll {
		summary += "（replace_all）"
	}
	return tool.PatchPreview{
		Patch:         preview,
		Summary:       summary,
		AffectedPaths: []string{rel},
	}, true
}

func textEditMatchFailure(
	started time.Time,
	params textEditParams,
	resolvedPath string,
	permissionContext tool.PermissionContext,
	beforeDigest string,
	count int,
	err error,
) tool.ExecutionOutcome {
	rel := filepath.ToSlash(relativeEditPath(resolvedPath, permissionContext.WorkspaceRootOrCWD()))
	code := ErrorCodeToolExecutionFailed
	recovery := []string{"read_file", "retry_text_edit"}
	msg := err.Error()
	switch {
	case errors.Is(err, errTextNotFound):
		code = ErrorCodeTextNotFound
		msg = fmt.Sprintf("%s: old_string not found in %s", ErrorCodeTextNotFound, params.Path)
		recovery = []string{"read_file", "widen_or_correct_old_string"}
	case errors.Is(err, errTextNotUnique):
		code = ErrorCodeTextNotUnique
		msg = fmt.Sprintf("%s: old_string matched %d times in %s; enlarge context or set replace_all=true",
			ErrorCodeTextNotUnique, count, params.Path)
		recovery = []string{"enlarge_old_string_context", "set_replace_all_true"}
	}
	outcome := tool.FailedOutcome(msg, code, tool.ErrorCategoryDomain, true)
	outcome.RecoveryOptions = recovery
	outcome.AffectedPaths = []string{rel}
	outcome.Diagnostic = map[string]any{
		"path":          rel,
		"before_sha256": beforeDigest,
		"match_count":   count,
		"replace_all":   params.ReplaceAll,
	}
	return outcome.WithTiming(started, time.Now().UTC())
}

func validateTextEditParams(params textEditParams) error {
	if strings.TrimSpace(params.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if params.OldString == "" {
		return fmt.Errorf("old_string is required and must be non-empty")
	}
	if params.OldString == params.NewString {
		return fmt.Errorf("old_string and new_string must differ")
	}
	return nil
}

func parseTextEditParams(input string) (textEditParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return textEditParams{}, fmt.Errorf("parameters are required")
	}
	var params textEditParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return textEditParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	return params, nil
}

func init() {
	tool.MustRegisterToolWithMode(&TextEditTool{}, tool.DefinitionValidationStrictBuiltin)
}
