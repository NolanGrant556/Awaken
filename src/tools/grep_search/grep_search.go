package grepsearch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"awaken/src/tool"
	"awaken/src/tools/internal/filematch"
	"awaken/src/tools/internal/filesafe"
)

const (
	defaultInclude    = "**/*"
	defaultMaxMatches = 100
	maxMatchesLimit   = 1000
	defaultLineChars  = 500
	maxLineCharsLimit = 4000
)

type GrepSearchTool struct{}

type grepSearchParams struct {
	Query        string `json:"query"`
	Root         string `json:"root"`
	Include      string `json:"include"`
	MaxResults   int    `json:"max_results"`
	Offset       int    `json:"offset"`
	MaxLineChars int    `json:"max_line_chars"`
	OutputMode   string `json:"output_mode"`
}

type grepOptions struct {
	root          string
	workspaceRoot string
	include       string
	maxResults    int
	offset        int
	maxLineChars  int
	outputMode    string
}

type fileMatch struct {
	path  string
	rel   string
	count int
	lines []lineMatch
}

type lineMatch struct {
	number  int
	content string
}

func (t *GrepSearchTool) Name() string {
	return "grep_search"
}

func (t *GrepSearchTool) Description() string {
	return "使用正则表达式搜索文件内容，返回匹配文件、行号和匹配行"
}

func (t *GrepSearchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "query", Description: "要搜索的正则表达式", Type: "string", Required: true},
		{Name: "root", Description: "搜索根目录，默认当前工作目录", Type: "string", Required: false},
		{Name: "include", Description: "文件匹配模式，例如 **/*.go，默认 **/*", Type: "string", Required: false},
		{Name: "max_results", Description: "最多返回多少条匹配，默认 100，最大 1000", Type: "integer", Required: false},
		{Name: "offset", Description: "跳过前多少条结果，默认 0", Type: "integer", Required: false},
		{Name: "max_line_chars", Description: "每条匹配行最多返回多少字符，默认 500，最大 4000", Type: "integer", Required: false},
		{Name: "output_mode", Description: "输出模式: content、files_with_matches 或 count，默认 content", Type: "string", Required: false},
	}
}

func (t *GrepSearchTool) IsReadOnly() bool {
	return true
}

