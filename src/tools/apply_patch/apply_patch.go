package applypatch

import (
	"context"
	"fmt"
	"strings"

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
	patch = resolvePatchPaths(patch, tool.PermissionContextFromContext(ctx))

	summary, err := applyPatch(ctx, patch)
	if err != nil {
		return "", err
	}
	return summary.String(), nil
}

func resolvePatchPaths(patch *patchDocument, permissionContext tool.PermissionContext) *patchDocument {
	if patch == nil {
		return nil
	}
	for i := range patch.operations {
		patch.operations[i].path = permissionContext.ResolvePath(patch.operations[i].path)
		if strings.TrimSpace(patch.operations[i].movePath) != "" {
			patch.operations[i].movePath = permissionContext.ResolvePath(patch.operations[i].movePath)
		}
	}
	return patch
}

func (t *ApplyPatchTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}

	patch, err := parsePatch(input)
	if err != nil {
		return tool.AllowPermission()
	}

	reason := "apply_patch will modify files"
	if patchHasDangerousChange(patch, permissionContext) {
		reason = "apply_patch will delete, move, or touch files outside writable roots"
	}
	return tool.AskPermission(reason, tool.ApprovalRequest{
		Title:       "Approve file changes",
		Description: patchApprovalDescription(patch),
		RiskTone:    patchRiskTone(patch, permissionContext),
	})
}

func patchApprovalDescription(patch *patchDocument) string {
	var builder strings.Builder
	builder.WriteString("apply_patch will change files:\n")
	for _, op := range patch.operations {
		switch op.kind {
		case patchKindAdd:
			fmt.Fprintf(&builder, "A %s\n", op.path)
		case patchKindDelete:
			fmt.Fprintf(&builder, "D %s\n", op.path)
		case patchKindUpdate:
			if strings.TrimSpace(op.movePath) != "" {
				fmt.Fprintf(&builder, "M %s -> %s\n", op.path, op.movePath)
			} else {
				fmt.Fprintf(&builder, "M %s\n", op.path)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func patchRiskTone(patch *patchDocument, permissionContext tool.PermissionContext) string {
	if patchHasDangerousChange(patch, permissionContext) {
		return "danger"
	}
	return "warning"
}

func patchHasDangerousChange(patch *patchDocument, permissionContext tool.PermissionContext) bool {
	for _, op := range patch.operations {
		if op.kind == patchKindDelete || strings.TrimSpace(op.movePath) != "" {
			return true
		}
		if !permissionContext.IsWithinWritableRoots(op.path) {
			return true
		}
		if strings.TrimSpace(op.movePath) != "" && !permissionContext.IsWithinWritableRoots(op.movePath) {
			return true
		}
	}
	return false
}

func init() {
	tool.MustRegisterTool(&ApplyPatchTool{})
}
