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
	return "按 glob 模式查找文件，返回稳定排序的文件路径列表"
}

func (t *ListFilesTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "pattern", Description: "文件匹配模式，例如 *.go 或 **/*.md，默认 **/*", Type: "string", Required: false},
		{Name: "root", Description: "搜索根目录，默认当前工作目录", Type: "string", Required: false},
		{Name: "max_results", Description: "最多返回多少条结果，默认 200，最大 1000", Type: "integer", Required: false},
	}
}

func (t *ListFilesTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}

	root := strings.TrimSpace(params.Root)
	if root == "" {
		root = "."
	}
	pattern := strings.TrimSpace(params.Pattern)
	if pattern == "" {
		pattern = defaultPattern
	}
	maxResults := normalizeMaxResults(params.MaxResults)

	results, total, err := listFiles(ctx, root, pattern, maxResults)
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

func listFiles(ctx context.Context, root, pattern string, maxResults int) ([]string, int, error) {
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

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if filematch.Match(pattern, rel) {
			matches = append(matches, rel)
		}
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

func init() {
	tool.MustRegisterTool(&ListFilesTool{})
}