func (t *GrepSearchTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        "grep_search",
		DisplayName: "Grep Search",
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

func (t *GrepSearchTool) Execute(ctx context.Context, input string) (string, error) {
	params, err := parseParams(input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(params.Query) == "" {
		return "", fmt.Errorf("缺少必需参数: query")
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
	include := strings.TrimSpace(params.Include)
	if include == "" {
		include = defaultInclude
	}
	options := grepOptions{
		root:          root,
		workspaceRoot: permissionContext.WorkspaceRootOrCWD(),
		include:       include,
		maxResults:    normalizeMaxResults(params.MaxResults),
		offset:        normalizeOffset(params.Offset),
		maxLineChars:  normalizeMaxLineChars(params.MaxLineChars),
		outputMode:    normalizeOutputMode(params.OutputMode),
	}

	re, err := regexp.Compile(params.Query)
	if err != nil {
		return "", fmt.Errorf("query 不是有效正则表达式: %w", err)
	}

	results, truncated, err := searchFiles(ctx, re, options, permissionContext)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "未找到匹配内容", nil
	}
	if truncated {
		results = append(results, fmt.Sprintf("[grep_search truncated: showing %d results after offset %d]", len(results), options.offset))
	}
	return strings.Join(results, "\n"), nil
}

func (t *GrepSearchTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	params, err := parseParams(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	if !permissionContext.IsWithinReadableRootsResolved(params.Root) {
		return tool.DenyPermission(fmt.Sprintf("root %s is outside readable roots or escapes via symlink", params.Root))
	}
	return tool.AllowPermission()
}

func parseParams(input string) (grepSearchParams, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return grepSearchParams{}, nil
	}

	var params grepSearchParams
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return grepSearchParams{}, fmt.Errorf("参数格式错误，需要 JSON 对象: %w", err)
	}
	return params, nil
}

func normalizeMaxResults(value int) int {
	if value <= 0 {
		return defaultMaxMatches
	}
	if value > maxMatchesLimit {
		return maxMatchesLimit
	}
	return value
}

func normalizeOffset(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeMaxLineChars(value int) int {
	if value <= 0 {
		return defaultLineChars
	}
	if value > maxLineCharsLimit {
		return maxLineCharsLimit
	}
	return value
}

func normalizeOutputMode(value string) string {
	switch strings.TrimSpace(value) {
	case "", "content":
		return "content"
	case "files_with_matches", "count":
		return value
	default:
		return "content"
	}
}

func searchFiles(ctx context.Context, re *regexp.Regexp, options grepOptions, permissionContext tool.PermissionContext) ([]string, bool, error) {
	paths, err := matchingFiles(ctx, options.root, options.include, permissionContext)
	if err != nil {
		return nil, false, err
	}

	var results []string
	seen := 0
	truncated := false
	for _, path := range paths {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		binary, err := filesafe.LooksBinary(path)
		if err != nil {
			return nil, false, err
		}
		if binary {
			continue
		}

		match, err := searchFile(path, re, options)
		if err != nil {
			return nil, false, err
		}
		if match.count == 0 {
			continue
		}

		added, done := appendMatchResults(&results, match, options, &seen)
		truncated = done
		if added && len(results) >= options.maxResults {
			truncated = true
			break
		}
		if done {
			break
		}
	}
	return results, truncated, nil
}

func appendMatchResults(results *[]string, match fileMatch, options grepOptions, seen *int) (bool, bool) {
	added := false
	switch options.outputMode {
	case "files_with_matches":
		(*seen)++
		if *seen <= options.offset {
			return false, false
		}
		*results = append(*results, match.rel)
		added = true
	case "count":
		(*seen)++
		if *seen <= options.offset {
			return false, false
		}
		*results = append(*results, fmt.Sprintf("%s:%d", match.rel, match.count))
		added = true
	default:
		for _, line := range match.lines {
			(*seen)++
			if *seen <= options.offset {
				continue
			}
			*results = append(*results, fmt.Sprintf("%s:%d:%s", match.rel, line.number, line.content))
			added = true
			if len(*results) >= options.maxResults {
				return added, true
			}
		}
	}
	return added, len(*results) >= options.maxResults
}

func matchingFiles(ctx context.Context, root, include string, permissionContext tool.PermissionContext) ([]string, error) {
	var paths []string
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

		// Skip files that escape the workspace through symlinks.
		if !permissionContext.IsWithinReadableRootsResolved(path) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if filematch.Match(include, rel) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("搜索文件失败: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func searchFile(path string, re *regexp.Regexp, options grepOptions) (fileMatch, error) {
	result := fileMatch{path: path}
	file, err := os.Open(path)
	if err != nil {
		return result, fmt.Errorf("打开文件失败 %s: %w", path, err)
	}
	defer file.Close()

	rel, err := filepath.Rel(options.workspaceRoot, path)
	if err != nil {
		return result, err
	}
	result.rel = filepath.ToSlash(rel)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if re.MatchString(line) {
			result.count++
			result.lines = append(result.lines, lineMatch{
				number:  lineNumber,
				content: trimMatchLine(line, options.maxLineChars),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("读取文件失败 %s: %w", path, err)
	}
	return result, nil
}

func trimMatchLine(line string, maxChars int) string {
	line = strings.TrimSpace(line)
	runes := []rune(line)
	if len(runes) <= maxChars {
		return line
	}
	return string(runes[:maxChars]) + " [... line truncated]"
}

func init() {
	tool.MustRegisterTool(&GrepSearchTool{})
}
