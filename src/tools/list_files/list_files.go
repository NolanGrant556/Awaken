package listfiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/filematch"
	"awaken/src/tools/internal/filesafe"
)

const (
	defaultPattern    = "**/*"
	defaultMaxResults = 200
	maxResultsLimit   = 1000
)

type ListFilesTool struct{}

type listFilesParams struct {
	Pattern    string `json:"pattern"`
	Root       string `json:"root"`
	MaxResults int    `json:"max_results"`
}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return "在本机目录内按 glob 列出文件，返回稳定排序路径；root 可用绝对路径、相对当前工作区路径或已上传附件目录名。扫描大目录时请设置 pattern/max_results，本地 runtime/命令依赖优先用 workspace_dependencies"
}

func (t *ListFilesTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "pattern", Description: "文件匹配模式，例如 *.go 或 **/*.md，默认 **/*", Type: "string", Required: false},
		{Name: "root", Description: "搜索根目录，可用绝对路径、相对当前工作区路径或已上传附件目录名；默认当前工作区", Type: "string", Required: false},
		{Name: "max_results", Description: "最多返回多少条结果，默认 200，最大 1000", Type: "integer", Required: false},
	}
}

func (t *ListFilesTool) IsReadOnly() bool {
	return true
}

func (t *ListFilesTool) Definition() tool.ToolDefinition {
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "list_files",
		DisplayName: "List Files",
		Description: t.Description(),
		Purpose:     "按 glob 枚举目录中的文件路径，不读取文件内容。",
		WhenToUse: []string{
			"需要发现文件、目录结构或匹配扩展名的路径集合时",
			"后续读取或搜索前需要确定候选文件时",
		},
		WhenNotToUse: []string{
			"需要匹配文件内容或函数定义时使用 grep_search",
			"已知精确文件路径并需要内容时直接使用 read_file 或 read_artifact",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"root":          {Type: "string", Description: "实际搜索根目录"},
			"pattern":       {Type: "string", Description: "实际 glob 模式"},
			"match_count":   {Type: "integer", Description: "本次返回的路径数量"},
			"total_matches": {Type: "integer", Description: "观察到的总匹配数"},
			"max_results":   {Type: "integer", Description: "本次结果上限"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"directory entries"}, Description: "遍历目录项，不读取内容或修改文件。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "列出所有 Go 文件", Input: map[string]any{"root": ".", "pattern": "**/*.go", "max_results": 200}, Outcome: "返回稳定排序的匹配路径及截断事实"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "list_files_invalid_input", Cause: "glob、root 或 max_results 参数无效", Retryable: false, Recovery: []string{"修正枚举参数"}},
			{Code: "list_files_root_unavailable", Cause: "目录不存在或不在任务可读范围", Retryable: false, Recovery: []string{"检查 root 和任务可读范围"}},
			{Code: "list_files_scan_failed", Cause: "目录遍历在执行阶段失败", Retryable: true, Recovery: []string{"缩小范围并检查文件系统状态"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	})
}

func (t *ListFilesTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome exposes stable result counts and truncation state while
// keeping directory scans read-only. Expected input, path, and filesystem
// failures are business outcomes; a Go error is reserved for implementation
// or transport failures outside this tool's domain.
func (t *ListFilesTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "list_files_invalid_input", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_json_parameters"}
		outcome.Diagnostic = map[string]any{"expected_input": "JSON object"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	root := permissionContext.ResolvePath(params.Root)
	if tool.HasPermissionContext(ctx) {
		checkedRoot, err := permissionContext.ResolveReadablePathChecked(params.Root)
		if err != nil {
			outcome := tool.FailedOutcome(err.Error(), "list_files_root_unavailable", tool.ErrorCategoryDomain, false)
			outcome.RecoveryOptions = []string{"choose_readable_root", "list_workspace_root"}
			outcome.Diagnostic = map[string]any{"requested_root": params.Root}
			return outcome.WithTiming(started, time.Now().UTC()), nil
		}
		root = checkedRoot
	}
	workspaceRoot := permissionContext.WorkspaceRootOrCWD()
	pattern := strings.TrimSpace(params.Pattern)
	if pattern == "" {
		pattern = defaultPattern
	}
	maxResults := normalizeMaxResults(params.MaxResults)

	results, total, err := listFiles(ctx, root, workspaceRoot, pattern, maxResults, permissionContext)
	if err != nil {
		outcome := listFilesFailureOutcome(err, root, pattern)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	truncated := total > len(results)
	resultCount := len(results)
	if len(results) == 0 {
		outcome := tool.SucceededOutcome("未找到匹配文件")
		outcome.Diagnostic = map[string]any{
			"root": root, "pattern": pattern, "result_count": 0, "match_count": 0,
			"total_matches": 0, "max_results": maxResults, "truncated": false,
		}
		outcome.Details = listFilesDetails(root, pattern, 0, 0, maxResults)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if truncated {
		results = append(results, fmt.Sprintf("[list_files truncated: showing %d of %d matches]", len(results), total))
	}
	outcome := tool.SucceededOutcome(strings.Join(results, "\n"))
	outcome.Truncated = truncated
	outcome.Diagnostic = map[string]any{
		"root": root, "pattern": pattern, "result_count": resultCount, "match_count": resultCount,
		"total_matches": total, "max_results": maxResults, "truncated": truncated,
	}
	outcome.Details = listFilesDetails(root, pattern, resultCount, total, maxResults)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func listFilesDetails(root, pattern string, matchCount, totalMatches, maxResults int) map[string]any {
	return map[string]any{
		"root": root, "pattern": pattern, "match_count": matchCount,
		"total_matches": totalMatches, "max_results": maxResults,
	}
}

func listFilesFailureOutcome(err error, root, pattern string) tool.ExecutionOutcome {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		outcome := tool.FailedOutcome(err.Error(), "list_files_cancelled", tool.ErrorCategoryCancelled, true)
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.RecoveryOptions = []string{"retry_when_context_available"}
		outcome.Diagnostic = map[string]any{"root": root, "pattern": pattern}
		return outcome
	}
	outcome := tool.FailedOutcome(err.Error(), "list_files_scan_failed", tool.ErrorCategoryRuntime, true)
	outcome.RecoveryOptions = []string{"inspect_root_access", "choose_another_root", "retry_list_files"}
	outcome.Diagnostic = map[string]any{"root": root, "pattern": pattern}
	return outcome
}

func (t *ListFilesTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if _, err := parseParams(input); err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	return tool.AllowPermission()
}

func parseParams(input string) (listFilesParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return listFilesParams{}, nil
	}

	var params listFilesParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return listFilesParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	return params, nil
}

func normalizeMaxResults(value int) int {
	if value <= 0 {
		return defaultMaxResults
	}
	if value > maxResultsLimit {
		return maxResultsLimit
	}
	return value
}

func listFiles(ctx context.Context, root, workspaceRoot, pattern string, maxResults int, permissionContext tool.PermissionContext) ([]string, int, error) {
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			if path != root && filesafe.ShouldSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}

		relRoot, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relRoot = filepath.ToSlash(relRoot)
		if !filematch.Match(pattern, relRoot) {
			return nil
		}
		matches = append(matches, displayPathForListedFile(path, root, workspaceRoot))
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("查找文件失败: %w", err)
	}

	sort.Strings(matches)
	total := len(matches)
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	return matches, total, nil
}

func displayPathForListedFile(path string, root string, workspaceRoot string) string {
	if rel, ok := relativePathWithin(workspaceRoot, path); ok {
		return filepath.ToSlash(rel)
	}
	if rel, err := filepath.Rel(root, path); err == nil {
		if rel == "." {
			return filepath.Base(path)
		}
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func relativePathWithin(root string, path string) (string, bool) {
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	if rel == "." {
		return filepath.Base(path), true
	}
	return rel, true
}

func init() {
	tool.MustRegisterToolWithMode(&ListFilesTool{}, tool.DefinitionValidationStrictBuiltin)
}
