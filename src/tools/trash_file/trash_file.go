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

type trashFileParams struct {
	Path string `json:"path"`
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
		{Name: "path", Description: "要移入回收站的本机文件或文件夹路径，可以是项目内或项目外路径", Type: "string", Required: true},
	}
}

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
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectDelete, Resources: []string{"one local file or directory"}, Description: "把资源移动到系统回收站；恢复能力取决于操作系统。", Reversibility: tool.ReversibilityConditional}},
		Examples:    []tool.ToolExample{{Scenario: "可恢复删除临时文件", Input: map[string]any{"path": "tmp/obsolete.txt"}, Outcome: "返回 original_path、trashed_path、method 和 trashed change"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: tool.ErrorCodeValidationError, Cause: "path 缺失或参数不是 JSON 对象", Retryable: false, Recovery: []string{"提供明确 path"}},
			{Code: tool.ErrorCodeToolExecutionFailed, Cause: "回收站移动失败且最终文件状态未知", Retryable: false, Recovery: []string{"先检查原路径和回收站状态", "不要原样重试或改用永久删除"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     true,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func (t *TrashFileTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseTrashFileParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if strings.TrimSpace(params.Path) == "" {
		return tool.DenyPermission("trash_file requires path")
	}
	resolved := permissionContext.ResolvePath(params.Path)
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "Move to Trash",
		Description:   resolved,
		RiskTone:      "danger",
		AffectedPaths: []string{resolved},
	})
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
	path := strings.TrimSpace(params.Path)
	if path == "" {
		outcome := tool.FailedOutcome("trash_file requires path", tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.RecoveryOptions = []string{"provide_path"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	resolved := tool.PermissionContextFromContext(ctx).ResolvePath(path)
	cleaned, err := filepath.Abs(resolved)
	if err != nil {
		outcome := tool.FailedOutcome(fmt.Sprintf("resolve path %q: %v", path, err), tool.ErrorCodeValidationError, tool.ErrorCategoryValidation, false)
		outcome.TransportStatus = tool.TransportNotStarted
		outcome.RecoveryOptions = []string{"provide_resolvable_path"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	result, err := trash.DefaultAdapter().TrashPath(cleaned)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), tool.ErrorCodeToolExecutionFailed, tool.ErrorCategoryTool, false)
		outcome.Operation = &tool.OperationDescriptor{Type: t.Name(), Target: cleaned, SideEffectState: tool.OutcomeSideEffectUnknown}
		outcome.RecoveryOptions = []string{"inspect_original_and_trash_state", "do_not_retry_until_state_is_known"}
		outcome.Diagnostic = map[string]any{"original_path": cleaned, "final_state": "unknown"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	typedResult := trashFileResult{
		OriginalPath: result.OriginalPath,
		TrashedPath:  result.TrashedPath,
		Method:       result.Method,
	}
	data, err := json.MarshalIndent(typedResult, "", "  ")
	if err != nil {
		return tool.ExecutionOutcome{}, err
	}
	outcome := tool.SucceededOutcome(string(data)).WithTiming(started, time.Now().UTC())
	outcome.Summary = "Moved resource to the operating-system Trash: " + result.OriginalPath
	outcome.AffectedPaths = []string{result.OriginalPath}
	outcome.Operation = &tool.OperationDescriptor{Type: t.Name(), Target: result.OriginalPath, SideEffectState: tool.OutcomeSideEffectApplied}
	outcome.Details = map[string]any{"original_path": result.OriginalPath, "trashed_path": result.TrashedPath, "method": result.Method}
	outcome.Changes = []tool.ResourceChange{{
		Kind: "file", Action: "trashed", Resource: result.OriginalPath,
		Summary: "Moved to the operating-system Trash.", Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
		Details: map[string]any{"trashed_path": result.TrashedPath, "method": result.Method},
	}}
	return outcome, nil
}

func parseTrashFileParams(input string) (trashFileParams, error) {
	var params trashFileParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return trashFileParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	if strings.TrimSpace(params.Path) == "" {
		return trashFileParams{}, fmt.Errorf("path is required")
	}
	return params, nil
}

func init() {
	tool.MustRegisterToolWithMode(&TrashFileTool{}, tool.DefinitionValidationStrictBuiltin)
}
