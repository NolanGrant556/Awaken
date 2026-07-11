package applypatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/tool"
)

type ApplyPatchTool struct{}

var _ tool.OutcomeTool = (*ApplyPatchTool)(nil)
var _ tool.LegacySingleValueInput = (*ApplyPatchTool)(nil)

func (t *ApplyPatchTool) UnwrapSingleParameterInput() bool { return true }

func (t *ApplyPatchTool) Name() string {
	return "apply_patch"
}

func (t *ApplyPatchTool) Description() string {
	return "使用 Awaken 结构化 patch 处理多文件、移动、删除或高级 diff；普通单文件精确修改请优先 text_edit，新建普通文本用 text_file_create，删除用 trash_file。输入必须以 *** Begin Patch 开始并以 *** End Patch 结束。删除会进入系统回收站。当前默认工具池可能不暴露本工具，除非内部 transform 或 grammar-capable provider"
}

func (t *ApplyPatchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "patch", Description: "严格 Awaken 结构化 patch 文本：第一行 *** Begin Patch，最后一行 *** End Patch；支持 Add File、Update File、Move to、Delete File；End of File 仅在确有需要时使用，普通截断编辑不要默认加", Type: "string", Required: true},
	}
}

func (t *ApplyPatchTool) Execute(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	patch, err := parsePatch(input)
	if err != nil {
		code := classifyLegacyPatchError(err)
		return "", fmt.Errorf("%s: %w。apply_patch 只接受 Awaken 结构化 patch；普通单文件修改请改用 text_edit，Markdown/普通文本请改用 text_file_create，生成 docx/xlsx/pptx 请改用 document_create、spreadsheet_create 或 presentation_create", code, err)
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
		code := classifyLegacyPatchError(err)
		if code != ErrorCodeInvalidPatchFormat || IsPatchContextMismatch(err) {
			return "", fmt.Errorf("%s: %w", code, err)
		}
		return "", err
	}
	return summary.String(), nil
}

// ExecuteOutcome keeps apply_patch grammar, context, and path failures as
// typed business outcomes. This is the path consumed by ToolRuntime and
// FailureSupervisor; Go errors are reserved for executor/transport failures.
func (t *ApplyPatchTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		finished := time.Now().UTC()
		outcome := tool.ExecutionOutcome{
			Content:         err.Error(),
			Status:          tool.OutcomeCancelled,
			TransportStatus: tool.TransportCancelled,
			SemanticStatus:  tool.SemanticFailure,
			ProcessStatus:   tool.ProcessNotApplicable,
			ErrorCode:       tool.ErrorCodeToolCancelled,
			ErrorCategory:   tool.ErrorCategoryCancelled,
			ErrorMessage:    err.Error(),
			Retryable:       false,
		}
		return outcome.WithTiming(started, finished), nil
	}

	patch, err := parsePatch(input)
	if err != nil {
		code := classifyLegacyPatchError(err)
		message := fmt.Sprintf("%s: %v。apply_patch 只接受 Awaken 结构化 patch；普通单文件修改请改用 text_edit，Markdown/普通文本请改用 text_file_create，生成 docx/xlsx/pptx 请改用 document_create、spreadsheet_create 或 presentation_create", code, err)
		return patchFailureOutcome(started, message, code, tool.ErrorCategoryValidation, false,
			[]string{"use_text_edit_or_text_file_create", "re_read_file", "do_not_replay_same_patch"}), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	affectedPaths := patchAffectedPaths(patch, permissionContext)
	patch = resolvePatchPaths(patch, permissionContext)
	if tool.HasPermissionContext(ctx) {
		for _, op := range patch.operations {
			if err := validateMutationPath(op.path, permissionContext); err != nil {
				return patchFailureOutcome(started, err.Error(), ErrorCodePatchPathNotWritable, tool.ErrorCategoryPolicy, false,
					[]string{"choose_path_within_writable_roots", "do_not_bypass_with_shell"}), nil
			}
			if strings.TrimSpace(op.movePath) != "" {
				if err := validateMutationPath(op.movePath, permissionContext); err != nil {
					return patchFailureOutcome(started, err.Error(), ErrorCodePatchPathNotWritable, tool.ErrorCategoryPolicy, false,
						[]string{"choose_path_within_writable_roots", "do_not_bypass_with_shell"}), nil
				}
			}
		}
	}
	summary, err := applyPatch(ctx, patch)
	if err != nil {
		if IsPatchContextMismatch(err) {
			code := classifyLegacyPatchError(err)
			return patchFailureOutcomeWithProgress(started, err.Error(), code, tool.ErrorCategoryValidation,
				[]string{"read_file", "use_text_edit", "do_not_replay_same_patch"}, patch, summary, permissionContext, affectedPaths), nil
		}
		return patchFailureOutcomeWithProgress(started, err.Error(), ErrorCodeApplyPatchFailed, tool.ErrorCategoryTool,
			[]string{"inspect_error", "verify_file_state_before_retry"}, patch, summary, permissionContext, affectedPaths), nil
	}

	finished := time.Now().UTC()
	outcome := tool.SucceededOutcome(summary.String())
	outcome.AffectedPaths = affectedPaths
	outcome.Changes = patchResourceChanges(patch, summary, permissionContext)
	outcome.PatchSummary = summarizePatchResourceChanges(outcome.Changes)
	outcome.Warnings = patchOverwriteNotices(outcome.Changes)
	return outcome.WithTiming(started, finished), nil
}

