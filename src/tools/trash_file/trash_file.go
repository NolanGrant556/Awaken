package trashfile

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tool/trash"
)

type TrashFileTool struct{}

var _ tool.OutcomeTool = (*TrashFileTool)(nil)
var _ tool.ArtifactProducingTool = (*TrashFileTool)(nil)

type trashFileParams struct {
	Path  string   `json:"path"`
	Paths []string `json:"paths"`
}

type trashFileResult struct {
	OriginalPath string `json:"original_path"`
	TrashedPath  string `json:"trashed_path,omitempty"`
	Method       string `json:"method"`
}

func (t *TrashFileTool) Name() string {
	return "trash_file"
}

func (t *TrashFileTool) Description() string {
	return "将本机文件或文件夹移入系统回收站/Trash；删除需求必须使用此工具，不要用 shell 的 rm/rmdir/unlink 永久删除"
}

func (t *TrashFileTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "path", Description: "要移入回收站的单个本机文件或文件夹路径", Type: "string", Required: false},
		{Name: "paths", Description: "要依次移入回收站的明确路径列表；用于安全转换 shell 删除", Type: "array", Required: false},
	}
}

func (t *TrashFileTool) InputSchema() tool.ToolParametersSchema {
	additional := tool.BoolPtr(false)
	item := tool.ToolParam{Type: "string", Description: "要移入回收站的明确路径", MinLength: intPtr(1)}
	minItems := 1
	return tool.ToolParametersSchema{
		Type:        "object",
		Description: "path 与 paths 必须且只能提供一个；单目标使用 path，批量目标使用 paths",
		Properties: map[string]tool.ToolParam{
			"path":  {Type: "string", Description: "单个明确路径；不得与 paths 同时提供", MinLength: intPtr(1)},
			"paths": {Type: "array", Description: "一个或多个明确路径；不得与 path 同时提供", Items: &item, MinItems: &minItems},
		},
		AdditionalProperties: additional,
		OneOf: []tool.ToolParam{
			{Type: "object", Properties: map[string]tool.ToolParam{"path": {Type: "string", Description: "单个明确路径", MinLength: intPtr(1)}}, Required: []string{"path"}, AdditionalProperties: additional},
			{Type: "object", Properties: map[string]tool.ToolParam{"paths": {Type: "array", Description: "一个或多个明确路径", Items: &item, MinItems: &minItems}}, Required: []string{"paths"}, AdditionalProperties: additional},
		},
	}
}

func intPtr(value int) *int { return &value }

func (t *TrashFileTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        t.Name(),
		DisplayName: "Trash File",
		Description: t.Description(),
		Purpose:     "把一个本机文件或目录移入操作系统回收站，并返回原路径和实际回收站路径。",
		WhenToUse: []string{
			"用户要求删除本机文件或目录时",
			"需要可恢复删除语义而不是永久移除时",
		},
		WhenNotToUse: []string{
			"不要通过 shell rm、rmdir 或 unlink 绕过回收站",
			"只想从 Awaken 项目列表移除项目但保留磁盘内容时使用项目生命周期能力",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"original_path": {Type: "string", Description: "删除前绝对路径"},
			"trashed_path":  {Type: "string", Description: "操作系统返回的回收站路径（若可得）"},
			"method":        {Type: "string", Description: "实际回收站实现"},
			"results":       {Type: "array", Description: "批量安全转换时每个已处理目标的结果", Items: &tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(true)}},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectDelete, Resources: []string{"one local file or directory"}, Description: "把资源移动到系统回收站；恢复能力取决于操作系统。", Reversibility: tool.ReversibilityConditional}},
		Examples:    []tool.ToolExample{{Scenario: "可恢复删除临时文件", Input: map[string]any{"path": "tmp/obsolete.txt"}, Outcome: "返回 original_path、trashed_path、method 和 trashed change"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeValidationError, Cause: "path 缺失或参数不是 JSON 对象", Retryable: false, Recovery: []string{"提供明确 path"}},
			{Code: tool.ErrorCodeUnsafeRecoverableDeletePath, Cause: "path 是敏感/受保护路径、符号链接、根目录或特殊文件", Retryable: false, Recovery: []string{"选择明确的普通文件或目录目标"}},
			{Code: tool.ErrorCodeToolExecutionFailed, Cause: "回收站移动失败且最终文件状态未知", Retryable: false, Recovery: []string{"先检查原路径和回收站状态", "不要原样重试或改用永久删除"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     true,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
		ProviderID:   tool.ProviderID("builtin:trash_file"),
		Capabilities: []tool.Capability{tool.CapabilityFileDelete},
	})
}

func (t *TrashFileTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseTrashFileParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	paths := trashFilePaths(params)
	for _, path := range paths {
		_, resolveErr := permissionContext.ResolveRecoverableDeletePathChecked(path)
		if resolveErr != nil {
			return tool.DenyPermission(resolveErr.Error())
		}
	}
	return tool.AllowPermission()
}

