package trashfile

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"awaken/src/tool"
	"awaken/src/tool/trash"
)

type TrashFileTool struct{}

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
	return tool.ToolDefinition{
		Name:        t.Name(),
		DisplayName: "Trash File",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     true,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
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
	if err := ctx.Err(); err != nil {
		return "", err
	}
	params, err := parseTrashFileParams(input)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(params.Path)
	if path == "" {
		return "", fmt.Errorf("trash_file requires path")
	}
	resolved := tool.PermissionContextFromContext(ctx).ResolvePath(path)
	cleaned, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	result, err := trash.DefaultAdapter().TrashPath(cleaned)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(trashFileResult{
		OriginalPath: result.OriginalPath,
		TrashedPath:  result.TrashedPath,
		Method:       result.Method,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
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
	tool.MustRegisterTool(&TrashFileTool{})
}