func patchFailureOutcomeWithProgress(
	started time.Time,
	message string,
	code string,
	category tool.ErrorCategory,
	recovery []string,
	patch *patchDocument,
	summary patchSummary,
	permissionContext tool.PermissionContext,
	plannedPaths []string,
) tool.ExecutionOutcome {
	outcome := patchFailureOutcome(started, message, code, category, false, recovery)
	outcome.AffectedPaths = append([]string(nil), plannedPaths...)
	completedCount := len(summary.added) + len(summary.overwritten) + len(summary.modified) + len(summary.deleted)
	plannedCount := 0
	if patch != nil {
		plannedCount = len(patch.operations)
	}
	if completedCount > plannedCount {
		completedCount = plannedCount
	}
	completedPatch := &patchDocument{}
	if patch != nil && completedCount > 0 {
		completedPatch.operations = append([]patchOperation(nil), patch.operations[:completedCount]...)
		outcome.Changes = patchResourceChanges(completedPatch, summary, permissionContext)
		outcome.PatchSummary = summarizePatchResourceChanges(outcome.Changes)
	}
	outcome.Changes = append(outcome.Changes, patchPartialResourceChanges(summary.partial, permissionContext)...)
	if len(outcome.Changes) > 0 {
		outcome.PatchSummary = summarizePatchResourceChanges(outcome.Changes) + "；后续 patch 操作失败，存在已观察到的部分写入"
		outcome.Operation = &tool.OperationDescriptor{Type: "apply_patch", SideEffectState: tool.OutcomeSideEffectPartial}
		outcome.Warnings = []tool.OutcomeNotice{{
			Code: "patch_partially_applied", Message: "Earlier patch operations were applied before a later operation failed; verify current files before recovery.",
		}}
		outcome.NextActions = []tool.NextAction{{
			ID: "verify_partial_patch_state", Description: "Read the affected files and verify which patch operations were applied before planning recovery.",
			Tool: "read_file", Preconditions: []string{"Do not replay the original multi-operation patch unchanged."},
		}}
	} else {
		outcome.Operation = &tool.OperationDescriptor{Type: "apply_patch", SideEffectState: tool.OutcomeSideEffectUnknown}
	}
	outcome.Diagnostic = map[string]any{
		"planned_operation_count":   plannedCount,
		"completed_operation_count": completedCount,
		"partial_write_count":       len(summary.partial),
	}
	outcome.Errors = []tool.OutcomeError{{
		Type: code, Category: category, Stage: tool.ErrorStageExecution, Cause: message,
		PossibleFixes: append([]string(nil), outcome.RecoveryOptions...), RetryStrategy: tool.RetryDoNotRetry,
	}}
	return outcome
}

