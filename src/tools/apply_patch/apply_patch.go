package applypatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	permissionContext := tool.PermissionContextFromContext(ctx)
	patch = resolvePatchPaths(patch, permissionContext)

	if tool.HasPermissionContext(ctx) {
		for _, op := range patch.operations {
			if err := validateMutationPath(op.path, permissionContext); err != nil {
				return "", err
			}
			if strings.TrimSpace(op.movePath) != "" {
				if err := validateMutationPath(op.movePath, permissionContext); err != nil {
					return "", err
				}
			}
		}
	}

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
		Title:         "Approve file changes",
		Description:   patchApprovalDescription(patch),
		RiskTone:      patchRiskTone(patch, permissionContext),
		AffectedPaths: patchAffectedPaths(patch, permissionContext),
		PatchSummary:  buildPatchSummary(patch),
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
		// A path that is lexically outside writable roots or resolves via
		// symlink to a location outside writable roots is dangerous even if it
		// appears to be inside the workspace textually.
		if validateMutationPath(op.path, permissionContext) != nil {
			return true
		}
		if strings.TrimSpace(op.movePath) != "" && validateMutationPath(op.movePath, permissionContext) != nil {
			return true
		}
	}
	return false
}

func patchAffectedPaths(patch *patchDocument, permissionContext tool.PermissionContext) []string {
	root := permissionContext.WorkspaceRootOrCWD()
	seen := make(map[string]struct{})
	paths := make([]string, 0, len(patch.operations)*2)
	for _, op := range patch.operations {
		addPath(seen, &paths, permissionContext.ResolvePath(op.path), root)
		if strings.TrimSpace(op.movePath) != "" {
			addPath(seen, &paths, permissionContext.ResolvePath(op.movePath), root)
		}
	}
	return paths
}

func addPath(seen map[string]struct{}, paths *[]string, path, root string) {
	rel := relativeToRoot(path, root)
	if _, ok := seen[rel]; ok {
		return
	}
	seen[rel] = struct{}{}
	*paths = append(*paths, rel)
}

func relativeToRoot(path, root string) string {
	absPath, err1 := filepath.Abs(path)
	absRoot, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil || absRoot == "" {
		return path
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return absPath
	}
	return rel
}

// validateMutationPath performs a symlink-resolved writable-root check before
// any filesystem mutation. It walks up to the deepest existing ancestor,
// resolves symlinks via filepath.EvalSymlinks, and ensures the real target is
// within the configured writable roots. This blocks symlink escapes such as a
// workspace link pointing outside the workspace.
func validateMutationPath(path string, permissionContext tool.PermissionContext) error {
	path = permissionContext.ResolvePath(path)
	path = filepath.Clean(path)
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	// Lexical guard first.
	if !permissionContext.IsWithinWritableRoots(path) {
		return fmt.Errorf("path %q is outside writable roots", path)
	}

	current := path
	for {
		info, err := os.Lstat(current)
		if err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return fmt.Errorf("cannot resolve path %q: %w", current, resolveErr)
			}
			if !permissionContext.IsWithinWritableRootsResolved(resolved) {
				return fmt.Errorf("path %q resolves to %q which is outside writable roots", path, resolved)
			}
			// If this ancestor exists but is not the target path itself, it must
			// be a directory for descendants to be created safely.
			if current != path && !info.IsDir() {
				return fmt.Errorf("path %q cannot be created under non-directory %q", path, current)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot inspect path %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("path %q has no existing parent within writable roots", path)
		}
		current = parent
	}
}

func buildPatchSummary(patch *patchDocument) string {
	addCount, updateCount, deleteCount, moveCount := 0, 0, 0, 0
	for _, op := range patch.operations {
		switch op.kind {
		case patchKindAdd:
			addCount++
		case patchKindDelete:
			deleteCount++
		case patchKindUpdate:
			if strings.TrimSpace(op.movePath) != "" {
				moveCount++
			} else {
				updateCount++
			}
		}
	}

	parts := make([]string, 0, 4)
	if addCount > 0 {
		parts = append(parts, fmt.Sprintf("新增 %d 个文件", addCount))
	}
	if updateCount > 0 {
		parts = append(parts, fmt.Sprintf("修改 %d 个文件", updateCount))
	}
	if deleteCount > 0 {
		parts = append(parts, fmt.Sprintf("删除 %d 个文件", deleteCount))
	}
	if moveCount > 0 {
		parts = append(parts, fmt.Sprintf("移动 %d 个文件", moveCount))
	}
	if len(parts) == 0 {
		return "无变更"
	}
	return strings.Join(parts, "，")
}

func init() {
	tool.MustRegisterTool(&ApplyPatchTool{})
}
