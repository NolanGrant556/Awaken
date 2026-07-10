package textfilecreate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"awaken/src/tool"
)

const maxTextFileCreateBytes = 2 * 1024 * 1024

type TextFileCreateTool struct{}

type textFileCreateParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Append  bool   `json:"append"`
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
	}
}

func (t *TextFileCreateTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        "text_file_create",
		DisplayName: "Text File Create",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite,
		},
	}
}

func (t *TextFileCreateTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseTextFileCreateParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if strings.TrimSpace(params.Path) == "" {
		return tool.DenyPermission("text_file_create requires path")
	}
	if _, err := permissionContext.ResolveMutationPathChecked(params.Path); err != nil {
		return tool.DenyPermission(err.Error())
	}
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "Create text file",
		Description:   params.Path,
		RiskTone:      "warning",
		AffectedPaths: []string{params.Path},
	})
}

func (t *TextFileCreateTool) Execute(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	params, err := parseTextFileCreateParams(input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(params.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if !utf8.ValidString(params.Content) {
		return "", fmt.Errorf("content must be valid UTF-8 text")
	}
	if len([]byte(params.Content)) > maxTextFileCreateBytes {
		return "", fmt.Errorf("content is too large for text_file_create: %d bytes > %d bytes", len([]byte(params.Content)), maxTextFileCreateBytes)
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	outputPath, err := permissionContext.ResolveMutationPathChecked(params.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return "", err
	}

	action := "created"
	if params.Append {
		action = "appended"
		file, openErr := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr != nil {
			return "", openErr
		}
		if _, writeErr := file.WriteString(params.Content); writeErr != nil {
			_ = file.Close()
			return "", writeErr
		}
		if closeErr := file.Close(); closeErr != nil {
			return "", closeErr
		}
	} else {
		if err := os.WriteFile(outputPath, []byte(params.Content), 0o644); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%s %s (%d bytes)", action, outputPath, len([]byte(params.Content))), nil
}

func (t *TextFileCreateTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	params, err := parseTextFileCreateParams(input)
	if err != nil || strings.TrimSpace(params.Path) == "" {
		return nil
	}
	path := filepath.ToSlash(relativeTextFilePath(permissionContext.ResolvePath(params.Path), permissionContext.WorkspaceRootOrCWD()))
	return []tool.ArtifactDescriptor{{
		Action:      tool.ArtifactActionCreated,
		ID:          "text_file_create:" + path,
		Title:       path,
		Description: "Created " + path,
		Kind:        "text",
		Path:        path,
		Content:     params.Content,
		Status:      "ready",
	}}
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
	tool.MustRegisterTool(&TextFileCreateTool{})
}