func patchPartialResourceChanges(entries []partialWriteEntry, permissionContext tool.PermissionContext) []tool.ResourceChange {
	if len(entries) == 0 {
		return nil
	}
	root := permissionContext.WorkspaceRootOrCWD()
	changes := make([]tool.ResourceChange, 0, len(entries))
	for _, entry := range entries {
		source := normalizeArtifactPath(entry.sourcePath, permissionContext, root)
		destination := normalizeArtifactPath(entry.destinationPath, permissionContext, root)
		changes = append(changes, tool.ResourceChange{
			Kind: "file", Action: firstNonEmpty(strings.TrimSpace(entry.action), "written"), Resource: destination,
			Summary:   "Move destination was written before source cleanup failed.",
			Certainty: tool.ChangeObserved, Reversibility: tool.ChangeConditional,
			Details: map[string]any{
				"source_path": source, "source_state": "unknown", "destination_path": destination,
				"failure_cause": entry.cause,
			},
		})
	}
	return changes
}

func patchResourceChanges(patch *patchDocument, summary patchSummary, permissionContext tool.PermissionContext) []tool.ResourceChange {
	if patch == nil {
		return nil
	}
	root := permissionContext.WorkspaceRootOrCWD()
	trashedBySource := make(map[string]string, len(summary.deleted))
	for _, entry := range summary.deleted {
		trashedBySource[filepath.Clean(entry.path)] = strings.TrimSpace(entry.trashedPath)
	}
	changes := make([]tool.ResourceChange, 0, len(patch.operations))
	for _, op := range patch.operations {
		source := normalizeArtifactPath(op.path, permissionContext, root)
		change := tool.ResourceChange{
			Kind: "file", Resource: source, Certainty: tool.ChangeObserved,
			Reversibility: tool.ChangeConditional,
		}
		switch op.kind {
		case patchKindAdd:
			if summaryContainsPath(summary.overwritten, op.path) {
				change.Action = "overwritten"
				change.Summary = "Add File replaced existing target " + source
				change.Details = map[string]any{"previous_state": "replaced"}
			} else {
				change.Action = "created"
				change.Summary = "Created " + source
			}
		case patchKindDelete:
			change.Action = "trashed"
			change.Summary = "Moved resource to the operating-system Trash."
			change.Details = map[string]any{"original_path": source}
			if trashedPath := trashedBySource[filepath.Clean(op.path)]; trashedPath != "" {
				change.Details["trashed_path"] = trashedPath
			}
		case patchKindUpdate:
			if strings.TrimSpace(op.movePath) == "" {
				change.Action = "updated"
				change.Summary = "Updated " + source
				break
			}
			destination := normalizeArtifactPath(op.movePath, permissionContext, root)
			change.Action = "moved"
			change.Summary = "Moved " + source + " to " + destination
			change.Details = map[string]any{"source_path": source, "destination_path": destination}
			if summaryContainsPath(summary.replacedDestinations, op.movePath) {
				change.Details["destination_replaced"] = true
				change.Summary += " and replaced the existing destination"
			}
		default:
			continue
		}
		changes = append(changes, change)
	}
	return changes
}

func summarizePatchResourceChanges(changes []tool.ResourceChange) string {
	counts := make(map[string]int)
	for _, change := range changes {
		counts[strings.TrimSpace(change.Action)]++
	}
	parts := make([]string, 0, 5)
	for _, item := range []struct {
		action string
		label  string
	}{{"created", "新增"}, {"overwritten", "覆盖"}, {"updated", "修改"}, {"moved", "移动"}, {"trashed", "回收站删除"}, {"written", "部分写入"}} {
		if count := counts[item.action]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d 个文件", item.label, count))
		}
	}
	if len(parts) == 0 {
		return "无已观察变更"
	}
	return strings.Join(parts, "，")
}

