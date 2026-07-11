package grepsearch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
	return "在本机目录内搜索文本文件内容，返回匹配文件、行号和匹配行；root 可用绝对路径、相对当前工作区路径或已上传附件目录名。会跳过二进制文件，扫描大目录时请设置 include/max_results；本地 runtime/命令依赖优先用 workspace_dependencies"
}

func (t *GrepSearchTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "query", Description: "要搜索的 Go 正则表达式", Type: "string", Required: true},
		{Name: "root", Description: "搜索根目录，可用绝对路径、相对当前工作区路径或已上传附件目录名；默认当前工作区", Type: "string", Required: false},
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
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name:        "grep_search",
		DisplayName: "Grep Search",
		Description: t.Description(),
		Purpose:     "用 Go 正则在本机文本文件内容中搜索，并返回文件、行号和有界匹配。",
		WhenToUse: []string{
			"需要查找符号定义、调用点、配置值或跨文件文本时",
			"需要按 include 范围筛选内容匹配时",
		},
		WhenNotToUse: []string{
			"只需要按文件名或扩展名枚举路径时使用 list_files",
			"查询互联网公开信息时使用 web_search",
		},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"query":       {Type: "string", Description: "实际 Go 正则"},
			"root":        {Type: "string", Description: "实际搜索根目录"},
			"include":     {Type: "string", Description: "实际文件 include 模式"},
			"output_mode": {Type: "string", Description: "实际输出模式"},
			"match_count": {Type: "integer", Description: "本次返回的匹配数量"},
			"offset":      {Type: "integer", Description: "本次结果起始偏移"},
			"max_results": {Type: "integer", Description: "本次结果上限"},
		}},
		SideEffects: []tool.ToolSideEffect{{Kind: tool.SideEffectRead, Resources: []string{"workspace text files"}, Description: "扫描文本内容，不修改文件。", Reversibility: tool.ReversibilityNotApplicable}},
		Examples:    []tool.ToolExample{{Scenario: "搜索函数定义", Input: map[string]any{"query": `func\s+ResolveDefinition\b`, "root": ".", "include": "**/*.go", "max_results": 100}, Outcome: "返回匹配文件、行号和文本"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "grep_search_query_required", Cause: "query 为空", Retryable: false, Recovery: []string{"提供非空正则"}},
			{Code: "grep_search_invalid_regex", Cause: "query 不是有效 Go 正则", Retryable: false, Recovery: []string{"修正正则表达式"}},
			{Code: "grep_search_root_unavailable", Cause: "搜索根不存在或不可读", Retryable: false, Recovery: []string{"检查 root 和任务可读范围"}},
			{Code: "grep_search_scan_failed", Cause: "文本扫描在执行阶段失败", Retryable: true, Recovery: []string{"缩小 include 和结果范围"}},
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

func (t *GrepSearchTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return outcome.Content, err
	}
	if outcome.IsError() {
		return "", fmt.Errorf("%s", outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

// ExecuteOutcome distinguishes invalid search requests from runtime scan
// failures and makes pagination/truncation state explicit for the agent.
func (t *GrepSearchTool) ExecuteOutcome(ctx context.Context, input string, _ tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	params, err := parseParams(input)
	if err != nil {
		outcome := tool.FailedOutcome(err.Error(), "grep_search_invalid_input", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_json_parameters"}
		outcome.Diagnostic = map[string]any{"expected_input": "JSON object"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if strings.TrimSpace(params.Query) == "" {
		outcome := tool.FailedOutcome("缺少必需参数: query", "grep_search_query_required", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"provide_non_empty_query"}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	root := permissionContext.ResolvePath(params.Root)
	if tool.HasPermissionContext(ctx) {
		checkedRoot, err := permissionContext.ResolveReadablePathChecked(params.Root)
		if err != nil {
			outcome := tool.FailedOutcome(err.Error(), "grep_search_root_unavailable", tool.ErrorCategoryDomain, false)
			outcome.RecoveryOptions = []string{"choose_readable_root", "search_workspace_root"}
			outcome.Diagnostic = map[string]any{"requested_root": params.Root}
			return outcome.WithTiming(started, time.Now().UTC()), nil
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
		message := fmt.Sprintf("query 不是有效正则表达式: %v", err)
		outcome := tool.FailedOutcome(message, "grep_search_invalid_regex", tool.ErrorCategoryValidation, false)
		outcome.RecoveryOptions = []string{"fix_regular_expression", "escape_literal_metacharacters"}
		outcome.Diagnostic = map[string]any{"query": params.Query}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}

	results, truncated, err := searchFiles(ctx, re, options, permissionContext)
	if err != nil {
		outcome := grepSearchFailureOutcome(err, options)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	resultCount := len(results)
	if len(results) == 0 {
		outcome := tool.SucceededOutcome("未找到匹配内容")
		outcome.Diagnostic = grepSearchDiagnostic(options, params.Query, 0, false)
		outcome.Details = grepSearchDetails(options, params.Query, 0)
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	if truncated {
		results = append(results, fmt.Sprintf("[grep_search truncated: showing %d results after offset %d]", len(results), options.offset))
	}
	outcome := tool.SucceededOutcome(strings.Join(results, "\n"))
	outcome.Truncated = truncated
	outcome.Diagnostic = grepSearchDiagnostic(options, params.Query, resultCount, truncated)
	outcome.Details = grepSearchDetails(options, params.Query, resultCount)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func grepSearchFailureOutcome(err error, options grepOptions) tool.ExecutionOutcome {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		outcome := tool.FailedOutcome(err.Error(), "grep_search_cancelled", tool.ErrorCategoryCancelled, true)
		outcome.Status = tool.OutcomeCancelled
		outcome.TransportStatus = tool.TransportCancelled
		outcome.RecoveryOptions = []string{"retry_when_context_available"}
		outcome.Diagnostic = grepSearchDiagnostic(options, "", 0, false)
		return outcome
	}
	outcome := tool.FailedOutcome(err.Error(), "grep_search_scan_failed", tool.ErrorCategoryRuntime, true)
	outcome.RecoveryOptions = []string{"inspect_root_access", "narrow_include_pattern", "retry_grep_search"}
	outcome.Diagnostic = grepSearchDiagnostic(options, "", 0, false)
	return outcome
}

func grepSearchDiagnostic(options grepOptions, query string, resultCount int, truncated bool) map[string]any {
	diagnostic := map[string]any{
		"root": options.root, "include": options.include, "output_mode": options.outputMode,
		"offset": options.offset, "max_results": options.maxResults,
		"result_count": resultCount, "match_count": resultCount, "truncated": truncated,
	}
	if strings.TrimSpace(query) != "" {
		diagnostic["query"] = query
	}
	return diagnostic
}

func grepSearchDetails(options grepOptions, query string, resultCount int) map[string]any {
	return map[string]any{
		"query": query, "root": options.root, "include": options.include,
		"output_mode": options.outputMode, "match_count": resultCount,
		"offset": options.offset, "max_results": options.maxResults,
	}
}

func (t *GrepSearchTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if _, err := parseParams(input); err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
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
	paths, err := matchingFiles(ctx, options.root, options.include)
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

func matchingFiles(ctx context.Context, root, include string) ([]string, error) {
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

	result.rel = displayPathForSearchFile(path, options.root, options.workspaceRoot)

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

func displayPathForSearchFile(path string, root string, workspaceRoot string) string {
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

func trimMatchLine(line string, maxChars int) string {
	line = strings.TrimSpace(line)
	runes := []rune(line)
	if len(runes) <= maxChars {
		return line
	}
	return string(runes[:maxChars]) + " [... line truncated]"
}

func init() {
	tool.MustRegisterToolWithMode(&GrepSearchTool{}, tool.DefinitionValidationStrictBuiltin)
}