func (t *TrashFileTool) Execute(ctx context.Context, input string) (string, error) {
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

func (t *TrashFileTool) Artifacts(input string, _ string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	params, err := parseTrashFileParams(input)
	if err != nil {
		return nil
	}
	root := permissionContext.WorkspaceRootOrCWD()
	paths := trashFilePaths(params)
	artifacts := make([]tool.ArtifactDescriptor, 0, len(paths))
	for _, requested := range paths {
		resolved := permissionContext.ResolvePath(requested)
		path := filepath.ToSlash(relativeTrashArtifactPath(resolved, root))
		artifacts = append(artifacts, tool.ArtifactDescriptor{
			ContractVersion: tool.ArtifactContractVersion,
			Role:            tool.ArtifactRoleSupport,
			Producer:        t.Name(),
			Action:          tool.ArtifactActionUpdated,
			ID:              t.Name() + ":deleted:" + path,
			Title:           path,
			Description:     "Moved " + path + " to Trash",
			Kind:            "deleted",
			Path:            path,
			Status:          "ready",
		})
	}
	return artifacts
}

func relativeTrashArtifactPath(path string, root string) string {
	if strings.TrimSpace(root) == "" {
		return filepath.Clean(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Clean(path)
	}
	return rel
}

func (t *TrashFileTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeToolCancelled, tool.ErrorCategoryCancelled, false)
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	params, err := parseTrashFileParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.RecoveryOptions = []string{"provide_valid_path"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	requestedPaths := trashFilePaths(params)
	cleanedPaths := make([]string, 0, len(requestedPaths))
	for _, path := range requestedPaths {
		cleaned, resolveErr := permissionContext.ResolveRecoverableDeletePathChecked(path)
		if resolveErr != nil {
			outcome := tool.FailedOutcome(resolveErr.Error(), tool.ErrorCodeUnsafeRecoverableDeletePath, tool.ErrorCategoryPolicy, false)
			outcome.Status = tool.OutcomeDenied
			outcome.TransportStatus = tool.TransportNotStarted
			outcome.ProcessStatus = tool.ProcessNotApplicable
			outcome.RecoveryOptions = []string{"choose_explicit_non_sensitive_non_symlink_path"}
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
		cleanedPaths = append(cleanedPaths, cleaned)
	}

	results := make([]trashFileResult, 0, len(cleanedPaths))
	changes := make([]tool.ResourceChange, 0, len(cleanedPaths))
	for _, cleaned := range cleanedPaths {
		result, trashErr := trash.DefaultAdapter().TrashPath(cleaned)
		if trashErr != nil {
			outcome := tool.FailedOutcome(trashErr.Error(), tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryTool, false)
			state := tool.OutcomeSideEffectUnknown
			if len(results) > 0 {
				state = tool.OutcomeSideEffectPartial
			}
			outcome.Operation = &tool.OperationDescriptor{Type: t.Name(), Target: cleaned, SideEffectState: state}
			outcome.RecoveryOptions = []string{"inspect_original_and_trash_state", "do_not_retry_until_state_is_known"}
			outcome.Diagnostic = map[string]any{"original_path": cleaned, "final_state": "unknown", "completed_results": results}
			outcome.Changes = changes
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
		results = append(results, trashFileResult{OriginalPath: result.OriginalPath, TrashedPath: result.TrashedPath, Method: result.Method})
		changes = append(changes, tool.ResourceChange{
			Kind: "file", Action: "trashed", Resource: result.OriginalPath,
			Summary: "Moved to the operating-system Trash.", Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
			Details: map[string]any{"trashed_path": result.TrashedPath, "method": result.Method},
		})
	}
	typedResult := results[0]
	payload := any(typedResult)
	if len(results) > 1 {
		payload = map[string]any{"results": results}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return tool.ExecutionOutcome{}, err
	}
	outcome := tool.SucceededOutcome(string(data)).WithTiming(started, time.Now().UTC())
	outcome.Summary = fmt.Sprintf("Moved %d resource(s) to the operating-system Trash.", len(results))
	outcome.AffectedPaths = append([]string(nil), cleanedPaths...)
	outcome.Operation = &tool.OperationDescriptor{Type: t.Name(), Target: cleanedPaths[0], SideEffectState: tool.OutcomeSideEffectApplied}
	outcome.Details = map[string]any{"results": results}
	if len(results) == 1 {
		outcome.Details = map[string]any{"original_path": typedResult.OriginalPath, "trashed_path": typedResult.TrashedPath, "method": typedResult.Method}
	}
	outcome.Changes = changes
	return outcome, nil
}

func parseTrashFileParams(input string) (trashFileParams, error) {
	var params trashFileParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return trashFileParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	paths := trashFilePaths(params)
	if len(paths) == 0 {
		return trashFileParams{}, fmt.Errorf("exactly one of path or paths is required")
	}
	return params, nil
}

func trashFilePaths(params trashFileParams) []string {
	if strings.TrimSpace(params.Path) != "" && len(params.Paths) > 0 {
		return nil
	}
	values := append([]string(nil), params.Paths...)
	if strings.TrimSpace(params.Path) != "" {
		values = []string{params.Path}
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func init() {
	tool.MustRegisterToolWithMode(&TrashFileTool{}, tool.DefinitionValidationStrictBuiltin)
}