func patchOverwriteNotices(changes []tool.ResourceChange) []tool.OutcomeNotice {
	var notices []tool.OutcomeNotice
	for _, change := range changes {
		switch {
		case change.Action == "overwritten":
			notices = append(notices, tool.OutcomeNotice{
				Code: "patch_add_replaced_existing", Message: "Add File used compatibility overwrite semantics and replaced an existing target.",
			})
		case change.Action == "moved" && change.Details["destination_replaced"] == true:
			notices = append(notices, tool.OutcomeNotice{
				Code: "patch_move_replaced_destination", Message: "Move compatibility semantics replaced an existing destination before trashing the source.",
			})
		}
	}
	return notices
}

func patchFailureOutcome(started time.Time, message, code string, category tool.ErrorCategory, retryable bool, recovery []string) tool.ExecutionOutcome {
	finished := time.Now().UTC()
	outcome := tool.FailedOutcome(message, code, category, retryable)
	outcome.RecoveryOptions = append([]string(nil), recovery...)
	return outcome.WithTiming(started, finished)
}

func (t *ApplyPatchTool) Artifacts(input string, output string, permissionContext tool.PermissionContext) []tool.ArtifactDescriptor {
	patch, err := parsePatch(input)
	if err != nil {
		return nil
	}
	root := permissionContext.WorkspaceRootOrCWD()
	artifacts := make([]tool.ArtifactDescriptor, 0, len(patch.operations))
	for _, op := range patch.operations {
		var kind string
		var action tool.ArtifactAction
		// Normalize source and (for move) destination paths so description,
		// title, id, and path all share the same workspace-relative口径.
		sourcePath := normalizeArtifactPath(op.path, permissionContext, root)
		switch op.kind {
		case patchKindAdd:
			resolvedPath := permissionContext.ResolvePath(op.path)
			if patchOutputContainsPathMarker(output, "O", resolvedPath) {
				kind = "overwritten"
				action = tool.ArtifactActionUpdated
			} else {
				kind = "added"
				action = tool.ArtifactActionCreated
			}
		case patchKindDelete:
			kind = "deleted"
			action = tool.ArtifactActionUpdated
		case patchKindUpdate:
			if strings.TrimSpace(op.movePath) != "" {
				kind = "moved"
				action = tool.ArtifactActionUpdated
			} else {
				kind = "modified"
				action = tool.ArtifactActionUpdated
			}
		default:
			continue
		}
		// summaryPath() returns movePath for move operations, so the
		// destination path is used as the artifact path.
		path := normalizeArtifactPath(op.summaryPath(), permissionContext, root)
		var description string
		switch kind {
		case "added":
			description = "Added " + path
		case "overwritten":
			description = "Overwrote existing target " + path
		case "deleted":
			description = "Deleted " + path
		case "modified":
			description = "Modified " + path
		case "moved":
			description = "Moved " + sourcePath + " to " + path
			if patchOutputLineContains(output, "M", permissionContext.ResolvePath(op.movePath), "replaced existing destination") {
				description += " and replaced the existing destination"
			}
		}
		artifacts = append(artifacts, tool.ArtifactDescriptor{
			ContractVersion: tool.ArtifactContractVersion,
			Role:            tool.ArtifactRoleSupport,
			Producer:        "apply_patch",
			Action:          action,
			ID:              fmt.Sprintf("apply_patch:%s:%s", kind, path),
			Title:           path,
			Description:     description,
			Kind:            kind,
			Path:            path,
			Status:          "ready",
		})
	}
	return artifacts
}

func patchOutputContainsPathMarker(output string, marker string, path string) bool {
	return patchOutputLineContains(output, marker, path, "")
}

func patchOutputLineContains(output string, marker string, path string, fragment string) bool {
	prefix := strings.TrimSpace(marker) + " " + strings.TrimSpace(path)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) && (fragment == "" || strings.Contains(trimmed, fragment)) {
			return true
		}
	}
	return false
}

