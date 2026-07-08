package listfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

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
	return tool.ToolDefinition{
		Name:        "list_files",
		DisplayName: "List Files",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        true,
			Destructive:     false,
			OpenWorld:       false,
			CapabilityClass: tool.CapabilityArtifactRead,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
		},
	}
}

func (t *ListFilesTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	root := permissionContext.ResolvePath(params.Root)
	if tool.HasPermissionContext(ctx) {
		checkedRoot, err := permissionContext.ResolveReadablePathChecked(params.Root)
		if err != nil {
			return "", err
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
		return "", err
	}
	if len(results) == 0 {
		return "未找到匹配文件", nil
	}
	if total > len(results) {
		results = append(results, fmt.Sprintf("[list_files truncated: showing %d of %d matches]", len(results), total))
	}
	return strings.Join(results, "\n"), nil
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
	tool.MustRegisterTool(&ListFilesTool{})
}
