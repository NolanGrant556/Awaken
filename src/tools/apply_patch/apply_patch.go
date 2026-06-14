package applypatch

import (
	"context"

	"awaken/src/tool"
)

type ApplyPatchTool struct{}

func (t *ApplyPatchTool) Name() string {
	return "apply_patch"
}

func (t *ApplyPatchTool) Description() string {
	return "使用 Codex 风格 patch 新增、修改、移动或删除文件"
}

func (t *ApplyPatchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "patch", Description: "Codex 风格 patch，支持 Add File、Update File、Move to、Delete File、End of File", Type: "string", Required: true},
	}
}

func (t *ApplyPatchTool) Execute(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	patch, err := parsePatch(input)
	if err != nil {
		return "", err
	}

	summary, err := applyPatch(ctx, patch)
	if err != nil {
		return "", err
	}
	return summary.String(), nil
}

func init() {
	tool.MustRegisterTool(&ApplyPatchTool{})
}