func (t *ApplyPatchTool) PatchPreview(input string, permissionContext tool.PermissionContext) (tool.PatchPreview, bool) {
	patch, err := parsePatch(input)
	if err != nil {
		return tool.PatchPreview{}, false
	}
	return tool.PatchPreview{
		Patch:         strings.TrimSpace(input),
		Summary:       buildPatchSummary(patch),
		AffectedPaths: patchAffectedPaths(patch, permissionContext),
	}, true
}

// normalizeArtifactPath resolves the raw patch path against the permission
// context and converts it to a workspace-relative path with forward slashes,
// matching the口径 of patchAffectedPaths so artifact paths stay consistent
// with approval affected paths.
func normalizeArtifactPath(rawPath string, permissionContext tool.PermissionContext, root string) string {
	resolved := permissionContext.ResolvePath(rawPath)
	rel := relativeToRoot(resolved, root)
	return filepath.ToSlash(rel)
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

func (t *ApplyPatchTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "apply_patch",
		DisplayName: "Apply Patch",
		Description: t.Description(),
		Purpose:     "以 Awaken patch grammar 原子表达多文件新增、更新、移动和回收站删除。",
		WhenToUse: []string{
			"一次操作需要多文件变化、文件移动或高级上下文 diff 时",
			"当前 effective tool pool 明确暴露 apply_patch 且输入能遵循 patch grammar 时",
		},
		WhenNotToUse: []string{
			"单文件精确字符串替换使用 text_edit；普通新建文本使用 text_file_create",
			"只删除一个文件或目录使用 trash_file",
			"provider 当前未暴露 grammar/compat apply_patch 时不得用 shell 模拟",
		},
		OutputSchema: tool.NoDetailsOutputSchema(),
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectCreate, Resources: []string{"patch add targets"}, Description: "创建 patch 中声明的新文件。", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectUpdate, Resources: []string{"patch update/move targets"}, Description: "更新或移动 patch 中声明的文件。", Reversibility: tool.ReversibilityConditional},
			{Kind: tool.SideEffectDelete, Resources: []string{"patch delete targets"}, Description: "删除操作移动到系统回收站，不执行永久 rm。", Reversibility: tool.ReversibilityConditional},
		},
		Examples: []tool.ToolExample{{Scenario: "多文件结构化变更", Input: map[string]any{"patch": "*** Begin Patch\n*** Update File: a.md\n@@\n-old\n+new\n*** End Patch"}, Outcome: "返回实际 affected paths、patch summary 和回收站事实"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: ErrorCodeInvalidPatchFormat, Cause: "输入不符合 Awaken patch grammar", Retryable: false, Recovery: []string{"改用正确 patch grammar", "简单编辑改用 text_edit"}},
			{Code: ErrorCodePatchContextMismatch, Cause: "patch 上下文与当前文件不一致", Retryable: false, Recovery: []string{"重新 read_file", "基于当前内容重建 patch"}},
			{Code: ErrorCodePatchPathNotWritable, Cause: "目标不在任务可写范围", Retryable: false, Recovery: []string{"选择 writable-root 内路径"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     true,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
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
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "File changes",
		Description:   firstNonEmpty(patchApprovalDescription(patch), reason),
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	rel := filepath.ToSlash(relativeToRoot(path, root))
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
//
// Sensitive paths (e.g. .git/, .env, key/certificate files, agent metadata
// directories, runtime config directories) are denied first, regardless of
// whether they are inside the workspace or writable roots. Reads are not
// affected because this function is only invoked for mutation operations.
func validateMutationPath(path string, permissionContext tool.PermissionContext) error {
	path = permissionContext.ResolvePath(path)
	path = filepath.Clean(path)
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	// Sensitive path guard. Applies regardless of workspace membership: a
	// .git/ inside the workspace is still sensitive.
	if err := permissionContext.CheckSensitivePath(path); err != nil {
		return err
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
	tool.MustRegisterToolWithMode(&ApplyPatchTool{}, tool.DefinitionValidationStrictBuiltin)
}
